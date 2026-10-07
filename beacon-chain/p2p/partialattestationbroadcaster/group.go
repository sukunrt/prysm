package partialattestationbroadcaster

import (
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// All state in this file is owned by the Start loop; nothing here is safe
// for concurrent use.

const (
	// heartbeatInterval is the cadence of the Start loop's cleanup tick.
	heartbeatInterval = time.Second
	// sigTTLHeartbeats is how long a validated signature stays servable,
	// per attestation, sized to cover the advertise -> request -> serve trip.
	sigTTLHeartbeats = 6
	// maxIndicesPerClaim caps claim sets and wire index lists (= ssz_max).
	maxIndicesPerClaim = 2048
	// maxAttsPerBundle caps attestations per outgoing bundle (spec SHOULD);
	// a full bundle rolls over into a fresh one for the same key.
	maxAttsPerBundle = 50
	// validationWorkers is the number of goroutines that run validation jobs.
	validationWorkers = 5000
	// maxPushesPerVote caps the peers one vote is pushed to. Dhi (12) is below
	// it, so the mesh never loses a vote to the cap.
	maxPushesPerVote        = 16
	maxGroupsPerTopic       = 128 // we need 1 per slot
	maxAttDataGroupsPerSlot = 2048
	// advertiseWindow is how long a validated vote is advertised to gossip
	// peers, as IHAVE lists only recent messages.
	advertiseWindow = 3 * time.Second
	// seenEntriesWarn is the seen-cache size above which cleanup warns;
	// honest all-subnet load holds ~2M entries.
	seenEntriesWarn = 5_000_000
)

// seenTTLHeartbeats mirrors gossipsub's seen-message TTL of two epochs (see
// setPubSubParameters).
func seenTTLHeartbeats() uint64 {
	cfg := params.BeaconConfig()
	return 2 * uint64(cfg.SlotsPerEpoch) * cfg.SecondsPerSlot
}

// validatedAtt is one validator's live validated signature this slot.
type validatedAtt struct {
	CommitteeIndex primitives.CommitteeIndex
	Signature      []byte
	// AttHash is the HTR of the attested data, a key into slotAtts.attData.
	AttHash   string
	Committed time.Time
}

// attDataEntry is one live AttestationData and the number of validated
// signatures referencing it; the entry dies with its last reference.
type attDataEntry struct {
	data *ethpb.AttestationData
	refs int
}

// sigExpiry schedules one validated signature's expiry. Entries append in
// commit order with a constant TTL, so remaining TTLs never decrease front
// to back and expiry only ever pops the front.
type sigExpiry struct {
	validatorIdx primitives.ValidatorIndex
	ttl          uint64
}

// slotAtts holds one slot's state on one subnet topic.
type slotAtts struct {
	// attData holds the live datas by string(HTR(AttestationData)).
	attData map[string]*attDataEntry
	// validated holds each validator's live signature this slot (one vote per
	// validator per slot); the entry is deleted when the signature expires.
	validated map[primitives.ValidatorIndex]validatedAtt
	// expiry drives signature expiry in commit order.
	expiry []sigExpiry
	// meshPending holds validated votes not yet offered to the mesh, in commit
	// order. Every mesh walk reads all of it; a nil publish clears it.
	meshPending []primitives.ValidatorIndex
	// pushes counts the peers each vote was bundled for. The entry is created
	// at commit.
	pushes map[primitives.ValidatorIndex]uint8
	// sent marks validators already offered to the mesh: pushes are never
	// replayed, late mesh joiners catch up via requests, and once the validated
	// entry expires it is the remaining replay filter.
	sent map[primitives.ValidatorIndex]struct{}
	// pushDeadline is the end of the slot. cleanup sets pushClosed
	// once it passes; after that nothing enters or leaves the push structures.
	pushDeadline time.Time
	pushClosed   bool
}

// known reports whether the validator is already accounted for this slot:
// anything further from it is a replay or a slashable equivocation.
func (g *slotAtts) known(idx primitives.ValidatorIndex) bool {
	if _, ok := g.validated[idx]; ok {
		return true
	}
	_, ok := g.sent[idx]
	return ok
}

// ensureSlotGroup returns the slot group, creating it as needed; nil when the
// cap prevented it.
func (b *Broadcaster) ensureSlotGroup(topic string, slot primitives.Slot) *slotAtts {
	byTopic := b.groups[topic]
	if byTopic == nil {
		byTopic = make(map[primitives.Slot]*slotAtts)
		b.groups[topic] = byTopic
	}
	g := byTopic[slot]
	if g == nil {
		if len(byTopic) >= maxGroupsPerTopic {
			return nil
		}
		g = &slotAtts{
			attData:      make(map[string]*attDataEntry),
			validated:    make(map[primitives.ValidatorIndex]validatedAtt),
			pushes:       make(map[primitives.ValidatorIndex]uint8),
			sent:         make(map[primitives.ValidatorIndex]struct{}),
			pushDeadline: b.pushDue(slot),
		}
		byTopic[slot] = g
	}
	return g
}

// commitSig stores one validated signature and schedules its expiry; false
// when the data cap prevented tracking. The caller must have checked known.
func (g *slotAtts) commitSig(
	root string, data *ethpb.AttestationData, committee primitives.CommitteeIndex,
	idx primitives.ValidatorIndex, sig []byte, ttl uint64,
) bool {
	if g.known(idx) {
		return true
	}

	e := g.attData[root]
	if e == nil {
		if len(g.attData) >= maxAttDataGroupsPerSlot {
			return false
		}
		e = &attDataEntry{data: data}
		g.attData[root] = e
	}
	e.refs++
	g.validated[idx] = validatedAtt{
		CommitteeIndex: committee, Signature: sig, AttHash: root, Committed: time.Now(),
	}
	g.expiry = append(g.expiry, sigExpiry{validatorIdx: idx, ttl: ttl})
	if !g.pushClosed {
		g.meshPending = append(g.meshPending, idx)
		g.pushes[idx] = 0
	}
	return true
}

// settleMesh moves the mesh-pending votes into sent. It runs only after a
// publish that offered all of them to every mesh peer.
func (g *slotAtts) settleMesh() {
	for _, idx := range g.meshPending {
		g.sent[idx] = struct{}{}
	}
	g.meshPending = nil
}

// closePush ends every push for the slot: past the end of the slot only the
// request path serves it.
func (g *slotAtts) closePush() {
	g.meshPending = nil
	clear(g.pushes)
	g.pushClosed = true
}

// cleanup runs once per heartbeat: it closes the pushes of slots past their
// end, expires signatures and drops slots outside the
// propagation window; a slot's sent set stays for replay filtering until the
// slot itself is dropped.
func (b *Broadcaster) cleanup(current primitives.Slot) {
	now := time.Now()
	for topic, byTopic := range b.groups {
		for slot, g := range byTopic {
			if b.slotExpired(slot, current, now) {
				delete(byTopic, slot)
				continue
			}
			if !g.pushClosed && !now.Before(g.pushDeadline) {
				g.closePush()
			}
			g.expireSigs()
		}
		if len(byTopic) == 0 {
			delete(b.groups, topic)
		}
	}
	for id, ttl := range b.seen {
		if ttl <= 1 {
			delete(b.seen, id)
			continue
		}
		b.seen[id] = ttl - 1
	}
	if len(b.seen) > seenEntriesWarn {
		log.WithField("entries", len(b.seen)).Warn("Seen-tuple cache far above the honest envelope")
	}
}

// slotExpired reports whether a slot's votes are past the gossip window. From
// Heze a vote is valid only in its own slot, up to the clock disparity past its
// end; before Heze, up to the end of the next epoch (EIP-7045).
func (b *Broadcaster) slotExpired(slot, current primitives.Slot, now time.Time) bool {
	cfg := params.BeaconConfig()
	if slots.ToEpoch(slot) >= cfg.HezeForkEpoch {
		return now.After(b.pushDue(slot).Add(cfg.MaximumGossipClockDisparityDuration()))
	}
	return slots.ToEpoch(slot)+1 < slots.ToEpoch(current)
}

// expireSigs pops the expired front of the expiry queue, deleting each
// signature from validated and dropping its data with the last reference.
func (g *slotAtts) expireSigs() {
	for i := range g.expiry {
		g.expiry[i].ttl--
	}
	n := 0
	for n < len(g.expiry) && g.expiry[n].ttl == 0 {
		idx := g.expiry[n].validatorIdx
		v := g.validated[idx]
		delete(g.validated, idx)
		if e := g.attData[v.AttHash]; e != nil {
			e.refs--
			if e.refs == 0 {
				delete(g.attData, v.AttHash)
			}
		}
		n++
	}
	g.expiry = g.expiry[n:]
}

// sortedIndices returns the map's keys sorted ascending.
func sortedIndices[V any](set map[primitives.ValidatorIndex]V) []primitives.ValidatorIndex {
	indices := make([]primitives.ValidatorIndex, 0, len(set))
	for idx := range set {
		indices = append(indices, idx)
	}
	slices.Sort(indices)
	return indices
}

// capClaim truncates a sorted index list to the wire claim cap.
func capClaim(indices []primitives.ValidatorIndex) []primitives.ValidatorIndex {
	if len(indices) > maxIndicesPerClaim {
		return indices[:maxIndicesPerClaim]
	}
	return indices
}

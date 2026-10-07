// Package partialattestationbroadcaster implements bundled attestation
// propagation over the gossipsub partial-messages extension: the group ID is
// the slot, and bundles carry validator indices + signatures for one data.
package partialattestationbroadcaster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"iter"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialmsgmux"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("package", "beacon-chain/p2p/partialattestationbroadcaster")

// GroupIDLength is the length of a group ID: a little-endian uint64 slot.
const GroupIDLength = 8

// DefaultPushInterval is the push cadence when the caller gives none. The mesh
// push is tick-driven, so bursts of commits batch into one push per data group.
const DefaultPushInterval = 20 * time.Millisecond

// GroupID returns the partial-messages group ID for a slot.
func GroupID(slot primitives.Slot) []byte {
	groupID := make([]byte, GroupIDLength)
	binary.LittleEndian.PutUint64(groupID, uint64(slot))
	return groupID
}

func slotFromGroupID(groupID []byte) (primitives.Slot, error) {
	if len(groupID) != GroupIDLength {
		return 0, errors.Errorf("invalid group ID length: got %d, expected %d", len(groupID), GroupIDLength)
	}
	return primitives.Slot(binary.LittleEndian.Uint64(groupID)), nil
}

// incomingRPC is a decoded RPC handed from the gossipsub event loop to the
// Start loop. DataRoot is the bundle's HTR(AttestationData).
type incomingRPC struct {
	From     peer.ID
	Topic    string
	Slot     primitives.Slot
	Received time.Time
	Bundle   *ethpb.AttestationBundle
	DataRoot [32]byte
	Meta     *ethpb.CommitteeAttestationPartsMetadata
}

// ProcessAttestationFn replays one attestation through the classic gossip
// validator, which owns every check, and reports whether it was accepted.
// received is when the bundle's RPC arrived.
type ProcessAttestationFn func(
	topic string, att *ethpb.SingleAttestation, received time.Time,
) (bool, error)

// Broadcaster exchanges attestations with peers over the gossipsub
// partial-messages extension.
type Broadcaster struct {
	ctx         context.Context
	currentSlot func() primitives.Slot
	// pushDue reports the end of a slot, after which the slot's votes are
	// served on request only.
	pushDue func(primitives.Slot) time.Time

	// publishPartial pushes partial-message actions into gossipsub.
	publishPartial partialmsgmux.PublishPartialFn

	// incoming receives decoded RPCs from the gossipsub event loop.
	incoming chan incomingRPC

	// submit receives pre-validated attestations from local origination and
	// classic-gossip acceptance (the classic-path interop ingress).
	submit chan submission

	// valJobs/valDone carry work to and results from the validation
	// goroutine. Jobs are dropped (and left retryable) when valJobs is full.
	valJobs chan valJob
	valDone chan valDone

	// groups holds the store per topic and slot; seen maps each tuple
	// dispatched to validation to its remaining TTL. Owned by the Start loop.
	groups map[string]map[primitives.Slot]*slotAtts
	seen   map[[32]byte]uint64
	// queued holds the validators with a job in the lane, so an advertisement
	// of them is not fetched again. Owned by the Start loop.
	queued map[queuedKey]struct{}

	// gossip hands the library's gossip emissions to the Start loop; an
	// emission is droppable, so the channel is tiny.
	gossip chan gossipEmit

	// sigTTL is sigTTLHeartbeats, overridable in tests.
	sigTTL uint64
	// pushInterval is the cadence of the publish tick.
	pushInterval time.Duration
}

// gossipEmit is one library gossip emission: the peers to advertise a slot to.
type gossipEmit struct {
	topic string
	slot  primitives.Slot
	peers []peer.ID
}

// submission is a pre-validated attestation handed to the Start loop by
// Submit. It is never revalidated: replays hit the classic seen-caches.
type submission struct {
	Topic string
	Att   *ethpb.SingleAttestation
}

// valJob is one validation dispatch to the classic gossip pipeline. One job
// carries one attestation, so the queue bound counts signatures.
type valJob struct {
	topic    string
	dataRoot [32]byte
	att      *ethpb.SingleAttestation
	received time.Time
}

// queuedKey identifies one validator's slot vote in the validation lane.
type queuedKey struct {
	topic string
	slot  primitives.Slot
	idx   primitives.ValidatorIndex
}

// valDone is the result of a validation job.
type valDone struct {
	valJob
	accepted bool
}

var _ partialmsgmux.Handler = (*Broadcaster)(nil)

func NewBroadcaster(
	ctx context.Context, currentSlot func() primitives.Slot,
	pushDue func(primitives.Slot) time.Time, pushInterval time.Duration,
) *Broadcaster {
	if pushInterval <= 0 {
		pushInterval = DefaultPushInterval
	}
	return &Broadcaster{
		ctx:         ctx,
		currentSlot: currentSlot,
		pushDue:     pushDue,
		incoming:    make(chan incomingRPC, 1000),
		submit:      make(chan submission, 1000),
		// One job is one signature: ~1000 queued signatures drain in well
		// under a second (plan.md "Validation pipeline sizing").
		valJobs: make(chan valJob, 1000),
		valDone: make(chan valDone, 1000),
		groups:  make(map[string]map[primitives.Slot]*slotAtts),
		seen:    make(map[[32]byte]uint64),
		queued:  make(map[queuedKey]struct{}),
		gossip:  make(chan gossipEmit, 3),
		sigTTL:  sigTTLHeartbeats,

		pushInterval: pushInterval,
	}
}

// Start runs the blocking event loop owning all group state.
func (b *Broadcaster) Start(process ProcessAttestationFn) {
	for range validationWorkers {
		go b.validationLoop(process)
	}

	cleanupTicker := time.NewTicker(heartbeatInterval)
	defer cleanupTicker.Stop()
	pushTicker := time.NewTicker(b.pushInterval)
	defer pushTicker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case in := <-b.incoming:
			b.handleIncoming(in)
		case sub := <-b.submit:
			b.handleSubmission(sub)
		case d := <-b.valDone:
			b.handleValDone(d)
		case ge := <-b.gossip:
			b.emitGossip(ge)
		case <-cleanupTicker.C:
			b.cleanup(b.currentSlot())
		case <-pushTicker.C:
			b.flushDirty()
		}
	}
}

// validationLoop executes validation jobs one at a time. Each job waits one
// batch-verifier window, so the workers share the window and the batch.
func (b *Broadcaster) validationLoop(process ProcessAttestationFn) {
	for {
		select {
		case <-b.ctx.Done():
			return
		case j := <-b.valJobs:
			select {
			case b.valDone <- b.runValJob(process, j):
			case <-b.ctx.Done():
				return
			}
		}
	}
}

// runValJob validates a job through the classic pipeline; it can hit chain
// state and disk, so it runs on this lane, never on the Start loop.
func (b *Broadcaster) runValJob(process ProcessAttestationFn, j valJob) valDone {
	accepted, err := process(j.topic, j.att, j.received)
	if err != nil {
		log.WithError(err).WithField("topic", j.topic).Warn("Bundled attestation rejected")
	}
	return valDone{valJob: j, accepted: accepted}
}

// recordBundles counts sent or received bundles and, with the ledger on,
// writes one line per event for the run analysis. pending is the mesh push
// queue; the other paths pass zero.
func recordBundles(
	path string, slot primitives.Slot, bundles, signatures, peers, fresh, pending int,
) {
	if bundles == 0 {
		return
	}
	bundlesTotal.WithLabelValues(path).Add(float64(bundles))
	bundleSignatures.WithLabelValues(path).Observe(float64(signatures) / float64(bundles))
	if !features.Get().GoldfishVoteLedger {
		return
	}
	log.WithFields(logrus.Fields{
		"path":       path,
		"slot":       slot,
		"bundles":    bundles,
		"signatures": signatures,
		"peers":      peers,
		"new":        fresh,
		"pending":    pending,
	}).Info("Attestation bundles")
}

func (b *Broadcaster) handleIncoming(in incomingRPC) {
	if in.Bundle != nil {
		b.handleBundle(in)
	}
	if in.Meta != nil {
		b.handleMeta(in)
	}
}

// handleBundle filters a received bundle's attestations against the store
// and hands each new one to the validation lane, one job per attestation.
func (b *Broadcaster) handleBundle(in incomingRPC) {
	bundle := in.Bundle
	g := b.groups[in.Topic][in.Slot]
	fresh := 0
entries:
	for i, idx := range bundle.AttesterIndices {
		vIdx := primitives.ValidatorIndex(idx)
		// A known validator is final for the slot: anything further from it
		// is a replay or a slashable equivocation.
		if g != nil && g.known(vIdx) {
			continue
		}
		// The seen cache drops tuples already dispatched to validation.
		id := attTupleID(in.DataRoot, bundle.CommitteeIndex, vIdx, bundle.Signatures[i])
		if b.seenTuple(id) {
			continue
		}
		job := valJob{
			topic:    in.Topic,
			dataRoot: in.DataRoot,
			att: &ethpb.SingleAttestation{
				CommitteeId:   bundle.CommitteeIndex,
				AttesterIndex: vIdx,
				Data:          bundle.AttestationData,
				Signature:     bundle.Signatures[i],
			},
			received: in.Received,
		}
		select {
		case b.valJobs <- job:
			// Marked seen only once queued: a dropped job stays retryable.
			b.markTupleSeen(id)
			b.queued[queuedKey{in.Topic, in.Slot, vIdx}] = struct{}{}
			fresh++
		default:
			log.WithFields(logrus.Fields{"from": in.From, "topic": in.Topic}).
				Warn("Validation queue full, dropping bundle")
			break entries
		}
	}
	recordBundles("recv", in.Slot, 1, len(bundle.AttesterIndices), 1, fresh, 0)
}

// handleMeta answers a peer's metadata: it serves the peer's requests and
// requests the advertised votes we lack. Nothing is pushed to the peer.
func (b *Broadcaster) handleMeta(in incomingRPC) {
	if b.publishPartial == nil {
		return
	}
	g := b.groups[in.Topic][in.Slot]

	// Build the fetch request upfront: what the peer advertises beyond our
	// holdings, with no available list of our own.
	var fetchEnc []byte
	var want []uint64
	for _, idx := range in.Meta.Available {
		vIdx := primitives.ValidatorIndex(idx)
		if _, ok := b.queued[queuedKey{in.Topic, in.Slot, vIdx}]; ok || g != nil && g.known(vIdx) {
			continue
		}
		want = append(want, idx)
		if len(want) >= maxIndicesPerClaim {
			break
		}
	}
	if len(want) > 0 {
		meta := &ethpb.CommitteeAttestationPartsMetadata{
			CommitteeIndex: in.Meta.CommitteeIndex,
			Requests:       want,
		}
		enc, err := meta.MarshalSSZ()
		if err != nil {
			log.WithError(err).Debug("Could not marshal fetch request")
		} else {
			fetchEnc = enc
		}
	}

	b.respondMeta(in, g, fetchEnc)
}

// respondMeta answers a peer's metadata in a single publishPartial call:
// requests are answered from live signatures the peer does not already have,
// and advertised validators we lack are requested back. Requests and
// advertisements never mix: an answer carries only bundles, a fetch carries
// only requests. Duplicate responses from concurrent advertisers die in
// handleBundle's filters.
func (b *Broadcaster) respondMeta(in incomingRPC, g *slotAtts, fetchEnc []byte) {
	serve := g != nil && len(g.validated) > 0 && len(in.Meta.Requests) > 0
	if !serve && fetchEnc == nil {
		return
	}
	// The closure reads the store directly: the caller is parked in the
	// synchronous publishPartial while it runs on the GS event loop, which
	// owns the peer states the served bundles are diffed against.
	sentBundles, sentSignatures := 0, 0
	fn := func(peerStates map[peer.ID]blocks.PartialMessagePeerState, peerRequestsPartial func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
		return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
			if serve && peerRequestsPartial(in.From) {
				ps, tracked := peerStates[in.From]
				// Answer the requests from live signatures the peer lacks,
				// one bundle per (data, committee). served dedups the request
				// list: a duplicate index would make our outgoing bundle
				// invalid at the receiver.
				byKey := make(map[bundleKey]*ethpb.AttestationBundle)
				var bundles []*ethpb.AttestationBundle
				served := make(map[primitives.ValidatorIndex]struct{}, len(in.Meta.Requests))
				for _, r := range in.Meta.Requests {
					idx := primitives.ValidatorIndex(r)
					if _, ok := served[idx]; ok {
						continue
					}
					served[idx] = struct{}{}
					v, ok := g.validated[idx]
					if !ok {
						continue
					}
					if _, ok := ps.Att.Available[v.CommitteeIndex][r]; ok {
						continue
					}
					k := bundleKey{root: v.AttHash, committee: v.CommitteeIndex}
					bundle := byKey[k]
					if bundle == nil || len(bundle.AttesterIndices) >= maxAttsPerBundle {
						bundle = &ethpb.AttestationBundle{
							CommitteeIndex:  v.CommitteeIndex,
							AttestationData: g.attData[v.AttHash].data,
						}
						byKey[k] = bundle
						bundles = append(bundles, bundle)
					}
					bundle.AttesterIndices = append(bundle.AttesterIndices, r)
					bundle.Signatures = append(bundle.Signatures, v.Signature)
				}
				for _, bundle := range bundles {
					encoded, err := bundle.MarshalSSZ()
					if err != nil {
						log.WithError(err).Debug("Could not marshal serve bundle")
						continue
					}
					// The served positions fold into the requester's
					// availability, so a replayed request goes quiet.
					recordPeerIndices(&ps.Att, bundle.CommitteeIndex, bundle.AttesterIndices)
					// A write back after the peer left the map would re-insert
					// it as unknown, which the next flush treats as mesh.
					if tracked {
						peerStates[in.From] = ps
					}
					sentBundles++
					sentSignatures += len(bundle.AttesterIndices)
					if !yield(in.From, partialmessages.PublishAction{EncodedPartialMessage: encoded}) {
						return
					}
				}
			}
			if fetchEnc != nil {
				yield(in.From, partialmessages.PublishAction{EncodedPartsMetadata: fetchEnc})
			}
		}
	}
	if err := b.publishPartial(in.Topic, GroupID(in.Slot), fn); err != nil {
		log.WithError(err).WithField("topic", in.Topic).
			Warn("Could not respond to attestation metadata")
	}
	recordBundles("serve", in.Slot, sentBundles, sentSignatures, 1, 0, 0)
}

// attTupleID hashes (data root, committee, validator index, signature): the
// deduplication identity of one attestation on the wire.
func attTupleID(
	dataRoot [32]byte, committeeIndex primitives.CommitteeIndex,
	idx primitives.ValidatorIndex, sig []byte,
) [32]byte {
	buf := make([]byte, 0, 32+8+8+96)
	buf = append(buf, dataRoot[:]...)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(committeeIndex))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(idx))
	buf = append(buf, sig...)
	return sha256.Sum256(buf)
}

func (b *Broadcaster) seenTuple(id [32]byte) bool {
	_, ok := b.seen[id]
	return ok
}

func (b *Broadcaster) markTupleSeen(id [32]byte) {
	b.seen[id] = seenTTLHeartbeats()
}

// handleValDone commits an accepted attestation.
func (b *Broadcaster) handleValDone(d valDone) {
	delete(b.queued, queuedKey{d.topic, d.att.Data.Slot, d.att.AttesterIndex})
	if !d.accepted {
		return
	}
	b.commitAttestation(d.topic, d.dataRoot, d.att)
}

// commitAttestation stores one validated attestation and queues it for the
// mesh; a known validator is dropped, which also neutralizes the origination
// feedback loops.
func (b *Broadcaster) commitAttestation(
	topic string, dataRoot [32]byte, att *ethpb.SingleAttestation,
) {
	slot, idx := att.Data.Slot, att.AttesterIndex
	g := b.ensureSlotGroup(topic, slot)
	if g == nil {
		log.WithFields(logrus.Fields{"topic": topic, "slot": slot}).
			Warn("Attestation group cap hit, not tracking")
		return
	}
	if !g.commitSig(string(dataRoot[:]), att.Data, att.CommitteeId, idx, att.Signature, b.sigTTL) {
		log.WithFields(logrus.Fields{"topic": topic, "slot": slot}).
			Warn("Attestation data cap hit, not tracking")
		return
	}
	log.WithFields(logrus.Fields{
		"topic":     topic,
		"slot":      slot,
		"validator": idx,
	}).Debug("Stored validated attestation")
}

// Submit feeds an already-validated attestation into the store so partial
// peers receive it while gossipsub withholds the classic copy.
func (b *Broadcaster) Submit(topic string, att *ethpb.SingleAttestation) {
	if att == nil || att.Data == nil {
		return
	}
	if !b.slotInPropagationWindow(att.Data.Slot) {
		return
	}
	select {
	case b.submit <- submission{Topic: topic, Att: att}:
	default:
		log.WithField("topic", topic).Warn("Submit queue full, dropping attestation")
	}
}

// handleSubmission commits a pre-validated attestation inline.
func (b *Broadcaster) handleSubmission(sub submission) {
	dataRoot, err := sub.Att.Data.HashTreeRoot()
	if err != nil {
		log.WithError(err).Debug("Could not hash submitted attestation data")
		return
	}
	b.commitAttestation(sub.Topic, dataRoot, sub.Att)
}

// bundleKey groups outgoing signatures into one bundle per (data, committee).
type bundleKey struct {
	root      string
	committee primitives.CommitteeIndex
}

// flushDirty offers every mesh-pending vote to the mesh, one publish per
// group. A group with nothing pending is not published at all.
func (b *Broadcaster) flushDirty() {
	for topic, byTopic := range b.groups {
		for slot, g := range byTopic {
			if len(g.meshPending) == 0 {
				continue
			}
			b.flushGroup(topic, slot, g)
		}
	}
}

// flushGroup runs one publish for the group: it offers meshPending to every
// mesh peer, then settles what was offered. Gossip peers get only the
// advertisement and fetch what they lack.
func (b *Broadcaster) flushGroup(topic string, slot primitives.Slot, g *slotAtts) {
	if g.pushClosed || b.publishPartial == nil {
		return
	}
	aborted := false
	sentBundles, sentSignatures, sentPeers := 0, 0, 0
	// The closure reads the store directly: the caller is parked in the
	// synchronous publishPartial while it runs on the GS event loop, which owns
	// the peer states the bundles are diffed against.
	fn := func(
		peerStates map[peer.ID]blocks.PartialMessagePeerState,
		peerRequestsPartial func(peer.ID) bool,
	) iter.Seq2[peer.ID, partialmessages.PublishAction] {
		return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
			for _, pid := range meshPeers(peerStates, peerRequestsPartial) {
				ps := peerStates[pid]
				byKey := make(map[bundleKey]*ethpb.AttestationBundle)
				var bundles []*ethpb.AttestationBundle
				for _, idx := range g.meshPending {
					v, ok := g.validated[idx]
					if !ok {
						continue
					}
					if _, ok := ps.Att.Available[v.CommitteeIndex][uint64(idx)]; ok {
						continue
					}
					k := bundleKey{root: v.AttHash, committee: v.CommitteeIndex}
					bundle := byKey[k]
					if bundle == nil || len(bundle.AttesterIndices) >= maxAttsPerBundle {
						bundle = &ethpb.AttestationBundle{
							CommitteeIndex:  v.CommitteeIndex,
							AttestationData: g.attData[v.AttHash].data,
						}
						byKey[k] = bundle
						bundles = append(bundles, bundle)
					}
					bundle.AttesterIndices = append(bundle.AttesterIndices, uint64(idx))
					bundle.Signatures = append(bundle.Signatures, v.Signature)
				}
				if len(bundles) > 0 {
					sentPeers++
				}
				for _, bundle := range bundles {
					encoded, err := bundle.MarshalSSZ()
					if err != nil {
						log.WithError(err).Debug("Could not marshal push bundle")
						continue
					}
					recordPeerIndices(&ps.Att, bundle.CommitteeIndex, bundle.AttesterIndices)
					peerStates[pid] = ps
					for _, idx := range bundle.AttesterIndices {
						g.pushes[primitives.ValidatorIndex(idx)]++
					}
					sentBundles++
					sentSignatures += len(bundle.AttesterIndices)
					if !yield(pid, partialmessages.PublishAction{EncodedPartialMessage: encoded}) {
						aborted = true
						return
					}
				}
			}
		}
	}
	if err := b.publishPartial(topic, GroupID(slot), fn); err != nil {
		log.WithError(err).WithField("topic", topic).
			Warn("Could not publish partial attestation push")
	} else if !aborted {
		g.settleMesh()
	}
	recordBundles("push", slot, sentBundles, sentSignatures, sentPeers, 0, len(g.meshPending))
}

// meshPeers returns the group's mesh peers that request partial messages, in
// a stable order. Unknown entries are the library's mesh additions; remote
// entries are dropped so a mesh peer that sent to us first is pushed to again.
func meshPeers(
	peerStates map[peer.ID]blocks.PartialMessagePeerState, peerRequestsPartial func(peer.ID) bool,
) []peer.ID {
	var mesh []peer.ID
	for pid, ps := range peerStates {
		switch ps.Att.Kind {
		case blocks.PeerUnknown:
			ps.Att.Kind = blocks.PeerMesh
			peerStates[pid] = ps
			fallthrough
		case blocks.PeerMesh:
			if peerRequestsPartial(pid) {
				mesh = append(mesh, pid)
			}
		case blocks.PeerRemote:
			delete(peerStates, pid)
		}
	}
	// A stable walk order keeps the push deterministic across ticks.
	slices.Sort(mesh)
	return mesh
}

// emitGossip sends the per-committee Available advertisement of the votes
// validated within advertiseWindow once to the gossip peers: a receiver that
// wants the data requests it, and the request/serve path delivers.
func (b *Broadcaster) emitGossip(ge gossipEmit) {
	if b.publishPartial == nil {
		return
	}
	g := b.groups[ge.topic][ge.slot]
	if g == nil || g.pushClosed {
		return
	}
	byCommittee := make(map[primitives.CommitteeIndex][]primitives.ValidatorIndex, 1)
	for _, idx := range sortedIndices(g.validated) {
		v := g.validated[idx]
		if time.Since(v.Committed) > advertiseWindow {
			continue
		}
		byCommittee[v.CommitteeIndex] = append(byCommittee[v.CommitteeIndex], idx)
	}
	for ci, indices := range byCommittee {
		indices = capClaim(indices)
		avail := make([]uint64, len(indices))
		for i, idx := range indices {
			avail[i] = uint64(idx)
		}
		meta := &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: ci, Available: avail}
		encoded, err := meta.MarshalSSZ()
		if err != nil {
			log.WithError(err).Debug("Could not marshal parts metadata")
			continue
		}
		// The advertisement goes to ge.peers directly; OnEmitGossip already
		// dropped them from peerStates.
		fn := func(map[peer.ID]blocks.PartialMessagePeerState, func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
			return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
				for _, pid := range ge.peers {
					if !yield(pid, partialmessages.PublishAction{EncodedPartsMetadata: encoded}) {
						return
					}
				}
			}
		}
		if err := b.publishPartial(ge.topic, GroupID(ge.slot), fn); err != nil {
			log.WithError(err).WithField("topic", ge.topic).Debug("Could not emit gossip advertisement")
		}
	}
}

// InitPubSub receives the pubsub hooks from the mux at construction time.
func (b *Broadcaster) InitPubSub(_ partialmsgmux.PeerFeedbackFn, publishPartial partialmsgmux.PublishPartialFn) {
	b.publishPartial = publishPartial
}

// slotInPropagationWindow is the group-ID pre-filter: at most one slot ahead
// of our clock and not expired.
func (b *Broadcaster) slotInPropagationWindow(slot primitives.Slot) bool {
	current := b.currentSlot()
	return slot <= current+1 && !b.slotExpired(slot, current, time.Now())
}

// OnEmitGossip runs on the gossipsub event loop. It drops the gossip targets
// from the map, so the flush never pushes to them and the library can pick
// them again, and hands them to the Start loop for one advertisement.
func (b *Broadcaster) OnEmitGossip(topic string, groupID []byte, peers []peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState) {
	slot, err := slotFromGroupID(groupID)
	if err != nil {
		return
	}
	for _, pid := range peers {
		delete(peerStates, pid)
	}
	select {
	case b.gossip <- gossipEmit{topic: topic, slot: slot, peers: peers}:
	default: // dropping the emission is fine, as for IHAVE
	}
}

// OnIncomingRPC runs on the gossipsub event loop: structural checks and peer
// state only, then a non-blocking handoff to the Start loop.
func (b *Broadcaster) OnIncomingRPC(from peer.ID, peerStates map[peer.ID]blocks.PartialMessagePeerState, rpc *pubsub_pb.PartialMessagesExtension) error {
	slot, err := slotFromGroupID(rpc.GetGroupID())
	if err != nil {
		return err
	}
	if !b.slotInPropagationWindow(slot) {
		// The spec says ignore out-of-window groups, not penalize them.
		log.WithFields(logrus.Fields{"from": from, "slot": slot}).Debug("Ignoring group outside the attestation propagation window")
		return nil
	}

	// Claims are recorded even if the handoff drops: they stay true and only
	// ever suppress sends to the claimant.
	in := incomingRPC{From: from, Topic: rpc.GetTopicID(), Slot: slot, Received: time.Now()}
	st, tracked := peerStates[from]
	if msg := rpc.GetPartialMessage(); len(msg) > 0 {
		bundle := &ethpb.AttestationBundle{}
		if err := bundle.UnmarshalSSZ(msg); err != nil {
			return errors.Wrap(err, "unmarshal attestation bundle")
		}
		if err := checkBundle(bundle, slot); err != nil {
			return err
		}
		dataRoot, err := bundle.AttestationData.HashTreeRoot()
		if err != nil {
			return errors.Wrap(err, "hash bundle attestation data")
		}
		recordPeerIndices(&st.Att, bundle.CommitteeIndex, bundle.AttesterIndices)
		in.Bundle, in.DataRoot = bundle, dataRoot
	}
	if md := rpc.GetPartsMetadata(); len(md) > 0 {
		meta := &ethpb.CommitteeAttestationPartsMetadata{}
		if err := meta.UnmarshalSSZ(md); err != nil {
			return errors.Wrap(err, "unmarshal parts metadata")
		}
		recordPeerIndices(&st.Att, meta.CommitteeIndex, meta.Available)
		in.Meta = meta
	}
	// An untracked sender is remote: the flush never pushes to it and drops
	// it. A sender that advertises gets only what it requests.
	if !tracked {
		st.Att.Kind = blocks.PeerRemote
	}
	if peerStates != nil {
		peerStates[from] = st
	}
	if in.Bundle == nil && in.Meta == nil {
		return nil
	}

	select {
	case b.incoming <- in:
	default:
		log.WithField("from", from).Warn("Dropping incoming partial attestation RPC, queue full")
	}
	return nil
}

// checkBundle runs the structural checks that need no chain state; duplicate
// indices are a wire error, so downstream sees one signature per validator.
func checkBundle(bundle *ethpb.AttestationBundle, slot primitives.Slot) error {
	if bundle.GetAttestationData().GetSlot() != slot {
		return errors.Errorf("bundle slot %d does not match group slot %d", bundle.GetAttestationData().GetSlot(), slot)
	}
	if len(bundle.AttesterIndices) != len(bundle.Signatures) {
		return errors.Errorf("bundle has %d attester indices but %d signatures", len(bundle.AttesterIndices), len(bundle.Signatures))
	}
	seen := make(map[uint64]struct{}, len(bundle.AttesterIndices))
	for _, idx := range bundle.AttesterIndices {
		if _, ok := seen[idx]; ok {
			return errors.Errorf("duplicate attester index %d in bundle", idx)
		}
		seen[idx] = struct{}{}
	}
	return nil
}

// maxCommitteesPerPeer caps tracked committees per peer per slot group.
const maxCommitteesPerPeer = 64

// recordPeerIndices folds indices into the peer's per-committee availability;
// claims are attacker-sized, so both dimensions are capped.
func recordPeerIndices(ps *blocks.PartialAttestationPeerState, committee primitives.CommitteeIndex, indices []uint64) {
	set := ps.Available[committee]
	if set == nil {
		if len(ps.Available) >= maxCommitteesPerPeer {
			log.WithField("committee", committee).Debug("Peer availability committee cap hit, ignoring claim")
			return
		}
		if ps.Available == nil {
			ps.Available = make(map[primitives.CommitteeIndex]map[uint64]struct{})
		}
		set = make(map[uint64]struct{}, len(indices))
		ps.Available[committee] = set
	}
	for _, idx := range indices {
		if len(set) >= maxIndicesPerClaim {
			return
		}
		set[idx] = struct{}{}
	}
}

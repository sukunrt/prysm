package doublylinkedtree

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// goldfishVoteRetention is how many slots behind the current one available
// attestation votes are kept for. The walk only ever reads the previous slot;
// the extra slack absorbs late arrivals and clock disparity.
const goldfishVoteRetention = primitives.Slot(3)

// goldfishVote is one validator's available attestation vote for one slot.
type goldfishVote struct {
	root           [32]byte
	payloadPresent bool
	// seats is the number of available committee seats the signer holds in the
	// vote's slot. The committee is a fixed size list with repeats, so a
	// validator holding k seats counts k in both score and denominator.
	seats uint64
}

// goldfishVotes holds the available attestation votes that drive the Goldfish
// head walk, keyed by slot and then by validator index.
//
// This is deliberately NOT the `f.votes` store: those are epoch granular and
// never expire, while an available attestation vote is only read during the
// slot after the one it names.
//
// The caller must hold the forkchoice write lock for every method here.
type goldfishVotes struct {
	votes map[primitives.Slot]map[primitives.ValidatorIndex]goldfishVote
	// equivocators are validators that signed two different available
	// attestations for the same slot. Their first vote stays in `votes` so
	// they keep counting in the viability denominator.
	equivocators map[primitives.Slot]map[primitives.ValidatorIndex]bool
}

func newGoldfishVotes() *goldfishVotes {
	return &goldfishVotes{
		votes:        make(map[primitives.Slot]map[primitives.ValidatorIndex]goldfishVote),
		equivocators: make(map[primitives.Slot]map[primitives.ValidatorIndex]bool),
	}
}

// insert records a validator's vote for the given slot. A second vote with
// different content moves the validator to the slot's equivocation set; the
// first vote is kept so the validator still counts in the denominator.
func (g *goldfishVotes) insert(slot primitives.Slot, index primitives.ValidatorIndex, v goldfishVote) {
	if g.equivocators[slot][index] {
		return
	}
	byIndex, ok := g.votes[slot]
	if !ok {
		byIndex = make(map[primitives.ValidatorIndex]goldfishVote)
		g.votes[slot] = byIndex
	}
	previous, ok := byIndex[index]
	if !ok {
		byIndex[index] = v
		return
	}
	if previous.root == v.root && previous.payloadPresent == v.payloadPresent {
		return
	}
	eq, ok := g.equivocators[slot]
	if !ok {
		eq = make(map[primitives.ValidatorIndex]bool)
		g.equivocators[slot] = eq
	}
	eq[index] = true
	goldfishEquivocationCount.Inc()
}

// prune drops every slot that is more than goldfishVoteRetention behind the
// given slot.
func (g *goldfishVotes) prune(current primitives.Slot) {
	for slot := range g.votes {
		if slot+goldfishVoteRetention < current {
			delete(g.votes, slot)
			delete(g.equivocators, slot)
		}
	}
	for slot := range g.equivocators {
		if slot+goldfishVoteRetention < current {
			delete(g.equivocators, slot)
		}
	}
}

// seats returns the number of committee seats that voted for the given slot,
// equivocators included. This is the electorate the majority gate divides.
func (g *goldfishVotes) seats(slot primitives.Slot) uint64 {
	total := uint64(0)
	for _, v := range g.votes[slot] {
		total += v.seats
	}
	return total
}

// InsertAvailableAttestation records a validated available attestation vote.
// The caller MUST hold the forkchoice write lock.
func (f *ForkChoice) InsertAvailableAttestation(
	slot primitives.Slot,
	index primitives.ValidatorIndex,
	seats uint64,
	root [32]byte,
	payloadPresent bool,
) {
	if seats == 0 {
		return
	}
	s := f.store
	if slot < s.currentSlot() {
		goldfishLateVoteCount.Inc()
	}
	s.goldfishVotes.insert(slot, index, goldfishVote{
		root:           root,
		payloadPresent: payloadPresent,
		seats:          seats,
	})
}

// goldfishActive reports whether the Goldfish head vote is the head rule, that
// is whether the wall clock has reached the Heze fork. Shipped configs keep
// HezeForkEpoch at the far future epoch, so this is false everywhere but on
// the devnet, e2e and simulation configs.
func (s *Store) goldfishActive() bool {
	return goldfishActiveAt(s.currentSlot())
}

// goldfishActiveAt reports whether the given slot is at or after the Heze fork.
func goldfishActiveAt(slot primitives.Slot) bool {
	return slots.ToEpoch(slot) >= params.BeaconConfig().HezeForkEpoch
}

// goldfishNewSlot prunes the vote store and reports the share of the committee
// that was heard from for the slot that just ended. Called from NewSlot, that
// is at the slot boundary, which is exactly the "before the next slot start"
// cutoff the metric names.
func (s *Store) goldfishNewSlot(slot primitives.Slot) {
	if slot > 0 {
		seats := s.goldfishVotes.seats(slot - 1)
		goldfishSeatFraction.Set(float64(seats) / float64(decoupled.AvailableAttestationCommitteeSize))
	}
	s.goldfishVotes.prune(slot)
}

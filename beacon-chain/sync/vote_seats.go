package sync

import (
	"math"
	"sync"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// voteSeats counts the head vote seats of a slot as they are taken in. It is
// package level because a node's own vote is recorded by the RPC, which has no
// *Service. It counts messages, not validators, so an equivocator counts twice.
var voteSeats = &slotSeatCounter{m: make(map[primitives.Slot]uint64)}

type slotSeatCounter struct {
	mu sync.Mutex
	m  map[primitives.Slot]uint64
}

func (c *slotSeatCounter) add(slot primitives.Slot, seats uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[slot] += seats
}

// take reads a slot's count and forgets it and everything older: the slot has
// ended, so no later vote for it is counted.
func (c *slotSeatCounter) take(slot primitives.Slot) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seats := c.m[slot]
	for s := range c.m {
		if s <= slot {
			delete(c.m, s)
		}
	}
	return seats
}

// runVoteSeatJobs publishes the two per-slot seat counts at the points their
// consumers read them: fork choice takes a slot's Goldfish seats at the next
// slot start, the aggregator takes the FFG pool at the aggregate due.
func (s *Service) runVoteSeatJobs() {
	clock, err := s.clockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("Failed to receive clock for vote seat job routine")
		return
	}
	cfg := params.BeaconConfig()
	aggregateDue := cfg.SlotComponentDuration(cfg.AggregateDueBPS)
	if cfg.GloasForkEpoch != math.MaxUint64 {
		// The intervals are fixed here, so a fork later in the run keeps the old
		// point until the node restarts.
		aggregateDue = cfg.SlotComponentDuration(cfg.AggregateDueBPSGloas)
	}
	slot := time.Duration(cfg.SlotDurationMillis()) * time.Millisecond
	if aggregateDue >= slot {
		log.WithField("aggregateDue", aggregateDue).
			Error("Aggregate due point does not fit the slot, not running the seat jobs")
		return
	}
	ticker := slots.NewSlotTickerWithIntervals(clock.GenesisTime(), []time.Duration{0, aggregateDue})
	defer ticker.Done()
	for {
		select {
		case tick := <-ticker.C():
			if tick.Interval == 0 {
				if tick.Slot > 0 {
					goldfishVoteSeats.Set(float64(voteSeats.take(tick.Slot - 1)))
				}
				continue
			}
			s.recordFFGSeats(tick.Slot)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting vote seat job routine")
			return
		}
	}
}

// recordFFGSeats reads the attestation pool for the committees this node
// subscribes to and publishes how much of them has attested by the aggregate
// due point. It is the same read an aggregator does for its duty, run on every
// node whether or not it holds one.
func (s *Service) recordFFGSeats(slot primitives.Slot) {
	if s.cfg.initialSync.Syncing() {
		return
	}
	if slots.ToEpoch(slot) < params.BeaconConfig().ElectraForkEpoch {
		return
	}
	st, err := s.cfg.chain.HeadStateReadOnly(s.ctx)
	if err != nil {
		log.WithError(err).Debug("Could not get head state for FFG seat count")
		return
	}
	committees, err := helpers.BeaconCommittees(s.ctx, st, slot)
	if err != nil {
		log.WithError(err).Debug("Could not get committees for FFG seat count")
		return
	}
	subnets := s.persistentAndAggregatorSubnetIndices(slot)
	var seats, expected uint64
	for i, committee := range committees {
		if len(committee) == 0 {
			continue
		}
		idx := primitives.CommitteeIndex(i)
		if !subnets[helpers.ComputeSubnetForCommitteesPerSlot(uint64(len(committees)), idx, slot)] {
			continue
		}
		attested := s.attestedSeats(slot, idx, uint64(len(committee)))
		seats += attested
		expected += uint64(len(committee))
		ffgCommitteeSeatFraction.Observe(float64(attested) / float64(len(committee)))
	}
	ffgVoteSeats.Set(float64(seats))
	ffgExpectedSeats.Set(float64(expected))
}

// attestedSeats is the size of the union of the aggregation bits the pool holds
// for one committee. The pool folds an unaggregated attestation into an
// aggregate and drops it, so both lists are read.
func (s *Service) attestedSeats(
	slot primitives.Slot, idx primitives.CommitteeIndex, size uint64,
) uint64 {
	var atts []ethpb.Att
	if features.Get().EnableExperimentalAttestationPool {
		atts = cache.GetBySlotAndCommitteeIndex[ethpb.Att](s.cfg.attestationCache, slot, idx)
	} else {
		for _, att := range s.cfg.attPool.AggregatedAttestationsBySlotIndexElectra(s.ctx, slot, idx) {
			atts = append(atts, att)
		}
		for _, att := range s.cfg.attPool.UnaggregatedAttestationsBySlotIndexElectra(s.ctx, slot, idx) {
			atts = append(atts, att)
		}
	}
	union := bitfield.NewBitlist(size)
	for _, att := range atts {
		bits := att.GetAggregationBits()
		if bits.Len() != size {
			continue
		}
		merged, err := union.Or(bits)
		if err != nil {
			log.WithError(err).Debug("Could not union FFG aggregation bits")
			continue
		}
		union = merged
	}
	return union.Count()
}

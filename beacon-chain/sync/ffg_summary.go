package sync

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// ffgSubnetCount is one subnet's share of one slot's accepted FFG votes.
type ffgSubnetCount struct {
	votes uint64
	seats uint64
}

// ffgVoteCounters counts the FFG votes the node accepted off the attestation
// subnets, keyed by the vote's own slot and its subnet. The counters of a slot
// are read and dropped once, when that slot's summary line is written.
type ffgVoteCounters struct {
	mu     sync.Mutex
	bySlot map[primitives.Slot]map[uint64]*ffgSubnetCount
}

func newFFGVoteCounters() *ffgVoteCounters {
	return &ffgVoteCounters{bySlot: make(map[primitives.Slot]map[uint64]*ffgSubnetCount)}
}

func (c *ffgVoteCounters) count(slot primitives.Slot, subnet, seats uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bySubnet, ok := c.bySlot[slot]
	if !ok {
		bySubnet = make(map[uint64]*ffgSubnetCount)
		c.bySlot[slot] = bySubnet
	}
	entry, ok := bySubnet[subnet]
	if !ok {
		entry = &ffgSubnetCount{}
		bySubnet[subnet] = entry
	}
	entry.votes++
	entry.seats += seats
}

// take returns the counters of the given slot and drops every slot up to and
// including it, so a late vote for an older slot cannot pile up.
func (c *ffgVoteCounters) take(slot primitives.Slot) map[uint64]ffgSubnetCount {
	c.mu.Lock()
	defer c.mu.Unlock()
	taken := make(map[uint64]ffgSubnetCount, len(c.bySlot[slot]))
	for subnet, entry := range c.bySlot[slot] {
		taken[subnet] = *entry
	}
	for s := range c.bySlot {
		if s <= slot {
			delete(c.bySlot, s)
		}
	}
	return taken
}

func (s *Service) countFFGVote(att eth.Att, subnet uint64) {
	slot := att.GetData().Slot
	if !decoupled.SummaryActive(slot) {
		return
	}
	seats := uint64(1)
	if bits := att.GetAggregationBits(); bits != nil {
		seats = bits.Count()
	}
	s.ffgVotes.count(slot, subnet, seats)
}

// runFFGVoteSummary writes one FFG votes line per slot, at the aggregation
// deadline. A vote that arrives after the tick is not counted.
func (s *Service) runFFGVoteSummary() {
	cfg := params.BeaconConfig()
	offset := cfg.SlotComponentDuration(cfg.AggregateDueBPSGloas)
	ticker := slots.NewSlotTickerWithOffset(s.cfg.clock.GenesisTime(), offset, cfg.SecondsPerSlot)
	defer ticker.Done()
	for {
		select {
		case slot := <-ticker.C():
			s.logFFGVoteSummary(slot)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting FFG vote summary routine")
			return
		}
	}
}

func (s *Service) logFFGVoteSummary(slot primitives.Slot) {
	if !decoupled.SummaryActive(slot) {
		return
	}
	subnets := s.subscribedAttestationSubnets()
	counted := s.ffgVotes.take(slot)
	votes, seats := uint64(0), uint64(0)
	perSubnet := make([]string, 0, len(subnets))
	for _, subnet := range subnets {
		entry := counted[subnet]
		votes += entry.votes
		seats += entry.seats
		perSubnet = append(perSubnet, fmt.Sprintf("%d:%d", subnet, entry.votes))
	}
	fields := decoupled.SummaryFields(slot)
	fields["subnets"] = len(subnets)
	fields["votes"] = votes
	fields["seats"] = seats
	fields["perSubnet"] = strings.Join(perSubnet, ",")
	log.WithFields(fields).Info("FFG votes")
}

// subscribedAttestationSubnets returns the attestation subnets the node holds a
// subscription for, sorted. A subscribed subnet with no vote is still listed.
func (s *Service) subscribedAttestationSubnets() []uint64 {
	prefix := "/" + p2p.GossipAttestationMessage + "_"
	var subnets []uint64
	for _, topic := range s.subHandler.allTopics() {
		start := strings.LastIndex(topic, prefix)
		if start < 0 {
			continue
		}
		rest := topic[start+len(prefix):]
		if end := strings.Index(rest, "/"); end >= 0 {
			rest = rest[:end]
		}
		subnet, err := strconv.ParseUint(rest, 10, 64)
		if err != nil {
			continue
		}
		subnets = append(subnets, subnet)
	}
	slices.Sort(subnets)
	return subnets
}

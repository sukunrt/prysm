package sync

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// summaryService returns a service subscribed to the given attestation subnets,
// on a config where Heze is active from genesis.
func summaryService(t *testing.T, subnets ...uint64) *Service {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	s := &Service{subHandler: newSubTopicHandler(), ffgVotes: newFFGVoteCounters()}
	digest := params.ForkDigest(0)
	for _, subnet := range subnets {
		topic := fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, subnet) + "/ssz_snappy"
		s.subHandler.addTopic(topic, nil)
	}
	return s
}

func TestFFGVoteCounters_KeyedBySlotAndSubnet(t *testing.T) {
	c := newFFGVoteCounters()
	c.count(4, 3, 2)
	c.count(4, 3, 1)
	c.count(4, 17, 5)
	c.count(3, 3, 9)

	taken := c.take(4)
	require.Equal(t, 2, len(taken))
	require.Equal(t, uint64(2), taken[3].votes)
	require.Equal(t, uint64(3), taken[3].seats)
	require.Equal(t, uint64(1), taken[17].votes)
	require.Equal(t, uint64(5), taken[17].seats)

	// The previous slot's counters are dropped with the slot that was read.
	require.Equal(t, 0, len(c.bySlot))
}

func TestLogFFGVoteSummary_CountsTheCurrentSlotOnly(t *testing.T) {
	hook := logTest.NewGlobal()
	s := summaryService(t, 3, 17)
	s.ffgVotes.count(4, 3, 1)
	s.ffgVotes.count(4, 3, 1)
	s.ffgVotes.count(4, 17, 2)
	s.ffgVotes.count(3, 17, 4)

	s.logFFGVoteSummary(4)
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, "FFG votes", entry.Message)
	require.Equal(t, decoupled.SummaryPurpose, entry.Data["purpose"])
	require.Equal(t, primitives.Slot(4), entry.Data["slot"])
	require.Equal(t, 2, entry.Data["subnets"])
	require.Equal(t, uint64(3), entry.Data["votes"])
	require.Equal(t, uint64(4), entry.Data["seats"])
	require.Equal(t, "3:2,17:1", entry.Data["perSubnet"])

	// Slot 3 was dropped with slot 4, so it cannot reappear.
	hook.Reset()
	s.logFFGVoteSummary(5)
	entry = hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, uint64(0), entry.Data["votes"])
	require.Equal(t, "3:0,17:0", entry.Data["perSubnet"])
}

func TestSubscribedAttestationSubnets_AttestationTopicsOnly(t *testing.T) {
	s := summaryService(t, 17, 3)
	digest := params.ForkDigest(0)
	s.subHandler.addTopic(
		fmt.Sprintf(p2p.AggregateAndProofSubnetTopicFormat, digest)+"/ssz_snappy", nil)
	require.DeepEqual(t, []uint64{3, 17}, s.subscribedAttestationSubnets())
}

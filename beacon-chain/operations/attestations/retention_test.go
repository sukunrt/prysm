package attestations

import (
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHezeRetentionAtSlotStart(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{}))
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)
	vote := func(slot primitives.Slot) ethpb.Att {
		bits := bitfield.NewBitlist(8)
		bits.SetBitAt(0, true)
		return util.HydrateAttestation(&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: slot}, AggregationBits: bits})
	}
	for slot := primitives.Slot(0); slot <= 4; slot++ {
		require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestation(vote(slot)))
	}
	s.genesisTime = time.Now().Add(-4 * time.Duration(cfg.SecondsPerSlot) * time.Second)
	require.Equal(t, true, s.pruneCurrentSlot())
	for _, a := range s.cfg.Pool.UnaggregatedAttestations() {
		require.Equal(t, true, a.GetData().Slot >= 1)
	}
	require.Equal(t, 4, s.cfg.Pool.UnaggregatedAttestationCount())
	require.Equal(t, 0, s.cfg.Pool.ForkchoiceAttestationCount())
}

func retentionAggregate(t *testing.T, slot primitives.Slot, bitsOn ...uint64) ethpb.Att {
	t.Helper()
	key, err := bls.RandKey()
	require.NoError(t, err)
	bits := bitfield.NewBitlist(8)
	for _, bit := range bitsOn {
		bits.SetBitAt(bit, true)
	}
	return util.HydrateAttestation(&ethpb.Attestation{
		Data: &ethpb.AttestationData{Slot: slot}, AggregationBits: bits,
		Signature: key.Sign([]byte{byte(slot), byte(len(bitsOn))}).Marshal(),
	})
}

func TestExperimentalRetentionAtHezeActivation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{EnableExperimentalAttestationPool: true}))
	cache := cache.NewAttestationCache()
	s, err := NewService(t.Context(), &Config{Cache: cache, Pool: NewPool()})
	require.NoError(t, err)
	a := retentionAggregate(t, 0, 0, 1)
	require.NoError(t, cache.Add(a))
	s.genesisTime = time.Now().Add(-time.Duration(cfg.SlotsPerEpoch-1) * cfg.SlotDuration())
	require.Equal(t, false, s.pruneCurrentSlot())
	require.Equal(t, 1, cache.Count())
	s.genesisTime = time.Now().Add(-time.Duration(cfg.SlotsPerEpoch) * cfg.SlotDuration())
	require.Equal(t, true, s.pruneCurrentSlot())
	require.Equal(t, 0, cache.Count())
	require.Equal(t, 0, len(cache.ForkchoiceAttestations()))
	require.NoError(t, cache.Add(a))
	require.Equal(t, 0, cache.Count())
}

func TestHezePruneMetricsAndStartup(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{}))
	pool := NewPool()
	s, err := NewService(t.Context(), &Config{Pool: pool, InitialSyncComplete: make(chan struct{})})
	require.NoError(t, err)
	require.NoError(t, pool.SaveAggregatedAttestation(retentionAggregate(t, 1, 0, 1)))
	single := retentionAggregate(t, 0, 0)
	require.NoError(t, pool.SaveUnaggregatedAttestation(single))
	block := retentionAggregate(t, 1, 2, 3)
	block.GetData().BeaconBlockRoot[0] = 1
	require.NoError(t, pool.SaveBlockAttestation(block))
	beforeAgg := retentionCounter(t, expiredAggregatedAtts)
	beforeSingle := retentionCounter(t, expiredUnaggregatedAtts)
	beforeBlock := retentionCounter(t, expiredBlockAtts)
	s.genesisTime = time.Now().Add(-5 * time.Duration(cfg.SecondsPerSlot) * time.Second)
	close(s.cfg.InitialSyncComplete)
	s.Start()
	t.Cleanup(func() { require.NoError(t, s.Stop()) })
	require.Equal(t, 0, pool.AggregatedAttestationCount())
	require.Equal(t, 0, pool.UnaggregatedAttestationCount())
	require.Equal(t, 0, len(pool.BlockAttestations()))
	require.Equal(t, beforeAgg+1, retentionCounter(t, expiredAggregatedAtts))
	require.Equal(t, beforeSingle+1, retentionCounter(t, expiredUnaggregatedAtts))
	require.Equal(t, beforeBlock+1, retentionCounter(t, expiredBlockAtts))
}

func retentionCounter(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	return metric.GetCounter().GetValue()
}

func TestHezeRetentionSlotTick(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.SecondsPerSlot = 1
	cfg.SlotDurationMilliseconds = 1000
	params.OverrideBeaconConfig(cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{}))
	pool := NewPool()
	s, err := NewService(t.Context(), &Config{Pool: pool})
	require.NoError(t, err)
	require.NoError(t, pool.SaveUnaggregatedAttestation(retentionAggregate(t, 0, 0)))
	s.genesisTime = time.Now().Truncate(time.Second).Add(-3 * time.Second)
	go s.pruneExpired()
	t.Cleanup(func() { require.NoError(t, s.Stop()) })
	deadline := time.Now().Add(3 * time.Second)
	for pool.UnaggregatedAttestationCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, 0, pool.UnaggregatedAttestationCount())
}

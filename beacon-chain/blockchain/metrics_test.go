package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, g.Write(m))
	return m.GetGauge().GetValue()
}

// TestReportSlotMetrics_RoundGaugesAt8Over32 pins the units the *_epoch and
// *_round gauges carry. Under the shipped configs the two are the same number,
// so only a config where a round is shorter than an epoch can tell them apart.
func TestReportSlotMetrics_RoundGaugesAt8Over32(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)
	require.Equal(t, params.BeaconConfig().SlotsPerEpoch, primitives.Slot(32))

	// Finalized round 6 starts at slot 48, which is in epoch 1.
	reportSlotMetrics(100, 100, 100, &eth.Checkpoint{Epoch: 6})
	require.Equal(t, float64(6), gaugeValue(t, headFinalizedRound))
	require.Equal(t, float64(1), gaugeValue(t, headFinalizedEpoch))
	require.Equal(t, float64(52), gaugeValue(t, finalityLatencySlots))
}

func TestReportRoundMetrics_BadAttestation(t *testing.T) {
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	h, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, h.AppendCurrentEpochAttestations(&eth.PendingAttestation{InclusionDelay: 0}))
	err = reportRoundMetrics(t.Context(), s, h)
	require.ErrorContains(t, "attestation with inclusion delay of 0", err)
}

func TestReportRoundMetrics_SlashedValidatorOutOfBound(t *testing.T) {
	h, _ := util.DeterministicGenesisState(t, 1)
	v, err := h.ValidatorAtIndex(0)
	require.NoError(t, err)
	v.Slashed = true
	require.NoError(t, h.UpdateValidatorAtIndex(0, v))
	require.NoError(t, h.AppendCurrentEpochAttestations(&eth.PendingAttestation{InclusionDelay: 1, Data: util.HydrateAttestationData(&eth.AttestationData{})}))
	err = reportRoundMetrics(t.Context(), h, h)
	require.ErrorContains(t, "slot 0 out of bounds", err)
}

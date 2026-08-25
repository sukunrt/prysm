package sync

import (
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/features"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	dto "github.com/prometheus/client_model/go"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func ledgerService(t *testing.T) *Service {
	t.Helper()
	return &Service{cfg: &config{clock: startup.NewClock(time.Now(), [32]byte{})}}
}

func counterVecValue(t *testing.T, reason string) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, availableAttDropCount.WithLabelValues(reason).Write(&m))
	return m.GetCounter().GetValue()
}

func ledgerVote() *ethpb.AvailableAttestation {
	bits := bitfield.NewBitvector512()
	bits.SetBitAt(3, true)
	return &ethpb.AvailableAttestation{
		AggregationBits: bits,
		Data: &ethpb.AvailableAttestationData{
			Slot:            0,
			BeaconBlockRoot: make([]byte, 32),
		},
	}
}

func TestLogVote_QuietUnlessTheLedgerIsOn(t *testing.T) {
	hook := logTest.NewGlobal()
	s := ledgerService(t)

	s.logVote(ledgerVote(), voteAccepted, "", time.Now())
	require.Equal(t, 0, len(hook.AllEntries()))

	reset := features.InitWithReset(&features.Flags{GoldfishVoteLedger: true})
	defer reset()
	s.logVote(ledgerVote(), voteAccepted, "", time.Now())
	require.Equal(t, 1, len(hook.AllEntries()))
	require.Equal(t, voteAccepted, hook.LastEntry().Data["outcome"])
}

// Every discard has to move the counter, whether or not the ledger is on:
// the counter is what a run without the ledger has to reconcile against.
func TestDropVote_AlwaysCountsTheDrop(t *testing.T) {
	s := ledgerService(t)
	before := counterVecValue(t, "target_state")
	s.dropVote(ledgerVote(), "target_state", time.Now())
	require.Equal(t, before+1, counterVecValue(t, "target_state"))
}

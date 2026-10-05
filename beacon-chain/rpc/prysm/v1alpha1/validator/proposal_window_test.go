package validator

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProposalWindowHeze(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	for _, block := range []primitives.Slot{1, 2, 3, 4, 8, 9} {
		var atts []ethpb.Att
		for slot := primitives.Slot(0); slot <= block+1; slot++ {
			atts = append(atts, &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: slot}})
		}
		got := proposalWindow(atts, block)
		lower := primitives.Slot(0)
		if block > 3 {
			lower = block - 3
		}
		require.Equal(t, int(block-lower), len(got))
		for i, a := range got {
			require.Equal(t, lower+primitives.Slot(i), a.GetData().Slot)
		}
	}
	cfg.HezeForkEpoch = cfg.FarFutureEpoch
	params.OverrideBeaconConfig(cfg)
	atts := []ethpb.Att{&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 0}}, &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 4}}}
	require.Equal(t, 2, len(proposalWindow(atts, 4)))
	cfg.HezeForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	activation := cfg.SlotsPerEpoch
	atts = []ethpb.Att{
		&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 0}},
		&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: activation - 3}},
		&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: activation - 1}},
		&ethpb.Attestation{Data: &ethpb.AttestationData{Slot: activation}},
	}
	require.Equal(t, 4, len(proposalWindow(atts, activation-1)))
	require.Equal(t, 2, len(proposalWindow(atts, activation)))
}

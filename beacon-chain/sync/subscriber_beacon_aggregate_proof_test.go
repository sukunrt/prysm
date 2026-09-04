package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBeaconAggregateProofSubscriber_CanSaveAggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &ethpb.SignedAggregateAttestationAndProof{
		Message: &ethpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x07},
			}),
			AggregatorIndex: 100,
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(t.Context(), a))
	assert.DeepSSZEqual(t, []ethpb.Att{a.Message.Aggregate}, r.cfg.attPool.AggregatedAttestations(), "Did not save aggregated attestation")
}

func TestBeaconAggregateProofSubscriber_CanSaveUnaggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	a := &ethpb.SignedAggregateAttestationAndProof{
		Message: &ethpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0x03},
				Signature:       make([]byte, fieldparams.BLSSignatureLength),
			}),
			AggregatorIndex: 100,
		},
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(t.Context(), a))

	atts := r.cfg.attPool.UnaggregatedAttestations()
	assert.DeepEqual(t, []ethpb.Att{a.Message.Aggregate}, atts, "Did not save unaggregated attestation")
}

func TestBeaconAggregateProofSubscriber_SavesGloasAggregateAsElectra(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
	}

	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(0, true)
	aggregate := util.HydrateAttestationGloas(&ethpb.AttestationGloas{
		AggregationBits: bitfield.Bitlist{0x07},
		CommitteeBits:   cb,
	})
	a := &ethpb.SignedAggregateAttestationAndProofGloas{
		Message: &ethpb.AggregateAttestationAndProofGloas{
			Aggregate:       aggregate,
			AggregatorIndex: 100,
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}
	require.NoError(t, r.beaconAggregateProofSubscriber(t.Context(), a))

	atts := r.cfg.attPool.AggregatedAttestations()
	require.Equal(t, 1, len(atts))
	assert.Equal(t, version.Electra, atts[0].Version(), "Aggregate was not stored as Electra")
	assert.DeepEqual(t, aggregate.AggregationBits, atts[0].GetAggregationBits())
	assert.DeepEqual(t, aggregate.Data, atts[0].GetData())
}

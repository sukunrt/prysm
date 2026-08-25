package decoupled

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestVoteLedgerValidators(t *testing.T) {
	require.Equal(t, "", VoteLedgerValidators(nil))
	require.Equal(t, "7", VoteLedgerValidators([]uint64{7}))
	require.Equal(t, "3,1,2", VoteLedgerValidators([]uint64{3, 1, 2}))
}

func TestVoteLedgerDataRoot(t *testing.T) {
	att := func(seat uint64, headRoot byte) *ethpb.AttestationElectra {
		bits := bitfield.NewBitlist(250)
		bits.SetBitAt(seat, true)
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(0, true)
		root := make([]byte, 32)
		root[0] = headRoot
		return &ethpb.AttestationElectra{
			Data: &ethpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: root,
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
			AggregationBits: bits,
			CommitteeBits:   committeeBits,
			Signature:       make([]byte, 96),
		}
	}

	// Seats that name the same head aggregate together, so they share the key.
	require.Equal(t, VoteLedgerDataRoot(att(0, 0xaa)), VoteLedgerDataRoot(att(9, 0xaa)))
	// A different head is a different attestation data, and never aggregates in.
	require.NotEqual(t, VoteLedgerDataRoot(att(0, 0xaa)), VoteLedgerDataRoot(att(0, 0xbb)))
	require.Equal(t, "", VoteLedgerDataRoot(&ethpb.AttestationElectra{}))
}

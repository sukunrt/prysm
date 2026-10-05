package cache

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAttestationCacheRetention(t *testing.T) {
	c := NewAttestationCache()
	vote := func(slot primitives.Slot) ethpb.Att {
		bits := bitfield.NewBitlist(8)
		bits.SetBitAt(0, true)
		return &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: slot, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}}, AggregationBits: bits, Signature: make([]byte, 96)}
	}
	require.NoError(t, c.SaveForkchoiceAttestations([]ethpb.Att{vote(4)}))
	for slot := primitives.Slot(0); slot <= 4; slot++ {
		require.NoError(t, c.Add(vote(slot)))
	}
	require.Equal(t, uint64(1), c.PruneRetainedBefore(1))
	require.Equal(t, 4, c.Count())
	require.Equal(t, 1, len(c.ForkchoiceAttestations()))
	require.Equal(t, uint64(0), c.PruneRetainedBefore(0))
	require.NoError(t, c.Add(vote(0)))
	require.Equal(t, 4, c.Count())
	require.NoError(t, c.Add(vote(0)))
	require.Equal(t, 1, len(c.ForkchoiceAttestations()))
	for _, a := range c.ForkchoiceAttestations() {
		require.NoError(t, c.DeleteForkchoiceAttestation(a))
	}
	require.Equal(t, 0, len(c.ForkchoiceAttestations()))
}

func TestAttestationCacheLegacyExpiryDoesNotAdvanceHezeCutoff(t *testing.T) {
	c := NewAttestationCache()
	bits := bitfield.NewBitlist(8)
	bits.SetBitAt(0, true)
	a := &ethpb.Attestation{Data: &ethpb.AttestationData{Slot: 0, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}}, AggregationBits: bits, Signature: make([]byte, 96)}
	require.NoError(t, c.Add(a))
	require.Equal(t, uint64(1), c.PruneBefore(1))
	require.Equal(t, 0, len(c.ForkchoiceAttestations()))
	require.NoError(t, c.Add(a))
	require.Equal(t, 1, c.Count())
	require.Equal(t, uint64(1), c.PruneRetainedBefore(2))
	require.Equal(t, 0, len(c.ForkchoiceAttestations()))
	require.NoError(t, c.Add(a))
	require.Equal(t, 0, c.Count())
}

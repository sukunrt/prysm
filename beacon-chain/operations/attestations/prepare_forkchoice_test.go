package attestations

import (
	"fmt"
	"sort"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestBatchAttestations_Multiple(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	priv, err := bls.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte("dummy_test_data"))
	var mockRoot [32]byte

	unaggregatedAtts := []ethpb.Att{
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            2,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100100}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b101000}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            0,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100010}, Signature: sig.Marshal()},
	}
	aggregatedAtts := []ethpb.Att{
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            2,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b111000}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100011}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: &ethpb.AttestationData{
			Slot:            0,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b110001}, Signature: sig.Marshal()},
	}
	blockAtts := []*ethpb.Attestation{
		{Data: &ethpb.AttestationData{
			Slot:            2,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100001}, Signature: sig.Marshal()},
		{Data: &ethpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100100}, Signature: sig.Marshal()},
		{Data: &ethpb.AttestationData{
			Slot:            0,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100100}, Signature: sig.Marshal()},
		{Data: &ethpb.AttestationData{
			Slot:            2,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b111000}, Signature: sig.Marshal()}, // Duplicated
		{Data: &ethpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: mockRoot[:],
			Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
			Target:          &ethpb.Checkpoint{Root: mockRoot[:]}}, AggregationBits: bitfield.Bitlist{0b100011}, Signature: sig.Marshal()}, // Duplicated
	}
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestations(unaggregatedAtts))
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(aggregatedAtts))
	for _, att := range blockAtts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}
	require.NoError(t, s.batchForkChoiceAtts(t.Context()))

	received := s.cfg.Pool.ForkchoiceAttestations()
	assert.DeepEqual(t, attestationBitsBySlot(append(append(unaggregatedAtts, aggregatedAtts...), toAtts(blockAtts)...)), attestationBitsBySlot(received))
}

func TestBatchAttestations_Single(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	priv, err := bls.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte("dummy_test_data"))
	var mockRoot [32]byte
	d := &ethpb.AttestationData{
		BeaconBlockRoot: mockRoot[:],
		Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
		Target:          &ethpb.Checkpoint{Root: mockRoot[:]},
	}

	unaggregatedAtts := []ethpb.Att{
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b101000}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b100100}, Signature: sig.Marshal()},
	}
	aggregatedAtts := []ethpb.Att{
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b101100}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b110010}, Signature: sig.Marshal()},
	}
	blockAtts := []ethpb.Att{
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b110010}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b100010}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b110010}, Signature: sig.Marshal()}, // Duplicated
	}
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestations(unaggregatedAtts))
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(aggregatedAtts))

	for _, att := range blockAtts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}
	require.NoError(t, s.batchForkChoiceAtts(t.Context()))

	got := s.cfg.Pool.ForkchoiceAttestations()
	assert.DeepEqual(t, attestationBitsBySlot(append(append(unaggregatedAtts, aggregatedAtts...), blockAtts...)), attestationBitsBySlot(got))
}

func TestBatchAttestations_LoneSingleReachesForkChoice(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)
	priv, err := bls.RandKey()
	require.NoError(t, err)
	att := util.HydrateAttestation(&ethpb.Attestation{
		Data:            &ethpb.AttestationData{Slot: 1},
		AggregationBits: bitfield.Bitlist{0b1001},
		Signature:       priv.Sign([]byte("single")).Marshal(),
	})
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestation(att))
	require.NoError(t, s.batchForkChoiceAtts(t.Context()))
	got := s.cfg.Pool.ForkchoiceAttestations()
	require.Equal(t, 1, len(got))
	assert.DeepSSZEqual(t, att, got[0])
}

func toAtts(atts []*ethpb.Attestation) []ethpb.Att {
	result := make([]ethpb.Att, len(atts))
	for i, att := range atts {
		result[i] = att
	}
	return result
}

func attestationBitsBySlot(atts []ethpb.Att) map[uint64]map[int]bool {
	result := make(map[uint64]map[int]bool)
	for _, att := range atts {
		slot := uint64(att.GetData().Slot)
		if result[slot] == nil {
			result[slot] = make(map[int]bool)
		}
		for _, bit := range att.GetAggregationBits().BitIndices() {
			result[slot][bit] = true
		}
	}
	return result
}

func TestAggregateAndSaveForkChoiceAtts_Single(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	priv, err := bls.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte("dummy_test_data"))
	var mockRoot [32]byte
	d := &ethpb.AttestationData{
		BeaconBlockRoot: mockRoot[:],
		Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
		Target:          &ethpb.Checkpoint{Root: mockRoot[:]},
	}

	atts := []ethpb.Att{
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b101}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b110}, Signature: sig.Marshal()}}
	require.NoError(t, s.aggregateAndSaveForkChoiceAtts(atts))

	wanted, err := attaggregation.Aggregate(atts)
	require.NoError(t, err)
	assert.DeepEqual(t, wanted, s.cfg.Pool.ForkchoiceAttestations())
}

func TestAggregateAndSaveForkChoiceAtts_Multiple(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	priv, err := bls.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte("dummy_test_data"))
	var mockRoot [32]byte
	d := &ethpb.AttestationData{
		BeaconBlockRoot: mockRoot[:],
		Source:          &ethpb.Checkpoint{Root: mockRoot[:]},
		Target:          &ethpb.Checkpoint{Root: mockRoot[:]},
	}
	d1, ok := proto.Clone(d).(*ethpb.AttestationData)
	require.Equal(t, true, ok, "Entity is not of type *ethpb.AttestationData")
	d1.Slot = 1
	d2, ok := proto.Clone(d).(*ethpb.AttestationData)
	require.Equal(t, true, ok, "Entity is not of type *ethpb.AttestationData")
	d2.Slot = 2

	atts1 := []ethpb.Att{
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b101}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b110}, Signature: sig.Marshal()},
	}
	require.NoError(t, s.aggregateAndSaveForkChoiceAtts(atts1))
	atts2 := []ethpb.Att{
		&ethpb.Attestation{Data: d1, AggregationBits: bitfield.Bitlist{0b10110}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d1, AggregationBits: bitfield.Bitlist{0b11100}, Signature: sig.Marshal()},
		&ethpb.Attestation{Data: d1, AggregationBits: bitfield.Bitlist{0b11000}, Signature: sig.Marshal()},
	}
	require.NoError(t, s.aggregateAndSaveForkChoiceAtts(atts2))
	att3 := []ethpb.Att{
		&ethpb.Attestation{Data: d2, AggregationBits: bitfield.Bitlist{0b1100}, Signature: sig.Marshal()},
	}
	require.NoError(t, s.aggregateAndSaveForkChoiceAtts(att3))

	wanted, err := attaggregation.Aggregate(atts1)
	require.NoError(t, err)
	aggregated, err := attaggregation.Aggregate(atts2)
	require.NoError(t, err)

	wanted = append(wanted, aggregated...)
	wanted = append(wanted, att3...)

	received := s.cfg.Pool.ForkchoiceAttestations()
	sort.Slice(received, func(i, j int) bool {
		return received[i].GetData().Slot < received[j].GetData().Slot
	})
	for i, a := range wanted {
		assert.Equal(t, true, proto.Equal(a, received[i]))
	}
}

func TestSeenAttestations_PresentInCache(t *testing.T) {
	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	ad1 := util.HydrateAttestationData(&ethpb.AttestationData{})
	att1 := &ethpb.Attestation{Data: ad1, Signature: []byte{'A'}, AggregationBits: bitfield.Bitlist{0x13} /* 0b00010011 */}
	got, err := s.seen(att1)
	require.NoError(t, err)
	assert.Equal(t, false, got)

	att2 := &ethpb.Attestation{Data: ad1, Signature: []byte{'A'}, AggregationBits: bitfield.Bitlist{0x17} /* 0b00010111 */}
	got, err = s.seen(att2)
	require.NoError(t, err)
	assert.Equal(t, false, got)

	att3 := &ethpb.Attestation{Data: ad1, Signature: []byte{'A'}, AggregationBits: bitfield.Bitlist{0x17} /* 0b00010111 */}
	got, err = s.seen(att3)
	require.NoError(t, err)
	assert.Equal(t, true, got)
}

func TestService_seen(t *testing.T) {
	ad1 := util.HydrateAttestationData(&ethpb.AttestationData{Slot: 1})

	ad2 := util.HydrateAttestationData(&ethpb.AttestationData{Slot: 2})

	// Attestation are checked in order of this list.
	tests := []struct {
		att  *ethpb.Attestation
		want bool
	}{
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b11011},
				Data:            ad1,
			},
			want: false,
		},
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b11011},
				Data:            ad1,
			},
			want: true, // Exact same attestation should return true
		},
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b10101},
				Data:            ad1,
			},
			want: false, // Haven't seen the bit at index 2 yet.
		},
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b11111},
				Data:            ad1,
			},
			want: true, // We've full committee at this point.
		},
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b11111},
				Data:            ad2,
			},
			want: false, // Different root is different bitlist.
		},
		{
			att: &ethpb.Attestation{
				AggregationBits: bitfield.Bitlist{0b11111001},
				Data:            ad1,
			},
			want: false, // Sanity test that an attestation of different lengths does not panic.
		},
	}

	s, err := NewService(t.Context(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := s.seen(tt.att)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

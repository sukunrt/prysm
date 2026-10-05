package attestations_test

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMaxCoverAttestationAggregation_BoundedWork(t *testing.T) {
	const bitCount = 16
	message := [32]byte{1, 2, 3}
	keys := make([]common.SecretKey, bitCount)
	for i := range keys {
		var err error
		keys[i], err = bls.RandKey()
		require.NoError(t, err)
	}

	makeAtt := func(indices ...uint64) ethpb.Att {
		bits := bitfield.NewBitlist(bitCount)
		signatures := make([]bls.Signature, 0, len(indices))
		for _, idx := range indices {
			bits.SetBitAt(idx, true)
			signatures = append(signatures, keys[idx].Sign(message[:]))
		}
		return &ethpb.Attestation{
			AggregationBits: bits,
			Data:            &ethpb.AttestationData{},
			Signature:       bls.AggregateSignatures(signatures).Marshal(),
		}
	}

	tests := []struct {
		name         string
		inputs       []ethpb.Att
		wantCount    int
		maxBits      uint64
		mergedCount  int
		unionCovered bool
	}{
		{
			name: "at most three inputs per aggregate",
			inputs: []ethpb.Att{
				makeAtt(0), makeAtt(1), makeAtt(2), makeAtt(3), makeAtt(4),
			},
			wantCount:   2,
			maxBits:     3,
			mergedCount: 1,
		},
		{
			name: "at most three rounds with unprocessed inputs returned",
			inputs: []ethpb.Att{
				makeAtt(0, 2), makeAtt(1, 3),
				makeAtt(0, 4), makeAtt(1, 5),
				makeAtt(0, 6), makeAtt(1, 7),
				makeAtt(0, 8), makeAtt(1, 9),
			},
			wantCount:   5,
			maxBits:     4,
			mergedCount: 3,
		},
		{
			name: "aggregate covered by union of earlier aggregates is retained",
			inputs: []ethpb.Att{
				makeAtt(0, 2), makeAtt(1, 3),
				makeAtt(0, 3), makeAtt(1, 4),
				makeAtt(0, 2), makeAtt(1, 4),
			},
			wantCount:    3,
			maxBits:      4,
			mergedCount:  3,
			unionCovered: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantUnion := bitfield.NewBitlist(bitCount)
			for _, att := range tt.inputs {
				var err error
				wantUnion, err = wantUnion.Or(att.GetAggregationBits())
				require.NoError(t, err)
			}
			got, err := attestations.MaxCoverAttestationAggregation(tt.inputs)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, len(got))
			gotUnion := bitfield.NewBitlist(bitCount)
			mergedCount := 0
			for _, att := range got {
				bits := att.GetAggregationBits()
				assert.Equal(t, true, bits.Count() <= tt.maxBits)
				if bits.Count() > 2 {
					mergedCount++
				}
				gotUnion, err = gotUnion.Or(bits)
				require.NoError(t, err)
				pubkeys := make([]common.PublicKey, 0, bits.Count())
				for _, idx := range bits.BitIndices() {
					pubkeys = append(pubkeys, keys[idx].PublicKey())
				}
				sig, err := bls.SignatureFromBytes(att.GetSignature())
				require.NoError(t, err)
				assert.Equal(t, true, sig.FastAggregateVerify(pubkeys, message))
			}
			assert.Equal(t, tt.mergedCount, mergedCount)
			assert.DeepEqual(t, wantUnion.Bytes(), gotUnion.Bytes())
			if tt.unionCovered {
				firstTwo, err := got[0].GetAggregationBits().Or(got[1].GetAggregationBits())
				require.NoError(t, err)
				contains, err := firstTwo.Contains(got[2].GetAggregationBits())
				require.NoError(t, err)
				assert.Equal(t, true, contains)
				for _, att := range got[:2] {
					contains, err := att.GetAggregationBits().Contains(got[2].GetAggregationBits())
					require.NoError(t, err)
					assert.Equal(t, false, contains)
				}
			}
		})
	}
}

func TestAggregateAttestations_MaxCover_NewMaxCover(t *testing.T) {
	type args struct {
		atts []*ethpb.Attestation
	}
	tests := []struct {
		name string
		args args
		want *aggregation.MaxCoverProblem
	}{
		{
			name: "nil attestations",
			args: args{
				atts: nil,
			},
			want: &aggregation.MaxCoverProblem{Candidates: []*aggregation.MaxCoverCandidate{}},
		},
		{
			name: "no attestations",
			args: args{
				atts: []*ethpb.Attestation{},
			},
			want: &aggregation.MaxCoverProblem{Candidates: []*aggregation.MaxCoverCandidate{}},
		},
		{
			name: "single attestation",
			args: args{
				atts: []*ethpb.Attestation{
					{AggregationBits: bitfield.Bitlist{0b00001010, 0b1}},
				},
			},
			want: &aggregation.MaxCoverProblem{
				Candidates: aggregation.MaxCoverCandidates{
					aggregation.NewMaxCoverCandidate(0, &bitfield.Bitlist{0b00001010, 0b1}),
				},
			},
		},
		{
			name: "multiple attestations",
			args: args{
				atts: []*ethpb.Attestation{
					{AggregationBits: bitfield.Bitlist{0b00001010, 0b1}},
					{AggregationBits: bitfield.Bitlist{0b00101010, 0b1}},
					{AggregationBits: bitfield.Bitlist{0b11111010, 0b1}},
					{AggregationBits: bitfield.Bitlist{0b00000010, 0b1}},
					{AggregationBits: bitfield.Bitlist{0b00000001, 0b1}},
				},
			},
			want: &aggregation.MaxCoverProblem{
				Candidates: aggregation.MaxCoverCandidates{
					aggregation.NewMaxCoverCandidate(0, &bitfield.Bitlist{0b00001010, 0b1}),
					aggregation.NewMaxCoverCandidate(1, &bitfield.Bitlist{0b00101010, 0b1}),
					aggregation.NewMaxCoverCandidate(2, &bitfield.Bitlist{0b11111010, 0b1}),
					aggregation.NewMaxCoverCandidate(3, &bitfield.Bitlist{0b00000010, 0b1}),
					aggregation.NewMaxCoverCandidate(4, &bitfield.Bitlist{0b00000001, 0b1}),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DeepEqual(t, tt.want, attestations.NewMaxCover(tt.args.atts))
		})
	}
}

func TestAggregateAttestations_MaxCover_AttList_validate(t *testing.T) {
	tests := []struct {
		name      string
		atts      attestations.AttList
		wantedErr string
	}{
		{
			name:      "nil list",
			atts:      nil,
			wantedErr: "nil list",
		},
		{
			name:      "empty list",
			atts:      attestations.AttList{},
			wantedErr: "empty list",
		},
		{
			name:      "first bitlist is nil",
			atts:      attestations.AttList{&ethpb.Attestation{}},
			wantedErr: "bitlist cannot be nil or empty",
		},
		{
			name: "non first bitlist is nil",
			atts: attestations.AttList{
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
				&ethpb.Attestation{},
			},
			wantedErr: "bitlist cannot be nil or empty",
		},
		{
			name: "first bitlist is empty",
			atts: attestations.AttList{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{}},
			},
			wantedErr: "bitlist cannot be nil or empty",
		},
		{
			name: "non first bitlist is empty",
			atts: attestations.AttList{
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{}},
			},
			wantedErr: "bitlist cannot be nil or empty",
		},
		{
			name: "valid bitlists",
			atts: attestations.AttList{
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
				&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(64)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.atts.ValidateForTesting()
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAggregateAttestations_rearrangeProcessedAttestations(t *testing.T) {
	tests := []struct {
		name     string
		atts     []ethpb.Att
		keys     []int
		wantAtts []ethpb.Att
	}{
		{
			name: "nil attestations",
		},
		{
			name: "single attestation no processed keys",
			atts: []ethpb.Att{
				&ethpb.Attestation{},
			},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{},
			},
		},
		{
			name: "single attestation processed",
			atts: []ethpb.Att{
				&ethpb.Attestation{},
			},
			keys: []int{0},
			wantAtts: []ethpb.Att{
				nil,
			},
		},
		{
			name: "multiple processed, last attestation marked",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
			},
			keys: []int{1, 4}, // Only attestation at index 1, should be moved, att at 4 is already at the end.
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				nil, nil,
			},
		},
		{
			name: "all processed",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
			},
			keys: []int{0, 1, 2, 3, 4},
			wantAtts: []ethpb.Att{
				nil, nil, nil, nil, nil,
			},
		},
		{
			name: "operate on slice, single attestation marked",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				// Assuming some attestations have been already marked as nil, during previous rounds:
				nil, nil, nil,
			},
			keys: []int{2},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				nil, nil, nil, nil,
			},
		},
		{
			name: "operate on slice, non-last attestation marked",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x05}},
				// Assuming some attestations have been already marked as nil, during previous rounds:
				nil, nil, nil,
			},
			keys: []int{2, 3},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x05}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				nil, nil, nil, nil, nil,
			},
		},
		{
			name: "operate on slice, last attestation marked",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				// Assuming some attestations have been already marked as nil, during previous rounds:
				nil, nil, nil,
			},
			keys: []int{2, 4},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				nil, nil, nil, nil, nil,
			},
		},
		{
			name: "many items, many selected, keys unsorted",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x02}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x04}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x05}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x06}},
			},
			keys: []int{4, 1, 2, 5, 6},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x03}},
				nil, nil, nil, nil, nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := make([]*bitfield.Bitlist64, len(tt.atts))
			for i := 0; i < len(tt.atts); i++ {
				if tt.atts[i] != nil {
					var err error
					candidates[i], err = tt.atts[i].GetAggregationBits().ToBitlist64()
					if err != nil {
						t.Error(err)
					}
				}
			}
			attestations.RearrangeProcessedAttestations(tt.atts, candidates, tt.keys)
			assert.DeepEqual(t, tt.atts, tt.wantAtts)
		})
	}
}

func TestAggregateAttestations_aggregateAttestations(t *testing.T) {
	sign := bls.NewAggregateSignature().Marshal()
	tests := []struct {
		name          string
		atts          []ethpb.Att
		wantAtts      []ethpb.Att
		keys          []int
		coverage      *bitfield.Bitlist64
		wantTargetIdx int
		wantErr       string
	}{
		{
			name:          "nil attestation",
			wantTargetIdx: 0,
			wantErr:       attestations.ErrInvalidAttestationCount.Error(),
			keys:          []int{0, 1, 2},
		},
		{
			name: "single attestation",
			atts: []ethpb.Att{
				&ethpb.Attestation{},
			},
			wantTargetIdx: 0,
			wantErr:       attestations.ErrInvalidAttestationCount.Error(),
			keys:          []int{0, 1, 2},
		},
		{
			name:          "no keys",
			wantTargetIdx: 0,
			wantErr:       attestations.ErrInvalidAttestationCount.Error(),
		},
		{
			name: "two attestations, none selected",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
			},
			wantTargetIdx: 0,
			wantErr:       attestations.ErrInvalidAttestationCount.Error(),
			keys:          []int{},
		},
		{
			name: "two attestations, one selected",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x00}},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x01}},
			},
			wantTargetIdx: 0,
			wantErr:       attestations.ErrInvalidAttestationCount.Error(),
			keys:          []int{0},
		},
		{
			name: "two attestations, both selected, empty coverage",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00000001, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00000110, 0b1}, Signature: sign},
			},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00000111, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0b00000110, 0b1}, Signature: sign},
			},
			wantTargetIdx: 0,
			wantErr:       "invalid or empty coverage",
			keys:          []int{0, 1},
		},
		{
			name: "two attestations, both selected",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000001, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000010, 0b1}, Signature: sign},
			},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000011, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000010, 0b1}, Signature: sign},
			},
			wantTargetIdx: 0,
			keys:          []int{0, 1},
			coverage: func() *bitfield.Bitlist64 {
				b, err := bitfield.NewBitlist64FromBytes(64, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000011})
				if err != nil {
					t.Fatal(err)
				}
				return b
			}(),
		},
		{
			name: "many attestations, several selected",
			atts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000001, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000010, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000100, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00001000, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00010000, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00100000, 0b1}, Signature: sign},
			},
			wantAtts: []ethpb.Att{
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000001, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00010110, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00000100, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00001000, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00010000, 0b1}, Signature: sign},
				&ethpb.Attestation{AggregationBits: bitfield.Bitlist{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00100000, 0b1}, Signature: sign},
			},
			wantTargetIdx: 1,
			keys:          []int{1, 2, 4},
			coverage: func() *bitfield.Bitlist64 {
				b, err := bitfield.NewBitlist64FromBytes(64, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0b00010110})
				if err != nil {
					t.Fatal(err)
				}
				return b
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTargetIdx, err := attestations.AggregateAttestations(tt.atts, tt.keys, tt.coverage)
			if tt.wantErr != "" {
				assert.ErrorContains(t, tt.wantErr, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantTargetIdx, gotTargetIdx)
			extractBitlists := func(atts []ethpb.Att) []bitfield.Bitlist {
				bl := make([]bitfield.Bitlist, len(atts))
				for i, att := range atts {
					bl[i] = att.GetAggregationBits()
				}
				return bl
			}
			assert.DeepEqual(t, extractBitlists(tt.atts), extractBitlists(tt.wantAtts))
		})
	}
}

func TestAggregateAttestations_PreservesGloasType(t *testing.T) {
	signature := bls.NewAggregateSignature().Marshal()
	committeeBits := primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(0, true)
	atts := []ethpb.Att{
		&ethpb.AttestationGloas{
			AggregationBits: bitfield.Bitlist{0b00000001, 0b1},
			CommitteeBits:   committeeBits,
			Data:            &ethpb.AttestationData{},
			Signature:       signature,
		},
		&ethpb.AttestationGloas{
			AggregationBits: bitfield.Bitlist{0b00000010, 0b1},
			CommitteeBits:   committeeBits,
			Data:            &ethpb.AttestationData{},
			Signature:       signature,
		},
	}
	coverage, err := bitfield.NewBitlist64FromBytes(8, []byte{0b00000011})
	assert.NoError(t, err)

	targetIdx, err := attestations.AggregateAttestations(atts, []int{0, 1}, coverage)
	assert.NoError(t, err)
	assert.Equal(t, 0, targetIdx)
	_, ok := atts[targetIdx].(*ethpb.AttestationGloas)
	assert.Equal(t, true, ok)
}

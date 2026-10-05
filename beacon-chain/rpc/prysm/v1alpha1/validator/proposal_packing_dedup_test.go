//go:build minimal

package validator

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// referencePackingDedup preserves the original pairwise containment procedure.
func referencePackingDedup(input proposerAtts) (proposerAtts, error) {
	if len(input) < 2 {
		return input, nil
	}
	groups := make(map[attestation.Id][]ethpb.Att)
	for _, att := range input {
		id, err := attestation.NewId(att, attestation.Data)
		if err != nil {
			return nil, err
		}
		groups[id] = append(groups[id], att)
	}
	var result proposerAtts
	for _, atts := range groups {
		for i := 0; i < len(atts); i++ {
			for j := i + 1; j < len(atts); j++ {
				contains, err := atts[i].GetAggregationBits().Contains(atts[j].GetAggregationBits())
				if err != nil {
					return nil, err
				}
				if contains {
					atts[j] = atts[len(atts)-1]
					atts = atts[:len(atts)-1]
					j--
					continue
				}
				contains, err = atts[j].GetAggregationBits().Contains(atts[i].GetAggregationBits())
				if err != nil {
					return nil, err
				}
				if contains {
					atts[i] = atts[len(atts)-1]
					atts = atts[:len(atts)-1]
					i--
					break
				}
			}
		}
		result = append(result, atts...)
	}
	return result, nil
}

func packingDedupKeys(t *testing.T, atts proposerAtts) []string {
	t.Helper()
	keys := make([]string, 0, len(atts))
	for _, att := range atts {
		id, err := attestation.NewId(att, attestation.Data)
		require.NoError(t, err)
		keys = append(keys, fmt.Sprintf("%x/%x", id, att.GetAggregationBits()))
	}
	slices.Sort(keys)
	return keys
}

func packingDedupAtt(electra bool, root byte, committee uint64, positions ...uint64) ethpb.Att {
	bits := bitfield.NewBitlist(32)
	for _, position := range positions {
		bits.SetBitAt(position, true)
	}
	data := &ethpb.AttestationData{
		Slot: 7, BeaconBlockRoot: make([]byte, 32),
		Source: &ethpb.Checkpoint{Root: make([]byte, 32)},
		Target: &ethpb.Checkpoint{Root: make([]byte, 32)},
	}
	data.BeaconBlockRoot[0] = root
	if electra {
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(committee, true)
		return &ethpb.AttestationElectra{Data: data, AggregationBits: bits, CommitteeBits: cb, Signature: []byte{root, byte(committee)}}
	}
	data.CommitteeIndex = primitives.CommitteeIndex(committee)
	return &ethpb.Attestation{Data: data, AggregationBits: bits, Signature: []byte{root, byte(committee)}}
}

func TestProposalPackingDedupDifferential(t *testing.T) {
	for _, electra := range []bool{false, true} {
		for seed := int64(0); seed < 200; seed++ {
			rng := rand.New(rand.NewSource(seed))
			atts := make(proposerAtts, 0, 32)
			for i := range 24 {
				positions := make([]uint64, 0, 12)
				for bit := uint64(0); bit < 32; bit++ {
					if rng.Intn(6) == 0 {
						positions = append(positions, bit)
					}
				}
				if i%7 == 0 && len(atts) > 0 {
					atts = append(atts, atts[len(atts)-1].Clone())
					continue
				}
				atts = append(atts, packingDedupAtt(electra, byte(i%2), uint64(i%3), positions...))
			}
			rng.Shuffle(len(atts), func(i, j int) { atts[i], atts[j] = atts[j], atts[i] })
			before := make(proposerAtts, len(atts))
			for i := range atts {
				before[i] = atts[i].Clone()
			}
			want, err := referencePackingDedup(atts)
			require.NoError(t, err)
			got, err := atts.dedup()
			require.NoError(t, err)
			require.DeepEqual(t, packingDedupKeys(t, want), packingDedupKeys(t, got))
			for i := range atts {
				require.DeepEqual(t, before[i], atts[i])
			}
		}
	}
}

func TestProposalPackingDedupLargeDifferential(t *testing.T) {
	for _, electra := range []bool{false, true} {
		for seed := int64(0); seed < 100; seed++ {
			for _, size := range []int{32, 33, 80} {
				rng := rand.New(rand.NewSource(seed + 1000))
				atts := make(proposerAtts, 0, size)
				for i := range size {
					if i%11 == 0 && len(atts) > 0 {
						atts = append(atts, atts[len(atts)-1].Clone())
						continue
					}
					positions := make([]uint64, 0, 20)
					for bit := uint64(0); bit < 32; bit++ {
						if rng.Intn(3+i%4) == 0 {
							positions = append(positions, bit)
						}
					}
					atts = append(atts, packingDedupAtt(electra, 1, 0, positions...))
				}
				rng.Shuffle(len(atts), func(i, j int) { atts[i], atts[j] = atts[j], atts[i] })
				want, err := referencePackingDedup(atts)
				require.NoError(t, err)
				got, err := atts.dedup()
				require.NoError(t, err)
				require.DeepEqual(t, packingDedupKeys(t, want), packingDedupKeys(t, got))
			}
		}
	}
}

func TestProposalPackingDedupUnionAndIsolation(t *testing.T) {
	for _, electra := range []bool{false, true} {
		ab := packingDedupAtt(electra, 1, 0, 0, 1)
		bc := packingDedupAtt(electra, 1, 0, 1, 2)
		abc := packingDedupAtt(electra, 1, 0, 0, 1, 2)
		otherCommittee := packingDedupAtt(electra, 1, 1, 0)
		otherRoot := packingDedupAtt(electra, 2, 0, 0)
		multi := packingDedupAtt(true, 1, 0, 0)
		multi.CommitteeBitsVal().SetBitAt(1, true)
		for _, atts := range []proposerAtts{
			{ab, bc, abc},
			{abc, ab, bc, otherCommittee, otherRoot},
			{abc, ab, bc, multi},
		} {
			want, err := referencePackingDedup(atts)
			require.NoError(t, err)
			got, err := atts.dedup()
			require.NoError(t, err)
			require.DeepEqual(t, packingDedupKeys(t, want), packingDedupKeys(t, got))
		}
	}
}

func TestProposalPackingDedupMalformedLength(t *testing.T) {
	a := packingDedupAtt(true, 1, 0, 0)
	b := packingDedupAtt(true, 1, 0, 0)
	b.(*ethpb.AttestationElectra).AggregationBits = bitfield.NewBitlist(64)
	_, err := proposerAtts{a, b}.dedup()
	require.ErrorContains(t, "bitlists are different lengths", err)
	_, referenceErr := referencePackingDedup(proposerAtts{a, b})
	require.Equal(t, referenceErr, err)
	large := make(proposerAtts, 33)
	for i := range large {
		large[i] = packingDedupAtt(true, 1, 0, uint64(i%32))
	}
	large[32].(*ethpb.AttestationElectra).AggregationBits = bitfield.NewBitlist(64)
	_, err = large.dedup()
	require.Equal(t, referenceErr, err)

	_, err = proposerAtts{&ethpb.Attestation{AggregationBits: bitfield.NewBitlist(32)}, a}.dedup()
	require.NotNil(t, err)
}

func TestProposalPackingDedupVersionIsolation(t *testing.T) {
	phase0 := packingDedupAtt(false, 1, 0, 0)
	electra := packingDedupAtt(true, 1, 0, 0)
	got, err := proposerAtts{phase0, electra}.dedup()
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
}

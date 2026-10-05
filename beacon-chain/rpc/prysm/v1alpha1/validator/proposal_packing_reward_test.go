//go:build !minimal

package validator

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coretime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func referencePackingReward(ctx context.Context, st state.ReadOnlyBeaconState, att ethpb.Att, totalBalance uint64) (uint64, error) {
	data := att.GetData()
	delay, err := st.Slot().SafeSubSlot(data.Slot)
	if err != nil {
		return 0, fmt.Errorf("attestation slot %d exceeds state slot %d", data.Slot, st.Slot())
	}
	parentSlot, err := gloas.ParentSlotFromBid(st)
	if err != nil {
		return 0, err
	}
	flags, err := altair.AttestationParticipationFlagIndices(st, data, delay, parentSlot)
	if err != nil {
		return 0, err
	}
	committees, err := helpers.AttestationCommitteesFromState(ctx, st, att)
	if err != nil {
		return 0, err
	}
	indices, err := attestation.AttestingIndices(att, committees...)
	if err != nil {
		return 0, err
	}
	var participation interface {
		Len() int
		At(uint64) byte
	}
	if data.Target.Epoch == coretime.CurrentRound(st) {
		participation, err = st.CurrentEpochParticipationReadOnly()
	} else {
		participation, err = st.PreviousEpochParticipationReadOnly()
	}
	if err != nil {
		return 0, err
	}
	cfg := params.BeaconConfig()
	var reward uint64
	for _, index := range indices {
		if index >= uint64(participation.Len()) {
			return 0, fmt.Errorf("index %d exceeds participation length %d", index, participation.Len())
		}
		base, err := altair.BaseRewardWithTotalBalance(st, primitives.ValidatorIndex(index), totalBalance)
		if err != nil {
			return 0, err
		}
		for _, entry := range []struct {
			flag   uint8
			weight uint64
		}{
			{cfg.TimelySourceFlagIndex, cfg.TimelySourceWeight},
			{cfg.TimelyTargetFlagIndex, cfg.TimelyTargetWeight},
			{cfg.TimelyHeadFlagIndex, cfg.TimelyHeadWeight},
		} {
			if flags[entry.flag] {
				hasVoted, err := altair.HasValidatorFlag(participation.At(index), entry.flag)
				if err != nil {
					return 0, err
				}
				if !hasVoted {
					reward += base * entry.weight
				}
			}
		}
	}
	return reward, nil
}

func TestProposalPackingRewardEquivalence(t *testing.T) {
	fixture := newPackingLargeFixture(t, true)
	st := fixture.state
	committee, err := helpers.BeaconCommitteeFromState(t.Context(), st, 13, 0)
	require.NoError(t, err)
	for position, index := range committee {
		if position%7 == 0 {
			validator, err := st.ValidatorAtIndex(index)
			require.NoError(t, err)
			validator.EffectiveBalance /= 2
			require.NoError(t, st.UpdateValidatorAtIndex(index, validator))
		}
	}
	participation, err := st.CurrentEpochParticipation()
	require.NoError(t, err)
	for position, index := range committee {
		if position%2 == 0 {
			participation[index] |= 1 << params.BeaconConfig().TimelySourceFlagIndex
		}
		if position%3 == 0 {
			participation[index] |= 1 << params.BeaconConfig().TimelyTargetFlagIndex
		}
	}
	require.NoError(t, st.SetCurrentParticipationBits(participation))
	totalBalance, err := helpers.TotalActiveBalance(t.Context(), st)
	require.NoError(t, err)
	packed, err := fixture.server.packAttestations(t.Context(), st, 14)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(packed))
	for _, att := range packed {
		want, wantErr := referencePackingReward(t.Context(), st, att, totalBalance)
		got, gotErr := electra.GetProposerRewardNumerator(t.Context(), st, att, totalBalance)
		require.Equal(t, wantErr, gotErr)
		require.Equal(t, want, got)
	}
	snapshot := fixture.server.AttPool.AggregatedAttestations()
	for _, att := range snapshot {
		if att.CommitteeBitsVal().BitAt(0) {
			phase0 := &ethpb.Attestation{AggregationBits: att.GetAggregationBits(), Data: att.GetData(), Signature: att.GetSignature()}
			want, wantErr := referencePackingReward(t.Context(), st, phase0, totalBalance)
			got, gotErr := electra.GetProposerRewardNumerator(t.Context(), st, phase0, totalBalance)
			require.Equal(t, wantErr, gotErr)
			require.Equal(t, want, got)
			break
		}
	}
	base := packed[0].Clone().(*ethpb.AttestationElectra)
	badLength := base.Copy()
	badLength.AggregationBits = bitfield.NewBitlist(base.GetAggregationBits().Len() + 1)
	want, wantErr := referencePackingReward(t.Context(), st, badLength, totalBalance)
	got, gotErr := electra.GetProposerRewardNumerator(t.Context(), st, badLength, totalBalance)
	require.ErrorContains(t, "bitfield length", wantErr)
	require.Equal(t, wantErr.Error(), gotErr.Error())
	require.Equal(t, want, got)
	if len(base.CommitteeBitsVal().BitIndices()) > 1 {
		emptyFirst := base.Copy()
		emptyFirst.AggregationBits = bitfield.NewBitlist(base.GetAggregationBits().Len())
		for _, bit := range base.GetAggregationBits().BitIndices() {
			if bit >= packingLargeCommitteeSize {
				emptyFirst.AggregationBits.SetBitAt(uint64(bit), true)
			}
		}
		want, wantErr = referencePackingReward(t.Context(), st, emptyFirst, totalBalance)
		got, gotErr = electra.GetProposerRewardNumerator(t.Context(), st, emptyFirst, totalBalance)
		require.ErrorContains(t, "no attesting indices", wantErr)
		require.Equal(t, wantErr.Error(), gotErr.Error())
		require.Equal(t, want, got)
	}
}

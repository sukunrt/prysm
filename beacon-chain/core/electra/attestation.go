package electra

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

var (
	ProcessAttestationsNoVerifySignature = altair.ProcessAttestationsNoVerifySignature
)

// GetProposerRewardNumerator returns the numerator of the proposer reward for an attestation.
func GetProposerRewardNumerator(
	ctx context.Context,
	st state.ReadOnlyBeaconState,
	att ethpb.Att,
	totalBalance uint64,
) (uint64, error) {
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

	var participation customtypes.ReadOnlyParticipation
	if data.Target.Epoch == time.CurrentRound(st) {
		participation, err = st.CurrentEpochParticipationReadOnly()
	} else {
		participation, err = st.PreviousEpochParticipationReadOnly()
	}
	if err != nil {
		return 0, err
	}

	cfg := params.BeaconConfig()
	var rewardNumerator uint64
	addReward := func(index uint64) error {
		if index >= uint64(participation.Len()) {
			return fmt.Errorf("index %d exceeds participation length %d", index, participation.Len())
		}

		br, err := altair.BaseRewardWithTotalBalance(st, primitives.ValidatorIndex(index), totalBalance)
		if err != nil {
			return err
		}

		for _, entry := range []struct {
			flagIndex uint8
			weight    uint64
		}{
			{cfg.TimelySourceFlagIndex, cfg.TimelySourceWeight},
			{cfg.TimelyTargetFlagIndex, cfg.TimelyTargetWeight},
			{cfg.TimelyHeadFlagIndex, cfg.TimelyHeadWeight},
		} {
			if flags[entry.flagIndex] { // If set, the validator voted correctly for the attestation given flag index.
				hasVoted, err := altair.HasValidatorFlag(participation.At(index), entry.flagIndex)
				if err != nil {
					return err
				}
				if !hasVoted { // If set, the validator has already voted in the beacon state so we don't double count.
					rewardNumerator += br * entry.weight
				}
			}
		}
		return nil
	}

	if att.Version() < version.Electra {
		indices, err := attestation.AttestingIndices(att, committees...)
		if err != nil {
			return 0, err
		}
		for _, index := range indices {
			if err := addReward(index); err != nil {
				return 0, err
			}
		}
		return rewardNumerator, nil
	}

	bits := att.GetAggregationBits()
	committeeLen := 0
	for _, committee := range committees {
		committeeLen += len(committee)
	}
	if bits.Len() != uint64(committeeLen) {
		return 0, fmt.Errorf("bitfield length %d is not equal to committee length %d", bits.Len(), committeeLen)
	}
	offset := uint64(0)
	for ci, committee := range committees {
		hasAttester := false
		for position, index := range committee {
			if !bits.BitAt(offset + uint64(position)) {
				continue
			}
			hasAttester = true
			if err := addReward(uint64(index)); err != nil {
				return 0, err
			}
		}
		if !hasAttester {
			return 0, fmt.Errorf("no attesting indices found for committee index %d", ci)
		}
		offset += uint64(len(committee))
	}

	return rewardNumerator, nil
}

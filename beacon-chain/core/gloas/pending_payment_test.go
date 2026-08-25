package gloas

import (
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderQuorumThreshold(t *testing.T) {
	helpers.ClearCache()
	cfg := params.BeaconConfig()

	validators := []*ethpb.Validator{
		{EffectiveBalance: cfg.MaxEffectiveBalance, ActivationEpoch: 0, ExitEpoch: 1},
		{EffectiveBalance: cfg.MaxEffectiveBalance, ActivationEpoch: 0, ExitEpoch: 1},
	}
	st, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{Validators: validators})
	require.NoError(t, err)

	got, err := builderQuorumThreshold(t.Context(), st)
	require.NoError(t, err)

	total := uint64(len(validators)) * cfg.MaxEffectiveBalance
	perSlot := total / uint64(cfg.SlotsPerRound)
	want := (perSlot * cfg.BuilderPaymentThresholdNumerator) / cfg.BuilderPaymentThresholdDenominator
	require.Equal(t, primitives.Gwei(want), got)

	// Identity config: the round is the epoch, so the epoch divisor gives the same answer.
	require.Equal(t, cfg.SlotsPerEpoch, cfg.SlotsPerRound)
}

// Under a round shorter than an epoch the threshold must follow the round: a slot's
// committees hold activeBalance/SlotsPerRound, so an epoch divisor would put the
// threshold out of reach and no builder payment would ever settle.
func TestBuilderQuorumThreshold_ShortRound(t *testing.T) {
	helpers.ClearCache()

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)

	// 256 validators, one round of 8 slots, so 32 validators attest per slot.
	validators := make([]*ethpb.Validator, 256)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			EffectiveBalance: cfg.MaxEffectiveBalance,
			ActivationEpoch:  0,
			ExitEpoch:        1,
		}
	}
	st, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{Validators: validators})
	require.NoError(t, err)

	got, err := builderQuorumThreshold(t.Context(), st)
	require.NoError(t, err)

	total := uint64(len(validators)) * cfg.MaxEffectiveBalance
	oneSlotShare := total / uint64(cfg.SlotsPerRound)
	want := (oneSlotShare * cfg.BuilderPaymentThresholdNumerator) / cfg.BuilderPaymentThresholdDenominator
	require.Equal(t, primitives.Gwei(want), got)

	// The threshold is reachable: a slot's whole attesting balance clears it.
	require.Equal(t, true, got <= primitives.Gwei(oneSlotShare))

	// The epoch divisor would have asked for four times a slot's share.
	epochDivisor := total / uint64(cfg.SlotsPerEpoch)
	require.Equal(t, oneSlotShare, epochDivisor*4)
}

func TestProcessBuilderPendingPayments(t *testing.T) {
	helpers.ClearCache()
	cfg := params.BeaconConfig()

	buildPayments := func(weights ...primitives.Gwei) []*ethpb.BuilderPendingPayment {
		p := make([]*ethpb.BuilderPendingPayment, 2*int(cfg.SlotsPerEpoch))
		for i := range p {
			p[i] = &ethpb.BuilderPendingPayment{
				Withdrawal: &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, 20)},
			}
		}
		for i, w := range weights {
			p[i].Weight = w
			p[i].Withdrawal.Amount = 1
		}
		return p
	}

	validators := []*ethpb.Validator{
		{EffectiveBalance: cfg.MaxEffectiveBalance, ActivationEpoch: 0, ExitEpoch: 1},
		{EffectiveBalance: cfg.MaxEffectiveBalance, ActivationEpoch: 0, ExitEpoch: 1},
	}
	pbSt, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Validators: validators})
	require.NoError(t, err)

	total := uint64(len(validators)) * cfg.MaxEffectiveBalance
	perSlot := total / uint64(cfg.SlotsPerEpoch)
	quorum := (perSlot * cfg.BuilderPaymentThresholdNumerator) / cfg.BuilderPaymentThresholdDenominator
	slotsPerEpoch := int(cfg.SlotsPerEpoch)

	t.Run("append qualifying withdrawals", func(t *testing.T) {
		payments := buildPayments(primitives.Gwei(quorum+1), primitives.Gwei(quorum+2))
		st := &testProcessState{BeaconState: pbSt, payments: payments}

		require.NoError(t, ProcessBuilderPendingPayments(t.Context(), st))
		require.Equal(t, 2, len(st.withdrawals))
		require.Equal(t, payments[0].Withdrawal, st.withdrawals[0])
		require.Equal(t, payments[1].Withdrawal, st.withdrawals[1])

		require.Equal(t, 2*slotsPerEpoch, len(st.payments))
		for i := slotsPerEpoch; i < 2*slotsPerEpoch; i++ {
			require.Equal(t, primitives.Gwei(0), st.payments[i].Weight)
			require.Equal(t, primitives.Gwei(0), st.payments[i].Withdrawal.Amount)
			require.Equal(t, 20, len(st.payments[i].Withdrawal.FeeRecipient))
		}
	})

	t.Run("no withdrawals when below quorum", func(t *testing.T) {
		payments := buildPayments(primitives.Gwei(quorum - 1))
		st := &testProcessState{BeaconState: pbSt, payments: payments}

		require.NoError(t, ProcessBuilderPendingPayments(t.Context(), st))
		require.Equal(t, 0, len(st.withdrawals))
	})
}

type testProcessState struct {
	state.BeaconState
	payments    []*ethpb.BuilderPendingPayment
	withdrawals []*ethpb.BuilderPendingWithdrawal
}

func (t *testProcessState) BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error) {
	return t.payments, nil
}

func (t *testProcessState) AppendBuilderPendingWithdrawals(withdrawals []*ethpb.BuilderPendingWithdrawal) error {
	t.withdrawals = append(t.withdrawals, withdrawals...)
	return nil
}

func (t *testProcessState) RotateBuilderPendingPayments() error {
	slotsPerEpoch := int(params.BeaconConfig().SlotsPerEpoch)
	rotated := slices.Clone(t.payments[slotsPerEpoch:])
	for range slotsPerEpoch {
		rotated = append(rotated, &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		})
	}
	t.payments = rotated
	return nil
}

package transition_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const roundTestValidators = 4

// hezeRoundState returns a Heze state at the given slot whose four validators are all
// active and have all three participation flags set for the current period, with the
// previous period empty.
func hezeRoundState(t *testing.T, slot primitives.Slot) state.BeaconState {
	t.Helper()
	far := params.BeaconConfig().FarFutureEpoch
	amount := params.BeaconConfig().MaxEffectiveBalance
	st, err := util.NewBeaconStateHeze(func(s *ethpb.BeaconStateHeze) error {
		s.Slot = slot
		s.Validators = make([]*ethpb.Validator, roundTestValidators)
		s.Balances = make([]uint64, roundTestValidators)
		s.InactivityScores = make([]uint64, roundTestValidators)
		s.PreviousEpochParticipation = make([]byte, roundTestValidators)
		s.CurrentEpochParticipation = make([]byte, roundTestValidators)
		for i := range roundTestValidators {
			s.Validators[i] = &ethpb.Validator{
				PublicKey:             make([]byte, 48),
				WithdrawalCredentials: make([]byte, 32),
				EffectiveBalance:      amount,
				ExitEpoch:             far,
				WithdrawableEpoch:     far,
			}
			s.Balances[i] = amount
			s.CurrentEpochParticipation[i] = 0b111
		}
		return nil
	})
	require.NoError(t, err)
	helpers.ClearCache()
	return st
}

func participation(t *testing.T, st state.BeaconState) (previous, current []byte) {
	t.Helper()
	previous, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	current, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	return previous, current
}

// TestProcessRound_At8Over32 pins the cadence split at the devnet's non-identity shape.
// Justification runs at every round boundary, and the participation arrays rotate there
// too -- except at a boundary that is also an epoch boundary, where epoch processing
// still owns the rotation at its original late position so rewards keep reading the
// pre-rotation arrays.
func TestProcessRound_At8Over32(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	cfg.FFGTargetOffsetSlots = 1
	params.OverrideBeaconConfig(cfg)

	full := []byte{0b111, 0b111, 0b111, 0b111}
	empty := []byte{0, 0, 0, 0}

	t.Run("mid-epoch round boundary justifies and rotates", func(t *testing.T) {
		// Slot 71 is the last slot of round 8 and sits in the middle of epoch 2.
		st, err := transition.ProcessRound(t.Context(), hezeRoundState(t, 71))
		require.NoError(t, err)
		assert.Equal(t, primitives.Round(8), st.CurrentJustifiedCheckpoint().Epoch)
		previous, current := participation(t, st)
		assert.DeepEqual(t, full, previous)
		assert.DeepEqual(t, empty, current)
	})

	t.Run("epoch boundary justifies but defers the rotation", func(t *testing.T) {
		// Slot 95 ends round 11 and epoch 2 at once.
		st, err := transition.ProcessRound(t.Context(), hezeRoundState(t, 95))
		require.NoError(t, err)
		assert.Equal(t, primitives.Round(11), st.CurrentJustifiedCheckpoint().Epoch)
		previous, current := participation(t, st)
		assert.DeepEqual(t, empty, previous)
		assert.DeepEqual(t, full, current)
	})

	t.Run("mid-round slot is a no-op", func(t *testing.T) {
		st, err := transition.ProcessRound(t.Context(), hezeRoundState(t, 70))
		require.NoError(t, err)
		assert.Equal(t, primitives.Round(0), st.CurrentJustifiedCheckpoint().Epoch)
		previous, current := participation(t, st)
		assert.DeepEqual(t, empty, previous)
		assert.DeepEqual(t, full, current)
	})
}

// TestProcessRound_IdentityNeverRotates is the identity rule for the cadence split:
// under the shipped configs every round boundary is an epoch boundary, so the round
// part never rotates and the whole sequence stays value-identical to Gloas.
func TestProcessRound_IdentityNeverRotates(t *testing.T) {
	require.Equal(t, params.BeaconConfig().SlotsPerEpoch, params.BeaconConfig().SlotsPerRound)

	slot := params.BeaconConfig().SlotsPerEpoch*3 - 1
	st, err := transition.ProcessRound(t.Context(), hezeRoundState(t, slot))
	require.NoError(t, err)
	assert.Equal(t, primitives.Round(2), st.CurrentJustifiedCheckpoint().Epoch)
	previous, current := participation(t, st)
	assert.DeepEqual(t, []byte{0, 0, 0, 0}, previous)
	assert.DeepEqual(t, []byte{0b111, 0b111, 0b111, 0b111}, current)
}

// TestProcessRound_PreHezeIsUntouched: Gloas and below have no round cadence, and their
// epoch processing keeps justification where it has always been.
func TestProcessRound_PreHezeIsUntouched(t *testing.T) {
	st, err := util.NewBeaconStateGloas()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*3-1))
	out, err := transition.ProcessRound(t.Context(), st)
	require.NoError(t, err)
	assert.Equal(t, primitives.Round(0), out.CurrentJustifiedCheckpoint().Epoch)
}

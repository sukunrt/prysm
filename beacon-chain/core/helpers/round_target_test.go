package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// setupRoundsConfig switches the running config to the non-identity shape the
// devnet uses -- 8-slot rounds inside 32-slot epochs -- with the given FFG
// target offset. Everything the identity rule hides is only visible here.
func setupRoundsConfig(t *testing.T, offset primitives.Slot) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	cfg.FFGTargetOffsetSlots = offset
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
}

// stateWithMarkedBlockRoots returns a state whose block root at slot i is the
// 32-byte value {byte(i)}, so a target root identifies its slot by inspection.
func stateWithMarkedBlockRoots(t *testing.T, slot primitives.Slot) state.BeaconState {
	t.Helper()
	roots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range roots {
		roots[i] = []byte{byte(i)}
	}
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		BlockRoots: roots,
		Slot:       slot,
	})
	require.NoError(t, err)
	return st
}

func TestFFGTargetSlot_BothOffsetsAt8Over32(t *testing.T) {
	t.Run("offset 1 targets the last slot before the round", func(t *testing.T) {
		setupRoundsConfig(t, 1)
		for _, tc := range []struct {
			round primitives.Round
			want  primitives.Slot
		}{
			{0, 0}, // underflow clamps at the anchor
			{1, 7},
			{2, 15},
			{5, 39},
		} {
			got, err := slots.FFGTargetSlot(tc.round)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "round %d", tc.round)
		}
	})

	t.Run("offset 0 targets the round's own first slot", func(t *testing.T) {
		setupRoundsConfig(t, 0)
		for _, tc := range []struct {
			round primitives.Round
			want  primitives.Slot
		}{
			{0, 0},
			{1, 8},
			{2, 16},
			{5, 40},
		} {
			got, err := slots.FFGTargetSlot(tc.round)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "round %d", tc.round)
		}
	})
}

func TestFFGTargetRoot_BothOffsetsAt8Over32(t *testing.T) {
	t.Run("offset 1", func(t *testing.T) {
		setupRoundsConfig(t, 1)
		st := stateWithMarkedBlockRoots(t, 200)

		// Round 0 has no earlier slot, so it names the anchor block.
		r, err := helpers.FFGTargetRoot(st, 0)
		require.NoError(t, err)
		anchor := [32]byte{0}
		assert.DeepEqual(t, anchor[:], r)

		// Round 5 spans slots 40-47; its target is the block at slot 39.
		r, err = helpers.FFGTargetRoot(st, 5)
		require.NoError(t, err)
		want := [32]byte{39}
		assert.DeepEqual(t, want[:], r)
	})

	t.Run("offset 0", func(t *testing.T) {
		setupRoundsConfig(t, 0)
		st := stateWithMarkedBlockRoots(t, 200)

		r, err := helpers.FFGTargetRoot(st, 0)
		require.NoError(t, err)
		anchor := [32]byte{0}
		assert.DeepEqual(t, anchor[:], r)

		// Round 5's own first slot is 40.
		r, err = helpers.FFGTargetRoot(st, 5)
		require.NoError(t, err)
		want := [32]byte{40}
		assert.DeepEqual(t, want[:], r)
	})
}

// TestFFGTargetRoot_RoundsNotEpochsAt8Over32 is the test the identity rule
// cannot express: with 8-slot rounds inside 32-slot epochs the round target and
// the epoch target are different blocks.
func TestFFGTargetRoot_RoundsNotEpochsAt8Over32(t *testing.T) {
	setupRoundsConfig(t, 1)
	st := stateWithMarkedBlockRoots(t, 200)

	// Slot 44 is in epoch 1 (slots 32-63) and round 5 (slots 40-47).
	assert.Equal(t, primitives.Epoch(1), slots.ToEpoch(44))
	assert.Equal(t, primitives.Round(5), slots.RoundAt(44))

	r, err := helpers.FFGTargetRoot(st, slots.RoundAt(44))
	require.NoError(t, err)
	roundTarget := [32]byte{39}
	assert.DeepEqual(t, roundTarget[:], r, "the round target is the block at slot 39")

	epochStart, err := slots.EpochStart(slots.ToEpoch(44))
	require.NoError(t, err)
	epochTarget := [32]byte{byte(epochStart - 1)}
	assert.Equal(t, primitives.Slot(32), epochStart)
	require.DeepNotEqual(t, epochTarget[:], roundTarget[:])
}

func TestCheckpointEpoch_At8Over32(t *testing.T) {
	setupRoundsConfig(t, 1)
	for _, tc := range []struct {
		round primitives.Round
		want  primitives.Epoch
	}{
		{0, 0},
		{3, 0},  // rounds 0-3 are slots 0-31, epoch 0
		{4, 1},  // round 4 starts at slot 32, epoch 1
		{7, 1},  // round 7 is slots 56-63, still epoch 1
		{8, 2},  // round 8 starts at slot 64
		{12, 3}, // round 12 starts at slot 96
	} {
		got, err := helpers.CheckpointEpoch(tc.round)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "round %d", tc.round)
	}
}

// TestIsEligibleForActivation_GatesOnTheCheckpointsEpochAt8Over32 is the
// activation-gate half of the mixed-units audit: reading a finalized ROUND as
// an epoch would unlock activations roughly four rounds early on the devnet.
func TestIsEligibleForActivation_GatesOnTheCheckpointsEpochAt8Over32(t *testing.T) {
	setupRoundsConfig(t, 1)
	// Finalized round 6 sits in epoch 1 (round 6 starts at slot 48).
	finalizedRound := primitives.Round(6)
	finalizedEpoch, err := helpers.CheckpointEpoch(finalizedRound)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(1), finalizedEpoch)

	eligibleInEpoch1 := &ethpb.Validator{
		ActivationEligibilityEpoch: 1,
		ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
	}
	assert.Equal(t, true, helpers.IsEligibleForActivation(finalizedEpoch, eligibleInEpoch1))

	// Epoch 2 is NOT finalized: reading round 6 as epoch 6 would let it through.
	eligibleInEpoch2 := &ethpb.Validator{
		ActivationEligibilityEpoch: 2,
		ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
	}
	assert.Equal(t, false, helpers.IsEligibleForActivation(finalizedEpoch, eligibleInEpoch2))
}

func TestValidateSlotTargetRound_At8Over32(t *testing.T) {
	setupRoundsConfig(t, 1)
	// Slot 44 is round 5; a target naming its epoch (1) must be rejected.
	require.NoError(t, helpers.ValidateSlotTargetRound(&ethpb.AttestationData{
		Slot:   44,
		Target: &ethpb.Checkpoint{Epoch: 5},
	}))
	err := helpers.ValidateSlotTargetRound(&ethpb.AttestationData{
		Slot:   44,
		Target: &ethpb.Checkpoint{Epoch: 1},
	})
	require.ErrorContains(t, "does not match target round", err)
}

// TestFinalityDelay_StaysEpochBasedAt8Over32 pins plan-finality-round 2.5: the
// inactivity leak keeps counting EPOCHS, and the retype's only consequence is the one
// conversion of the now round-valued finalized checkpoint at the callers. While
// finality advances every round the delay stays 0 and the leak never arms; when
// finality stalls the delay grows one per epoch, exactly as it did before.
func TestFinalityDelay_StaysEpochBasedAt8Over32(t *testing.T) {
	setupRoundsConfig(t, 1)

	// Finality advancing once per round. Rounds 8-11 all start inside epoch 2, so the
	// finalized EPOCH the leak reads never moves while the finalized ROUND moves four
	// times -- and the delay is 0 throughout.
	for _, finalized := range []primitives.Round{8, 9, 10, 11} {
		finalizedEpoch, err := helpers.CheckpointEpoch(finalized)
		require.NoError(t, err)
		assert.Equal(t, primitives.Epoch(2), finalizedEpoch, "round %d", finalized)
		assert.Equal(t, primitives.Epoch(0), helpers.FinalityDelay(2, finalizedEpoch))
		assert.Equal(t, false, helpers.IsInInactivityLeak(2, finalizedEpoch))
	}

	// Finality stalled at round 8 while the chain runs on: the delay counts epochs and
	// the leak arms once it passes MinEpochsToInactivityPenalty.
	stalled, err := helpers.CheckpointEpoch(8)
	require.NoError(t, err)
	for _, tc := range []struct {
		prevEpoch primitives.Epoch
		wantDelay primitives.Epoch
		wantLeak  bool
	}{
		{prevEpoch: 2, wantDelay: 0, wantLeak: false},
		{prevEpoch: 6, wantDelay: 4, wantLeak: false},
		{prevEpoch: 8, wantDelay: 6, wantLeak: true},
	} {
		assert.Equal(t, tc.wantDelay, helpers.FinalityDelay(tc.prevEpoch, stalled),
			"previous epoch %d", tc.prevEpoch)
		assert.Equal(t, tc.wantLeak, helpers.IsInInactivityLeak(tc.prevEpoch, stalled),
			"previous epoch %d", tc.prevEpoch)
	}
}

package precompute_test

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestProcessJustificationAndFinalizationPreCompute_ConsecutiveEpochs(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := range blockRoots {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		FinalizedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:   bitfield.Bitvector4{0x0F}, // 0b1111
		Validators:          []*ethpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:            []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:          blockRoots,
	}
	state, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	// The FFG target for epoch 2 is the block at slot 63, one slot before the
	// epoch starts.
	rt := [32]byte{byte(63)}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected justified root")
	assert.Equal(t, primitives.Round(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.Equal(t, primitives.Round(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root, "Unexpected finalized root")
	assert.Equal(t, primitives.Round(0), newState.FinalizedCheckpointRound(), "Unexpected finalized epoch")
}

func TestProcessJustificationAndFinalizationPreCompute_JustifyCurrentEpoch(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := range blockRoots {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		FinalizedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:   bitfield.Bitvector4{0x03}, // 0b0011
		Validators:          []*ethpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:            []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:          blockRoots,
	}
	state, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	// The FFG target for epoch 2 is the block at slot 63, one slot before the
	// epoch starts.
	rt := [32]byte{byte(63)}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected current justified root")
	assert.Equal(t, primitives.Round(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.Equal(t, primitives.Round(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root)
	assert.Equal(t, primitives.Round(0), newState.FinalizedCheckpointRound(), "Unexpected finalized epoch")
}

func TestProcessJustificationAndFinalizationPreCompute_JustifyPrevEpoch(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := range blockRoots {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		JustificationBits: bitfield.Bitvector4{0x03}, // 0b0011
		Validators:        []*ethpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:          []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:        blockRoots, FinalizedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	state, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	// The FFG target for epoch 2 is the block at slot 63, one slot before the
	// epoch starts.
	rt := [32]byte{byte(63)}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected current justified root")
	assert.Equal(t, primitives.Round(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.Equal(t, primitives.Round(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root)
	assert.Equal(t, primitives.Round(0), newState.FinalizedCheckpointRound(), "Unexpected finalized epoch")
}

func TestUnrealizedCheckpoints(t *testing.T) {
	validators := make([]*ethpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	balances := make([]uint64, len(validators))
	for i := range validators {
		validators[i] = &ethpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	pjr := [32]byte{'p'}
	cjr := [32]byte{'c'}
	je := primitives.Round(3)
	fe := primitives.Round(2)
	pjcp := &ethpb.Checkpoint{Root: pjr[:], Epoch: fe}
	cjcp := &ethpb.Checkpoint{Root: cjr[:], Epoch: je}
	fcp := &ethpb.Checkpoint{Root: pjr[:], Epoch: fe}
	tests := []struct {
		name                                 string
		slot                                 primitives.Slot
		prevVals, currVals                   int
		expectedJustified, expectedFinalized primitives.Round // The expected unrealized checkpoint rounds
	}{
		{
			"Not enough votes, keep previous justification",
			129,
			len(validators) / 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Not enough votes, keep previous justification, N+2",
			161,
			len(validators) / 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Enough to justify previous epoch but not current",
			129,
			2*len(validators)/3 + 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Enough to justify previous epoch but not current, N+2",
			161,
			2*len(validators)/3 + 3,
			len(validators) / 3,
			je + 1,
			fe,
		},
		{
			"Enough to justify current epoch",
			129,
			len(validators) / 3,
			2*len(validators)/3 + 3,
			je + 1,
			fe,
		},
		{
			"Enough to justify current epoch, but not previous",
			161,
			len(validators) / 3,
			2*len(validators)/3 + 3,
			je + 2,
			fe,
		},
		{
			"Enough to justify current and previous",
			161,
			2*len(validators)/3 + 3,
			2*len(validators)/3 + 3,
			je + 2,
			fe,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := &ethpb.BeaconStateAltair{
				RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),

				Validators:                  validators,
				Slot:                        test.slot,
				CurrentEpochParticipation:   make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
				PreviousEpochParticipation:  make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
				Balances:                    balances,
				PreviousJustifiedCheckpoint: pjcp,
				CurrentJustifiedCheckpoint:  cjcp,
				FinalizedCheckpoint:         fcp,
				InactivityScores:            make([]uint64, len(validators)),
				JustificationBits:           make(bitfield.Bitvector4, 1),
			}
			for i := 0; i < test.prevVals; i++ {
				base.PreviousEpochParticipation[i] = 0xFF
			}
			for i := 0; i < test.currVals; i++ {
				base.CurrentEpochParticipation[i] = 0xFF
			}
			if test.slot > 130 {
				base.JustificationBits.SetBitAt(2, true)
				base.JustificationBits.SetBitAt(3, true)
			} else {
				base.JustificationBits.SetBitAt(1, true)
				base.JustificationBits.SetBitAt(2, true)
			}

			state, err := state_native.InitializeFromProtoAltair(base)
			require.NoError(t, err)

			_, _, err = altair.InitializePrecomputeValidators(t.Context(), state)
			require.NoError(t, err)

			jc, fc, err := precompute.UnrealizedCheckpoints(state)
			require.NoError(t, err)
			require.DeepEqual(t, test.expectedJustified, jc.Epoch)
			require.DeepEqual(t, test.expectedFinalized, fc.Epoch)
		})
	}
}

func Test_ComputeCheckpoints_CantUpdateToLower(t *testing.T) {
	st, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Slot: params.BeaconConfig().SlotsPerEpoch * 2,
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{
			Epoch: 2,
		},
	})
	require.NoError(t, err)
	jb := make(bitfield.Bitvector4, 1)
	jb.SetBitAt(1, true)
	cp, _, err := precompute.ComputeCheckpoints(st, jb)
	require.NoError(t, err)
	require.Equal(t, primitives.Round(2), cp.Epoch)
}

// TestProcessJustificationAndFinalization_RoundProgressionAt8Over32 pins the per-round
// finality cadence at the devnet's non-identity shape: 8-slot rounds inside 32-slot
// epochs. With full participation, round R is justified at the R->R+1 boundary and
// finalized at the R+1->R+2 boundary -- a finality latency of two ROUNDS, 16 slots,
// against the two epochs (64 slots) the same votes would have bought before.
func TestProcessJustificationAndFinalization_RoundProgressionAt8Over32(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	cfg.FFGTargetOffsetSlots = 1
	params.OverrideBeaconConfig(cfg)

	far := params.BeaconConfig().FarFutureEpoch
	amount := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &ethpb.BeaconState{
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:           bitfield.Bitvector4{0x00},
		Validators: []*ethpb.Validator{
			{ExitEpoch: far}, {ExitEpoch: far}, {ExitEpoch: far}, {ExitEpoch: far},
		},
		Balances:   []uint64{amount, amount, amount, amount},
		BlockRoots: blockRoots,
	}
	st, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)

	// Everyone attests to the correct target every round.
	total := 4 * amount
	full := &precompute.Balance{
		ActiveCurrentEpoch:         total,
		PrevEpochTargetAttested:    total,
		CurrentEpochTargetAttested: total,
	}

	// The genesis guard stays epoch-based, so the first boundary that justifies at all
	// is the first round ending past slot EpochStart(2) == 64, which is round 8.
	for _, tc := range []struct {
		round     primitives.Round
		justified primitives.Round
		finalized primitives.Round
	}{
		{round: 7, justified: 0, finalized: 0}, // still inside the genesis guard
		{round: 8, justified: 8, finalized: 0},
		{round: 9, justified: 9, finalized: 8},
		{round: 10, justified: 10, finalized: 9},
		{round: 11, justified: 11, finalized: 10},
	} {
		end, err := slots.RoundEnd(tc.round)
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(end))
		st, err = precompute.ProcessJustificationAndFinalizationPreCompute(st, full)
		require.NoError(t, err)
		assert.Equal(t, tc.justified, st.CurrentJustifiedCheckpoint().Epoch,
			"justified round after the round %d boundary", tc.round)
		assert.Equal(t, tc.finalized, st.FinalizedCheckpointRound(),
			"finalized round after the round %d boundary", tc.round)
	}

	// Rounds 8-11 all live inside epoch 2, so the whole progression above happened
	// without a single epoch boundary: this cadence is invisible to epoch processing.
	firstEpoch, err := helpers.CheckpointEpoch(8)
	require.NoError(t, err)
	lastEpoch, err := helpers.CheckpointEpoch(11)
	require.NoError(t, err)
	assert.Equal(t, primitives.Epoch(2), firstEpoch)
	assert.Equal(t, primitives.Epoch(2), lastEpoch)

	// The finalized checkpoint names round 10's FFG target block, the last block
	// before the round started: RoundStart(10) - 1 == slot 79.
	targetSlot, err := slots.FFGTargetSlot(10)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(79), targetSlot)
	want := bytesutil.ToBytes32(blockRoots[targetSlot])
	assert.DeepEqual(t, want[:], st.FinalizedCheckpoint().Root)

	// Two rounds of latency: round 10's target block sits at slot 79 and the state
	// learns it is final while still in round 11, at slot 95.
	assert.Equal(t, primitives.Round(11), time.CurrentRound(st))
	assert.Equal(t, primitives.Slot(95), st.Slot())
	assert.Equal(t, primitives.Slot(16), st.Slot()-targetSlot)
	assert.Equal(t, primitives.Slot(16), 2*params.BeaconConfig().SlotsPerRound)
}

package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestSameRootRoundCheckpointStateDiagnostic exercises getAttPreState with the
// empty-chain shape: the genesis block remains the checkpoint root while the
// checkpoint round advances. It verifies regeneration and cache isolation,
// rather than constructing the later states by setting their slots directly.
func TestSameRootRoundCheckpointStateDiagnostic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	service, _ := minimalTestService(t)
	ctx := t.Context()
	genesisState, _ := util.DeterministicGenesisState(t, 64)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, genesisRoot))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Root: genesisRoot[:]}))

	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: genesisRoot[:]}
	fcState, fcBlock, err := prepareForkchoiceState(
		ctx,
		0,
		genesisRoot,
		params.BeaconConfig().ZeroHash,
		[32]byte{'s'},
		cp0,
		cp0,
	)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, fcState, fcBlock))

	state0, err := service.getAttPreState(ctx, cp0)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), state0.Slot())

	cp1 := &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.SafeCopyBytes(genesisRoot[:])}
	state8, err := service.getAttPreState(ctx, cp1)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(8), state8.Slot())

	// Advancing the same root for a new round must not mutate either the state
	// returned earlier or the independently keyed round-0 cache entry.
	require.Equal(t, primitives.Slot(0), state0.Slot())
	cached0, err := service.checkpointStateCache.StateByCheckpoint(cp0)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), cached0.Slot())
	cached1, err := service.checkpointStateCache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(8), cached1.Slot())

	cp2 := &ethpb.Checkpoint{Epoch: 2, Root: bytesutil.SafeCopyBytes(genesisRoot[:])}
	state16, err := service.getAttPreState(ctx, cp2)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(16), state16.Slot())

	// Populate the real committee cache, then exercise ActiveValidatorCount on
	// both regenerated nonzero-slot states without changing either state.
	require.NoError(t, helpers.UpdateCommitteeCache(ctx, state8, 0))
	count8, err := helpers.ActiveValidatorCount(ctx, state8, 0)
	require.NoError(t, err)
	count16, err := helpers.ActiveValidatorCount(ctx, state16, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(64), count8)
	require.Equal(t, count8, count16)
}

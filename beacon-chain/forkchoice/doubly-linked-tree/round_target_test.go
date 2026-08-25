package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// setupRoundsConfig switches the running config to 8-slot rounds inside 32-slot
// epochs -- the devnet shape, where rounds and epochs are no longer the same
// number -- with the given FFG target offset.
func setupRoundsConfig(t *testing.T, offset primitives.Slot) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	cfg.FFGTargetOffsetSlots = offset
	params.OverrideBeaconConfig(cfg)
}

// stateForChain returns a state whose block root at every slot is the root of
// the deepest chain block at or before that slot, mirroring the spec's
// copy-forward block_roots. It is the state side of the target computation, so
// helpers.FFGTargetRoot and forkchoice's node.target can be compared directly.
func stateForChain(t *testing.T, chain map[primitives.Slot][32]byte, headSlot primitives.Slot) *ethpb.BeaconState {
	t.Helper()
	roots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	var last [32]byte
	for s := primitives.Slot(0); s <= headSlot; s++ {
		if r, ok := chain[s]; ok {
			last = r
		}
		c := last
		roots[s] = c[:]
	}
	for i := range roots {
		if roots[i] == nil {
			roots[i] = make([]byte, 32)
		}
	}
	return &ethpb.BeaconState{BlockRoots: roots, Slot: headSlot}
}

// TestTargetRootForRound_MatchesFFGTargetRoot is the symmetric-pair check: the
// forkchoice target and the state target must name the same block for every
// round on the same chain, at every supported offset. If they diverge,
// VerifyLmdFfgConsistency rejects every vote.
func TestTargetRootForRound_MatchesFFGTargetRoot(t *testing.T) {
	// Slot 16 is a round start that is deliberately EMPTY: it is the
	// offset-0 corner the review named.
	chainSlots := []primitives.Slot{1, 2, 7, 8, 9, 15, 17, 18, 23, 24, 25}

	for _, offset := range []primitives.Slot{1, 0} {
		t.Run("offset "+string(rune('0'+offset)), func(t *testing.T) {
			setupRoundsConfig(t, offset)
			ctx := t.Context()
			f := setup(0, 0)

			chain := map[primitives.Slot][32]byte{}
			parent := params.BeaconConfig().ZeroHash
			var headRoot [32]byte
			for _, s := range chainSlots {
				root := [32]byte{byte(s)}
				st, blk, err := prepareForkchoiceState(ctx, s, root, parent, params.BeaconConfig().ZeroHash, 0, 0)
				require.NoError(t, err)
				require.NoError(t, f.InsertNode(ctx, st, blk))
				chain[s] = root
				parent = root
				headRoot = root
			}

			headSlot := chainSlots[len(chainSlots)-1]
			st, err := state_native.InitializeFromProtoPhase0(stateForChain(t, chain, headSlot))
			require.NoError(t, err)

			for round := primitives.Round(0); round <= slots.RoundAt(headSlot); round++ {
				want, err := helpers.FFGTargetRoot(st, round)
				require.NoError(t, err)
				got, err := f.TargetRootForRound(headRoot, round)
				require.NoError(t, err)
				require.DeepEqual(t, want, got[:], "round %d", round)
			}
		})
	}
}

// TestInsert_TargetIsTheRoundsTargetBlock pins the node.target insert rule at
// each offset, including the empty-round-start-slot case.
func TestInsert_TargetIsTheRoundsTargetBlock(t *testing.T) {
	ctx := t.Context()

	t.Run("offset 1: blocks in a round share the last block before it", func(t *testing.T) {
		setupRoundsConfig(t, 1)
		f := setup(0, 0)
		zero := params.BeaconConfig().ZeroHash
		a, b, c := [32]byte{7}, [32]byte{8}, [32]byte{9}
		insertChain(t, ctx, f, []chainBlock{{7, a, zero}, {8, b, a}, {9, c, b}})

		// Round 1 is slots 8-15; its target is the block at slot 7.
		require.Equal(t, a, f.store.emptyNodeByRoot[b].node.target.root)
		require.Equal(t, a, f.store.emptyNodeByRoot[c].node.target.root)
	})

	t.Run("offset 0: the round-start block is its own target", func(t *testing.T) {
		setupRoundsConfig(t, 0)
		f := setup(0, 0)
		zero := params.BeaconConfig().ZeroHash
		a, b, c := [32]byte{7}, [32]byte{8}, [32]byte{9}
		insertChain(t, ctx, f, []chainBlock{{7, a, zero}, {8, b, a}, {9, c, b}})

		require.Equal(t, b, f.store.emptyNodeByRoot[b].node.target.root)
		require.Equal(t, b, f.store.emptyNodeByRoot[c].node.target.root)
	})

	t.Run("offset 0: an empty round start falls back to the parent", func(t *testing.T) {
		setupRoundsConfig(t, 0)
		f := setup(0, 0)
		zero := params.BeaconConfig().ZeroHash
		// Slot 8 is skipped, so the round-1 block at slot 9 is not at the
		// round start and must take its parent, not itself.
		a, c := [32]byte{7}, [32]byte{9}
		insertChain(t, ctx, f, []chainBlock{{7, a, zero}, {9, c, a}})

		require.Equal(t, a, f.store.emptyNodeByRoot[c].node.target.root)
	})
}

// TestIsViableForCheckpoint_OffsetBoundAt8Over32 pins the offset-dependent arm
// of IsViableForCheckpoint: at offset 1 a child sitting exactly on the round's
// first slot makes its parent viable; at offset 0 that child is its own target
// and is no evidence for the parent.
func TestIsViableForCheckpoint_OffsetBoundAt8Over32(t *testing.T) {
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	// The node sits in round 0 and the checkpoint is round 2, two rounds ahead,
	// so the "checkpoint round - 1" arm does not short-circuit the decision and
	// the child bound is what is under test. Round 2 starts at slot 16.
	parent, childAtStart := [32]byte{6}, [32]byte{16}
	chain := []chainBlock{{6, parent, zero}, {16, childAtStart, parent}}
	cp := &forkchoicetypes.Checkpoint{Root: parent, Epoch: 2}

	t.Run("offset 1 admits the round-start child as evidence", func(t *testing.T) {
		setupRoundsConfig(t, 1)
		f := setup(0, 0)
		insertChain(t, ctx, f, chain)
		ok, err := f.IsViableForCheckpoint(cp)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("offset 0 rejects it", func(t *testing.T) {
		setupRoundsConfig(t, 0)
		f := setup(0, 0)
		insertChain(t, ctx, f, chain)
		ok, err := f.IsViableForCheckpoint(cp)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}

// TestStore_PruneBoundIsOffsetAware is the rounds twin of
// TestStore_PruneKeepsTheEpochStartChild. The bound prune uses must be the same
// offset-aware one the insert rule and IsViableForCheckpoint read: a child of the
// finalized node survives only when it sits strictly after the round's FFG target
// slot. At offset 1 that keeps a child exactly on the round's first slot; at
// offset 0 that child is its own target and contradicts the finalized checkpoint,
// so it must go.
func TestStore_PruneBoundIsOffsetAware(t *testing.T) {
	for _, tt := range []struct {
		offset            primitives.Slot
		keepsRoundStarter bool
	}{
		{offset: 1, keepsRoundStarter: true},
		{offset: 0, keepsRoundStarter: false},
	} {
		t.Run("offset "+string(rune('0'+tt.offset)), func(t *testing.T) {
			setupRoundsConfig(t, tt.offset)
			ctx := t.Context()
			f := setup(0, 0)
			zero := params.BeaconConfig().ZeroHash
			spr := params.BeaconConfig().SlotsPerRound

			checkpoint, competing, roundStarter := [32]byte{1}, [32]byte{2}, [32]byte{3}
			insertChain(t, ctx, f, []chainBlock{
				{spr - 2, checkpoint, zero},
				{spr - 1, competing, checkpoint},
				{spr, roundStarter, checkpoint},
			})

			s := f.store
			s.finalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: checkpoint}
			s.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: checkpoint}
			require.NoError(t, s.prune(ctx))

			// The competing block sits before the round starts on either offset.
			require.Equal(t, false, f.HasNode(competing))
			require.Equal(t, tt.keepsRoundStarter, f.HasNode(roundStarter))
			if !tt.keepsRoundStarter {
				return
			}
			target, err := f.TargetRootForRound(roundStarter, 1)
			require.NoError(t, err)
			require.Equal(t, checkpoint, target)
		})
	}
}

// TestDependentRootForEpoch_UnchangedByRoundTargets checks step 1.3: shuffling
// stays epoch-keyed even though the target pointers it rides are round-keyed.
func TestDependentRootForEpoch_UnchangedByRoundTargets(t *testing.T) {
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	// A dense chain across epoch 0 and into epoch 1 (slots 0-35 at 32/epoch).
	var blocks []chainBlock
	parent := zero
	for s := primitives.Slot(1); s <= 35; s++ {
		root := [32]byte{byte(s)}
		blocks = append(blocks, chainBlock{s, root, parent})
		parent = root
	}
	head := parent

	for _, offset := range []primitives.Slot{1, 0} {
		t.Run("offset "+string(rune('0'+offset)), func(t *testing.T) {
			setupRoundsConfig(t, offset)
			f := setup(0, 0)
			insertChain(t, ctx, f, blocks)

			// The dependent root for epoch 1 is the last block before slot 32.
			got, err := f.store.dependentRootForEpoch(head, 1)
			require.NoError(t, err)
			require.Equal(t, [32]byte{31}, got)
		})
	}
}

type chainBlock struct {
	slot   primitives.Slot
	root   [32]byte
	parent [32]byte
}

func insertChain(t *testing.T, ctx context.Context, f *ForkChoice, blocks []chainBlock) {
	t.Helper()
	for _, b := range blocks {
		st, blk, err := prepareForkchoiceState(
			ctx, b.slot, b.root, b.parent, params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))
	}
}

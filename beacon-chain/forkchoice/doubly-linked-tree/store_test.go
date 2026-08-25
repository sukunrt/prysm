package doublylinkedtree

import (
	"testing"
	"time"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStore_JustifiedEpoch(t *testing.T) {
	j := primitives.Round(100)
	f := setup(j, j)
	require.Equal(t, j, f.JustifiedCheckpoint().Epoch)
}

func TestStore_FinalizedEpoch(t *testing.T) {
	j := primitives.Round(50)
	f := setup(j, j)
	require.Equal(t, j, f.FinalizedCheckpoint().Epoch)
}

func TestStore_NodeCount(t *testing.T) {
	f := setup(0, 0)
	state, blk, err := prepareForkchoiceState(t.Context(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(t.Context(), state, blk))
	require.Equal(t, 2, f.NodeCount())
}

func TestStore_NodeByRoot(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(t.Context(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(t.Context(), 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	node0 := f.store.emptyNodeByRoot[params.BeaconConfig().ZeroHash]
	node1 := f.store.emptyNodeByRoot[indexToHash(1)]
	node2 := f.store.emptyNodeByRoot[indexToHash(2)]

	expectedRoots := map[[32]byte]*PayloadNode{
		params.BeaconConfig().ZeroHash: node0,
		indexToHash(1):                 node1,
		indexToHash(2):                 node2,
	}

	require.Equal(t, 3, f.NodeCount())
	for root, node := range f.store.emptyNodeByRoot {
		v, ok := expectedRoots[root]
		require.Equal(t, ok, true)
		require.Equal(t, v, node)
	}
}

func TestForkChoice_HasNode(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(t.Context(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.Equal(t, true, f.HasNode(indexToHash(1)))
}

func TestStore_Head_UnknownJustifiedRoot(t *testing.T) {
	f := setup(0, 0)

	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: [32]byte{'a'}}
	_, err := f.store.head(t.Context())
	assert.ErrorContains(t, errUnknownJustifiedRoot.Error(), err)
}

func TestStore_Head_Itself(t *testing.T) {
	f := setup(0, 0)
	state, blkRoot, err := prepareForkchoiceState(t.Context(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(t.Context(), state, blkRoot))

	// Since the justified node does not have a best descendant, the best node
	// is itself.
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 0, Root: indexToHash(1)}
	h, err := f.store.head(t.Context())
	require.NoError(t, err)
	assert.Equal(t, indexToHash(1), h)
}

func TestStore_Head_BestDescendant(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(t.Context(), 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(t.Context(), 2, indexToHash(2), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(t.Context(), 3, indexToHash(3), indexToHash(1), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(t.Context(), 4, indexToHash(4), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 0, Root: indexToHash(1)}
	h, err := f.store.head(t.Context())
	require.NoError(t, err)
	require.Equal(t, h, indexToHash(4))
}

func TestStore_Insert(t *testing.T) {
	// The new node does not have a parent.
	treeRootNode := &Node{slot: 0, root: indexToHash(0)}
	emptyRootPN := &PayloadNode{node: treeRootNode}
	fullRootPN := &PayloadNode{node: treeRootNode, full: true, optimistic: true}
	emptyNodeByRoot := map[[32]byte]*PayloadNode{indexToHash(0): emptyRootPN}
	fullNodeByRoot := map[[32]byte]*PayloadNode{indexToHash(0): fullRootPN}
	jc := &forkchoicetypes.Checkpoint{Epoch: 0}
	fc := &forkchoicetypes.Checkpoint{Epoch: 0}
	s := &Store{emptyNodeByRoot: emptyNodeByRoot, fullNodeByRoot: fullNodeByRoot, treeRootNode: treeRootNode, justifiedCheckpoint: jc, finalizedCheckpoint: fc, highestReceivedNode: &Node{}}
	payloadHash := [32]byte{'a'}
	ctx := t.Context()
	_, blk, err := prepareForkchoiceState(ctx, 100, indexToHash(100), indexToHash(0), payloadHash, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, len(s.emptyNodeByRoot), "Did not insert block")
	assert.Equal(t, (*PayloadNode)(nil), treeRootNode.parent, "Incorrect parent")
	children := s.allConsensusChildren(treeRootNode)
	assert.Equal(t, 1, len(children), "Incorrect children number")
	assert.Equal(t, payloadHash, children[0].blockHash, "Incorrect payload hash")
	child := children[0]
	assert.Equal(t, primitives.Round(1), child.justifiedEpoch, "Incorrect justification")
	assert.Equal(t, primitives.Round(1), child.finalizedEpoch, "Incorrect finalization")
	assert.Equal(t, indexToHash(100), child.root, "Incorrect root")
}

func TestStore_Prune_MoreThanThreshold(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}

	s := f.store

	// Finalized round 3 puts the pruning horizon at slot 32 -- two epochs before
	// the epoch holding round 3's first slot -- so slots 0-31 go and 32-99 stay.
	// The horizon trails the finalized node on purpose: epoch-keyed lookups such
	// as dependentRootForEpoch still have to reach the previous epoch boundary.
	s.finalizedCheckpoint.Root = indexToHash(99)
	s.finalizedCheckpoint.Epoch = 3
	require.NoError(t, s.prune(t.Context()))
	assert.Equal(t, 68, len(s.emptyNodeByRoot), "Incorrect nodes count")
	assert.Equal(t, primitives.Slot(32), s.treeRootNode.slot, "Incorrect tree root")
}

func TestStore_Prune_MoreThanOnce(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}

	s := f.store

	// Finalized round 3 puts the horizon at slot 32, so slots 32-99 survive.
	s.finalizedCheckpoint.Root = indexToHash(99)
	s.finalizedCheckpoint.Epoch = 3
	require.NoError(t, s.prune(t.Context()))
	assert.Equal(t, 68, len(s.emptyNodeByRoot), "Incorrect nodes count")

	// One more time: the horizon has not moved, so neither has the tree.
	require.NoError(t, s.prune(t.Context()))
	assert.Equal(t, 68, len(s.emptyNodeByRoot), "Incorrect nodes count")
}

func TestStore_Prune_ReturnEarly(t *testing.T) {
	// Define 100 nodes in store.
	numOfNodes := uint64(100)
	f := setup(0, 0)
	ctx := t.Context()
	state, blkRoot, err := prepareForkchoiceState(ctx, 1, indexToHash(1), params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	for i := uint64(2); i < numOfNodes; i++ {
		state, blkRoot, err = prepareForkchoiceState(ctx, primitives.Slot(i), indexToHash(i), indexToHash(i-1), params.BeaconConfig().ZeroHash, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}
	require.NoError(t, f.store.prune(ctx))
	nodeCount := f.NodeCount()
	require.NoError(t, f.store.prune(ctx))
	require.Equal(t, nodeCount, f.NodeCount())
}

// forkAtGenesisThenChain builds two children of the genesis node -- a dead branch
// at slot 2 and a chain at slots 1,3,4,...,last -- and returns the fork choice.
// The chain is long enough that a finalized round can put the pruning horizon
// above the dead branch, which is where pruning becomes observable now that the
// horizon trails the finalized round by two epochs.
func forkAtGenesisThenChain(t *testing.T, last uint64) *ForkChoice {
	t.Helper()
	f := setup(0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	insert := func(slot primitives.Slot, root, parent [32]byte) {
		state, blkRoot, err := prepareForkchoiceState(ctx, slot, root, parent, zero, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	}
	insert(1, indexToHash(1), zero)
	// The dead branch: a second child of genesis, on no one's finalized chain.
	insert(2, indexToHash(2), zero)
	for i := uint64(3); i <= last; i++ {
		insert(primitives.Slot(i), indexToHash(i), indexToHash(i-1))
	}
	return f
}

// This unit test starts with a simple branch like this
//
//   - 1 -- 3 -- 4 -- ... -- 99
//     /
//
// -- 0 -- 2
//
// and finalizes 99 at round 3, which puts the horizon at slot 32. Everything
// below the horizon -- genesis and the dangling branch at slot 2 -- goes.
func TestStore_Prune_NoDanglingBranch(t *testing.T) {
	f := forkAtGenesisThenChain(t, 99)

	s := f.store
	s.finalizedCheckpoint.Root = indexToHash(99)
	s.finalizedCheckpoint.Epoch = 3
	require.NoError(t, s.prune(t.Context()))

	require.Equal(t, false, f.HasNode(indexToHash(2)))
	require.Equal(t, false, f.HasNode(params.BeaconConfig().ZeroHash))
	require.Equal(t, primitives.Slot(32), s.treeRootNode.slot)
}

// This test starts with the following branching diagram
// / We start with the following diagram
//
//	              E -- F
//	             /
//	       C -- D
//	      /      \
//	A -- B        G -- H -- I
//	      \        \
//	       J        -- K -- L
func TestStore_tips(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'j'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'c'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'e'}, [32]byte{'d'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'g'}, [32]byte{'d'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'f'}, [32]byte{'e'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'h'}, [32]byte{'g'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'k'}, [32]byte{'g'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'i'}, [32]byte{'h'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'l'}, [32]byte{'k'}, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	expectedMap := map[[32]byte]primitives.Slot{
		{'f'}: 105,
		{'i'}: 106,
		{'l'}: 106,
		{'j'}: 102,
	}
	roots, slots := f.store.tips()
	for i, r := range roots {
		expectedSlot, ok := expectedMap[r]
		require.Equal(t, true, ok)
		require.Equal(t, slots[i], expectedSlot)
	}
}

func TestStore_PruneMapsNodes(t *testing.T) {
	f := forkAtGenesisThenChain(t, 99)

	s := f.store
	s.finalizedCheckpoint.Root = indexToHash(99)
	s.finalizedCheckpoint.Epoch = 3
	require.NoError(t, s.prune(t.Context()))
	// Slots 32-99 remain; genesis, slot 1, the dead branch at slot 2 and slots
	// 3-31 are all below the horizon.
	require.Equal(t, 68, len(s.emptyNodeByRoot))
}

func TestForkChoice_ReceivedBlocksLastEpoch(t *testing.T) {
	f := setup(1, 1)
	s := f.store
	var b [32]byte

	// Make sure it doesn't underflow
	f.SetGenesisTime(time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	ctx := t.Context()
	_, blk, err := prepareForkchoiceState(ctx, 1, [32]byte{'a'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	count, err := f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(1), f.HighestReceivedBlockSlot())

	// 64
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 64, [32]byte{'A'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration((-64*int64(params.BeaconConfig().SecondsPerSlot))-1) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(64), f.HighestReceivedBlockSlot())

	// 64 65
	// Received block last epoch is 2
	_, blk, err = prepareForkchoiceState(ctx, 65, [32]byte{'B'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-66*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
	require.Equal(t, primitives.Slot(65), f.HighestReceivedBlockSlot())

	// 64 65 66
	// Received block last epoch is 3
	_, blk, err = prepareForkchoiceState(ctx, 66, [32]byte{'C'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-66*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(3), count)
	require.Equal(t, primitives.Slot(66), f.HighestReceivedBlockSlot())

	// 64 65 66
	//       98
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 98, [32]byte{'D'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-98*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(98), f.HighestReceivedBlockSlot())

	// 64 65 66
	//       98
	//              132
	// Received block last epoch is 1
	_, blk, err = prepareForkchoiceState(ctx, 132, [32]byte{'E'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(132), f.HighestReceivedBlockSlot())

	// 64 65 66
	//       98
	//              132
	//       99
	// Received block last epoch is still 1. 99 is outside the window
	_, blk, err = prepareForkchoiceState(ctx, 99, [32]byte{'F'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(132), f.HighestReceivedBlockSlot())

	// 64 65 66
	//       98
	//              132
	//       99 100
	// Received block last epoch is still 1. 100 is at the same position as 132
	_, blk, err = prepareForkchoiceState(ctx, 100, [32]byte{'G'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	require.Equal(t, primitives.Slot(132), f.HighestReceivedBlockSlot())

	// 64 65 66
	//       98
	//              132
	//       99 100 101
	// Received block last epoch is 2. 101 is within the window
	_, blk, err = prepareForkchoiceState(ctx, 101, [32]byte{'H'}, b, b, 1, 1)
	require.NoError(t, err)
	_, err = s.insert(ctx, blk, 1, 1)
	require.NoError(t, err)
	f.SetGenesisTime(time.Now().Add(time.Duration(-132*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
	require.Equal(t, primitives.Slot(132), f.HighestReceivedBlockSlot())

	f.SetGenesisTime(time.Now().Add(time.Duration(-134*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	f.SetGenesisTime(time.Now().Add(time.Duration(-165*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))
	count, err = f.ReceivedBlocksLastEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)
}

func TestStore_TargetRootForEpoch(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	// Insert a block in slot 32
	state, blk, err := prepareForkchoiceState(ctx, params.BeaconConfig().SlotsPerEpoch, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk))
	// The FFG target for epoch 1 is the block at slot 31, which here is the
	// genesis block the store was set up with.
	target, err := f.TargetRootForRound(blk.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().ZeroHash, target)
	dependent, err := f.DependentRoot(1)
	require.NoError(t, err)
	require.Equal(t, dependent, [32]byte{})

	// Insert a block in slot 33
	state, blk1, err := prepareForkchoiceState(ctx, params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'b'}, blk.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk1))
	headRoot, err := f.Head(ctx) // To cache the head root
	require.NoError(t, err)
	require.Equal(t, headRoot, blk1.Root())
	target, err = f.TargetRootForRound(blk1.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().ZeroHash, target)
	dependent, err = f.DependentRoot(1)
	require.NoError(t, err)
	require.Equal(t, dependent, [32]byte{})

	// Insert a block for the next epoch (missed slot 0), slot 65

	state, blk2, err := prepareForkchoiceState(ctx, 2*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'c'}, blk1.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk2))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk2.Root())
	target, err = f.TargetRootForRound(blk2.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())
	dependent, err = f.DependentRoot(1)
	require.NoError(t, err)
	require.Equal(t, dependent, [32]byte{})
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk2.Root())
	dependent, err = f.DependentRoot(2)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())

	// Insert a block at slot 66
	state, blk3, err := prepareForkchoiceState(ctx, 2*params.BeaconConfig().SlotsPerEpoch+2, [32]byte{'d'}, blk2.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk3))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk3.Root())
	target, err = f.TargetRootForRound(blk2.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())
	dependent, err = f.DependentRoot(2)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())

	// Prune finalization
	s := f.store
	s.finalizedCheckpoint.Root = blk1.Root()
	s.justifiedCheckpoint.Root = blk1.Root()
	require.NoError(t, s.prune(ctx))
	// The pruning horizon trails the finalized round by two epochs, and the
	// finalized round here is 1, so the horizon is still at genesis and nothing
	// is cut: epoch 1's target and dependent root are unchanged. This is the
	// point of the trailing horizon -- an epoch-keyed lookup for the previous
	// epoch must keep resolving right after a prune.
	target, err = f.TargetRootForRound(blk1.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().ZeroHash, target)
	dependent, err = f.DependentRoot(1)
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, dependent)
	dependent, err = f.DependentRoot(2)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), dependent)

	// Insert a block for the next epoch, slot 96 (descends from finalized at slot 33)
	state, blk4, err := prepareForkchoiceState(ctx, 3*params.BeaconConfig().SlotsPerEpoch, [32]byte{'e'}, blk1.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk4))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk4.Root())
	// Slot 95 was missed, so epoch 3's target is the block at slot 33.
	target, err = f.TargetRootForRound(blk4.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), target)
	dependent, err = f.DependentRoot(3)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())
	dependent, err = f.DependentRoot(2)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())

	// Insert a block at slot 97
	state, blk5, err := prepareForkchoiceState(ctx, 3*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'f'}, blk4.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk5))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk5.Root())
	target, err = f.TargetRootForRound(blk5.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), target)
	dependent, err = f.DependentRoot(3)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())
	dependent, err = f.DependentRoot(2)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())

	// Target root where the target epoch is same or ahead of the block slot
	target, err = f.TargetRootForRound(blk5.Root(), 4)
	require.NoError(t, err)
	require.Equal(t, target, blk5.Root())

	// Target root where the target epoch is two epochs ago
	target, err = f.TargetRootForRound(blk5.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), target) // the parent of root4 in epoch 3 is root 1 in epoch 1

	// Target root where the target is two epochs ago, slot 0 was missed
	state, blk6, err := prepareForkchoiceState(ctx, 4*params.BeaconConfig().SlotsPerEpoch+1, [32]byte{'g'}, blk5.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk6))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, headRoot, blk6.Root())
	target, err = f.TargetRootForRound(blk6.Root(), 4)
	require.NoError(t, err)
	require.Equal(t, target, blk5.Root())
	dependent, err = f.DependentRoot(4)
	require.NoError(t, err)
	require.Equal(t, dependent, blk5.Root())
	dependent, err = f.DependentRoot(3)
	require.NoError(t, err)
	require.Equal(t, dependent, blk1.Root())
	dependent, err = f.DependentRoot(1)
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, dependent)
	target, err = f.TargetRootForRound(blk6.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, target, blk1.Root())

	// Prune finalization, finalize the block at slot 96 in round 3. The horizon
	// sits two epochs before round 3's epoch, i.e. slot 32, so the tree is cut at
	// the block at slot 32 and only genesis goes. Everything the epoch-keyed
	// lookups need for epoch 3 is still reachable.
	s.finalizedCheckpoint.Root = blk4.Root()
	s.finalizedCheckpoint.Epoch = 3
	require.NoError(t, s.prune(ctx))
	require.Equal(t, blk.Root(), s.treeRootNode.root)
	require.Equal(t, false, f.HasNode(params.BeaconConfig().ZeroHash))
	// Slot 95 was missed, so epoch 3's target is still the block at slot 33.
	target, err = f.TargetRootForRound(blk4.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), target)
	dependent, err = f.DependentRootForEpoch(blk4.Root(), 3)
	require.NoError(t, err)
	require.Equal(t, blk1.Root(), dependent)
}

func TestStore_DependentRootForEpoch(t *testing.T) {
	ctx := t.Context()
	f := setup(1, 1)

	// Build the following tree structure:
	//                   /------------37
	// 0<--31<---32 <---33 <--- 35 <-------- 65 <--- 66
	//             \-- 36 ------------- 38

	// Insert block at slot 31 (epoch 0)
	state, blk31, err := prepareForkchoiceState(ctx, 31, [32]byte{31}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk31))

	// Insert block at slot 32 (epoch 1)
	state, blk32, err := prepareForkchoiceState(ctx, 32, [32]byte{32}, blk31.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk32))

	// Insert block at slot 33 (epoch 1)
	state, blk33, err := prepareForkchoiceState(ctx, 33, [32]byte{33}, blk32.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk33))

	// Insert block at slot 35 (epoch 1)
	state, blk35, err := prepareForkchoiceState(ctx, 35, [32]byte{35}, blk33.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk35))

	// Insert fork: block at slot 36 (epoch 1) descending from block 32
	state, blk36, err := prepareForkchoiceState(ctx, 36, [32]byte{36}, blk32.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk36))

	// Insert block at slot 37 (epoch 1) descending from block 33
	state, blk37, err := prepareForkchoiceState(ctx, 37, [32]byte{37}, blk33.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk37))

	// Insert block at slot 38 (epoch 1) descending from block 36
	state, blk38, err := prepareForkchoiceState(ctx, 38, [32]byte{38}, blk36.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk38))

	// Insert block at slot 65 (epoch 2) descending from block 35
	state, blk65, err := prepareForkchoiceState(ctx, 65, [32]byte{65}, blk35.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk65))

	// Insert block at slot 66 (epoch 2) descending from block 65
	state, blk66, err := prepareForkchoiceState(ctx, 66, [32]byte{66}, blk65.Root(), params.BeaconConfig().ZeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blk66))

	// Test dependent root for block 32 at epoch 1 - should be block 31
	dependent, err := f.DependentRootForEpoch(blk32.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, blk31.Root(), dependent)

	// Test dependent root for block 32 at epoch 2 - should be block 32
	dependent, err = f.DependentRootForEpoch(blk32.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, blk32.Root(), dependent)

	// Test dependent root for block 33 at epoch 1 - should be block 31
	dependent, err = f.DependentRootForEpoch(blk33.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, blk31.Root(), dependent)

	// Test dependent root for block 38 at epoch 1 - should be block 31
	dependent, err = f.DependentRootForEpoch(blk38.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, blk31.Root(), dependent)

	// Test dependent root for block 36 at epoch 2 - should be block 36
	dependent, err = f.DependentRootForEpoch(blk36.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, blk36.Root(), dependent)

	// Test dependent root for block 66 at epoch 1 - should be block 31
	dependent, err = f.DependentRootForEpoch(blk66.Root(), 1)
	require.NoError(t, err)
	require.Equal(t, blk31.Root(), dependent)

	// Test dependent root for block 66 at epoch 2 - should be block 35
	dependent, err = f.DependentRootForEpoch(blk66.Root(), 2)
	require.NoError(t, err)
	require.Equal(t, blk35.Root(), dependent)
}

func TestStore_CleanupInserting(t *testing.T) {
	f := setup(0, 0)
	ctx := t.Context()
	st, blk, err := prepareForkchoiceState(ctx, 1, indexToHash(1), indexToHash(2), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, f.InsertNode(ctx, st, blk))
	require.Equal(t, false, f.HasNode(blk.Root()))
}

// TestStore_FFGTargetShift pins the decoupled fork's FFG target rule: the
// target for epoch E is the block at slot StartSlot(E)-1, and epoch 0 falls
// back to the anchor. The state side computes the same root in
// helpers.FFGTargetRoot; if the two disagree, VerifyLmdFfgConsistency rejects
// every vote.
func TestStore_FFGTargetShift(t *testing.T) {
	ctx := t.Context()
	f := setup(0, 0)
	genesis := params.BeaconConfig().ZeroHash
	spe := params.BeaconConfig().SlotsPerEpoch

	insert := func(slot primitives.Slot, root, parent [32]byte) {
		t.Helper()
		st, blk, err := prepareForkchoiceState(ctx, slot, root, parent, genesis, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))
	}
	target := func(root [32]byte, epoch primitives.Round) [32]byte {
		t.Helper()
		tr, err := f.TargetRootForRound(root, epoch)
		require.NoError(t, err)
		return tr
	}

	// One block in epoch 0, then the last slot of epoch 0 and the first two
	// slots of epoch 1.
	first, last0, first1, second1 := [32]byte{1}, [32]byte{2}, [32]byte{3}, [32]byte{4}
	insert(1, first, genesis)
	insert(spe-1, last0, first)
	insert(spe, first1, last0)
	insert(spe+1, second1, first1)

	// Epoch 0 underflows: every block of epoch 0 targets the anchor.
	require.Equal(t, genesis, target(first, 0))
	require.Equal(t, genesis, target(last0, 0))
	// Epoch 1 targets the block at the last slot of epoch 0, not the block at
	// the first slot of epoch 1.
	require.Equal(t, last0, target(first1, 1))
	require.Equal(t, last0, target(second1, 1))

	// With slot 63 missed, epoch 2's target is the newest block before it.
	skipped, afterSkip := [32]byte{5}, [32]byte{6}
	insert(2*spe-2, skipped, second1)
	insert(2*spe, afterSkip, skipped)
	require.Equal(t, skipped, target(afterSkip, 2))
	// And the older epoch is still reachable from the newer block.
	require.Equal(t, last0, target(afterSkip, 1))
}

// TestStore_PruneKeepsTheEpochStartChild checks the pruning rule that follows
// from the shifted FFG target: the finalized checkpoint block sits before the
// epoch's first slot, so its child at that first slot is on the finalized
// chain and only earlier children compete with it.
func TestStore_PruneKeepsTheEpochStartChild(t *testing.T) {
	ctx := t.Context()
	f := setup(0, 0)
	genesis := params.BeaconConfig().ZeroHash
	spe := params.BeaconConfig().SlotsPerEpoch

	insert := func(slot primitives.Slot, root, parent [32]byte) {
		t.Helper()
		st, blk, err := prepareForkchoiceState(ctx, slot, root, parent, genesis, 0, 0)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, st, blk))
	}
	checkpoint, competing, canonical := [32]byte{1}, [32]byte{2}, [32]byte{3}
	insert(spe-2, checkpoint, genesis)
	insert(spe-1, competing, checkpoint)
	insert(spe, canonical, checkpoint)

	s := f.store
	s.finalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: checkpoint}
	s.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: checkpoint}
	require.NoError(t, s.prune(ctx))

	require.Equal(t, true, f.HasNode(canonical))
	require.Equal(t, false, f.HasNode(competing))
	target, err := f.TargetRootForRound(canonical, 1)
	require.NoError(t, err)
	require.Equal(t, checkpoint, target)
}

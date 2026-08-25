package doublylinkedtree

import (
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// counterValue reads a package level prometheus counter, which is process wide
// and therefore only usable as a before/after delta.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, c.Write(m))
	return m.GetCounter().GetValue()
}

func goldfishRetreats(t *testing.T) float64 {
	return counterValue(t, goldfishGateRetreatCount)
}

func goldfishGateStops(t *testing.T) float64 {
	return counterValue(t, goldfishGateStopCount)
}

func TestGoldfishVotes_InsertAndOverwrite(t *testing.T) {
	g := newGoldfishVotes()
	rootA := [32]byte{'a'}
	g.insert(3, 7, goldfishVote{root: rootA, seats: 2})
	require.Equal(t, 1, len(g.votes[3]))
	require.Equal(t, rootA, g.votes[3][7].root)
	require.Equal(t, uint64(2), g.votes[3][7].seats)

	// The same content again is idempotent and is not an equivocation.
	g.insert(3, 7, goldfishVote{root: rootA, seats: 2})
	require.Equal(t, 1, len(g.votes[3]))
	require.Equal(t, 0, len(g.equivocators[3]))
	require.Equal(t, uint64(2), g.seats(3))
}

func TestGoldfishVotes_Equivocation(t *testing.T) {
	g := newGoldfishVotes()
	rootA := [32]byte{'a'}
	rootB := [32]byte{'b'}
	g.insert(3, 7, goldfishVote{root: rootA, seats: 2})
	g.insert(3, 7, goldfishVote{root: rootB, seats: 2})
	require.Equal(t, true, g.equivocators[3][7])
	// The first vote stays so the equivocator keeps counting in the denominator.
	require.Equal(t, rootA, g.votes[3][7].root)
	require.Equal(t, uint64(2), g.seats(3))

	// A third vote cannot rehabilitate or change anything.
	g.insert(3, 7, goldfishVote{root: rootB, seats: 2})
	require.Equal(t, rootA, g.votes[3][7].root)
	require.Equal(t, true, g.equivocators[3][7])
}

func TestGoldfishVotes_EquivocationOnPayloadBitOnly(t *testing.T) {
	g := newGoldfishVotes()
	rootA := [32]byte{'a'}
	g.insert(3, 7, goldfishVote{root: rootA, seats: 1, payloadPresent: false})
	g.insert(3, 7, goldfishVote{root: rootA, seats: 1, payloadPresent: true})
	require.Equal(t, true, g.equivocators[3][7])
}

func TestGoldfishVotes_Prune(t *testing.T) {
	g := newGoldfishVotes()
	for slot := primitives.Slot(1); slot <= 10; slot++ {
		g.insert(slot, 1, goldfishVote{root: [32]byte{byte(slot)}, seats: 1})
	}
	g.insert(1, 1, goldfishVote{root: [32]byte{'z'}, seats: 1}) // equivocate at slot 1
	require.Equal(t, true, g.equivocators[1][1])

	g.prune(10)
	// Retention is 3 slots: 7, 8, 9 and 10 survive.
	require.Equal(t, 4, len(g.votes))
	for slot := primitives.Slot(7); slot <= 10; slot++ {
		require.Equal(t, 1, len(g.votes[slot]))
	}
	require.Equal(t, 0, len(g.equivocators))
}

func TestGoldfishVotes_SeatMultiplicity(t *testing.T) {
	g := newGoldfishVotes()
	g.insert(4, 1, goldfishVote{root: [32]byte{'a'}, seats: 3})
	g.insert(4, 2, goldfishVote{root: [32]byte{'a'}, seats: 1})
	require.Equal(t, uint64(4), g.seats(4))
}

func TestInsertAvailableAttestation_ZeroSeatsIgnored(t *testing.T) {
	f := setup(0, 0)
	f.InsertAvailableAttestation(1, 1, 0, [32]byte{'a'}, false)
	require.Equal(t, 0, len(f.store.goldfishVotes.votes))
	f.InsertAvailableAttestation(1, 1, 2, [32]byte{'a'}, false)
	require.Equal(t, uint64(2), f.store.goldfishVotes.seats(1))
}

func TestGoldfishActive_GatedOnHeze(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	f := setup(0, 0)
	driftGenesisTime(f, 4, 0)
	require.Equal(t, false, f.store.goldfishActive())

	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	require.Equal(t, true, f.store.goldfishActive())
}

// setupGoldfish returns a forkchoice store on a config where Heze (and so the
// Goldfish head walk) is active from genesis, with four rounds to the epoch.
func setupGoldfish(t *testing.T, justified, finalized primitives.Epoch) *ForkChoice {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)
	f := setup(justified, finalized)
	require.NotNil(t, f)
	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	f.store.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	return f
}

// blockHashFor is the execution block hash the test tree gives to a block root.
func blockHashFor(root [32]byte) [32]byte {
	if root == params.BeaconConfig().ZeroHash {
		return params.BeaconConfig().ZeroHash
	}
	var h [32]byte
	copy(h[:], root[:])
	h[31] ^= 0xff
	return h
}

// insertGoldfishBlock adds a block at the given slot. onFull selects whether it
// builds on its parent's full or empty payload node.
func insertGoldfishBlock(
	t *testing.T, f *ForkChoice, slot primitives.Slot, root, parentRoot [32]byte, onFull bool,
) {
	t.Helper()
	parentBlockHash := [32]byte{'n', 'o', 'p', 'e'}
	if onFull {
		parentBlockHash = blockHashFor(parentRoot)
	}
	st, blk, err := prepareGloasForkchoiceState(
		t.Context(), slot, root, parentRoot, blockHashFor(root), parentBlockHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(t.Context(), st, blk))
}

func TestGoldfishWalk_GatePassesAndBlocks(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB, rootC := indexToHash(1), indexToHash(2), indexToHash(3)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	insertGoldfishBlock(t, f, 2, rootC, rootA, false)

	// Four seats split evenly between the two slot-2 children: neither clears
	// the majority gate, so the walk stops at their parent.
	f.InsertAvailableAttestation(2, 1, 1, rootB, false)
	f.InsertAvailableAttestation(2, 2, 1, rootB, false)
	f.InsertAvailableAttestation(2, 3, 1, rootC, false)
	f.InsertAvailableAttestation(2, 4, 1, rootC, false)
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, head)

	// One more seat for B and it clears the gate.
	f.InsertAvailableAttestation(2, 5, 1, rootB, false)
	head, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, head)
}

func TestGoldfishWalk_SeatMultiplicityDecides(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB, rootC := indexToHash(1), indexToHash(2), indexToHash(3)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	insertGoldfishBlock(t, f, 2, rootC, rootA, false)

	// Two validators vote B with one seat each, one validator votes C with
	// three seats. Seats decide, not head count.
	f.InsertAvailableAttestation(2, 1, 1, rootB, false)
	f.InsertAvailableAttestation(2, 2, 1, rootB, false)
	f.InsertAvailableAttestation(2, 3, 3, rootC, false)
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, head)
}

func TestGoldfishWalk_CurrentSlotPassthrough(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	// Slot 7 is not a round start with SLOTS_PER_ROUND = 8.
	driftGenesisTime(f, 7, 0)
	insertGoldfishBlock(t, f, 6, rootA, zero, true)
	insertGoldfishBlock(t, f, 7, rootB, rootA, false)
	f.InsertAvailableAttestation(6, 1, 4, rootA, false)

	// B has a Goldfish score of zero by construction; the current-slot
	// passthrough is what admits it.
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, head)
}

func TestGoldfishWalk_NoPassthroughAtRoundStart(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	driftGenesisTime(f, 8, 0)
	insertGoldfishBlock(t, f, 7, rootA, zero, true)
	insertGoldfishBlock(t, f, 8, rootB, rootA, false)
	f.InsertAvailableAttestation(7, 1, 4, rootA, false)

	// Slot 8 starts a round, so the proposal has to earn its votes.
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootA, head)
}

func TestGoldfishWalk_EquivocatorScoresEveryChild(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB, rootC := indexToHash(1), indexToHash(2), indexToHash(3)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	insertGoldfishBlock(t, f, 2, rootC, rootA, false)

	f.InsertAvailableAttestation(2, 1, 1, rootB, false)
	f.InsertAvailableAttestation(2, 2, 1, rootB, false)
	f.InsertAvailableAttestation(2, 3, 1, rootC, false)
	// Validator 4 equivocates: it stays in the denominator and, per the spec's
	// get_available_attestation_score, its seat is added to every child.
	f.InsertAvailableAttestation(2, 4, 1, rootB, false)
	f.InsertAvailableAttestation(2, 4, 1, rootC, false)

	justified := f.store.treeRootNode
	sc := f.store.goldfishScoresForSlot(2, justified)
	require.Equal(t, uint64(2), sc.threshold) // 4 voting seats, equivocator included
	require.Equal(t, uint64(1), sc.equivocatorSeats)
	require.Equal(t, uint64(3), sc.nodeScore(f.store.emptyNodeByRoot[rootB].node))
	require.Equal(t, uint64(2), sc.nodeScore(f.store.emptyNodeByRoot[rootC].node))

	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, head)
}

func TestGoldfishWalk_PayloadStatusDerivation(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA := indexToHash(1)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	en := f.store.emptyNodeByRoot[rootA]
	fn := f.store.fullNodeByRoot[rootA]
	justified := f.store.treeRootNode

	// A vote from a later slot with payload_present set supports (A, FULL).
	f.InsertAvailableAttestation(2, 1, 1, rootA, true)
	sc := f.store.goldfishScoresForSlot(2, justified)
	require.Equal(t, uint64(1), sc.payloadScore(fn))
	require.Equal(t, uint64(0), sc.payloadScore(en))
	require.Equal(t, uint64(1), sc.nodeScore(en.node))

	// A vote from a later slot without the bit supports (A, EMPTY).
	f.InsertAvailableAttestation(3, 2, 1, rootA, false)
	sc = f.store.goldfishScoresForSlot(3, justified)
	require.Equal(t, uint64(0), sc.payloadScore(fn))
	require.Equal(t, uint64(1), sc.payloadScore(en))

	// A vote cast in the block's own slot decides nothing: it supports
	// (A, PENDING) only.
	f.InsertAvailableAttestation(1, 3, 1, rootA, false)
	sc = f.store.goldfishScoresForSlot(1, justified)
	require.Equal(t, uint64(0), sc.payloadScore(fn))
	require.Equal(t, uint64(0), sc.payloadScore(en))
	require.Equal(t, uint64(1), sc.nodeScore(en.node))
}

func TestGoldfishWalk_PayloadChoiceRoutesTheDescent(t *testing.T) {
	zero := params.BeaconConfig().ZeroHash
	rootA, rootC, rootD := indexToHash(1), indexToHash(2), indexToHash(3)

	for _, tt := range []struct {
		name  string
		voted [32]byte
	}{
		{name: "child on full", voted: rootC},
		{name: "child on empty", voted: rootD},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := setupGoldfish(t, 0, 0)
			driftGenesisTime(f, 3, 0)
			insertGoldfishBlock(t, f, 1, rootA, zero, true)
			pe, err := prepareGloasForkchoicePayload(rootA)
			require.NoError(t, err)
			require.NoError(t, f.InsertPayload(pe))
			insertGoldfishBlock(t, f, 2, rootC, rootA, true)
			insertGoldfishBlock(t, f, 2, rootD, rootA, false)

			f.InsertAvailableAttestation(2, 1, 3, tt.voted, false)
			head, err := f.Head(t.Context())
			require.NoError(t, err)
			require.Equal(t, tt.voted, head)
		})
	}
}

func TestGoldfishWalk_HeadRetreatsWhenLateBlockLosesPassthrough(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	ctx := t.Context()
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB, rootC := indexToHash(1), indexToHash(2), indexToHash(3)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	f.InsertAvailableAttestation(2, 1, 4, rootB, false)
	insertGoldfishBlock(t, f, 3, rootC, rootB, false)

	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootC, head)

	// Slot 3's voters did not see C, they named B. At slot 4 C has lost the
	// passthrough and never earned a vote, so the head retreats to B.
	before := goldfishRetreats(t)
	driftGenesisTime(f, 4, 0)
	f.InsertAvailableAttestation(3, 1, 4, rootB, false)
	head, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, rootB, head)
	require.Equal(t, before+1, goldfishRetreats(t))
}

func TestGoldfishWalk_ColdStarts(t *testing.T) {
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	t.Run("fresh start", func(t *testing.T) {
		f := setupGoldfish(t, 0, 0)
		driftGenesisTime(f, 1, 0)
		insertGoldfishBlock(t, f, 1, rootA, zero, true)
		// No votes exist yet; the passthrough still admits the current block.
		head, err := f.Head(t.Context())
		require.NoError(t, err)
		require.Equal(t, rootA, head)
	})

	t.Run("restart with a populated tree and no votes", func(t *testing.T) {
		f := setupGoldfish(t, 0, 0)
		driftGenesisTime(f, 5, 0)
		insertGoldfishBlock(t, f, 1, rootA, zero, true)
		insertGoldfishBlock(t, f, 2, rootB, rootA, false)
		head, err := f.Head(t.Context())
		require.NoError(t, err)
		require.Equal(t, zero, head) // the justified root, one slot of retreat
	})

	t.Run("checkpoint sync with a justified root past genesis", func(t *testing.T) {
		f := setupGoldfish(t, 1, 0)
		driftGenesisTime(f, 34, 0)
		insertGoldfishBlock(t, f, 32, rootA, zero, true)
		f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: rootA}
		insertGoldfishBlock(t, f, 33, rootB, rootA, false)

		// No votes: the walk stops at the justified root rather than panicking
		// or returning a zero head.
		head, err := f.Head(t.Context())
		require.NoError(t, err)
		require.Equal(t, rootA, head)

		// One slot later the store is populated and the head recovers.
		f.InsertAvailableAttestation(33, 1, 4, rootB, false)
		head, err = f.Head(t.Context())
		require.NoError(t, err)
		require.Equal(t, rootB, head)
	})
}

func TestGoldfishWalk_GateStopCounted(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	driftGenesisTime(f, 4, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	before := goldfishGateStops(t)
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, zero, head)
	require.Equal(t, true, goldfishGateStops(t) > before)
}

func TestGoldfishWalk_IsCanonicalAndCanonicalNodeAtSlot(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB, rootC := indexToHash(1), indexToHash(2), indexToHash(3)

	driftGenesisTime(f, 3, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	insertGoldfishBlock(t, f, 2, rootC, rootA, false)
	f.InsertAvailableAttestation(2, 1, 4, rootB, false)

	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootB, head)

	require.Equal(t, true, f.IsCanonical(rootB))
	require.Equal(t, true, f.IsCanonical(rootA))
	require.Equal(t, true, f.IsCanonical(zero))
	require.Equal(t, false, f.IsCanonical(rootC))
	require.Equal(t, false, f.IsCanonical(indexToHash(99)))

	// CanonicalNodeAtSlot walks parents from the head the walk chose.
	r, full := f.CanonicalNodeAtSlot(1)
	require.Equal(t, rootA, r)
	require.Equal(t, false, full)
	r, _ = f.CanonicalNodeAtSlot(2)
	require.Equal(t, rootB, r)
}

func TestGoldfishWalk_FullHeadResolvesPreviousSlotPayload(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA := indexToHash(1)

	driftGenesisTime(f, 2, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	f.InsertAvailableAttestation(1, 1, 4, rootA, false)

	// The walk picks the head; the empty/full choice for the previous slot's
	// block still resolves through choosePayloadContent, so a block builder at
	// slot 2 gets a payload hash to extend.
	head, headHash, full, err := f.FullHead(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootA, head)
	require.Equal(t, true, full)
	require.Equal(t, blockHashFor(rootA), headHash)

	r, full := f.CanonicalNodeAtSlot(1)
	require.Equal(t, rootA, r)
	require.Equal(t, true, full)
}

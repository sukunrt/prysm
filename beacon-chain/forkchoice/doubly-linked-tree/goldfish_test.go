package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// counterValue reads a package level prometheus counter, which is process wide
// and therefore only usable as a before/after delta.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, c.Write(m))
	return m.GetCounter().GetValue()
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
	driftGenesisTime(f, 1, 0)
	f.InsertAvailableAttestation(1, 1, 0, [32]byte{'a'}, false)
	require.Equal(t, 0, len(f.store.goldfishVotes.votes))
	f.InsertAvailableAttestation(1, 1, 2, [32]byte{'a'}, false)
	require.Equal(t, uint64(2), f.store.goldfishVotes.seats(1))
}

// A vote replayed from the sync pending queue after its own slot ended must
// show up in the late vote accounting rather than vanishing.
func TestInsertAvailableAttestation_CountsLateVote(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	f := setup(0, 0)
	driftGenesisTime(f, 4, 0)

	before := counterValue(t, goldfishLateVoteCount)
	f.InsertAvailableAttestation(3, 1, 1, [32]byte{'a'}, false)
	require.Equal(t, before+1, counterValue(t, goldfishLateVoteCount))

	// A vote naming the current slot is on time and must not be counted.
	f.InsertAvailableAttestation(4, 2, 1, [32]byte{'a'}, false)
	require.Equal(t, before+1, counterValue(t, goldfishLateVoteCount))
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
func setupGoldfish(t *testing.T, justified, finalized primitives.Round) *ForkChoice {
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
	insertGoldfishBlockAtRound(t, f, slot, root, parentRoot, onFull, 0)
}

// insertGoldfishBlockAtRound inserts a block whose node carries the given justified
// ROUND, as a checkpoint-synced node's forkchoice does for the blocks it imports.
func insertGoldfishBlockAtRound(
	t *testing.T, f *ForkChoice, slot primitives.Slot, root, parentRoot [32]byte, onFull bool,
	justifiedRound primitives.Round,
) {
	t.Helper()
	parentBlockHash := [32]byte{'n', 'o', 'p', 'e'}
	if onFull {
		parentBlockHash = blockHashFor(parentRoot)
	}
	st, blk, err := prepareGloasForkchoiceState(
		t.Context(), slot, root, parentRoot, blockHashFor(root), parentBlockHash, justifiedRound, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(t.Context(), st, blk))
}

func TestHezeHead_HighestEligibleWithoutVoteAuthority(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	a, b, c, d := indexToHash(1), indexToHash(2), indexToHash(3), indexToHash(4)
	driftGenesisTime(f, 131, 0)
	insertGoldfishBlock(t, f, 128, a, zero, true)
	insertGoldfishBlock(t, f, 130, b, a, false)
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, b, head)
	// A vote for the old fork cannot displace a newer imported block.
	f.InsertAvailableAttestation(130, 1, 512, a, false)
	insertGoldfishBlock(t, f, 131, c, b, false)
	head, err = f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, c, head)
	insertGoldfishBlock(t, f, 129, d, a, false)
	head, err = f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, c, head)
}

func TestHezeHead_TieAndJustifiedSubtree(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	a, b, c, d := indexToHash(1), indexToHash(2), indexToHash(3), indexToHash(4)
	driftGenesisTime(f, 129, 0)
	insertGoldfishBlock(t, f, 117, a, zero, false)
	insertGoldfishBlock(t, f, 128, b, a, false)
	insertGoldfishBlock(t, f, 120, c, zero, true)
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 15, Root: a}
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, b, head)
	for i := primitives.ValidatorIndex(0); i < 392; i++ {
		f.InsertAvailableAttestation(128, i, 1, c, false)
	}
	head, err = f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, b, head)
	insertGoldfishBlock(t, f, 128, d, a, false)
	head, err = f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, b, head)
}

func TestHezeHead_SkippedCheckpointTargetKeepsCompatibleBranch(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	a, b, c, d := indexToHash(117), indexToHash(118), indexToHash(129), indexToHash(128)
	driftGenesisTime(f, 129, 0)
	insertGoldfishBlock(t, f, 117, a, zero, false)
	insertGoldfishBlock(t, f, 118, b, a, false)
	insertGoldfishBlock(t, f, 129, c, b, false)
	insertGoldfishBlock(t, f, 128, d, a, false)
	f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 15, Root: a}
	f.store.headNode = f.store.emptyNodeByRoot[c].node // A cached head from before the checkpoint update is stale.
	for index := primitives.ValidatorIndex(0); index < 392; index++ {
		f.InsertAvailableAttestation(128, index, 1, c, false)
	}
	for index := primitives.ValidatorIndex(392); index < 512; index++ {
		f.InsertAvailableAttestation(128, index, 1, d, false)
	}
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, d, head)
}

func TestGoldfish_ProposerBoostNotApplied(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA := indexToHash(1)

	driftGenesisTime(f, 1, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootA, head)

	// The block was timely, so the boost root is recorded - the walk's payload
	// tiebreaker reads it - but no boost weight lands on the node.
	require.Equal(t, rootA, f.store.proposerBoostRoot)
	require.Equal(t, uint64(0), f.store.emptyNodeByRoot[rootA].node.balance)
	require.Equal(t, uint64(0), f.store.emptyNodeByRoot[rootA].node.weight)
}

func TestGoldfish_LateBlockReorgOff(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	// A block that arrives late in its slot on a weak head: pre-Heze this is
	// exactly the shape ShouldOverrideFCU and GetProposerHead act on.
	driftGenesisTime(f, 2, 11*time.Second)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	f.InsertAvailableAttestation(1, 1, 4, rootA, false)
	insertGoldfishBlock(t, f, 2, rootB, rootA, false)
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootB, head)

	require.Equal(t, false, f.ShouldOverrideFCU())
	require.Equal(t, rootB, f.GetProposerHead())
}

func TestGoldfish_PreviousSlotPayloadDecisionFollowsTheNewBlock(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA, rootC := indexToHash(1), indexToHash(2)

	driftGenesisTime(f, 2, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	f.InsertAvailableAttestation(1, 1, 4, rootA, false)

	// The slot-2 block built on A's empty payload node. Both of A's payload
	// nodes pass through as previous-slot decisions, so the tiebreaker decides,
	// and it must follow the new block or the walk would stop at A.
	insertGoldfishBlock(t, f, 2, rootC, rootA, false)
	head, err := f.Head(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootC, head)
}

func TestGoldfish_ProposerBuildsOnWalkHead(t *testing.T) {
	f := setupGoldfish(t, 0, 0)
	zero := params.BeaconConfig().ZeroHash
	rootA, rootB := indexToHash(1), indexToHash(2)

	driftGenesisTime(f, 2, 0)
	insertGoldfishBlock(t, f, 1, rootA, zero, true)
	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))
	f.InsertAvailableAttestation(1, 1, 4, rootA, false)

	head, headHash, full, err := f.FullHead(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootA, head)
	// The three values a proposer reads - GetProposerHead for the parent root,
	// CachedHeadRoot for the head it processed slots from, and FullBeatsEmpty
	// for the parentFull flag that picks the block hash - all agree with the
	// walk head.
	require.Equal(t, head, f.GetProposerHead())
	require.Equal(t, head, f.CachedHeadRoot())
	require.Equal(t, full, f.FullBeatsEmpty(head))
	require.Equal(t, blockHashFor(rootA), headHash)

	// A block whose own payload has not arrived moves all three together.
	insertGoldfishBlock(t, f, 2, rootB, rootA, true)
	head, headHash, full, err = f.FullHead(t.Context())
	require.NoError(t, err)
	require.Equal(t, rootB, head)
	require.Equal(t, false, full)
	require.Equal(t, head, f.GetProposerHead())
	require.Equal(t, head, f.CachedHeadRoot())
	require.Equal(t, full, f.FullBeatsEmpty(head))
	// With no payload for B the head hash is its full ancestor's, that is A's.
	require.Equal(t, blockHashFor(rootA), headHash)
}

func TestGoldfishNewSlot_WritesTheSummaryLine(t *testing.T) {
	hook := logTest.NewGlobal()
	f := setupGoldfish(t, 0, 0)
	ctx := context.Background()
	f.store.goldfishVotes.insert(1, 7, goldfishVote{root: [32]byte{'a'}, seats: 3})
	f.store.goldfishVotes.insert(1, 8, goldfishVote{root: [32]byte{'a'}, seats: 2})

	require.NoError(t, f.NewSlot(ctx, 2))
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, "Goldfish votes", entry.Message)
	require.Equal(t, decoupled.SummaryPurpose, entry.Data["purpose"])
	require.Equal(t, primitives.Slot(1), entry.Data["slot"])
	require.Equal(t, uint64(2), entry.Data["uniqueValidators"])
	require.Equal(t, uint64(2), entry.Data["recordedMessages"])
	require.Equal(t, "next_slot_start", entry.Data["cutoff"])
	require.Equal(t, uint64(5), entry.Data["seats"])
	require.Equal(t, uint64(decoupled.AvailableAttestationCommitteeSize), entry.Data["committeeSeats"])
	require.Equal(t, "", entry.Data["blockRoot"])
	require.Equal(t, uint64(2), entry.Data["otherVoters"])
	require.Equal(t, "0x61000000:2", entry.Data["otherRoots"])

	hook.Reset()
	require.NoError(t, f.NewSlot(ctx, 3))
	entry = hook.LastEntry()
	require.NotNil(t, entry)
	require.Equal(t, "Goldfish votes", entry.Message)
	require.Equal(t, primitives.Slot(2), entry.Data["slot"])
	require.Equal(t, uint64(0), entry.Data["uniqueValidators"])
	require.Equal(t, uint64(0), entry.Data["seats"])
}

func TestGoldfishNewSlot_SummaryCountsTheVoters(t *testing.T) {
	rootA, rootB := [32]byte{'a'}, [32]byte{'b'}
	zero := [32]byte{}
	all := []primitives.ValidatorIndex{0, 1, 2, 3, 4, 5, 6, 7}
	type votes struct {
		root    [32]byte
		indices []primitives.ValidatorIndex
	}
	tests := []struct {
		name        string
		noBlock     bool
		votes       []votes
		blockRoot   string
		blockVoters uint64
		otherVoters uint64
		otherRoots  string
		nonVoters   uint64
	}{
		{
			name:        "all voted for the block",
			votes:       []votes{{rootA, all}},
			blockRoot:   "0x61000000",
			blockVoters: 8,
		},
		{
			name:        "some voted for another root",
			votes:       []votes{{rootA, all[:5]}, {zero, all[5:7]}, {rootB, all[7:]}},
			blockRoot:   "0x61000000",
			blockVoters: 5,
			otherVoters: 3,
			otherRoots:  "0x00000000:2,0x62000000:1",
		},
		{
			name:        "some did not vote",
			votes:       []votes{{rootA, all[:5]}},
			blockRoot:   "0x61000000",
			blockVoters: 5,
			nonVoters:   3,
		},
		{
			name:        "no block at the slot",
			noBlock:     true,
			votes:       []votes{{zero, all}},
			otherVoters: 8,
			otherRoots:  "0x00000000:8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			f := setupGoldfish(t, 0, 0)
			// Eight validators fill all 512 seats, so the committee is 0..7.
			cfg := params.BeaconConfig().Copy()
			cfg.MinGenesisActiveValidatorCount = 8
			params.OverrideBeaconConfig(cfg)
			driftGenesisTime(f, 1, 0)
			if !tt.noBlock {
				insertGoldfishBlock(t, f, 1, rootA, zero, true)
			}
			_, err := f.Head(t.Context())
			require.NoError(t, err)
			for _, v := range tt.votes {
				for _, index := range v.indices {
					f.InsertAvailableAttestation(1, index, 1, v.root, false)
				}
			}

			require.NoError(t, f.NewSlot(t.Context(), 2))
			entry := hook.LastEntry()
			require.NotNil(t, entry)
			require.Equal(t, "Goldfish votes", entry.Message)
			require.Equal(t, tt.blockRoot, entry.Data["blockRoot"])
			require.Equal(t, tt.blockVoters, entry.Data["blockVoters"])
			require.Equal(t, tt.otherVoters, entry.Data["otherVoters"])
			require.Equal(t, tt.otherRoots, entry.Data["otherRoots"])
			require.Equal(t, tt.nonVoters, entry.Data["nonVoters"])
		})
	}
}

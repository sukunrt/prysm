package doublylinkedtree

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

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

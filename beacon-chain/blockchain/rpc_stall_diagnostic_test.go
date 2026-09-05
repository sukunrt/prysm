package blockchain

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestDiagnosticGenesisFullFlipBlocksForkchoiceOnStateFeed demonstrates the lock
// amplification possible when genesis changes payload status at startup. This is
// diagnostic coverage, not a proposed production behavior test.
func TestDiagnosticGenesisFullFlipBlocksForkchoiceOnStateFeed(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	genesisState, _ := util.DeterministicGenesisStateGloas(t, 64)
	service, tr := minimalTestService(t, WithFinalizedStateAtStartUp(genesisState))
	require.NoError(t, service.saveGenesisData(t.Context(), genesisState))

	root := service.originBlockRoot
	headBlock, err := service.HeadBlock(t.Context())
	require.NoError(t, err)
	headState, err := service.HeadState(t.Context())
	require.NoError(t, err)
	_, full := service.HeadRootAndFull()
	require.Equal(t, true, full)

	// An unbuffered subscriber models any state-feed consumer whose buffer is full.
	events := make(chan *feed.Event)
	sub := service.cfg.StateNotifier.StateFeed().Subscribe(events)
	defer sub.Unsubscribe()

	saveStarted := make(chan struct{})
	saveDone := make(chan error, 1)
	go func() {
		service.cfg.ForkChoiceStore.Lock()
		defer service.cfg.ForkChoiceStore.Unlock()
		close(saveStarted)
		saveDone <- service.saveHead(t.Context(), root, headBlock, headState, false)
	}()
	select {
	case <-saveStarted:
	case <-time.After(time.Second):
		t.Fatal("saveHead did not start")
	}

	// Give saveHead time to reach the synchronous Reorg feed send. It cannot
	// finish because this test deliberately has not received from events yet.
	select {
	case err := <-saveDone:
		t.Fatalf("same-root genesis flip unexpectedly completed before feed drain: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	readDone := make(chan struct{})
	go func() {
		service.cfg.ForkChoiceStore.RLock()
		service.cfg.ForkChoiceStore.RUnlock()
		close(readDone)
	}()
	select {
	case <-readDone:
		t.Fatal("forkchoice reader acquired while saveHead was blocked on the state feed")
	case <-time.After(100 * time.Millisecond):
	}

	blockedFor := time.Now()
	var ev *feed.Event
	select {
	case ev = <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reorg event")
	}
	require.Equal(t, feed.EventType(statefeed.Reorg), ev.Type)
	select {
	case err := <-saveDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("saveHead remained blocked after the reorg subscriber drained")
	}
	select {
	case <-readDone:
		t.Logf("forkchoice reader released %s after the reorg subscriber drained", time.Since(blockedFor))
	case <-time.After(time.Second):
		t.Fatal("forkchoice reader remained blocked after the reorg subscriber drained")
	}

	// Keep the startup fixture alive until all accesses above are complete.
	require.NotNil(t, tr)
}

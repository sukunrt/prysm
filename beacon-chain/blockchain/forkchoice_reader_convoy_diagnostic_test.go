package blockchain

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// TestDiagnosticAttestationTargetStateForkchoiceReaderConvoy reproduces the
// lock ordering in AttestationTargetState: forkchoice RLock, then the
// checkpoint-key multilock in getAttPreState. Holding the multilock long enough
// for all readers to arrive makes the write-lock delay deterministic without
// adding an artificial delay to any production callback.
func TestDiagnosticAttestationTargetStateForkchoiceReaderConvoy(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	pb := updateHeadDiagnosticGenesis(t)
	st, err := util.NewBeaconStateHeze(func(dst *ethpb.BeaconStateHeze) error {
		*dst = *pb
		return nil
	})
	require.NoError(t, err)
	service, _ := minimalTestService(t, WithFinalizedStateAtStartUp(st))
	require.NoError(t, service.saveGenesisData(t.Context(), st))
	cp := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp, st))
	key := string(cp.Root) + strconv.FormatUint(uint64(cp.Epoch), 10)

	for _, readers := range []int{50, 500, 2500} {
		t.Run(strconv.Itoa(readers), func(t *testing.T) {
			gate := async.NewMultilock(key)
			gate.Lock()
			var readerWG sync.WaitGroup
			readerWG.Add(readers)
			readLocked := make(chan struct{}, readers)
			for range readers {
				go func() {
					defer readerWG.Done()
					service.cfg.ForkChoiceStore.RLock()
					readLocked <- struct{}{}
					_, lookupErr := service.getAttPreState(t.Context(), cp)
					service.cfg.ForkChoiceStore.RUnlock()
					require.NoError(t, lookupErr)
				}()
			}
			for range readers {
				<-readLocked
			}

			writerAcquired := make(chan struct{}, 1)
			go func() {
				service.cfg.ForkChoiceStore.Lock()
				writerAcquired <- struct{}{}
				service.cfg.ForkChoiceStore.Unlock()
			}()
			// Releasing the checkpoint-key gate lets the established readers drain.
			// Measure through writer acquisition without assuming the writer goroutine
			// was scheduled before the release.
			started := time.Now()
			gate.Unlock()
			select {
			case <-writerAcquired:
				elapsed := time.Since(started)
				t.Logf("readers=%d forkchoice_writer_wait=%s", readers, elapsed)
			case <-time.After(30 * time.Second):
				t.Fatal("forkchoice writer did not acquire lock")
			}
			readerWG.Wait()
		})
	}
}

package blockchain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/metrics"
	runtimeTrace "runtime/trace"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

type syncIndexCountProbe struct {
	Call               int   `json:"call"`
	HandlerDurationUS  int64 `json:"handler_duration_us"`
	ObserverDurationUS int64 `json:"observer_duration_us"`
	JobsAtStart        int64 `json:"jobs_at_start"`
	JobsAtEnd          int64 `json:"jobs_at_end"`
	ActiveScansAtStart int64 `json:"active_scans_at_start"`
	ActiveScansAtEnd   int64 `json:"active_scans_at_end"`
	LoadActiveAtStart  bool  `json:"load_active_at_start"`
	StackAttempted     bool  `json:"stack_attempted"`
	StackCaptured      bool  `json:"stack_captured"`
}

type parentDependencyPhase struct {
	Phase                    string `json:"phase"`
	StartedUTC               string `json:"started_utc"`
	DurationUS               int64  `json:"duration_us"`
	SinceTriggerUS           int64  `json:"since_trigger_us"`
	ContextAtEnter           string `json:"context_at_enter"`
	ContextAtExit            string `json:"context_at_exit"`
	JobsAtEnter              int64  `json:"jobs_at_enter"`
	JobsAtExit               int64  `json:"jobs_at_exit"`
	ActiveCheckpointsAtEnter int64  `json:"active_checkpoints_at_enter"`
	ActiveCheckpointsAtExit  int64  `json:"active_checkpoints_at_exit"`
	ActiveScansAtEnter       int64  `json:"active_scans_at_enter"`
	ActiveScansAtExit        int64  `json:"active_scans_at_exit"`
	Detail                   string `json:"detail"`
}

type parentDependencyResult struct {
	Phases            []parentDependencyPhase
	OldHeadRoot       [32]byte
	HeadRoot          [32]byte
	ParentRoot        [32]byte
	PostHeadRoot      [32]byte
	ParentFull        bool
	NextSlotCacheHit  bool
	HeadStateSlot     primitives.Slot
	PreparedStateSlot primitives.Slot
	Err               error
	ContextAtExit     error
	Elapsed           time.Duration
}

func parentDependencyContextErr(ctx context.Context) string {
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	return ""
}

type syncIndexCountProbeResult struct {
	duration           time.Duration
	jobsAtStart        int64
	jobsAtEnd          int64
	activeScansAtStart int64
	activeScansAtEnd   int64
	err                error
}

func syncIndexDiagnosticInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	require.NoError(t, err)
	if value <= 0 {
		t.Fatalf("%s must be positive", name)
	}
	return value
}

func syncIndexDiagnosticUserCPUSeconds() float64 {
	samples := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return samples[0].Value.Float64()
}

func syncIndexDiagnosticRelevantStacks() string {
	buf := make([]byte, 4<<20)
	n := runtime.Stack(buf, true)
	var syncMatches []string
	var checkpointCleanSample string
	checkpointCleanWaiters := 0
	checkpointLockWaiters := 0
	activeCountStacks := 0
	for _, stack := range bytes.Split(buf[:n], []byte("\n\n")) {
		if bytes.Contains(stack, []byte("HeadSyncCommitteeIndices")) ||
			bytes.Contains(stack, []byte("getSyncCommitteeHeadState")) {
			syncMatches = append(syncMatches, string(stack))
		}
		if bytes.Contains(stack, []byte("AttestationTargetState")) &&
			bytes.Contains(stack, []byte("async.Clean")) {
			checkpointCleanWaiters++
			if checkpointCleanSample == "" {
				checkpointCleanSample = string(stack)
			}
		}
		if bytes.Contains(stack, []byte("AttestationTargetState")) &&
			bytes.Contains(stack, []byte("async.(*Lock).Lock")) {
			checkpointLockWaiters++
		}
		if bytes.Contains(stack, []byte("ActiveValidatorCount")) {
			activeCountStacks++
		}
	}
	return fmt.Sprintf("checkpoint_clean_waiters=%d checkpoint_lock_waiters=%d active_count_stacks=%d sync_stacks=%d\n%s\ncheckpoint_clean_sample:\n%s",
		checkpointCleanWaiters, checkpointLockWaiters, activeCountStacks, len(syncMatches),
		strings.Join(syncMatches, "\n\n"), checkpointCleanSample)
}

func syncIndexDiagnosticPercentile(samples []time.Duration, percentile int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*percentile + 99) / 100
	if index == 0 {
		index = 1
	}
	return sorted[index-1]
}

// TestDiagnosticWarmSyncIndexUnderCheckpointCountLoad compares a real
// genesis-state validator scan with its known-correct memoized result. Both
// arms run the same number of AttestationTargetState calls, which retain the
// production fork-choice RLock -> checkpoint multilock/cache lookup ordering,
// and the same warm, same-slot HeadSyncCommitteeIndices calls.
func TestDiagnosticWarmSyncIndexUnderCheckpointCountLoad(t *testing.T) {
	arm := os.Getenv("PRYSM_DIAGNOSTIC_SYNC_COUNT_ARM")
	if arm == "" {
		t.Skip("PRYSM_DIAGNOSTIC_SYNC_COUNT_ARM is not set")
	}
	if arm != "scan" && arm != "memoized" {
		t.Fatalf("unknown count arm %q", arm)
	}
	workers := syncIndexDiagnosticInt(t, "PRYSM_DIAGNOSTIC_SYNC_COUNT_WORKERS", 256)
	jobs := syncIndexDiagnosticInt(t, "PRYSM_DIAGNOSTIC_SYNC_COUNT_JOBS", 4096)
	probes := syncIndexDiagnosticInt(t, "PRYSM_DIAGNOSTIC_SYNC_PROBES", 32)
	stackAfter := time.Duration(syncIndexDiagnosticInt(t, "PRYSM_DIAGNOSTIC_SYNC_STACK_AFTER_MS", 25)) * time.Millisecond

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	fixtureStarted := time.Now()
	pb := updateHeadDiagnosticGenesis(t)
	// Keep validator 0 in the current sync committee after the position cache
	// replaces the initial linear lookup. The generic fixture otherwise gives
	// every validator and committee position the same zero public key.
	pb.Validators[0].PublicKey[0] = 1
	pb.CurrentSyncCommittee.Pubkeys[0][0] = 1
	checkpointState, err := util.NewBeaconStateHeze(func(dst *ethpb.BeaconStateHeze) error {
		*dst = *pb
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), checkpointState.Slot())
	require.Equal(t, updateHeadDiagnosticValidatorCount, checkpointState.NumValidators())
	service, _ := minimalTestService(t, WithFinalizedStateAtStartUp(checkpointState))
	require.NoError(t, service.saveGenesisData(t.Context(), checkpointState))
	cp := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp, checkpointState))
	fixtureElapsed := time.Since(fixtureStarted)

	// The first call fills the one-entry sync head-state cache for slot 1. The
	// committee-cache work below also gives the async sync-position fill time to
	// finish before the verified warm call.
	indices, err := service.HeadSyncCommitteeIndices(t.Context(), 0, 1)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(indices))
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), checkpointState, 0))
	memoizedCount, err := helpers.ActiveValidatorCount(t.Context(), checkpointState, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(updateHeadDiagnosticValidatorCount), memoizedCount)
	indices, err = service.HeadSyncCommitteeIndices(t.Context(), 0, 1)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(indices))
	cachedSyncState, err := service.syncCommitteeHeadState.Get(1)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(1), cachedSyncState.Slot())

	var workerWG sync.WaitGroup
	var readyWG sync.WaitGroup
	start := make(chan struct{})
	jobCh := make(chan struct{}, jobs)
	for range jobs {
		jobCh <- struct{}{}
	}
	close(jobCh)
	var completed atomic.Int64
	var activeScans atomic.Int64
	progress := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	recordError := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	workerWG.Add(workers)
	readyWG.Add(workers)
	for range workers {
		go func() {
			defer workerWG.Done()
			readyWG.Done()
			<-start
			for range jobCh {
				// AttestationTargetState returns before the count starts, matching
				// production ordering: fork choice is not held during the scan.
				st, targetErr := service.AttestationTargetState(context.Background(), cp)
				if targetErr != nil {
					recordError(targetErr)
					continue
				}
				count := memoizedCount
				if arm == "scan" {
					activeScans.Add(1)
					count, targetErr = helpers.ActiveValidatorCount(context.Background(), st, 0)
					activeScans.Add(-1)
				}
				if targetErr != nil {
					recordError(targetErr)
					continue
				}
				if count != memoizedCount {
					recordError(fmt.Errorf("active count %d, want %d", count, memoizedCount))
					continue
				}
				completed.Add(1)
				select {
				case progress <- struct{}{}:
				default:
				}
			}
		}()
	}
	initialProbeReady := make(chan struct{})
	initialProbeDone := make(chan syncIndexCountProbeResult, 1)
	go func() {
		close(initialProbeReady)
		<-start
		jobsAtStart := completed.Load()
		scansAtStart := activeScans.Load()
		var got []primitives.CommitteeIndex
		var probeErr error
		var handlerElapsed time.Duration
		runtimeTrace.WithRegion(context.Background(), "sync-index-probe-1", func() {
			handlerStarted := time.Now()
			got, probeErr = service.HeadSyncCommitteeIndices(context.Background(), 0, 1)
			handlerElapsed = time.Since(handlerStarted)
		})
		if probeErr == nil && len(got) == 0 {
			probeErr = fmt.Errorf("empty sync committee indices")
		}
		initialProbeDone <- syncIndexCountProbeResult{
			duration: handlerElapsed, jobsAtStart: jobsAtStart, jobsAtEnd: completed.Load(),
			activeScansAtStart: scansAtStart, activeScansAtEnd: activeScans.Load(), err: probeErr,
		}
	}()

	readyWG.Wait()
	<-initialProbeReady
	if tracePath := os.Getenv("PRYSM_DIAGNOSTIC_SYNC_TRACE"); tracePath != "" {
		traceFile, createErr := os.Create(tracePath)
		require.NoError(t, createErr)
		require.NoError(t, runtimeTrace.Start(traceFile))
		defer func() {
			runtimeTrace.Stop()
			require.NoError(t, traceFile.Close())
		}()
	}
	cpuBefore := syncIndexDiagnosticUserCPUSeconds()
	workStarted := time.Now()
	close(start)
	t.Logf("SYNC_INDEX_PHASE arm=%s phase=work_released fixture_elapsed=%s workers=%d jobs=%d probes=%d", arm, fixtureElapsed, workers, jobs, probes)

	latencies := make([]time.Duration, 0, probes)
	cleanLatencies := make([]time.Duration, 0, probes)
	probesUnderLoad := 0
	probesDuringScans := 0
	stackCaptureCount := 0
	stackAttempted := false
	for call := 1; call <= probes; call++ {
		// Use completed production work as the sampling clock so scan-arm
		// probes cover later checkpoint re-entry without adding a sleep or
		// holding any application lock/channel.
		var done <-chan syncIndexCountProbeResult
		var callStarted time.Time
		if call == 1 {
			done = initialProbeDone
			callStarted = workStarted
		} else {
			threshold := int64((call - 1) * jobs / probes)
			for completed.Load() < threshold {
				<-progress
			}
			callStarted = time.Now()
			resultCh := make(chan syncIndexCountProbeResult, 1)
			done = resultCh
			probeCall := call
			go func() {
				jobsAtStart := completed.Load()
				scansAtStart := activeScans.Load()
				var got []primitives.CommitteeIndex
				var probeErr error
				var handlerElapsed time.Duration
				runtimeTrace.WithRegion(context.Background(), fmt.Sprintf("sync-index-probe-%d", probeCall), func() {
					handlerStarted := time.Now()
					got, probeErr = service.HeadSyncCommitteeIndices(context.Background(), 0, 1)
					handlerElapsed = time.Since(handlerStarted)
				})
				if probeErr == nil && len(got) == 0 {
					probeErr = fmt.Errorf("empty sync committee indices")
				}
				resultCh <- syncIndexCountProbeResult{
					duration: handlerElapsed, jobsAtStart: jobsAtStart, jobsAtEnd: completed.Load(),
					activeScansAtStart: scansAtStart, activeScansAtEnd: activeScans.Load(), err: probeErr,
				}
			}()
		}
		timer := time.NewTimer(stackAfter)
		var result syncIndexCountProbeResult
		var stack string
		attemptedThisCall := false
		select {
		case result = <-done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			// Prefer a completed result if the call and timer became ready
			// together. Capture all goroutines at most once, and mark that
			// sample as perturbed by the stop-the-world operation.
			select {
			case result = <-done:
			default:
				if !stackAttempted {
					stackAttempted = true
					attemptedThisCall = true
					stack = syncIndexDiagnosticRelevantStacks()
				}
				result = <-done
			}
		}
		observerElapsed := time.Since(callStarted)
		if result.jobsAtStart < int64(jobs) {
			probesUnderLoad++
		}
		if result.activeScansAtStart > 0 {
			probesDuringScans++
		}
		latencies = append(latencies, result.duration)
		if !attemptedThisCall {
			cleanLatencies = append(cleanLatencies, result.duration)
		}
		if result.err != nil {
			recordError(result.err)
		}
		if stack != "" {
			stackCaptureCount++
			t.Logf("SYNC_INDEX_SLOW_STACK arm=%s call=%d threshold=%s\n%s", arm, call, stackAfter, stack)
		}
		event := syncIndexCountProbe{
			Call: call, HandlerDurationUS: result.duration.Microseconds(), ObserverDurationUS: observerElapsed.Microseconds(), JobsAtStart: result.jobsAtStart,
			JobsAtEnd: result.jobsAtEnd, ActiveScansAtStart: result.activeScansAtStart, ActiveScansAtEnd: result.activeScansAtEnd, LoadActiveAtStart: result.jobsAtStart < int64(jobs),
			StackAttempted: attemptedThisCall, StackCaptured: stack != "",
		}
		encoded, marshalErr := json.Marshal(event)
		require.NoError(t, marshalErr)
		t.Logf("SYNC_INDEX_PROBE %s", encoded)
	}

	workerWG.Wait()
	workElapsed := time.Since(workStarted)
	cpuSeconds := syncIndexDiagnosticUserCPUSeconds() - cpuBefore
	select {
	case workerErr := <-errCh:
		t.Fatal(workerErr)
	default:
	}
	require.Equal(t, int64(jobs), completed.Load())
	require.Equal(t, int64(0), activeScans.Load())

	summary := map[string]any{
		"arm": arm, "workers": workers, "jobs": jobs, "jobs_completed": completed.Load(),
		"sync_probes": probes, "probes_under_load": probesUnderLoad, "probes_during_scans": probesDuringScans,
		"sync_p50_us":       syncIndexDiagnosticPercentile(latencies, 50).Microseconds(),
		"sync_p95_us":       syncIndexDiagnosticPercentile(latencies, 95).Microseconds(),
		"sync_max_us":       syncIndexDiagnosticPercentile(latencies, 100).Microseconds(),
		"sync_clean_max_us": syncIndexDiagnosticPercentile(cleanLatencies, 100).Microseconds(),
		"stack_after_ms":    stackAfter.Milliseconds(), "stack_capture_count": stackCaptureCount,
		"work_elapsed_ms": workElapsed.Milliseconds(), "estimated_user_cpu_seconds": cpuSeconds,
		"fixture_elapsed_ms": fixtureElapsed.Milliseconds(), "validator_count": checkpointState.NumValidators(),
		"memoized_count": memoizedCount, "gomaxprocs": runtime.GOMAXPROCS(0), "num_cpu": runtime.NumCPU(),
	}
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	t.Logf("SYNC_INDEX_SUMMARY %s", encoded)
}

// TestDiagnosticSlot13ParentDependencyChainUnderCheckpointCountLoad composes
// the public dependencies used to prepare a proposal parent. It does not call
// the validator RPC package's private getParentState method.
func TestDiagnosticSlot13ParentDependencyChainUnderCheckpointCountLoad(t *testing.T) {
	arm := os.Getenv("PRYSM_DIAGNOSTIC_PARENT_COUNT_ARM")
	if arm == "" {
		t.Skip("PRYSM_DIAGNOSTIC_PARENT_COUNT_ARM is not set")
	}
	if arm != "scan" && arm != "memoized" {
		t.Fatalf("unknown count arm %q", arm)
	}
	const (
		workers     = 6_144
		jobs        = 15_000
		targetSlot  = primitives.Slot(13)
		probeBudget = 10_683 * time.Millisecond
	)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = 8
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	fixtureStarted := time.Now()
	pb := updateHeadDiagnosticGenesis(t)
	pb.InactivityScores = make([]uint64, updateHeadDiagnosticValidatorCount)
	checkpointState, err := util.NewBeaconStateHeze(func(dst *ethpb.BeaconStateHeze) error {
		*dst = *pb
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), checkpointState.Slot())
	require.Equal(t, updateHeadDiagnosticValidatorCount, checkpointState.NumValidators())
	service, _ := minimalTestService(t, WithFinalizedStateAtStartUp(checkpointState))
	require.NoError(t, service.saveGenesisData(t.Context(), checkpointState))
	require.Equal(t, true, service.inRegularSync())
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp0, checkpointState))
	originalSkipSlotCache := transition.SkipSlotCache
	t.Cleanup(func() { transition.SkipSlotCache = originalSkipSlotCache })
	transition.SkipSlotCache = cache.NewSkipSlotCache()
	preflightStarted := time.Now()
	preflightState, err := transition.ProcessSlotsUsingNextSlotCache(
		t.Context(), checkpointState.Copy(), service.originBlockRoot[:], targetSlot,
	)
	preflightElapsed := time.Since(preflightStarted)
	require.NoError(t, err)
	require.Equal(t, targetSlot, preflightState.Slot())
	transition.SkipSlotCache = cache.NewSkipSlotCache()
	helpers.ClearCache()
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), checkpointState, 0))
	memoizedCount, err := helpers.ActiveValidatorCount(t.Context(), checkpointState, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(updateHeadDiagnosticValidatorCount), memoizedCount)
	require.Equal(t, service.originBlockRoot, service.CachedHeadRoot())
	require.Equal(t, service.originBlockRoot, service.GetProposerHead())
	initialHeadState, err := service.HeadState(t.Context())
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), initialHeadState.Slot())
	initialHeadRoot, initialFull := service.HeadRootAndFull()
	require.Equal(t, service.originBlockRoot, initialHeadRoot)
	fixtureElapsed := time.Since(fixtureStarted)

	jobCh := make(chan struct{}, jobs)
	for range jobs {
		jobCh <- struct{}{}
	}
	close(jobCh)
	start := make(chan struct{})
	firstCompleted := make(chan time.Time, 1)
	errCh := make(chan error, 1)
	var workerWG sync.WaitGroup
	var readyWG sync.WaitGroup
	var completed atomic.Int64
	var activeCheckpoints atomic.Int64
	var activeScans atomic.Int64
	recordError := func(workerErr error) {
		select {
		case errCh <- workerErr:
		default:
		}
	}

	workerWG.Add(workers)
	readyWG.Add(workers)
	for range workers {
		go func() {
			defer workerWG.Done()
			readyWG.Done()
			<-start
			for range jobCh {
				activeCheckpoints.Add(1)
				st, workerErr := service.AttestationTargetState(context.Background(), cp0)
				activeCheckpoints.Add(-1)
				if workerErr != nil {
					recordError(workerErr)
					continue
				}
				count := memoizedCount
				if arm == "scan" {
					activeScans.Add(1)
					count, workerErr = helpers.ActiveValidatorCount(context.Background(), st, 0)
					activeScans.Add(-1)
				}
				if workerErr != nil {
					recordError(workerErr)
					continue
				}
				if count != memoizedCount {
					recordError(fmt.Errorf("active count %d, want %d", count, memoizedCount))
					continue
				}
				if completed.Add(1) == 1 {
					firstCompleted <- time.Now()
				}
			}
		}()
	}

	readyWG.Wait()
	tracePath := os.Getenv("PRYSM_DIAGNOSTIC_PARENT_TRACE")
	if tracePath != "" {
		traceFile, createErr := os.Create(tracePath)
		require.NoError(t, createErr)
		require.NoError(t, runtimeTrace.Start(traceFile))
		defer func() {
			runtimeTrace.Stop()
			require.NoError(t, traceFile.Close())
		}()
	}
	workStarted := time.Now()
	close(start)
	t.Logf("PARENT_DEPENDENCY_PHASE arm=%s phase=work_released started_utc=%s fixture_elapsed=%s preflight_elapsed=%s preflight_slot=%d workers=%d jobs=%d target_slot=%d budget=%s regular_sync=%t initial_root=%#x initial_state_slot=%d initial_full=%t",
		arm, workStarted.UTC().Format(time.RFC3339Nano), fixtureElapsed, preflightElapsed, preflightState.Slot(), workers, jobs, targetSlot, probeBudget,
		service.inRegularSync(), service.originBlockRoot, initialHeadState.Slot(), initialFull)

	firstCompletedAt := <-firstCompleted
	triggeredAt := time.Now()
	probeCtx, cancelProbe := context.WithCancel(t.Context())
	var budgetCanceledAt atomic.Int64
	budgetTimer := time.AfterFunc(probeBudget, func() {
		budgetCanceledAt.CompareAndSwap(0, time.Now().UnixNano())
		cancelProbe()
	})
	resultCh := make(chan parentDependencyResult, 1)
	go func() {
		result := parentDependencyResult{}
		probeStarted := time.Now()
		recordPhase := func(name string, run func() string) {
			phaseStarted := time.Now()
			phase := parentDependencyPhase{
				Phase: name, StartedUTC: phaseStarted.UTC().Format(time.RFC3339Nano),
				ContextAtEnter: parentDependencyContextErr(probeCtx), JobsAtEnter: completed.Load(),
				ActiveCheckpointsAtEnter: activeCheckpoints.Load(), ActiveScansAtEnter: activeScans.Load(),
			}
			phase.Detail = run()
			phase.DurationUS = time.Since(phaseStarted).Microseconds()
			phase.SinceTriggerUS = time.Since(triggeredAt).Microseconds()
			phase.ContextAtExit = parentDependencyContextErr(probeCtx)
			phase.JobsAtExit = completed.Load()
			phase.ActiveCheckpointsAtExit = activeCheckpoints.Load()
			phase.ActiveScansAtExit = activeScans.Load()
			result.Phases = append(result.Phases, phase)
		}

		runtimeTrace.WithRegion(probeCtx, "slot13-parent-dependency-probe", func() {
			recordPhase("cached_head_root_before_update", func() string {
				result.OldHeadRoot = service.CachedHeadRoot()
				return fmt.Sprintf("root=%#x", result.OldHeadRoot)
			})
			recordPhase("update_head_13", func() string {
				runtimeTrace.WithRegion(probeCtx, "slot13-parent-update-head", func() {
					service.UpdateHead(probeCtx, targetSlot)
				})
				return fmt.Sprintf("regular_sync=%t concrete_forkchoice=%T", service.inRegularSync(), service.cfg.ForkChoiceStore)
			})
			recordPhase("cached_head_root_after_update", func() string {
				result.HeadRoot = service.CachedHeadRoot()
				return fmt.Sprintf("root=%#x", result.HeadRoot)
			})
			recordPhase("get_proposer_head", func() string {
				result.ParentRoot = service.GetProposerHead()
				return fmt.Sprintf("root=%#x", result.ParentRoot)
			})

			var headState = initialHeadState
			recordPhase("next_slot_state_before_head_state", func() string {
				cached := transition.NextSlotState(result.ParentRoot[:], targetSlot)
				result.NextSlotCacheHit = cached != nil
				if cached != nil {
					headState = cached
					return fmt.Sprintf("hit=true state_slot=%d", cached.Slot())
				}
				headState = nil
				return "hit=false"
			})
			recordPhase("head_state_on_next_slot_miss", func() string {
				if headState != nil {
					result.HeadStateSlot = headState.Slot()
					return fmt.Sprintf("skipped=true state_slot=%d", headState.Slot())
				}
				headState, result.Err = service.HeadState(probeCtx)
				if result.Err != nil {
					return fmt.Sprintf("error=%q", result.Err)
				}
				result.HeadStateSlot = headState.Slot()
				return fmt.Sprintf("state_slot=%d", headState.Slot())
			})
			recordPhase("process_slots_using_next_slot_cache_13", func() string {
				if result.Err != nil || headState == nil {
					return "skipped=true"
				}
				if headState.Slot() >= targetSlot {
					result.PreparedStateSlot = headState.Slot()
					return fmt.Sprintf("skipped=true state_slot=%d", headState.Slot())
				}
				headState, result.Err = transition.ProcessSlotsUsingNextSlotCache(probeCtx, headState, result.ParentRoot[:], targetSlot)
				if result.Err != nil {
					return fmt.Sprintf("error=%q", result.Err)
				}
				result.PreparedStateSlot = headState.Slot()
				return fmt.Sprintf("state_slot=%d", headState.Slot())
			})
			recordPhase("head_root_and_full", func() string {
				result.PostHeadRoot, result.ParentFull = service.HeadRootAndFull()
				if result.PostHeadRoot != result.ParentRoot {
					service.cfg.ForkChoiceStore.RLock()
					result.ParentFull = service.cfg.ForkChoiceStore.FullBeatsEmpty(result.ParentRoot)
					service.cfg.ForkChoiceStore.RUnlock()
					return fmt.Sprintf("head_root=%#x parent_root=%#x conditional_full=%t", result.PostHeadRoot, result.ParentRoot, result.ParentFull)
				}
				return fmt.Sprintf("head_root=%#x matched_parent=true full=%t", result.PostHeadRoot, result.ParentFull)
			})
		})
		result.ContextAtExit = probeCtx.Err()
		result.Elapsed = time.Since(probeStarted)
		resultCh <- result
	}()

	result := <-resultCh
	budgetTimer.Stop()
	budgetCanceledAtNanos := budgetCanceledAt.Load()
	cancelProbe()
	workerWG.Wait()
	workElapsed := time.Since(workStarted)
	for _, phase := range result.Phases {
		encoded, marshalErr := json.Marshal(phase)
		require.NoError(t, marshalErr)
		t.Logf("PARENT_DEPENDENCY_PROBE arm=%s %s", arm, encoded)
	}
	select {
	case workerErr := <-errCh:
		t.Fatal(workerErr)
	default:
	}
	require.Equal(t, int64(jobs), completed.Load())
	require.Equal(t, int64(0), activeCheckpoints.Load())
	require.Equal(t, int64(0), activeScans.Load())
	require.Equal(t, service.originBlockRoot, result.OldHeadRoot)
	require.Equal(t, service.originBlockRoot, result.HeadRoot)
	require.Equal(t, service.originBlockRoot, result.ParentRoot)
	require.Equal(t, service.originBlockRoot, result.PostHeadRoot)
	require.Equal(t, primitives.Slot(0), result.HeadStateSlot)
	if result.Err != nil && !errors.Is(result.Err, context.Canceled) && !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("unexpected parent dependency error: %v", result.Err)
	}
	if arm == "memoized" {
		require.NoError(t, result.Err)
	}
	if result.Err == nil {
		require.Equal(t, targetSlot, result.PreparedStateSlot)
	}

	summary := map[string]any{
		"arm": arm, "workers": workers, "jobs": jobs, "jobs_completed": completed.Load(),
		"target_slot": targetSlot, "probe_budget_ms": probeBudget.Milliseconds(),
		"trace_enabled": tracePath != "", "trace_path": tracePath,
		"first_completed_utc":           firstCompletedAt.UTC().Format(time.RFC3339Nano),
		"probe_triggered_utc":           triggeredAt.UTC().Format(time.RFC3339Nano),
		"trigger_dispatch_us":           triggeredAt.Sub(firstCompletedAt).Microseconds(),
		"jobs_at_trigger":               result.Phases[0].JobsAtEnter,
		"active_checkpoints_at_trigger": result.Phases[0].ActiveCheckpointsAtEnter,
		"active_scans_at_trigger":       result.Phases[0].ActiveScansAtEnter,
		"probe_elapsed_us":              result.Elapsed.Microseconds(), "probe_error": fmt.Sprint(result.Err),
		"context_at_probe_exit": fmt.Sprint(result.ContextAtExit), "next_slot_cache_hit": result.NextSlotCacheHit,
		"head_state_slot": result.HeadStateSlot, "prepared_state_slot": result.PreparedStateSlot,
		"old_head_root": fmt.Sprintf("%#x", result.OldHeadRoot), "head_root": fmt.Sprintf("%#x", result.HeadRoot),
		"parent_root": fmt.Sprintf("%#x", result.ParentRoot), "post_head_root": fmt.Sprintf("%#x", result.PostHeadRoot),
		"parent_full": result.ParentFull, "regular_sync": service.inRegularSync(),
		"work_elapsed_ms": workElapsed.Milliseconds(), "fixture_elapsed_ms": fixtureElapsed.Milliseconds(),
		"preflight_elapsed_us": preflightElapsed.Microseconds(), "preflight_state_slot": preflightState.Slot(),
		"memoized_count": memoizedCount, "validator_count": checkpointState.NumValidators(),
		"gomaxprocs": runtime.GOMAXPROCS(0), "num_cpu": runtime.NumCPU(),
	}
	if budgetCanceledAtNanos != 0 {
		summary["budget_cancel_since_trigger_us"] = time.Unix(0, budgetCanceledAtNanos).Sub(triggeredAt).Microseconds()
	}
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	t.Logf("PARENT_DEPENDENCY_SUMMARY %s", encoded)
}

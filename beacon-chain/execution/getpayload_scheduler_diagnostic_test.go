package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	runtimeTrace "runtime/trace"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/rpc"
)

type schedulerDiagnosticEvent struct {
	Event             string `json:"event"`
	Call              int64  `json:"call,omitempty"`
	OffsetUS          int64  `json:"offset_us,omitempty"`
	WallUnixNano      int64  `json:"wall_unix_nano,omitempty"`
	DurationUS        int64  `json:"duration_us,omitempty"`
	Bytes             int    `json:"bytes,omitempty"`
	Reused            bool   `json:"reused,omitempty"`
	JobsCompleted     int64  `json:"jobs_completed,omitempty"`
	ScansActive       int64  `json:"scans_active,omitempty"`
	LoadActive        bool   `json:"load_active,omitempty"`
	ErrorType         string `json:"error_type,omitempty"`
	Error             string `json:"error,omitempty"`
	Timeout           bool   `json:"timeout,omitempty"`
	DeadlineExceeded  bool   `json:"deadline_exceeded,omitempty"`
	ContextError      string `json:"context_error,omitempty"`
	MappedError       string `json:"mapped_error,omitempty"`
	MappedHTTPTimeout bool   `json:"mapped_http_timeout,omitempty"`
}

type schedulerDiagnosticRecorder struct {
	start       time.Time
	mu          sync.Mutex
	events      []schedulerDiagnosticEvent
	completed   *atomic.Int64
	activeScans *atomic.Int64
	totalJobs   int64
	loadEnabled bool
}

func (r *schedulerDiagnosticRecorder) add(ctx context.Context, event schedulerDiagnosticEvent) {
	event.OffsetUS = time.Since(r.start).Microseconds()
	event.WallUnixNano = time.Now().UnixNano()
	if r.completed != nil {
		event.JobsCompleted = r.completed.Load()
		event.LoadActive = r.loadEnabled && event.JobsCompleted < r.totalJobs
	}
	if r.activeScans != nil {
		event.ScansActive = r.activeScans.Load()
	}
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	runtimeTrace.Logf(ctx, "startup.engine", "phase=execution.probe.%d.%s slot=1 payload_id=0x0489a8a1998c6091 wall_unix_nano=%d", event.Call, event.Event, time.Now().UnixNano())
}

type schedulerDiagnosticBody struct {
	io.ReadCloser
	ctx      context.Context
	call     int64
	recorder *schedulerDiagnosticRecorder
}

func (b *schedulerDiagnosticBody) Read(dst []byte) (int, error) {
	started := time.Now()
	n, err := b.ReadCloser.Read(dst)
	b.recorder.add(b.ctx, schedulerDiagnosticEvent{Event: "BodyRead", Call: b.call, DurationUS: time.Since(started).Microseconds(), Bytes: n, Error: fmt.Sprint(err)})
	return n, err
}

func (b *schedulerDiagnosticBody) Close() error {
	started := time.Now()
	err := b.ReadCloser.Close()
	b.recorder.add(b.ctx, schedulerDiagnosticEvent{Event: "BodyClose", Call: b.call, DurationUS: time.Since(started).Microseconds(), Error: fmt.Sprint(err)})
	return err
}

type schedulerDiagnosticTransport struct {
	base     http.RoundTripper
	call     atomic.Int64
	recorder *schedulerDiagnosticRecorder
}

func (t *schedulerDiagnosticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	call := t.call.Add(1)
	ctx := req.Context()
	mark := func(event string) { t.recorder.add(ctx, schedulerDiagnosticEvent{Event: event, Call: call}) }
	clientTrace := &httptrace.ClientTrace{
		GetConn:      func(string) { mark("GetConn") },
		ConnectStart: func(_, _ string) { mark("ConnectStart") },
		ConnectDone:  func(_, _ string, _ error) { mark("ConnectDone") },
		GotConn: func(info httptrace.GotConnInfo) {
			t.recorder.add(ctx, schedulerDiagnosticEvent{Event: "GotConn", Call: call, Reused: info.Reused})
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { mark("WroteRequest") },
		GotFirstResponseByte: func() { mark("GotFirstResponseByte") },
	}
	req = req.WithContext(httptrace.WithClientTrace(ctx, clientTrace))
	response, err := t.base.RoundTrip(req)
	if response != nil {
		response.Body = &schedulerDiagnosticBody{ReadCloser: response.Body, ctx: ctx, call: call, recorder: t.recorder}
	}
	return response, err
}

type schedulerDiagnosticRPC struct {
	client   *rpc.Client
	call     atomic.Int64
	recorder *schedulerDiagnosticRecorder
}

func (c *schedulerDiagnosticRPC) Close() { c.client.Close() }

func (c *schedulerDiagnosticRPC) BatchCall(elements []rpc.BatchElem) error {
	return c.client.BatchCall(elements)
}

func (c *schedulerDiagnosticRPC) CallContext(ctx context.Context, result any, method string, args ...any) error {
	call := c.call.Add(1)
	started := time.Now()
	err := c.client.CallContext(ctx, result, method, args...)
	var timeout interface{ Timeout() bool }
	event := schedulerDiagnosticEvent{
		Event: "RawCallReturn", Call: call, DurationUS: time.Since(started).Microseconds(),
		ErrorType: fmt.Sprintf("%T", err), Error: fmt.Sprint(err),
		DeadlineExceeded: errors.Is(err, context.DeadlineExceeded), ContextError: fmt.Sprint(ctx.Err()),
	}
	if errors.As(err, &timeout) {
		event.Timeout = timeout.Timeout()
	}
	c.recorder.add(ctx, event)
	return err
}

func schedulerDiagnosticInt(t *testing.T, name string, fallback int) int {
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

func schedulerDiagnosticCPUSeconds() float64 {
	samples := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return samples[0].Value.Float64()
}

func TestDiagnosticGetPayloadV6UnderGenesisCountLoad(t *testing.T) {
	endpoint := os.Getenv("PRYSM_DIAGNOSTIC_ENGINE_ENDPOINT")
	if endpoint == "" {
		t.Skip("PRYSM_DIAGNOSTIC_ENGINE_ENDPOINT is not set")
	}
	arm := os.Getenv("PRYSM_DIAGNOSTIC_COUNT_ARM")
	if arm == "" {
		arm = "none"
	}
	if arm != "none" && arm != "scan" && arm != "memoized" {
		t.Fatalf("unknown count arm %q", arm)
	}
	workers := schedulerDiagnosticInt(t, "PRYSM_DIAGNOSTIC_COUNT_WORKERS", 128)
	jobs := schedulerDiagnosticInt(t, "PRYSM_DIAGNOSTIC_COUNT_JOBS", 1024)
	calls := schedulerDiagnosticInt(t, "PRYSM_DIAGNOSTIC_PAYLOAD_CALLS", 16)
	genesisPath := os.Getenv("PRYSM_STARTUP_REAL_GENESIS_SSZ")
	if genesisPath == "" {
		genesisPath = "/tmp/prysm-startup3-wire-h/bundle/network-configs/genesis.ssz"
	}
	configPath := filepath.Join(filepath.Dir(genesisPath), "config.yaml")

	params.SetupTestConfigCleanup(t)
	require.NoError(t, params.LoadChainConfigFile(configPath, nil))
	require.Equal(t, primitives.Epoch(0), params.BeaconConfig().GloasForkEpoch)
	raw, err := os.ReadFile(genesisPath)
	require.NoError(t, err)
	protoState := &ethpb.BeaconStateHeze{}
	require.NoError(t, protoState.UnmarshalSSZ(raw))
	checkpoint, err := state_native.InitializeFromProtoUnsafeHeze(protoState)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), checkpoint.Slot())
	require.Equal(t, 120_000, checkpoint.NumValidators())
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), checkpoint, 0))
	committee, err := helpers.BeaconCommitteeFromCache(t.Context(), checkpoint, 1, 0)
	require.NoError(t, err)
	if len(committee) == 0 {
		t.Fatal("committee cache did not contain the warmed epoch-zero seed")
	}
	memoizedCount, err := helpers.ActiveValidatorCount(t.Context(), checkpoint, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(120_000), memoizedCount)

	recorder := &schedulerDiagnosticRecorder{start: time.Now()}
	transport := &schedulerDiagnosticTransport{base: http.DefaultTransport.(*http.Transport).Clone(), recorder: recorder}
	httpClient := &http.Client{Transport: transport}
	rpcClient, err := rpc.DialOptions(t.Context(), endpoint, rpc.WithHTTPClient(httpClient))
	require.NoError(t, err)
	instrumented := &schedulerDiagnosticRPC{client: rpcClient, recorder: recorder}
	defer instrumented.Close()
	service := &Service{rpcClient: instrumented}
	payloadID := [8]byte{0x04, 0x89, 0xa8, 0xa1, 0x99, 0x8c, 0x60, 0x91}
	_, err = service.GetPayload(t.Context(), payloadID, 1)
	require.NoError(t, err)

	if tracePath := os.Getenv("PRYSM_DIAGNOSTIC_RUNTIME_TRACE"); tracePath != "" {
		traceFile, createErr := os.Create(tracePath)
		require.NoError(t, createErr)
		require.NoError(t, runtimeTrace.Start(traceFile))
		defer func() {
			runtimeTrace.Stop()
			require.NoError(t, traceFile.Close())
		}()
	}

	var workWG sync.WaitGroup
	var readyWG sync.WaitGroup
	start := make(chan struct{})
	jobCh := make(chan struct{}, jobs)
	for range jobs {
		jobCh <- struct{}{}
	}
	close(jobCh)
	var completed atomic.Int64
	var activeScans atomic.Int64
	var countMismatch atomic.Bool
	recorder.completed = &completed
	recorder.activeScans = &activeScans
	recorder.totalJobs = int64(jobs)
	recorder.loadEnabled = arm != "none"
	if arm != "none" {
		workWG.Add(workers)
		readyWG.Add(workers)
		for range workers {
			go func() {
				defer workWG.Done()
				readyWG.Done()
				<-start
				for range jobCh {
					count := memoizedCount
					if arm == "scan" {
						var countErr error
						activeScans.Add(1)
						count, countErr = helpers.ActiveValidatorCount(context.Background(), checkpoint, 0)
						activeScans.Add(-1)
						if countErr != nil {
							countMismatch.Store(true)
						}
					}
					if count != memoizedCount {
						countMismatch.Store(true)
					}
					completed.Add(1)
				}
			}()
		}
	}
	cpuBefore := schedulerDiagnosticCPUSeconds()
	workStarted := time.Now()
	readyWG.Wait()
	close(start)
	recorder.add(t.Context(), schedulerDiagnosticEvent{Event: "WorkReleased", JobsCompleted: completed.Load(), LoadActive: arm != "none"})
	timeouts := 0
	for call := 1; call <= calls; call++ {
		jobsAtStart := completed.Load()
		recorder.add(t.Context(), schedulerDiagnosticEvent{Event: "PayloadCallStart", Call: int64(call + 1), JobsCompleted: jobsAtStart, LoadActive: arm != "none" && jobsAtStart < int64(jobs)})
		callStarted := time.Now()
		_, callErr := service.GetPayload(t.Context(), payloadID, 1)
		jobsAtEnd := completed.Load()
		mapped := schedulerDiagnosticEvent{
			Event: "MappedCallReturn", Call: int64(call + 1), DurationUS: time.Since(callStarted).Microseconds(),
			ErrorType: fmt.Sprintf("%T", callErr), MappedError: fmt.Sprint(callErr),
			MappedHTTPTimeout: errors.Is(callErr, ErrHTTPTimeout), DeadlineExceeded: errors.Is(callErr, context.DeadlineExceeded),
			ContextError: fmt.Sprint(t.Context().Err()), JobsCompleted: jobsAtEnd,
			LoadActive: arm != "none" && jobsAtEnd < int64(jobs),
		}
		recorder.add(t.Context(), mapped)
		if callErr != nil {
			require.ErrorIs(t, callErr, ErrHTTPTimeout)
			require.Equal(t, false, errors.Is(callErr, context.DeadlineExceeded))
			require.NoError(t, t.Context().Err())
			timeouts++
		}
	}
	workWG.Wait()
	cpuAfter := schedulerDiagnosticCPUSeconds()
	require.Equal(t, false, countMismatch.Load())
	if arm == "none" {
		require.Equal(t, int64(0), completed.Load())
	} else {
		require.Equal(t, int64(jobs), completed.Load())
	}
	if arm != "scan" {
		require.Equal(t, 0, timeouts)
	}

	summary := map[string]any{
		"arm": arm, "workers": workers, "jobs": jobs, "jobs_completed": completed.Load(),
		"payload_calls": calls, "gomaxprocs": runtime.GOMAXPROCS(0), "num_cpu": runtime.NumCPU(),
		"elapsed_ms": time.Since(workStarted).Milliseconds(), "estimated_user_cpu_seconds": cpuAfter - cpuBefore,
		"genesis": genesisPath, "validator_count": checkpoint.NumValidators(), "memoized_count": memoizedCount,
		"recorder_start_unix_nano": recorder.start.UnixNano(), "timeouts": timeouts,
	}
	encodedSummary, err := json.Marshal(summary)
	require.NoError(t, err)
	t.Log(string(encodedSummary))
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, event := range recorder.events {
		encoded, marshalErr := json.Marshal(event)
		require.NoError(t, marshalErr)
		t.Log(string(encoded))
	}
}

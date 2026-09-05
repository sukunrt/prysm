package sync_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	runtimetrace "runtime/trace"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	rpccore "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	rpcvalidator "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/prysm/v1alpha1/validator"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	mocksync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	fullGossipDomainProbeCount    = 32
	fullGossipWriterProbeCount    = 48
	fullGossipDomainProbeInterval = 250 * time.Millisecond
	fullGossipDomainOfferDuration = 2 * time.Second
	fullGossipDomainMetadataKey   = "x-prysm-full-gossip-domain-probe"
)

type fullGossipDomainClientProbe struct {
	Probe              int    `json:"probe"`
	PlannedUnixNano    int64  `json:"planned_unix_nano"`
	InvokeUnixNano     int64  `json:"invoke_unix_nano"`
	ReturnUnixNano     int64  `json:"return_unix_nano"`
	DurationNano       int64  `json:"duration_nano"`
	ScheduleLateNano   int64  `json:"schedule_lateness_nano"`
	DeadlineUnixNano   int64  `json:"deadline_unix_nano"`
	BudgetAtInvokeNano int64  `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano int64  `json:"budget_at_return_nano"`
	StatusCode         string `json:"status_code"`
	ResponseBytes      int    `json:"response_bytes"`
	ResponseMatches    bool   `json:"response_matches_warm"`
	Skipped            bool   `json:"skipped"`
	SkipObservedNano   int64  `json:"skip_observed_unix_nano,omitempty"`
	SkipReason         string `json:"skip_reason,omitempty"`
	Error              string `json:"error,omitempty"`
}

type fullGossipAttestationDataClient struct {
	InvokeUnixNano     int64                     `json:"invoke_unix_nano"`
	ReturnUnixNano     int64                     `json:"return_unix_nano"`
	DurationNano       int64                     `json:"duration_nano"`
	DeadlineUnixNano   int64                     `json:"deadline_unix_nano"`
	BudgetAtInvokeNano int64                     `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano int64                     `json:"budget_at_return_nano"`
	StatusCode         string                    `json:"status_code"`
	ResponseSlot       primitives.Slot           `json:"response_slot"`
	ResponseIndex      primitives.CommitteeIndex `json:"response_index"`
	BeaconBlockRoot    []byte                    `json:"beacon_block_root,omitempty"`
	Source             *ethpb.Checkpoint         `json:"source,omitempty"`
	Target             *ethpb.Checkpoint         `json:"target,omitempty"`
	Error              string                    `json:"error,omitempty"`
	ContextError       string                    `json:"context_error,omitempty"`
	Omitted            bool                      `json:"omitted"`
	OmitReason         string                    `json:"omit_reason,omitempty"`
}

type fullGossipAttestationDataPreflight struct {
	fullGossipAttestationDataClient
	CacheCold      bool `json:"cache_cold"`
	CachePopulated bool `json:"cache_populated"`
}

type fullGossipDomainServerProbe struct {
	Probe             int                             `json:"probe"`
	AdmissionUnixNano int64                           `json:"admission_unix_nano"`
	ReturnUnixNano    int64                           `json:"return_unix_nano"`
	HandlerDuration   int64                           `json:"handler_duration_nano"`
	StatsAtAdmission  prysmsync.FullGossipDomainStats `json:"stats_at_admission"`
	StatsAtReturn     prysmsync.FullGossipDomainStats `json:"stats_at_return"`
}

type fullGossipDomainColdSync struct {
	InvokeUnixNano     int64                           `json:"invoke_unix_nano"`
	ReturnUnixNano     int64                           `json:"return_unix_nano"`
	DurationNano       int64                           `json:"duration_nano"`
	DeadlineUnixNano   int64                           `json:"deadline_unix_nano"`
	BudgetAtInvokeNano int64                           `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano int64                           `json:"budget_at_return_nano"`
	IndexCount         int                             `json:"index_count"`
	StatsAtInvoke      prysmsync.FullGossipDomainStats `json:"stats_at_invoke"`
	StatsAtReturn      prysmsync.FullGossipDomainStats `json:"stats_at_return"`
	Error              string                          `json:"error,omitempty"`
	ContextError       string                          `json:"context_error,omitempty"`
}

type fullGossipAttestationDataServer struct {
	AdmissionUnixNano int64                           `json:"admission_unix_nano"`
	ReturnUnixNano    int64                           `json:"return_unix_nano"`
	HandlerDuration   int64                           `json:"handler_duration_nano"`
	StatsAtAdmission  prysmsync.FullGossipDomainStats `json:"stats_at_admission"`
	StatsAtReturn     prysmsync.FullGossipDomainStats `json:"stats_at_return"`
	Error             string                          `json:"error,omitempty"`
}

type fullGossipHeadCopyCall struct {
	InvokeUnixNano int64  `json:"invoke_unix_nano"`
	ReturnUnixNano int64  `json:"return_unix_nano"`
	DurationNano   int64  `json:"duration_nano"`
	Error          string `json:"error,omitempty"`
}

type fullGossipDomainSummary struct {
	Arm                         string                              `json:"arm"`
	FixtureStartUnixNano        int64                               `json:"fixture_start_unix_nano"`
	FixtureReadyUnixNano        int64                               `json:"fixture_ready_unix_nano"`
	FixtureDurationNano         int64                               `json:"fixture_duration_nano"`
	GenesisUnixNano             int64                               `json:"genesis_unix_nano"`
	SlotOneStartUnixNano        int64                               `json:"slot_one_start_unix_nano"`
	SlotTwoDeadlineUnixNano     int64                               `json:"slot_two_deadline_unix_nano"`
	SetupCompletedUnixNano      int64                               `json:"setup_completed_unix_nano"`
	WorkReleasedUnixNano        int64                               `json:"work_released_unix_nano"`
	OfferFinishedUnixNano       int64                               `json:"offer_finished_unix_nano"`
	SettledUnixNano             int64                               `json:"settled_unix_nano"`
	SetupBudgetAtReleaseNano    int64                               `json:"slot_budget_at_work_release_nano"`
	Published                   int64                               `json:"published"`
	PublishErrors               int64                               `json:"publish_errors"`
	MaximumPublishLatenessNano  int64                               `json:"maximum_publish_lateness_nano"`
	FinalStats                  prysmsync.FullGossipDomainStats     `json:"final_stats"`
	ColdSync                    fullGossipDomainColdSync            `json:"cold_sync"`
	WriterComposition           bool                                `json:"writer_composition"`
	TimedAttestationData        bool                                `json:"timed_attestation_data"`
	TimedAttestationDataOmitted bool                                `json:"timed_attestation_data_omitted"`
	AttestationCacheCold        bool                                `json:"attestation_cache_cold"`
	AttestationCachePopulated   bool                                `json:"attestation_cache_populated"`
	CanonicalSlotOneRoot        [32]byte                            `json:"canonical_slot_one_root"`
	CanonicalSlotOneFull        bool                                `json:"canonical_slot_one_full"`
	AttestationHandlerAdmitted  bool                                `json:"attestation_handler_admitted"`
	AttestationHandlerDone      bool                                `json:"attestation_handler_done"`
	AttestationHandlerPending   bool                                `json:"attestation_handler_pending"`
	AttestationDataPreflight    *fullGossipAttestationDataPreflight `json:"attestation_data_preflight,omitempty"`
	AttestationDataClient       *fullGossipAttestationDataClient    `json:"attestation_data_client,omitempty"`
	AttestationDataServer       *fullGossipAttestationDataServer    `json:"attestation_data_server,omitempty"`
	HeadCopy                    *fullGossipHeadCopyCall             `json:"head_copy,omitempty"`
	RuntimeTrace                bool                                `json:"runtime_trace"`
	RuntimeTraceStartUnixNano   int64                               `json:"runtime_trace_start_unix_nano,omitempty"`
	RuntimeTraceStopUnixNano    int64                               `json:"runtime_trace_stop_unix_nano,omitempty"`
	RuntimeNumCPU               int                                 `json:"runtime_num_cpu"`
	GOMAXPROCS                  int                                 `json:"gomaxprocs"`
}

type fullGossipDomainServerRecorder struct {
	fixture         *prysmsync.FullGossipDomainDiagnosticFixture
	mu              sync.Mutex
	records         []fullGossipDomainServerProbe
	attestationData *fullGossipAttestationDataServer
	attestationIn   chan struct{}
	attestationDone chan struct{}
	admissionOnce   sync.Once
	doneOnce        sync.Once
}

func (r *fullGossipDomainServerRecorder) intercept(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	probe := -1
	if values := md.Get(fullGossipDomainMetadataKey); len(values) == 1 {
		probe, _ = strconv.Atoi(values[0])
	}
	started := time.Now()
	isAttestationData := info.FullMethod == "/ethereum.eth.v1alpha1.BeaconNodeValidator/GetAttestationData"
	if isAttestationData && r.attestationIn != nil {
		r.admissionOnce.Do(func() { close(r.attestationIn) })
	}
	record := fullGossipDomainServerProbe{
		Probe: probe, AdmissionUnixNano: started.UnixNano(), StatsAtAdmission: r.fixture.Stats(),
	}
	response, err := handler(ctx, request)
	returned := time.Now()
	record.ReturnUnixNano = returned.UnixNano()
	record.HandlerDuration = returned.Sub(started).Nanoseconds()
	record.StatsAtReturn = r.fixture.Stats()
	if isAttestationData {
		attestationRecord := &fullGossipAttestationDataServer{
			AdmissionUnixNano: record.AdmissionUnixNano, ReturnUnixNano: record.ReturnUnixNano,
			HandlerDuration: record.HandlerDuration, StatsAtAdmission: record.StatsAtAdmission,
			StatsAtReturn: record.StatsAtReturn,
		}
		if err != nil {
			attestationRecord.Error = err.Error()
		}
		r.mu.Lock()
		r.attestationData = attestationRecord
		r.mu.Unlock()
		if r.attestationDone != nil {
			r.doneOnce.Do(func() { close(r.attestationDone) })
		}
	}
	if probe >= 0 {
		r.mu.Lock()
		r.records = append(r.records, record)
		r.mu.Unlock()
	}
	return response, err
}

type fullGossipHeadCopyFetcher struct {
	*blockchain.Service
	mu     sync.Mutex
	record *fullGossipHeadCopyCall
}

func (f *fullGossipHeadCopyFetcher) HeadState(ctx context.Context) (state.BeaconState, error) {
	invoked := time.Now()
	result, err := f.Service.HeadState(ctx)
	returned := time.Now()
	record := &fullGossipHeadCopyCall{
		InvokeUnixNano: invoked.UnixNano(), ReturnUnixNano: returned.UnixNano(),
		DurationNano: returned.Sub(invoked).Nanoseconds(),
	}
	if err != nil {
		record.Error = err.Error()
	}
	f.mu.Lock()
	f.record = record
	f.mu.Unlock()
	return result, err
}

func (f *fullGossipHeadCopyFetcher) Record() *fullGossipHeadCopyCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil {
		return nil
	}
	copy := *f.record
	return &copy
}

func TestDiagnosticFullGossipDomainTCPClient(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_CHILD") != "1" {
		t.Skip("external full-gossip DomainData client only")
	}
	address := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_ADDRESS")
	resultPath := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_CLIENT_RESULTS")
	attestationResultPath := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_ATTESTATION_RESULTS")
	writerComposition := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_WRITER") == "1"
	omitTimedAttestationData := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_OMIT_TIMED_ATTESTATION_DATA") == "1"
	timedAttestationData := writerComposition && !omitTimedAttestationData
	if omitTimedAttestationData {
		require.Equal(t, true, writerComposition)
	}
	require.NotEqual(t, "", address)
	require.NotEqual(t, "", resultPath)
	if writerComposition {
		require.NotEqual(t, "", attestationResultPath)
	}

	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { require.NoError(t, connection.Close()) }()
	client := ethpb.NewBeaconNodeValidatorClient(connection)
	request := &ethpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainRandao[:]}
	warmContext, warmCancel := context.WithTimeout(t.Context(), 12*time.Second)
	warmResponse, err := client.DomainData(warmContext, request)
	warmCancel()
	require.NoError(t, err)

	ready := os.NewFile(uintptr(3), "full-gossip-domain-ready")
	require.NotNil(t, ready)
	_, err = ready.WriteString("READY\n")
	require.NoError(t, err)
	require.NoError(t, ready.Close())

	var header [16]byte
	_, err = io.ReadFull(os.Stdin, header[:])
	require.NoError(t, err)
	probeStart := time.Unix(0, int64(binary.LittleEndian.Uint64(header[:8])))
	firstDeadline := time.Unix(0, int64(binary.LittleEndian.Uint64(header[8:])))
	probeCount := fullGossipDomainProbeCount
	firstSequentialProbe := 0
	records := make([]fullGossipDomainClientProbe, 0, fullGossipWriterProbeCount)
	var attestationDone chan fullGossipAttestationDataClient
	if writerComposition {
		probeCount = fullGossipWriterProbeCount
		firstSequentialProbe = 1
		start := make(chan struct{})
		firstDomainDone := make(chan fullGossipDomainClientProbe, 1)
		if timedAttestationData {
			attestationDone = make(chan fullGossipAttestationDataClient, 1)
		}
		go func() {
			<-start
			firstDomainDone <- invokeFullGossipDomain(
				t.Context(), client, request, warmResponse, 0, probeStart, firstDeadline,
			)
		}()
		if timedAttestationData {
			go func() {
				<-start
				attestationDone <- invokeFullGossipAttestationData(t.Context(), client, firstDeadline)
			}()
		}
		close(start)
		records = append(records, <-firstDomainDone)
	}
	for probe := firstSequentialProbe; probe < probeCount; probe++ {
		planned := probeStart.Add(time.Duration(probe) * fullGossipDomainProbeInterval)
		if probe > 0 && !time.Now().Before(planned) {
			observed := time.Now()
			records = append(records, fullGossipDomainClientProbe{
				Probe: probe, PlannedUnixNano: planned.UnixNano(), Skipped: true,
				SkipObservedNano: observed.UnixNano(), SkipReason: "planned tick elapsed while prior RPC was pending",
			})
			continue
		}
		if delay := time.Until(planned); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-t.Context().Done():
				timer.Stop()
				t.Fatal(t.Context().Err())
			case <-timer.C:
			}
		}
		deadline := time.Now().Add(12 * time.Second)
		if writerComposition || probe == 0 {
			deadline = firstDeadline
		}
		records = append(records, invokeFullGossipDomain(
			t.Context(), client, request, warmResponse, probe, planned, deadline,
		))
	}
	if writerComposition {
		attestationRecord := fullGossipAttestationDataClient{}
		if timedAttestationData {
			attestationRecord = <-attestationDone
		} else {
			attestationRecord.Omitted = true
			attestationRecord.OmitReason = "diagnostic control omitted timed GetAttestationData"
		}
		require.NoError(t, writeFullGossipDomainJSON(attestationResultPath, attestationRecord))
	}
	require.NoError(t, writeFullGossipDomainJSON(resultPath, records))
}

func invokeFullGossipDomain(
	parent context.Context,
	client ethpb.BeaconNodeValidatorClient,
	request *ethpb.DomainRequest,
	warmResponse *ethpb.DomainResponse,
	probe int,
	planned time.Time,
	deadline time.Time,
) fullGossipDomainClientProbe {
	invoked := time.Now()
	callContext, callCancel := context.WithDeadline(parent, deadline)
	callContext = metadata.AppendToOutgoingContext(callContext, fullGossipDomainMetadataKey, strconv.Itoa(probe))
	response, callErr := client.DomainData(callContext, request)
	returned := time.Now()
	callCancel()
	record := fullGossipDomainClientProbe{
		Probe: probe, PlannedUnixNano: planned.UnixNano(), InvokeUnixNano: invoked.UnixNano(),
		ReturnUnixNano: returned.UnixNano(), DurationNano: returned.Sub(invoked).Nanoseconds(),
		ScheduleLateNano: invoked.Sub(planned).Nanoseconds(), DeadlineUnixNano: deadline.UnixNano(),
		BudgetAtInvokeNano: deadline.Sub(invoked).Nanoseconds(), BudgetAtReturnNano: deadline.Sub(returned).Nanoseconds(),
		StatusCode: status.Code(callErr).String(),
	}
	if response != nil {
		record.ResponseBytes = len(response.SignatureDomain)
		record.ResponseMatches = bytes.Equal(response.SignatureDomain, warmResponse.SignatureDomain)
	}
	if callErr != nil {
		record.Error = callErr.Error()
	}
	return record
}

func invokeFullGossipAttestationData(
	parent context.Context,
	client ethpb.BeaconNodeValidatorClient,
	deadline time.Time,
) fullGossipAttestationDataClient {
	invoked := time.Now()
	callContext, callCancel := context.WithDeadline(parent, deadline)
	response, callErr := client.GetAttestationData(callContext, &ethpb.AttestationDataRequest{
		Slot: 1, CommitteeIndex: 0,
	})
	returned := time.Now()
	contextErr := callContext.Err()
	callCancel()
	record := fullGossipAttestationDataClient{
		InvokeUnixNano: invoked.UnixNano(), ReturnUnixNano: returned.UnixNano(),
		DurationNano: returned.Sub(invoked).Nanoseconds(), DeadlineUnixNano: deadline.UnixNano(),
		BudgetAtInvokeNano: deadline.Sub(invoked).Nanoseconds(), BudgetAtReturnNano: deadline.Sub(returned).Nanoseconds(),
		StatusCode: status.Code(callErr).String(),
	}
	fullGossipRecordAttestationResponse(&record, response)
	if callErr != nil {
		record.Error = callErr.Error()
	}
	if contextErr != nil {
		record.ContextError = contextErr.Error()
	}
	return record
}

func runFullGossipAttestationDataPreflight(
	t *testing.T,
	fixture *prysmsync.FullGossipDomainDiagnosticFixture,
	deadline time.Time,
) *fullGossipAttestationDataPreflight {
	t.Helper()
	preflightCache := cache.NewAttestationDataCache()
	record := &fullGossipAttestationDataPreflight{
		CacheCold: !fullGossipAttestationCachePopulated(preflightCache),
	}
	require.Equal(t, true, record.CacheCold)
	preflightServer := &rpcvalidator.Server{
		Ctx: t.Context(), SyncChecker: &mocksync.Sync{IsSyncing: false},
		CoreService: &rpccore.Service{
			HeadFetcher: fixture.Chain, ChainInfoFetcher: fixture.Chain,
			GenesisTimeFetcher: fixture.Chain, OptimisticModeFetcher: fixture.Chain,
			AttestationCache: preflightCache,
		},
	}
	invoked := time.Now()
	preflightContext, preflightCancel := context.WithDeadline(t.Context(), deadline)
	response, err := preflightServer.GetAttestationData(preflightContext, &ethpb.AttestationDataRequest{
		Slot: 1, CommitteeIndex: 0,
	})
	returned := time.Now()
	contextErr := preflightContext.Err()
	preflightCancel()
	record.InvokeUnixNano = invoked.UnixNano()
	record.ReturnUnixNano = returned.UnixNano()
	record.DurationNano = returned.Sub(invoked).Nanoseconds()
	record.DeadlineUnixNano = deadline.UnixNano()
	record.BudgetAtInvokeNano = deadline.Sub(invoked).Nanoseconds()
	record.BudgetAtReturnNano = deadline.Sub(returned).Nanoseconds()
	record.StatusCode = status.Code(err).String()
	fullGossipRecordAttestationResponse(&record.fullGossipAttestationDataClient, response)
	if err != nil {
		record.Error = err.Error()
	}
	if contextErr != nil {
		record.ContextError = contextErr.Error()
	}
	record.CachePopulated = fullGossipAttestationCachePopulated(preflightCache)
	require.NoError(t, err)
	require.Equal(t, uint64(1), uint64(record.ResponseSlot))
	require.Equal(t, uint64(1), uint64(record.ResponseIndex))
	require.DeepEqual(t, fixture.GenesisRoot[:], record.BeaconBlockRoot)
	require.NotNil(t, record.Source)
	require.NotNil(t, record.Target)
	require.Equal(t, uint64(0), uint64(record.Source.Epoch))
	require.Equal(t, uint64(0), uint64(record.Target.Epoch))
	require.DeepEqual(t, make([]byte, 32), record.Source.Root)
	require.DeepEqual(t, fixture.GenesisRoot[:], record.Target.Root)
	require.Equal(t, true, record.CachePopulated)
	return record
}

func fullGossipRecordAttestationResponse(record *fullGossipAttestationDataClient, response *ethpb.AttestationData) {
	if response == nil {
		return
	}
	record.ResponseSlot = response.Slot
	record.ResponseIndex = response.CommitteeIndex
	record.BeaconBlockRoot = append([]byte(nil), response.BeaconBlockRoot...)
	if response.Source != nil {
		record.Source = &ethpb.Checkpoint{Epoch: response.Source.Epoch, Root: append([]byte(nil), response.Source.Root...)}
	}
	if response.Target != nil {
		record.Target = &ethpb.Checkpoint{Epoch: response.Target.Epoch, Root: append([]byte(nil), response.Target.Root...)}
	}
}

func TestDiagnosticFullGossipDomainTCP(t *testing.T) {
	arm := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_ARM")
	writerComposition := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_WRITER") == "1"
	omitTimedAttestationData := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_OMIT_TIMED_ATTESTATION_DATA") == "1"
	timedAttestationData := writerComposition && !omitTimedAttestationData
	if omitTimedAttestationData {
		require.Equal(t, true, writerComposition)
	}
	if arm == "" {
		t.Skip("PRYSM_DIAGNOSTIC_FULL_GOSSIP_ARM is not set")
	}
	if arm != "shared" && arm != "snapshot" {
		t.Fatalf("unknown full-gossip arm %q", arm)
	}
	outputDirectory := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_OUTPUT_DIR")
	if outputDirectory == "" {
		outputDirectory = filepath.Join(os.TempDir(), "prysm-full-gossip-domain-"+arm)
	}
	require.NoError(t, os.MkdirAll(outputDirectory, 0o755))

	fixtureStarted := time.Now()
	fixture := prysmsync.NewFullGossipDomainDiagnosticFixture(t, arm == "snapshot")
	fixtureReady := time.Now()
	slotOneStart, err := slots.StartTime(fixture.GenesisTime, 1)
	require.NoError(t, err)
	slotTwoDeadline, err := slots.StartTime(fixture.GenesisTime, 2)
	require.NoError(t, err)
	require.Equal(t, uint64(1), uint64(slots.CurrentSlot(fixture.GenesisTime)))
	canonicalSlotOneRoot, canonicalSlotOneFull := fixture.Chain.CanonicalNodeAtSlot(1)
	var attestationPreflight *fullGossipAttestationDataPreflight
	if writerComposition {
		require.Equal(t, fixture.GenesisRoot, canonicalSlotOneRoot)
		require.Equal(t, true, canonicalSlotOneFull)
		attestationPreflight = runFullGossipAttestationDataPreflight(t, fixture, slotTwoDeadline)
	}

	recorder := &fullGossipDomainServerRecorder{fixture: fixture}
	if timedAttestationData {
		recorder.attestationIn = make(chan struct{})
		recorder.attestationDone = make(chan struct{})
	}
	attestationDataCache := cache.NewAttestationDataCache()
	attestationCacheCold := !fullGossipAttestationCachePopulated(attestationDataCache)
	require.Equal(t, true, attestationCacheCold)
	headCopyFetcher := &fullGossipHeadCopyFetcher{Service: fixture.Chain}
	validatorServer := &rpcvalidator.Server{Ctx: t.Context(), HeadFetcher: fixture.Chain}
	if writerComposition {
		validatorServer.SyncChecker = &mocksync.Sync{IsSyncing: false}
		validatorServer.CoreService = &rpccore.Service{
			HeadFetcher: headCopyFetcher, ChainInfoFetcher: fixture.Chain,
			GenesisTimeFetcher: fixture.Chain, OptimisticModeFetcher: fixture.Chain,
			AttestationCache: attestationDataCache,
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(recorder.intercept))
	ethpb.RegisterBeaconNodeValidatorServer(grpcServer, validatorServer)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	executable, err := os.Executable()
	require.NoError(t, err)
	triggerRead, triggerWrite, err := os.Pipe()
	require.NoError(t, err)
	readyRead, readyWrite, err := os.Pipe()
	require.NoError(t, err)
	clientResultPath := filepath.Join(outputDirectory, "client.jsonl")
	attestationResultPath := filepath.Join(outputDirectory, "attestation-data-client.json")
	clientLog, err := os.Create(filepath.Join(outputDirectory, "client.test.log"))
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run", "^TestDiagnosticFullGossipDomainTCPClient$", "-test.v", "-test.timeout=8m")
	command.Env = append(os.Environ(),
		"PRYSM_DIAGNOSTIC_FULL_GOSSIP_CHILD=1",
		"PRYSM_DIAGNOSTIC_FULL_GOSSIP_ADDRESS="+listener.Addr().String(),
		"PRYSM_DIAGNOSTIC_FULL_GOSSIP_CLIENT_RESULTS="+clientResultPath,
	)
	if writerComposition {
		command.Env = append(command.Env,
			"PRYSM_DIAGNOSTIC_FULL_GOSSIP_WRITER=1",
			"PRYSM_DIAGNOSTIC_FULL_GOSSIP_ATTESTATION_RESULTS="+attestationResultPath,
		)
		if !timedAttestationData {
			command.Env = append(command.Env, "PRYSM_DIAGNOSTIC_FULL_GOSSIP_OMIT_TIMED_ATTESTATION_DATA=1")
		}
	}
	command.Stdin = triggerRead
	command.Stdout = clientLog
	command.Stderr = clientLog
	command.ExtraFiles = []*os.File{readyWrite}
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
	})
	require.NoError(t, triggerRead.Close())
	require.NoError(t, readyWrite.Close())
	clientDone := make(chan error, 1)
	go func() { clientDone <- command.Wait() }()
	readyLine := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(readyRead).ReadString('\n')
		readyLine <- line
	}()
	select {
	case line := <-readyLine:
		require.Equal(t, "READY\n", line)
	case childErr := <-clientDone:
		t.Fatalf("DomainData client exited before ready: %v", childErr)
	case <-time.After(30 * time.Second):
		t.Fatal("DomainData client did not establish and warm its connection")
	}
	require.NoError(t, readyRead.Close())
	tracePath := os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_TRACE")
	traceStarted := time.Time{}
	traceStopped := time.Time{}
	var traceFile *os.File
	traceRunning := false
	stopTrace := func() {
		if !traceRunning {
			return
		}
		runtimetrace.Stop()
		traceStopped = time.Now()
		traceRunning = false
		require.NoError(t, traceFile.Close())
	}
	defer stopTrace()
	if tracePath != "" {
		traceFile, err = os.Create(tracePath)
		require.NoError(t, err)
		require.NoError(t, runtimetrace.Start(traceFile))
		traceStarted = time.Now()
		traceRunning = true
	}

	workReleased := time.Now()
	if !workReleased.Before(slotTwoDeadline) {
		t.Fatalf("external client setup exhausted slot one: ready=%s deadline=%s", workReleased.Sub(slotOneStart), slotTwoDeadline.Sub(slotOneStart))
	}
	type offerResult struct {
		finished    time.Time
		published   int64
		errors      int64
		maxLateness time.Duration
	}
	offerDone := make(chan offerResult, 1)
	go func() {
		result := offerResult{}
		for index := range prysmsync.FullGossipDomainDiagnosticMessages {
			planned := workReleased.Add(time.Duration(index) * fullGossipDomainOfferDuration / prysmsync.FullGossipDomainDiagnosticMessages)
			if delay := time.Until(planned); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-t.Context().Done():
					timer.Stop()
					result.errors++
					result.finished = time.Now()
					offerDone <- result
					return
				case <-timer.C:
				}
			}
			offered := time.Now()
			result.maxLateness = max(result.maxLateness, offered.Sub(planned))
			if publishErr := fixture.Publish(t.Context(), index); publishErr != nil {
				result.errors++
			} else {
				result.published++
			}
		}
		result.finished = time.Now()
		offerDone <- result
	}()

	select {
	case <-fixture.FirstIterator():
	case <-time.After(20 * time.Second):
		t.Fatalf("no checkpoint validator iterator started: %+v", fixture.Stats())
	}
	syncContext, syncCancel := context.WithDeadline(t.Context(), slotTwoDeadline)
	syncInvoked := time.Now()
	coldSync := fullGossipDomainColdSync{
		InvokeUnixNano: syncInvoked.UnixNano(), DeadlineUnixNano: slotTwoDeadline.UnixNano(),
		BudgetAtInvokeNano: slotTwoDeadline.Sub(syncInvoked).Nanoseconds(), StatsAtInvoke: fixture.Stats(),
	}
	indices, syncErr := fixture.Chain.HeadSyncCommitteeIndices(syncContext, 0, primitives.Slot(1))
	syncReturned := time.Now()
	syncContextErr := syncContext.Err()
	syncCancel()
	coldSync.ReturnUnixNano = syncReturned.UnixNano()
	coldSync.DurationNano = syncReturned.Sub(syncInvoked).Nanoseconds()
	coldSync.BudgetAtReturnNano = slotTwoDeadline.Sub(syncReturned).Nanoseconds()
	coldSync.IndexCount = len(indices)
	coldSync.StatsAtReturn = fixture.Stats()
	if syncErr != nil {
		coldSync.Error = syncErr.Error()
	}
	if syncContextErr != nil {
		coldSync.ContextError = syncContextErr.Error()
	}
	if syncErr != nil && !errors.Is(syncErr, context.Canceled) && !errors.Is(syncErr, context.DeadlineExceeded) {
		t.Fatalf("cold sync-index prerequisite: %v", syncErr)
	}
	probeStart := time.Now()
	if writerComposition {
		probeStart = slotTwoDeadline.Add(-12 * time.Second)
		require.Equal(t, slotOneStart, probeStart)
	}
	var header [16]byte
	binary.LittleEndian.PutUint64(header[:8], uint64(probeStart.UnixNano()))
	binary.LittleEndian.PutUint64(header[8:], uint64(slotTwoDeadline.UnixNano()))
	_, err = triggerWrite.Write(header[:])
	require.NoError(t, err)
	require.NoError(t, triggerWrite.Close())

	offer := <-offerDone
	settled := waitFullGossipDomainSettled(t, fixture)
	select {
	case err = <-clientDone:
		require.NoError(t, err)
	case <-time.After(3 * time.Minute):
		_ = command.Process.Kill()
		t.Fatal("external DomainData client did not finish")
	}
	require.NoError(t, clientLog.Close())
	clientRecords, err := readFullGossipDomainClientJSONL(clientResultPath)
	require.NoError(t, err)
	if writerComposition {
		require.Equal(t, fullGossipWriterProbeCount, len(clientRecords))
		for _, record := range clientRecords {
			if record.Skipped {
				require.NotEqual(t, "", record.SkipReason)
				continue
			}
			switch record.StatusCode {
			case "OK":
				require.Equal(t, true, record.ResponseMatches)
				require.Equal(t, 32, record.ResponseBytes)
			case "Canceled", "DeadlineExceeded":
				// Deadline outcomes are measured results for this absolute-slot budget.
			default:
				t.Fatalf("DomainData probe %d returned non-deadline status %s: %s", record.Probe, record.StatusCode, record.Error)
			}
		}
	}
	attestationHandlerAdmitted := false
	attestationHandlerDone := false
	attestationHandlerPending := false
	if timedAttestationData {
		select {
		case <-recorder.attestationIn:
			attestationHandlerAdmitted = true
		case <-time.After(100 * time.Millisecond):
		}
		if attestationHandlerAdmitted {
			select {
			case <-recorder.attestationDone:
				attestationHandlerDone = true
			case <-time.After(30 * time.Second):
				attestationHandlerPending = true
			}
		}
	}
	stopTrace()

	finalStats := fixture.Stats()
	require.Equal(t, int64(0), offer.errors)
	require.Equal(t, int64(prysmsync.FullGossipDomainDiagnosticMessages), offer.published)
	require.Equal(t, int64(0), finalStats.ValidationRejected)
	require.Equal(t, finalStats.ValidationAccepted, finalStats.SubscriberCompleted)
	require.Equal(t, finalStats.SubscriberCompleted, int64(finalStats.UnaggregatedPoolEntries))
	require.Equal(t, int64(0), finalStats.ActiveIterators)
	require.Equal(t, finalStats.IteratorEntries, finalStats.IteratorExits)
	require.NotEqual(t, int64(0), finalStats.ValidationStarted)
	if syncErr == nil {
		require.NotEqual(t, 0, len(indices))
	}
	var attestationClient *fullGossipAttestationDataClient
	if writerComposition {
		attestationClient = new(fullGossipAttestationDataClient)
		require.NoError(t, readFullGossipDomainJSON(attestationResultPath, attestationClient))
		if timedAttestationData {
			switch attestationClient.StatusCode {
			case "OK":
				require.Equal(t, uint64(1), uint64(attestationClient.ResponseSlot))
				require.Equal(t, uint64(1), uint64(attestationClient.ResponseIndex))
				require.DeepEqual(t, fixture.GenesisRoot[:], attestationClient.BeaconBlockRoot)
				require.NotNil(t, attestationClient.Source)
				require.NotNil(t, attestationClient.Target)
				require.Equal(t, uint64(0), uint64(attestationClient.Source.Epoch))
				require.Equal(t, uint64(0), uint64(attestationClient.Target.Epoch))
				require.DeepEqual(t, make([]byte, 32), attestationClient.Source.Root)
				require.DeepEqual(t, fixture.GenesisRoot[:], attestationClient.Target.Root)
			case "Canceled", "DeadlineExceeded":
				// A deadline outcome is the timed discriminator, not a fixture failure.
			default:
				t.Fatalf("cold GetAttestationData returned non-deadline status %s: %s", attestationClient.StatusCode, attestationClient.Error)
			}
		} else {
			require.Equal(t, true, attestationClient.Omitted)
			require.NotEqual(t, "", attestationClient.OmitReason)
			require.Equal(t, int64(0), attestationClient.InvokeUnixNano)
			require.Equal(t, int64(0), attestationClient.ReturnUnixNano)
			require.Equal(t, "", attestationClient.StatusCode)
		}
	}

	recorder.mu.Lock()
	serverRecords := append([]fullGossipDomainServerProbe(nil), recorder.records...)
	attestationServer := recorder.attestationData
	if attestationServer != nil {
		copy := *attestationServer
		attestationServer = &copy
	}
	recorder.mu.Unlock()
	headCopy := headCopyFetcher.Record()
	attestationCachePopulated := false
	if !attestationHandlerPending {
		attestationCachePopulated = fullGossipAttestationCachePopulated(attestationDataCache)
	}
	if timedAttestationData && attestationClient.StatusCode == "OK" {
		require.NotNil(t, attestationServer)
		require.NotNil(t, headCopy)
		require.Equal(t, true, attestationCachePopulated)
	}
	if omitTimedAttestationData {
		require.Equal(t, false, attestationHandlerAdmitted)
		require.Equal(t, false, attestationHandlerDone)
		require.Equal(t, false, attestationHandlerPending)
		require.NotNil(t, attestationClient)
		require.Equal(t, true, attestationClient.Omitted)
		require.IsNil(t, attestationServer)
		require.IsNil(t, headCopy)
		require.Equal(t, false, attestationCachePopulated)
	}
	require.NoError(t, writeFullGossipDomainJSON(filepath.Join(outputDirectory, "server.jsonl"), serverRecords))
	require.NoError(t, writeFullGossipDomainJSON(filepath.Join(outputDirectory, "cold-sync.json"), coldSync))
	if timedAttestationData {
		require.NoError(t, writeFullGossipDomainJSON(filepath.Join(outputDirectory, "attestation-data-server.json"), attestationServer))
		require.NoError(t, writeFullGossipDomainJSON(filepath.Join(outputDirectory, "head-copy.json"), headCopy))
	}
	summary := fullGossipDomainSummary{
		Arm: arm, FixtureStartUnixNano: fixtureStarted.UnixNano(), FixtureReadyUnixNano: fixtureReady.UnixNano(),
		FixtureDurationNano: fixtureReady.Sub(fixtureStarted).Nanoseconds(), GenesisUnixNano: fixture.GenesisTime.UnixNano(),
		SlotOneStartUnixNano: slotOneStart.UnixNano(), SlotTwoDeadlineUnixNano: slotTwoDeadline.UnixNano(),
		SetupCompletedUnixNano: fixture.SetupCompleted.UnixNano(), WorkReleasedUnixNano: workReleased.UnixNano(),
		OfferFinishedUnixNano: offer.finished.UnixNano(), SettledUnixNano: settled.UnixNano(),
		SetupBudgetAtReleaseNano: slotTwoDeadline.Sub(workReleased).Nanoseconds(), Published: offer.published,
		PublishErrors: offer.errors, MaximumPublishLatenessNano: offer.maxLateness.Nanoseconds(),
		FinalStats: finalStats, ColdSync: coldSync, WriterComposition: writerComposition,
		TimedAttestationData: timedAttestationData, TimedAttestationDataOmitted: omitTimedAttestationData,
		AttestationCacheCold: attestationCacheCold, AttestationCachePopulated: attestationCachePopulated,
		CanonicalSlotOneRoot: canonicalSlotOneRoot, CanonicalSlotOneFull: canonicalSlotOneFull,
		AttestationHandlerAdmitted: attestationHandlerAdmitted, AttestationHandlerDone: attestationHandlerDone,
		AttestationHandlerPending: attestationHandlerPending,
		AttestationDataPreflight:  attestationPreflight,
		AttestationDataClient:     attestationClient,
		AttestationDataServer:     attestationServer, HeadCopy: headCopy,
		RuntimeTrace: tracePath != "", RuntimeTraceStartUnixNano: traceStarted.UnixNano(),
		RuntimeTraceStopUnixNano: traceStopped.UnixNano(),
		RuntimeNumCPU:            runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
	require.NoError(t, writeFullGossipDomainJSON(filepath.Join(outputDirectory, "summary.json"), summary))
	t.Logf("FULL_GOSSIP_DOMAIN arm=%s writer=%t published=%d stats=%+v cold_sync=%s domain_probes=%d output=%s",
		arm, writerComposition, offer.published, finalStats, time.Duration(coldSync.DurationNano), len(serverRecords), outputDirectory)
}

func waitFullGossipDomainSettled(t *testing.T, fixture *prysmsync.FullGossipDomainDiagnosticFixture) time.Time {
	t.Helper()
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	last := fixture.Stats()
	stableSince := time.Now()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("full gossip validation did not settle: %+v", fixture.Stats())
		case <-ticker.C:
			current := fixture.Stats()
			if current != last {
				last = current
				stableSince = time.Now()
			}
			if current.ActiveIterators == 0 && current.ValidationAccepted == current.SubscriberCompleted && time.Since(stableSince) >= 3*time.Second {
				return time.Now()
			}
		}
	}
}

func writeFullGossipDomainJSON(path string, value any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if records, ok := value.([]fullGossipDomainClientProbe); ok {
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				_ = file.Close()
				return err
			}
		}
		return file.Close()
	}
	if records, ok := value.([]fullGossipDomainServerProbe); ok {
		for _, record := range records {
			if err := encoder.Encode(record); err != nil {
				_ = file.Close()
				return err
			}
		}
		return file.Close()
	}
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readFullGossipDomainJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return json.NewDecoder(file).Decode(value)
}

func readFullGossipDomainClientJSONL(path string) ([]fullGossipDomainClientProbe, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	records := make([]fullGossipDomainClientProbe, 0, fullGossipWriterProbeCount)
	for {
		var record fullGossipDomainClientProbe
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				return records, nil
			}
			return nil, err
		}
		records = append(records, record)
	}
}

func fullGossipAttestationCachePopulated(attestationCache *cache.AttestationDataCache) bool {
	attestationCache.RLock()
	defer attestationCache.RUnlock()
	return attestationCache.Get() != nil
}

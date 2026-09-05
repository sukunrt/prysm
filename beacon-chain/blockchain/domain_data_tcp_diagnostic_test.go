package blockchain_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	rpcvalidator "github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/prysm/v1alpha1/validator"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	domainDataDiagnosticWorkers = 6_144
	domainDataDiagnosticJobs    = 15_000
	domainDataDiagnosticProbes  = 32
	domainDataDiagnosticBudget  = 12 * time.Second
	domainDataDiagnosticArrival = 2 * time.Second
	domainDataDiagnosticBins    = 20
	domainDataStartupInterval   = 250 * time.Millisecond
	domainDataProbeMetadataKey  = "x-prysm-domain-data-diagnostic-probe"
)

type domainDataClientProbe struct {
	Probe               int    `json:"probe"`
	KickReadUnixNano    int64  `json:"kick_read_unix_nano"`
	InvokeUnixNano      int64  `json:"invoke_unix_nano"`
	ReturnUnixNano      int64  `json:"return_unix_nano"`
	DurationNano        int64  `json:"duration_nano"`
	ResponseBytes       int    `json:"response_bytes"`
	ResponseMatchesWarm bool   `json:"response_matches_warm"`
	StatusCode          string `json:"status_code"`
	Error               string `json:"error,omitempty"`
	DeadlineAtReturn    bool   `json:"deadline_at_return"`
	DeadlineUnixNano    int64  `json:"deadline_unix_nano"`
	BudgetAtInvokeNano  int64  `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano  int64  `json:"budget_at_return_nano"`
	PlannedInvokeNano   int64  `json:"planned_invoke_unix_nano"`
	ScheduleLateness    int64  `json:"schedule_lateness_nano"`
	Skipped             bool   `json:"skipped"`
	SkipReason          string `json:"skip_reason,omitempty"`
}

type domainDataServerProbe struct {
	Probe                    int   `json:"probe"`
	AdmissionUnixNano        int64 `json:"admission_unix_nano"`
	HandlerReturnUnixNano    int64 `json:"handler_return_unix_nano"`
	AdmissionToHandlerNano   int64 `json:"admission_to_handler_nano"`
	HandlerDurationNano      int64 `json:"handler_duration_nano"`
	JobsAtAdmission          int64 `json:"jobs_at_admission"`
	JobsAtReturn             int64 `json:"jobs_at_return"`
	ActiveCheckpointsAtStart int64 `json:"active_checkpoints_at_start"`
	ActiveCheckpointsAtEnd   int64 `json:"active_checkpoints_at_end"`
	ActiveScansAtStart       int64 `json:"active_scans_at_start"`
	ActiveScansAtEnd         int64 `json:"active_scans_at_end"`
	CountEntriesAtStart      int64 `json:"count_entries_at_start"`
	CountEntriesAtEnd        int64 `json:"count_entries_at_end"`
	CountExitsAtStart        int64 `json:"count_exits_at_start"`
	CountExitsAtEnd          int64 `json:"count_exits_at_end"`
	ActiveAggregatesAtStart  int64 `json:"active_aggregates_at_start"`
	ActiveAggregatesAtEnd    int64 `json:"active_aggregates_at_end"`
	AggregateEntriesAtStart  int64 `json:"aggregate_entries_at_start"`
	AggregateEntriesAtEnd    int64 `json:"aggregate_entries_at_end"`
	AggregateExitsAtStart    int64 `json:"aggregate_exits_at_start"`
	AggregateExitsAtEnd      int64 `json:"aggregate_exits_at_end"`
}

type domainDataCoordinatorProbe struct {
	Probe                   int   `json:"probe"`
	CompletedThreshold      int64 `json:"completed_threshold"`
	KickUnixNano            int64 `json:"kick_unix_nano"`
	JobsAtKick              int64 `json:"jobs_at_kick"`
	ActiveCheckpointsAtKick int64 `json:"active_checkpoints_at_kick"`
	ActiveScansAtKick       int64 `json:"active_scans_at_kick"`
	CountEntriesAtKick      int64 `json:"count_entries_at_kick"`
	CountExitsAtKick        int64 `json:"count_exits_at_kick"`
	DeadlineUnixNano        int64 `json:"deadline_unix_nano"`
	ActiveAggregatesAtKick  int64 `json:"active_aggregates_at_kick"`
	AggregateEntriesAtKick  int64 `json:"aggregate_entries_at_kick"`
	AggregateExitsAtKick    int64 `json:"aggregate_exits_at_kick"`
}

type domainDataColdSyncProbe struct {
	Slot                    uint64 `json:"slot"`
	ValidatorIndex          uint64 `json:"validator_index"`
	CacheColdBeforeInvoke   bool   `json:"cache_cold_before_invoke"`
	DeadlineUnixNano        int64  `json:"deadline_unix_nano"`
	InvokeUnixNano          int64  `json:"invoke_unix_nano"`
	ReturnUnixNano          int64  `json:"return_unix_nano"`
	DurationNano            int64  `json:"duration_nano"`
	BudgetAtInvokeNano      int64  `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano      int64  `json:"budget_at_return_nano"`
	IndexCount              int    `json:"index_count"`
	Error                   string `json:"error,omitempty"`
	ContextError            string `json:"context_error,omitempty"`
	JobsAtInvoke            int64  `json:"jobs_at_invoke"`
	JobsAtReturn            int64  `json:"jobs_at_return"`
	ActiveCheckpointsInvoke int64  `json:"active_checkpoints_at_invoke"`
	ActiveCheckpointsReturn int64  `json:"active_checkpoints_at_return"`
	ActiveScansInvoke       int64  `json:"active_scans_at_invoke"`
	ActiveScansReturn       int64  `json:"active_scans_at_return"`
	CountEntriesInvoke      int64  `json:"count_entries_at_invoke"`
	CountEntriesReturn      int64  `json:"count_entries_at_return"`
	CountExitsInvoke        int64  `json:"count_exits_at_invoke"`
	CountExitsReturn        int64  `json:"count_exits_at_return"`
	ActiveAggregatesInvoke  int64  `json:"active_aggregates_at_invoke"`
	ActiveAggregatesReturn  int64  `json:"active_aggregates_at_return"`
	AggregateEntriesInvoke  int64  `json:"aggregate_entries_at_invoke"`
	AggregateEntriesReturn  int64  `json:"aggregate_entries_at_return"`
	AggregateExitsInvoke    int64  `json:"aggregate_exits_at_invoke"`
	AggregateExitsReturn    int64  `json:"aggregate_exits_at_return"`
}

type domainDataOfferBin struct {
	Bin                     int   `json:"bin"`
	ScheduledStartUnixNano  int64 `json:"scheduled_start_unix_nano"`
	ScheduledEndUnixNano    int64 `json:"scheduled_end_unix_nano"`
	ScheduledJobs           int64 `json:"scheduled_jobs"`
	FirstOfferedUnixNano    int64 `json:"first_offered_unix_nano"`
	LastOfferedUnixNano     int64 `json:"last_offered_unix_nano"`
	OfferedJobs             int64 `json:"offered_jobs"`
	MaxOfferLatenessNano    int64 `json:"max_offer_lateness_nano"`
	CompletedAtLastOffer    int64 `json:"completed_at_last_offer"`
	ActiveCheckpointsAtLast int64 `json:"active_checkpoints_at_last_offer"`
	ActiveScansAtLast       int64 `json:"active_scans_at_last_offer"`
	CountEntriesAtLast      int64 `json:"count_entries_at_last_offer"`
	CountExitsAtLast        int64 `json:"count_exits_at_last_offer"`
	ActiveAggregatesAtLast  int64 `json:"active_aggregates_at_last_offer"`
	AggregateEntriesAtLast  int64 `json:"aggregate_entries_at_last_offer"`
	AggregateExitsAtLast    int64 `json:"aggregate_exits_at_last_offer"`
}

type domainDataJob struct {
	ScheduledAt time.Time
	OfferedAt   time.Time
}

type domainDataAdmissionEvent struct {
	ScheduledAt      time.Time
	AdmittedAt       time.Time
	OfferedAt        time.Time
	Completed        int64
	ActiveCheckpoint int64
	ActiveScan       int64
	CountEntries     int64
	CountExits       int64
	ActiveAggregate  int64
	AggregateEntries int64
	AggregateExits   int64
}

type domainDataAdmissionBin struct {
	Bin                     int   `json:"bin"`
	IntervalStartUnixNano   int64 `json:"interval_start_unix_nano"`
	IntervalEndUnixNano     int64 `json:"interval_end_unix_nano"`
	AdmittedJobs            int64 `json:"admitted_jobs"`
	FirstAdmissionUnixNano  int64 `json:"first_admission_unix_nano"`
	LastAdmissionUnixNano   int64 `json:"last_admission_unix_nano"`
	MaxOfferToAdmissionNano int64 `json:"max_offer_to_admission_nano"`
	MaxScheduleToAdmission  int64 `json:"max_schedule_to_admission_nano"`
	CompletedAtLast         int64 `json:"completed_at_last_admission"`
	ActiveCheckpointsAtLast int64 `json:"active_checkpoints_at_last_admission"`
	ActiveScansAtLast       int64 `json:"active_scans_at_last_admission"`
	CountEntriesAtLast      int64 `json:"count_entries_at_last_admission"`
	CountExitsAtLast        int64 `json:"count_exits_at_last_admission"`
	ActiveAggregatesAtLast  int64 `json:"active_aggregates_at_last_admission"`
	AggregateEntriesAtLast  int64 `json:"aggregate_entries_at_last_admission"`
	AggregateExitsAtLast    int64 `json:"aggregate_exits_at_last_admission"`
}

type domainDataJoinedProbe struct {
	Probe                     int    `json:"probe"`
	ClientDurationNano        int64  `json:"client_duration_nano"`
	KickToInvokeNano          int64  `json:"kick_to_invoke_nano"`
	InvokeToAdmissionNano     int64  `json:"invoke_to_admission_nano,omitempty"`
	HandlerDurationNano       int64  `json:"handler_duration_nano,omitempty"`
	HandlerReturnToClientNano int64  `json:"handler_return_to_client_nano,omitempty"`
	ServerAdmitted            bool   `json:"server_admitted"`
	LoadedAtAdmission         bool   `json:"loaded_at_admission"`
	JobsAtAdmission           int64  `json:"jobs_at_admission,omitempty"`
	ActiveCheckpoints         int64  `json:"active_checkpoints_at_admission,omitempty"`
	ActiveScans               int64  `json:"active_scans_at_admission,omitempty"`
	ActiveAggregates          int64  `json:"active_aggregates_at_admission,omitempty"`
	AggregateEntries          int64  `json:"aggregate_entries_at_admission,omitempty"`
	AggregateExits            int64  `json:"aggregate_exits_at_admission,omitempty"`
	StatusCode                string `json:"status_code"`
	Error                     string `json:"error,omitempty"`
	DeadlineUnixNano          int64  `json:"deadline_unix_nano"`
	BudgetAtInvokeNano        int64  `json:"budget_at_invoke_nano"`
	BudgetAtReturnNano        int64  `json:"budget_at_return_nano"`
	PlannedInvokeNano         int64  `json:"planned_invoke_unix_nano"`
	ScheduleLateness          int64  `json:"schedule_lateness_nano"`
	Skipped                   bool   `json:"skipped"`
	SkipReason                string `json:"skip_reason,omitempty"`
}

type domainDataServerRecorder struct {
	completed         *atomic.Int64
	activeCheckpoints *atomic.Int64
	activeScans       *atomic.Int64
	countEntries      *atomic.Int64
	countExits        *atomic.Int64
	activeAggregates  *atomic.Int64
	aggregateEntries  *atomic.Int64
	aggregateExits    *atomic.Int64
	mu                sync.Mutex
	records           []domainDataServerProbe
}

func (r *domainDataServerRecorder) interceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(domainDataProbeMetadataKey)
	probe := -1
	if len(values) == 1 {
		probe, _ = strconv.Atoi(values[0])
	}
	started := time.Now()
	record := domainDataServerProbe{
		Probe: probe, AdmissionUnixNano: started.UnixNano(),
		JobsAtAdmission: r.completed.Load(), ActiveCheckpointsAtStart: r.activeCheckpoints.Load(),
		ActiveScansAtStart: r.activeScans.Load(), CountEntriesAtStart: r.countEntries.Load(),
		CountExitsAtStart: r.countExits.Load(), ActiveAggregatesAtStart: r.activeAggregates.Load(),
		AggregateEntriesAtStart: r.aggregateEntries.Load(), AggregateExitsAtStart: r.aggregateExits.Load(),
	}
	handlerStarted := time.Now()
	response, err := handler(ctx, req)
	returned := time.Now()
	if probe >= 0 {
		record.HandlerReturnUnixNano = returned.UnixNano()
		record.AdmissionToHandlerNano = handlerStarted.Sub(started).Nanoseconds()
		record.HandlerDurationNano = returned.Sub(handlerStarted).Nanoseconds()
		record.JobsAtReturn = r.completed.Load()
		record.ActiveCheckpointsAtEnd = r.activeCheckpoints.Load()
		record.ActiveScansAtEnd = r.activeScans.Load()
		record.CountEntriesAtEnd = r.countEntries.Load()
		record.CountExitsAtEnd = r.countExits.Load()
		record.ActiveAggregatesAtEnd = r.activeAggregates.Load()
		record.AggregateEntriesAtEnd = r.aggregateEntries.Load()
		record.AggregateExitsAtEnd = r.aggregateExits.Load()
		r.mu.Lock()
		r.records = append(r.records, record)
		r.mu.Unlock()
	}
	return response, err
}

func (r *domainDataServerRecorder) snapshot() []domainDataServerProbe {
	r.mu.Lock()
	defer r.mu.Unlock()
	records := append([]domainDataServerProbe(nil), r.records...)
	sort.Slice(records, func(i, j int) bool { return records[i].Probe < records[j].Probe })
	return records
}

func writeDomainDataJSONLines(path string, records any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	switch values := records.(type) {
	case []domainDataClientProbe:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataServerProbe:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataJoinedProbe:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataCoordinatorProbe:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataOfferBin:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataAdmissionBin:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	case []domainDataColdSyncProbe:
		for _, value := range values {
			if err := encoder.Encode(value); err != nil {
				_ = file.Close()
				return err
			}
		}
	default:
		_ = file.Close()
		return fmt.Errorf("unsupported JSONL record type %T", records)
	}
	return file.Close()
}

func readDomainDataClientProbes(path string) ([]domainDataClientProbe, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []domainDataClientProbe
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record domainDataClientProbe
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func domainDataPercentile(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*percentile + 99) / 100
	if index == 0 {
		index = 1
	}
	return sorted[index-1]
}

func recordDomainDataMaximum(maximum *atomic.Int64, value int64) {
	for current := maximum.Load(); value > current; current = maximum.Load() {
		if maximum.CompareAndSwap(current, value) {
			return
		}
	}
}

func offerDomainDataJobs(
	start time.Time,
	jobCh chan<- domainDataJob,
	completed, activeCheckpoints, activeScans, countEntries, countExits *atomic.Int64,
	activeAggregates, aggregateEntries, aggregateExits *atomic.Int64,
	offered *atomic.Int64,
) []domainDataOfferBin {
	bins := make([]domainDataOfferBin, domainDataDiagnosticBins)
	for bin := range bins {
		bins[bin] = domainDataOfferBin{
			Bin:                    bin,
			ScheduledStartUnixNano: start.Add(domainDataDiagnosticArrival * time.Duration(bin) / domainDataDiagnosticBins).UnixNano(),
			ScheduledEndUnixNano:   start.Add(domainDataDiagnosticArrival * time.Duration(bin+1) / domainDataDiagnosticBins).UnixNano(),
		}
	}
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	for job := range domainDataDiagnosticJobs {
		scheduled := start.Add(domainDataDiagnosticArrival * time.Duration(job) / domainDataDiagnosticJobs)
		if wait := time.Until(scheduled); wait > 0 {
			timer.Reset(wait)
			<-timer.C
		}
		offeredAt := time.Now()
		jobCh <- domainDataJob{ScheduledAt: scheduled, OfferedAt: offeredAt}
		offered.Add(1)
		bin := job * domainDataDiagnosticBins / domainDataDiagnosticJobs
		record := &bins[bin]
		record.ScheduledJobs++
		record.OfferedJobs++
		if record.FirstOfferedUnixNano == 0 {
			record.FirstOfferedUnixNano = offeredAt.UnixNano()
		}
		record.LastOfferedUnixNano = offeredAt.UnixNano()
		lateness := offeredAt.Sub(scheduled).Nanoseconds()
		if lateness > record.MaxOfferLatenessNano {
			record.MaxOfferLatenessNano = lateness
		}
		record.CompletedAtLastOffer = completed.Load()
		record.ActiveCheckpointsAtLast = activeCheckpoints.Load()
		record.ActiveScansAtLast = activeScans.Load()
		record.CountEntriesAtLast = countEntries.Load()
		record.CountExitsAtLast = countExits.Load()
		record.ActiveAggregatesAtLast = activeAggregates.Load()
		record.AggregateEntriesAtLast = aggregateEntries.Load()
		record.AggregateExitsAtLast = aggregateExits.Load()
	}
	close(jobCh)
	return bins
}

func summarizeDomainDataAdmissions(start time.Time, events []domainDataAdmissionEvent) []domainDataAdmissionBin {
	const binWidth = 100 * time.Millisecond
	byBin := make(map[int]*domainDataAdmissionBin)
	for _, event := range events {
		bin := int(event.AdmittedAt.Sub(start) / binWidth)
		record := byBin[bin]
		if record == nil {
			record = &domainDataAdmissionBin{
				Bin:                   bin,
				IntervalStartUnixNano: start.Add(time.Duration(bin) * binWidth).UnixNano(),
				IntervalEndUnixNano:   start.Add(time.Duration(bin+1) * binWidth).UnixNano(),
			}
			byBin[bin] = record
		}
		record.AdmittedJobs++
		if record.FirstAdmissionUnixNano == 0 {
			record.FirstAdmissionUnixNano = event.AdmittedAt.UnixNano()
		}
		record.LastAdmissionUnixNano = event.AdmittedAt.UnixNano()
		delay := event.AdmittedAt.Sub(event.OfferedAt).Nanoseconds()
		if delay > record.MaxOfferToAdmissionNano {
			record.MaxOfferToAdmissionNano = delay
		}
		scheduleDelay := event.AdmittedAt.Sub(event.ScheduledAt).Nanoseconds()
		if scheduleDelay > record.MaxScheduleToAdmission {
			record.MaxScheduleToAdmission = scheduleDelay
		}
		record.CompletedAtLast = event.Completed
		record.ActiveCheckpointsAtLast = event.ActiveCheckpoint
		record.ActiveScansAtLast = event.ActiveScan
		record.CountEntriesAtLast = event.CountEntries
		record.CountExitsAtLast = event.CountExits
		record.ActiveAggregatesAtLast = event.ActiveAggregate
		record.AggregateEntriesAtLast = event.AggregateEntries
		record.AggregateExitsAtLast = event.AggregateExits
	}
	bins := make([]domainDataAdmissionBin, 0, len(byBin))
	for _, record := range byBin {
		bins = append(bins, *record)
	}
	sort.Slice(bins, func(i, j int) bool { return bins[i].Bin < bins[j].Bin })
	return bins
}

func TestDiagnosticDomainDataTCPClientProcess(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_CHILD") != "1" {
		t.Skip("external diagnostic client only")
	}
	address := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_ADDRESS")
	resultPath := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_CLIENT_RESULTS")
	require.NotEqual(t, "", address)
	require.NotEqual(t, "", resultPath)

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	request := &ethpb.DomainRequest{Epoch: 0, Domain: params.BeaconConfig().DomainRandao[:]}
	warmCtx, cancelWarm := context.WithTimeout(t.Context(), domainDataDiagnosticBudget)
	warmResponse, err := client.DomainData(warmCtx, request)
	cancelWarm()
	require.NoError(t, err)
	require.Equal(t, 32, len(warmResponse.SignatureDomain))

	ready := os.NewFile(uintptr(3), "domain-data-ready")
	require.NotNil(t, ready)
	_, err = ready.WriteString("READY\n")
	require.NoError(t, err)
	require.NoError(t, ready.Close())

	startupClock := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_STARTUP_CLOCK") == "1"
	var startupStart time.Time
	var firstDeadline time.Time
	if startupClock {
		var header [16]byte
		_, err = io.ReadFull(os.Stdin, header[:])
		require.NoError(t, err)
		startupStart = time.Unix(0, int64(binary.LittleEndian.Uint64(header[:8])))
		firstDeadline = time.Unix(0, int64(binary.LittleEndian.Uint64(header[8:])))
	}
	records := make([]domainDataClientProbe, 0, domainDataDiagnosticProbes)
	for probe := range domainDataDiagnosticProbes {
		var planned time.Time
		var deadline time.Time
		kickReadAt := time.Now()
		if startupClock {
			planned = startupStart.Add(time.Duration(probe) * domainDataStartupInterval)
			if probe > 0 {
				if !time.Now().Before(planned) {
					records = append(records, domainDataClientProbe{
						Probe: probe, PlannedInvokeNano: planned.UnixNano(), Skipped: true,
						SkipReason: "planned startup sample elapsed before prior sequential work returned",
					})
					continue
				}
				timer := time.NewTimer(time.Until(planned))
				<-timer.C
			}
			kickReadAt = time.Now()
			deadline = kickReadAt.Add(domainDataDiagnosticBudget)
			if probe == 0 {
				deadline = firstDeadline
			}
		} else {
			var deadlineBytes [8]byte
			_, err = io.ReadFull(os.Stdin, deadlineBytes[:])
			require.NoError(t, err)
			kickReadAt = time.Now()
			deadlineUnixNano := int64(binary.LittleEndian.Uint64(deadlineBytes[:]))
			deadline = kickReadAt.Add(domainDataDiagnosticBudget)
			if deadlineUnixNano != 0 {
				deadline = time.Unix(0, deadlineUnixNano)
			}
		}
		probeCtx, cancel := context.WithDeadline(t.Context(), deadline)
		probeCtx = metadata.AppendToOutgoingContext(probeCtx, domainDataProbeMetadataKey, strconv.Itoa(probe))
		started := time.Now()
		response, callErr := client.DomainData(probeCtx, request)
		returned := time.Now()
		ctxErr := probeCtx.Err()
		cancel()
		record := domainDataClientProbe{
			Probe: probe, KickReadUnixNano: kickReadAt.UnixNano(), InvokeUnixNano: started.UnixNano(),
			ReturnUnixNano: returned.UnixNano(), DurationNano: returned.Sub(started).Nanoseconds(),
			StatusCode: status.Code(callErr).String(), DeadlineAtReturn: ctxErr != nil,
			DeadlineUnixNano: deadline.UnixNano(), BudgetAtInvokeNano: deadline.Sub(started).Nanoseconds(),
			BudgetAtReturnNano: deadline.Sub(returned).Nanoseconds(),
		}
		if startupClock {
			record.PlannedInvokeNano = planned.UnixNano()
			record.ScheduleLateness = started.Sub(planned).Nanoseconds()
		}
		if response != nil {
			record.ResponseBytes = len(response.SignatureDomain)
			record.ResponseMatchesWarm = bytes.Equal(response.SignatureDomain, warmResponse.SignatureDomain)
		}
		if callErr != nil {
			record.Error = callErr.Error()
		}
		records = append(records, record)
	}
	require.NoError(t, writeDomainDataJSONLines(resultPath, records))
	t.Logf("DOMAIN_DATA_CLIENT probes=%d warm_response_bytes=%d result_path=%s", len(records), len(warmResponse.SignatureDomain), resultPath)
}

func TestDiagnosticDomainDataTCPUnderCheckpointCountLoad(t *testing.T) {
	arm := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_ARM")
	if arm == "" {
		t.Skip("PRYSM_DIAGNOSTIC_DOMAIN_ARM is not set")
	}
	if arm != "scan" && arm != "memoized" {
		t.Fatalf("unknown domain-data count arm %q", arm)
	}
	arrivalMode := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_ARRIVAL")
	if arrivalMode == "" {
		arrivalMode = "burst"
	}
	if arrivalMode != "burst" && arrivalMode != "paced" {
		t.Fatalf("unknown domain-data arrival mode %q", arrivalMode)
	}
	coldSyncMode := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_COLD_SYNC")
	if coldSyncMode == "" {
		coldSyncMode = "0"
	}
	if coldSyncMode != "0" && coldSyncMode != "1" {
		t.Fatalf("unknown cold-sync mode %q", coldSyncMode)
	}
	singletonAggregateMode := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_SINGLETON_AGG")
	if singletonAggregateMode == "" {
		singletonAggregateMode = "0"
	}
	if singletonAggregateMode != "0" && singletonAggregateMode != "1" {
		t.Fatalf("unknown singleton-aggregate mode %q", singletonAggregateMode)
	}
	if singletonAggregateMode == "1" && (arrivalMode != "paced" || coldSyncMode != "1") {
		t.Fatal("singleton-aggregate mode requires paced arrival and the cold sync-index prerequisite")
	}
	outputDir := os.Getenv("PRYSM_DIAGNOSTIC_DOMAIN_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = filepath.Join(os.TempDir(), "prysm-domain-data-"+arm)
	}
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	fixtureStarted := time.Now()
	fixture := blockchain.NewDomainDataDiagnosticFixture(t)
	fixtureElapsed := time.Since(fixtureStarted)
	preflightState, err := fixture.Service.AttestationTargetState(t.Context(), fixture.Checkpoint)
	require.NoError(t, err)
	preflightCount, err := helpers.ActiveValidatorCount(t.Context(), preflightState, 0)
	require.NoError(t, err)
	require.Equal(t, fixture.MemoizedCount, preflightCount)
	require.Equal(t, true, fixture.SyncSlot1Cold)
	require.Equal(t, params.BeaconConfig().BLSPubkeyLength, len(fixture.SingletonKey))

	var completed atomic.Int64
	var offered atomic.Int64
	var activeCheckpoints atomic.Int64
	var activeScans atomic.Int64
	var countEntries atomic.Int64
	var countExits atomic.Int64
	var activeAggregates atomic.Int64
	var maxActiveAggregates atomic.Int64
	var aggregateEntries atomic.Int64
	var aggregateExits atomic.Int64
	var maxActiveCheckpoints atomic.Int64
	var maxActiveScans atomic.Int64
	jobCh := make(chan domainDataJob, domainDataDiagnosticJobs)
	if arrivalMode == "burst" {
		offeredAt := time.Now()
		for range domainDataDiagnosticJobs {
			jobCh <- domainDataJob{OfferedAt: offeredAt}
		}
		offered.Store(domainDataDiagnosticJobs)
		close(jobCh)
	}
	start := make(chan struct{})
	progress := make(chan struct{}, 1)
	workerErrs := make(chan error, 1)
	var workerWG sync.WaitGroup
	var readyWG sync.WaitGroup
	admissionEvents := make(chan domainDataAdmissionEvent, domainDataDiagnosticJobs)
	recordWorkerError := func(workerErr error) {
		select {
		case workerErrs <- workerErr:
		default:
		}
	}
	workerWG.Add(domainDataDiagnosticWorkers)
	readyWG.Add(domainDataDiagnosticWorkers)
	for range domainDataDiagnosticWorkers {
		go func() {
			defer workerWG.Done()
			readyWG.Done()
			<-start
			for job := range jobCh {
				if arrivalMode == "paced" {
					admittedAt := time.Now()
					admissionEvents <- domainDataAdmissionEvent{
						ScheduledAt: job.ScheduledAt, AdmittedAt: admittedAt, OfferedAt: job.OfferedAt, Completed: completed.Load(),
						ActiveCheckpoint: activeCheckpoints.Load(), ActiveScan: activeScans.Load(),
						CountEntries: countEntries.Load(), CountExits: countExits.Load(),
						ActiveAggregate: activeAggregates.Load(), AggregateEntries: aggregateEntries.Load(),
						AggregateExits: aggregateExits.Load(),
					}
				}
				activeCheckpointCount := activeCheckpoints.Add(1)
				recordDomainDataMaximum(&maxActiveCheckpoints, activeCheckpointCount)
				st, workerErr := fixture.Service.AttestationTargetState(context.Background(), fixture.Checkpoint)
				activeCheckpoints.Add(-1)
				count := fixture.MemoizedCount
				if workerErr == nil && arm == "scan" {
					activeScanCount := activeScans.Add(1)
					recordDomainDataMaximum(&maxActiveScans, activeScanCount)
					countEntries.Add(1)
					count, workerErr = helpers.ActiveValidatorCount(context.Background(), st, 0)
					countExits.Add(1)
					activeScans.Add(-1)
				}
				if workerErr == nil && singletonAggregateMode == "1" {
					activeAggregateCount := activeAggregates.Add(1)
					recordDomainDataMaximum(&maxActiveAggregates, activeAggregateCount)
					aggregateEntries.Add(1)
					aggregateKey, aggregateErr := st.AggregateKeyFromIndices([]uint64{0})
					aggregateExits.Add(1)
					activeAggregates.Add(-1)
					if aggregateErr != nil {
						workerErr = aggregateErr
					} else if !bytes.Equal(aggregateKey.Marshal(), fixture.SingletonKey) {
						workerErr = errors.New("singleton aggregate did not equal validator 0 public key")
					}
				}
				if workerErr == nil && count != fixture.MemoizedCount {
					workerErr = fmt.Errorf("active validator count %d, want %d", count, fixture.MemoizedCount)
				}
				if workerErr != nil {
					recordWorkerError(workerErr)
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
	readyWG.Wait()

	recorder := &domainDataServerRecorder{
		completed: &completed, activeCheckpoints: &activeCheckpoints, activeScans: &activeScans,
		countEntries: &countEntries, countExits: &countExits,
		activeAggregates: &activeAggregates, aggregateEntries: &aggregateEntries, aggregateExits: &aggregateExits,
		records: make([]domainDataServerProbe, 0, domainDataDiagnosticProbes),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(recorder.interceptor))
	ethpb.RegisterBeaconNodeValidatorServer(grpcServer, &rpcvalidator.Server{
		Ctx: t.Context(), HeadFetcher: fixture.Service,
	})
	serveDone := make(chan error, 1)
	go func() { serveDone <- grpcServer.Serve(listener) }()
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
	clientResultPath := filepath.Join(outputDir, "client.jsonl")
	clientTestLogPath := filepath.Join(outputDir, "client.test.log")
	clientTestLog, err := os.Create(clientTestLogPath)
	require.NoError(t, err)
	cmd := exec.Command(executable,
		"-test.run", "^TestDiagnosticDomainDataTCPClientProcess$", "-test.v", "-test.timeout=8m")
	cmd.Env = append(os.Environ(),
		"PRYSM_DIAGNOSTIC_DOMAIN_CHILD=1",
		"PRYSM_DIAGNOSTIC_DOMAIN_ADDRESS="+listener.Addr().String(),
		"PRYSM_DIAGNOSTIC_DOMAIN_CLIENT_RESULTS="+clientResultPath,
		"PRYSM_DIAGNOSTIC_DOMAIN_STARTUP_CLOCK="+singletonAggregateMode,
	)
	cmd.Stdin = triggerRead
	cmd.Stdout = clientTestLog
	cmd.Stderr = clientTestLog
	cmd.ExtraFiles = []*os.File{readyWrite}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	require.NoError(t, triggerRead.Close())
	require.NoError(t, readyWrite.Close())
	clientDone := make(chan error, 1)
	go func() { clientDone <- cmd.Wait() }()
	readyLine := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(readyRead).ReadString('\n')
		readyLine <- line
	}()
	select {
	case line := <-readyLine:
		require.Equal(t, "READY\n", line)
	case childErr := <-clientDone:
		t.Fatalf("external client exited before ready: %v", childErr)
	case <-time.After(30 * time.Second):
		t.Fatal("external client did not establish and warm its connection within 30s")
	}
	require.NoError(t, readyRead.Close())

	workReleasedAt := time.Now()
	close(start)
	offerRecordsDone := make(chan []domainDataOfferBin, 1)
	if arrivalMode == "paced" {
		go func() {
			offerRecordsDone <- offerDomainDataJobs(
				workReleasedAt, jobCh, &completed, &activeCheckpoints, &activeScans, &countEntries, &countExits,
				&activeAggregates, &aggregateEntries, &aggregateExits, &offered,
			)
		}()
	} else {
		offerRecordsDone <- nil
	}
	workFinished := make(chan time.Time, 1)
	go func() {
		workerWG.Wait()
		workFinished <- time.Now()
	}()
	coordinatorRecords := make([]domainDataCoordinatorProbe, 0, domainDataDiagnosticProbes)
	coldSyncRecords := make([]domainDataColdSyncProbe, 0, 1)
	var coldSyncUnexpectedErr error
	waitForThreshold := func(probe int, threshold int64) {
		progressTimeout := time.NewTimer(5 * time.Minute)
		for completed.Load() < threshold {
			select {
			case <-progress:
			case <-progressTimeout.C:
				t.Fatalf("checkpoint/count workload did not reach probe %d threshold %d; completed=%d", probe, threshold, completed.Load())
			}
		}
		if !progressTimeout.Stop() {
			select {
			case <-progressTimeout.C:
			default:
			}
		}
	}
	runColdSync := func(deadline time.Time) {
		if coldSyncMode == "1" {
			syncCtx, cancelSync := context.WithDeadline(t.Context(), deadline)
			invoked := time.Now()
			record := domainDataColdSyncProbe{
				Slot: 1, ValidatorIndex: 0, CacheColdBeforeInvoke: fixture.SyncSlot1Cold,
				DeadlineUnixNano: deadline.UnixNano(), InvokeUnixNano: invoked.UnixNano(),
				BudgetAtInvokeNano: deadline.Sub(invoked).Nanoseconds(), JobsAtInvoke: completed.Load(),
				ActiveCheckpointsInvoke: activeCheckpoints.Load(), ActiveScansInvoke: activeScans.Load(),
				CountEntriesInvoke: countEntries.Load(), CountExitsInvoke: countExits.Load(),
				ActiveAggregatesInvoke: activeAggregates.Load(), AggregateEntriesInvoke: aggregateEntries.Load(),
				AggregateExitsInvoke: aggregateExits.Load(),
			}
			indices, syncErr := fixture.Service.HeadSyncCommitteeIndices(syncCtx, 0, primitives.Slot(1))
			returned := time.Now()
			syncContextErr := syncCtx.Err()
			cancelSync()
			record.ReturnUnixNano = returned.UnixNano()
			record.DurationNano = returned.Sub(invoked).Nanoseconds()
			record.BudgetAtReturnNano = deadline.Sub(returned).Nanoseconds()
			record.IndexCount = len(indices)
			record.JobsAtReturn = completed.Load()
			record.ActiveCheckpointsReturn = activeCheckpoints.Load()
			record.ActiveScansReturn = activeScans.Load()
			record.CountEntriesReturn = countEntries.Load()
			record.CountExitsReturn = countExits.Load()
			record.ActiveAggregatesReturn = activeAggregates.Load()
			record.AggregateEntriesReturn = aggregateEntries.Load()
			record.AggregateExitsReturn = aggregateExits.Load()
			if syncErr != nil {
				record.Error = syncErr.Error()
			}
			if syncContextErr != nil {
				record.ContextError = syncContextErr.Error()
			}
			coldSyncRecords = append(coldSyncRecords, record)
			if syncErr != nil && !errors.Is(syncErr, context.Canceled) && !errors.Is(syncErr, context.DeadlineExceeded) {
				coldSyncUnexpectedErr = syncErr
			}
			if syncErr == nil && len(indices) == 0 {
				coldSyncUnexpectedErr = errors.New("cold sync-index call returned no committee positions")
			}
		}
	}
	recordCoordinator := func(probe int, threshold, deadlineUnixNano int64) {
		coordinatorRecords = append(coordinatorRecords, domainDataCoordinatorProbe{
			Probe: probe, CompletedThreshold: threshold, KickUnixNano: time.Now().UnixNano(),
			JobsAtKick: completed.Load(), ActiveCheckpointsAtKick: activeCheckpoints.Load(),
			ActiveScansAtKick: activeScans.Load(), CountEntriesAtKick: countEntries.Load(),
			CountExitsAtKick: countExits.Load(), DeadlineUnixNano: deadlineUnixNano,
			ActiveAggregatesAtKick: activeAggregates.Load(), AggregateEntriesAtKick: aggregateEntries.Load(),
			AggregateExitsAtKick: aggregateExits.Load(),
		})
	}
	if singletonAggregateMode == "1" {
		waitForThreshold(0, 1)
		deadline := workReleasedAt.Add(domainDataDiagnosticBudget)
		runColdSync(deadline)
		recordCoordinator(0, 1, deadline.UnixNano())
		var startupHeader [16]byte
		binary.LittleEndian.PutUint64(startupHeader[:8], uint64(workReleasedAt.UnixNano()))
		binary.LittleEndian.PutUint64(startupHeader[8:], uint64(deadline.UnixNano()))
		_, err = triggerWrite.Write(startupHeader[:])
		require.NoError(t, err)
	} else {
		for probe := range domainDataDiagnosticProbes {
			threshold := int64(probe * domainDataDiagnosticJobs / domainDataDiagnosticProbes)
			if threshold < 1 {
				threshold = 1
			}
			waitForThreshold(probe, threshold)
			deadlineUnixNano := int64(0)
			if probe == 0 && coldSyncMode == "1" {
				deadline := workReleasedAt.Add(domainDataDiagnosticBudget)
				deadlineUnixNano = deadline.UnixNano()
				runColdSync(deadline)
			}
			recordCoordinator(probe, threshold, deadlineUnixNano)
			var deadlineBytes [8]byte
			binary.LittleEndian.PutUint64(deadlineBytes[:], uint64(deadlineUnixNano))
			_, err = triggerWrite.Write(deadlineBytes[:])
			require.NoError(t, err)
		}
	}
	require.NoError(t, triggerWrite.Close())

	var childErr error
	select {
	case childErr = <-clientDone:
	case <-time.After(7 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("external client exceeded the bounded 7m wait")
	}
	clientReturnedAt := time.Now()
	require.NoError(t, clientTestLog.Close())
	var workCompletedAt time.Time
	select {
	case workCompletedAt = <-workFinished:
	case <-time.After(5 * time.Minute):
		t.Fatal("checkpoint/count workers did not drain within 5m")
	}
	close(admissionEvents)
	offerRecords := <-offerRecordsDone
	admissionEventRecords := make([]domainDataAdmissionEvent, 0, domainDataDiagnosticJobs)
	for event := range admissionEvents {
		admissionEventRecords = append(admissionEventRecords, event)
	}
	admissionRecords := summarizeDomainDataAdmissions(workReleasedAt, admissionEventRecords)
	select {
	case workerErr := <-workerErrs:
		t.Fatalf("checkpoint/count worker failed: %v", workerErr)
	default:
	}
	require.Equal(t, int64(domainDataDiagnosticJobs), completed.Load())
	require.Equal(t, int64(domainDataDiagnosticJobs), offered.Load())
	if arrivalMode == "paced" {
		require.Equal(t, domainDataDiagnosticBins, len(offerRecords))
		require.Equal(t, domainDataDiagnosticJobs, len(admissionEventRecords))
	}
	if arm == "scan" {
		require.Equal(t, int64(domainDataDiagnosticJobs), countEntries.Load())
		require.Equal(t, int64(domainDataDiagnosticJobs), countExits.Load())
	} else {
		require.Equal(t, int64(0), countEntries.Load())
		require.Equal(t, int64(0), countExits.Load())
	}
	if singletonAggregateMode == "1" {
		require.Equal(t, int64(domainDataDiagnosticJobs), aggregateEntries.Load())
		require.Equal(t, int64(domainDataDiagnosticJobs), aggregateExits.Load())
	} else {
		require.Equal(t, int64(0), aggregateEntries.Load())
		require.Equal(t, int64(0), aggregateExits.Load())
	}
	require.NoError(t, childErr)

	clientRecords, err := readDomainDataClientProbes(clientResultPath)
	require.NoError(t, err)
	require.Equal(t, domainDataDiagnosticProbes, len(clientRecords))
	serverRecords := recorder.snapshot()
	serverByProbe := make(map[int]domainDataServerProbe, len(serverRecords))
	for _, record := range serverRecords {
		serverByProbe[record.Probe] = record
	}
	joined := make([]domainDataJoinedProbe, 0, len(clientRecords))
	clientDurations := make([]int64, 0, len(clientRecords))
	preBodyDurations := make([]int64, 0, len(serverRecords))
	postBodyDurations := make([]int64, 0, len(serverRecords))
	successes := 0
	deadlines := 0
	skipped := 0
	loadedAdmissions := 0
	for _, clientRecord := range clientRecords {
		joinedRecord := domainDataJoinedProbe{
			Probe: clientRecord.Probe, ClientDurationNano: clientRecord.DurationNano,
			KickToInvokeNano: clientRecord.InvokeUnixNano - clientRecord.KickReadUnixNano,
			StatusCode:       clientRecord.StatusCode, Error: clientRecord.Error,
			DeadlineUnixNano: clientRecord.DeadlineUnixNano, BudgetAtInvokeNano: clientRecord.BudgetAtInvokeNano,
			BudgetAtReturnNano: clientRecord.BudgetAtReturnNano,
			PlannedInvokeNano:  clientRecord.PlannedInvokeNano, ScheduleLateness: clientRecord.ScheduleLateness,
			Skipped: clientRecord.Skipped, SkipReason: clientRecord.SkipReason,
		}
		if clientRecord.Skipped {
			skipped++
		} else {
			clientDurations = append(clientDurations, clientRecord.DurationNano)
		}
		if clientRecord.StatusCode == "OK" {
			successes++
			require.Equal(t, 32, clientRecord.ResponseBytes)
			require.Equal(t, true, clientRecord.ResponseMatchesWarm)
		} else if clientRecord.StatusCode == "DeadlineExceeded" {
			deadlines++
		}
		if serverRecord, ok := serverByProbe[clientRecord.Probe]; ok {
			joinedRecord.ServerAdmitted = true
			joinedRecord.InvokeToAdmissionNano = serverRecord.AdmissionUnixNano - clientRecord.InvokeUnixNano
			joinedRecord.HandlerDurationNano = serverRecord.HandlerDurationNano
			joinedRecord.HandlerReturnToClientNano = clientRecord.ReturnUnixNano - serverRecord.HandlerReturnUnixNano
			joinedRecord.JobsAtAdmission = serverRecord.JobsAtAdmission
			joinedRecord.ActiveCheckpoints = serverRecord.ActiveCheckpointsAtStart
			joinedRecord.ActiveScans = serverRecord.ActiveScansAtStart
			joinedRecord.ActiveAggregates = serverRecord.ActiveAggregatesAtStart
			joinedRecord.AggregateEntries = serverRecord.AggregateEntriesAtStart
			joinedRecord.AggregateExits = serverRecord.AggregateExitsAtStart
			joinedRecord.LoadedAtAdmission = serverRecord.JobsAtAdmission < domainDataDiagnosticJobs &&
				(serverRecord.ActiveCheckpointsAtStart > 0 || serverRecord.ActiveScansAtStart > 0 || serverRecord.ActiveAggregatesAtStart > 0)
			if joinedRecord.LoadedAtAdmission {
				loadedAdmissions++
			}
			preBodyDurations = append(preBodyDurations, joinedRecord.InvokeToAdmissionNano)
			postBodyDurations = append(postBodyDurations, joinedRecord.HandlerReturnToClientNano)
		}
		joined = append(joined, joinedRecord)
	}

	serverPath := filepath.Join(outputDir, "server.jsonl")
	joinedPath := filepath.Join(outputDir, "joined.jsonl")
	coordinatorPath := filepath.Join(outputDir, "coordinator.jsonl")
	offersPath := filepath.Join(outputDir, "offers.jsonl")
	admissionsPath := filepath.Join(outputDir, "admissions.jsonl")
	coldSyncPath := filepath.Join(outputDir, "cold-sync.jsonl")
	require.NoError(t, writeDomainDataJSONLines(serverPath, serverRecords))
	require.NoError(t, writeDomainDataJSONLines(joinedPath, joined))
	require.NoError(t, writeDomainDataJSONLines(coordinatorPath, coordinatorRecords))
	require.NoError(t, writeDomainDataJSONLines(offersPath, offerRecords))
	require.NoError(t, writeDomainDataJSONLines(admissionsPath, admissionRecords))
	require.NoError(t, writeDomainDataJSONLines(coldSyncPath, coldSyncRecords))
	firstKick := coordinatorRecords[0]
	summary := map[string]any{
		"arm": arm, "arrival_mode": arrivalMode, "cold_sync_mode": coldSyncMode, "workers": domainDataDiagnosticWorkers, "jobs": domainDataDiagnosticJobs,
		"offered_jobs": offered.Load(), "arrival_window_ms": domainDataDiagnosticArrival.Milliseconds(),
		"completed_jobs": completed.Load(), "probes": len(clientRecords), "server_admissions": len(serverRecords),
		"successful_probes": successes, "deadline_probes": deadlines, "skipped_probes": skipped, "loaded_admissions": loadedAdmissions,
		"checkpoint_slot": uint64(preflightState.Slot()), "checkpoint_active_validators": preflightCount,
		"slots_per_round":    params.BeaconConfig().SlotsPerRound,
		"fixture_elapsed_ms": fixtureElapsed.Milliseconds(), "work_released_unix_nano": workReleasedAt.UnixNano(),
		"first_kick_unix_nano": firstKick.KickUnixNano, "client_returned_unix_nano": clientReturnedAt.UnixNano(),
		"work_completed_unix_nano": workCompletedAt.UnixNano(), "jobs_at_first_kick": firstKick.JobsAtKick,
		"active_checkpoints_at_first_kick": firstKick.ActiveCheckpointsAtKick, "active_scans_at_first_kick": firstKick.ActiveScansAtKick,
		"max_active_checkpoints": maxActiveCheckpoints.Load(), "max_active_scans": maxActiveScans.Load(),
		"count_entries": countEntries.Load(), "count_exits": countExits.Load(),
		"singleton_aggregate_mode": singletonAggregateMode, "max_active_aggregates": maxActiveAggregates.Load(),
		"aggregate_entries": aggregateEntries.Load(), "aggregate_exits": aggregateExits.Load(),
		"client_p50_us":              domainDataPercentile(clientDurations, 50) / 1_000,
		"client_p95_us":              domainDataPercentile(clientDurations, 95) / 1_000,
		"client_max_us":              domainDataPercentile(clientDurations, 100) / 1_000,
		"invoke_to_admission_p95_us": domainDataPercentile(preBodyDurations, 95) / 1_000,
		"invoke_to_admission_max_us": domainDataPercentile(preBodyDurations, 100) / 1_000,
		"handler_max_us": domainDataPercentile(func() []int64 {
			values := make([]int64, 0, len(serverRecords))
			for _, record := range serverRecords {
				values = append(values, record.HandlerDurationNano)
			}
			return values
		}(), 100) / 1_000,
		"handler_return_to_client_max_us": domainDataPercentile(postBodyDurations, 100) / 1_000,
		"gomaxprocs":                      os.Getenv("GOMAXPROCS"), "output_dir": outputDir,
	}
	summaryBytes, err := json.MarshalIndent(summary, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "summary.json"), append(summaryBytes, '\n'), 0o644))
	t.Logf("DOMAIN_DATA_SUMMARY %s", summaryBytes)
	if coldSyncUnexpectedErr != nil {
		t.Fatalf("cold sync-index probe failed: %v", coldSyncUnexpectedErr)
	}
}

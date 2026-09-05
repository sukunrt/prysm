//go:build minimal

package validator

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	chainmock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	executiontesting "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	attpool "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	stategenmock "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

const diagnosticEth1Warning = "Voting period before genesis + follow distance, using eth1data from head"

type diagnosticBuildStages struct {
	mu     sync.Mutex
	stages []string
}

func (s *diagnosticBuildStages) add(stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stages = append(s.stages, stage)
}

func (s *diagnosticBuildStages) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.stages...)
}

type diagnosticBlockingHeadFetcher struct {
	*chainmock.ChainService
	head       state.BeaconState
	hook       *logtest.Hook
	stages     *diagnosticBuildStages
	entered    chan struct{}
	release    chan struct{}
	enterOnce  sync.Once
	releaseOne sync.Once
}

func (f *diagnosticBlockingHeadFetcher) HeadETH1Data() *ethpb.Eth1Data {
	for _, entry := range f.hook.AllEntries() {
		if entry.Message == diagnosticEth1Warning {
			f.stages.add("warning")
			break
		}
	}
	f.stages.add("head_access")
	f.enterOnce.Do(func() { close(f.entered) })
	<-f.release
	// Make the following deposit branch exit immediately, after the test has
	// isolated the pre-packer HeadETH1Data dependency.
	_ = f.head.SetDepositRequestsStartIndex(f.head.Eth1DepositIndex())
	return f.ChainService.ETH1Data
}

func (f *diagnosticBlockingHeadFetcher) unblock() {
	f.releaseOne.Do(func() { close(f.release) })
}

type diagnosticMarkingAttPool struct {
	attpool.Pool
	stages *diagnosticBuildStages
	enter  chan struct{}
	once   sync.Once
}

func (p *diagnosticMarkingAttPool) AggregatedAttestations() []ethpb.Att {
	p.stages.add("packer")
	p.once.Do(func() { close(p.enter) })
	return p.Pool.AggregatedAttestations()
}

type diagnosticBuildEngine struct {
	execution.EngineCaller
	response *consensusblocks.GetPayloadResponse
	err      error
	returned chan struct{}
	once     sync.Once
}

func (e *diagnosticBuildEngine) GetPayload(context.Context, [8]byte, primitives.Slot) (*consensusblocks.GetPayloadResponse, error) {
	e.once.Do(func() { close(e.returned) })
	return e.response, e.err
}

type diagnosticBuildResult struct {
	err error
}

type diagnosticParentStateResult struct {
	state state.BeaconState
	err   error
}

type diagnosticMarkingBlock struct {
	interfaces.SignedBeaconBlock
	stages *diagnosticBuildStages
	done   chan struct{}
	once   sync.Once
}

func (b *diagnosticMarkingBlock) SetParentExecutionRequests(requests *enginev1.ExecutionRequestsGloas) error {
	err := b.SignedBeaconBlock.SetParentExecutionRequests(requests)
	b.stages.add("consensus_done")
	b.once.Do(func() { close(b.done) })
	return err
}

func diagnosticBuildFixture(t *testing.T, payloadErr error) (*Server, state.BeaconState, *diagnosticBlockingHeadFetcher, *diagnosticMarkingAttPool, *diagnosticBuildEngine, *ethpb.SignedBeaconBlockGloas) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	head, err := util.NewBeaconStateGloas(func(pb *ethpb.BeaconStateGloas) error {
		pb.DepositRequestsStartIndex = params.BeaconConfig().UnsetDepositRequestsStartIndex
		return nil
	})
	require.NoError(t, err)
	parentRoot := [32]byte{0x42}
	chain := &chainmock.ChainService{
		State:    head,
		ETH1Data: &ethpb.Eth1Data{DepositRoot: make([]byte, 32), BlockHash: make([]byte, 32)},
	}
	hook := logtest.NewGlobal()
	stages := &diagnosticBuildStages{}
	headFetcher := &diagnosticBlockingHeadFetcher{
		ChainService: chain,
		head:         head,
		hook:         hook,
		stages:       stages,
		entered:      make(chan struct{}),
		release:      make(chan struct{}),
	}
	t.Cleanup(headFetcher.unblock)

	basePool := attpool.NewPool()
	markingPool := &diagnosticMarkingAttPool{Pool: basePool, stages: stages, enter: make(chan struct{})}
	payload, err := consensusblocks.WrappedExecutionPayloadDeneb(&enginev1.ExecutionPayloadDeneb{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  params.BeaconConfig().DefaultFeeRecipient.Bytes(),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     []byte{},
	})
	require.NoError(t, err)
	engine := &diagnosticBuildEngine{
		response: &consensusblocks.GetPayloadResponse{
			ExecutionData:          payload,
			Bid:                    big.NewInt(0),
			BlobsBundler:           &enginev1.BlobsBundle{},
			ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
		},
		err:      payloadErr,
		returned: make(chan struct{}),
	}
	stateGen := stategenmock.NewService()
	stateGen.AddStateForRoot(head.Copy(), parentRoot)
	server := &Server{
		PayloadIDCache:           cache.NewPayloadIDCache(),
		ProposerPreferencesCache: cache.NewProposerPreferencesCache(),
		HighestBidCache:          cache.NewHighestExecutionPayloadBidCache(),
		ExecutionEngineCaller:    engine,
		HeadFetcher:              headFetcher,
		ForkchoiceFetcher:        chain,
		FinalizationFetcher:      chain,
		TimeFetcher:              chain,
		Eth1InfoFetcher:          executiontesting.New(),
		AttPool:                  markingPool,
		SlashingsPool:            slashings.NewPool(),
		ExitPool:                 voluntaryexits.NewPool(),
		SyncCommitteePool:        synccommittee.NewPool(),
		BLSChangesPool:           blstoexec.NewPool(),
		StateGen:                 stateGen,
	}
	server.PayloadIDCache.Set(1, parentRoot, false, [8]byte{1})
	block := &ethpb.SignedBeaconBlockGloas{Block: &ethpb.BeaconBlockGloas{
		Slot:       1,
		ParentRoot: parentRoot[:],
		Body:       &ethpb.BeaconBlockBodyGloas{},
	}}
	return server, head, headFetcher, markingPool, engine, block
}

func diagnosticRunBuild(t *testing.T, ctx context.Context, server *Server, protoBlock *ethpb.SignedBeaconBlockGloas, consensusDone chan struct{}) <-chan diagnosticBuildResult {
	t.Helper()
	result := make(chan diagnosticBuildResult, 1)
	go func() {
		block, err := consensusblocks.NewSignedBeaconBlock(protoBlock)
		if err == nil {
			marked := &diagnosticMarkingBlock{SignedBeaconBlock: block, stages: server.HeadFetcher.(*diagnosticBlockingHeadFetcher).stages, done: consensusDone}
			_, err = server.buildBlockGloas(ctx, marked, server.HeadFetcher.(*diagnosticBlockingHeadFetcher).head, true, false, false, nil)
		}
		result <- diagnosticBuildResult{err: err}
	}()
	return result
}

func diagnosticAwait(t *testing.T, ch <-chan struct{}, stage string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", stage)
	}
}

func diagnosticAwaitNoConsensusGoroutine(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stacks := make([]byte, 1<<20)
		n := runtime.Stack(stacks, true)
		if !bytes.Contains(stacks[:n], []byte("validator.(*Server).buildBlockGloas.func1")) {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("consensus goroutine remained after its completion marker")
}

func TestDiagnosticBuildBlockGloasControlledCancellationSemantics(t *testing.T) {
	t.Run("payload_failure_returns_while_consensus_dependency_is_blocked", func(t *testing.T) {
		server, _, headFetcher, pool, engine, block := diagnosticBuildFixture(t, errors.New("controlled payload failure"))
		consensusDone := make(chan struct{})
		result := diagnosticRunBuild(t, t.Context(), server, block, consensusDone)
		diagnosticAwait(t, engine.returned, "local payload failure")
		diagnosticAwait(t, headFetcher.entered, "HeadETH1Data entry")

		select {
		case got := <-result:
			require.ErrorContains(t, "Could not get local payload and no P2P bid fallback", got.err)
		case <-time.After(2 * time.Second):
			t.Fatal("payload failure waited for the blocked consensus dependency")
		}
		require.DeepEqual(t, []string{"warning", "head_access"}, headFetcher.stages.snapshot())

		headFetcher.unblock()
		diagnosticAwait(t, pool.enter, "abandoned consensus packer entry")
		diagnosticAwait(t, consensusDone, "abandoned consensus completion")
		diagnosticAwaitNoConsensusGoroutine(t)
		require.DeepEqual(t, []string{"warning", "head_access", "packer", "consensus_done"}, headFetcher.stages.snapshot())
	})

	t.Run("payload_success_waits_then_reports_late_canceled_state_root", func(t *testing.T) {
		server, _, headFetcher, pool, engine, block := diagnosticBuildFixture(t, nil)
		ctx, cancel := context.WithCancel(t.Context())
		consensusDone := make(chan struct{})
		result := diagnosticRunBuild(t, ctx, server, block, consensusDone)
		diagnosticAwait(t, engine.returned, "local payload success")
		diagnosticAwait(t, headFetcher.entered, "HeadETH1Data entry")

		cancel()
		select {
		case got := <-result:
			t.Fatalf("build returned before the consensus dependency was released: %v", got.err)
		case <-time.After(50 * time.Millisecond):
		}

		headFetcher.unblock()
		diagnosticAwait(t, pool.enter, "required consensus packer entry")
		diagnosticAwait(t, consensusDone, "required consensus completion")
		select {
		case got := <-result:
			require.ErrorContains(t, "Could not compute state root", got.err)
			require.ErrorContains(t, "context", got.err)
		case <-time.After(2 * time.Second):
			t.Fatal("build did not finish after releasing the consensus dependency")
		}
		require.DeepEqual(t, []string{"warning", "head_access", "packer", "consensus_done"}, headFetcher.stages.snapshot())
	})
}

type diagnosticBlockingHeadStateFetcher struct {
	*chainmock.ChainService
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *diagnosticBlockingHeadStateFetcher) HeadState(context.Context) (state.BeaconState, error) {
	f.once.Do(func() { close(f.entered) })
	<-f.release
	return f.ChainService.State, nil
}

func diagnosticParentStateCall(ctx context.Context, server *Server, root [32]byte) <-chan diagnosticParentStateResult {
	result := make(chan diagnosticParentStateResult, 1)
	go func() {
		st, err := server.getParentStateFromReorgData(ctx, 13, root, root, root)
		result <- diagnosticParentStateResult{state: st, err: err}
	}()
	return result
}

func diagnosticSkipSlotCacheKey(t *testing.T, st state.BeaconState) [32]byte {
	t.Helper()
	headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	return hash.Hash(append(bytesutil.Bytes32(uint64(st.Slot())), headerRoot[:]...))
}

func diagnosticParentStateWaitingInSkipSlotCache(ctx context.Context) bool {
	stacks := make([]byte, 1<<20)
	for ctx.Err() == nil {
		n := runtime.Stack(stacks, true)
		for _, stack := range strings.Split(string(stacks[:n]), "\n\n") {
			if strings.Contains(stack, "getParentStateFromReorgData") && strings.Contains(stack, "cache.(*SkipSlotCache).Get") {
				return true
			}
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func TestDiagnosticParentStateSlot13CancellationBoundaries(t *testing.T) {
	head, err := util.NewBeaconStateGloas()
	require.NoError(t, err)

	t.Run("cancellation_while_head_dependency_is_blocked", func(t *testing.T) {
		root := [32]byte{0x13, 0x01}
		fetcher := &diagnosticBlockingHeadStateFetcher{
			ChainService: &chainmock.ChainService{State: head.Copy(), Root: root[:]},
			entered:      make(chan struct{}),
			release:      make(chan struct{}),
		}
		server := &Server{HeadFetcher: fetcher}
		ctx, cancel := context.WithCancel(t.Context())
		result := diagnosticParentStateCall(ctx, server, root)
		diagnosticAwait(t, fetcher.entered, "blocking head-state access")
		cancel()
		select {
		case got := <-result:
			t.Fatalf("head dependency honored cancellation before release: %v", got.err)
		case <-time.After(50 * time.Millisecond):
		}
		close(fetcher.release)
		select {
		case got := <-result:
			require.Equal(t, true, got.state == nil)
			require.ErrorContains(t, "Could not process slots up to 13", got.err)
			require.ErrorContains(t, "context canceled", got.err)
		case <-time.After(2 * time.Second):
			t.Fatal("parent-state call did not observe cancellation after head release")
		}
	})

	t.Run("cancellation_while_real_skip_slot_cache_is_in_progress", func(t *testing.T) {
		root := [32]byte{0x13, 0x02}
		st := head.Copy()
		originalCache := transition.SkipSlotCache
		transition.SkipSlotCache = cache.NewSkipSlotCache()
		t.Cleanup(func() { transition.SkipSlotCache = originalCache })
		key := diagnosticSkipSlotCacheKey(t, st)
		require.NoError(t, transition.SkipSlotCache.MarkInProgress(key))
		t.Cleanup(func() { transition.SkipSlotCache.MarkNotInProgress(key) })

		server := &Server{HeadFetcher: &chainmock.ChainService{State: st, Root: root[:]}}
		ctx, cancel := context.WithCancel(t.Context())
		result := diagnosticParentStateCall(ctx, server, root)
		observationCtx, stopObservation := context.WithTimeout(t.Context(), 2*time.Second)
		require.Equal(t, true, diagnosticParentStateWaitingInSkipSlotCache(observationCtx))
		stopObservation()
		cancel()
		select {
		case got := <-result:
			require.Equal(t, true, got.state == nil)
			require.ErrorContains(t, "Could not process slots up to 13", got.err)
			require.ErrorContains(t, "context canceled", got.err)
		case <-time.After(2 * time.Second):
			t.Fatal("in-progress skip-slot cache wait did not observe cancellation")
		}
	})
}

package client

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/dgraph-io/ristretto/v2"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
)

func newStartupDiagnosticDomainCache(t *testing.T) *ristretto.Cache[string, proto.Message] {
	t.Helper()
	cache, err := ristretto.NewCache(&ristretto.Config[string, proto.Message]{
		NumCounters: 1920,
		MaxCost:     192,
		BufferItems: 64,
	})
	require.NoError(t, err)
	t.Cleanup(cache.Close)
	return cache
}

func rolesAtWaitingInSingleflight(ctx context.Context) bool {
	buf := make([]byte, 1<<20)
	for ctx.Err() == nil {
		n := runtime.Stack(buf, true)
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, ".RolesAt(") && strings.Contains(stack, "singleflight.(*Group).Do") {
				return true
			}
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// TestRolesAtDeadlineStageDifferentialDiagnostic demonstrates that the same
// deadline category at the sync-index stage can represent either a long
// sync-index RPC or an already-expired context after an earlier proof wait.
func TestRolesAtDeadlineStageDifferentialDiagnostic(t *testing.T) {
	for _, test := range []struct {
		name                  string
		selectionPastDeadline bool
	}{
		{name: "sync-index-occupies-budget"},
		{name: "selection-singleflight-occupies-budget", selectionPastDeadline: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			v, mocks, validatorKey, finish := setup(t, false)
			defer finish()
			v.domainDataCache = newStartupDiagnosticDomainCache(t)

			pubkey := bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())
			const slot = primitives.Slot(1)
			v.duties = testDutyStore(&ethpb.ValidatorDuty{
				ValidatorIndex:  0,
				CommitteeIndex:  0,
				CommitteeLength: 1,
				AttesterSlot:    slot,
				ProposerSlots:   []primitives.Slot{slot},
				PublicKey:       pubkey[:],
				IsSyncCommittee: true,
				Status:          ethpb.ValidatorStatus_ACTIVE,
			})

			selectionEntered := make(chan struct{})
			releaseSelection := make(chan struct{})
			var selectionOnce sync.Once
			var releaseOnce sync.Once
			releaseWinner := func() { releaseOnce.Do(func() { close(releaseSelection) }) }
			defer releaseWinner()
			t.Cleanup(releaseWinner)
			var domainCalls atomic.Int32
			mocks.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, req *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
					domainCalls.Add(1)
					switch string(req.Domain) {
					case string(params.BeaconConfig().DomainSelectionProof[:]):
						selectionOnce.Do(func() { close(selectionEntered) })
						if test.selectionPastDeadline {
							<-releaseSelection
						}
						return &ethpb.DomainResponse{SignatureDomain: make([]byte, fieldparams.RootLength)}, nil
					case string(params.BeaconConfig().DomainRandao[:]):
						require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
						return nil, ctx.Err()
					default:
						t.Fatalf("unexpected domain %#x", req.Domain)
						return nil, errors.New("unexpected domain")
					}
				}).Times(2)

			type syncObservation struct {
				enteredWith error
				duration    time.Duration
			}
			syncObserved := make(chan syncObservation, 1)
			mocks.validatorClient.EXPECT().SyncSubcommitteeIndex(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, _ *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
					started := time.Now()
					atEntry := ctx.Err()
					if atEntry == nil {
						<-ctx.Done()
					}
					syncObserved <- syncObservation{enteredWith: atEntry, duration: time.Since(started)}
					return nil, ctx.Err()
				}).Times(1)
			mocks.validatorClient.EXPECT().BeaconBlock(gomock.Any(), gomock.Any()).Times(0)

			var backgroundProofDone chan error
			if test.selectionPastDeadline {
				backgroundProofDone = make(chan error, 1)
				go func() {
					_, err := v.aggSelector.AttestationSelectionProof(context.Background(), slot, pubkey)
					backgroundProofDone <- err
				}()
				select {
				case <-selectionEntered:
				case <-time.After(time.Second):
					t.Fatal("background selection proof did not enter DomainData")
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
			defer cancel()
			rolesResult := make(chan map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole, 1)
			rolesErr := make(chan error, 1)
			rolesStarted := time.Now()
			go func() {
				roles, err := v.RolesAt(ctx, slot)
				rolesResult <- roles
				rolesErr <- err
			}()

			if test.selectionPastDeadline {
				require.True(t, rolesAtWaitingInSingleflight(ctx), "RolesAt did not observably join the in-flight selection proof before its deadline")
				<-ctx.Done()
				releaseWinner()
			}
			var roles map[[fieldparams.BLSPubkeyLength]byte][]iface.ValidatorRole
			select {
			case roles = <-rolesResult:
			case <-time.After(time.Second):
				t.Fatal("RolesAt did not return")
			}
			select {
			case err := <-rolesErr:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("RolesAt error result did not return")
			}
			require.Contains(t, roles[pubkey], iface.RoleProposer)
			var observation syncObservation
			select {
			case observation = <-syncObserved:
			case <-time.After(time.Second):
				t.Fatal("SyncSubcommitteeIndex observation did not return")
			}
			if test.selectionPastDeadline {
				require.ErrorIs(t, observation.enteredWith, context.DeadlineExceeded)
				require.Less(t, observation.duration, 25*time.Millisecond)
				select {
				case err := <-backgroundProofDone:
					require.NoError(t, err)
				case <-time.After(time.Second):
					t.Fatal("background selection proof did not return")
				}
			} else {
				require.NoError(t, observation.enteredWith)
				require.GreaterOrEqual(t, observation.duration, 50*time.Millisecond)
			}

			v.ProposeBlock(ctx, slot, pubkey)
			require.Contains(t, hook.LastEntry().Message, "Failed to sign randao reveal")
			require.Equal(t, int32(2), domainCalls.Load())
			t.Logf("roles_elapsed=%s sync_entered_with=%v sync_duration=%s", time.Since(rolesStarted), observation.enteredWith, observation.duration)
		})
	}
}

func TestRolesAtSyncSelectionDomainDeadlineDiagnostic(t *testing.T) {
	hook := logTest.NewGlobal()
	v, mocks, validatorKey, finish := setup(t, false)
	defer finish()
	v.domainDataCache = newStartupDiagnosticDomainCache(t)

	pubkey := bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())
	const slot = primitives.Slot(1)
	v.duties = testDutyStore(&ethpb.ValidatorDuty{
		ValidatorIndex: 0, CommitteeLength: 1, AttesterSlot: slot,
		ProposerSlots: []primitives.Slot{slot}, PublicKey: pubkey[:],
		IsSyncCommittee: true, Status: ethpb.ValidatorStatus_ACTIVE,
	})

	type observation struct {
		enteredWith error
		duration    time.Duration
	}
	syncDomainObserved := make(chan observation, 1)
	mocks.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, req *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
			switch string(req.Domain) {
			case string(params.BeaconConfig().DomainSelectionProof[:]):
				return &ethpb.DomainResponse{SignatureDomain: make([]byte, fieldparams.RootLength)}, nil
			case string(params.BeaconConfig().DomainSyncCommitteeSelectionProof[:]):
				started := time.Now()
				atEntry := ctx.Err()
				if atEntry == nil {
					<-ctx.Done()
				}
				syncDomainObserved <- observation{enteredWith: atEntry, duration: time.Since(started)}
				return nil, ctx.Err()
			case string(params.BeaconConfig().DomainRandao[:]):
				require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
				return nil, ctx.Err()
			default:
				t.Fatalf("unexpected domain %#x", req.Domain)
				return nil, errors.New("unexpected domain")
			}
		}).Times(3)
	mocks.validatorClient.EXPECT().SyncSubcommitteeIndex(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
			require.NoError(t, ctx.Err())
			return &ethpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{0}}, nil
		}).Times(1)
	mocks.validatorClient.EXPECT().BeaconBlock(gomock.Any(), gomock.Any()).Times(0)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	roles, err := v.RolesAt(ctx, slot)
	require.NoError(t, err)
	require.Contains(t, roles[pubkey], iface.RoleProposer)
	observed := <-syncDomainObserved
	require.NoError(t, observed.enteredWith)
	require.GreaterOrEqual(t, observed.duration, 50*time.Millisecond)
	v.ProposeBlock(ctx, slot, pubkey)
	require.Contains(t, hook.LastEntry().Message, "Failed to sign randao reveal")
	t.Logf("roles_elapsed=%s sync_selection_domain_entered_with=%v domain_duration=%s", time.Since(started), observed.enteredWith, observed.duration)
}

func TestRolesAtLiveBudgetReachesBeaconBlockDiagnostic(t *testing.T) {
	v, mocks, validatorKey, finish := setup(t, false)
	defer finish()
	v.domainDataCache = newStartupDiagnosticDomainCache(t)
	v.head = newHeadTracker()
	v.genesisTime = time.Now().Add(-time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	v.graffiti = []byte("diagnostic")

	pubkey := bytesutil.ToBytes48(validatorKey.PublicKey().Marshal())
	const slot = primitives.Slot(1)
	v.duties = testDutyStore(&ethpb.ValidatorDuty{
		ValidatorIndex: 0, CommitteeLength: 1, AttesterSlot: slot,
		ProposerSlots: []primitives.Slot{slot}, PublicKey: pubkey[:],
		IsSyncCommittee: true, Status: ethpb.ValidatorStatus_ACTIVE,
	})
	mocks.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).Return(
		&ethpb.DomainResponse{SignatureDomain: make([]byte, fieldparams.RootLength)}, nil,
	).Times(2)
	mocks.validatorClient.EXPECT().SyncSubcommitteeIndex(gomock.Any(), gomock.Any()).Return(
		&ethpb.SyncSubcommitteeIndexResponse{}, nil,
	).Times(1)
	beaconBlockEntered := make(chan struct{})
	mocks.validatorClient.EXPECT().BeaconBlock(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
			require.NoError(t, ctx.Err())
			close(beaconBlockEntered)
			return nil, errors.New("controlled block stop")
		}).Times(1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	roles, err := v.RolesAt(ctx, slot)
	require.NoError(t, err)
	require.Contains(t, roles[pubkey], iface.RoleProposer)
	v.ProposeBlock(ctx, slot, pubkey)
	select {
	case <-beaconBlockEntered:
	default:
		t.Fatal("live-budget proposer did not reach BeaconBlock")
	}
}

// TestAttestationDataCanceledFanoutDiagnostic exercises the real post-Electra
// cache lock with the slot-1 owner cardinality: 75 callers, one live RPC, then
// 74 serialized RPC entries carrying the already-expired context.
func TestAttestationDataCanceledFanoutDiagnostic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctrl := gomock.NewController(t)
	client := validatormock.NewMockValidatorClient(ctrl)
	v := &validator{validatorClient: client}

	const callers = 75
	var concurrent atomic.Int32
	var maximum atomic.Int32
	var entries atomic.Int32
	var liveEntries atomic.Int32
	client.EXPECT().AttestationData(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *ethpb.AttestationDataRequest) (*ethpb.AttestationData, error) {
			entries.Add(1)
			current := concurrent.Add(1)
			defer concurrent.Add(-1)
			for {
				old := maximum.Load()
				if current <= old || maximum.CompareAndSwap(old, current) {
					break
				}
			}
			if ctx.Err() == nil {
				liveEntries.Add(1)
				<-ctx.Done()
			}
			return nil, ctx.Err()
		}).Times(callers)

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	start := make(chan struct{})
	errorsCh := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for i := 0; i < callers; i++ {
		go func(index int) {
			defer workers.Done()
			<-start
			_, err := v.getAttestationData(ctx, 1, primitives.CommitteeIndex(index))
			errorsCh <- err
		}(i)
	}
	close(start)
	<-ctx.Done()
	workers.Wait()
	drainAfterDeadline := time.Since(deadline)
	close(errorsCh)
	for err := range errorsCh {
		require.ErrorIs(t, err, context.DeadlineExceeded)
	}
	require.Equal(t, int32(callers), entries.Load())
	require.Equal(t, int32(1), liveEntries.Load())
	require.Equal(t, int32(1), maximum.Load())
	require.Less(t, drainAfterDeadline, time.Second)
	t.Logf("rpc_entries=%d live_at_entry=%d max_concurrent=%d drain_after_deadline=%s", entries.Load(), liveEntries.Load(), maximum.Load(), drainAfterDeadline)
}

package sync

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"iter"
	"os"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	blockchaintesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	mocksync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/genesis"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

const (
	FullGossipDomainDiagnosticMessages = 15_000
	fullGossipDomainRegistrySize       = 120_000
	fullGossipDomainCommitteeCount     = 6
)

// FullGossipDomainStats contains bounded counters from the real pubsub,
// validation, signature-batch, feed, and legacy-pool pipeline.
type FullGossipDomainStats struct {
	ValidationStarted       int64
	ValidationAccepted      int64
	ValidationIgnored       int64
	ValidationRejected      int64
	SubscriberStarted       int64
	SubscriberCompleted     int64
	SubscriberFailed        int64
	FeedEvents              int64
	IteratorEntries         int64
	IteratorExits           int64
	ActiveIterators         int64
	MaximumActiveIterators  int64
	UnaggregatedPoolEntries int
	FirstError              string
}

type fullGossipDomainCounters struct {
	validationStarted     atomic.Int64
	validationAccepted    atomic.Int64
	validationIgnored     atomic.Int64
	validationRejected    atomic.Int64
	subscriberStarted     atomic.Int64
	subscriberCompleted   atomic.Int64
	subscriberFailed      atomic.Int64
	feedEvents            atomic.Int64
	iteratorEntries       atomic.Int64
	iteratorExits         atomic.Int64
	activeIterators       atomic.Int64
	maximumActiveIterator atomic.Int64
	firstIterator         chan struct{}
	firstIteratorOnce     sync.Once
	firstErrorMu          sync.Mutex
	firstError            string
}

func (c *fullGossipDomainCounters) recordError(err error) {
	if err == nil {
		return
	}
	c.firstErrorMu.Lock()
	if c.firstError == "" {
		c.firstError = err.Error()
	}
	c.firstErrorMu.Unlock()
}

func fullGossipDomainRecordMaximum(maximum *atomic.Int64, value int64) {
	for {
		old := maximum.Load()
		if value <= old || maximum.CompareAndSwap(old, value) {
			return
		}
	}
}

type fullGossipDomainState struct {
	state.ReadOnlyBeaconState
	counters *fullGossipDomainCounters
	snapshot []state.ReadOnlyValidator
}

func (s *fullGossipDomainState) ValidatorsReadOnlySeq() iter.Seq2[primitives.ValidatorIndex, state.ReadOnlyValidator] {
	return func(yield func(primitives.ValidatorIndex, state.ReadOnlyValidator) bool) {
		s.counters.iteratorEntries.Add(1)
		active := s.counters.activeIterators.Add(1)
		fullGossipDomainRecordMaximum(&s.counters.maximumActiveIterator, active)
		s.counters.firstIteratorOnce.Do(func() { close(s.counters.firstIterator) })
		defer func() {
			s.counters.activeIterators.Add(-1)
			s.counters.iteratorExits.Add(1)
		}()
		if s.snapshot != nil {
			for index, validator := range s.snapshot {
				if !yield(primitives.ValidatorIndex(index), validator) {
					return
				}
			}
			return
		}
		for index, validator := range s.ReadOnlyBeaconState.ValidatorsReadOnlySeq() {
			if !yield(index, validator) {
				return
			}
		}
	}
}

type fullGossipDomainChain struct {
	*blockchain.Service
	counters *fullGossipDomainCounters
	snapshot []state.ReadOnlyValidator
}

func (c *fullGossipDomainChain) AttestationTargetState(ctx context.Context, checkpoint *ethpb.Checkpoint) (state.ReadOnlyBeaconState, error) {
	st, err := c.Service.AttestationTargetState(ctx, checkpoint)
	if err != nil {
		return nil, err
	}
	return &fullGossipDomainState{ReadOnlyBeaconState: st, counters: c.counters, snapshot: c.snapshot}, nil
}

// FullGossipDomainDiagnosticFixture is an env-gated test fixture exported only
// to package sync_test. It keeps the real blockchain service available for the
// external DomainData server while private sync methods stay in this package.
type FullGossipDomainDiagnosticFixture struct {
	Chain                 *blockchain.Service
	Checkpoint            *ethpb.Checkpoint
	GenesisRoot           [32]byte
	GenesisValidatorsRoot [32]byte
	GenesisTime           time.Time
	SetupCompleted        time.Time
	Committees            [][]primitives.ValidatorIndex
	Topics                []string
	RuntimeNumCPU         int
	GOMAXPROCS            int

	service  *Service
	sender   *p2ptest.TestP2P
	pool     attestations.Pool
	counters *fullGossipDomainCounters
	encoded  [][]byte
	topics   []string
}

// NewFullGossipDomainDiagnosticFixture builds one real slot-zero chain and six
// real SingleAttestation gossip subscriptions. snapshotValidators changes only
// the returned checkpoint state's registry iterator; every other dependency is
// delegated to the same production blockchain service.
func NewFullGossipDomainDiagnosticFixture(t *testing.T, snapshotValidators bool) *FullGossipDomainDiagnosticFixture {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	cfg.MaxCommitteesPerSlot = 64
	params.OverrideBeaconConfig(cfg)
	forkEpochs := []primitives.Epoch{
		cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch, cfg.DenebForkEpoch,
		cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch, cfg.HezeForkEpoch,
	}
	for i, epoch := range forkEpochs {
		require.Equal(t, uint64(0), uint64(epoch))
		if i > 0 {
			require.Equal(t, true, forkEpochs[i-1] <= epoch)
		}
	}
	resetFeatures := features.InitWithReset(&features.Flags{EnableExperimentalAttestationPool: false})
	t.Cleanup(resetFeatures)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	validators := make([]*ethpb.Validator, fullGossipDomainRegistrySize)
	balances := make([]uint64, fullGossipDomainRegistrySize)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey:                  make([]byte, params.BeaconConfig().BLSPubkeyLength),
			WithdrawalCredentials:      make([]byte, 32),
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
		}
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	scratch, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Validators = validators
		pb.Balances = balances
		pb.PreviousEpochParticipation = make([]byte, fullGossipDomainRegistrySize)
		pb.CurrentEpochParticipation = make([]byte, fullGossipDomainRegistrySize)
		pb.InactivityScores = make([]uint64, fullGossipDomainRegistrySize)
		return nil
	})
	require.NoError(t, err)
	committees := make([][]primitives.ValidatorIndex, fullGossipDomainCommitteeCount)
	for committee := range committees {
		committees[committee], err = helpers.BeaconCommitteeFromState(t.Context(), scratch, 1, primitives.CommitteeIndex(committee))
		require.NoError(t, err)
		require.Equal(t, 2500, len(committees[committee]))
	}

	signerIndices := make(map[primitives.ValidatorIndex]struct{}, FullGossipDomainDiagnosticMessages)
	for _, committee := range committees {
		for _, validatorIndex := range committee {
			signerIndices[validatorIndex] = struct{}{}
		}
	}
	keys := make(map[primitives.ValidatorIndex]bls.SecretKey, FullGossipDomainDiagnosticMessages)
	syncPublicKeys := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for validatorIndex := range validators {
		secretBytes := make([]byte, params.BeaconConfig().BLSSecretKeyLength)
		binary.BigEndian.PutUint64(secretBytes[len(secretBytes)-8:], uint64(validatorIndex)+1)
		secretKey, keyErr := bls.SecretKeyFromBytes(secretBytes)
		require.NoError(t, keyErr)
		validators[validatorIndex].PublicKey = secretKey.PublicKey().Marshal()
		_, keyErr = bls.PublicKeyFromBytes(validators[validatorIndex].PublicKey)
		require.NoError(t, keyErr)
		if _, signer := signerIndices[primitives.ValidatorIndex(validatorIndex)]; signer {
			keys[primitives.ValidatorIndex(validatorIndex)] = secretKey
		}
		if validatorIndex < len(syncPublicKeys) {
			syncPublicKeys[validatorIndex] = append([]byte(nil), validators[validatorIndex].PublicKey...)
		}
	}
	syncAggregate, err := bls.AggregatePublicKeys(syncPublicKeys)
	require.NoError(t, err)
	registryRoot, err := stateutil.ValidatorRegistryRoot(version.Heze, stateutil.CompactValidatorsFromProto(validators))
	require.NoError(t, err)
	// All expensive key construction and registry hashing precede the genesis
	// clock. The remaining chain, encoding, and peer setup must finish before
	// slot one so the timed release has the historical slot-one budget.
	genesisTime := time.Now().Add(3 * time.Second).Truncate(time.Second)
	genesisState, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.GenesisTime = uint64(genesisTime.Unix())
		pb.GenesisValidatorsRoot = append([]byte(nil), registryRoot[:]...)
		pb.Fork = params.ForkFromConfig(params.BeaconConfig(), 0)
		pb.Validators = validators
		pb.Balances = balances
		pb.PreviousEpochParticipation = make([]byte, fullGossipDomainRegistrySize)
		pb.CurrentEpochParticipation = make([]byte, fullGossipDomainRegistrySize)
		pb.InactivityScores = make([]uint64, fullGossipDomainRegistrySize)
		pb.CurrentSyncCommittee = &ethpb.SyncCommittee{
			Pubkeys: syncPublicKeys, AggregatePubkey: syncAggregate.Marshal(),
		}
		pb.NextSyncCommittee = &ethpb.SyncCommittee{
			Pubkeys: syncPublicKeys, AggregatePubkey: syncAggregate.Marshal(),
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, version.Heze, genesisState.Version())
	require.Equal(t, fullGossipDomainRegistrySize, genesisState.NumValidators())
	require.Equal(t, uint64(0), uint64(genesisState.Slot()))
	require.DeepEqual(t, registryRoot[:], genesisState.GenesisValidatorsRoot())
	helpers.ClearCache()
	for committee := range committees {
		finalCommittee, committeeErr := helpers.BeaconCommitteeFromState(t.Context(), genesisState, 1, primitives.CommitteeIndex(committee))
		require.NoError(t, committeeErr)
		require.DeepEqual(t, committees[committee], finalCommittee)
	}

	params.BeaconConfig().ApplyOptions(params.WithGenesisValidatorsRoot(registryRoot))
	params.BeaconConfig().InitializeForkSchedule()
	genesis.StoreStateDuringTest(t, genesisState)

	database := dbtest.SetupDB(t)
	require.NoError(t, database.SaveGenesisData(t.Context(), genesisState))
	genesisBlock, err := database.GenesisBlock(t.Context())
	require.NoError(t, err)
	genesisRoot, err := genesisBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	checkpoint := &ethpb.Checkpoint{Epoch: 0, Root: append([]byte(nil), genesisRoot[:]...)}

	forkChoice := doublylinkedtree.New()
	stateGen := stategen.New(database, forkChoice)
	clockSync := startup.NewClockSynchronizer()
	pool := attestations.NewPool()
	attestationService, err := attestations.NewService(t.Context(), &attestations.Config{Pool: pool})
	require.NoError(t, err)
	chain, err := blockchain.NewService(t.Context(),
		blockchain.WithDatabase(database),
		blockchain.WithStateGen(stateGen),
		blockchain.WithForkChoiceStore(forkChoice),
		blockchain.WithClockSynchronizer(clockSync),
		blockchain.WithAttestationPool(pool),
		blockchain.WithAttestationService(attestationService),
		blockchain.WithStateNotifier(blockchaintesting.NewSimpleStateNotifier()),
		blockchain.WithFinalizedStateAtStartUp(genesisState),
	)
	require.NoError(t, err)
	require.NoError(t, chain.StartFromSavedState(genesisState))
	require.Equal(t, uint64(0), uint64(chain.HeadSlot()))
	require.Equal(t, true, chain.InForkchoice(genesisRoot))
	require.Equal(t, true, chain.HasFullNode(genesisRoot))
	optimistic, err := chain.IsOptimistic(t.Context())
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
	targetRoot, err := chain.TargetRootForRound(genesisRoot, 0)
	require.NoError(t, err)
	require.Equal(t, genesisRoot, targetRoot)
	waitFullGossipDomainUntil(t, genesisTime)
	warmState, err := chain.AttestationTargetState(t.Context(), checkpoint)
	require.NoError(t, err)
	require.Equal(t, uint64(0), uint64(warmState.Slot()))
	require.Equal(t, fullGossipDomainRegistrySize, warmState.NumValidators())
	headState, err := chain.HeadState(t.Context())
	require.NoError(t, err)
	headValidatorsPointer := fullGossipDomainValidatorStoragePointer(headState)
	targetValidatorsPointer := fullGossipDomainValidatorStoragePointer(warmState)
	require.NotEqual(t, uintptr(0), headValidatorsPointer)
	require.Equal(t, headValidatorsPointer, targetValidatorsPointer)

	var snapshot []state.ReadOnlyValidator
	if snapshotValidators {
		snapshot = make([]state.ReadOnlyValidator, fullGossipDomainRegistrySize)
		for i := range snapshot {
			snapshot[i], err = warmState.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(i))
			require.NoError(t, err)
		}
		for _, i := range []int{0, 1, len(snapshot) / 2, len(snapshot) - 1} {
			nativeValidator, nativeErr := warmState.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(i))
			require.NoError(t, nativeErr)
			require.Equal(t, nativeValidator.PublicKey(), snapshot[i].PublicKey())
			require.Equal(t, nativeValidator.EffectiveBalance(), snapshot[i].EffectiveBalance())
		}
		verificationCounters := &fullGossipDomainCounters{firstIterator: make(chan struct{})}
		snapshotState := &fullGossipDomainState{
			ReadOnlyBeaconState: warmState, counters: verificationCounters, snapshot: snapshot,
		}
		nativeActive, activeErr := helpers.ActiveValidatorCount(t.Context(), warmState, 0)
		require.NoError(t, activeErr)
		snapshotActive, activeErr := helpers.ActiveValidatorCount(t.Context(), snapshotState, 0)
		require.NoError(t, activeErr)
		require.Equal(t, nativeActive, snapshotActive)
		require.Equal(t, uint64(fullGossipDomainRegistrySize), snapshotActive)
	}

	counters := &fullGossipDomainCounters{firstIterator: make(chan struct{})}
	wrapper := &fullGossipDomainChain{Service: chain, counters: counters, snapshot: snapshot}
	operationNotifier := &blockchaintesting.SimpleNotifier{Feed: new(event.Feed)}
	receiver, sender := newFullGossipDomainPeers(t, registryRoot[:])
	clock := startup.NewClock(genesisTime, registryRoot)
	syncService := NewService(t.Context(),
		WithP2P(receiver),
		WithDatabase(database),
		WithChainService(wrapper),
		WithInitialSync(&mocksync.Sync{IsSyncing: false}),
		WithAttestationPool(pool),
		WithAttestationNotifier(operationNotifier),
		WithOperationNotifier(operationNotifier),
		WithStateGen(stateGen),
		WithBatchVerifierLimit(1000),
	)
	require.NotNil(t, syncService)
	syncService.cfg.clock = clock
	syncService.markForChainStart()
	go syncService.verifierRoutine()
	t.Cleanup(func() { require.NoError(t, syncService.Stop()) })

	events := make(chan *feed.Event, FullGossipDomainDiagnosticMessages)
	subscription := operationNotifier.OperationFeed().Subscribe(events)
	t.Cleanup(subscription.Unsubscribe)
	go func() {
		for {
			select {
			case <-t.Context().Done():
				return
			case <-events:
				counters.feedEvents.Add(1)
			}
		}
	}()

	digest := params.ForkDigest(0)
	topics := make([]string, fullGossipDomainCommitteeCount)
	committeeSubnets := make([]uint64, fullGossipDomainCommitteeCount)
	for committeeIndex := range topics {
		committeeSubnets[committeeIndex] = helpers.ComputeSubnetForAttestation(fullGossipDomainRegistrySize, &ethpb.SingleAttestation{
			CommitteeId: primitives.CommitteeIndex(committeeIndex),
			Data:        &ethpb.AttestationData{Slot: 1},
		})
		topics[committeeIndex] = fmt.Sprintf(p2p.GossipTypeMapping[reflect.TypeFor[*ethpb.SingleAttestation]()], digest, committeeSubnets[committeeIndex]) + receiver.Encoding().ProtocolSuffix()
		validator := func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
			counters.validationStarted.Add(1)
			result, validateErr := syncService.validateCommitteeIndexBeaconAttestation(ctx, pid, msg)
			switch result {
			case pubsub.ValidationAccept:
				counters.validationAccepted.Add(1)
			case pubsub.ValidationReject:
				counters.validationRejected.Add(1)
			default:
				counters.validationIgnored.Add(1)
			}
			counters.recordError(validateErr)
			return result, validateErr
		}
		handler := func(ctx context.Context, msg proto.Message) error {
			counters.subscriberStarted.Add(1)
			handleErr := syncService.committeeIndexBeaconAttestationSubscriber(ctx, msg)
			if handleErr != nil {
				counters.subscriberFailed.Add(1)
				counters.recordError(handleErr)
				return handleErr
			}
			counters.subscriberCompleted.Add(1)
			return nil
		}
		require.NotNil(t, syncService.subscribeWithBase(topics[committeeIndex], validator, handler))
		senderSubscription, subscribeErr := sender.SubscribeToTopic(topics[committeeIndex], pubsub.WithBufferSize(5000))
		require.NoError(t, subscribeErr)
		t.Cleanup(senderSubscription.Cancel)
		go func() {
			for {
				if _, nextErr := senderSubscription.Next(t.Context()); nextErr != nil {
					return
				}
			}
		}()
	}
	waitFullGossipDomainPeers(t, receiver, sender, topics)

	domain, err := signing.Domain(genesisState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, registryRoot[:])
	require.NoError(t, err)
	configDomain, err := signing.Domain(params.ForkFromConfig(params.BeaconConfig(), 0), 0, params.BeaconConfig().DomainBeaconAttester, registryRoot[:])
	require.NoError(t, err)
	require.DeepEqual(t, domain, configDomain)
	encoded := make([][]byte, FullGossipDomainDiagnosticMessages)
	messageTopics := make([]string, FullGossipDomainDiagnosticMessages)
	for committeeIndex, committee := range committees {
		for position, validatorIndex := range committee {
			messageIndex := position*fullGossipDomainCommitteeCount + committeeIndex
			data := &ethpb.AttestationData{
				Slot:            1,
				CommitteeIndex:  1,
				BeaconBlockRoot: append([]byte(nil), genesisRoot[:]...),
				Source:          &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Epoch: 0, Root: append([]byte(nil), genesisRoot[:]...)},
			}
			signingRoot, signingErr := signing.ComputeSigningRoot(data, domain)
			require.NoError(t, signingErr)
			single := &ethpb.SingleAttestation{
				CommitteeId:   primitives.CommitteeIndex(committeeIndex),
				AttesterIndex: validatorIndex,
				Data:          data,
				Signature:     keys[validatorIndex].Sign(signingRoot[:]).Marshal(),
			}
			subnet := helpers.ComputeSubnetForAttestation(fullGossipDomainRegistrySize, single)
			require.Equal(t, committeeSubnets[committeeIndex], subnet)
			buf := new(bytes.Buffer)
			_, signingErr = sender.Encoding().EncodeGossip(buf, single)
			require.NoError(t, signingErr)
			encoded[messageIndex] = buf.Bytes()
			messageTopics[messageIndex] = topics[committeeIndex]
		}
	}
	slotOneStart, err := slots.StartTime(genesisTime, 1)
	require.NoError(t, err)
	setupCompleted := time.Now()
	if !setupCompleted.Before(slotOneStart) {
		t.Fatalf("full-gossip fixture setup completed %s after slot-one start %s", setupCompleted.Sub(genesisTime), slotOneStart.Sub(genesisTime))
	}
	waitFullGossipDomainUntil(t, slotOneStart)
	require.Equal(t, uint64(1), uint64(slots.CurrentSlot(genesisTime)))
	if os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_WRITER") == "1" {
		forkChoice.Lock()
		selectedRoot, _, selectedFull, err := forkChoice.FullHead(t.Context())
		forkChoice.Unlock()
		require.NoError(t, err)
		require.Equal(t, genesisRoot, selectedRoot)
		require.Equal(t, true, selectedFull)
		canonicalRoot, canonicalFull := chain.CanonicalNodeAtSlot(1)
		require.Equal(t, genesisRoot, canonicalRoot)
		require.Equal(t, true, canonicalFull)
	}

	return &FullGossipDomainDiagnosticFixture{
		Chain: chain, Checkpoint: checkpoint, GenesisRoot: genesisRoot, GenesisValidatorsRoot: registryRoot,
		GenesisTime: genesisTime, SetupCompleted: setupCompleted, Committees: committees, Topics: topics,
		RuntimeNumCPU: runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0),
		service: syncService, sender: sender, pool: pool, counters: counters, encoded: encoded, topics: messageTopics,
	}
}

func waitFullGossipDomainUntil(t *testing.T, target time.Time) {
	t.Helper()
	if !time.Now().Before(target) {
		return
	}
	timer := time.NewTimer(time.Until(target))
	defer timer.Stop()
	select {
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	case <-timer.C:
	}
}

func newFullGossipDomainPeers(t *testing.T, genesisValidatorsRoot []byte) (*p2ptest.TestP2P, *p2ptest.TestP2P) {
	t.Helper()
	gossipParams := pubsub.DefaultGossipSubParams()
	gossipParams.D = 8
	gossipParams.Dlo = 6
	gossipParams.Dhi = 12
	gossipParams.HeartbeatInterval = 700 * time.Millisecond
	gossipParams.HistoryLength = 6
	gossipParams.HistoryGossip = 3
	options := []pubsub.Option{
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageIdFn(func(message *pubsubpb.Message) string { return p2p.MsgID(genesisValidatorsRoot, message) }),
		pubsub.WithPeerOutboundQueueSize(1000),
		pubsub.WithValidateQueueSize(1000),
		pubsub.WithMaxMessageSize(int(p2p.MaxMessageSize())),
		pubsub.WithGossipSubParams(gossipParams),
	}
	receiver := p2ptest.NewTestP2PWithPubsubOptions(t, options)
	sender := p2ptest.NewTestP2PWithPubsubOptions(t, options)
	digest := params.ForkDigest(0)
	receiver.Digest = digest
	sender.Digest = digest
	receiver.Connect(sender)
	t.Cleanup(func() {
		require.NoError(t, receiver.BHost.Close())
		require.NoError(t, sender.BHost.Close())
	})
	return receiver, sender
}

func waitFullGossipDomainPeers(t *testing.T, receiver, sender *p2ptest.TestP2P, topics []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready := true
		for _, topic := range topics {
			ready = ready && len(receiver.PubSub().ListPeers(topic)) > 0 && len(sender.PubSub().ListPeers(topic)) > 0
		}
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("persistent gossip peers did not discover all six subscribed topics")
		case <-ticker.C:
		}
	}
}

func fullGossipDomainValidatorStoragePointer(st state.ReadOnlyBeaconState) uintptr {
	value := reflect.ValueOf(st)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return 0
	}
	field := value.Elem().FieldByName("validatorsMultiValue")
	if !field.IsValid() || field.Kind() != reflect.Pointer || field.IsNil() {
		return 0
	}
	return field.Pointer()
}

// Publish sends one pre-encoded valid SingleAttestation from the persistent
// remote peer through the real GossipSub transport.
func (f *FullGossipDomainDiagnosticFixture) Publish(ctx context.Context, index int) error {
	if index < 0 || index >= len(f.encoded) {
		return fmt.Errorf("message index %d outside [0,%d)", index, len(f.encoded))
	}
	return f.sender.PublishToTopic(ctx, f.topics[index], f.encoded[index])
}

func (f *FullGossipDomainDiagnosticFixture) FirstIterator() <-chan struct{} {
	return f.counters.firstIterator
}

func (f *FullGossipDomainDiagnosticFixture) Stats() FullGossipDomainStats {
	f.counters.firstErrorMu.Lock()
	firstError := f.counters.firstError
	f.counters.firstErrorMu.Unlock()
	return FullGossipDomainStats{
		ValidationStarted: f.counters.validationStarted.Load(), ValidationAccepted: f.counters.validationAccepted.Load(),
		ValidationIgnored: f.counters.validationIgnored.Load(), ValidationRejected: f.counters.validationRejected.Load(),
		SubscriberStarted: f.counters.subscriberStarted.Load(), SubscriberCompleted: f.counters.subscriberCompleted.Load(),
		SubscriberFailed: f.counters.subscriberFailed.Load(), FeedEvents: f.counters.feedEvents.Load(),
		IteratorEntries: f.counters.iteratorEntries.Load(), IteratorExits: f.counters.iteratorExits.Load(),
		ActiveIterators: f.counters.activeIterators.Load(), MaximumActiveIterators: f.counters.maximumActiveIterator.Load(),
		UnaggregatedPoolEntries: f.pool.UnaggregatedAttestationCount(), FirstError: firstError,
	}
}

// TestDiagnosticFullGossipRealChainPreflight checks the public-chain fixture
// and one full remote gossip validation before the timed external test is run.
func TestDiagnosticFullGossipRealChainPreflight(t *testing.T) {
	if testing.Short() || !bytes.Equal([]byte("1"), []byte(os.Getenv("PRYSM_DIAGNOSTIC_FULL_GOSSIP_PREFLIGHT"))) {
		t.Skip("full-gossip diagnostic preflight is disabled")
	}
	fixture := NewFullGossipDomainDiagnosticFixture(t, false)
	require.NoError(t, fixture.Publish(t.Context(), 0))
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		stats := fixture.Stats()
		if stats.SubscriberCompleted == 1 && stats.FeedEvents == 1 {
			require.Equal(t, int64(1), stats.ValidationAccepted)
			require.Equal(t, int64(1), stats.FeedEvents)
			require.Equal(t, 1, stats.UnaggregatedPoolEntries)
			require.Equal(t, "", stats.FirstError)
			t.Logf("FULL_GOSSIP_PREFLIGHT stats=%+v genesis_root=%#x", stats, fixture.GenesisRoot)
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("full gossip preflight timed out: %+v", stats)
		case <-ticker.C:
		}
	}
}

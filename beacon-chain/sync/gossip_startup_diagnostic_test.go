package sync

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mockchain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	attpool "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	mocksync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

const gossipDiagnosticRegistrySize = 120_000

type gossipDiagnosticFixture struct {
	state       state.BeaconState
	committees  [][]primitives.ValidatorIndex
	atts        []eth.Att
	singles     []*eth.SingleAttestation
	electraAtts []eth.Att
}

func newGossipDiagnosticFixture(b *testing.B, registrySize, messages int, makeElectra bool) *gossipDiagnosticFixture {
	b.Helper()
	helpers.ClearCache()
	validators := make([]*eth.Validator, registrySize)
	balances := make([]uint64, registrySize)
	for i := range validators {
		validators[i] = &eth.Validator{
			PublicKey:                  make([]byte, 48),
			WithdrawalCredentials:      make([]byte, 32),
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
		}
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	st, err := util.NewBeaconStateHeze(func(pb *eth.BeaconStateHeze) error {
		pb.Validators = validators
		pb.Balances = balances
		pb.PreviousEpochParticipation = make([]byte, registrySize)
		pb.CurrentEpochParticipation = make([]byte, registrySize)
		pb.InactivityScores = make([]uint64, registrySize)
		return nil
	})
	require.NoError(b, err)
	require.NoError(b, st.SetSlot(1))

	committees := make([][]primitives.ValidatorIndex, 6)
	for committeeIndex := range committees {
		committees[committeeIndex], err = helpers.BeaconCommitteeFromState(b.Context(), st, 1, primitives.CommitteeIndex(committeeIndex))
		require.NoError(b, err)
	}
	if registrySize == gossipDiagnosticRegistrySize {
		require.Equal(b, 2500, len(committees[0]))
	}
	require.Equal(b, true, messages <= len(committees)*len(committees[0]))

	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(1), params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(b, err)
	atts := make([]eth.Att, messages)
	var electraAtts []eth.Att
	var singles []*eth.SingleAttestation
	if makeElectra {
		electraAtts = make([]eth.Att, messages)
		singles = make([]*eth.SingleAttestation, messages)
	}
	for i := range messages {
		committeeIndex := i % len(committees)
		position := i / len(committees)
		validatorIndex := committees[committeeIndex][position]
		key, keyErr := bls.RandKey()
		require.NoError(b, keyErr)
		validator := validators[validatorIndex]
		validator.PublicKey = key.PublicKey().Marshal()
		require.NoError(b, st.UpdateValidatorAtIndex(validatorIndex, validator))

		bits := bitfield.NewBitlist(uint64(len(committees[committeeIndex])))
		bits.SetBitAt(uint64(position), true)
		data := &eth.AttestationData{
			Slot:            1,
			CommitteeIndex:  primitives.CommitteeIndex(committeeIndex),
			BeaconBlockRoot: make([]byte, 32),
			Source:          &eth.Checkpoint{Root: make([]byte, 32)},
			Target:          &eth.Checkpoint{Root: make([]byte, 32)},
		}
		root, rootErr := signing.ComputeSigningRoot(data, domain)
		require.NoError(b, rootErr)
		atts[i] = &eth.Attestation{AggregationBits: bits, Data: data, Signature: key.Sign(root[:]).Marshal()}
		if makeElectra {
			electraData := &eth.AttestationData{
				Slot:            data.Slot,
				BeaconBlockRoot: data.BeaconBlockRoot,
				Source:          data.Source,
				Target:          data.Target,
			}
			electraRoot, electraRootErr := signing.ComputeSigningRoot(electraData, domain)
			require.NoError(b, electraRootErr)
			single := &eth.SingleAttestation{
				CommitteeId:   primitives.CommitteeIndex(committeeIndex),
				AttesterIndex: validatorIndex,
				Data:          electraData,
				Signature:     key.Sign(electraRoot[:]).Marshal(),
			}
			singles[i] = single
			electraAtts[i] = single.ToAttestationElectra(committees[committeeIndex])
		}
	}
	return &gossipDiagnosticFixture{state: st, committees: committees, atts: atts, singles: singles, electraAtts: electraAtts}
}

func BenchmarkGossipStartupValidation(b *testing.B) {
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	cfg.MaxCommitteesPerSlot = 6
	params.OverrideBeaconConfig(cfg)

	for _, tc := range []struct {
		registrySize int
		messages     int
	}{
		{registrySize: 15_000, messages: 1000},
		{registrySize: gossipDiagnosticRegistrySize, messages: 1000},
		{registrySize: gossipDiagnosticRegistrySize, messages: 2500},
	} {
		registrySize, messages := tc.registrySize, tc.messages
		name := fmt.Sprintf("registry_%d/messages_%d", registrySize, messages)
		b.Run(name, func(b *testing.B) {
			fixture := newGossipDiagnosticFixture(b, registrySize, messages, false)
			// Match startup after duties have already populated the committee cache.
			for committeeIndex := range fixture.committees {
				_, err := helpers.BeaconCommitteeFromState(b.Context(), fixture.state, 1, primitives.CommitteeIndex(committeeIndex))
				require.NoError(b, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				sets := make([]*signatureVerifier, 0, len(fixture.atts))
				for _, att := range fixture.atts {
					committeeIndex := int(att.GetCommitteeIndex())
					committee, err := helpers.BeaconCommitteeFromState(context.Background(), fixture.state, 1, primitives.CommitteeIndex(committeeIndex))
					require.NoError(b, err)
					res, err := validateAttesterData(context.Background(), att, committee)
					require.NoError(b, err)
					require.Equal(b, uint8(0), uint8(res))
					set, err := blocks.AttestationSignatureBatch(context.Background(), fixture.state, []eth.Att{att})
					require.NoError(b, err)
					sets = append(sets, &signatureVerifier{set: set, resChan: make(chan error, 1)})
				}
				for start := 0; start < len(sets); start += 64 {
					end := min(start+64, len(sets))
					verifyBatch(sets[start:end])
					for _, set := range sets[start:end] {
						require.NoError(b, <-set.resChan)
					}
				}
			}
		})
	}
}

// BenchmarkGossipStartupLegacyPool measures the production pool stages excluded
// from BenchmarkGossipStartupValidation. The historical runs did not enable the
// experimental attestation pool.
func BenchmarkGossipStartupLegacyPool(b *testing.B) {
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	cfg.MaxCommitteesPerSlot = 6
	params.OverrideBeaconConfig(cfg)

	const messages = 15_000
	fixture := newGossipDiagnosticFixture(b, gossipDiagnosticRegistrySize, messages, true)

	b.Run("subscriber_ingest", func(b *testing.B) {
		for b.Loop() {
			pool := attpool.NewAttCaches()
			jobs := make(chan eth.Att)
			var wg sync.WaitGroup
			for range 64 {
				wg.Go(func() {
					for att := range jobs {
						exists, err := pool.HasAggregatedAttestation(att)
						require.NoError(b, err)
						if !exists {
							require.NoError(b, pool.SaveUnaggregatedAttestation(att))
						}
					}
				})
			}
			for _, att := range fixture.electraAtts {
				jobs <- att
			}
			close(jobs)
			wg.Wait()
			require.Equal(b, messages, len(pool.UnaggregatedAttestations()))
		}
	})

	pool := attpool.NewAttCaches()
	require.NoError(b, pool.SaveUnaggregatedAttestations(fixture.electraAtts))
	b.Run("six_aggregator_selections", func(b *testing.B) {
		for b.Loop() {
			var wg sync.WaitGroup
			durations := make(chan time.Duration, len(fixture.committees))
			for committeeIndex := range fixture.committees {
				wg.Go(func() {
					start := time.Now()
					atts := pool.UnaggregatedAttestationsBySlotIndexElectra(
						b.Context(), 1, primitives.CommitteeIndex(committeeIndex),
					)
					require.Equal(b, 2500, len(atts))
					as := make([]eth.Att, len(atts))
					for i := range atts {
						as[i] = atts[i].Clone()
					}
					aggregated, err := attaggregation.Aggregate(as)
					require.NoError(b, err)
					require.Equal(b, 1, len(aggregated))
					require.Equal(b, 2500, int(aggregated[0].GetAggregationBits().Count()))
					durations <- time.Since(start)
				})
			}
			wg.Wait()
			close(durations)
			var maxDuration time.Duration
			for duration := range durations {
				maxDuration = max(maxDuration, duration)
			}
			b.ReportMetric(float64(maxDuration.Nanoseconds()), "selection-max-ns")
		}
	})

	b.Run("proposer_pool_snapshot", func(b *testing.B) {
		for b.Loop() {
			atts := pool.UnaggregatedAttestations()
			require.Equal(b, messages, len(atts))
		}
	})
}

// BenchmarkGossipStartupFullValidation includes gossip encoding/decoding, real
// database presence checks, the production verifier routine and legacy pool
// subscriber. Chain and fork-choice answers remain mocked.
func BenchmarkGossipStartupFullValidation(b *testing.B) {
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	cfg.MaxCommitteesPerSlot = 6
	params.OverrideBeaconConfig(cfg)

	const messages = 15_000
	fixture := newGossipDiagnosticFixture(b, gossipDiagnosticRegistrySize, messages, true)
	database := dbtest.SetupDB(b)
	root := [32]byte{}
	require.NoError(b, database.SaveState(b.Context(), fixture.state, root))
	p2pService := p2ptest.NewFuzzTestP2P()
	digest := params.ForkDigest(slots.ToEpoch(1))
	encoded := make([]*pubsub.Message, messages)
	for i, single := range fixture.singles {
		subnet := helpers.ComputeSubnetForAttestation(gossipDiagnosticRegistrySize, single)
		topic := fmt.Sprintf(p2p.GossipTypeMapping[reflect.TypeFor[*eth.Attestation]()], digest, subnet) + p2pService.Encoding().ProtocolSuffix()
		buf := new(bytes.Buffer)
		_, err := p2pService.Encoding().EncodeGossip(buf, single)
		require.NoError(b, err)
		encoded[i] = &pubsub.Message{Message: &pubsubpb.Message{Data: buf.Bytes(), Topic: &topic}}
	}

	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		genesis := time.Now().Add(-time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
		chain := &mockchain.ChainService{
			Genesis:            genesis,
			ValidatorsRoot:     bytesutil.ToBytes32(fixture.state.GenesisValidatorsRoot()),
			DB:                 database,
			State:              fixture.state,
			InitSyncBlockRoots: map[[32]byte]bool{root: true},
		}
		notifier := chain.OperationNotifier()
		serviceCtx, cancel := context.WithCancel(b.Context())
		s := &Service{
			ctx: serviceCtx,
			cfg: &config{
				initialSync:         &mocksync.Sync{IsSyncing: false},
				p2p:                 p2pService,
				beaconDB:            database,
				chain:               chain,
				clock:               startup.NewClock(genesis, chain.ValidatorsRoot),
				attestationNotifier: notifier,
				attPool:             attpool.NewAttCaches(),
				batchVerifierLimit:  64,
			},
			blkRootToPendingAtts:             make(map[[32]byte][]any),
			seenUnAggregatedAttestationCache: lruwrpr.New(messages),
			signatureChan:                    make(chan *signatureVerifier, 64),
			ffgVotes:                         newFFGVoteCounters(),
		}
		s.initCaches()
		go s.verifierRoutine()
		events := make(chan *feed.Event)
		drained := make(chan struct{}, messages)
		subscription := notifier.OperationFeed().Subscribe(events)
		var notified atomic.Int64
		go func() {
			for {
				select {
				case <-serviceCtx.Done():
					return
				case <-events:
					notified.Add(1)
					drained <- struct{}{}
				}
			}
		}()
		b.StartTimer()
		jobs := make(chan *pubsub.Message)
		var accepted atomic.Int64
		var ignored atomic.Int64
		var wg sync.WaitGroup
		for range 64 {
			wg.Go(func() {
				for msg := range jobs {
					result, err := s.validateCommitteeIndexBeaconAttestation(b.Context(), "remote", msg)
					if result != pubsub.ValidationAccept {
						ignored.Add(1)
						continue
					}
					require.NoError(b, err)
					require.NoError(b, s.committeeIndexBeaconAttestationSubscriber(b.Context(), msg.ValidatorData.(eth.Att)))
					accepted.Add(1)
				}
			})
		}
		for _, msg := range encoded {
			jobs <- msg
		}
		close(jobs)
		wg.Wait()
		acceptedCount := accepted.Load()
		for int64(0) < acceptedCount {
			<-drained
			acceptedCount--
		}
		b.StopTimer()
		subscription.Unsubscribe()
		cancel()
		require.Equal(b, int64(messages), accepted.Load())
		require.Equal(b, int64(0), ignored.Load())
		require.Equal(b, int64(messages), notified.Load())
	}
}

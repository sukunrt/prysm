//go:build !minimal

package validator

import (
	"context"
	"strings"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	stategenmock "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

const historicalStateRootValidatorCount = 120_000

type historicalStateRootFixture struct {
	preState   state.BeaconState
	block      interfaces.SignedBeaconBlock
	server     *Server
	parentRoot [32]byte
}

type historicalStateRootFixtures struct {
	clean         historicalStateRootFixture
	residualDirty historicalStateRootFixture
}

var (
	historicalStateRootStateSink state.BeaconState
	historicalStateRootHashSink  [32]byte
)

func newHistoricalStateRootFixtures(b *testing.B) historicalStateRootFixtures {
	b.Helper()
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	params.OverrideBeaconConfig(cfg)
	b.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()
	b.Cleanup(helpers.ClearCache)

	validatorKeys, validatorPubkeys, err := interop.DeterministicallyGenerateKeys(0, historicalStateRootValidatorCount)
	require.NoError(b, err)
	validators := make([]*ethpb.Validator, historicalStateRootValidatorCount)
	balances := make([]uint64, historicalStateRootValidatorCount)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey:                  validatorPubkeys[i].Marshal(),
			WithdrawalCredentials:      make([]byte, 32),
			EffectiveBalance:           cfg.MaxEffectiveBalance,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  cfg.FarFutureEpoch,
			WithdrawableEpoch:          cfg.FarFutureEpoch,
		}
		balances[i] = cfg.MaxEffectiveBalance
	}
	parentState, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Validators = validators
		pb.Balances = balances
		pb.PreviousEpochParticipation = make([]byte, len(validators))
		pb.CurrentEpochParticipation = make([]byte, len(validators))
		pb.InactivityScores = make([]uint64, len(validators))
		pb.Fork = &ethpb.Fork{
			PreviousVersion: cfg.GloasForkVersion,
			CurrentVersion:  cfg.HezeForkVersion,
			Epoch:           0,
		}
		return nil
	})
	require.NoError(b, err)
	return prepareHistoricalStateRootFixtures(b, parentState, validatorKeys, nil)
}

func newHistoricalCompact3StateRootFixtures(b *testing.B) historicalStateRootFixtures {
	b.Helper()
	lateFixture := newDiagnosticLateFixture(b)
	pool := diagnosticLateCounterfactualPool(b, lateFixture, true)
	packed, _ := diagnosticLatePack(b, lateFixture, pool)
	require.Equal(b, 3, len(packed))

	compact := make([]*ethpb.AttestationGloas, 0, len(packed))
	for _, att := range packed {
		gloasAtt, ok := ethpb.AttestationGloasFromAtt(att)
		require.Equal(b, true, ok)
		compact = append(compact, gloasAtt)
	}
	return prepareHistoricalStateRootFixtures(
		b, lateFixture.creditedState.Copy(), lateFixture.keys, compact,
	)
}

func newHistoricalDensity8StateRootFixtures(b *testing.B) historicalStateRootFixtures {
	b.Helper()
	lateFixture := newDiagnosticLateFixture(b)
	preState := lateFixture.state.Copy()
	source := preState.PreviousJustifiedCheckpoint()
	target := preState.CurrentJustifiedCheckpoint()
	fullCommitteeCounts := []int{2500, 2500, 2500, 2500, 2500, 2500}
	partials := make([]ethpb.Att, 0, 8*len(fullCommitteeCounts))
	for slot := primitives.Slot(88); slot <= 95; slot++ {
		root := [32]byte{byte(slot), 0xd8}
		require.NoError(b, preState.UpdateBlockRootAtIndex(
			uint64(slot%params.BeaconConfig().SlotsPerHistoricalRoot), root,
		))
		partials = append(partials, diagnosticLateAggregateSet(
			b,
			preState,
			lateFixture.keys,
			slot,
			root,
			source,
			target,
			fullCommitteeCounts,
		)...)
	}
	packed, err := diagnosticLateNormalizeDedupAggregate(partials)
	require.NoError(b, err)
	require.Equal(b, 8, len(packed))
	diagnosticLateRequireSignatures(b, preState, packed)

	participantPositions := uint64(0)
	density := make([]*ethpb.AttestationGloas, 0, len(packed))
	for _, att := range packed {
		participantPositions += att.GetAggregationBits().Count()
		gloasAtt, ok := ethpb.AttestationGloasFromAtt(att)
		require.Equal(b, true, ok)
		density = append(density, gloasAtt)
	}
	require.Equal(b, uint64(120_000), participantPositions)
	return prepareHistoricalStateRootFixtures(b, preState, lateFixture.keys, density)
}

func prepareHistoricalStateRootFixtures(
	b *testing.B,
	parentState state.BeaconState,
	validatorKeys []bls.SecretKey,
	attestations []*ethpb.AttestationGloas,
) historicalStateRootFixtures {
	b.Helper()
	cfg := params.BeaconConfig()
	require.Equal(b, historicalStateRootValidatorCount, parentState.NumValidators())
	require.Equal(b, version.Heze, parentState.Version())
	require.Equal(b, historicalStateRootValidatorCount, len(validatorKeys))

	syncPubkeys := make([][]byte, cfg.SyncCommitteeSize)
	for i := range syncPubkeys {
		syncPubkeys[i] = validatorKeys[i].PublicKey().Marshal()
	}
	aggregateSyncPubkey, err := bls.AggregatePublicKeys(syncPubkeys)
	require.NoError(b, err)
	newSyncCommittee := func() *ethpb.SyncCommittee {
		pubkeys := make([][]byte, len(syncPubkeys))
		for i := range syncPubkeys {
			pubkeys[i] = append([]byte(nil), syncPubkeys[i]...)
		}
		return &ethpb.SyncCommittee{
			Pubkeys:         pubkeys,
			AggregatePubkey: aggregateSyncPubkey.Marshal(),
		}
	}
	require.NoError(b, parentState.SetCurrentSyncCommittee(newSyncCommittee()))
	require.NoError(b, parentState.SetNextSyncCommittee(newSyncCommittee()))

	const blockSlot = primitives.Slot(97)
	const parentSlot = blockSlot - 1
	require.NoError(b, parentState.SetSlot(parentSlot))

	parentHeader := &ethpb.BeaconBlockHeader{
		Slot:          parentSlot,
		ProposerIndex: 0,
		ParentRoot:    make([]byte, 32),
		StateRoot:     make([]byte, 32),
		BodyRoot:      make([]byte, 32),
	}
	require.NoError(b, parentState.SetLatestBlockHeader(parentHeader))

	latestBlockHash := [32]byte{0x42}
	parentBidBlockHash := [32]byte{0x24}
	require.NoError(b, parentState.SetLatestBlockHash(latestBlockHash))
	parentBid, err := consensusblocks.WrappedROExecutionPayloadBid(
		util.HydrateExecutionPayloadBid(&ethpb.ExecutionPayloadBid{
			Slot:      parentSlot,
			BlockHash: parentBidBlockHash[:],
		}),
	)
	require.NoError(b, err)
	require.NoError(b, parentState.SetExecutionPayloadBid(parentBid))

	// Advance an actual slot-96 parent once. ProcessSlot hashes the parent and
	// then dirties the slot, roots-ring, header, and payload-availability fields.
	// The residual-dirty fixture retains that exact state. Its clean companion
	// is an independent copy of the same values whose base root is warmed once.
	preAdvanceRoot, err := parentHeader.HashTreeRoot()
	require.NoError(b, err)
	require.Equal(b, true, transition.NextSlotState(preAdvanceRoot[:], blockSlot) == nil)
	residualDirtyState, err := transition.ProcessSlotsUsingNextSlotCache(
		b.Context(), parentState, preAdvanceRoot[:], blockSlot,
	)
	require.NoError(b, err)
	require.Equal(b, blockSlot, residualDirtyState.Slot())
	parentRoot, err := residualDirtyState.LatestBlockHeader().HashTreeRoot()
	require.NoError(b, err)
	storedParentRoot, err := helpers.BlockRootAtSlot(residualDirtyState, parentSlot)
	require.NoError(b, err)
	require.DeepEqual(b, parentRoot[:], storedParentRoot)

	proposerIndex, err := helpers.BeaconProposerIndex(b.Context(), residualDirtyState)
	require.NoError(b, err)
	randaoReveal, err := util.RandaoReveal(residualDirtyState, slots.ToEpoch(blockSlot), validatorKeys)
	require.NoError(b, err)
	prevRandao, err := helpers.RandaoMix(residualDirtyState, slots.ToEpoch(blockSlot))
	require.NoError(b, err)
	blockHash := [32]byte{0x99}
	blockBid := &ethpb.SignedExecutionPayloadBid{
		Message: util.HydrateExecutionPayloadBid(&ethpb.ExecutionPayloadBid{
			ParentBlockHash:  latestBlockHash[:],
			ParentBlockRoot:  parentRoot[:],
			BlockHash:        blockHash[:],
			PrevRandao:       prevRandao,
			BuilderIndex:     cfg.BuilderIndexSelfBuild,
			Slot:             blockSlot,
			Value:            0,
			ExecutionPayment: 0,
		}),
		Signature: append([]byte(nil), common.InfiniteSignature[:]...),
	}
	body := util.HydrateBeaconBlockBodyGloas(&ethpb.BeaconBlockBodyGloas{
		RandaoReveal:              randaoReveal,
		Eth1Data:                  residualDirtyState.Eth1Data(),
		Attestations:              attestations,
		SignedExecutionPayloadBid: blockBid,
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      bitfield.NewBitvector512(),
			SyncCommitteeSignature: append([]byte(nil), common.InfiniteSignature[:]...),
		},
		ParentExecutionRequests: &enginev1.ExecutionRequestsGloas{},
	})
	protoBlock := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:          blockSlot,
			ProposerIndex: proposerIndex,
			ParentRoot:    parentRoot[:],
			Body:          body,
		},
	})
	block, err := consensusblocks.NewSignedBeaconBlock(protoBlock)
	require.NoError(b, err)

	cleanState := residualDirtyState.Copy()
	_, err = cleanState.HashTreeRoot(b.Context())
	require.NoError(b, err)
	fixtures := historicalStateRootFixtures{
		clean:         makeHistoricalStateRootFixture(cleanState, block, parentRoot),
		residualDirty: makeHistoricalStateRootFixture(residualDirtyState, block, parentRoot),
	}
	validateHistoricalStateRootFixture(b, fixtures.clean, true, len(attestations))
	validateHistoricalStateRootFixture(b, fixtures.residualDirty, false, len(attestations))
	return fixtures
}

func makeHistoricalStateRootFixture(
	preState state.BeaconState,
	block interfaces.SignedBeaconBlock,
	parentRoot [32]byte,
) historicalStateRootFixture {
	stateGen := stategenmock.NewService()
	stateGen.AddStateForRoot(preState, parentRoot)
	return historicalStateRootFixture{
		preState:   preState,
		block:      block,
		server:     &Server{StateGen: stateGen},
		parentRoot: parentRoot,
	}
}

func validateHistoricalStateRootFixture(
	b *testing.B,
	fixture historicalStateRootFixture,
	baseRootIsWarm bool,
	expectedAttestations int,
) {
	b.Helper()
	ctx := context.Background()
	require.Equal(b, historicalStateRootValidatorCount, fixture.preState.NumValidators())
	require.Equal(b, primitives.Slot(97), fixture.preState.Slot())
	require.Equal(b, false, features.Get().EnableProposerPreprocessing)
	require.Equal(b, expectedAttestations, len(fixture.block.Block().Body().Attestations()))
	storedRoot, err := helpers.BlockRootAtSlot(fixture.preState, 96)
	require.NoError(b, err)
	require.DeepEqual(b, fixture.parentRoot[:], storedRoot)
	require.Equal(b, true, transition.NextSlotState(fixture.parentRoot[:], 97) == nil)

	// The clean fixture was explicitly rooted once. The residual-dirty fixture
	// must never be rooted directly: computing through a copy below proves the
	// block is accepted without clearing its pending slot-advance changes.
	if baseRootIsWarm {
		_, err = fixture.preState.HashTreeRoot(ctx)
		require.NoError(b, err)
	}
	hook := logtest.NewGlobal()
	root, postState, err := fixture.server.computePostBlockStateAndRoot(ctx, fixture.block)
	require.NoError(b, err)
	require.Equal(b, 32, len(root))
	require.NotNil(b, postState)
	require.Equal(b, expectedAttestations, len(fixture.block.Block().Body().Attestations()))
	for _, entry := range hook.AllEntries() {
		if strings.HasPrefix(entry.Message, "Retrying block construction") {
			b.Fatalf("fixture used retry fallback: %s", entry.Message)
		}
	}
}

func benchmarkHistoricalStateRootScenario(
	b *testing.B,
	fixtures historicalStateRootFixtures,
	expectedAttestations int,
	participantPositions int,
) {
	b.Helper()
	ctx := context.Background()
	b.ReportMetric(float64(expectedAttestations), "attestations")
	b.ReportMetric(float64(participantPositions), "participant_positions")

	b.Run("calculate_post_state/prepared_at_block_slot", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			postState, err := transition.CalculatePostState(ctx, fixtures.clean.preState, fixtures.clean.block)
			if err != nil {
				b.Fatal(err)
			}
			historicalStateRootStateSink = postState
		}
	})

	b.Run("hash_each_fresh_post_state_once/clean_pre_state", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			b.StopTimer()
			postState, err := transition.CalculatePostState(ctx, fixtures.clean.preState, fixtures.clean.block)
			if err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			root, err := postState.HashTreeRoot(ctx)
			if err != nil {
				b.Fatal(err)
			}
			historicalStateRootHashSink = root
		}
	})

	b.Run("hash_each_fresh_post_state_once/residual_slot_advance_dirty", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			b.StopTimer()
			postState, err := transition.CalculatePostState(
				ctx, fixtures.residualDirty.preState, fixtures.residualDirty.block,
			)
			if err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			root, err := postState.HashTreeRoot(ctx)
			if err != nil {
				b.Fatal(err)
			}
			historicalStateRootHashSink = root
		}
	})

	b.Run("full_proposer_wrapper/prepared_at_block_slot", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			root, postState, err := fixtures.clean.server.computePostBlockStateAndRoot(ctx, fixtures.clean.block)
			if err != nil {
				b.Fatal(err)
			}
			if len(fixtures.clean.block.Block().Body().Attestations()) != expectedAttestations {
				b.Fatalf("state-root retry changed attestation count: got %d, want %d", len(fixtures.clean.block.Block().Body().Attestations()), expectedAttestations)
			}
			historicalStateRootHashSink = [32]byte(root)
			historicalStateRootStateSink = postState
		}
	})
}

// BenchmarkHistoricalStateRootEmptyBlock120K isolates the consensus
// transition and state-root tail of proposal construction with a mainnet-size
// validator registry. The pre-state is already at block slot 97, so slot and
// epoch/round advancement are intentionally outside these measurements.
func BenchmarkHistoricalStateRootEmptyBlock120K(b *testing.B) {
	fixtures := newHistoricalStateRootFixtures(b)
	benchmarkHistoricalStateRootScenario(b, fixtures, 0, 0)
}

// BenchmarkHistoricalStateRootCompact3Block120K measures the same state-root
// stages with three valid on-chain attestations packed from the retained
// historical vote shapes. The retained slot-97 block had eight attestations,
// so this is explicitly a compact lower-density fixture.
func BenchmarkHistoricalStateRootCompact3Block120K(b *testing.B) {
	fixtures := newHistoricalCompact3StateRootFixtures(b)
	benchmarkHistoricalStateRootScenario(b, fixtures, 3, 44_847)
}

// BenchmarkHistoricalStateRootDensity8Block120K is a synthetic maximum-density
// count control, not a replay of the retained slot-97 block's eight on-chain
// attestations. It uses eight distinct valid previous-round votes at slots 88
// through 95, each covering six full committees, for 120,000 controlled
// participant positions; that participant count was not observed historically.
func BenchmarkHistoricalStateRootDensity8Block120K(b *testing.B) {
	fixtures := newHistoricalDensity8StateRootFixtures(b)
	benchmarkHistoricalStateRootScenario(b, fixtures, 8, 120_000)
}

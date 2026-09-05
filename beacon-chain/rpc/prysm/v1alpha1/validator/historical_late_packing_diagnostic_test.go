package validator

import (
	"context"
	"encoding/hex"
	"os"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	mockchain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	attpool "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const (
	diagnosticLateStateSlot     = primitives.Slot(96)
	diagnosticLateHeadSlot      = primitives.Slot(95)
	diagnosticLateRound         = primitives.Round(11)
	diagnosticLateCommitteeSize = 2500
)

var diagnosticLateOldCounts = map[primitives.Slot][]int{
	91: {2486, 2488, 2481, 2491, 2487, 2487},
	93: {2489, 2485, 2484, 2487, 2492, 2490},
}

type diagnosticLateFixture struct {
	state             state.BeaconState
	creditedState     state.BeaconState
	oldGloas          []ethpb.Att
	oldElectra        []ethpb.Att
	fresh             []ethpb.Att
	compactRetained   []ethpb.Att
	normalizedPruned  []ethpb.Att
	chain             *mockchain.ChainService
	keys              []bls.SecretKey
	totalOldPositions int
	freshPositions    int
	stateSlot         primitives.Slot
	headSlot          primitives.Slot
}

type diagnosticLateCoverageKey struct {
	slot primitives.Slot
	root [32]byte
}

func diagnosticLateRoot(tb testing.TB, value string) [32]byte {
	tb.Helper()
	bts, err := hex.DecodeString(value)
	require.NoError(tb, err)
	require.Equal(tb, 32, len(bts))
	return [32]byte(bts)
}

func diagnosticLateCloneAtts(atts []ethpb.Att) []ethpb.Att {
	clones := make([]ethpb.Att, len(atts))
	for i := range atts {
		clones[i] = atts[i].Clone()
	}
	return clones
}

func diagnosticLateAggregateSet(
	tb testing.TB,
	st state.BeaconState,
	keys []bls.SecretKey,
	slot primitives.Slot,
	beaconRoot [32]byte,
	source *ethpb.Checkpoint,
	target *ethpb.Checkpoint,
	counts []int,
) []ethpb.Att {
	tb.Helper()
	data := &ethpb.AttestationData{
		Slot:            slot,
		CommitteeIndex:  0,
		BeaconBlockRoot: append([]byte(nil), beaconRoot[:]...),
		Source:          source.Copy(),
		Target:          target.Copy(),
	}
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(slot), params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(tb, err)
	signingRoot, err := signing.ComputeSigningRoot(data, domain)
	require.NoError(tb, err)

	atts := make([]ethpb.Att, 0, len(counts))
	for committeeIndex, count := range counts {
		committee, err := helpers.BeaconCommitteeFromCache(tb.Context(), st, slot, primitives.CommitteeIndex(committeeIndex))
		require.NoError(tb, err)
		require.Equal(tb, diagnosticLateCommitteeSize, len(committee))
		require.Equal(tb, true, count > 1 && count <= len(committee))

		bits := bitfield.NewBitlist(uint64(len(committee)))
		for position := 0; position < count; position++ {
			bits.SetBitAt(uint64(position), true)
		}
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(committeeIndex), true)
		signatures := make([]bls.Signature, count)
		for position, validatorIndex := range committee[:count] {
			signatures[position] = keys[validatorIndex].Sign(signingRoot[:])
		}
		atts = append(atts, &ethpb.AttestationElectra{
			AggregationBits: bits,
			CommitteeBits:   committeeBits,
			Data:            data.Copy(),
			Signature:       bls.AggregateSignatures(signatures).Marshal(),
		})
	}
	return atts
}

func newDiagnosticLateFixture(tb testing.TB) diagnosticLateFixture {
	return newDiagnosticLateFixtureAt(tb, diagnosticLateStateSlot, diagnosticLateHeadSlot)
}

func newDiagnosticLateFixtureAt(tb testing.TB, stateSlot, headSlot primitives.Slot) diagnosticLateFixture {
	tb.Helper()
	params.SetupTestConfigCleanup(tb)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = diagnosticLateCommitteeSize
	params.OverrideBeaconConfig(cfg)
	tb.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()
	tb.Cleanup(helpers.ClearCache)

	keys, pubkeys, err := interop.DeterministicallyGenerateKeys(0, 120_000)
	require.NoError(tb, err)
	validators := make([]*ethpb.Validator, 120_000)
	balances := make([]uint64, len(validators))
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey:                  pubkeys[i].Marshal(),
			WithdrawalCredentials:      make([]byte, 32),
			EffectiveBalance:           cfg.MaxEffectiveBalance,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  cfg.FarFutureEpoch,
			WithdrawableEpoch:          cfg.FarFutureEpoch,
		}
		balances[i] = cfg.MaxEffectiveBalance
	}

	old91Root := diagnosticLateRoot(tb, "7b609d078b5fe84889cd4d55cbdb1dce39842760ece749b01b05943e5f93f23a")
	old93Root := diagnosticLateRoot(tb, "bd3087a87d2ce719d2523f4f7dec64af872539b649c0133de27387bae0b78a1c")
	targetRoot := [32]byte{0x88}
	freshRoot := [32]byte{0x95}
	sourceRoot := [32]byte{0x77}
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	stateRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = make([]byte, 32)
		stateRoots[i] = make([]byte, 32)
	}
	targetSlot, err := slots.FFGTargetSlot(diagnosticLateRound)
	require.NoError(tb, err)
	require.Equal(tb, primitives.Slot(87), targetSlot)
	for slot, root := range map[primitives.Slot][32]byte{
		targetSlot: targetRoot,
		91:         old91Root,
		93:         old93Root,
		95:         freshRoot,
	} {
		blockRoots[uint64(slot)%uint64(cfg.SlotsPerHistoricalRoot)] = append([]byte(nil), root[:]...)
	}

	st, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Slot = stateSlot
		pb.Validators = validators
		pb.Balances = balances
		pb.BlockRoots = blockRoots
		pb.StateRoots = stateRoots
		pb.PreviousEpochParticipation = make([]byte, len(validators))
		pb.CurrentEpochParticipation = make([]byte, len(validators))
		pb.InactivityScores = make([]uint64, len(validators))
		pb.Fork = &ethpb.Fork{
			PreviousVersion: cfg.GloasForkVersion,
			CurrentVersion:  cfg.HezeForkVersion,
			Epoch:           0,
		}
		pb.PreviousJustifiedCheckpoint = &ethpb.Checkpoint{Epoch: 10, Root: sourceRoot[:]}
		pb.CurrentJustifiedCheckpoint = &ethpb.Checkpoint{Epoch: 11, Root: targetRoot[:]}
		pb.LatestExecutionPayloadBid.Slot = headSlot
		return nil
	})
	require.NoError(tb, err)
	require.Equal(tb, uint64(6), helpers.SlotCommitteeCount(120_000))
	require.Equal(tb, uint64(64), cfg.MaxCommitteesPerSlot)
	require.NoError(tb, helpers.UpdateCommitteeCache(tb.Context(), st, slots.ToEpoch(91)))

	source := st.PreviousJustifiedCheckpoint()
	target := &ethpb.Checkpoint{Epoch: diagnosticLateRound, Root: targetRoot[:]}
	oldGloas := make([]ethpb.Att, 0, 12)
	oldElectra := make([]ethpb.Att, 0, 12)
	totalOldPositions := 0
	for _, slot := range []primitives.Slot{91, 93} {
		root := old91Root
		if slot == 93 {
			root = old93Root
		}
		counts := diagnosticLateOldCounts[slot]
		for _, count := range counts {
			totalOldPositions += count
		}
		electraSet := diagnosticLateAggregateSet(tb, st, keys, slot, root, source, target, counts)
		oldElectra = append(oldElectra, electraSet...)
		for _, att := range electraSet {
			gloasAtt, ok := ethpb.AttestationGloasFromAtt(att)
			require.Equal(tb, true, ok)
			oldGloas = append(oldGloas, gloasAtt)
		}
	}
	freshCounts := []int{2500, 2500, 2500, 2500, 2500, 2500}
	fresh := diagnosticLateAggregateSet(tb, st, keys, diagnosticLateHeadSlot, freshRoot, source, target, freshCounts)

	creditedState := st.Copy()
	credited := make([]byte, len(validators))
	flags := byte(1<<cfg.TimelySourceFlagIndex | 1<<cfg.TimelyTargetFlagIndex | 1<<cfg.TimelyHeadFlagIndex)
	for _, att := range oldElectra {
		committees, err := helpers.AttestationCommitteesFromState(tb.Context(), st, att)
		require.NoError(tb, err)
		indexed, err := attestation.ConvertToIndexed(tb.Context(), att, committees...)
		require.NoError(tb, err)
		for _, index := range indexed.GetAttestingIndices() {
			credited[index] = flags
		}
	}
	require.NoError(tb, creditedState.SetPreviousParticipationBits(credited))

	headSlotValue := headSlot
	currentSlot := stateSlot
	headRoot := freshRoot
	if headSlot != diagnosticLateHeadSlot {
		headRoot = [32]byte{byte(headSlot)}
	}
	chain := &mockchain.ChainService{
		MockHeadSlot: &headSlotValue,
		Slot:         &currentSlot,
		Root:         append([]byte(nil), headRoot[:]...),
		TargetRoot:   targetRoot,
		BlockSlot:    diagnosticLateHeadSlot,
	}
	compactRetained := append(diagnosticLateCloneAtts(oldGloas), diagnosticLateCloneAtts(fresh)...)
	normalizedPruned := diagnosticLateCloneAtts(fresh)
	return diagnosticLateFixture{
		state:             st,
		creditedState:     creditedState,
		oldGloas:          oldGloas,
		oldElectra:        oldElectra,
		fresh:             fresh,
		compactRetained:   compactRetained,
		normalizedPruned:  normalizedPruned,
		chain:             chain,
		keys:              keys,
		totalOldPositions: totalOldPositions,
		freshPositions:    15_000,
		stateSlot:         stateSlot,
		headSlot:          headSlot,
	}
}

func diagnosticLatePool(tb testing.TB, atts []ethpb.Att) attpool.Pool {
	tb.Helper()
	pool := attpool.NewPool()
	require.NoError(tb, pool.SaveAggregatedAttestations(diagnosticLateCloneAtts(atts)))
	return pool
}

// diagnosticLateCounterfactualPool executes the same Electra-form deletion
// against either the historical Gloas wire form or its normalized Electra
// form. The former reproduces the old version-key mismatch; the latter is the
// counterfactual in which the already included votes are removed.
func diagnosticLateCounterfactualPool(tb testing.TB, fixture diagnosticLateFixture, retainGloas bool) attpool.Pool {
	tb.Helper()
	pool := attpool.NewPool()
	old := fixture.oldElectra
	wantAfterDelete := 0
	if retainGloas {
		old = fixture.oldGloas
		wantAfterDelete = len(fixture.oldGloas)
	}
	require.NoError(tb, pool.SaveAggregatedAttestations(diagnosticLateCloneAtts(old)))
	for _, includedElectra := range fixture.oldElectra {
		require.NoError(tb, pool.DeleteAggregatedAttestation(includedElectra))
	}
	require.Equal(tb, wantAfterDelete, len(pool.AggregatedAttestations()))
	require.NoError(tb, pool.SaveAggregatedAttestations(diagnosticLateCloneAtts(fixture.fresh)))
	require.Equal(tb, wantAfterDelete+len(fixture.fresh), len(pool.AggregatedAttestations()))
	return pool
}

func diagnosticLateServer(pool attpool.Pool, chain *mockchain.ChainService) *Server {
	return &Server{
		AttPool:           pool,
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		Eth1InfoFetcher:   &mockExecution.Chain{NotConnected: true},
	}
}

func diagnosticLateNormalizeDedupAggregate(atts []ethpb.Att) (proposerAtts, error) {
	versionAtts := make([]ethpb.Att, 0, len(atts))
	for _, att := range atts {
		converted, ok := ethpb.AttestationElectraFromAtt(att)
		if ok {
			versionAtts = append(versionAtts, converted)
		}
	}
	deduped, err := proposerAtts(versionAtts).dedup()
	if err != nil {
		return nil, err
	}
	attsByID := make(map[attestation.Id][]ethpb.Att, len(deduped))
	for _, att := range deduped {
		id, err := attestation.NewId(att, attestation.Data)
		if err != nil {
			return nil, err
		}
		attsByID[id] = append(attsByID[id], att)
	}
	for id, grouped := range attsByID {
		grouped, err = attaggregation.Aggregate(grouped)
		if err != nil {
			return nil, err
		}
		attsByID[id] = grouped
	}
	onChain, err := onChainAggregates(attsByID)
	if err != nil {
		return nil, err
	}
	return onChain.dedup()
}

func diagnosticLateCoverage(
	tb testing.TB,
	st state.ReadOnlyBeaconState,
	atts []ethpb.Att,
) map[diagnosticLateCoverageKey]map[primitives.ValidatorIndex]struct{} {
	tb.Helper()
	coverage := make(map[diagnosticLateCoverageKey]map[primitives.ValidatorIndex]struct{})
	for _, att := range atts {
		var root [32]byte
		copy(root[:], att.GetData().BeaconBlockRoot)
		key := diagnosticLateCoverageKey{slot: att.GetData().Slot, root: root}
		if coverage[key] == nil {
			coverage[key] = make(map[primitives.ValidatorIndex]struct{})
		}
		committees, err := helpers.AttestationCommitteesFromState(tb.Context(), st, att)
		require.NoError(tb, err)
		indexed, err := attestation.ConvertToIndexed(tb.Context(), att, committees...)
		require.NoError(tb, err)
		for _, index := range indexed.GetAttestingIndices() {
			coverage[key][primitives.ValidatorIndex(index)] = struct{}{}
		}
	}
	return coverage
}

func diagnosticLateFreshCoverage(
	tb testing.TB,
	st state.ReadOnlyBeaconState,
	atts []ethpb.Att,
) map[primitives.ValidatorIndex]struct{} {
	tb.Helper()
	coverage := diagnosticLateCoverage(tb, st, atts)
	var freshRoot [32]byte
	freshRoot[0] = 0x95
	return coverage[diagnosticLateCoverageKey{slot: diagnosticLateHeadSlot, root: freshRoot}]
}

func diagnosticLateRequireSignatures(tb testing.TB, st state.ReadOnlyBeaconState, atts []ethpb.Att) {
	tb.Helper()
	batch, err := blocks.AttestationSignatureBatch(tb.Context(), st, atts)
	require.NoError(tb, err)
	ok, err := batch.Verify()
	require.NoError(tb, err)
	require.Equal(tb, true, ok)
}

func diagnosticLatePack(
	tb testing.TB,
	fixture diagnosticLateFixture,
	pool attpool.Pool,
) ([]ethpb.Att, attpool.Pool) {
	tb.Helper()
	server := diagnosticLateServer(pool, fixture.chain)
	packed, err := server.packAttestations(tb.Context(), fixture.creditedState, fixture.stateSlot)
	require.NoError(tb, err)
	return packed, pool
}

func requireDiagnosticLatePreflight(tb testing.TB, fixture diagnosticLateFixture) {
	tb.Helper()
	require.Equal(tb, 12, len(fixture.oldGloas))
	require.Equal(tb, 12, len(fixture.oldElectra))
	require.Equal(tb, 6, len(fixture.fresh))
	require.Equal(tb, 18, len(fixture.compactRetained))
	require.Equal(tb, 6, len(fixture.normalizedPruned))
	require.Equal(tb, 29_847, fixture.totalOldPositions)
	require.Equal(tb, 15_000, fixture.freshPositions)

	valid, invalid := proposerAtts(fixture.compactRetained).filter(tb.Context(), fixture.state)
	require.Equal(tb, 18, len(valid))
	require.Equal(tb, 0, len(invalid))
	diagnosticLateRequireSignatures(tb, fixture.state, fixture.compactRetained)

	totalBalance, err := helpers.TotalActiveBalance(tb.Context(), fixture.state)
	require.NoError(tb, err)
	for _, old := range fixture.oldElectra {
		uncredited, err := electra.GetProposerRewardNumerator(tb.Context(), fixture.state, old, totalBalance)
		require.NoError(tb, err)
		require.Equal(tb, true, uncredited > 0)
		credited, err := electra.GetProposerRewardNumerator(tb.Context(), fixture.creditedState, old, totalBalance)
		require.NoError(tb, err)
		require.Equal(tb, uint64(0), credited)
	}
	freshReward := uint64(0)
	for _, fresh := range fixture.fresh {
		reward, err := electra.GetProposerRewardNumerator(tb.Context(), fixture.creditedState, fresh, totalBalance)
		require.NoError(tb, err)
		freshReward += reward
	}
	require.Equal(tb, true, freshReward > 0)

	retainedPacked, retainedPool := diagnosticLatePack(tb, fixture, diagnosticLateCounterfactualPool(tb, fixture, true))
	prunedPacked, prunedPool := diagnosticLatePack(tb, fixture, diagnosticLateCounterfactualPool(tb, fixture, false))
	require.Equal(tb, 18, len(retainedPool.AggregatedAttestations()))
	require.Equal(tb, 6, len(prunedPool.AggregatedAttestations()))
	require.Equal(tb, 3, len(retainedPacked))
	require.Equal(tb, 1, len(prunedPacked))
	require.DeepEqual(
		tb,
		diagnosticLateFreshCoverage(tb, fixture.state, prunedPacked),
		diagnosticLateFreshCoverage(tb, fixture.state, retainedPacked),
	)
	require.Equal(tb, fixture.freshPositions, len(diagnosticLateFreshCoverage(tb, fixture.state, retainedPacked)))
	diagnosticLateRequireSignatures(tb, fixture.state, retainedPacked)
	diagnosticLateRequireSignatures(tb, fixture.state, prunedPacked)
	tb.Logf("HISTORICAL_LATE_PACKED retained_outputs=%d pruned_outputs=%d fresh_coverage=%d", len(retainedPacked), len(prunedPacked), len(diagnosticLateFreshCoverage(tb, fixture.state, retainedPacked)))
}

func TestDiagnosticHistoricalLatePackingPreflight(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_HISTORICAL_LATE_PACKING") != "1" {
		t.Skip("set PRYSM_DIAGNOSTIC_HISTORICAL_LATE_PACKING=1 to run the fixture preflight")
	}
	fixture := newDiagnosticLateFixture(t)
	requireDiagnosticLatePreflight(t, fixture)
	t.Logf(
		"HISTORICAL_LATE_PREFLIGHT validators=120000 state_slot=96 head_slot=95 committees=6 raw_objects=%d pruned_objects=%d old_positions=%d fresh_positions=%d signatures_valid=true",
		len(fixture.compactRetained),
		len(fixture.normalizedPruned),
		fixture.totalOldPositions,
		fixture.freshPositions,
	)
}

func BenchmarkDiagnosticHistoricalLatePacking(b *testing.B) {
	fixture := newDiagnosticLateFixture(b)
	requireDiagnosticLatePreflight(b, fixture)
	ctx := context.Background()

	for _, scenario := range []struct {
		name        string
		retainGloas bool
	}{
		{name: "compact_gloas_retained", retainGloas: true},
		{name: "normalized_electra_pruned", retainGloas: false},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			pool := diagnosticLateCounterfactualPool(b, fixture, scenario.retainGloas)
			server := diagnosticLateServer(pool, fixture.chain)
			b.ReportAllocs()
			b.ReportMetric(float64(len(pool.AggregatedAttestations())), "pool_objects")
			b.Run("pool_snapshot_and_validation", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					atts := pool.AggregatedAttestations()
					diagnosticLateAttSink = server.validateAndDeleteAttsInPool(ctx, fixture.creditedState, atts)
				}
			})
			b.Run("dedup_and_aggregation", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					atts := pool.AggregatedAttestations()
					var err error
					diagnosticLateAttSink, err = diagnosticLateNormalizeDedupAggregate(atts)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("reward_sort", func(b *testing.B) {
				onChain, err := diagnosticLateNormalizeDedupAggregate(pool.AggregatedAttestations())
				require.NoError(b, err)
				b.ReportAllocs()
				for b.Loop() {
					candidates := append(proposerAtts(nil), onChain...)
					diagnosticLateAttSink, err = candidates.sortOnChainAggregates(ctx, fixture.creditedState)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("full_pack_attestations", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					diagnosticLateAttSink, err = server.packAttestations(ctx, fixture.creditedState, fixture.stateSlot)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkDiagnosticHistoricalLateRewardParticipants(b *testing.B) {
	fixture := newDiagnosticLateFixture(b)
	totalBalance, err := helpers.TotalActiveBalance(b.Context(), fixture.state)
	require.NoError(b, err)
	for _, scenario := range []struct {
		name  string
		state state.ReadOnlyBeaconState
		atts  []ethpb.Att
	}{
		{name: "old_uncredited_29847_positions", state: fixture.state, atts: fixture.oldElectra},
		{name: "old_credited_29847_positions", state: fixture.creditedState, atts: fixture.oldElectra},
		{name: "fresh_on_partially_credited_15000_positions", state: fixture.creditedState, atts: fixture.fresh},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var reward uint64
				for _, att := range scenario.atts {
					value, err := electra.GetProposerRewardNumerator(b.Context(), scenario.state, att, totalBalance)
					if err != nil {
						b.Fatal(err)
					}
					reward += value
				}
				diagnosticLateRewardSink = reward
			}
		})
	}
}

var (
	diagnosticLateAttSink    []ethpb.Att
	diagnosticLateRewardSink uint64
)

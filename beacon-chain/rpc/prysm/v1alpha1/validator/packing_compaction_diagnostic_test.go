package validator

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	mockchain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	attpool "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const historicalPackingGenesisRootHex = "1a40155d770d5a166e5976f7f9c1804026797f51b522c4959fdcde7fcaa010d1"

type historicalPackingFixture struct {
	state            state.BeaconState
	atts             []ethpb.Att
	expectedCoverage map[primitives.ValidatorIndex]struct{}
	chain            *mockchain.ChainService
}

// snapshotNotifyingPackingPool reports only after the underlying pool has
// returned its real cloned unaggregated-attestation snapshot. Closing a channel
// is nonblocking, and the underlying getter has released its read lock before
// this wrapper runs.
type snapshotNotifyingPackingPool struct {
	attpool.Pool
	once          sync.Once
	snapshotReady chan struct{}
	snapshotCount atomic.Int64
	snapshotAt    atomic.Int64
}

func (p *snapshotNotifyingPackingPool) UnaggregatedAttestations() []ethpb.Att {
	atts := p.Pool.UnaggregatedAttestations()
	p.once.Do(func() {
		p.snapshotCount.Store(int64(len(atts)))
		p.snapshotAt.Store(time.Now().UnixNano())
		close(p.snapshotReady)
	})
	return atts
}

type historicalPackingOverlapResult struct {
	deposits []*ethpb.Deposit
	atts     []ethpb.Att
	err      error
	endedAt  time.Time
}

func historicalPackingRoot(t *testing.T, value string) [32]byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	require.Equal(t, 32, len(decoded))
	return [32]byte(decoded)
}

func historicalPackingData(slot primitives.Slot, targetRound primitives.Round, genesisRoot [32]byte) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            slot,
		CommitteeIndex:  1,
		BeaconBlockRoot: append([]byte(nil), genesisRoot[:]...),
		Source:          &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		Target:          &ethpb.Checkpoint{Epoch: targetRound, Root: append([]byte(nil), genesisRoot[:]...)},
	}
}

func requireHistoricalPackingDataRoots(t *testing.T, genesisRoot [32]byte) {
	t.Helper()
	for _, check := range []struct {
		slot        primitives.Slot
		targetRound primitives.Round
		want        string
	}{
		{slot: 4, targetRound: 0, want: "af105c327435d258e6a435c00311591bc93afc8558a6773b6d833bdab34802"},
		{slot: 8, targetRound: 1, want: "3fb84d64e264f1396d72654b6e72a0a12db7831639fcbe07de6efd95bdd552"},
	} {
		data := historicalPackingData(check.slot, check.targetRound, genesisRoot)
		dataRoot, err := data.HashTreeRoot()
		require.NoError(t, err)
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(5, true)
		observerAtt := &ethpb.AttestationElectra{
			AggregationBits: bitfield.NewBitlist(diagnosticCommitteeSize),
			CommitteeBits:   committeeBits,
			Data:            data,
			Signature:       make([]byte, 96),
		}
		observerRoot := decoupled.VoteLedgerDataRoot(observerAtt)
		require.Equal(t, "0x"+check.want, observerRoot)

		// VoteLedgerDataRoot is the production pool grouping key: for Electra it
		// hashes the SSZ data root with the comma-separated committee indices,
		// then renders bytes 1..31 so attestation versions can share a key.
		groupingInput := append(append([]byte(nil), dataRoot[:]...), strconv.Itoa(5)...)
		fullGroupingRoot := hash.Hash(groupingInput)
		require.Equal(t, check.want, hex.EncodeToString(fullGroupingRoot[1:]))
		t.Logf("PACKING_DATA_ROOT slot=%d committee=5 target_round=%d ssz_data_root=%x full_electra_grouping_root=%x observer400=%s", check.slot, check.targetRound, dataRoot, fullGroupingRoot, observerRoot)
	}
}

func newHistoricalPackingFixture(t *testing.T) historicalPackingFixture {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = diagnosticCommitteeSize
	params.OverrideBeaconConfig(cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	genesisRoot := historicalPackingRoot(t, historicalPackingGenesisRootHex)
	requireHistoricalPackingDataRoots(t, genesisRoot)
	require.Equal(t, uint64(6), helpers.SlotCommitteeCount(120_000))
	t.Logf("PACKING_CONFIG target_committee_size=%d configured_max_committees_per_slot=%d configured_max_validators_per_committee=%d computed_committees_per_slot=%d", cfg.TargetCommitteeSize, cfg.MaxCommitteesPerSlot, cfg.MaxValidatorsPerCommittee, helpers.SlotCommitteeCount(120_000))

	validators := make([]*ethpb.Validator, 120_000)
	balances := make([]uint64, len(validators))
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey: make([]byte, 48), WithdrawalCredentials: make([]byte, 32),
			EffectiveBalance: cfg.MaxEffectiveBalance, ActivationEpoch: 0,
			ActivationEligibilityEpoch: 0, ExitEpoch: cfg.FarFutureEpoch,
			WithdrawableEpoch: cfg.FarFutureEpoch,
		}
		balances[i] = cfg.MaxEffectiveBalance
	}
	st, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Validators = validators
		pb.Balances = balances
		pb.PreviousEpochParticipation = make([]byte, len(validators))
		pb.CurrentEpochParticipation = make([]byte, len(validators))
		pb.InactivityScores = make([]uint64, len(validators))
		return nil
	})
	require.NoError(t, err)
	const blockSlot = primitives.Slot(14)
	const attestationSlot = primitives.Slot(7)
	require.NoError(t, st.SetSlot(blockSlot))
	require.NoError(t, st.SetPreviousJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)}))
	require.NoError(t, st.SetCurrentJustifiedCheckpoint(&ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)}))

	committees := make([][]primitives.ValidatorIndex, 6)
	for committeeIndex := range committees {
		committees[committeeIndex], err = helpers.BeaconCommitteeFromState(t.Context(), st, attestationSlot, primitives.CommitteeIndex(committeeIndex))
		require.NoError(t, err)
		require.Equal(t, diagnosticCommitteeSize, len(committees[committeeIndex]))
	}

	atts := make([]ethpb.Att, 0, len(committees)*diagnosticCommitteeSize)
	expectedCoverage := make(map[primitives.ValidatorIndex]struct{}, cap(atts))
	data := historicalPackingData(attestationSlot, 0, genesisRoot)
	for committeeIndex, committee := range committees {
		for position, validatorIndex := range committee {
			key, keyErr := bls.RandKey()
			require.NoError(t, keyErr)
			validator := validators[validatorIndex]
			validator.PublicKey = key.PublicKey().Marshal()
			require.NoError(t, st.UpdateValidatorAtIndex(validatorIndex, validator))

			bits := bitfield.NewBitlist(diagnosticCommitteeSize)
			bits.SetBitAt(uint64(position), true)
			committeeBits := primitives.NewAttestationCommitteeBits()
			committeeBits.SetBitAt(uint64(committeeIndex), true)
			sig, signErr := signing.ComputeDomainAndSign(st, slots.ToEpoch(attestationSlot), data, cfg.DomainBeaconAttester, key)
			require.NoError(t, signErr)
			atts = append(atts, &ethpb.AttestationElectra{
				AggregationBits: bits,
				CommitteeBits:   committeeBits,
				Data:            data.Copy(),
				Signature:       sig,
			})
			expectedCoverage[validatorIndex] = struct{}{}
		}
	}
	require.Equal(t, 15_000, len(atts))
	require.Equal(t, 15_000, len(expectedCoverage))

	headSlot := primitives.Slot(0)
	currentSlot := blockSlot
	chain := &mockchain.ChainService{
		MockHeadSlot: &headSlot,
		Slot:         &currentSlot,
		Root:         append([]byte(nil), genesisRoot[:]...),
		TargetRoot:   genesisRoot,
		BlockSlot:    0,
	}
	return historicalPackingFixture{state: st, atts: atts, expectedCoverage: expectedCoverage, chain: chain}
}

func historicalPackingPool(t *testing.T, atts []ethpb.Att) attpool.Pool {
	t.Helper()
	pool := attpool.NewPool()
	clones := make([]ethpb.Att, len(atts))
	for i, att := range atts {
		clones[i] = att.Clone()
	}
	require.NoError(t, pool.SaveUnaggregatedAttestations(clones))
	return pool
}

func historicalPackingServer(pool attpool.Pool, chain *mockchain.ChainService) *Server {
	return &Server{
		AttPool:           pool,
		HeadFetcher:       chain,
		TimeFetcher:       chain,
		ForkchoiceFetcher: chain,
		Eth1InfoFetcher:   &mockExecution.Chain{NotConnected: true},
	}
}

func historicalPackingCoverage(t *testing.T, st state.ReadOnlyBeaconState, atts []ethpb.Att) map[primitives.ValidatorIndex]struct{} {
	t.Helper()
	covered := make(map[primitives.ValidatorIndex]struct{})
	for _, att := range atts {
		committees, err := helpers.AttestationCommitteesFromState(t.Context(), st, att)
		require.NoError(t, err)
		indexed, err := attestation.ConvertToIndexed(t.Context(), att, committees...)
		require.NoError(t, err)
		for _, index := range indexed.GetAttestingIndices() {
			covered[primitives.ValidatorIndex(index)] = struct{}{}
		}
	}
	return covered
}

func requireHistoricalPackingSignatures(t *testing.T, st state.ReadOnlyBeaconState, atts []ethpb.Att) {
	t.Helper()
	batch, err := blocks.AttestationSignatureBatch(t.Context(), st, atts)
	require.NoError(t, err)
	verified, err := batch.Verify()
	require.NoError(t, err)
	require.Equal(t, true, verified)
}

// TestDiagnosticHistoricalPackingCompaction compares the real proposer packer
// before and after the real background pool compaction on identical votes.
func TestDiagnosticHistoricalPackingCompaction(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_PACKING_COMPACTION") != "1" {
		t.Skip("set PRYSM_DIAGNOSTIC_PACKING_COMPACTION=1 to run the bounded diagnostic")
	}

	fixtureStarted := time.Now()
	fixture := newHistoricalPackingFixture(t)
	t.Logf("PACKING_FIXTURE validators=120000 block_slot=14 attestation_slot=7 committees=6 singles=15000 elapsed=%s", time.Since(fixtureStarted))

	rawPool := historicalPackingPool(t, fixture.atts)
	compactPool := historicalPackingPool(t, fixture.atts)
	compactionStarted := time.Now()
	require.NoError(t, compactPool.AggregateUnaggregatedAttestations(t.Context()))
	compactionElapsed := time.Since(compactionStarted)
	require.Equal(t, 15_000, len(rawPool.UnaggregatedAttestations()))
	require.Equal(t, 0, len(rawPool.AggregatedAttestations()))
	require.Equal(t, 0, len(compactPool.UnaggregatedAttestations()))
	require.Equal(t, 6, len(compactPool.AggregatedAttestations()))
	t.Logf("PACKING_COMPACTION elapsed=%s raw_unaggregated=%d raw_aggregated=%d compact_unaggregated=%d compact_aggregated=%d", compactionElapsed, len(rawPool.UnaggregatedAttestations()), len(rawPool.AggregatedAttestations()), len(compactPool.UnaggregatedAttestations()), len(compactPool.AggregatedAttestations()))

	rawServer := historicalPackingServer(rawPool, fixture.chain)
	compactServer := historicalPackingServer(compactPool, fixture.chain)
	uncanceled := make(map[string][]ethpb.Att, 2)
	for _, arm := range []struct {
		name   string
		server *Server
	}{
		{name: "raw", server: rawServer},
		{name: "compact", server: compactServer},
	} {
		started := time.Now()
		packed, err := arm.server.packAttestations(context.Background(), fixture.state, 14)
		elapsed := time.Since(started)
		require.NoError(t, err)
		require.Equal(t, 1, len(packed))
		require.DeepEqual(t, fixture.atts[0].GetData(), packed[0].GetData())
		coverage := historicalPackingCoverage(t, fixture.state, packed)
		require.DeepEqual(t, fixture.expectedCoverage, coverage)
		requireHistoricalPackingSignatures(t, fixture.state, packed)
		uncanceled[arm.name] = packed
		t.Logf("PACKING_UNCANCELED arm=%s elapsed=%s outputs=%d coverage=%d signatures_valid=true", arm.name, elapsed, len(packed), len(coverage))
	}
	require.DeepEqual(t, historicalPackingCoverage(t, fixture.state, uncanceled["raw"]), historicalPackingCoverage(t, fixture.state, uncanceled["compact"]))
	require.Equal(t, 15_000, len(rawPool.UnaggregatedAttestations()))
	require.Equal(t, 0, len(rawPool.AggregatedAttestations()))
	require.Equal(t, 0, len(compactPool.UnaggregatedAttestations()))
	require.Equal(t, 6, len(compactPool.AggregatedAttestations()))
	t.Logf("PACKING_POOL_AFTER_UNCANCELED raw_unaggregated=%d raw_aggregated=%d compact_unaggregated=%d compact_aggregated=%d", len(rawPool.UnaggregatedAttestations()), len(rawPool.AggregatedAttestations()), len(compactPool.UnaggregatedAttestations()), len(compactPool.AggregatedAttestations()))

	const budget = 300 * time.Millisecond
	for _, arm := range []struct {
		name        string
		server      *Server
		wantTimeout bool
	}{
		{name: "raw", server: rawServer, wantTimeout: true},
		{name: "compact", server: compactServer},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		started := time.Now()
		deposits, packed, err := arm.server.packDepositsAndAttestations(ctx, fixture.state, 14, &ethpb.Eth1Data{})
		elapsed := time.Since(started)
		ctxErr := ctx.Err()
		cancel()
		if arm.wantTimeout {
			require.Equal(t, context.DeadlineExceeded, err)
			require.Equal(t, context.DeadlineExceeded, ctxErr)
			require.Equal(t, true, errors.Is(err, context.DeadlineExceeded))
			require.Equal(t, true, elapsed > budget)
		} else {
			require.NoError(t, err)
			require.NoError(t, ctxErr)
			require.Equal(t, true, elapsed < budget)
			require.Equal(t, 1, len(packed))
		}
		t.Logf("PACKING_DEADLINE arm=%s budget=%s elapsed=%s deposits=%d outputs=%d err=%v bare_deadline=%t", arm.name, budget, elapsed, len(deposits), len(packed), err, err == context.DeadlineExceeded)
	}

	canceledPool := historicalPackingPool(t, fixture.atts)
	require.NoError(t, canceledPool.AggregateUnaggregatedAttestations(t.Context()))
	canceledServer := historicalPackingServer(canceledPool, fixture.chain)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, _, err := canceledServer.packDepositsAndAttestations(canceledCtx, fixture.state, 14, &ethpb.Eth1Data{})
	require.Equal(t, context.Canceled, err)
	t.Logf("PACKING_CANCELED elapsed=%s err=%v bare_canceled=%t", time.Since(started), err, err == context.Canceled)
}

// TestDiagnosticHistoricalPackingSnapshotOverlap tests the proposal-specific
// snapshot boundary: real compaction changes the live pool after an old pack
// has cloned the raw singles, then a fresh pack reads the compacted pool while
// the old pack finishes work over its private raw snapshot.
func TestDiagnosticHistoricalPackingSnapshotOverlap(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_PACKING_SNAPSHOT_OVERLAP") != "1" {
		t.Skip("set PRYSM_DIAGNOSTIC_PACKING_SNAPSHOT_OVERLAP=1 to run the bounded diagnostic")
	}

	fixtureStarted := time.Now()
	fixture := newHistoricalPackingFixture(t)
	t.Logf("PACKING_OVERLAP_FIXTURE validators=120000 block_slot=14 attestation_slot=7 committees=6 singles=15000 elapsed=%s", time.Since(fixtureStarted))

	livePool := historicalPackingPool(t, fixture.atts)
	notifyingPool := &snapshotNotifyingPackingPool{
		Pool:          livePool,
		snapshotReady: make(chan struct{}),
	}
	server := historicalPackingServer(notifyingPool, fixture.chain)
	oldState := fixture.state.Copy()
	freshState := fixture.state.Copy()

	const budget = 1662 * time.Millisecond
	oldCtx, cancelOld := context.WithCancel(context.Background())
	defer cancelOld()
	var timerFiredAt atomic.Int64
	var oldEndedAt atomic.Int64
	var oldDone atomic.Bool
	oldResult := make(chan historicalPackingOverlapResult, 1)
	oldStarted := time.Now()
	timer := time.AfterFunc(budget, func() {
		timerFiredAt.Store(time.Now().UnixNano())
		cancelOld()
	})
	defer timer.Stop()
	go func() {
		deposits, atts, err := server.packDepositsAndAttestations(oldCtx, oldState, 14, &ethpb.Eth1Data{})
		endedAt := time.Now()
		oldEndedAt.Store(endedAt.UnixNano())
		oldDone.Store(true)
		oldResult <- historicalPackingOverlapResult{deposits: deposits, atts: atts, err: err, endedAt: endedAt}
	}()

	<-notifyingPool.snapshotReady
	snapshotAt := time.Unix(0, notifyingPool.snapshotAt.Load())
	require.Equal(t, int64(15_000), notifyingPool.snapshotCount.Load())
	t.Logf("PACKING_OVERLAP_SNAPSHOT elapsed=%s copied=%d live_unaggregated=%d live_aggregated=%d old_done=%t", snapshotAt.Sub(oldStarted), notifyingPool.snapshotCount.Load(), livePool.UnaggregatedAttestationCount(), livePool.AggregatedAttestationCount(), oldDone.Load())

	compactionStarted := time.Now()
	compactionResult := make(chan error, 1)
	go func() {
		compactionResult <- livePool.AggregateUnaggregatedAttestations(context.Background())
	}()
	require.NoError(t, <-compactionResult)
	compactionEnded := time.Now()
	oldActiveAtCompactionEnd := !oldDone.Load()
	require.Equal(t, 0, livePool.UnaggregatedAttestationCount())
	require.Equal(t, 6, livePool.AggregatedAttestationCount())
	t.Logf("PACKING_OVERLAP_COMPACTION start=%s end=%s elapsed=%s live_unaggregated=%d live_aggregated=%d old_active_at_end=%t timer_fired=%t", compactionStarted.Sub(oldStarted), compactionEnded.Sub(oldStarted), compactionEnded.Sub(compactionStarted), livePool.UnaggregatedAttestationCount(), livePool.AggregatedAttestationCount(), oldActiveAtCompactionEnd, timerFiredAt.Load() != 0)

	freshStarted := time.Now()
	oldActiveAtFreshStart := !oldDone.Load()
	freshDeposits, freshAtts, freshErr := server.packDepositsAndAttestations(context.Background(), freshState, 14, &ethpb.Eth1Data{})
	freshEnded := time.Now()
	oldActiveAtFreshEnd := !oldDone.Load()
	require.NoError(t, freshErr)
	require.Equal(t, 0, len(freshDeposits))
	require.Equal(t, 1, len(freshAtts))
	require.DeepEqual(t, fixture.atts[0].GetData(), freshAtts[0].GetData())
	freshCoverage := historicalPackingCoverage(t, fixture.state, freshAtts)
	require.DeepEqual(t, fixture.expectedCoverage, freshCoverage)
	requireHistoricalPackingSignatures(t, fixture.state, freshAtts)
	t.Logf("PACKING_OVERLAP_FRESH start=%s end=%s elapsed=%s deposits=%d outputs=%d coverage=%d signatures_valid=true old_active_at_start=%t old_active_at_end=%t", freshStarted.Sub(oldStarted), freshEnded.Sub(oldStarted), freshEnded.Sub(freshStarted), len(freshDeposits), len(freshAtts), len(freshCoverage), oldActiveAtFreshStart, oldActiveAtFreshEnd)

	old := <-oldResult
	timerAt := timerFiredAt.Load()
	require.NotEqual(t, int64(0), timerAt)
	require.Equal(t, context.Canceled, old.err)
	require.Equal(t, true, errors.Is(old.err, context.Canceled))
	require.Equal(t, true, timerAt <= oldEndedAt.Load())
	t.Logf("PACKING_OVERLAP_OLD budget=%s timer=%s end=%s elapsed=%s deposits=%d outputs=%d err=%v bare_canceled=%t old_active_at_compaction_end=%t old_active_at_fresh_start=%t old_active_at_fresh_end=%t", budget, time.Unix(0, timerAt).Sub(oldStarted), old.endedAt.Sub(oldStarted), old.endedAt.Sub(oldStarted), len(old.deposits), len(old.atts), old.err, old.err == context.Canceled, oldActiveAtCompactionEnd, oldActiveAtFreshStart, oldActiveAtFreshEnd)
}

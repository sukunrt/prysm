//go:build !minimal

package validator

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	mockchain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	attpool "github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const (
	packingLargeCommittees        = 10
	packingLargeCommitteeSize     = 1000
	packingLargePeersPerCommittee = 16
)

type packingLargeFixture struct {
	server   *Server
	state    state.BeaconState
	keys     []bls.SecretKey
	retained int
	coverage int
}

// The Heze round has 10 committees of 1000; all 10000 participants sign real votes.
func newPackingLargeFixture(tb testing.TB, peers bool) packingLargeFixture {
	tb.Helper()
	params.SetupTestConfigCleanup(tb)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = packingLargeCommitteeSize
	cfg.MaxCommitteesPerSlot = packingLargeCommittees
	params.OverrideBeaconConfig(cfg)
	tb.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, 80_000)
	keys := make([]bls.SecretKey, len(validators))
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
	require.NoError(tb, err)
	require.NoError(tb, st.SetSlot(14))
	previous := &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)}
	current := &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)}
	require.NoError(tb, st.SetPreviousJustifiedCheckpoint(previous))
	require.NoError(tb, st.SetCurrentJustifiedCheckpoint(current))

	pool := attpool.NewPool()
	const voteSlot primitives.Slot = 13
	for ci := range packingLargeCommittees {
		committee, err := helpers.BeaconCommitteeFromState(tb.Context(), st, voteSlot, primitives.CommitteeIndex(ci))
		require.NoError(tb, err)
		require.Equal(tb, packingLargeCommitteeSize, len(committee))
		data := &ethpb.AttestationData{
			Slot: voteSlot, BeaconBlockRoot: make([]byte, 32), Source: current, Target: current,
		}
		data.BeaconBlockRoot[0] = 1
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(ci), true)
		require.Equal(tb, true, committeeBits.BitAt(uint64(ci)))
		singles := make([]ethpb.Att, packingLargeCommitteeSize)
		for position, validatorIndex := range committee {
			key, err := bls.RandKey()
			require.NoError(tb, err)
			keys[validatorIndex] = key
			validator := validators[validatorIndex]
			validator.PublicKey = key.PublicKey().Marshal()
			require.NoError(tb, st.UpdateValidatorAtIndex(validatorIndex, validator))
			bits := bitfield.NewBitlist(packingLargeCommitteeSize)
			bits.SetBitAt(uint64(position), true)
			sig, err := signing.ComputeDomainAndSign(st, slots.ToEpoch(voteSlot), data, cfg.DomainBeaconAttester, key)
			require.NoError(tb, err)
			singles[position] = &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: committeeBits, Data: data, Signature: sig}
		}
		singleCount := packingLargeCommitteeSize
		if peers {
			singleCount = 500
		}
		require.NoError(tb, pool.SaveUnaggregatedAttestations(singles[:singleCount]))
		if peers {
			for peer := range packingLargePeersPerCommittee {
				width := 580 + peer%3*10
				bits := bitfield.NewBitlist(packingLargeCommitteeSize)
				signatures := make([][]byte, width)
				for offset := range width {
					position := (offset*37 + peer*17) % packingLargeCommitteeSize
					bits.SetBitAt(uint64(position), true)
					signatures[offset] = singles[position].GetSignature()
				}
				sig, err := bls.AggregateCompressedSignatures(signatures)
				require.NoError(tb, err)
				agg := &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: committeeBits, Data: data, Signature: sig.Marshal()}
				require.NoError(tb, pool.SaveAggregatedAttestation(agg))
			}
		}
	}
	headSlot := primitives.Slot(14)
	chain := &mockchain.ChainService{MockHeadSlot: &headSlot, Slot: &headSlot}
	server := &Server{AttPool: pool, HeadFetcher: chain, TimeFetcher: chain, ForkchoiceFetcher: chain}
	snapshot := pool.AggregatedAttestations()
	snapshot = append(snapshot, pool.UnaggregatedAttestations()...)
	coverage := make([]bitfield.Bitlist, packingLargeCommittees)
	for ci := range coverage {
		coverage[ci] = bitfield.NewBitlist(packingLargeCommitteeSize)
	}
	for _, att := range snapshot {
		for ci := range coverage {
			if !att.CommitteeBitsVal().BitAt(uint64(ci)) {
				continue
			}
			for _, position := range att.GetAggregationBits().BitIndices() {
				coverage[ci].SetBitAt(uint64(position), true)
			}
		}
	}
	covered := 0
	for _, bits := range coverage {
		covered += int(bits.Count())
	}
	return packingLargeFixture{server: server, state: st, keys: keys, retained: len(snapshot), coverage: covered}
}

func verifyPackingLargeFixture(tb testing.TB, fixture packingLargeFixture, peers bool) {
	tb.Helper()
	require.Equal(tb, 10_000, fixture.coverage)
	snapshot := fixture.server.AttPool.AggregatedAttestations()
	snapshot = append(snapshot, fixture.server.AttPool.UnaggregatedAttestations()...)
	before := make([]ethpb.Att, len(snapshot))
	for i := range snapshot {
		before[i] = snapshot[i].Clone()
	}
	deduped, err := proposerAtts(snapshot).dedup()
	require.NoError(tb, err)
	require.DeepEqual(tb, before, snapshot)
	groups := make(map[attestation.Id][]ethpb.Att)
	for _, att := range deduped {
		id, err := attestation.NewId(att, attestation.Data)
		require.NoError(tb, err)
		groups[id] = append(groups[id], att)
	}
	for id, atts := range groups {
		groups[id], err = attaggregation.Aggregate(atts)
		require.NoError(tb, err)
	}
	onChain, err := onChainAggregates(groups)
	require.NoError(tb, err)
	onChain, err = onChain.dedup()
	require.NoError(tb, err)
	require.Equal(tb, fixture.coverage, packingLargeCoverage(tb, onChain))
	packed, err := fixture.server.packAttestations(context.Background(), fixture.state, 14)
	require.NoError(tb, err)
	require.NotEqual(tb, 0, len(packed))
	require.Equal(tb, true, len(packed) <= int(params.BeaconConfig().MaxAttestationsElectra))
	tb.Logf("retained=%d prelimit_coverage=%d packed=%d postlimit_coverage=%d", fixture.retained, fixture.coverage, len(packed), packingLargeCoverage(tb, packed))
	set, err := blocks.AttestationSignatureBatch(context.Background(), fixture.state, packed)
	require.NoError(tb, err)
	valid, err := set.Verify()
	require.NoError(tb, err)
	require.Equal(tb, true, valid)
}

func packingLargeCoverage(tb testing.TB, atts []ethpb.Att) int {
	tb.Helper()
	coverage := make([]bitfield.Bitlist, packingLargeCommittees)
	for ci := range coverage {
		coverage[ci] = bitfield.NewBitlist(packingLargeCommitteeSize)
	}
	for _, att := range atts {
		indices := att.CommitteeBitsVal().BitIndices()
		require.Equal(tb, packingLargeCommitteeSize*len(indices), int(att.GetAggregationBits().Len()))
		for offset, ci := range indices {
			for position := range packingLargeCommitteeSize {
				if att.GetAggregationBits().BitAt(uint64(offset*packingLargeCommitteeSize + position)) {
					coverage[ci].SetBitAt(uint64(position), true)
				}
			}
		}
	}
	covered := 0
	for _, bits := range coverage {
		covered += int(bits.Count())
	}
	return covered
}

func BenchmarkProposalPackingLargeClassicPool(b *testing.B) {
	for _, peers := range []bool{false, true} {
		name := "compact"
		if peers {
			name = "mixed_peers"
		}
		b.Run(name, func(b *testing.B) {
			fixture := newPackingLargeFixture(b, peers)
			b.Logf("retained=%d coverage=%d", fixture.retained, fixture.coverage)
			verifyPackingLargeFixture(b, fixture, peers)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				packed, err := fixture.server.packAttestations(context.Background(), fixture.state, 14)
				if err != nil || len(packed) == 0 {
					b.Fatalf("packed=%d err=%v", len(packed), err)
				}
			}
		})
	}
}

func addMixedAgeCandidates(tb testing.TB, fixture packingLargeFixture, prune bool) (before, after int) {
	tb.Helper()
	for _, slot := range []primitives.Slot{8, 9, 10} {
		for ci := range packingLargeCommittees {
			committee, err := helpers.BeaconCommitteeFromState(tb.Context(), fixture.state, slot, primitives.CommitteeIndex(ci))
			require.NoError(tb, err)
			data := &ethpb.AttestationData{Slot: slot, BeaconBlockRoot: make([]byte, 32), Source: fixture.state.CurrentJustifiedCheckpoint(), Target: fixture.state.CurrentJustifiedCheckpoint()}
			data.BeaconBlockRoot[0] = 1
			bits := bitfield.NewBitlist(packingLargeCommitteeSize)
			signatures := make([][]byte, len(committee))
			for position, validatorIndex := range committee {
				bits.SetBitAt(uint64(position), true)
				if fixture.keys[validatorIndex] == nil {
					key, err := bls.RandKey()
					require.NoError(tb, err)
					fixture.keys[validatorIndex] = key
					validator, err := fixture.state.ValidatorAtIndex(validatorIndex)
					require.NoError(tb, err)
					validator.PublicKey = key.PublicKey().Marshal()
					require.NoError(tb, fixture.state.UpdateValidatorAtIndex(validatorIndex, validator))
				}
				signatures[position], err = signing.ComputeDomainAndSign(fixture.state, slots.ToEpoch(slot), data, params.BeaconConfig().DomainBeaconAttester, fixture.keys[validatorIndex])
				require.NoError(tb, err)
			}
			sig, err := bls.AggregateCompressedSignatures(signatures)
			require.NoError(tb, err)
			committeeBits := primitives.NewAttestationCommitteeBits()
			committeeBits.SetBitAt(uint64(ci), true)
			old := &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: committeeBits, Data: data, Signature: sig.Marshal()}
			require.NoError(tb, fixture.server.AttPool.SaveAggregatedAttestation(old))
		}
	}
	before = len(fixture.server.AttPool.AggregatedAttestations())
	if prune {
		_, err := fixture.server.AttPool.PruneBefore(11)
		require.NoError(tb, err)
	}
	after = len(fixture.server.AttPool.AggregatedAttestations())
	return before, after
}

func TestProposalPackingMixedAgeRetention(t *testing.T) {
	f := newPackingLargeFixture(t, false)
	before, after := addMixedAgeCandidates(t, f, true)
	require.Equal(t, 40, before)
	require.Equal(t, 10, after)
	require.Equal(t, 0, f.server.AttPool.ForkchoiceAttestationCount())
	verifyPackingLargeFixture(t, f, false)
}

func BenchmarkProposalPackingMixedAgeRetained(b *testing.B) {
	f := newPackingLargeFixture(b, false)
	before, after := addMixedAgeCandidates(b, f, true)
	b.Logf("candidates_before=%d candidates_after=%d forkchoice_queue=%d", before, after, f.server.AttPool.ForkchoiceAttestationCount())
	verifyPackingLargeFixture(b, f, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := f.server.packAttestations(context.Background(), f.state, 14); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProposalPackingMixedAgeUnpruned(b *testing.B) {
	f := newPackingLargeFixture(b, false)
	before, after := addMixedAgeCandidates(b, f, false)
	b.Logf("candidates_before=%d candidates_after=%d", before, after)
	verifyPackingLargeFixture(b, f, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := f.server.packAttestations(context.Background(), f.state, 14); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProposalPackingLargeDedup(b *testing.B) {
	fixture := newPackingLargeFixture(b, true)
	atts := fixture.server.AttPool.AggregatedAttestations()
	atts = append(atts, fixture.server.AttPool.UnaggregatedAttestations()...)
	b.Logf("retained=%d coverage=%d", fixture.retained, fixture.coverage)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := proposerAtts(atts).dedup(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestProposalPackingLargeFixture(t *testing.T) {
	for _, peers := range []bool{false, true} {
		t.Run(fmt.Sprint(peers), func(t *testing.T) {
			fixture := newPackingLargeFixture(t, peers)
			verifyPackingLargeFixture(t, fixture, peers)
			t.Logf("retained=%d coverage=%d", fixture.retained, fixture.coverage)
		})
	}
}

func TestProposalPackingLargeProfile(t *testing.T) {
	path := os.Getenv("PRYSM_PROPOSAL_PACKING_PROFILE")
	if path == "" {
		t.Skip("set PRYSM_PROPOSAL_PACKING_PROFILE to collect the packing profile")
	}
	fixture := newPackingLargeFixture(t, true)
	f, err := os.Create(path) // #nosec G304 -- explicitly provided local profile path.
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, pprof.StartCPUProfile(f))
	for range 500 {
		packed, err := fixture.server.packAttestations(t.Context(), fixture.state, 14)
		require.NoError(t, err)
		require.NotEqual(t, 0, len(packed))
	}
	pprof.StopCPUProfile()
}

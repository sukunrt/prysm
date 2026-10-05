//go:build minimal

package validator

import (
	"context"
	"fmt"
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

type proposalPackingFixture struct {
	server   *Server
	state    state.BeaconState
	retained int
}

func proposalPackingCoverage(tb testing.TB, atts []ethpb.Att) int {
	tb.Helper()
	groups := make(map[[32]byte]bitfield.Bitlist)
	for _, att := range atts {
		root, err := att.GetData().HashTreeRoot()
		require.NoError(tb, err)
		bits := groups[root]
		if bits == nil {
			bits = bitfield.NewBitlist(125)
			groups[root] = bits
		}
		for _, position := range att.GetAggregationBits().BitIndices() {
			bits.SetBitAt(uint64(position), true)
		}
	}
	count := 0
	for _, bits := range groups {
		count += int(bits.Count())
	}
	return count
}

func verifyProposalPackingFixture(tb testing.TB, fixture proposalPackingFixture) {
	tb.Helper()
	snapshot := fixture.server.AttPool.AggregatedAttestations()
	snapshot = append(snapshot, fixture.server.AttPool.UnaggregatedAttestations()...)
	rawCoverage := proposalPackingCoverage(tb, snapshot)
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
	require.Equal(tb, rawCoverage, proposalPackingCoverage(tb, onChain))
	packed, err := fixture.server.packAttestations(context.Background(), fixture.state, 14)
	require.NoError(tb, err)
	require.NotEqual(tb, 0, len(packed))
	set, err := blocks.AttestationSignatureBatch(context.Background(), fixture.state, packed)
	require.NoError(tb, err)
	valid, err := set.Verify()
	require.NoError(tb, err)
	require.Equal(tb, true, valid)
}

// The 1000-validator minimal state yields one 125-validator committee per slot.
func newProposalPackingFixture(tb testing.TB, mixed bool) proposalPackingFixture {
	tb.Helper()
	params.SetupTestConfigCleanup(tb)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.HezeForkEpoch = cfg.FarFutureEpoch
	cfg.GloasForkEpoch = cfg.FarFutureEpoch
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 125
	cfg.MaxCommitteesPerSlot = 1
	params.OverrideBeaconConfig(cfg)
	tb.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, 1000)
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
	st, err := util.NewBeaconStateElectra(func(pb *ethpb.BeaconStateElectra) error {
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
	keys := make(map[primitives.ValidatorIndex]bls.SecretKey)
	for group := range 4 {
		slot := primitives.Slot(7)
		target := previous
		if group >= 2 {
			slot, target = 13, current
		}
		committee, err := helpers.BeaconCommitteeFromState(tb.Context(), st, slot, 0)
		require.NoError(tb, err)
		require.Equal(tb, 125, len(committee))
		root := make([]byte, 32)
		root[0], root[1] = byte(slot), byte(group%2+1)
		source := previous
		if group >= 2 {
			source = current
		}
		data := &ethpb.AttestationData{Slot: slot, BeaconBlockRoot: root, Source: source, Target: target}
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(0, true)
		singles := make([]ethpb.Att, 80)
		for position := range singles {
			validatorIndex := committee[position]
			key := keys[validatorIndex]
			if key == nil {
				key, err = bls.RandKey()
				require.NoError(tb, err)
				keys[validatorIndex] = key
				validator := validators[validatorIndex]
				validator.PublicKey = key.PublicKey().Marshal()
				require.NoError(tb, st.UpdateValidatorAtIndex(validatorIndex, validator))
			}
			bits := bitfield.NewBitlist(125)
			bits.SetBitAt(uint64(position), true)
			sig, err := signing.ComputeDomainAndSign(st, slots.ToEpoch(slot), data, cfg.DomainBeaconAttester, key)
			require.NoError(tb, err)
			singles[position] = &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: committeeBits, Data: data, Signature: sig}
		}
		count := 20
		if group == 3 {
			count = 1
		}
		require.NoError(tb, pool.SaveUnaggregatedAttestations(singles[:count]))
		if mixed && group == 0 {
			for _, window := range [][2]int{{0, 24}, {10, 34}, {20, 44}, {30, 54}, {40, 64}, {50, 80}, {10, 34}, {10, 22}, {55, 80}} {
				copies := make([]ethpb.Att, window[1]-window[0])
				for i, single := range singles[window[0]:window[1]] {
					copies[i] = single.Clone()
				}
				agg, err := attaggregation.AggregateDisjointOneBitAtts(copies)
				require.NoError(tb, err)
				require.NoError(tb, pool.SaveAggregatedAttestation(agg))
			}
		}
	}
	headSlot := primitives.Slot(14)
	chain := &mockchain.ChainService{MockHeadSlot: &headSlot, Slot: &headSlot}
	server := &Server{AttPool: pool, HeadFetcher: chain, TimeFetcher: chain, ForkchoiceFetcher: chain}
	return proposalPackingFixture{server: server, state: st, retained: len(pool.AggregatedAttestations()) + len(pool.UnaggregatedAttestations())}
}

func BenchmarkProposalPackingClassicPool(b *testing.B) {
	for _, mixed := range []bool{false, true} {
		name := "compact"
		if mixed {
			name = "mixed_peers"
		}
		b.Run(name, func(b *testing.B) {
			fixture := newProposalPackingFixture(b, mixed)
			b.Logf("retained candidates: %d", fixture.retained)
			verifyProposalPackingFixture(b, fixture)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				result, err := fixture.server.packAttestations(context.Background(), fixture.state, 14)
				if err != nil || len(result) == 0 {
					b.Fatalf("packed=%d err=%v", len(result), err)
				}
			}
		})
	}
}

func TestProposalPackingClassicPoolFixture(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			fixture := newProposalPackingFixture(t, mixed)
			verifyProposalPackingFixture(t, fixture)
			t.Logf("mixed=%t retained=%d", mixed, fixture.retained)
		})
	}
}

func BenchmarkProposalPackingDedupAlgorithm(b *testing.B) {
	for _, varying := range []bool{false, true} {
		name := "equal_size_distinct"
		if varying {
			name = "varying_size_incomparable"
		}
		b.Run(name, func(b *testing.B) {
			atts := make(proposerAtts, 500)
			data := &ethpb.AttestationData{Slot: 7, BeaconBlockRoot: make([]byte, 32), Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}}
			cb := primitives.NewAttestationCommitteeBits()
			cb.SetBitAt(0, true)
			for i := range atts {
				bits := bitfield.NewBitlist(1000)
				bits.SetBitAt(uint64(i+100), true)
				if varying {
					for bit := range 1 + i%7 {
						bits.SetBitAt(uint64(bit), true)
					}
				} else {
					bits.SetBitAt(uint64(i+500), true)
				}
				atts[i] = &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: cb, Data: data, Signature: []byte{1}}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				result, err := atts.dedup()
				if err != nil || len(result) != len(atts) {
					b.Fatalf("deduped=%d err=%v", len(result), err)
				}
			}
		})
	}
}

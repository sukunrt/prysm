package blockchain

import (
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const updateHeadDiagnosticValidatorCount = 120_000

func updateHeadDiagnosticGenesis(t *testing.T) *ethpb.BeaconStateHeze {
	t.Helper()
	st, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Validators = make([]*ethpb.Validator, updateHeadDiagnosticValidatorCount)
		pb.Balances = make([]uint64, updateHeadDiagnosticValidatorCount)
		pb.PreviousEpochParticipation = make([]byte, updateHeadDiagnosticValidatorCount)
		pb.CurrentEpochParticipation = make([]byte, updateHeadDiagnosticValidatorCount)
		for i := range pb.Validators {
			pb.Validators[i] = &ethpb.Validator{
				PublicKey:                  make([]byte, 48),
				WithdrawalCredentials:      make([]byte, 32),
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
				ActivationEpoch:            0,
				ActivationEligibilityEpoch: 0,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
			}
			pb.Balances[i] = params.BeaconConfig().MaxEffectiveBalance
		}
		return nil
	})
	require.NoError(t, err)
	return st.ToProtoUnsafe().(*ethpb.BeaconStateHeze)
}

func updateHeadDiagnosticPool(root [32]byte, n int) []ethpb.Att {
	atts := make([]ethpb.Att, n)
	for i := range atts {
		bits := bitfield.NewBitlist(129)
		bits.SetBitAt(uint64(i), true)
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(0, true)
		atts[i] = &ethpb.AttestationElectra{
			AggregationBits: bits,
			CommitteeBits:   committeeBits,
			Data: &ethpb.AttestationData{
				Slot:            0,
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: root[:]},
				Target:          &ethpb.Checkpoint{Root: root[:]},
			},
			Signature: make([]byte, 96),
		}
	}
	return atts
}

func TestDiagnosticGenesis120KUpdateHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	pb := updateHeadDiagnosticGenesis(t)
	st, err := util.NewBeaconStateHeze(func(dst *ethpb.BeaconStateHeze) error {
		*dst = *pb
		return nil
	})
	require.NoError(t, err)
	service, tr := minimalTestService(t, WithFinalizedStateAtStartUp(st))
	require.NoError(t, service.saveGenesisData(t.Context(), st))
	root := service.originBlockRoot
	service.genesisTime = time.Now().Add(-2 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	tr.fcs.SetGenesisTime(service.genesisTime)
	encoded, err := st.MarshalSSZ()
	require.NoError(t, err)
	coldProto := &ethpb.BeaconStateHeze{}
	require.NoError(t, coldProto.UnmarshalSSZ(encoded))
	coldState, err := state_native.InitializeFromProtoUnsafeHeze(coldProto)
	require.NoError(t, err)
	coldStateGen := stategen.New(service.cfg.BeaconDB, tr.fcs)
	service.cfg.StateGen = coldStateGen
	require.NoError(t, coldStateGen.SaveState(t.Context(), root, coldState))
	for _, slot := range []primitives.Slot{1, 2, 3} {
		idx, err := helpers.BeaconProposerIndexAtSlot(t.Context(), st, slot)
		require.NoError(t, err)
		service.cfg.SubscribedValidatorsCache.Add(idx)
		service.cfg.ProposerPreferencesCache.SetDefault(cache.ProposerPreference{ValidatorIndex: idx})
	}
	headBlock, err := service.HeadBlock(t.Context())
	require.NoError(t, err)
	headState := coldState
	require.NoError(t, service.OnAttestation(t.Context(), updateHeadDiagnosticPool(root, 1)[0], time.Second))

	for _, warmed := range []bool{false, true} {
		if warmed {
			_, err := coldState.HashTreeRoot(t.Context())
			require.NoError(t, err)
		}
		for _, slot := range []primitives.Slot{1, 2, 3} {
			idx, err := helpers.BeaconProposerIndexAtSlot(t.Context(), coldState, slot)
			require.NoError(t, err)
			service.cfg.SubscribedValidatorsCache.Add(idx)
			service.cfg.ProposerPreferencesCache.SetDefault(cache.ProposerPreference{ValidatorIndex: idx})
			transition.SkipSlotCache = cache.NewSkipSlotCache()
			attr := service.getPayloadAttribute(t.Context(), coldState, slot, root[:], true)
			require.Equal(t, false, attr.IsEmpty())
			for _, poolSize := range []int{0, 50} {
				for _, currentFull := range []bool{true, false} {
					name := fmt.Sprintf("warm_%t/slot_%d/pool_%d/current_full_%t", warmed, slot, poolSize, currentFull)
					t.Run(name, func(t *testing.T) {
						transition.SkipSlotCache = cache.NewSkipSlotCache()
						for sample := range 3 {
							if poolSize > 0 {
								require.NoError(t, service.cfg.AttPool.SaveForkchoiceAttestations(updateHeadDiagnosticPool(root, poolSize)))
							}
							require.NoError(t, service.setHead(&head{
								root: root, block: headBlock, state: headState,
								slot: 0, full: currentFull, optimistic: false,
							}))
							started := time.Now()
							service.UpdateHead(t.Context(), slot)
							elapsed := time.Since(started)
							require.Equal(t, 0, service.cfg.AttPool.ForkchoiceAttestationCount())
							_, full := service.HeadRootAndFull()
							require.Equal(t, true, full)
							t.Logf("sample=%d elapsed=%s", sample+1, elapsed)
						}
					})
				}
			}
		}
	}
	require.NotNil(t, tr)
}

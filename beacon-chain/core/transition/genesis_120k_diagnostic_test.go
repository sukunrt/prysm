package transition_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const genesisDiagnosticValidatorCount = 120_000

func genesis120KState(b *testing.B) state.BeaconState {
	b.Helper()
	st, err := util.NewBeaconStateHeze(func(pb *ethpb.BeaconStateHeze) error {
		pb.Validators = make([]*ethpb.Validator, genesisDiagnosticValidatorCount)
		pb.Balances = make([]uint64, genesisDiagnosticValidatorCount)
		pb.PreviousEpochParticipation = make([]byte, genesisDiagnosticValidatorCount)
		pb.CurrentEpochParticipation = make([]byte, genesisDiagnosticValidatorCount)
		pb.InactivityScores = make([]uint64, genesisDiagnosticValidatorCount)
		for i := range pb.Validators {
			pb.Validators[i] = &ethpb.Validator{
				PublicKey:                  make([]byte, 48),
				WithdrawalCredentials:      make([]byte, 32),
				EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
				ActivationEligibilityEpoch: 0,
				ActivationEpoch:            0,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
			}
			pb.Balances[i] = params.BeaconConfig().MaxEffectiveBalance
		}
		return nil
	})
	require.NoError(b, err)
	return st
}

func BenchmarkGenesis120KProcessSlots(b *testing.B) {
	coldState := genesis120KState(b)
	st := coldState.Copy()
	_, err := st.HashTreeRoot(b.Context())
	require.NoError(b, err)
	for _, target := range []primitives.Slot{1, 2, 3} {
		name := fmt.Sprintf("slot_%d", target)
		b.Run(name+"/uninitialized_parent", func(b *testing.B) {
			transition.SkipSlotCache.Disable()
			defer transition.SkipSlotCache.Enable()
			b.ReportAllocs()
			for b.Loop() {
				_, err := transition.ProcessSlots(b.Context(), coldState.Copy(), target)
				require.NoError(b, err)
			}
		})
		b.Run(name+"/copy", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = st.Copy()
			}
		})
		b.Run(name+"/cache_disabled", func(b *testing.B) {
			transition.SkipSlotCache.Disable()
			defer transition.SkipSlotCache.Enable()
			b.ReportAllocs()
			for b.Loop() {
				_, err := transition.ProcessSlots(b.Context(), st.Copy(), target)
				require.NoError(b, err)
			}
		})
		b.Run(name+"/cache_cold", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				transition.SkipSlotCache = cache.NewSkipSlotCache()
				_, err := transition.ProcessSlots(b.Context(), st.Copy(), target)
				require.NoError(b, err)
			}
		})
		b.Run(name+"/cache_warm_equal_target", func(b *testing.B) {
			transition.SkipSlotCache = cache.NewSkipSlotCache()
			_, err := transition.ProcessSlots(b.Context(), st.Copy(), target)
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, err := transition.ProcessSlots(b.Context(), st.Copy(), target)
				require.NoError(b, err)
			}
		})
		b.Run(name+"/parent_prepare", func(b *testing.B) {
			parentRoot := make([]byte, 32)
			parentRoot[0] = 1
			b.ReportAllocs()
			for b.Loop() {
				transition.SkipSlotCache = cache.NewSkipSlotCache()
				_, err := transition.ProcessSlotsIfNeeded(b.Context(), st, parentRoot, target)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkGenesis120KConcurrentProcessSlots(b *testing.B) {
	st := genesis120KState(b)
	_, err := st.HashTreeRoot(b.Context())
	require.NoError(b, err)
	for _, target := range []primitives.Slot{1, 2, 3} {
		for _, concurrency := range []int{75, 1000} {
			b.Run(fmt.Sprintf("slot_%d/requests_%d", target, concurrency), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					transition.SkipSlotCache = cache.NewSkipSlotCache()
					start := make(chan struct{})
					errs := make(chan error, concurrency)
					var wg sync.WaitGroup
					wg.Add(concurrency)
					for range concurrency {
						go func() {
							defer wg.Done()
							<-start
							_, err := transition.ProcessSlots(context.Background(), st.Copy(), target)
							errs <- err
						}()
					}
					close(start)
					wg.Wait()
					close(errs)
					for err := range errs {
						require.NoError(b, err)
					}
				}
			})
		}
	}
}

func BenchmarkGenesis120KEpochOneDuties(b *testing.B) {
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = 2500
	params.OverrideBeaconConfig(cfg)

	cold := genesis120KState(b)
	warm := cold.Copy()
	_, err := warm.HashTreeRoot(b.Context())
	require.NoError(b, err)
	for _, tc := range []struct {
		name string
		st   state.BeaconState
	}{
		{name: "cold", st: cold},
		{name: "root_warm", st: warm},
	} {
		b.Run(tc.name+"/single", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				transition.SkipSlotCache = cache.NewSkipSlotCache()
				_, err := transition.ProcessSlots(b.Context(), tc.st.Copy(), 32)
				require.NoError(b, err)
			}
		})
		b.Run(tc.name+"/fanout_4", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				transition.SkipSlotCache = cache.NewSkipSlotCache()
				start := make(chan struct{})
				errs := make(chan error, 4)
				var wg sync.WaitGroup
				wg.Add(4)
				for range 4 {
					go func() {
						defer wg.Done()
						<-start
						_, err := transition.ProcessSlots(context.Background(), tc.st.Copy(), 32)
						errs <- err
					}()
				}
				close(start)
				wg.Wait()
				close(errs)
				for err := range errs {
					require.NoError(b, err)
				}
			}
		})
	}

	b.Run("cold/epoch_one_blocks_slot_one", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			transition.SkipSlotCache = cache.NewSkipSlotCache()
			epochDone := make(chan error, 1)
			go func() {
				_, err := transition.ProcessSlots(context.Background(), cold.Copy(), 32)
				epochDone <- err
			}()
			time.Sleep(time.Millisecond)
			_, err := transition.ProcessSlots(b.Context(), cold.Copy(), 1)
			require.NoError(b, err)
			require.NoError(b, <-epochDone)
		}
	})
}

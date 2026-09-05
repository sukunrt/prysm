package transition_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
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

// BenchmarkGenesis120KProposalSlotPreparation measures the exact skipped-slot
// transition shape used by getParentState for the startup proposer slots. The
// fanout case shares the production SkipSlotCache key and therefore includes
// its in-progress serialization; it is a diagnostic total, not a simulation of
// the historical gossip mix.
func BenchmarkGenesis120KProposalSlotPreparation(b *testing.B) {
	params.SetupTestConfigCleanup(b)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = 8
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	dirty := genesis120KState(b)
	warm := dirty.Copy()
	_, err := warm.HashTreeRoot(b.Context())
	require.NoError(b, err)

	for _, fixture := range []struct {
		name string
		st   state.BeaconState
	}{
		{name: "merkle_dirty", st: dirty},
		{name: "merkle_warm", st: warm},
	} {
		for _, target := range []primitives.Slot{5, 8, 10, 13, 14} {
			for _, concurrency := range []int{1, 64} {
				b.Run(fmt.Sprintf("%s/slot_%d/requests_%d", fixture.name, target, concurrency), func(b *testing.B) {
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
								_, err := transition.ProcessSlots(context.Background(), fixture.st.Copy(), target)
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
}

// BenchmarkRetainedGenesisProposalSlotPreparation repeats the slot-13 probe
// against a retained, generated genesis rather than the synthetic zero-pubkey
// fixture. Set PRYSM_STARTUP_REAL_GENESIS_SSZ to override the retained path.
// Its sibling config.yaml is loaded before decoding because SSZ list limits are
// chain-config dependent. The benchmark skips when either artifact is absent.
func BenchmarkRetainedGenesisProposalSlotPreparation(b *testing.B) {
	genesisPath := os.Getenv("PRYSM_STARTUP_REAL_GENESIS_SSZ")
	if genesisPath == "" {
		genesisPath = "/tmp/prysm-startup3-wire-h/bundle/network-configs/genesis.ssz"
	}
	configPath := filepath.Join(filepath.Dir(genesisPath), "config.yaml")
	if _, err := os.Stat(genesisPath); err != nil {
		b.Skipf("retained genesis unavailable: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		b.Skipf("retained chain config unavailable: %v", err)
	}
	params.SetupTestConfigCleanup(b)
	require.NoError(b, params.LoadChainConfigFile(configPath, nil))
	require.Equal(b, primitives.Slot(4), params.BeaconConfig().SlotsPerRound)
	require.Equal(b, primitives.Epoch(0), params.BeaconConfig().HezeForkEpoch)
	raw, err := os.ReadFile(genesisPath)
	require.NoError(b, err)
	pb := &ethpb.BeaconStateHeze{}
	require.NoError(b, pb.UnmarshalSSZ(raw))
	retained, err := state_native.InitializeFromProtoUnsafeHeze(pb)
	require.NoError(b, err)
	require.Equal(b, genesisDiagnosticValidatorCount, retained.NumValidators())

	warm := retained.Copy()
	_, err = warm.HashTreeRoot(b.Context())
	require.NoError(b, err)
	for _, fixture := range []struct {
		name string
		st   state.BeaconState
	}{
		{name: "merkle_dirty", st: retained},
		{name: "merkle_warm", st: warm},
	} {
		for _, concurrency := range []int{1, 4} {
			b.Run(fmt.Sprintf("%s/slot_13/requests_%d", fixture.name, concurrency), func(b *testing.B) {
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
							_, err := transition.ProcessSlots(context.Background(), fixture.st.Copy(), 13)
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

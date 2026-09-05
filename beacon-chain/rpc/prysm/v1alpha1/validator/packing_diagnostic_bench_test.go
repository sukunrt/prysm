package validator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const diagnosticCommitteeSize = 2500

func diagnosticPackingAtts(n, committees, width int, sig []byte) []ethpb.Att {
	atts := make([]ethpb.Att, 0, n)
	perCommittee := n / committees
	for i := range n {
		committee := i / perCommittee
		position := i % perCommittee
		bits := bitfield.NewBitlist(diagnosticCommitteeSize)
		for j := range width {
			bits.SetBitAt(uint64((position+j)%perCommittee), true)
		}
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(committee), true)
		blockRoot := make([]byte, 32)
		blockRoot[0] = byte(committee + 1)
		atts = append(atts, &ethpb.AttestationElectra{
			AggregationBits: bits,
			CommitteeBits:   committeeBits,
			Data: &ethpb.AttestationData{
				Slot:            1,
				BeaconBlockRoot: blockRoot,
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
			Signature: sig,
		})
	}
	return atts
}

func diagnosticPackingSignature(b *testing.B) []byte {
	b.Helper()
	key, err := bls.RandKey()
	if err != nil {
		b.Fatal(err)
	}
	return key.Sign([]byte("packing diagnostic")).Marshal()
}

func BenchmarkDiagnosticPackingDisjointSingles(b *testing.B) {
	sig := diagnosticPackingSignature(b)
	for _, n := range []int{10, 50, 100, 250, 500, 1000, 2500} {
		b.Run(fmt.Sprintf("n_%d/dedup", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				atts := diagnosticPackingAtts(n, 1, 1, sig)
				b.StartTimer()
				if _, err := proposerAtts(atts).dedup(); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("n_%d/max_cover", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				atts := diagnosticPackingAtts(n, 1, 1, sig)
				b.StartTimer()
				if _, err := attaggregation.Aggregate(atts); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("n_%d/disjoint_one_bit", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				atts := diagnosticPackingAtts(n, 1, 1, sig)
				b.StartTimer()
				if _, err := attaggregation.AggregateDisjointOneBitAtts(atts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDiagnosticPackingOverlappingAggregates(b *testing.B) {
	sig := diagnosticPackingSignature(b)
	for _, n := range []int{10, 50, 100, 250, 500, 1000} {
		for _, operation := range []struct {
			name string
			fn   func([]ethpb.Att) error
		}{
			{name: "dedup", fn: func(atts []ethpb.Att) error { _, err := proposerAtts(atts).dedup(); return err }},
			{name: "max_cover", fn: func(atts []ethpb.Att) error { _, err := attaggregation.Aggregate(atts); return err }},
		} {
			b.Run(fmt.Sprintf("n_%d/%s", n, operation.name), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					atts := diagnosticPackingAtts(n, 1, 2, sig)
					b.StartTimer()
					if err := operation.fn(atts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkDiagnosticPackingSixCommitteesDedup(b *testing.B) {
	sig := diagnosticPackingSignature(b)
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		atts := diagnosticPackingAtts(15_000, 6, 1, sig)
		b.StartTimer()
		if _, err := proposerAtts(atts).dedup(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDiagnosticPackingMergeByData(b *testing.B) {
	sig := diagnosticPackingSignature(b)
	for _, n := range []int{10, 50, 100, 250, 500, 1000, 2500} {
		b.Run(fmt.Sprintf("n_%d/sequential", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				atts := diagnosticPackingAtts(n, 1, 1, sig)
				b.StartTimer()
				if _, err := mergeByData(atts); err != nil {
					b.Fatal(err)
				}
			}
		})
		for _, concurrency := range []int{4, 16} {
			b.Run(fmt.Sprintf("n_%d/concurrent_%d", n, concurrency), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					inputs := make([][]*ethpb.AttestationElectra, concurrency)
					for i := range inputs {
						generic := diagnosticPackingAtts(n, 1, 1, sig)
						inputs[i] = make([]*ethpb.AttestationElectra, n)
						for j := range generic {
							inputs[i][j] = generic[j].(*ethpb.AttestationElectra)
						}
					}
					b.StartTimer()
					start := make(chan struct{})
					errs := make(chan error, concurrency)
					var wg sync.WaitGroup
					wg.Add(concurrency)
					for _, atts := range inputs {
						go func() {
							defer wg.Done()
							<-start
							_, err := mergeByData(atts)
							errs <- err
						}()
					}
					close(start)
					wg.Wait()
					close(errs)
					for err := range errs {
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

type fullPackingShape string

const (
	fullPackingPrevious fullPackingShape = "previous_round_valid"
	fullPackingMixed    fullPackingShape = "mixed_previous_current_valid"
	fullPackingInvalid  fullPackingShape = "wrong_source_invalid"
)

func fullPackingFixture(tb testing.TB, n int, blockSlot primitives.Slot, shape fullPackingShape) (*Server, stateFixture) {
	tb.Helper()
	params.SetupTestConfigCleanup(tb)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.SlotsPerRound = 8
	cfg.TargetCommitteeSize = diagnosticCommitteeSize
	cfg.MaxCommitteesPerSlot = 6
	params.OverrideBeaconConfig(cfg)
	tb.Cleanup(features.InitWithReset(&features.Flags{}))
	helpers.ClearCache()

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
	require.NoError(tb, err)
	require.NoError(tb, st.SetSlot(blockSlot))
	previous := &ethpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)}
	current := &ethpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)}
	require.NoError(tb, st.SetPreviousJustifiedCheckpoint(previous))
	require.NoError(tb, st.SetCurrentJustifiedCheckpoint(current))

	previousSlot := primitives.Slot(7)
	currentSlot := blockSlot - 1
	committees := make(map[primitives.Slot][][]primitives.ValidatorIndex)
	for _, slot := range []primitives.Slot{previousSlot, currentSlot} {
		committees[slot] = make([][]primitives.ValidatorIndex, 6)
		for ci := range 6 {
			committees[slot][ci], err = helpers.BeaconCommitteeFromState(tb.Context(), st, slot, primitives.CommitteeIndex(ci))
			require.NoError(tb, err)
			require.Equal(tb, diagnosticCommitteeSize, len(committees[slot][ci]))
		}
	}

	keys := make(map[primitives.ValidatorIndex]bls.SecretKey)
	atts := make([]ethpb.Att, 0, n)
	for i := range n {
		attSlot := previousSlot
		target := previous
		if shape == fullPackingMixed && i%2 == 1 {
			attSlot = currentSlot
			target = current
		}
		ci := i % 6
		position := (i / 6) % diagnosticCommitteeSize
		validatorIndex := committees[attSlot][ci][position]
		key := keys[validatorIndex]
		if key == nil {
			key, err = bls.RandKey()
			require.NoError(tb, err)
			keys[validatorIndex] = key
			validator := validators[validatorIndex]
			validator.PublicKey = key.PublicKey().Marshal()
			require.NoError(tb, st.UpdateValidatorAtIndex(validatorIndex, validator))
		}
		bits := bitfield.NewBitlist(diagnosticCommitteeSize)
		bits.SetBitAt(uint64(position), true)
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(ci), true)
		root := make([]byte, 32)
		root[0], root[1] = byte(attSlot), byte(ci+1)
		source := previous
		if target.Epoch == 1 {
			source = current
		}
		if shape == fullPackingInvalid {
			source = &ethpb.Checkpoint{Epoch: source.Epoch, Root: bytes.Repeat([]byte{0xff}, 32)}
		}
		data := &ethpb.AttestationData{Slot: attSlot, BeaconBlockRoot: root, Source: source, Target: target}
		sig, signErr := signing.ComputeDomainAndSign(st, slots.ToEpoch(attSlot), data, cfg.DomainBeaconAttester, key)
		require.NoError(tb, signErr)
		atts = append(atts, &ethpb.AttestationElectra{AggregationBits: bits, CommitteeBits: committeeBits, Data: data, Signature: sig})
	}
	pool := attpool.NewPool()
	require.NoError(tb, pool.SaveUnaggregatedAttestations(atts))
	headSlot := blockSlot
	chain := &mockchain.ChainService{MockHeadSlot: &headSlot, Slot: &headSlot}
	return &Server{AttPool: pool, HeadFetcher: chain, TimeFetcher: chain, ForkchoiceFetcher: chain}, stateFixture{state: st, atts: atts}
}

type stateFixture struct {
	state state.BeaconState
	atts  []ethpb.Att
}

func TestDiagnosticFullPackingFixtureRoundSemantics(t *testing.T) {
	for _, shape := range []fullPackingShape{fullPackingPrevious, fullPackingMixed, fullPackingInvalid} {
		t.Run(string(shape), func(t *testing.T) {
			server, fixture := fullPackingFixture(t, 12, 14, shape)
			wantValid := shape != fullPackingInvalid
			for _, att := range fixture.atts {
				err := blocks.VerifyAttestationNoVerifySignature(t.Context(), fixture.state, att)
				require.Equal(t, wantValid, err == nil)
			}
			packed, err := server.packAttestations(t.Context(), fixture.state, 14)
			require.NoError(t, err)
			if wantValid {
				require.NotEqual(t, 0, len(packed))
			} else {
				require.Equal(t, 0, len(packed))
				require.Equal(t, 0, len(server.AttPool.UnaggregatedAttestations()))
			}
		})
	}
}

func BenchmarkDiagnosticFullProductionPacker(b *testing.B) {
	for _, blockSlot := range []primitives.Slot{10, 14} {
		for _, n := range []int{600, 3000, 15_000} {
			for _, shape := range []fullPackingShape{fullPackingPrevious, fullPackingMixed, fullPackingInvalid} {
				b.Run(fmt.Sprintf("slot_%d/n_%d/%s", blockSlot, n, shape), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						b.StopTimer()
						server, fixture := fullPackingFixture(b, n, blockSlot, shape)
						b.StartTimer()
						packed, err := server.packAttestations(context.Background(), fixture.state, blockSlot)
						b.StopTimer()
						require.NoError(b, err)
						if shape == fullPackingInvalid {
							require.Equal(b, 0, len(packed))
							require.Equal(b, 0, len(server.AttPool.UnaggregatedAttestations()))
						} else {
							require.NotEqual(b, 0, len(packed))
						}
						b.StartTimer()
					}
				})
			}
		}
	}
}

// TestDiagnosticFullPackingCPUProfile profiles only the production packing call;
// fixture construction and signature generation are deliberately outside it.
func TestDiagnosticFullPackingCPUProfile(t *testing.T) {
	path := os.Getenv("PRYSM_DIAGNOSTIC_PACKING_CPU_PROFILE")
	if path == "" {
		t.Skip("set PRYSM_DIAGNOSTIC_PACKING_CPU_PROFILE to collect the diagnostic profile")
	}
	server, fixture := fullPackingFixture(t, 15_000, 14, fullPackingMixed)
	f, err := os.Create(path) // #nosec G304 -- explicitly supplied diagnostic output path.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.NoError(t, pprof.StartCPUProfile(f))
	packed, err := server.packAttestations(t.Context(), fixture.state, 14)
	pprof.StopCPUProfile()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(packed))
}

func TestDiagnosticFullPackingCancellationAndGenesisScanLoad(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_FULL_PACKING_LOAD") != "1" {
		t.Skip("set PRYSM_DIAGNOSTIC_FULL_PACKING_LOAD=1 to run the bounded load diagnostic")
	}

	t.Run("deadline_observation", func(t *testing.T) {
		server, fixture := fullPackingFixture(t, 15_000, 14, fullPackingMixed)
		for _, deadline := range []time.Duration{300 * time.Millisecond, time.Second} {
			t.Run(deadline.String(), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), deadline)
				defer cancel()
				start := time.Now()
				packed, err := server.packAttestations(ctx, fixture.state, 14)
				elapsed := time.Since(start)
				require.Equal(t, true, ctx.Err() != nil)
				require.Equal(t, true, elapsed > deadline)
				t.Logf("deadline=%s elapsed=%s packed=%d err=%v", deadline, elapsed, len(packed), err)
			})
		}
	})

	for _, concurrency := range []int{64, 256, 1024} {
		for _, slotZero := range []bool{false, true} {
			name := "cached_slot_1"
			if slotZero {
				name = "genesis_slot_0_scan"
			}
			t.Run(fmt.Sprintf("concurrency_%d/%s", concurrency, name), func(t *testing.T) {
				server, fixture := fullPackingFixture(t, 15_000, 14, fullPackingMixed)
				scanState := fixture.state.Copy()
				if slotZero {
					require.NoError(t, scanState.SetSlot(0))
				} else {
					require.NoError(t, scanState.SetSlot(1))
					_, err := helpers.ActiveValidatorCount(t.Context(), scanState, 0)
					require.NoError(t, err)
				}

				startGate := make(chan struct{})
				durations := make(chan time.Duration, concurrency)
				var wg sync.WaitGroup
				wg.Add(concurrency)
				for range concurrency {
					go func() {
						defer wg.Done()
						<-startGate
						start := time.Now()
						_, err := helpers.ActiveValidatorCount(t.Context(), scanState, 0)
						require.NoError(t, err)
						durations <- time.Since(start)
					}()
				}
				dispatch := time.Now()
				close(startGate)
				packStart := time.Now()
				packed, err := server.packAttestations(t.Context(), fixture.state, 14)
				packElapsed := time.Since(packStart)
				require.NoError(t, err)
				require.NotEqual(t, 0, len(packed))
				wg.Wait()
				close(durations)
				allElapsed := time.Since(dispatch)
				var maxScan time.Duration
				for d := range durations {
					if d > maxScan {
						maxScan = d
					}
				}
				t.Logf("concurrency=%d slot_zero=%t pack=%s scans_complete=%s max_scan=%s", concurrency, slotZero, packElapsed, allElapsed, maxScan)
			})
		}
	}
}

func TestDiagnosticFullPackingSustainedGenesisScanLoad(t *testing.T) {
	if os.Getenv("PRYSM_DIAGNOSTIC_FULL_PACKING_SUSTAINED") != "1" {
		t.Skip("set PRYSM_DIAGNOSTIC_FULL_PACKING_SUSTAINED=1 to run the sustained diagnostic")
	}
	const jobs = 15_000
	const workers = 6_144

	for _, slotZero := range []bool{false, true} {
		name := "cached_slot_1"
		if slotZero {
			name = "genesis_slot_0_scan"
		}
		t.Run(name, func(t *testing.T) {
			server, fixture := fullPackingFixture(t, 15_000, 14, fullPackingMixed)
			scanState := fixture.state.Copy()
			if slotZero {
				require.NoError(t, scanState.SetSlot(0))
			} else {
				require.NoError(t, scanState.SetSlot(1))
				_, err := helpers.ActiveValidatorCount(t.Context(), scanState, 0)
				require.NoError(t, err)
			}

			jobCh := make(chan struct{}, jobs)
			for range jobs {
				jobCh <- struct{}{}
			}
			close(jobCh)
			startGate := make(chan struct{})
			var completed atomic.Uint64
			var wg sync.WaitGroup
			wg.Add(workers)
			for range workers {
				go func() {
					defer wg.Done()
					<-startGate
					for range jobCh {
						count, err := helpers.ActiveValidatorCount(t.Context(), scanState, 0)
						if err != nil || count != 120_000 {
							t.Errorf("ActiveValidatorCount() = %d, %v", count, err)
							return
						}
						completed.Add(1)
					}
				}()
			}

			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			dispatch := time.Now()
			close(startGate)
			packStart := time.Now()
			packed, packErr := server.packAttestations(ctx, fixture.state, 14)
			packElapsed := time.Since(packStart)
			wg.Wait()
			allElapsed := time.Since(dispatch)
			require.Equal(t, uint64(jobs), completed.Load())
			if packElapsed < 12*time.Second {
				require.NoError(t, packErr)
				require.NotEqual(t, 0, len(packed))
			} else {
				require.Equal(t, true, ctx.Err() != nil)
				require.Equal(t, 0, len(packed))
			}
			t.Logf("jobs=%d workers=%d slot_zero=%t pack=%s scans_complete=%s packed=%d err=%v", jobs, workers, slotZero, packElapsed, allElapsed, len(packed), packErr)
		})
	}
}

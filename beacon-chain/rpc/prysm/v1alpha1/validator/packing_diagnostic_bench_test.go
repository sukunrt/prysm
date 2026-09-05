package validator

import (
	"fmt"
	"sync"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
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

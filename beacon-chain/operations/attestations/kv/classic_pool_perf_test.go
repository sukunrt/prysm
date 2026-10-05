package kv

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

type classicPoolFixture struct {
	atts []ethpb.Att
	pubs []bls.PublicKey
	msg  [32]byte
}

func newClassicPoolFixture(tb testing.TB, seats int) classicPoolFixture {
	tb.Helper()
	data := util.HydrateAttestationData(&ethpb.AttestationData{Slot: 1})
	msg, err := data.HashTreeRoot()
	if err != nil {
		tb.Fatal(err)
	}
	f := classicPoolFixture{atts: make([]ethpb.Att, seats), pubs: make([]bls.PublicKey, seats), msg: msg}
	for i := 0; i < seats; i++ {
		var secret [32]byte
		binary.BigEndian.PutUint64(secret[24:], uint64(i+1))
		key, err := bls.SecretKeyFromBytes(secret[:])
		if err != nil {
			tb.Fatal(err)
		}
		bits := bitfield.NewBitlist(uint64(seats))
		bits.SetBitAt(uint64(i), true)
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(0, true)
		f.pubs[i] = key.PublicKey()
		f.atts[i] = util.HydrateAttestationElectra(&ethpb.AttestationElectra{
			AggregationBits: bits,
			CommitteeBits:   committeeBits,
			Data:            data,
			Signature:       key.Sign(msg[:]).Marshal(),
		})
	}
	return f
}

func checkClassicPoolCandidate(tb testing.TB, f classicPoolFixture, candidates []ethpb.Att) {
	tb.Helper()
	if len(candidates) != 1 {
		tb.Fatalf("got %d candidates, want one", len(candidates))
	}
	got := candidates[0]
	if got.GetAggregationBits().Count() != uint64(len(f.atts)) {
		tb.Fatalf("candidate covers %d of %d seats", got.GetAggregationBits().Count(), len(f.atts))
	}
	for i := range f.atts {
		if !got.GetAggregationBits().BitAt(uint64(i)) {
			tb.Fatalf("candidate omits seat %d", i)
		}
	}
	sig, err := bls.SignatureFromBytes(got.GetSignature())
	if err != nil {
		tb.Fatal(err)
	}
	if !sig.FastAggregateVerify(f.pubs, f.msg) {
		tb.Fatal("aggregate BLS signature does not verify")
	}
}

func checkClassicPoolCoverage(tb testing.TB, f classicPoolFixture, candidates []ethpb.Att) {
	tb.Helper()
	covered := make([]bool, len(f.atts))
	for _, candidate := range candidates {
		if candidate == nil {
			tb.Fatal("nil candidate")
		}
		pubs := make([]bls.PublicKey, 0, candidate.GetAggregationBits().Count())
		for _, seat := range candidate.GetAggregationBits().BitIndices() {
			if seat >= len(f.atts) {
				tb.Fatalf("candidate has out-of-range seat %d", seat)
			}
			covered[seat] = true
			pubs = append(pubs, f.pubs[seat])
		}
		sig, err := bls.SignatureFromBytes(candidate.GetSignature())
		if err != nil || !sig.FastAggregateVerify(pubs, f.msg) {
			tb.Fatalf("candidate signature invalid: %v", err)
		}
	}
	for i, yes := range covered {
		if !yes {
			tb.Fatalf("consumer candidates omit seat %d", i)
		}
	}
}

func TestClassicPoolSinglesCollapse(t *testing.T) {
	// 2500 exceeds mainnet's 2048-seat committee limit; it also exercises the
	// requested large single-ID stress case for this pool algorithm.
	for _, seats := range []int{1, 2500} {
		t.Run(fmt.Sprint(seats), func(t *testing.T) {
			f := newClassicPoolFixture(t, seats)
			pool := NewAttCaches()
			for _, a := range f.atts {
				has, err := pool.HasAggregatedAttestation(a)
				if err != nil || has {
					t.Fatalf("initial preflight: has=%t err=%v", has, err)
				}
				if err := pool.SaveUnaggregatedAttestation(a); err != nil {
					t.Fatal(err)
				}
			}
			if seats > 1 {
				if got := len(pool.unAggregatedAtt); got != 0 {
					t.Fatalf("%d original singleton records remain immediately after ingestion", got)
				}
				checkClassicPoolCandidate(t, f, pool.AggregatedAttestations())
			}
			if err := pool.AggregateUnaggregatedAttestations(t.Context()); err != nil {
				t.Fatal(err)
			}
			if seats == 1 {
				if got := pool.UnaggregatedAttestations(); len(got) != 1 || len(pool.AggregatedAttestations()) != 0 {
					t.Fatalf("singleton changed pools: unaggregated=%d aggregated=%d", len(got), len(pool.AggregatedAttestations()))
				}
				return
			}
			if got := len(pool.unAggregatedAtt); got != 0 {
				t.Fatalf("%d original singleton records remain", got)
			}
			checkClassicPoolCandidate(t, f, pool.AggregatedAttestations())
		})
	}
}

// BenchmarkClassicPoolSingles measures ingress, flush, consumer aggregation,
// deletion and duplicate lookups on the same public API for baseline and fix.
func BenchmarkClassicPoolSingles(b *testing.B) {
	for _, seats := range []int{250, 500, 1000, 2500} {
		b.Run(fmt.Sprint(seats), func(b *testing.B) {
			f := newClassicPoolFixture(b, seats)
			ctx := context.Background()
			var ingest, flush, consumer, cleanup time.Duration
			var last []ethpb.Att
			var rawSingles, retainedCandidates int
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pool := NewAttCaches()
				start := time.Now()
				for _, a := range f.atts {
					has, err := pool.HasAggregatedAttestation(a)
					if err != nil || has {
						b.Fatalf("preflight: has=%t err=%v", has, err)
					}
					if err := pool.SaveUnaggregatedAttestation(a); err != nil {
						b.Fatal(err)
					}
				}
				ingest += time.Since(start)
				rawSingles = len(pool.unAggregatedAtt)
				retainedCandidates = rawSingles + len(pool.AggregatedAttestations())
				start = time.Now()
				if err := pool.AggregateUnaggregatedAttestations(ctx); err != nil {
					b.Fatal(err)
				}
				flush += time.Since(start)
				start = time.Now()
				var err error
				last, err = attaggregation.Aggregate(pool.AggregatedAttestations())
				if err != nil {
					b.Fatal(err)
				}
				consumer += time.Since(start)
				start = time.Now()
				for _, a := range last {
					if err := pool.DeleteAggregatedAttestation(a); err != nil {
						b.Fatal(err)
					}
				}
				for _, a := range f.atts {
					has, err := pool.HasAggregatedAttestation(a)
					if err != nil || !has {
						b.Fatalf("duplicate lookup: has=%t err=%v", has, err)
					}
				}
				if _, err := pool.DeleteSeenUnaggregatedAttestations(); err != nil {
					b.Fatal(err)
				}
				cleanup += time.Since(start)
			}
			b.StopTimer()
			checkClassicPoolCandidate(b, f, last)
			b.ReportMetric(float64(ingest.Nanoseconds())/float64(b.N), "ingest-ns/op")
			b.ReportMetric(float64(flush.Nanoseconds())/float64(b.N), "flush-ns/op")
			b.ReportMetric(float64(consumer.Nanoseconds())/float64(b.N), "consumer-ns/op")
			b.ReportMetric(float64(cleanup.Nanoseconds())/float64(b.N), "cleanup-ns/op")
			b.ReportMetric(float64(rawSingles), "raw-singles/op")
			b.ReportMetric(float64(retainedCandidates), "retained-candidates/op")
		})
	}
}

// BenchmarkClassicPoolConsumerBeforeFlush measures the candidate set seen by a
// consumer between gossip ingestion and the next pool aggregation tick.
func BenchmarkClassicPoolConsumerBeforeFlush(b *testing.B) {
	for _, seats := range []int{250, 500, 1000, 2500} {
		b.Run(fmt.Sprint(seats), func(b *testing.B) {
			f := newClassicPoolFixture(b, seats)
			var ingress, consumer time.Duration
			var result []ethpb.Att
			var rawSingles, retainedCandidates int
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pool := NewAttCaches()
				start := time.Now()
				for _, a := range f.atts {
					has, err := pool.HasAggregatedAttestation(a)
					if err != nil || has {
						b.Fatalf("preflight: has=%t err=%v", has, err)
					}
					if err := pool.SaveUnaggregatedAttestation(a); err != nil {
						b.Fatal(err)
					}
				}
				ingress += time.Since(start)
				start = time.Now()
				candidates := append(pool.AggregatedAttestations(), pool.UnaggregatedAttestations()...)
				rawSingles = len(pool.unAggregatedAtt)
				retainedCandidates = len(candidates)
				var err error
				result, err = attaggregation.Aggregate(candidates)
				if err != nil {
					b.Fatal(err)
				}
				consumer += time.Since(start)
			}
			b.StopTimer()
			checkClassicPoolCoverage(b, f, result)
			b.ReportMetric(float64(ingress.Nanoseconds())/float64(b.N), "ingest-ns/op")
			b.ReportMetric(float64(consumer.Nanoseconds())/float64(b.N), "consumer-ns/op")
			b.ReportMetric(float64(len(result)), "candidates/op")
			b.ReportMetric(float64(rawSingles), "raw-singles/op")
			b.ReportMetric(float64(retainedCandidates), "retained-candidates/op")
		})
	}
}

// BenchmarkClassicPoolSeenLookup fixes bitlist width while growing the number
// of processed aggregate masks. The absent one-bit query includes ID hashing
// and bit-position extraction, matching a pool seen-bit check.
func BenchmarkClassicPoolSeenLookup(b *testing.B) {
	const width = 4096
	data := util.HydrateAttestationData(&ethpb.AttestationData{Slot: 1})
	msg, err := data.HashTreeRoot()
	if err != nil {
		b.Fatal(err)
	}
	var secret [32]byte
	secret[31] = 1
	key, err := bls.SecretKeyFromBytes(secret[:])
	if err != nil {
		b.Fatal(err)
	}
	sig := key.Sign(msg[:]).Marshal()
	committeeBits := primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(0, true)
	for _, masks := range []int{250, 500, 1000, 2500} {
		b.Run(fmt.Sprint(masks), func(b *testing.B) {
			pool := NewAttCaches()
			for i := 0; i < masks; i++ {
				bits := bitfield.NewBitlist(width)
				bits.SetBitAt(0, true)
				bits.SetBitAt(uint64(i+1), true)
				a := util.HydrateAttestationElectra(&ethpb.AttestationElectra{
					AggregationBits: bits,
					CommitteeBits:   committeeBits,
					Data:            data,
					Signature:       sig,
				})
				if err := pool.insertSeenBit(a); err != nil {
					b.Fatal(err)
				}
			}
			missing := bitfield.NewBitlist(width)
			missing.SetBitAt(width-1, true)
			query := util.HydrateAttestationElectra(&ethpb.AttestationElectra{
				AggregationBits: missing,
				CommitteeBits:   committeeBits,
				Data:            data,
				Signature:       sig,
			})
			if seen, err := pool.hasSeenBit(query); err != nil || seen {
				b.Fatalf("absent bit lookup: seen=%t err=%v", seen, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				seen, err := pool.hasSeenBit(query)
				if err != nil || seen {
					b.Fatalf("absent bit lookup: seen=%t err=%v", seen, err)
				}
			}
			b.ReportMetric(float64(masks), "stored-masks/op")
		})
	}
}

// BenchmarkClassicPoolConcurrentGroups measures independent committees arriving
// concurrently, including the batch aggregation tick used by the old pool.
func BenchmarkClassicPoolConcurrentGroups(b *testing.B) {
	const groups, seats = 16, 250
	f := newClassicPoolFixture(b, seats)
	atts := make([][]ethpb.Att, groups)
	for group := range atts {
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(group), true)
		atts[group] = make([]ethpb.Att, seats)
		for seat, original := range f.atts {
			atts[group][seat] = &ethpb.AttestationElectra{
				AggregationBits: original.GetAggregationBits(),
				CommitteeBits:   committeeBits,
				Data:            original.GetData(),
				Signature:       original.GetSignature(),
			}
		}
	}
	ctx := context.Background()
	var ingress, flush time.Duration
	var rawSingles, retainedCandidates int
	var last []ethpb.Att
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		pool := NewAttCaches()
		start := time.Now()
		var wg sync.WaitGroup
		errs := make(chan error, groups)
		for group := range atts {
			wg.Add(1)
			go func(group int) {
				defer wg.Done()
				for _, a := range atts[group] {
					has, err := pool.HasAggregatedAttestation(a)
					if err != nil || has {
						errs <- fmt.Errorf("group %d preflight: has=%t err=%v", group, has, err)
						return
					}
					if err := pool.SaveUnaggregatedAttestation(a); err != nil {
						errs <- fmt.Errorf("group %d save: %w", group, err)
						return
					}
				}
			}(group)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			b.Fatal(err)
		}
		ingress += time.Since(start)
		rawSingles = len(pool.unAggregatedAtt)
		retainedCandidates = rawSingles + len(pool.AggregatedAttestations())
		start = time.Now()
		if err := pool.AggregateUnaggregatedAttestations(ctx); err != nil {
			b.Fatal(err)
		}
		flush += time.Since(start)
		last = pool.AggregatedAttestations()
	}
	b.StopTimer()
	if len(last) != groups {
		b.Fatalf("got %d aggregated candidates, want %d groups", len(last), groups)
	}
	seen := make([]bool, groups)
	for _, a := range last {
		indices := a.CommitteeBitsVal().BitIndices()
		if len(indices) != 1 || indices[0] < 0 || indices[0] >= groups {
			b.Fatalf("invalid committee bits %v", indices)
		}
		group := indices[0]
		if seen[group] {
			b.Fatalf("duplicate group %d", group)
		}
		seen[group] = true
		checkClassicPoolCandidate(b, f, []ethpb.Att{a})
	}
	b.ReportMetric(float64(ingress.Nanoseconds())/float64(b.N), "ingest-ns/op")
	b.ReportMetric(float64(flush.Nanoseconds())/float64(b.N), "flush-ns/op")
	b.ReportMetric(float64(rawSingles), "raw-singles/op")
	b.ReportMetric(float64(retainedCandidates), "retained-candidates/op")
	b.ReportMetric(float64(groups), "groups/op")
}

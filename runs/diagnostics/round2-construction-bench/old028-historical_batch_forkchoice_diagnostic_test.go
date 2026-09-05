package attestations

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
)

const diagnosticBatchCommitteeSize = uint64(2500)

var diagnosticBatchGroupCounts = [...]uint64{2167, 2167, 2167, 2167, 2166, 2166}

type diagnosticBatchFixture struct {
	singles          []ethpb.Att
	compactElectra   []ethpb.Att
	retainedGloas    []ethpb.Att
	wantParticipants map[attestation.Id]uint64
}

func newDiagnosticBatchFixture(tb testing.TB) *diagnosticBatchFixture {
	tb.Helper()
	keys, _, err := interop.DeterministicallyGenerateKeys(1, diagnosticBatchCommitteeSize)
	if err != nil {
		tb.Fatalf("generate deterministic BLS keys: %v", err)
	}

	f := &diagnosticBatchFixture{wantParticipants: make(map[attestation.Id]uint64, 2*len(diagnosticBatchGroupCounts))}
	for group, count := range diagnosticBatchGroupCounts {
		dataRoot := make([]byte, 32)
		dataRoot[0] = byte(0x40 + group)
		targetRoot := make([]byte, 32)
		targetRoot[0] = byte(0x80 + group)
		data := &ethpb.AttestationData{
			Slot:            95,
			BeaconBlockRoot: dataRoot,
			Source:          &ethpb.Checkpoint{Epoch: 11, Root: make([]byte, 32)},
			Target:          &ethpb.Checkpoint{Epoch: 11, Root: targetRoot},
		}
		signingRoot, err := data.HashTreeRoot()
		if err != nil {
			tb.Fatalf("hash group %d attestation data: %v", group, err)
		}
		committeeBits := primitives.NewAttestationCommitteeBits()
		committeeBits.SetBitAt(uint64(group), true)

		groupSingles := make([]ethpb.Att, 0, count)
		for position := uint64(0); position < count; position++ {
			bits := bitfield.NewBitlist(diagnosticBatchCommitteeSize)
			bits.SetBitAt(position, true)
			single := &ethpb.AttestationElectra{
				AggregationBits: bits,
				CommitteeBits:   append(bitfield.Bitvector64(nil), committeeBits...),
				Data:            data.Copy(),
				Signature:       keys[position].Sign(signingRoot[:]).Marshal(),
			}
			if _, err := bls.SignatureFromBytes(single.Signature); err != nil {
				tb.Fatalf("parse group %d position %d BLS signature: %v", group, position, err)
			}
			groupSingles = append(groupSingles, single)
		}
		f.singles = append(f.singles, groupSingles...)

		compact, err := attaggregation.AggregateDisjointOneBitAtts(cloneDiagnosticBatchAtts(groupSingles))
		if err != nil {
			tb.Fatalf("precompact group %d: %v", group, err)
		}
		if got := compact.GetAggregationBits().Count(); got != count {
			tb.Fatalf("group %d compact coverage=%d, want %d", group, got, count)
		}
		f.compactElectra = append(f.compactElectra, compact)
		gloas, ok := ethpb.AttestationGloasFromAtt(compact)
		if !ok {
			tb.Fatalf("convert group %d covering aggregate to Gloas", group)
		}
		f.retainedGloas = append(f.retainedGloas, gloas)

		for _, candidate := range []ethpb.Att{compact, gloas} {
			id, err := attestation.NewId(candidate, attestation.Data)
			if err != nil {
				tb.Fatalf("derive group %d attestation ID: %v", group, err)
			}
			if _, exists := f.wantParticipants[id]; exists {
				tb.Fatalf("duplicate fixture attestation ID for group %d version %d", group, candidate.Version())
			}
			f.wantParticipants[id] = count
		}
	}
	if got := len(f.singles); got != 13000 {
		tb.Fatalf("raw single count=%d, want 13000", got)
	}
	return f
}

func cloneDiagnosticBatchAtts(atts []ethpb.Att) []ethpb.Att {
	cloned := make([]ethpb.Att, len(atts))
	for i := range atts {
		cloned[i] = atts[i].Clone()
	}
	return cloned
}

type diagnosticBatchArm string

const (
	diagnosticBatchRawFull      diagnosticBatchArm = "raw_full_batch"
	diagnosticBatchRawCompactor diagnosticBatchArm = "raw_compactor_only"
	diagnosticBatchPrecompacted diagnosticBatchArm = "precompacted_continuation"
)

func prepareDiagnosticBatchArm(tb testing.TB, f *diagnosticBatchFixture, arm diagnosticBatchArm) (*Service, Pool) {
	tb.Helper()
	pool := NewPool()
	service, err := NewService(context.Background(), &Config{Pool: pool})
	if err != nil {
		tb.Fatalf("create attestation service: %v", err)
	}
	if err := pool.SaveAggregatedAttestations(cloneDiagnosticBatchAtts(f.retainedGloas)); err != nil {
		tb.Fatalf("save retained Gloas aggregates: %v", err)
	}
	switch arm {
	case diagnosticBatchRawFull, diagnosticBatchRawCompactor:
		if err := pool.SaveUnaggregatedAttestations(cloneDiagnosticBatchAtts(f.singles)); err != nil {
			tb.Fatalf("save raw Electra singles: %v", err)
		}
	case diagnosticBatchPrecompacted:
		if err := pool.SaveAggregatedAttestations(cloneDiagnosticBatchAtts(f.compactElectra)); err != nil {
			tb.Fatalf("save precompacted Electra aggregates: %v", err)
		}
	default:
		tb.Fatalf("unknown diagnostic arm %q", arm)
	}
	return service, pool
}

func runDiagnosticBatchArm(ctx context.Context, service *Service, pool Pool, arm diagnosticBatchArm) error {
	switch arm {
	case diagnosticBatchRawFull, diagnosticBatchPrecompacted:
		return service.batchForkChoiceAtts(ctx)
	case diagnosticBatchRawCompactor:
		return pool.AggregateUnaggregatedAttestations(ctx)
	default:
		return fmt.Errorf("unknown diagnostic arm %q", arm)
	}
}

func validateDiagnosticBatchArm(tb testing.TB, f *diagnosticBatchFixture, pool Pool, arm diagnosticBatchArm) {
	tb.Helper()
	if got := pool.UnaggregatedAttestationCount(); got != 0 {
		tb.Fatalf("%s raw backing count=%d, want 0", arm, got)
	}
	if got := pool.AggregatedAttestationCount(); got != len(f.wantParticipants) {
		tb.Fatalf("%s aggregate count=%d, want %d", arm, got, len(f.wantParticipants))
	}

	var outputs []ethpb.Att
	if arm == diagnosticBatchRawCompactor {
		outputs = pool.AggregatedAttestations()
	} else {
		outputs = pool.ForkchoiceAttestations()
	}
	if got := len(outputs); got != len(f.wantParticipants) {
		tb.Fatalf("%s output count=%d, want %d", arm, got, len(f.wantParticipants))
	}
	seen := make(map[attestation.Id]bool, len(outputs))
	for _, output := range outputs {
		id, err := attestation.NewId(output, attestation.Data)
		if err != nil {
			tb.Fatalf("%s output attestation ID: %v", arm, err)
		}
		want, ok := f.wantParticipants[id]
		if !ok {
			tb.Fatalf("%s unexpected output ID/version %d", arm, output.Version())
		}
		if seen[id] {
			tb.Fatalf("%s duplicate output ID/version %d", arm, output.Version())
		}
		seen[id] = true
		if got := output.GetAggregationBits().Count(); got != want {
			tb.Fatalf("%s output coverage=%d, want %d", arm, got, want)
		}
		if _, err := bls.SignatureFromBytes(output.GetSignature()); err != nil {
			tb.Fatalf("%s output has malformed aggregate signature: %v", arm, err)
		}
	}
}

func TestDiagnosticHistoricalBatchForkChoiceAtts(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{})
	t.Cleanup(reset)
	f := newDiagnosticBatchFixture(t)
	for _, arm := range []diagnosticBatchArm{diagnosticBatchRawFull, diagnosticBatchRawCompactor, diagnosticBatchPrecompacted} {
		t.Run(string(arm), func(t *testing.T) {
			service, pool := prepareDiagnosticBatchArm(t, f, arm)
			requireNoDiagnosticBatchError(t, runDiagnosticBatchArm(t.Context(), service, pool, arm))
			validateDiagnosticBatchArm(t, f, pool, arm)
		})
	}
}

func BenchmarkHistoricalBatchForkChoiceAtts(b *testing.B) {
	reset := features.InitWithReset(&features.Flags{})
	b.Cleanup(reset)
	f := newDiagnosticBatchFixture(b)
	for _, arm := range []diagnosticBatchArm{diagnosticBatchRawFull, diagnosticBatchRawCompactor, diagnosticBatchPrecompacted} {
		b.Run(string(arm), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(13000, "raw_singles/input")
			b.ReportMetric(float64(len(f.retainedGloas)), "retained_gloas/input")
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				service, pool := prepareDiagnosticBatchArm(b, f, arm)
				b.StartTimer()
				err := runDiagnosticBatchArm(b.Context(), service, pool, arm)
				b.StopTimer()
				requireNoDiagnosticBatchError(b, err)
				validateDiagnosticBatchArm(b, f, pool, arm)
				b.StartTimer()
			}
		})
	}
}

func requireNoDiagnosticBatchError(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}

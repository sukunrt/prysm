package kv

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func retentionVote(t *testing.T, slot primitives.Slot, bit uint64) ethpb.Att {
	t.Helper()
	key, err := bls.RandKey()
	require.NoError(t, err)
	bits := bitfield.NewBitlist(8)
	bits.SetBitAt(bit, true)
	return util.HydrateAttestation(&ethpb.Attestation{
		Data: &ethpb.AttestationData{Slot: slot}, AggregationBits: bits,
		Signature: key.Sign([]byte{byte(slot), byte(bit)}).Marshal(),
	})
}

func TestPruneBeforeRetentionAndIndependentForkchoice(t *testing.T) {
	p := NewAttCaches()
	pending := retentionVote(t, 0, 3)
	require.NoError(t, p.SaveForkchoiceAttestations([]ethpb.Att{pending}))
	for slot := primitives.Slot(0); slot <= 4; slot++ {
		require.NoError(t, p.SaveUnaggregatedAttestation(retentionVote(t, slot, 0)))
		require.NoError(t, p.SaveUnaggregatedAttestation(retentionVote(t, slot, 1)))
	}
	require.Equal(t, 0, len(p.UnaggregatedAttestations()))
	require.Equal(t, 5, len(p.AggregatedAttestations()))
	removed, err := p.PruneBefore(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), removed.Aggregated)
	require.Equal(t, 1, p.ForkchoiceAttestationCount())
	require.Equal(t, 4, len(p.AggregatedAttestations()))
	require.Equal(t, 4, len(p.runningSig))
	_, err = p.PruneBefore(2)
	require.NoError(t, err)
	_, err = p.PruneBefore(1)
	require.NoError(t, err)
	require.NoError(t, p.SaveUnaggregatedAttestation(retentionVote(t, 0, 2)))
	require.NoError(t, p.SaveAggregatedAttestation(aggregateRetentionVote(t, 0)))
	require.Equal(t, 3, len(p.AggregatedAttestations()))
	require.Equal(t, 1, p.ForkchoiceAttestationCount())
	for _, a := range p.ForkchoiceAttestations() {
		require.NoError(t, p.DeleteForkchoiceAttestation(a))
	}
	require.Equal(t, 0, p.ForkchoiceAttestationCount())
}

func aggregateRetentionVote(t *testing.T, slot primitives.Slot) ethpb.Att {
	t.Helper()
	a := retentionVote(t, slot, 0)
	a.GetAggregationBits().SetBitAt(1, true)
	return a
}

func TestPruneBeforeConcurrentWrites(t *testing.T) {
	p := NewAttCaches()
	first := retentionVote(t, 7, 0)
	second := retentionVote(t, 7, 1)
	aggregate := aggregateRetentionVote(t, 7)
	require.NoError(t, p.SaveUnaggregatedAttestation(first))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.SaveUnaggregatedAttestation(second)
			_ = p.SaveAggregatedAttestation(aggregate)
		}()
	}
	_, err := p.PruneBefore(8)
	require.NoError(t, err)
	wg.Wait()
	require.Equal(t, 0, len(p.AggregatedAttestations()))
	require.Equal(t, 0, len(p.UnaggregatedAttestations()))
	require.Equal(t, 0, len(p.runningSig))
	require.Equal(t, 0, len(p.singleByData))
	require.Equal(t, 0, len(p.aggregatedCoverage))
	require.Equal(t, 0, len(p.blockCoverage))
}

func TestPruneBeforePausedSinglePromotion(t *testing.T) {
	p := NewAttCaches()
	first := retentionVote(t, 1, 0)
	second := retentionVote(t, 1, 1)
	require.NoError(t, p.SaveUnaggregatedAttestation(first))
	entered := make(chan struct{})
	resume := make(chan struct{})
	p.beforeSingleAggregate = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { done <- p.SaveUnaggregatedAttestation(second) }()
	<-entered
	_, err := p.PruneBefore(2)
	require.NoError(t, err)
	close(resume)
	require.NoError(t, <-done)
	require.Equal(t, 0, len(p.AggregatedAttestations()))
	require.Equal(t, 0, len(p.UnaggregatedAttestations()))
	require.Equal(t, 0, len(p.runningSig))
	require.Equal(t, 0, p.ForkchoiceAttestationCount())
}

func TestPruneBeforePausedPeerCommit(t *testing.T) {
	p := NewAttCaches()
	a := aggregateRetentionVote(t, 1)
	entered := make(chan struct{})
	resume := make(chan struct{})
	p.beforePeerCommit = func() { close(entered); <-resume }
	done := make(chan error, 1)
	go func() { done <- p.SaveAggregatedAttestation(a) }()
	<-entered
	_, err := p.PruneBefore(2)
	require.NoError(t, err)
	close(resume)
	require.NoError(t, <-done)
	require.Equal(t, 0, len(p.AggregatedAttestations()))
	require.Equal(t, 0, p.ForkchoiceAttestationCount())
}

func BenchmarkPruneBeforeMixedAge(b *testing.B) {
	key, err := bls.RandKey()
	if err != nil {
		b.Fatal(err)
	}
	var votes []ethpb.Att
	for slot := primitives.Slot(8); slot <= 13; slot++ {
		for committee := uint64(0); committee < 10; committee++ {
			bits := bitfield.NewBitlist(8)
			bits.SetBitAt(0, true)
			bits.SetBitAt(1, true)
			votes = append(votes, util.HydrateAttestation(&ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: slot, CommitteeIndex: primitives.CommitteeIndex(committee)},
				AggregationBits: bits, Signature: key.Sign([]byte{byte(slot), byte(committee)}).Marshal(),
			}))
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		p := NewAttCaches()
		if err := p.SaveAggregatedAttestations(votes); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		removed, err := p.PruneBefore(11)
		if err != nil || removed.Aggregated != 30 {
			b.Fatalf("removed=%+v err=%v", removed, err)
		}
	}
	b.ReportMetric(60, "before-candidates/op")
	b.ReportMetric(30, "after-candidates/op")
	b.ReportMetric(0, "forkchoice-candidates/op")
}

// BenchmarkPruneSteadySlots measures nonempty retirement over advancing slots.
func BenchmarkPruneSteadySlots(b *testing.B) {
	key, err := bls.RandKey()
	if err != nil {
		b.Fatal(err)
	}
	makeSlot := func(slot primitives.Slot) []ethpb.Att {
		atts := make([]ethpb.Att, 10)
		for committee := range atts {
			bits := bitfield.NewBitlist(8)
			bits.SetBitAt(0, true)
			bits.SetBitAt(1, true)
			atts[committee] = util.HydrateAttestation(&ethpb.Attestation{
				Data:            &ethpb.AttestationData{Slot: slot, CommitteeIndex: primitives.CommitteeIndex(committee)},
				AggregationBits: bits, Signature: key.Sign([]byte{byte(slot), byte(slot >> 8), byte(committee)}).Marshal(),
			})
		}
		return atts
	}
	p := NewAttCaches()
	for slot := primitives.Slot(0); slot < 4; slot++ {
		if err := p.SaveAggregatedAttestations(makeSlot(slot)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		slot := primitives.Slot(i + 4)
		b.StopTimer()
		if err := p.SaveAggregatedAttestations(makeSlot(slot)); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		counts, err := p.PruneBefore(slot - 3)
		if err != nil || counts.Aggregated != 10 {
			b.Fatalf("pruned=%+v err=%v", counts, err)
		}
	}
	b.StopTimer()
	if got := len(p.AggregatedAttestations()); got != 40 {
		b.Fatalf("retained=%d", got)
	}
	if got := p.ForkchoiceAttestationCount(); got != 0 {
		b.Fatalf("pending=%d", got)
	}
	b.ReportMetric(40, "retained-candidates/op")
	b.ReportMetric(10, "retired-candidates/op")
}

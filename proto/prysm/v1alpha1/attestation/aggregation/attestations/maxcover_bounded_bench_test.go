package attestations_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
)

func BenchmarkMaxCoverBoundedAggregates(b *testing.B) {
	for _, tt := range []struct {
		name    string
		count   int
		overlap bool
	}{
		{name: "disjoint/3", count: 3},
		{name: "disjoint/10", count: 10},
		{name: "disjoint/64", count: 64},
		{name: "overlapping/3", count: 3, overlap: true},
		{name: "overlapping/10", count: 10, overlap: true},
		{name: "overlapping/64", count: 64, overlap: true},
	} {
		b.Run(tt.name, func(b *testing.B) {
			message := [32]byte{1, 2, 3}
			bitCount := tt.count * 4
			keys := make([]common.SecretKey, bitCount)
			for i := range keys {
				var err error
				keys[i], err = bls.RandKey()
				if err != nil {
					b.Fatal(err)
				}
			}
			atts := make([]ethpb.Att, tt.count)
			wantCoverage := bitfield.NewBitlist(uint64(bitCount))
			for i := range atts {
				bits := bitfield.NewBitlist(uint64(bitCount))
				signs := make([]common.Signature, 0, 4)
				indices := []int{4 * i, 4*i + 1, 4*i + 2, 4*i + 3}
				if tt.overlap {
					indices = []int{2 * (i / 2), 2*(i/2) + 1, 2*tt.count + 2*i, 2*tt.count + 2*i + 1}
				}
				for _, idx := range indices {
					bits.SetBitAt(uint64(idx), true)
					signs = append(signs, keys[idx].Sign(message[:]))
				}
				var err error
				wantCoverage, err = wantCoverage.Or(bits)
				if err != nil {
					b.Fatal(err)
				}
				atts[i] = &ethpb.Attestation{
					AggregationBits: bits,
					Data:            &ethpb.AttestationData{},
					Signature:       bls.AggregateSignatures(signs).Marshal(),
				}
			}

			checkInputs := append([]ethpb.Att(nil), atts...)
			checked, err := attestations.MaxCoverAttestationAggregation(checkInputs)
			if err != nil {
				b.Fatal(err)
			}
			covered := bitfield.NewBitlist(uint64(bitCount))
			var largest uint64
			for _, att := range checked {
				bits := att.GetAggregationBits()
				covered, err = covered.Or(bits)
				if err != nil {
					b.Fatal(err)
				}
				if bits.Count() > largest {
					largest = bits.Count()
				}
				pubkeys := make([]common.PublicKey, 0, bits.Count())
				for _, idx := range bits.BitIndices() {
					pubkeys = append(pubkeys, keys[idx].PublicKey())
				}
				sig, err := bls.SignatureFromBytes(att.GetSignature())
				if err != nil {
					b.Fatal(err)
				}
				if !sig.FastAggregateVerify(pubkeys, message) {
					b.Fatal("invalid aggregate signature")
				}
			}
			if !bytes.Equal(covered.Bytes(), wantCoverage.Bytes()) {
				b.Fatal("aggregation lost participant coverage")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				inputs := append([]ethpb.Att(nil), atts...)
				got, err := attestations.MaxCoverAttestationAggregation(inputs)
				if err != nil {
					b.Fatal(err)
				}
				if len(got) == 0 {
					b.Fatal("aggregation returned no attestations")
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(len(checked)), "candidates")
			b.ReportMetric(float64(largest), "maxparticipants")
			b.ReportMetric(float64(covered.Count()), "coveredbits")
		})
	}
}

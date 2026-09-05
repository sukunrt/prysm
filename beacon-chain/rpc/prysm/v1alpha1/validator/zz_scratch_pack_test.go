package validator

import (
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
)

func scratchSingles(n int, sig []byte) []ethpb.Att {
	atts := make([]ethpb.Att, 0, n)
	for i := 0; i < n; i++ {
		bits := bitfield.NewBitlist(uint64(n))
		bits.SetBitAt(uint64(i), true)
		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(0, true)
		atts = append(atts, &ethpb.AttestationElectra{
			AggregationBits: bits, CommitteeBits: cb,
			Data: &ethpb.AttestationData{Slot: 1, BeaconBlockRoot: make([]byte, 32),
				Source: &ethpb.Checkpoint{Root: make([]byte, 32)}, Target: &ethpb.Checkpoint{Root: make([]byte, 32)}},
			Signature: sig,
		})
	}
	return atts
}

func TestScratchPackCost(t *testing.T) {
	key, err := bls.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	sig := key.Sign([]byte("x")).Marshal()
	for _, n := range []int{3000, 10000} {
		atts := scratchSingles(n, sig)
		st := time.Now()
		d, err := proposerAtts(atts).dedup()
		t.Logf("dedup n=%d -> %d in %v err=%v", n, len(d), time.Since(st), err)

		atts = scratchSingles(n, sig)
		st = time.Now()
		a, err := attaggregation.Aggregate(atts)
		t.Logf("Aggregate(maxcover) n=%d -> %d in %v err=%v", n, len(a), time.Since(st), err)

		atts = scratchSingles(n, sig)
		st = time.Now()
		one, err := attaggregation.AggregateDisjointOneBitAtts(atts)
		t.Logf("AggregateDisjointOneBitAtts n=%d -> ok=%v in %v err=%v", n, one != nil, time.Since(st), err)
	}
}

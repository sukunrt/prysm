package p2p

import (
	"bytes"
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sukunrt/go-waggle/waggle"
)

const benchVotes = 500

func benchTopic(b *testing.B, cfg WaggleAttConfig, validate func(valJob) bool) *waggleAttTopic {
	w := waggle.NewWaggle()
	tp := newWaggleAttTopic("bench", cfg, genesisAt(testWaggleSlot, 0), validate)
	wt, err := w.NewTopic("bench", 0, tp.onSendComplete)
	require.NoError(b, err)
	tp.topic = wt
	b.Cleanup(func() {
		wt.Close()
		w.Close()
	})
	return tp
}

func benchPeers(n int) []peer.ID {
	ps := make([]peer.ID, n)
	for i := range ps {
		ps[i] = peer.ID(fmt.Sprintf("peer-%d", i))
	}
	return ps
}

// benchSlot stores benchVotes votes in two data roots.
func benchSlot(s *slotAttestations) {
	for v := range primitives.ValidatorIndex(benchVotes) {
		att := testSingleAttestation(v)
		root := [32]byte{byte(v % 2)}
		att.Data.BeaconBlockRoot = bytes.Repeat([]byte{byte(v % 2)}, fieldparams.RootLength)
		s.store(root, att)
	}
}

func BenchmarkWaggleHandleBundle(b *testing.B) {
	var jobs []valJob
	tp := benchTopic(b, DefaultWaggleAttConfig(), func(j valJob) bool {
		jobs = append(jobs, j)
		return true
	})
	att := testSingleAttestation(0)
	bundle := &ethpb.AttestationBundle{AttestationData: att.Data}
	for range maxAttestationsPerBundle {
		bundle.AttesterIndices = append(bundle.AttesterIndices, 0)
		bundle.Signatures = append(bundle.Signatures, att.Signature)
	}
	root := [32]byte{1}
	from := peer.ID("from")
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		for k := range bundle.AttesterIndices {
			bundle.AttesterIndices[k] = uint64(i*maxAttestationsPerBundle + k)
		}
		jobs = jobs[:0]
		tp.handleBundle(b.Context(), from, testWaggleSlot, bundle, root)
		for _, j := range jobs {
			tp.handleValResult(valResult{dataRoot: j.dataRoot, att: j.att, res: pubsub.ValidationAccept})
		}
	}
}

func BenchmarkWagglePushTo(b *testing.B) {
	cfg := DefaultWaggleAttConfig()
	cfg.PushPeers = 1 << 30
	tp := benchTopic(b, cfg, nil)
	benchSlot(tp.slot(testWaggleSlot))
	peers := benchPeers(b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		tp.pushTo(peers[i], 1000)
	}
}

func BenchmarkWaggleAvailableLists(b *testing.B) {
	tp := benchTopic(b, DefaultWaggleAttConfig(), nil)
	s := tp.slot(testWaggleSlot)
	benchSlot(s)
	b.ReportAllocs()
	b.ResetTimer()
	p := peer.ID("p")
	for range b.N {
		tp.lists(tp.availableVotes(s, nil), func(root [32]byte, ids []uint64) bool {
			tp.sendList(p, testWaggleSlot, s, root, ids, nil)
			return true
		})
	}
}

func BenchmarkWaggleDecode(b *testing.B) {
	att := testSingleAttestation(0)
	bundle := &ethpb.AttestationBundle{AttestationData: att.Data}
	for k := range maxAttestationsPerBundle {
		bundle.AttesterIndices = append(bundle.AttesterIndices, uint64(k))
		bundle.Signatures = append(bundle.Signatures, att.Signature)
	}
	msg, err := waggleMessage{Slot: testWaggleSlot, Bundle: bundle}.encode()
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := decodeWaggleMessage(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWaggleHandleMetadata(b *testing.B) {
	tp := benchTopic(b, DefaultWaggleAttConfig(), nil)
	tp.slot(testWaggleSlot).store([32]byte{}, testSingleAttestation(0))
	available := make([]uint64, benchVotes)
	md := &ethpb.CommitteeAttestationPartsMetadata{DataRoot: make([]byte, fieldparams.RootLength), Available: available}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		for k := range available {
			available[k] = uint64(i*benchVotes + k)
		}
		tp.handleMetadata("from", testWaggleSlot, md)
	}
}

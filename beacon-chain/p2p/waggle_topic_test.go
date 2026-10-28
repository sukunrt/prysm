package p2p

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	wagglelibp2p "github.com/sukunrt/go-waggle/libp2p"
	"github.com/sukunrt/go-waggle/waggle"
)

const testWaggleSlot = primitives.Slot(100)

var testWaggleDigest = [fieldparams.VersionLength]byte{1, 2, 3, 4}

type validateRecorder struct {
	mu     sync.Mutex
	calls  map[primitives.ValidatorIndex]int
	reject bool
	got    chan primitives.ValidatorIndex
}

func newValidateRecorder(reject bool) *validateRecorder {
	return &validateRecorder{calls: make(map[primitives.ValidatorIndex]int), reject: reject, got: make(chan primitives.ValidatorIndex, 64)}
}

func (r *validateRecorder) validate(_ context.Context, _ peer.ID, _ string, att *ethpb.SingleAttestation) bool {
	r.mu.Lock()
	r.calls[att.AttesterIndex]++
	r.mu.Unlock()
	r.got <- att.AttesterIndex
	return !r.reject
}

func (r *validateRecorder) count(v primitives.ValidatorIndex) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[v]
}

type waggleTestNode struct {
	h   host.Host
	att *waggleAttestations
	rec *validateRecorder
}

func newWaggleTestNode(t *testing.T, cfg WaggleAttConfig, reject bool) *waggleTestNode {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	w := waggle.NewWaggle()
	stop, err := wagglelibp2p.ConfigureHost(w, h)
	require.NoError(t, err)
	a := newWaggleAttestations(t.Context(), w, "/ssz_snappy", cfg, genesisAt(testWaggleSlot, params.BeaconConfig().SlotDuration()/2))
	n := &waggleTestNode{h: h, att: a, rec: newValidateRecorder(reject)}
	a.Start(n.rec.validate)
	a.EnsureDigest(testWaggleDigest)
	a.Subscribe(testWaggleTopic())
	t.Cleanup(func() {
		stop()
		a.Close()
		w.Close()
		require.NoError(t, h.Close())
	})
	return n
}

// genesisAt returns a genesis time such that the clock is at offset into slot.
func genesisAt(slot primitives.Slot, offset time.Duration) func() time.Time {
	g := time.Now().Add(-time.Duration(slot)*params.BeaconConfig().SlotDuration() - offset)
	return func() time.Time { return g }
}

func testWaggleTopic() string {
	return attestationToTopic(0, testWaggleDigest) + "/ssz_snappy"
}

func connectWaggleNodes(t *testing.T, a, b *waggleTestNode) {
	require.NoError(t, a.h.Connect(t.Context(), peer.AddrInfo{ID: b.h.ID(), Addrs: b.h.Addrs()}))
	for _, n := range []*waggleTestNode{a, b} {
		topic := n.att.topic(testWaggleTopic()).topic
		require.Eventually(t, func() bool { return len(topic.PeersWithCapacity()) > 0 }, 5*time.Second, 10*time.Millisecond)
	}
}

func testSingleAttestation(v primitives.ValidatorIndex) *ethpb.SingleAttestation {
	return &ethpb.SingleAttestation{
		AttesterIndex: v,
		Data: &ethpb.AttestationData{
			Slot:            testWaggleSlot,
			BeaconBlockRoot: bytes.Repeat([]byte{1}, fieldparams.RootLength),
			Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
			Target:          &ethpb.Checkpoint{Epoch: 3, Root: make([]byte, fieldparams.RootLength)},
		},
		Signature: bytes.Repeat([]byte{byte(v)}, fieldparams.BLSSignatureLength),
	}
}

func waitValidated(t *testing.T, n *waggleTestNode, want ...primitives.ValidatorIndex) {
	pending := make(map[primitives.ValidatorIndex]bool)
	for _, v := range want {
		pending[v] = true
	}
	timeout := time.After(5 * time.Second)
	for len(pending) > 0 {
		select {
		case v := <-n.rec.got:
			delete(pending, v)
		case <-timeout:
			t.Fatalf("attestations not validated: %v", pending)
		}
	}
}

func TestWaggleAttTopic(t *testing.T) {
	t.Run("push forwards valid attestations", func(t *testing.T) {
		cfg := DefaultWaggleAttConfig()
		cfg.MetadataPeers = 0
		a, b, c := newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, false)
		connectWaggleNodes(t, a, b)
		connectWaggleNodes(t, b, c)

		a.att.Submit(testWaggleTopic(), testSingleAttestation(1))
		a.att.Submit(testWaggleTopic(), testSingleAttestation(2))
		waitValidated(t, b, 1, 2)
		waitValidated(t, c, 1, 2)
		require.Equal(t, 0, a.rec.count(1))
	})

	t.Run("request and serve without push", func(t *testing.T) {
		cfg := DefaultWaggleAttConfig()
		cfg.PushPeers = 0
		a, b := newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, false)
		connectWaggleNodes(t, a, b)

		a.att.Submit(testWaggleTopic(), testSingleAttestation(3))
		waitValidated(t, b, 3)
	})

	t.Run("validates each attestation once", func(t *testing.T) {
		cfg := DefaultWaggleAttConfig()
		a, b, c := newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, false)
		connectWaggleNodes(t, a, c)
		connectWaggleNodes(t, b, c)

		a.att.Submit(testWaggleTopic(), testSingleAttestation(4))
		b.att.Submit(testWaggleTopic(), testSingleAttestation(4))
		waitValidated(t, c, 4)
		time.Sleep(300 * time.Millisecond)
		require.Equal(t, 1, c.rec.count(4))
	})

	t.Run("does not forward invalid attestations", func(t *testing.T) {
		cfg := DefaultWaggleAttConfig()
		a, b, c := newWaggleTestNode(t, cfg, false), newWaggleTestNode(t, cfg, true), newWaggleTestNode(t, cfg, false)
		connectWaggleNodes(t, a, b)
		connectWaggleNodes(t, b, c)

		a.att.Submit(testWaggleTopic(), testSingleAttestation(5))
		waitValidated(t, b, 5)
		time.Sleep(300 * time.Millisecond)
		require.Equal(t, 0, c.rec.count(5))
	})
}

func TestWaggleAttTopicExpiry(t *testing.T) {
	cfg := DefaultWaggleAttConfig()
	disparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()

	t.Run("window", func(t *testing.T) {
		late := newWaggleAttTopic("t", cfg, genesisAt(testWaggleSlot, 2*disparity), nil)
		require.Equal(t, testWaggleSlot, late.oldestLive())
		require.Equal(t, false, late.inWindow(testWaggleSlot-1))
		require.Equal(t, true, late.inWindow(testWaggleSlot))
		require.Equal(t, true, late.inWindow(testWaggleSlot+1))
		require.Equal(t, false, late.inWindow(testWaggleSlot+2))

		early := newWaggleAttTopic("t", cfg, genesisAt(testWaggleSlot, disparity/2), nil)
		require.Equal(t, true, early.inWindow(testWaggleSlot-1))
		require.Equal(t, true, early.nextExpiry().After(time.Now()))
	})

	t.Run("prune drops the expired slots", func(t *testing.T) {
		tp := newWaggleAttTopic("t", cfg, genesisAt(testWaggleSlot, 2*disparity), nil)
		old, cur := testWaggleSlot-1, testWaggleSlot
		p1, p2 := peer.ID("p1"), peer.ID("p2")
		tp.slot(old).peer(p1).markKnown(1)
		tp.slot(cur).peer(p2).markKnown(1)
		tp.outbox[p1] = []waggleMessage{{Slot: old}}
		tp.outbox[p2] = []waggleMessage{{Slot: old}, {Slot: cur}}

		tp.prune()
		require.Equal(t, 1, len(tp.slots))
		require.Equal(t, true, tp.slots[cur].peerHas(p2, 1))
		require.Equal(t, 1, len(tp.outbox))
		require.Equal(t, 1, len(tp.outbox[p2]))
		require.Equal(t, cur, tp.outbox[p2][0].Slot)
	})
}

func TestAvailableDelta(t *testing.T) {
	tp := newWaggleAttTopic("t", DefaultWaggleAttConfig(), genesisAt(testWaggleSlot, 0), nil)
	add := func(s *waggleSlot, committee primitives.CommitteeIndex, v primitives.ValidatorIndex) {
		att := testSingleAttestation(v)
		att.CommitteeId = committee
		s.store([32]byte{byte(committee)}, att)
	}
	ids := func(msgs []waggleMessage) map[primitives.CommitteeIndex][]uint64 {
		got := make(map[primitives.CommitteeIndex][]uint64)
		for _, m := range msgs {
			got[m.Metadata.CommitteeIndex] = append(got[m.Metadata.CommitteeIndex], m.Metadata.Available...)
		}
		return got
	}

	t.Run("each validator once", func(t *testing.T) {
		s := tp.slot(testWaggleSlot)
		ps := &slotPeer{known: map[primitives.ValidatorIndex]struct{}{2: {}}}
		add(s, 0, 1)
		add(s, 0, 2)
		add(s, 1, 3)
		require.DeepEqual(t, map[primitives.CommitteeIndex][]uint64{0: {1}, 1: {3}}, ids(availableDelta(testWaggleSlot, s, ps)))

		ps.told = len(s.order)
		require.Equal(t, 0, len(availableDelta(testWaggleSlot, s, ps)))
		add(s, 0, 4)
		require.DeepEqual(t, map[primitives.CommitteeIndex][]uint64{0: {4}}, ids(availableDelta(testWaggleSlot, s, ps)))
	})

	t.Run("splits long lists", func(t *testing.T) {
		s := tp.slot(testWaggleSlot + 1)
		for v := range primitives.ValidatorIndex(maxIndicesPerMetadata + 1) {
			add(s, 0, v)
		}
		msgs := availableDelta(testWaggleSlot+1, s, &slotPeer{known: map[primitives.ValidatorIndex]struct{}{}})
		require.Equal(t, 2, len(msgs))
		require.Equal(t, maxIndicesPerMetadata, len(msgs[0].Metadata.Available))
		require.Equal(t, 1, len(msgs[1].Metadata.Available))
	})
}

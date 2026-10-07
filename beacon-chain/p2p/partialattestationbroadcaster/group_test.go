package partialattestationbroadcaster

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
)

// storeHarness drives handleIncoming directly with counting callbacks and a
// controllable clock.
type storeHarness struct {
	b          *Broadcaster
	current    *primitives.Slot
	deadline   time.Time       // the push deadline every slot group gets
	actions    []string        // "bundle" or "meta", in publish order
	processed  []uint64        // index passed to the process callback, per call
	acceptOnly map[uint64]bool // when non-nil, only these indices are accepted
	process    ProcessAttestationFn
}

func newStoreHarness(t *testing.T) *storeHarness {
	current := testSlot
	h := &storeHarness{current: &current, deadline: time.Now().Add(time.Hour)}
	h.process = func(_ string, att *ethpb.SingleAttestation, _ time.Time) (bool, error) {
		idx := uint64(att.AttesterIndex)
		h.processed = append(h.processed, idx)
		if att.Signature[0] == 0xEE { // garbage-signature marker
			return false, nil
		}
		if h.acceptOnly != nil && !h.acceptOnly[idx] {
			return false, nil
		}
		return true, nil
	}
	h.b = NewBroadcaster(t.Context(), func() primitives.Slot { return *h.current },
		func(primitives.Slot) time.Time { return h.deadline }, DefaultPushInterval)
	// Two heartbeats to signature expiry keeps the lifecycle tests short.
	h.b.sigTTL = 2
	return h
}

// idxSig derives a per-validator signature so distinct validators get
// distinct tuple identities across the tests.
func idxSig(idx uint64) []byte {
	return bytes.Repeat([]byte{byte(idx)}, 96)
}

// deliverSigsNoPump dispatches a bundle with explicit signatures, leaving
// its validation job queued.
func (h *storeHarness) deliverSigsNoPump(t *testing.T, slot primitives.Slot, indices []uint64, sigs [][]byte) {
	t.Helper()
	data := testAttData(slot)
	dataRoot, err := data.HashTreeRoot()
	require.NoError(t, err)
	h.b.handleIncoming(incomingRPC{
		From:     "peer",
		Topic:    testTopic,
		Slot:     slot,
		DataRoot: dataRoot,
		Bundle: &ethpb.AttestationBundle{
			CommitteeIndex:  3,
			AttestationData: data,
			AttesterIndices: indices,
			Signatures:      sigs,
		},
	})
}

// deliver dispatches a bundle with index-derived signatures and runs
// validation synchronously.
func (h *storeHarness) deliver(t *testing.T, slot primitives.Slot, indices ...uint64) {
	t.Helper()
	sigs := make([][]byte, len(indices))
	for i := range sigs {
		sigs[i] = idxSig(indices[i])
	}
	h.deliverSigsNoPump(t, slot, indices, sigs)
	h.pump(t)
}

// pump runs queued validation jobs and their completions synchronously.
func (h *storeHarness) pump(t *testing.T) {
	t.Helper()
	for {
		select {
		case j := <-h.b.valJobs:
			h.b.handleValDone(h.b.runValJob(h.process, j))
		default:
			return
		}
	}
}

// submit runs Submit and its Start-loop work synchronously.
func (h *storeHarness) submit(t *testing.T, slot primitives.Slot, attesterIndex primitives.ValidatorIndex, sig []byte) {
	t.Helper()
	att := &ethpb.SingleAttestation{
		CommitteeId:   3,
		AttesterIndex: attesterIndex,
		Data:          testAttData(slot),
		Signature:     sig,
	}
	h.b.Submit(testTopic, att)
	for {
		select {
		case s := <-h.b.submit:
			h.b.handleSubmission(s)
		default:
			h.pump(t)
			return
		}
	}
}

func (h *storeHarness) slotGroup(t *testing.T, slot primitives.Slot) *slotAtts {
	t.Helper()
	g := h.b.groups[testTopic][slot]
	require.NotNil(t, g)
	return g
}

func TestStoreDedupsValidatedIndices(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// An identical replay never reaches validation.
	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// An overlapping bundle only validates the new validator.
	h.deliver(t, testSlot, 107, 109)
	require.DeepEqual(t, []uint64{101, 107, 109}, h.processed)
}

func TestRejectedTupleNotRetriedWithinTTL(t *testing.T) {
	h := newStoreHarness(t)
	h.acceptOnly = map[uint64]bool{101: true}

	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// The rejected tuple is remembered: the same signature is not validated
	// again within the TTL.
	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// A different signature for the rejected validator is a different tuple
	// and validates immediately.
	h.acceptOnly = nil
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{bytes.Repeat([]byte{0xBB}, 96)})
	h.pump(t)
	require.DeepEqual(t, []uint64{101, 107, 107}, h.processed)
	require.Equal(t, true, hasIdx(h.slotGroup(t, testSlot).validated, 107))
}

func TestSeenCacheExpiresAfterTTL(t *testing.T) {
	h := newStoreHarness(t)
	h.acceptOnly = map[uint64]bool{} // reject everything

	h.deliver(t, testSlot, 101)
	require.Equal(t, 1, len(h.processed))
	require.Equal(t, 0, len(h.b.groups))

	// Still cached until the TTL runs out.
	for range seenTTLHeartbeats() - 1 {
		h.b.cleanup(*h.current)
	}
	h.deliver(t, testSlot, 101)
	require.Equal(t, 1, len(h.processed))

	// Expired: the tuple validates again.
	h.b.cleanup(*h.current)
	h.deliver(t, testSlot, 101)
	require.Equal(t, 2, len(h.processed))
}

// A racing garbage signature is a different tuple: both validate and the
// honest one lands in the store.
func TestGarbageSignatureDoesNotSuppressHonest(t *testing.T) {
	h := newStoreHarness(t)
	garbage := bytes.Repeat([]byte{0xEE}, 96)
	good := bytes.Repeat([]byte{0xBB}, 96)

	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{garbage})
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{good})
	// An exact duplicate of an in-flight tuple is dropped by the seen cache.
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{garbage})
	require.Equal(t, 2, len(h.b.valJobs))

	h.pump(t)
	require.Equal(t, 2, len(h.processed))

	g := h.slotGroup(t, testSlot)
	require.Equal(t, true, hasIdx(g.validated, 107))
	require.DeepEqual(t, good, g.validated[107].Signature)
}

func TestStoreLifecycle(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)
	g := h.slotGroup(t, testSlot)
	require.Equal(t, 2, len(g.validated))
	require.Equal(t, 1, len(g.attData))
	require.NotNil(t, g.validated[101].Signature)
	require.Equal(t, true, hasIdx(g.validated, 107))

	// Within the signature TTL nothing changes.
	h.b.cleanup(*h.current)
	require.Equal(t, 1, len(g.attData))

	// Past the TTL the signatures and their data are deleted together.
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.attData))
	require.Equal(t, 0, len(g.validated))

	// Replays of the expired tuples still never reach validation: the seen
	// cache outlives the signatures.
	before := len(h.processed)
	h.deliver(t, testSlot, 101, 107)
	require.Equal(t, before, len(h.processed))

	// A late reveal of a new validator validates and gets its own TTL.
	h.deliver(t, testSlot, 109)
	require.Equal(t, uint64(109), h.processed[len(h.processed)-1])
	require.Equal(t, 1, len(g.attData))
	require.NotNil(t, g.validated[109].Signature)
	require.Equal(t, true, hasIdx(g.validated, 109))

	// The late signature expires on its own schedule.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.attData))

	// Once the slot leaves the propagation window the group is deleted.
	twoEpochsLater := testSlot + 2*primitives.Slot(32)
	h.b.cleanup(twoEpochsLater)
	require.Equal(t, 0, len(h.b.groups))
}

func TestStore_HezeWindowEndsWithTheVoteSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	disparity := cfg.MaximumGossipClockDisparityDuration()

	h := newStoreHarness(t)
	h.deliver(t, testSlot, 1)
	h.slotGroup(t, testSlot)

	// Within the clock disparity past the slot end the slot stays.
	h.deadline = time.Now().Add(-disparity / 2)
	h.b.cleanup(*h.current)
	h.slotGroup(t, testSlot)
	require.Equal(t, true, h.b.slotInPropagationWindow(testSlot))

	// Past it, the group is dropped and the slot is out of the window, even
	// though the clock is still in the same slot.
	h.deadline = time.Now().Add(-2 * disparity)
	require.Equal(t, false, h.b.slotInPropagationWindow(testSlot))
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.b.groups))
	h.submit(t, testSlot, 2, idxSig(2))
	require.Equal(t, 0, len(h.b.groups))
}

// installPush installs a fake publishPartial capturing the bundles pushed to
// each peer, keyed by peer. peerStates and partial drive the per-peer diff.
func (h *storeHarness) installPush(t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState, partial map[peer.ID]bool) map[peer.ID][]*ethpb.AttestationBundle {
	pushed := map[peer.ID][]*ethpb.AttestationBundle{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		for pid, action := range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			require.NoError(t, action.Err)
			bundle := &ethpb.AttestationBundle{}
			require.NoError(t, bundle.UnmarshalSSZ(action.EncodedPartialMessage))
			pushed[pid] = append(pushed[pid], bundle)
		}
		return nil
	}
	return pushed
}

// installPushStopAfter is installPush with a hard stop: yield returns false for
// the n-th action, which the closure has already counted as sent.
func (h *storeHarness) installPushStopAfter(
	t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState,
	partial map[peer.ID]bool, n int,
) map[peer.ID][]*ethpb.AttestationBundle {
	pushed := map[peer.ID][]*ethpb.AttestationBundle{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		seen := 0
		for pid, action := range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			require.NoError(t, action.Err)
			bundle := &ethpb.AttestationBundle{}
			require.NoError(t, bundle.UnmarshalSSZ(action.EncodedPartialMessage))
			pushed[pid] = append(pushed[pid], bundle)
			seen++
			if seen >= n {
				break
			}
		}
		return nil
	}
	return pushed
}

// installFailingPush installs a publishPartial that fails without running the
// closure, as gossipsub does for an unknown topic or group.
func (h *storeHarness) installFailingPush() {
	h.b.publishPartial = func(
		string, []byte, partialmessages.PublishActionsFn[blocks.PartialMessagePeerState],
	) error {
		return errors.New("publish failed")
	}
}

// installBoth captures bundle and metadata actions from one publishPartial and
// records their order in h.actions.
func (h *storeHarness) installBoth(
	t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState, partial map[peer.ID]bool,
) (map[peer.ID][]*ethpb.AttestationBundle, map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata) {
	pushed := map[peer.ID][]*ethpb.AttestationBundle{}
	metas := map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		for pid, action := range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			require.NoError(t, action.Err)
			if len(action.EncodedPartialMessage) > 0 {
				bundle := &ethpb.AttestationBundle{}
				require.NoError(t, bundle.UnmarshalSSZ(action.EncodedPartialMessage))
				pushed[pid] = append(pushed[pid], bundle)
				h.actions = append(h.actions, "bundle")
				continue
			}
			meta := &ethpb.CommitteeAttestationPartsMetadata{}
			require.NoError(t, meta.UnmarshalSSZ(action.EncodedPartsMetadata))
			metas[pid] = meta
			h.actions = append(h.actions, "meta")
		}
		return nil
	}
	return pushed, metas
}

// kind reads a peer's tracked kind.
func kind(peerStates map[peer.ID]blocks.PartialMessagePeerState, pid peer.ID) blocks.PeerKind {
	return peerStates[pid].Att.Kind
}

// pushedIndices flattens the bundles pushed to a peer into a sorted index set.
func pushedIndices(bundles []*ethpb.AttestationBundle) []uint64 {
	var indices []uint64
	for _, b := range bundles {
		indices = append(indices, b.AttesterIndices...)
	}
	return indices
}

// installMetaPush installs a fake publishPartial capturing the parts metadata
// pushed to each peer.
func (h *storeHarness) installMetaPush(t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState) map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata {
	pushed := map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		for pid, action := range fn(peerStates, func(peer.ID) bool { return true }) {
			require.NoError(t, action.Err)
			require.Equal(t, 0, len(action.EncodedPartialMessage)) // metadata never delivers
			meta := &ethpb.CommitteeAttestationPartsMetadata{}
			require.NoError(t, meta.UnmarshalSSZ(action.EncodedPartsMetadata))
			pushed[pid] = meta
		}
		return nil
	}
	return pushed
}

// avail builds a peer availability set with the given indices.
func avail(indices ...uint64) map[uint64]struct{} {
	set := make(map[uint64]struct{}, len(indices))
	for _, idx := range indices {
		set[idx] = struct{}{}
	}
	return set
}

// peerWith builds a peer state claiming the given indices for committee 3.
func peerWith(indices ...uint64) blocks.PartialMessagePeerState {
	return blocks.PartialMessagePeerState{Att: blocks.PartialAttestationPeerState{
		Available: map[primitives.CommitteeIndex]map[uint64]struct{}{3: avail(indices...)},
	}}
}

// peerKind builds a peer state of the given kind claiming the given indices.
func peerKind(k blocks.PeerKind, indices ...uint64) blocks.PartialMessagePeerState {
	ps := peerWith(indices...)
	ps.Att.Kind = k
	return ps
}

// deliverMeta hands handleIncoming a parts-metadata RPC for the test slot.
func (h *storeHarness) deliverMeta(t *testing.T, from peer.ID, available, requests []uint64) {
	t.Helper()
	h.b.handleIncoming(incomingRPC{
		From:  from,
		Topic: testTopic,
		Slot:  testSlot,
		Meta: &ethpb.CommitteeAttestationPartsMetadata{
			CommitteeIndex: 3,
			Available:      available,
			Requests:       requests,
		},
	})
}

// A gossip emission advertises the slot's validated validators once to the
// given peers; nothing about them is tracked.
func TestEmitGossip(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{}
	pushed := h.installMetaPush(t, peerStates)
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1", "g2"}})

	require.Equal(t, 2, len(pushed))
	meta := pushed["g1"]
	require.NotNil(t, meta)
	require.Equal(t, primitives.CommitteeIndex(3), meta.CommitteeIndex)
	// Available is the recent validated snapshot; requests are empty.
	require.DeepEqual(t, []uint64{101, 107}, meta.Available)
	require.Equal(t, 0, len(meta.Requests))
	require.NotNil(t, pushed["g2"])
	// Gossip is oneshot: the gossip peers stay untracked.
	require.Equal(t, 0, len(peerStates))
}

// A vote validated before the advertise window is not advertised.
func TestEmitGossipSkipsOldVotes(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101, 107)
	g := h.slotGroup(t, testSlot)
	v := g.validated[101]
	v.Committed = time.Now().Add(-advertiseWindow - time.Second)
	g.validated[101] = v

	pushed := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{})
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.DeepEqual(t, []uint64{107}, pushed["g1"].Available)

	v = g.validated[107]
	v.Committed = time.Now().Add(-advertiseWindow - time.Second)
	g.validated[107] = v
	clear(pushed)
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.Equal(t, 0, len(pushed))
}

// A slot whose signatures all expired cannot serve and advertises nothing.
func TestEmitGossipSkipsExpired(t *testing.T) {
	h := newStoreHarness(t)
	pushed := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{})

	h.deliver(t, testSlot, 101)
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.Equal(t, 0, len(pushed))
}

// Committing pushes nothing; the flush tick pushes to the mesh peers lacking
// the attestations and folds them into their availability.
func TestPushIsTickDriven(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{
		"partialEmpty":   {},
		"partialCovered": peerWith(101, 107),
		"nonPartial":     {},
	}
	partial := map[peer.ID]bool{"partialEmpty": true, "partialCovered": true, "nonPartial": false}
	pushed := h.installPush(t, peerStates, partial)

	// Committing queues the validators for the mesh but pushes nothing.
	h.deliver(t, testSlot, 101, 107)
	g := h.slotGroup(t, testSlot)
	require.Equal(t, 0, len(pushed))
	require.DeepEqual(t, []primitives.ValidatorIndex{101, 107}, g.meshPending)

	// The push happens on the tick.
	h.b.flushDirty()
	require.Equal(t, 1, len(pushed))
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["partialEmpty"]))

	// Every unknown entry is a mesh peer after the first flush.
	require.Equal(t, blocks.PeerMesh, kind(peerStates, "partialEmpty"))
	require.Equal(t, blocks.PeerMesh, kind(peerStates, "nonPartial"))

	// The sent attestations fold into the receiving peer's availability.
	got := peerStates["partialEmpty"].Att.Available[3]
	require.NotNil(t, got)
	require.Equal(t, true, hasIdx(got, 101))
	require.Equal(t, true, hasIdx(got, 107))

	// The offered votes settle.
	require.Equal(t, 0, len(g.meshPending))
	require.Equal(t, true, hasIdx(g.sent, 101))

	// A flush with no new commits pushes nothing.
	clear(pushed)
	h.b.flushDirty()
	require.Equal(t, 0, len(pushed))
}

// A validator is offered at most once: late mesh joiners get nothing
// retroactively, only genuinely new attestations.
func TestPushSkipsAlreadyBroadcast(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"early": {}}
	partial := map[peer.ID]bool{"early": true, "late": true}
	pushed := h.installPush(t, peerStates, partial)

	// The early peer receives the initial attestations; sent now covers them.
	h.deliver(t, testSlot, 101, 107)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["early"]))

	// No new validations: nothing is published.
	peerStates["late"] = blocks.PartialMessagePeerState{}
	clear(pushed)
	invoked := false
	h.b.publishPartial = func(_ string, _ []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		invoked = true
		for range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			t.Fatal("a peer received a replayed push")
		}
		return nil
	}
	h.b.flushDirty()
	require.Equal(t, false, invoked)

	// Only the new attestation is offered, the late joiner included.
	pushed = h.installPush(t, peerStates, partial)
	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["early"]))
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["late"]))
}

// A push of one data group larger than maxAttsPerBundle splits into
// multiple bundles, none oversized, covering every attestation.
func TestPushSplitsOversizedBundles(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"peer": {}}
	partial := map[peer.ID]bool{"peer": true}
	pushed := h.installPush(t, peerStates, partial)

	indices := make([]uint64, 2*maxAttsPerBundle+1)
	for i := range indices {
		indices[i] = uint64(i)
	}
	h.deliver(t, testSlot, indices...)
	h.b.flushDirty()

	bundles := pushed["peer"]
	require.Equal(t, 3, len(bundles))
	for _, bundle := range bundles {
		require.Equal(t, true, len(bundle.AttesterIndices) <= maxAttsPerBundle,
			"bundle carries %d attestations", len(bundle.AttesterIndices))
	}
	require.DeepEqual(t, indices, pushedIndices(bundles))
}

// Signature expiry drops the data group but not sent: a late reveal gets its
// own push, old validators are never re-offered.
func TestPushAfterExpiryRevival(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"p": {}}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"p": true})

	h.deliver(t, testSlot, 101)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["p"]))

	// Two heartbeats expire the signature and delete the data group.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	// A late reveal of a new validator is pushed on its own.
	clear(pushed)
	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["p"]))
}

// A pending vote whose data group expired before the flush is skipped, but it
// still settles: sent keeps the replay filter.
func TestFlushDirtySkipsExpired(t *testing.T) {
	h := newStoreHarness(t)
	pushed := h.installPush(t, map[peer.ID]blocks.PartialMessagePeerState{"partialEmpty": {}}, map[peer.ID]bool{"partialEmpty": true})

	h.deliver(t, testSlot, 101)
	g := h.slotGroup(t, testSlot)
	require.DeepEqual(t, []primitives.ValidatorIndex{101}, g.meshPending)

	// Two heartbeats expire the signature and delete the data group,
	// leaving the pending entry stale.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.attData))

	h.b.flushDirty()
	require.Equal(t, 0, len(pushed))
	require.Equal(t, 0, len(g.meshPending))
	require.Equal(t, true, hasIdx(g.sent, 101))
}

// A submitted attestation commits inline: Submit is a pre-validated ingress
// and never re-enters validation.
func TestSubmitCommitsInline(t *testing.T) {
	h := newStoreHarness(t)

	// Deliver 101,107 to create the data group.
	h.deliver(t, testSlot, 101, 107)
	require.Equal(t, 2, len(h.processed))
	h.slotGroup(t, testSlot).meshPending = nil

	h.submit(t, testSlot, 109, idxSig(109))
	require.Equal(t, 2, len(h.processed)) // the process callback never ran for it
	g := h.slotGroup(t, testSlot)
	require.Equal(t, true, hasIdx(g.validated, 109))
	require.DeepEqual(t, idxSig(109), g.validated[109].Signature)
	require.DeepEqual(t, []primitives.ValidatorIndex{109}, g.meshPending)
}

// A validated attestation is dropped on Submit: the feedback-loop neutralizer.
func TestSubmitDropsValidatedIndex(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101)
	h.slotGroup(t, testSlot).meshPending = nil

	h.submit(t, testSlot, 101, idxSig(101)) // validator 101 already validated
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).meshPending))
	require.Equal(t, 1, len(h.processed))
}

// Submit with no existing group creates it inline; nothing touches the
// validation lane.
func TestSubmitCreatesGroupInline(t *testing.T) {
	h := newStoreHarness(t)

	h.submit(t, testSlot, 101, idxSig(101))
	require.Equal(t, 0, len(h.processed)) // pre-validated: no validation
	require.Equal(t, 0, len(h.b.valJobs))
	require.Equal(t, true, hasIdx(h.slotGroup(t, testSlot).validated, 101))
}

// Submit of an out-of-window slot is dropped before any channel enqueue.
func TestSubmitOutOfWindowDropped(t *testing.T) {
	h := newStoreHarness(t)

	att := &ethpb.SingleAttestation{
		CommitteeId:   3,
		AttesterIndex: 101,
		Data:          testAttData(testSlot + 100), // far in the future
		Signature:     idxSig(101),
	}
	h.b.Submit(testTopic, att)
	require.Equal(t, 0, len(h.b.submit))
}

// A request is answered immediately from live signatures, minus what the
// requester has; the fold makes a replayed request go quiet.
func TestServeRequestsImmediately(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101, 107)
	h.slotGroup(t, testSlot).meshPending = nil

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"req": {}}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"req": true})

	// A request for held and unheld validators is answered at once with the
	// held ones; a duplicated index is served once.
	h.deliverMeta(t, "req", nil, []uint64{101, 102, 107, 101})
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["req"]))
	got := peerStates["req"].Att.Available[3]
	require.NotNil(t, got)
	require.Equal(t, true, hasIdx(got, 101))
	require.Equal(t, true, hasIdx(got, 107))

	// The served positions folded into the requester's availability: the
	// replayed request goes quiet.
	clear(pushed)
	h.deliverMeta(t, "req", nil, []uint64{101, 107})
	require.Equal(t, 0, len(pushed))
}

// A request against a slot with no live signatures is forgotten at arrival:
// requests are served from live signatures only.
func TestServeRequestsExpiredSlot(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101)
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	pushed := h.installPush(t, map[peer.ID]blocks.PartialMessagePeerState{"req": {}}, map[peer.ID]bool{"req": true})
	h.deliverMeta(t, "req", nil, []uint64{101})
	require.Equal(t, 0, len(pushed))
}

// Advertised validators we lack are requested from the advertiser
// immediately; the fetch is oneshot, so a second advertiser is asked too.
func TestFetchAdvertisedIndices(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101)
	h.slotGroup(t, testSlot).meshPending = nil

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"adv": {}, "adv2": {}}
	metas := h.installMetaPush(t, peerStates)

	h.deliverMeta(t, "adv", []uint64{101, 102, 103}, nil)
	meta := metas["adv"]
	require.NotNil(t, meta)
	require.DeepEqual(t, []uint64{102, 103}, meta.Requests) // 101 already validated
	require.Equal(t, 0, len(meta.Available))

	// No requested state: a second advertiser is fetched from immediately.
	h.deliverMeta(t, "adv2", []uint64{102, 103}, nil)
	require.NotNil(t, metas["adv2"])
	require.DeepEqual(t, []uint64{102, 103}, metas["adv2"].Requests)
}

// Fetching for a slot we hold nothing of requests everything advertised:
// no data to gate on, no validation-lane involvement.
func TestFetchUnknownSlot(t *testing.T) {
	h := newStoreHarness(t)
	metas := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{"adv": {}})

	h.deliverMeta(t, "adv", []uint64{101, 102}, nil)
	require.Equal(t, 0, len(h.b.valJobs))
	require.NotNil(t, metas["adv"])
	require.DeepEqual(t, []uint64{101, 102}, metas["adv"].Requests)
	require.Equal(t, 0, len(metas["adv"].Available))
}

// OnEmitGossip marks the gossip peers in the tracked states and hands them to
// the Start loop; a full channel drops the emission but keeps the marking.
func TestOnEmitGossipMarksGossipPeers(t *testing.T) {
	b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot },
		func(primitives.Slot) time.Time { return time.Now().Add(time.Hour) }, DefaultPushInterval)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"g1": {}, "mesh": {}}

	b.OnEmitGossip(testTopic, GroupID(testSlot), []peer.ID{"g1"}, peerStates)
	require.Equal(t, 1, len(peerStates))
	require.Equal(t, false, hasIdx(peerStates, peer.ID("g1")))
	require.Equal(t, blocks.PeerUnknown, kind(peerStates, "mesh"))

	ge := <-b.gossip
	require.Equal(t, testTopic, ge.topic)
	require.Equal(t, testSlot, ge.slot)
	require.DeepEqual(t, []peer.ID{"g1"}, ge.peers)

	// A full channel drops instead of blocking the gossipsub loop.
	for range 5 {
		b.OnEmitGossip(testTopic, GroupID(testSlot), []peer.ID{"g1"}, peerStates)
	}
	require.Equal(t, 3, len(b.gossip))
}

func TestStoreCapsGroupsPerTopic(t *testing.T) {
	h := newStoreHarness(t)
	for s := range maxGroupsPerTopic {
		h.deliver(t, testSlot+primitives.Slot(s), 101)
	}
	require.Equal(t, maxGroupsPerTopic, len(h.b.groups[testTopic]))

	// Past the cap, bundles are still validated but not tracked.
	before := len(h.processed)
	h.deliver(t, testSlot+primitives.Slot(maxGroupsPerTopic), 101)
	require.Equal(t, maxGroupsPerTopic, len(h.b.groups[testTopic]))
	require.Equal(t, before+1, len(h.processed))
}

// A gossip emission sends gossip peers only the advertisement, never bundles.
func TestEmitGossipAdvertisesOnly(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"m": {}}
	partial := map[peer.ID]bool{"m": true, "g1": true}
	h.installPush(t, peerStates, partial)

	h.deliver(t, testSlot, 101, 107)
	h.b.flushDirty()
	g := h.slotGroup(t, testSlot)

	pushed, metas := h.installBoth(t, peerStates, partial)
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.Equal(t, 0, len(pushed))
	require.NotNil(t, metas["g1"])
	require.DeepEqual(t, []uint64{101, 107}, metas["g1"].Available)

	// No group and a closed group both publish nothing.
	clear(metas)
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot + 1, peers: []peer.ID{"g1"}})
	g.closePush()
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.Equal(t, 0, len(pushed))
	require.Equal(t, 0, len(metas))
}

// A tick pushes to the mesh peers only; remote entries are dropped from the
// map.
func TestPushGoesToMeshOnly(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{
		"m": {},
		"r": peerKind(blocks.PeerRemote),
	}
	partial := map[peer.ID]bool{"m": true, "r": true}
	pushed := h.installPush(t, peerStates, partial)

	h.deliver(t, testSlot, 101)
	h.b.flushDirty()

	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["m"]))
	require.Equal(t, 0, len(pushed["r"]))
	require.Equal(t, false, hasIdx(peerStates, peer.ID("r")))
	require.Equal(t, blocks.PeerMesh, kind(peerStates, "m"))
}

// A remote entry is never pushed to and is dropped at the first flush.
func TestRemotePeerNeverPushed(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"m": {}, "r": peerKind(blocks.PeerRemote)}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"m": true, "r": true})

	h.deliver(t, testSlot, 101, 107)
	h.b.flushDirty()
	require.Equal(t, false, hasIdx(peerStates, peer.ID("r")))

	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.Equal(t, false, hasIdx(pushed, peer.ID("r")))
}

// A failed publish and a refused yield both keep meshPending whole, and the
// next flush offers it without a duplicate to the peers already recorded.
func TestMeshPendingKeptOnFailedPublish(t *testing.T) {
	t.Run("publish error", func(t *testing.T) {
		h := newStoreHarness(t)
		h.installFailingPush()
		h.deliver(t, testSlot, 101)
		h.b.flushDirty()
		g := h.slotGroup(t, testSlot)
		require.DeepEqual(t, []primitives.ValidatorIndex{101}, g.meshPending)
		require.Equal(t, false, hasIdx(g.sent, 101))

		peerStates := map[peer.ID]blocks.PartialMessagePeerState{"p": {}}
		pushed := h.installPush(t, peerStates, map[peer.ID]bool{"p": true})
		h.b.flushDirty()
		require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["p"]))
		require.Equal(t, 0, len(g.meshPending))
	})

	t.Run("refused yield", func(t *testing.T) {
		h := newStoreHarness(t)
		peerStates := map[peer.ID]blocks.PartialMessagePeerState{"a": {}, "b": {}}
		partial := map[peer.ID]bool{"a": true, "b": true}
		pushed := h.installPushStopAfter(t, peerStates, partial, 1)

		h.deliver(t, testSlot, 101)
		h.b.flushDirty()
		g := h.slotGroup(t, testSlot)
		require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["a"]))
		require.DeepEqual(t, []primitives.ValidatorIndex{101}, g.meshPending)
		require.Equal(t, uint8(1), g.pushes[101])

		clear(pushed)
		pushed = h.installPush(t, peerStates, partial)
		h.b.flushDirty()
		require.Equal(t, 0, len(pushed["a"]))
		require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["b"]))
		require.Equal(t, 0, len(g.meshPending))
		require.Equal(t, uint8(2), g.pushes[101])
	})
}

// A mesh peer that the aborted walk never reached still gets the pending vote,
// which is not sent until every mesh peer was offered it.
func TestPendingSurvivesMissingMeshPeer(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"a": {}, "b": {}}
	partial := map[peer.ID]bool{"a": true, "b": true}
	h.installPushStopAfter(t, peerStates, partial, 1)

	h.deliver(t, testSlot, 101)
	h.b.flushDirty()
	g := h.slotGroup(t, testSlot)
	require.Equal(t, false, hasIdx(g.sent, 101))

	pushed := h.installPush(t, peerStates, partial)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["b"]))
	require.Equal(t, true, hasIdx(g.sent, 101))
}

// After a refused yield the retry sends only the bundles the peer never got.
func TestFalseYieldNoDuplicatePush(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"a": {}}
	partial := map[peer.ID]bool{"a": true}
	pushed := h.installPushStopAfter(t, peerStates, partial, 1)

	indices := make([]uint64, 60)
	for i := range indices {
		indices[i] = uint64(i)
	}
	h.deliver(t, testSlot, indices...)
	h.b.flushDirty()
	g := h.slotGroup(t, testSlot)
	require.DeepEqual(t, indices[:maxAttsPerBundle], pushedIndices(pushed["a"]))
	require.Equal(t, 60, len(g.meshPending))

	clear(pushed)
	pushed = h.installPush(t, peerStates, partial)
	h.b.flushDirty()
	require.DeepEqual(t, indices[maxAttsPerBundle:], pushedIndices(pushed["a"]))
	require.Equal(t, 0, len(g.meshPending))
}

// A mesh peer that appears after a failed publish gets the pending votes too.
func TestLateMeshPeerGetsPendingVotes(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"early": {}}
	partial := map[peer.ID]bool{"early": true, "late": true}

	h.installFailingPush()
	h.deliver(t, testSlot, 101)
	h.b.flushDirty()

	peerStates["late"] = blocks.PartialMessagePeerState{}
	pushed := h.installPush(t, peerStates, partial)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["early"]))
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["late"]))
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).meshPending))
}

// The retry after a refused yield keeps commit order across a bundle rollover.
func TestPushCommitOrderWithRollover(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"a": {}, "b": {}}
	partial := map[peer.ID]bool{"a": true, "b": true}
	pushed := h.installPushStopAfter(t, peerStates, partial, 1)

	indices := make([]uint64, 60)
	for i := range indices {
		indices[i] = uint64(i)
	}
	h.deliver(t, testSlot, indices...)
	h.b.flushDirty()
	require.DeepEqual(t, indices[:maxAttsPerBundle], pushedIndices(pushed["a"]))

	delete(peerStates, "a")
	clear(pushed)
	pushed = h.installPush(t, peerStates, partial)
	h.b.flushDirty()
	require.Equal(t, 2, len(pushed["b"]))
	require.DeepEqual(t, indices[:maxAttsPerBundle], pushed["b"][0].AttesterIndices)
	require.DeepEqual(t, indices[maxAttsPerBundle:], pushed["b"][1].AttesterIndices)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).meshPending))
}

// Past the end of the slot the cleanup pass closes the pushes: nothing is
// queued and nothing is offered, and the request path keeps serving.
func TestNoPushAfterDeadline(t *testing.T) {
	h := newStoreHarness(t)
	h.deadline = time.Now().Add(-time.Second)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"p": {}}
	partial := map[peer.ID]bool{"p": true}
	pushed := h.installPush(t, peerStates, partial)

	// The deadline bites only at the next cleanup pass.
	h.deliver(t, testSlot, 101)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["p"]))

	h.b.cleanup(*h.current)
	g := h.slotGroup(t, testSlot)
	require.Equal(t, true, g.pushClosed)
	require.Equal(t, 0, len(g.meshPending))

	clear(pushed)
	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.Equal(t, 0, len(pushed))
	require.Equal(t, 0, len(g.meshPending))
	require.Equal(t, true, hasIdx(g.validated, primitives.ValidatorIndex(101)))
	require.Equal(t, true, hasIdx(g.validated, primitives.ValidatorIndex(109)))
	require.Equal(t, true, hasIdx(g.sent, 101))
	require.Equal(t, false, hasIdx(g.sent, 109))

	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.validated))
	require.Equal(t, true, hasIdx(g.sent, 101))
}

// Serving an untracked requester never inserts it into the peer states: an
// unknown entry would be pushed to as a mesh peer.
func TestHandleMetaDoesNotReinsertPeer(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"req": true})
	h.deliverMeta(t, "req", nil, []uint64{101})

	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["req"]))
	require.Equal(t, 0, len(peerStates))
}

// A metadata sender gets only what it requests: nothing is pushed to it.
func TestMetadataSenderGetsOnlyRequests(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"m": {}}
	partial := map[peer.ID]bool{"m": true, "g": true}
	h.installPush(t, peerStates, partial)

	h.deliver(t, testSlot, 101, 107)
	h.b.flushDirty()

	peerStates["g"] = peerKind(blocks.PeerRemote, 101)
	pushed, metas := h.installBoth(t, peerStates, partial)
	h.deliverMeta(t, "g", []uint64{101}, nil)
	require.Equal(t, 0, len(pushed))
	require.Equal(t, 0, len(metas))

	h.deliverMeta(t, "g", []uint64{101}, []uint64{107})
	require.DeepEqual(t, []uint64{107}, pushedIndices(pushed["g"]))
	require.Equal(t, 0, len(metas))
}

// The cap must stay above the mesh high watermark gossipSubDhi (12, in
// beacon-chain/p2p/pubsub.go), or the mesh would lose votes to it.
func TestCapAboveMeshHigh(t *testing.T) {
	require.Equal(t, true, maxPushesPerVote > 12)
}

// An advertised validator whose vote is already in the validation lane is not
// fetched again; once the lane finishes, a new advertisement is.
func TestFetchSkipsQueuedIndices(t *testing.T) {
	h := newStoreHarness(t)
	h.acceptOnly = map[uint64]bool{} // reject everything
	metas := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{"adv": {}})

	h.deliverSigsNoPump(t, testSlot, []uint64{101}, [][]byte{idxSig(101)})
	h.deliverMeta(t, "adv", []uint64{101, 102}, nil)
	require.DeepEqual(t, []uint64{102}, metas["adv"].Requests)

	h.pump(t)
	require.Equal(t, 0, len(h.b.queued))
	h.deliverMeta(t, "adv", []uint64{101}, nil)
	require.DeepEqual(t, []uint64{101}, metas["adv"].Requests)
}

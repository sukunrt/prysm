package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sukunrt/go-waggle/pb"
	"github.com/sukunrt/go-waggle/waggle"
)

const (
	// maxIndicesPerMetadata is the ssz_max of the metadata lists.
	maxIndicesPerMetadata = 2048
	// maxOutboxPerPeer limits the requests, replies and available lists that wait for capacity to one peer.
	maxOutboxPerPeer = 64
	// advertWindow is the time after its arrival during which an attestation goes in available lists.
	advertWindow = 3 * time.Second
	// tombstoneAfter is the time after its arrival after which we keep only the validator of an
	// attestation, for dedup.
	tombstoneAfter = 6 * time.Second
)

// WaggleAttConfig holds the parameters of attestation propagation over waggle.
type WaggleAttConfig struct {
	// PushPeers is D: the number of peers that get each new attestation.
	PushPeers    int
	PushInterval time.Duration
	// MetadataPeers is K: the number of peers that get our available list on each metadata tick.
	MetadataPeers    int
	MetadataInterval time.Duration
	// RequestPeers is the maximum number of open requests for one attestation.
	RequestPeers int
	// RequestTimeout is the time after which we ask another peer for an attestation.
	RequestTimeout time.Duration
	// MaxValidations limits the validations that run at the same time, on all topics.
	MaxValidations int
}

// DefaultWaggleAttConfig returns the default parameters.
func DefaultWaggleAttConfig() WaggleAttConfig {
	return WaggleAttConfig{
		PushPeers:        8,
		PushInterval:     20 * time.Millisecond,
		MetadataPeers:    2,
		MetadataInterval: 100 * time.Millisecond,
		RequestPeers:     2,
		RequestTimeout:   500 * time.Millisecond,
		MaxValidations:   100_000,
	}
}

// withDefaults replaces the zero fields with the defaults. PushPeers and MetadataPeers can be zero.
func (c WaggleAttConfig) withDefaults() WaggleAttConfig {
	d := DefaultWaggleAttConfig()
	if c.PushInterval <= 0 {
		c.PushInterval = d.PushInterval
	}
	if c.MetadataInterval <= 0 {
		c.MetadataInterval = d.MetadataInterval
	}
	if c.RequestPeers <= 0 {
		c.RequestPeers = d.RequestPeers
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = d.RequestTimeout
	}
	if c.MaxValidations <= 0 {
		c.MaxValidations = d.MaxValidations
	}
	return c
}

// ValidateAttestationFn sends one attestation from peer from through the classic gossip
// validator. If the attestation is valid, it also stores it and sends it on gossipsub.
type ValidateAttestationFn = func(ctx context.Context, from peer.ID, topic string, att *ethpb.SingleAttestation) bool

// waggleIncoming is a decoded message in the propagation window. dataRoot is the root of the bundle data.
type waggleIncoming struct {
	from     peer.ID
	msg      waggleMessage
	dataRoot [32]byte
}

type valJob struct {
	ctx      context.Context
	from     peer.ID
	topic    string
	dataRoot [32]byte
	att      *ethpb.SingleAttestation
	done     chan<- valResult
}

type localAttestation struct {
	att      *ethpb.SingleAttestation
	dataRoot [32]byte
}

type valResult struct {
	dataRoot [32]byte
	att      *ethpb.SingleAttestation
	accepted bool
}

type bundleKey struct {
	committee primitives.CommitteeIndex
	dataRoot  [32]byte
}

type waggleBundle struct {
	data *ethpb.AttestationData
	sigs map[primitives.ValidatorIndex][]byte
}

// slotAttestations holds all the state of one slot. It expires when the attestations of the slot
// are no longer valid for gossip.
type slotAttestations struct {
	// seen is the dedup before validation. The key includes the data and the signature. It holds
	// the keys in validation and the rejected keys. validators and tombstones dedup the accepted ones.
	seen map[[32]byte]struct{}
	// queued counts the jobs in validation for each validator.
	queued map[primitives.ValidatorIndex]int
	// validators holds the validated attestations that are not tombstoned. After validation,
	// (slot, validator) is the dedup key.
	validators map[primitives.ValidatorIndex]bundleKey
	bundles    map[bundleKey]*waggleBundle
	// tombstones holds the validators of the validated attestations that we no longer keep.
	tombstones map[primitives.ValidatorIndex]struct{}
	// order holds the validated validators in the order we got them, and added their arrival
	// times. They only grow.
	order []primitives.ValidatorIndex
	added []time.Time
	// head is the first entry of order that is in the advert window. Entries before cold are
	// tombstoned. cold <= head.
	head int
	cold int
	// pending holds the validated validators that went to fewer than PushPeers peers.
	pending []primitives.ValidatorIndex
	// sends counts the peers that we pushed each attestation to.
	sends map[primitives.ValidatorIndex]int
	peers map[peer.ID]*slotPeer
	wants map[primitives.ValidatorIndex]*want
}

func newSlotAttestations() *slotAttestations {
	return &slotAttestations{
		seen:       make(map[[32]byte]struct{}),
		queued:     make(map[primitives.ValidatorIndex]int),
		validators: make(map[primitives.ValidatorIndex]bundleKey),
		bundles:    make(map[bundleKey]*waggleBundle),
		tombstones: make(map[primitives.ValidatorIndex]struct{}),
		sends:      make(map[primitives.ValidatorIndex]int),
		peers:      make(map[peer.ID]*slotPeer),
		wants:      make(map[primitives.ValidatorIndex]*want),
	}
}

// has reports whether we have the attestation of v or validate one now.
func (s *slotAttestations) has(v primitives.ValidatorIndex) bool {
	return s.validated(v) || s.queued[v] > 0
}

func (s *slotAttestations) validated(v primitives.ValidatorIndex) bool {
	if _, ok := s.validators[v]; ok {
		return true
	}
	_, ok := s.tombstones[v]
	return ok
}

func (s *slotAttestations) peer(p peer.ID) *slotPeer {
	sp := s.peers[p]
	if sp == nil {
		sp = &slotPeer{known: make(map[primitives.ValidatorIndex]struct{})}
		s.peers[p] = sp
	}
	return sp
}

func (s *slotAttestations) peerHas(p peer.ID, v primitives.ValidatorIndex) bool {
	sp := s.peers[p]
	return sp != nil && sp.has(v)
}

// slotPeer is our view of one peer in one slot.
type slotPeer struct {
	// known holds the attestations that the peer has: sent by us, sent to us, or in its available list.
	known map[primitives.ValidatorIndex]struct{}
	// told is the part of the slot order that our available lists to the peer cover. Lists start
	// at max(head, told).
	told int
}

func (sp *slotPeer) has(v primitives.ValidatorIndex) bool {
	_, ok := sp.known[v]
	return ok
}

func (sp *slotPeer) markKnown(v primitives.ValidatorIndex) {
	sp.known[v] = struct{}{}
}

type slotCommittee struct {
	slot      primitives.Slot
	committee primitives.CommitteeIndex
}

type openRequest struct {
	peer peer.ID
	at   time.Time
}

// want is an attestation that we do not have and that a peer announced.
type want struct {
	committee primitives.CommitteeIndex
	open      []openRequest
	asked     map[peer.ID]struct{}
}

// waggleAttTopic runs EIP-8334 on one attestation subnet topic. The run loop owns all state.
type waggleAttTopic struct {
	name    string
	topic   *waggle.Topic
	cfg     WaggleAttConfig
	genesis func() time.Time
	rng     *rand.Rand

	cancel context.CancelFunc
	wg     sync.WaitGroup

	incoming chan waggleIncoming
	local    chan localAttestation
	sendDone chan struct{}
	validate func(valJob) bool
	valDone  chan valResult

	slots  map[primitives.Slot]*slotAttestations
	outbox map[peer.ID][]waggleMessage
}

func newWaggleAttTopic(name string, cfg WaggleAttConfig, genesis func() time.Time, validate func(valJob) bool) *waggleAttTopic {
	return &waggleAttTopic{
		name:     name,
		cfg:      cfg,
		genesis:  genesis,
		rng:      rand.NewGenerator(),
		incoming: make(chan waggleIncoming, 64),
		local:    make(chan localAttestation, 1024),
		sendDone: make(chan struct{}, 1),
		validate: validate,
		valDone:  make(chan valResult, 256),
		slots:    make(map[primitives.Slot]*slotAttestations),
		outbox:   make(map[peer.ID][]waggleMessage),
	}
}

func (t *waggleAttTopic) start(ctx context.Context) {
	ctx, t.cancel = context.WithCancel(ctx)
	t.wg.Go(func() { t.receive(ctx) })
	t.wg.Go(func() { t.run(ctx) })
}

// stop must run after the waggle topic is closed, so that receive returns.
func (t *waggleAttTopic) stop() {
	t.cancel()
	t.wg.Wait()
}

// onSendComplete runs on the waggle sender goroutine. It must not block.
func (t *waggleAttTopic) onSendComplete(context.Context, peer.ID) {
	select {
	case t.sendDone <- struct{}{}:
	default:
	}
}

// submit runs on the caller goroutine. It does the checks and the hash before the run loop.
func (t *waggleAttTopic) submit(att *ethpb.SingleAttestation) {
	if att.Data == nil || !t.inWindow(att.Data.Slot) {
		return
	}
	root, err := att.Data.HashTreeRoot()
	if err != nil {
		log.WithError(err).Debug("Could not hash attestation data")
		return
	}
	select {
	case t.local <- localAttestation{att: att, dataRoot: root}:
	default:
		log.WithField("topic", t.name).Debug("Waggle submit queue full, dropping attestation")
	}
}

// receive decodes and checks the messages before the run loop gets them.
func (t *waggleAttTopic) receive(ctx context.Context) {
	for {
		from, raw := t.topic.ReceiveMessage()
		if raw == nil {
			return
		}
		in, err := t.decode(from, raw)
		if err != nil {
			log.WithError(err).WithField("peer", from).Debug("Could not decode waggle message")
			continue
		}
		if !t.inWindow(in.msg.Slot) {
			continue
		}
		select {
		case t.incoming <- in:
		case <-ctx.Done():
			return
		}
	}
}

func (t *waggleAttTopic) decode(from peer.ID, raw *pb.ObjectPieces) (waggleIncoming, error) {
	m, err := decodeWaggleMessage(raw)
	if err != nil {
		return waggleIncoming{}, err
	}
	in := waggleIncoming{from: from, msg: m}
	if m.Bundle != nil {
		if in.dataRoot, err = m.Bundle.AttestationData.HashTreeRoot(); err != nil {
			return waggleIncoming{}, errors.Wrap(err, "hash attestation data")
		}
	}
	return in, nil
}

func (t *waggleAttTopic) run(ctx context.Context) {
	push := time.NewTicker(t.cfg.PushInterval)
	defer push.Stop()
	meta := time.NewTicker(t.cfg.MetadataInterval)
	defer meta.Stop()
	expiry := time.NewTimer(time.Until(t.nextExpiry()))
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-t.incoming:
			t.handleIncoming(ctx, in)
		case r := <-t.valDone:
			t.handleValResult(r)
		case l := <-t.local:
			t.handleLocal(l)
		case <-t.sendDone:
			t.push()
		case <-push.C:
			t.expireRequests()
			t.push()
		case <-meta.C:
			t.age()
			t.sendMetadata()
		case <-expiry.C:
			t.prune()
			expiry.Reset(time.Until(t.nextExpiry()))
		}
	}
}

// A slot lives while its attestations are valid for gossip: until the end of the next epoch
// (EIP-7045), plus the clock disparity. After that, waggle drops its state.
func (t *waggleAttTopic) expiryGenesis() time.Time {
	return t.genesis().Add(params.BeaconConfig().MaximumGossipClockDisparityDuration())
}

func (t *waggleAttTopic) expiryEpoch() primitives.Epoch {
	return slots.ToEpoch(slots.CurrentSlot(t.expiryGenesis()))
}

// oldestLive returns the oldest slot that has not expired: the start of the previous epoch.
func (t *waggleAttTopic) oldestLive() primitives.Slot {
	e := t.expiryEpoch()
	if e == 0 {
		return 0
	}
	return slots.UnsafeEpochStart(e - 1)
}

// nextExpiry returns when the oldest live epoch expires.
func (t *waggleAttTopic) nextExpiry() time.Time {
	return slots.UnsafeStartTime(t.expiryGenesis(), slots.UnsafeEpochStart(t.expiryEpoch()+1))
}

// inWindow reports whether slot is live: not expired and at most one slot ahead of the clock.
func (t *waggleAttTopic) inWindow(slot primitives.Slot) bool {
	return slot >= t.oldestLive() && slot <= slots.CurrentSlot(t.genesis())+1
}

func (t *waggleAttTopic) slot(slot primitives.Slot) *slotAttestations {
	s := t.slots[slot]
	if s == nil {
		s = newSlotAttestations()
		t.slots[slot] = s
	}
	return s
}

// pendingSlots returns the slots with attestations to push, oldest first.
func (t *waggleAttTopic) pendingSlots() []primitives.Slot {
	var keys []primitives.Slot
	for slot, s := range t.slots {
		if len(s.pending) > 0 {
			keys = append(keys, slot)
		}
	}
	slices.Sort(keys)
	return keys
}

func (t *waggleAttTopic) handleIncoming(ctx context.Context, in waggleIncoming) {
	m := in.msg
	if m.Bundle != nil {
		t.handleBundle(ctx, in.from, m.Slot, m.Bundle, in.dataRoot)
	}
	if m.Metadata != nil {
		t.handleMetadata(in.from, m.Slot, m.Metadata)
	}
	if len(t.outbox[in.from]) > 0 {
		t.flush(in.from)
	}
}

// attestationKey is the dedup key before validation. It includes the data and the signature,
// so an invalid signature cannot stop a valid one.
func attestationKey(dataRoot [32]byte, committee primitives.CommitteeIndex, v primitives.ValidatorIndex, sig []byte) [32]byte {
	buf := make([]byte, 0, 32+8+8+len(sig))
	buf = append(buf, dataRoot[:]...)
	buf = binary.LittleEndian.AppendUint64(buf, uint64(committee))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(v))
	buf = append(buf, sig...)
	return sha256.Sum256(buf)
}

// handleBundle drops attestations of validated validators and seen keys, and sends the
// rest to validation. A job is marked seen only when it starts, so a dropped job can come again.
func (t *waggleAttTopic) handleBundle(ctx context.Context, from peer.ID, slot primitives.Slot, b *ethpb.AttestationBundle, root [32]byte) {
	s := t.slot(slot)
	sp := s.peer(from)
	for i, idx := range b.AttesterIndices {
		v := primitives.ValidatorIndex(idx)
		sp.markKnown(v)
		if s.validated(v) {
			continue
		}
		key := attestationKey(root, b.CommitteeIndex, v, b.Signatures[i])
		if _, ok := s.seen[key]; ok {
			continue
		}
		job := valJob{
			ctx:      ctx,
			from:     from,
			topic:    t.name,
			dataRoot: root,
			att: &ethpb.SingleAttestation{
				CommitteeId:   b.CommitteeIndex,
				AttesterIndex: v,
				Data:          b.AttestationData,
				Signature:     b.Signatures[i],
			},
			done: t.valDone,
		}
		if !t.validate(job) {
			log.WithField("topic", t.name).Debug("Waggle validation limit reached, dropping attestations")
			return
		}
		s.seen[key] = struct{}{}
		s.queued[v]++
		delete(s.wants, v)
	}
}

func (t *waggleAttTopic) handleValResult(r valResult) {
	s := t.slots[r.att.Data.Slot]
	if s == nil {
		return
	}
	v := r.att.AttesterIndex
	if s.queued[v]--; s.queued[v] <= 0 {
		delete(s.queued, v)
	}
	if r.accepted {
		delete(s.seen, attestationKey(r.dataRoot, r.att.CommitteeId, v, r.att.Signature))
		s.store(r.dataRoot, r.att)
	}
}

// handleLocal stores an attestation that is already validated: our own, or one from gossipsub.
func (t *waggleAttTopic) handleLocal(l localAttestation) {
	t.slot(l.att.Data.Slot).store(l.dataRoot, l.att)
}

// store keeps the first validated attestation of each validator and queues it for push.
func (s *slotAttestations) store(root [32]byte, att *ethpb.SingleAttestation) {
	v := att.AttesterIndex
	if s.validated(v) {
		return
	}
	key := bundleKey{committee: att.CommitteeId, dataRoot: root}
	b := s.bundles[key]
	if b == nil {
		b = &waggleBundle{data: att.Data, sigs: make(map[primitives.ValidatorIndex][]byte)}
		s.bundles[key] = b
	}
	b.sigs[v] = att.Signature
	s.validators[v] = key
	s.order = append(s.order, v)
	s.added = append(s.added, time.Now())
	s.pending = append(s.pending, v)
	delete(s.wants, v)
}

func (t *waggleAttTopic) age() {
	now := time.Now()
	for _, s := range t.slots {
		s.age(now)
	}
}

// age moves the head past the attestations older than advertWindow, and tombstones the ones
// older than tombstoneAfter.
func (s *slotAttestations) age(now time.Time) {
	for s.head < len(s.order) && now.Sub(s.added[s.head]) > advertWindow {
		s.head++
	}
	start := s.cold
	for s.cold < s.head && now.Sub(s.added[s.cold]) > tombstoneAfter {
		s.tombstone(s.order[s.cold])
		s.cold++
	}
	if s.cold > start {
		s.pending = slices.DeleteFunc(s.pending, func(v primitives.ValidatorIndex) bool {
			_, ok := s.validators[v]
			return !ok
		})
	}
}

func (s *slotAttestations) tombstone(v primitives.ValidatorIndex) {
	key, ok := s.validators[v]
	if !ok {
		return
	}
	delete(s.validators, v)
	s.tombstones[v] = struct{}{}
	if b := s.bundles[key]; b != nil {
		if delete(b.sigs, v); len(b.sigs) == 0 {
			delete(s.bundles, key)
		}
	}
	delete(s.sends, v)
	for _, sp := range s.peers {
		delete(sp.known, v)
	}
}

// handleMetadata requests the announced attestations that we do not have, and serves the
// requests from our store. A request that we cannot serve is dropped.
func (t *waggleAttTopic) handleMetadata(from peer.ID, slot primitives.Slot, md *ethpb.CommitteeAttestationPartsMetadata) {
	s := t.slot(slot)
	sp := s.peer(from)
	now := time.Now()
	var requests []uint64
	for _, idx := range md.Available {
		v := primitives.ValidatorIndex(idx)
		sp.markKnown(v)
		if s.has(v) {
			continue
		}
		if t.request(s, v, md.CommitteeIndex, from, now) {
			requests = append(requests, idx)
		}
	}
	if len(requests) > 0 {
		t.enqueue(from, waggleMessage{
			Slot:     slot,
			Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: md.CommitteeIndex, Requests: requests},
		})
	}
	if len(md.Requests) > 0 {
		t.serve(from, sp, slot, s, md.Requests)
	}
}

// request records an open request for v to p. It returns false if v has enough open requests
// or p was already asked.
func (t *waggleAttTopic) request(s *slotAttestations, v primitives.ValidatorIndex, committee primitives.CommitteeIndex, p peer.ID, now time.Time) bool {
	w := s.wants[v]
	if w == nil {
		w = &want{committee: committee, asked: make(map[peer.ID]struct{})}
		s.wants[v] = w
	}
	if len(w.open) >= t.cfg.RequestPeers {
		return false
	}
	if _, ok := w.asked[p]; ok {
		return false
	}
	w.open = append(w.open, openRequest{peer: p, at: now})
	w.asked[p] = struct{}{}
	return true
}

func (t *waggleAttTopic) serve(from peer.ID, sp *slotPeer, slot primitives.Slot, s *slotAttestations, requests []uint64) {
	bb := newBundler(slot, s)
	for _, idx := range requests {
		v := primitives.ValidatorIndex(idx)
		if _, ok := s.validators[v]; !ok || sp.has(v) {
			continue
		}
		sp.markKnown(v)
		bb.add(v)
	}
	for _, m := range bb.msgs {
		t.enqueue(from, m)
	}
}

// enqueue holds a request, a reply or an available list for p until p has capacity. It returns
// false if the outbox of p is full.
func (t *waggleAttTopic) enqueue(p peer.ID, m waggleMessage) bool {
	if len(t.outbox[p]) >= maxOutboxPerPeer {
		log.WithField("peer", p).Debug("Waggle outbox full, dropping message")
		return false
	}
	t.outbox[p] = append(t.outbox[p], m)
	return true
}

func (t *waggleAttTopic) send(p peer.ID, m waggleMessage) {
	msg, err := m.encode()
	if err != nil {
		log.WithError(err).Debug("Could not encode waggle message")
		return
	}
	t.topic.SendMessage(p, msg)
}

// flush sends to p while it has capacity: first the outbox, then the push. If p has no stream
// to us, its state goes, so that it starts fresh when it subscribes again.
func (t *waggleAttTopic) flush(p peer.ID) {
	free, err := t.topic.GetPeerCapacity(p)
	if errors.Is(err, waggle.ErrPeerNotFound) {
		delete(t.outbox, p)
		for _, s := range t.slots {
			delete(s.peers, p)
		}
		return
	}
	if err != nil || free == 0 {
		return
	}
	q := t.outbox[p]
	n := min(free, len(q))
	for _, m := range q[:n] {
		t.send(p, m)
	}
	if n == len(q) {
		delete(t.outbox, p)
	} else {
		t.outbox[p] = q[n:]
	}
	if free -= n; free > 0 {
		t.pushTo(p, free)
	}
}

// push sends the pending attestations to peers with capacity, in random order. Each
// attestation goes to at most PushPeers peers that do not have it.
func (t *waggleAttTopic) push() {
	for _, s := range t.slots {
		s.compactPending(t.cfg.PushPeers)
	}
	peers := t.topic.PeersWithCapacity()
	t.rng.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	for _, p := range peers {
		t.flush(p)
	}
}

// pushTo sends up to free bundles to p, oldest slot first.
func (t *waggleAttTopic) pushTo(p peer.ID, free int) {
	for _, slot := range t.pendingSlots() {
		s := t.slots[slot]
		bb := newBundler(slot, s)
		for _, v := range s.pending {
			if s.sends[v] < t.cfg.PushPeers && !s.peerHas(p, v) {
				bb.add(v)
			}
		}
		if len(bb.msgs) == 0 {
			continue
		}
		sp := s.peer(p)
		for _, m := range bb.msgs {
			if free == 0 {
				return
			}
			for _, idx := range m.Bundle.AttesterIndices {
				v := primitives.ValidatorIndex(idx)
				s.sends[v]++
				sp.markKnown(v)
			}
			t.send(p, m)
			free--
		}
	}
}

// compactPending drops the attestations that went to pushPeers peers.
func (s *slotAttestations) compactPending(pushPeers int) {
	s.pending = slices.DeleteFunc(s.pending, func(v primitives.ValidatorIndex) bool { return s.sends[v] >= pushPeers })
}

// sendMetadata sends our available lists to MetadataPeers random peers with capacity. The
// lists are deltas: a peer gets each validator at most once, and only in the advert window.
func (t *waggleAttTopic) sendMetadata() {
	peers := t.topic.PeersWithCapacity()
	t.rng.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	for _, p := range peers[:min(t.cfg.MetadataPeers, len(peers))] {
		for slot, s := range t.slots {
			sp := s.peer(p)
			msgs := availableDelta(slot, s, sp)
			if len(t.outbox[p])+len(msgs) > maxOutboxPerPeer {
				continue
			}
			for _, m := range msgs {
				t.enqueue(p, m)
			}
			sp.told = len(s.order)
		}
		t.flush(p)
	}
}

// availableDelta returns the available lists for the validators that we got after the last list
// to the peer and in the advert window, without the ones that the peer is known to have. It has
// one list per committee, split at maxIndicesPerMetadata.
func availableDelta(slot primitives.Slot, s *slotAttestations, sp *slotPeer) []waggleMessage {
	from := max(s.head, sp.told)
	if from >= len(s.order) {
		return nil
	}
	byCommittee := make(map[primitives.CommitteeIndex][]uint64)
	var committees []primitives.CommitteeIndex
	for _, v := range s.order[from:] {
		if sp.has(v) {
			continue
		}
		c := s.validators[v].committee
		if _, ok := byCommittee[c]; !ok {
			committees = append(committees, c)
		}
		byCommittee[c] = append(byCommittee[c], uint64(v))
	}
	var msgs []waggleMessage
	for _, c := range committees {
		for ids := range slices.Chunk(byCommittee[c], maxIndicesPerMetadata) {
			msgs = append(msgs, waggleMessage{
				Slot:     slot,
				Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: c, Available: ids},
			})
		}
	}
	return msgs
}

// expireRequests drops open requests that are older than RequestTimeout, and asks other
// peers that announced the attestation.
func (t *waggleAttTopic) expireRequests() {
	now := time.Now()
	batches := make(map[peer.ID]map[slotCommittee][]uint64)
	for slot, s := range t.slots {
		for v, w := range s.wants {
			w.open = slices.DeleteFunc(w.open, func(r openRequest) bool { return now.Sub(r.at) >= t.cfg.RequestTimeout })
			for p, sp := range s.peers {
				if len(w.open) >= t.cfg.RequestPeers {
					break
				}
				if !sp.has(v) || !t.request(s, v, w.committee, p, now) {
					continue
				}
				if batches[p] == nil {
					batches[p] = make(map[slotCommittee][]uint64)
				}
				sc := slotCommittee{slot: slot, committee: w.committee}
				batches[p][sc] = append(batches[p][sc], uint64(v))
			}
			if len(w.open) == 0 {
				delete(s.wants, v)
			}
		}
	}
	for p, bySC := range batches {
		for sc, ids := range bySC {
			t.enqueue(p, waggleMessage{
				Slot:     sc.slot,
				Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: sc.committee, Requests: ids},
			})
		}
		t.flush(p)
	}
}

// prune drops the expired slots and their messages in the outbox.
func (t *waggleAttTopic) prune() {
	live := t.oldestLive()
	for slot := range t.slots {
		if slot < live {
			delete(t.slots, slot)
		}
	}
	for p, q := range t.outbox {
		if q = slices.DeleteFunc(q, func(m waggleMessage) bool { return m.Slot < live }); len(q) == 0 {
			delete(t.outbox, p)
		} else {
			t.outbox[p] = q
		}
	}
}

// bundler groups the attestations of one slot into bundles: one per committee and data,
// with at most maxAttestationsPerBundle attestations.
type bundler struct {
	slot  primitives.Slot
	s     *slotAttestations
	byKey map[bundleKey]*ethpb.AttestationBundle
	msgs  []waggleMessage
}

func newBundler(slot primitives.Slot, s *slotAttestations) *bundler {
	return &bundler{slot: slot, s: s, byKey: make(map[bundleKey]*ethpb.AttestationBundle)}
}

func (b *bundler) add(v primitives.ValidatorIndex) {
	key := b.s.validators[v]
	wb := b.s.bundles[key]
	cur := b.byKey[key]
	if cur == nil || len(cur.AttesterIndices) >= maxAttestationsPerBundle {
		cur = &ethpb.AttestationBundle{CommitteeIndex: key.committee, AttestationData: wb.data}
		b.byKey[key] = cur
		b.msgs = append(b.msgs, waggleMessage{Slot: b.slot, Bundle: cur})
	}
	cur.AttesterIndices = append(cur.AttesterIndices, uint64(v))
	cur.Signatures = append(cur.Signatures, wb.sigs[v])
}

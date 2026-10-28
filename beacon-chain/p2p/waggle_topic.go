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
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
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
	// tombstoneAfter is the time after its arrival after which we keep only the vote of an
	// attestation, for dedup.
	tombstoneAfter = 6 * time.Second
	// maxVotesPerValidator limits the votes of a validator in a slot. Two different votes are a
	// double vote: enough for an AttesterSlashing. A validator is slashed only once.
	maxVotesPerValidator = 2
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
// ValidationReject means that from is at fault. ValidationIgnore does not.
type ValidateAttestationFn = func(ctx context.Context, from peer.ID, topic string, att *ethpb.SingleAttestation) pubsub.ValidationResult

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
	res      pubsub.ValidationResult
}

// seenEntry is a key in validation or a key that did not pass validation. senders holds the
// peers that sent the key.
type seenEntry struct {
	rejected bool
	senders  []peer.ID
}

func (e *seenEntry) addSender(p peer.ID) {
	if !slices.Contains(e.senders, p) {
		e.senders = append(e.senders, p)
	}
}

// vote identifies an attestation in a slot. A validator can have more than one vote in a slot
// (a double vote), so a vote is the data root and the validator.
type vote struct {
	root [32]byte
	v    primitives.ValidatorIndex
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
	// the keys in validation and the keys that did not pass. votes and tombstones dedup the
	// accepted ones.
	// TODO: the keys that did not pass stay until the slot expires, so a peer can fill seen with
	// bad signatures. Peer scoring must limit this.
	seen map[[32]byte]*seenEntry
	// queued counts the jobs in validation for each vote.
	queued map[vote]int
	// votes holds the committees of the validated votes that are not tombstoned.
	votes   map[vote]primitives.CommitteeIndex
	bundles map[bundleKey]*waggleBundle
	// tombstones holds the validated votes that we no longer keep.
	tombstones map[vote]struct{}
	// perValidator counts the validated votes of each validator, up to maxVotesPerValidator.
	perValidator map[primitives.ValidatorIndex]int
	// order holds the validated votes in the order we got them, and added their arrival times.
	// They only grow.
	order []vote
	added []time.Time
	// head is the first entry of order that is in the advert window. Entries before cold are
	// tombstoned. cold <= head.
	head int
	cold int
	// pending holds the validated votes that went to fewer than PushPeers peers.
	pending []vote
	// sends counts the peers that we pushed each vote to.
	sends map[vote]int
	peers map[peer.ID]*slotPeer
	wants map[vote]*want
}

func newSlotAttestations() *slotAttestations {
	return &slotAttestations{
		seen:         make(map[[32]byte]*seenEntry),
		queued:       make(map[vote]int),
		votes:        make(map[vote]primitives.CommitteeIndex),
		bundles:      make(map[bundleKey]*waggleBundle),
		tombstones:   make(map[vote]struct{}),
		perValidator: make(map[primitives.ValidatorIndex]int),
		sends:        make(map[vote]int),
		peers:        make(map[peer.ID]*slotPeer),
		wants:        make(map[vote]*want),
	}
}

// has reports whether we have the vote or validate it now.
func (s *slotAttestations) has(vt vote) bool {
	return s.validated(vt) || s.queued[vt] > 0
}

func (s *slotAttestations) validated(vt vote) bool {
	if _, ok := s.votes[vt]; ok {
		return true
	}
	_, ok := s.tombstones[vt]
	return ok
}

// full reports whether v has maxVotesPerValidator validated votes. Then we drop its other votes.
func (s *slotAttestations) full(v primitives.ValidatorIndex) bool {
	return s.perValidator[v] >= maxVotesPerValidator
}

func (s *slotAttestations) peer(p peer.ID) *slotPeer {
	sp := s.peers[p]
	if sp == nil {
		sp = &slotPeer{known: make(map[vote]struct{})}
		s.peers[p] = sp
	}
	return sp
}

func (s *slotAttestations) peerHas(p peer.ID, vt vote) bool {
	sp := s.peers[p]
	return sp != nil && sp.has(vt)
}

// slotPeer is our view of one peer in one slot.
type slotPeer struct {
	// known holds the votes that the peer has: sent by us, sent to us, or in its available list.
	known map[vote]struct{}
	// told is the part of the slot order that our available lists to the peer cover. Lists start
	// at max(head, told).
	told int
}

func (sp *slotPeer) has(vt vote) bool {
	_, ok := sp.known[vt]
	return ok
}

// TODO: known has no limit. The indices are not checked against the committee, so a peer can
// fill known with junk in each live slot. Limit it to MAX_VALIDATORS_PER_COMMITTEE for each peer
// and slot, and use peer scoring.
func (sp *slotPeer) markKnown(vt vote) {
	sp.known[vt] = struct{}{}
}

// listKey identifies a metadata list: one slot, committee and data root.
type listKey struct {
	slot primitives.Slot
	bundleKey
}

type openRequest struct {
	peer peer.ID
	at   time.Time
}

// want is a vote that we do not have and that a peer announced.
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
			// TODO: peer scoring. The sender of a message that we cannot decode is at fault.
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

// handleBundle drops validated votes, votes of validators with maxVotesPerValidator votes, and
// seen keys, and sends the rest to validation. A job is marked seen only when it starts, so a
// dropped job can come again.
func (t *waggleAttTopic) handleBundle(ctx context.Context, from peer.ID, slot primitives.Slot, b *ethpb.AttestationBundle, root [32]byte) {
	s := t.slot(slot)
	sp := s.peer(from)
	for i, idx := range b.AttesterIndices {
		v := primitives.ValidatorIndex(idx)
		vt := vote{root: root, v: v}
		sp.markKnown(vt)
		if s.validated(vt) || s.full(v) {
			continue
		}
		key := attestationKey(root, b.CommitteeIndex, v, b.Signatures[i])
		if e := s.seen[key]; e != nil {
			// TODO: peer scoring. If e.rejected, from is at fault.
			e.addSender(from)
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
		s.seen[key] = &seenEntry{senders: []peer.ID{from}}
		s.queued[vt]++
		delete(s.wants, vt)
	}
}

func (t *waggleAttTopic) handleValResult(r valResult) {
	s := t.slots[r.att.Data.Slot]
	if s == nil {
		return
	}
	vt := vote{root: r.dataRoot, v: r.att.AttesterIndex}
	if s.queued[vt]--; s.queued[vt] <= 0 {
		delete(s.queued, vt)
	}
	key := attestationKey(r.dataRoot, r.att.CommitteeId, vt.v, r.att.Signature)
	if r.res != pubsub.ValidationAccept {
		// TODO: peer scoring. On ValidationReject, all senders of the key are at fault, as in gossipsub.
		if e := s.seen[key]; e != nil {
			e.rejected = r.res == pubsub.ValidationReject
		}
		return
	}
	delete(s.seen, key)
	if s.store(r.dataRoot, r.att) && s.perValidator[vt.v] > 1 {
		log.WithField("validator", vt.v).WithField("slot", r.att.Data.Slot).Debug("Waggle got a double vote")
	}
}

// handleLocal stores an attestation that is already validated: our own, or one from gossipsub.
func (t *waggleAttTopic) handleLocal(l localAttestation) {
	t.slot(l.att.Data.Slot).store(l.dataRoot, l.att)
}

// store keeps a validated vote and queues it for push. It keeps at most maxVotesPerValidator
// votes of each validator. It returns false if it drops the vote.
func (s *slotAttestations) store(root [32]byte, att *ethpb.SingleAttestation) bool {
	vt := vote{root: root, v: att.AttesterIndex}
	if s.validated(vt) || s.full(vt.v) {
		return false
	}
	key := bundleKey{committee: att.CommitteeId, dataRoot: root}
	b := s.bundles[key]
	if b == nil {
		b = &waggleBundle{data: att.Data, sigs: make(map[primitives.ValidatorIndex][]byte)}
		s.bundles[key] = b
	}
	b.sigs[vt.v] = att.Signature
	s.votes[vt] = att.CommitteeId
	s.perValidator[vt.v]++
	s.order = append(s.order, vt)
	s.added = append(s.added, time.Now())
	s.pending = append(s.pending, vt)
	delete(s.wants, vt)
	return true
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
		s.pending = slices.DeleteFunc(s.pending, func(vt vote) bool {
			_, ok := s.votes[vt]
			return !ok
		})
	}
}

func (s *slotAttestations) tombstone(vt vote) {
	committee, ok := s.votes[vt]
	if !ok {
		return
	}
	delete(s.votes, vt)
	s.tombstones[vt] = struct{}{}
	key := bundleKey{committee: committee, dataRoot: vt.root}
	if b := s.bundles[key]; b != nil {
		if delete(b.sigs, vt.v); len(b.sigs) == 0 {
			delete(s.bundles, key)
		}
	}
	delete(s.sends, vt)
	for _, sp := range s.peers {
		delete(sp.known, vt)
	}
}

// handleMetadata requests the announced attestations that we do not have, and serves the
// requests from our store. A request that we cannot serve is dropped.
func (t *waggleAttTopic) handleMetadata(from peer.ID, slot primitives.Slot, md *ethpb.CommitteeAttestationPartsMetadata) {
	s := t.slot(slot)
	sp := s.peer(from)
	root := bytesutil.ToBytes32(md.DataRoot)
	now := time.Now()
	var requests []uint64
	for _, idx := range md.Available {
		vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
		sp.markKnown(vt)
		if s.has(vt) || s.full(vt.v) {
			continue
		}
		if t.request(s, vt, md.CommitteeIndex, from, now) {
			requests = append(requests, idx)
		}
	}
	if len(requests) > 0 {
		t.enqueue(from, waggleMessage{
			Slot:     slot,
			Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: md.CommitteeIndex, DataRoot: root[:], Requests: requests},
		})
	}
	if len(md.Requests) > 0 {
		t.serve(from, sp, slot, s, root, md.Requests)
	}
}

// request records an open request for vt to p. It returns false if vt has enough open requests
// or p was already asked.
func (t *waggleAttTopic) request(s *slotAttestations, vt vote, committee primitives.CommitteeIndex, p peer.ID, now time.Time) bool {
	w := s.wants[vt]
	if w == nil {
		w = &want{committee: committee, asked: make(map[peer.ID]struct{})}
		s.wants[vt] = w
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

func (t *waggleAttTopic) serve(from peer.ID, sp *slotPeer, slot primitives.Slot, s *slotAttestations, root [32]byte, requests []uint64) {
	bb := newBundler(slot, s)
	for _, idx := range requests {
		vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
		if _, ok := s.votes[vt]; !ok || sp.has(vt) {
			continue
		}
		sp.markKnown(vt)
		bb.add(vt)
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
		for _, vt := range s.pending {
			if s.sends[vt] < t.cfg.PushPeers && !s.peerHas(p, vt) {
				bb.add(vt)
			}
		}
		if len(bb.msgs) == 0 {
			continue
		}
		sp := s.peer(p)
		for i, m := range bb.msgs {
			if free == 0 {
				return
			}
			for _, idx := range m.Bundle.AttesterIndices {
				vt := vote{root: bb.roots[i], v: primitives.ValidatorIndex(idx)}
				s.sends[vt]++
				sp.markKnown(vt)
			}
			t.send(p, m)
			free--
		}
	}
}

// compactPending drops the attestations that went to pushPeers peers.
func (s *slotAttestations) compactPending(pushPeers int) {
	s.pending = slices.DeleteFunc(s.pending, func(vt vote) bool { return s.sends[vt] >= pushPeers })
}

// sendMetadata sends our available lists to MetadataPeers random peers with capacity. The
// lists are deltas: a peer gets each vote at most once, and only in the advert window.
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

// availableDelta returns the available lists for the votes that we got after the last list to
// the peer and in the advert window, without the ones that the peer is known to have. It has one
// list per committee and data root, split at maxIndicesPerMetadata.
func availableDelta(slot primitives.Slot, s *slotAttestations, sp *slotPeer) []waggleMessage {
	from := max(s.head, sp.told)
	if from >= len(s.order) {
		return nil
	}
	byKey := make(map[bundleKey][]uint64)
	var keys []bundleKey
	for _, vt := range s.order[from:] {
		if sp.has(vt) {
			continue
		}
		k := bundleKey{committee: s.votes[vt], dataRoot: vt.root}
		if _, ok := byKey[k]; !ok {
			keys = append(keys, k)
		}
		byKey[k] = append(byKey[k], uint64(vt.v))
	}
	var msgs []waggleMessage
	for _, k := range keys {
		for ids := range slices.Chunk(byKey[k], maxIndicesPerMetadata) {
			msgs = append(msgs, waggleMessage{
				Slot:     slot,
				Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: k.committee, DataRoot: k.dataRoot[:], Available: ids},
			})
		}
	}
	return msgs
}

// expireRequests drops open requests that are older than RequestTimeout, and asks other
// peers that announced the attestation.
// TODO: peer scoring. Peers that do not serve the attestations that they announced are at fault.
func (t *waggleAttTopic) expireRequests() {
	now := time.Now()
	batches := make(map[peer.ID]map[listKey][]uint64)
	for slot, s := range t.slots {
		for vt, w := range s.wants {
			w.open = slices.DeleteFunc(w.open, func(r openRequest) bool { return now.Sub(r.at) >= t.cfg.RequestTimeout })
			for p, sp := range s.peers {
				if len(w.open) >= t.cfg.RequestPeers {
					break
				}
				if !sp.has(vt) || !t.request(s, vt, w.committee, p, now) {
					continue
				}
				if batches[p] == nil {
					batches[p] = make(map[listKey][]uint64)
				}
				lk := listKey{slot: slot, bundleKey: bundleKey{committee: w.committee, dataRoot: vt.root}}
				batches[p][lk] = append(batches[p][lk], uint64(vt.v))
			}
			if len(w.open) == 0 {
				delete(s.wants, vt)
			}
		}
	}
	for p, byList := range batches {
		for lk, ids := range byList {
			t.enqueue(p, waggleMessage{
				Slot:     lk.slot,
				Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: lk.committee, DataRoot: lk.dataRoot[:], Requests: ids},
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

// bundler groups the votes of one slot into bundles: one per committee and data, with at most
// maxAttestationsPerBundle attestations. roots[i] is the data root of msgs[i].
type bundler struct {
	slot  primitives.Slot
	s     *slotAttestations
	byKey map[bundleKey]*ethpb.AttestationBundle
	msgs  []waggleMessage
	roots [][32]byte
}

func newBundler(slot primitives.Slot, s *slotAttestations) *bundler {
	return &bundler{slot: slot, s: s, byKey: make(map[bundleKey]*ethpb.AttestationBundle)}
}

func (b *bundler) add(vt vote) {
	key := bundleKey{committee: b.s.votes[vt], dataRoot: vt.root}
	wb := b.s.bundles[key]
	cur := b.byKey[key]
	if cur == nil || len(cur.AttesterIndices) >= maxAttestationsPerBundle {
		cur = &ethpb.AttestationBundle{CommitteeIndex: key.committee, AttestationData: wb.data}
		b.byKey[key] = cur
		b.msgs = append(b.msgs, waggleMessage{Slot: b.slot, Bundle: cur})
		b.roots = append(b.roots, vt.root)
	}
	cur.AttesterIndices = append(cur.AttesterIndices, uint64(vt.v))
	cur.Signatures = append(cur.Signatures, wb.sigs[vt.v])
}

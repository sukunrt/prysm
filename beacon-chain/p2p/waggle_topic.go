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

// slotAttestations holds our set of votes for one slot and the reconciliation state with each
// peer. It expires when the attestations of the slot are no longer valid for gossip.
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
		sp = &slotPeer{}
		s.peers[p] = sp
	}
	return sp
}

func (s *slotAttestations) peerHas(p peer.ID, vt vote) bool {
	sp := s.peers[p]
	return sp != nil && sp.has(vt)
}

// slotPeer is our view of one peer in one slot. The messages to the peer are built from it when
// the peer has send capacity, so nothing waits in a queue and nothing is dropped.
type slotPeer struct {
	// known is our view of the peer's set: the votes sent by us, sent to us, or in its
	// available lists.
	known map[vote]struct{}
	// told is the peer's view of our set: the part of the slot order that our available lists
	// to the peer cover. Lists start at max(head, told).
	told int
	// asks holds the votes that we request from the peer and did not send the request for.
	asks []vote
	// requested holds the votes that the peer requested from us and that we did not send. It
	// holds only votes that we have, so our set limits it.
	requested map[vote]struct{}
}

func (sp *slotPeer) has(vt vote) bool {
	if sp == nil {
		return false
	}
	_, ok := sp.known[vt]
	return ok
}

// TODO: known has no limit. The indices are not checked against the committee, so a peer can
// fill known with junk in each live slot. Limit it to MAX_VALIDATORS_PER_COMMITTEE for each peer
// and slot, and use peer scoring.
func (sp *slotPeer) markKnown(vt vote) {
	if sp.known == nil {
		sp.known = make(map[vote]struct{})
	}
	sp.known[vt] = struct{}{}
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
//
// Logically, the topic reconciles its set of votes with all its peers, for each live slot:
//
//   - Universe: <committee-defining state, committee>. The state of the target checkpoint fixes
//     the shuffling, so it fixes the committee members. A slot normally has one such state;
//     forks can add more.
//   - Elements: one vote for each committee member, identified by (data root, validator). A
//     second, different vote of a member is a double vote. We keep and send at most
//     maxVotesPerValidator votes of a member, so that the slashers get the evidence.
//
// The reconciliation has three parts:
//
//   - Push (push, pushTo): a new vote goes at once to PushPeers peers that do not have it.
//   - Announce (sendMetadata, availableDelta): this is a full-set exchange, made cheap. Each
//     peer gets the IDs of our votes as a delta: each vote once (slotPeer.told), only in
//     advertWindow after the vote arrives, and not if the peer has it (slotPeer.known). A peer
//     that we do not reach in advertWindow does not get these IDs from us, but it can get the
//     votes by push or from other peers.
//   - Difference (handleMetadata, request, serve, expireRequests): the receiver requests the
//     announced votes that it does not have, and the sender serves them. Each side runs the
//     same loop, so the difference goes in both directions.
//
// The lists are per (committee, data root). This key is finer than the universe: votes with
// different data roots and the same committee-defining state have the same committee members.
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

	slots map[primitives.Slot]*slotAttestations
	// slotOrder holds the keys of slots, oldest first.
	slotOrder []primitives.Slot
	// listsDue holds the peers that get our available lists when they have send capacity.
	listsDue map[peer.ID]struct{}

	// scratch is reused space for building messages. Only the run loop uses it.
	scratch struct {
		bundle ethpb.AttestationBundle
		meta   ethpb.CommitteeAttestationPartsMetadata
		root   [32]byte
		keys   []bundleKey
		ids    []uint64
		votes  []vote
		peers  []peer.ID
	}
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
		listsDue: make(map[peer.ID]struct{}),
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
		i, _ := slices.BinarySearch(t.slotOrder, slot)
		t.slotOrder = slices.Insert(t.slotOrder, i, slot)
	}
	return s
}

func (t *waggleAttTopic) handleIncoming(ctx context.Context, in waggleIncoming) {
	m := in.msg
	if m.Bundle != nil {
		t.handleBundle(ctx, in.from, m.Slot, m.Bundle, in.dataRoot)
	}
	if m.Metadata != nil {
		t.handleMetadata(in.from, m.Slot, m.Metadata)
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

// handleMetadata records the announced votes that we do not have as asks, and the requested
// votes that we have as replies. They go to the peer when it has send capacity. A request for a
// vote that we do not have is dropped.
func (t *waggleAttTopic) handleMetadata(from peer.ID, slot primitives.Slot, md *ethpb.CommitteeAttestationPartsMetadata) {
	s := t.slot(slot)
	sp := s.peer(from)
	root := bytesutil.ToBytes32(md.DataRoot)
	now := time.Now()
	for _, idx := range md.Available {
		vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
		sp.markKnown(vt)
		if s.has(vt) || s.full(vt.v) {
			continue
		}
		t.request(s, sp, vt, md.CommitteeIndex, from, now)
	}
	for _, idx := range md.Requests {
		vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
		if _, ok := s.votes[vt]; !ok || sp.has(vt) {
			continue
		}
		if sp.requested == nil {
			sp.requested = make(map[vote]struct{})
		}
		sp.requested[vt] = struct{}{}
	}
}

// request records an open request for vt to p, and adds vt to the asks of p. It returns false if
// vt has enough open requests or p was already asked.
func (t *waggleAttTopic) request(s *slotAttestations, sp *slotPeer, vt vote, committee primitives.CommitteeIndex, p peer.ID, now time.Time) bool {
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
	sp.asks = append(sp.asks, vt)
	return true
}

func (t *waggleAttTopic) send(p peer.ID, slot primitives.Slot, b *ethpb.AttestationBundle, md *ethpb.CommitteeAttestationPartsMetadata) {
	msg, err := waggleMessage{Slot: slot, Bundle: b, Metadata: md}.encode()
	if err != nil {
		log.WithError(err).Debug("Could not encode waggle message")
		return
	}
	t.topic.SendMessage(p, msg)
}

// sendList sends one metadata list. Exactly one of available and requests is set.
func (t *waggleAttTopic) sendList(p peer.ID, slot primitives.Slot, k bundleKey, available, requests []uint64) {
	md := &t.scratch.meta
	t.scratch.root = k.dataRoot
	md.CommitteeIndex, md.DataRoot, md.Available, md.Requests = k.committee, t.scratch.root[:], available, requests
	t.send(p, slot, nil, md)
}

// flush fills the free send slots of p, in this order: our requests, replies to its requests,
// available lists if they are due, and push. If p has no stream to us, its state goes, so that it
// starts fresh when it subscribes again.
func (t *waggleAttTopic) flush(p peer.ID) {
	free, err := t.topic.GetPeerCapacity(p)
	if errors.Is(err, waggle.ErrPeerNotFound) {
		delete(t.listsDue, p)
		for _, s := range t.slots {
			delete(s.peers, p)
		}
		return
	}
	if err != nil || free == 0 {
		return
	}
	free = t.sendAsks(p, free)
	free = t.sendReplies(p, free)
	if _, due := t.listsDue[p]; due {
		var done bool
		if free, done = t.sendLists(p, free); done {
			delete(t.listsDue, p)
		}
	}
	if free > 0 {
		t.pushTo(p, free)
	}
}

// sendAsks sends our requests to p, oldest slot first. The requests of a slot go together or wait.
func (t *waggleAttTopic) sendAsks(p peer.ID, free int) int {
	for _, slot := range t.slotOrder {
		s := t.slots[slot]
		sp := s.peers[p]
		if sp == nil || len(sp.asks) == 0 {
			continue
		}
		sp.asks = slices.DeleteFunc(sp.asks, func(vt vote) bool {
			_, ok := s.wants[vt]
			return !ok
		})
		keyOf := func(vt vote) (bundleKey, bool) {
			w, ok := s.wants[vt]
			if !ok {
				return bundleKey{}, false
			}
			return bundleKey{committee: w.committee, dataRoot: vt.root}, true
		}
		n := t.lists(sp.asks, keyOf, nil)
		if n > free {
			return free
		}
		t.lists(sp.asks, keyOf, func(k bundleKey, ids []uint64) { t.sendList(p, slot, k, nil, ids) })
		sp.asks = sp.asks[:0]
		free -= n
	}
	return free
}

// sendReplies sends the votes that p requested, oldest slot first, in up to free bundles.
func (t *waggleAttTopic) sendReplies(p peer.ID, free int) int {
	for _, slot := range t.slotOrder {
		if free == 0 {
			return 0
		}
		s := t.slots[slot]
		sp := s.peers[p]
		if sp == nil || len(sp.requested) == 0 {
			continue
		}
		votes := t.scratch.votes[:0]
		for vt := range sp.requested {
			if _, ok := s.votes[vt]; !ok || sp.has(vt) {
				delete(sp.requested, vt)
				continue
			}
			votes = append(votes, vt)
		}
		t.scratch.votes = votes
		t.bundleVotes(s, votes, func(b *ethpb.AttestationBundle, root [32]byte) bool {
			if free == 0 {
				return false
			}
			for _, idx := range b.AttesterIndices {
				vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
				delete(sp.requested, vt)
				sp.markKnown(vt)
			}
			t.send(p, slot, b, nil)
			free--
			return true
		})
	}
	return free
}

// sendLists sends our available lists to p, oldest slot first. The lists of a slot go together
// or wait. It returns true when all slots are done.
func (t *waggleAttTopic) sendLists(p peer.ID, free int) (int, bool) {
	for _, slot := range t.slotOrder {
		s := t.slots[slot]
		sp := s.peers[p]
		n := t.availableLists(s, sp, nil)
		if n > free {
			return free, false
		}
		if n == 0 {
			continue
		}
		t.availableLists(s, sp, func(k bundleKey, ids []uint64) { t.sendList(p, slot, k, ids, nil) })
		free -= n
		if sp == nil {
			sp = s.peer(p)
		}
		sp.told = len(s.order)
	}
	return free, true
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

// pushTo sends up to free bundles of pending votes to p, oldest slot first.
func (t *waggleAttTopic) pushTo(p peer.ID, free int) int {
	for _, slot := range t.slotOrder {
		if free == 0 {
			return 0
		}
		s := t.slots[slot]
		if len(s.pending) == 0 {
			continue
		}
		sp := s.peers[p]
		votes := t.scratch.votes[:0]
		for _, vt := range s.pending {
			if s.sends[vt] < t.cfg.PushPeers && !sp.has(vt) {
				votes = append(votes, vt)
			}
		}
		t.scratch.votes = votes
		if len(votes) == 0 {
			continue
		}
		if sp == nil {
			sp = s.peer(p)
		}
		t.bundleVotes(s, votes, func(b *ethpb.AttestationBundle, root [32]byte) bool {
			if free == 0 {
				return false
			}
			for _, idx := range b.AttesterIndices {
				vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
				s.sends[vt]++
				sp.markKnown(vt)
			}
			t.send(p, slot, b, nil)
			free--
			return true
		})
	}
	return free
}

// compactPending drops the attestations that went to pushPeers peers.
func (s *slotAttestations) compactPending(pushPeers int) {
	s.pending = slices.DeleteFunc(s.pending, func(vt vote) bool { return s.sends[vt] >= pushPeers })
}

// sendMetadata makes our available lists due for MetadataPeers random peers with capacity. The
// lists are deltas: a peer gets each vote at most once, and only in the advert window.
func (t *waggleAttTopic) sendMetadata() {
	peers := t.topic.PeersWithCapacity()
	t.rng.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	for _, p := range peers[:min(t.cfg.MetadataPeers, len(peers))] {
		t.listsDue[p] = struct{}{}
		t.flush(p)
	}
}

// availableLists calls emit for each available list to the peer: the votes that we got after the
// last list to the peer and in the advert window, without the ones that the peer is known to
// have. It returns the number of lists. sp can be nil.
func (t *waggleAttTopic) availableLists(s *slotAttestations, sp *slotPeer, emit func(k bundleKey, ids []uint64)) int {
	from := s.head
	if sp != nil {
		from = max(from, sp.told)
	}
	if from >= len(s.order) {
		return 0
	}
	return t.lists(s.order[from:], func(vt vote) (bundleKey, bool) {
		if sp.has(vt) {
			return bundleKey{}, false
		}
		c, ok := s.votes[vt]
		return bundleKey{committee: c, dataRoot: vt.root}, ok
	}, emit)
}

// lists groups votes by keyOf into lists of at most maxIndicesPerMetadata validator indices, one
// key after the other, and calls emit for each list. emit can be nil. ids is valid only during the
// call. It returns the number of lists.
func (t *waggleAttTopic) lists(votes []vote, keyOf func(vote) (bundleKey, bool), emit func(k bundleKey, ids []uint64)) int {
	keys := t.scratch.keys[:0]
	for _, vt := range votes {
		if k, ok := keyOf(vt); ok && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	t.scratch.keys = keys
	n := 0
	for _, k := range keys {
		ids := t.scratch.ids[:0]
		for _, vt := range votes {
			if kk, ok := keyOf(vt); !ok || kk != k {
				continue
			}
			ids = append(ids, uint64(vt.v))
			if len(ids) == maxIndicesPerMetadata {
				n++
				if emit != nil {
					emit(k, ids)
				}
				ids = ids[:0]
			}
		}
		if len(ids) > 0 {
			n++
			if emit != nil {
				emit(k, ids)
			}
		}
		t.scratch.ids = ids
	}
	return n
}

// bundleVotes groups the votes of s by committee and data root into bundles of at most
// maxAttestationsPerBundle, and calls send for each bundle. send returns false to stop. The
// bundle is scratch space, valid only during the call. Votes that are not in s.votes are skipped.
func (t *waggleAttTopic) bundleVotes(s *slotAttestations, votes []vote, send func(b *ethpb.AttestationBundle, root [32]byte) bool) {
	keys := t.scratch.keys[:0]
	for _, vt := range votes {
		c, ok := s.votes[vt]
		if k := (bundleKey{committee: c, dataRoot: vt.root}); ok && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	t.scratch.keys = keys
	b := &t.scratch.bundle
	for _, k := range keys {
		wb := s.bundles[k]
		b.CommitteeIndex, b.AttestationData = k.committee, wb.data
		b.AttesterIndices, b.Signatures = b.AttesterIndices[:0], b.Signatures[:0]
		for _, vt := range votes {
			if c, ok := s.votes[vt]; !ok || c != k.committee || vt.root != k.dataRoot {
				continue
			}
			b.AttesterIndices = append(b.AttesterIndices, uint64(vt.v))
			b.Signatures = append(b.Signatures, wb.sigs[vt.v])
			if len(b.AttesterIndices) == maxAttestationsPerBundle {
				if !send(b, k.dataRoot) {
					return
				}
				b.AttesterIndices, b.Signatures = b.AttesterIndices[:0], b.Signatures[:0]
			}
		}
		if len(b.AttesterIndices) > 0 && !send(b, k.dataRoot) {
			return
		}
	}
}

// expireRequests drops open requests that are older than RequestTimeout, and asks other
// peers that announced the attestation.
// TODO: peer scoring. Peers that do not serve the attestations that they announced are at fault.
func (t *waggleAttTopic) expireRequests() {
	now := time.Now()
	asked := t.scratch.peers[:0]
	for _, s := range t.slots {
		for vt, w := range s.wants {
			w.open = slices.DeleteFunc(w.open, func(r openRequest) bool { return now.Sub(r.at) >= t.cfg.RequestTimeout })
			for p, sp := range s.peers {
				if len(w.open) >= t.cfg.RequestPeers {
					break
				}
				if !sp.has(vt) || !t.request(s, sp, vt, w.committee, p, now) {
					continue
				}
				if !slices.Contains(asked, p) {
					asked = append(asked, p)
				}
			}
			if len(w.open) == 0 {
				delete(s.wants, vt)
			}
		}
	}
	t.scratch.peers = asked
	for _, p := range asked {
		t.flush(p)
	}
}

// prune drops the expired slots.
func (t *waggleAttTopic) prune() {
	live := t.oldestLive()
	for slot := range t.slots {
		if slot < live {
			delete(t.slots, slot)
		}
	}
	t.slotOrder = slices.DeleteFunc(t.slotOrder, func(slot primitives.Slot) bool { return slot < live })
}

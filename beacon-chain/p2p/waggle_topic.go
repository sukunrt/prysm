package p2p

import (
	"bytes"
	"context"
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

// vote identifies an attestation in a slot. A validator can have more than one vote in a slot
// (a double vote), so a vote is the data root and the validator.
type vote struct {
	root [32]byte
	v    primitives.ValidatorIndex
}

// voteState is our state of one vote. A vote is wanted (a peer announced it and we do not have
// it), in validation (running > 0), or validated (sig is set). tombstoneAfter after its arrival,
// a validated vote moves to slotAttestations.tombstones.
type voteState struct {
	// sig is the signature of the validated vote. It is nil until the vote passes validation.
	sig []byte
	// running counts the validation jobs of the vote.
	running int
	// attempts is the dedup before validation: the signatures in validation and the ones that did
	// not pass. After validation, sig and tombstones dedup.
	// TODO: the attempts that did not pass stay until the slot expires, so a peer can fill them
	// with bad signatures. Peer scoring must limit this. Gossipsub has the same problem.
	attempts []attempt
	// sends counts the peers that we pushed the vote to.
	sends int
	// asked holds our requests for the vote: the peers, and when we sent the requests. A request
	// is open for RequestTimeout.
	asked []openRequest
}

func (vs *voteState) validated() bool {
	return vs != nil && vs.sig != nil
}

func (vs *voteState) attempt(committee primitives.CommitteeIndex, sig []byte) *attempt {
	if vs == nil {
		return nil
	}
	for i := range vs.attempts {
		if a := &vs.attempts[i]; a.committee == committee && bytes.Equal(a.sig, sig) {
			return a
		}
	}
	return nil
}

func (vs *voteState) open(now time.Time, timeout time.Duration) int {
	n := 0
	for _, r := range vs.asked {
		if now.Sub(r.at) < timeout {
			n++
		}
	}
	return n
}

func (vs *voteState) askedPeer(p peer.ID) bool {
	return slices.ContainsFunc(vs.asked, func(r openRequest) bool { return r.peer == p })
}

// attempt is one signature of a vote that went to validation. first and others are the peers
// that sent it. Most attempts have one sender, so first does not allocate.
type attempt struct {
	committee primitives.CommitteeIndex
	sig       []byte
	rejected  bool
	first     peer.ID
	others    []peer.ID
}

func (a *attempt) addSender(p peer.ID) {
	if p != a.first && !slices.Contains(a.others, p) {
		a.others = append(a.others, p)
	}
}

type openRequest struct {
	peer peer.ID
	at   time.Time
}

// rootData is the attestation data of a data root, and its committee. A subnet has one
// committee in a slot, so the data root fixes the committee.
type rootData struct {
	data      *ethpb.AttestationData
	committee primitives.CommitteeIndex
}

// slotAttestations holds our set of votes for one slot and the reconciliation state with each
// peer. It expires when the attestations of the slot are no longer valid for gossip.
type slotAttestations struct {
	// votes holds the votes that we want, validate, or have. It does not hold tombstoned votes.
	votes map[vote]*voteState
	// tombstones holds the validated votes that we no longer keep.
	tombstones map[vote]struct{}
	// roots holds the data roots that have a validated vote. We drop available lists for other
	// roots, because they can be junk.
	roots map[[32]byte]rootData
	// perValidator counts the validated votes of each validator, up to maxVotesPerValidator.
	perValidator map[primitives.ValidatorIndex]int
	window       activeWindow
	// pending holds the validated votes that went to fewer than PushPeers peers.
	pending map[vote]struct{}
	// wanted holds the wanted votes. expireRequests removes the votes that are no longer wanted.
	wanted map[vote]struct{}
	peers  map[peer.ID]*slotPeer
}

// activeWindow holds the validated votes of the last tombstoneAfter in the order we got them, and
// their arrival times. head is the first vote in the advert window.
type activeWindow struct {
	order []vote
	added []time.Time
	head  int
}

func (w *activeWindow) add(vt vote, now time.Time) {
	w.order = append(w.order, vt)
	w.added = append(w.added, now)
}

func (w *activeWindow) advert() []vote {
	return w.order[w.head:]
}

// age moves head past the votes older than advertWindow. It removes the votes older than
// tombstoneAfter and calls drop for each.
func (w *activeWindow) age(now time.Time, drop func(vote)) {
	for w.head < len(w.order) && now.Sub(w.added[w.head]) > advertWindow {
		w.head++
	}
	n := 0
	for n < w.head && now.Sub(w.added[n]) > tombstoneAfter {
		drop(w.order[n])
		n++
	}
	if n == len(w.order) {
		*w = activeWindow{}
		return
	}
	w.order = slices.Delete(w.order, 0, n)
	w.added = slices.Delete(w.added, 0, n)
	w.head -= n
}

func newSlotAttestations() *slotAttestations {
	return &slotAttestations{
		votes:        make(map[vote]*voteState),
		tombstones:   make(map[vote]struct{}),
		roots:        make(map[[32]byte]rootData),
		wanted:       make(map[vote]struct{}),
		pending:      make(map[vote]struct{}),
		perValidator: make(map[primitives.ValidatorIndex]int),
		peers:        make(map[peer.ID]*slotPeer),
	}
}

func (s *slotAttestations) tombstoned(vt vote) bool {
	_, ok := s.tombstones[vt]
	return ok
}

func (s *slotAttestations) validated(vt vote) bool {
	return s.votes[vt].validated() || s.tombstoned(vt)
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

// slotPeer is our view of one peer in one slot. The messages to the peer are built from it when
// the peer has send capacity, so nothing waits in a queue and nothing is dropped.
type slotPeer struct {
	// known is our view of the peer's set. true: the peer has the vote. It sent the vote to us,
	// announced it, or got it from us. false: the peer got the vote in our available lists, so it
	// can request it. We do not push or announce known votes to the peer.
	known map[vote]bool
	// requested holds the votes that the peer requested from us and that we did not send. It
	// holds only votes that we have, so our set limits it.
	requested map[vote]struct{}
}

func (sp *slotPeer) knows(vt vote) bool {
	if sp == nil {
		return false
	}
	_, ok := sp.known[vt]
	return ok
}

func (sp *slotPeer) has(vt vote) bool {
	return sp != nil && sp.known[vt]
}

// TODO: known has no limit. The indices are not checked against the committee, so a peer can
// fill known with junk in each live slot. Limit it to MAX_VALIDATORS_PER_COMMITTEE for each peer
// and slot, and use peer scoring.
func (sp *slotPeer) markHas(vt vote) {
	if sp.known == nil {
		sp.known = make(map[vote]bool)
	}
	sp.known[vt] = true
}

func (sp *slotPeer) markTold(vt vote) {
	if sp.known == nil {
		sp.known = make(map[vote]bool)
	}
	if _, ok := sp.known[vt]; !ok {
		sp.known[vt] = false
	}
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
//   - Announce (sendMetadata, availableVotes): this is a full-set exchange, made cheap. Each
//     peer gets the IDs of our votes as a delta: only in advertWindow after the vote arrives,
//     and not if the vote is in slotPeer.known (the peer has it, or got its ID from us). A peer
//     that we do not reach in advertWindow does not get these IDs from us, but it can get the
//     votes by push or from other peers.
//   - Difference (handleMetadata, sendAsks, sendReplies, expireRequests): the receiver requests
//     the announced votes that it does not have, and the sender serves them. Each side runs the
//     same loop, so the difference goes in both directions.
//
// The lists are per data root, which fixes the committee. This key is finer than the universe:
// votes with different data roots and the same committee-defining state have the same committee
// members.
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
		roots  [][32]byte
		ids    []uint64
		votes  []vote
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

// handleBundle drops validated votes, votes of validators with maxVotesPerValidator votes, and
// attempts that we saw, and sends the rest to validation. An attempt is recorded only when its
// job starts, so a dropped job can come again.
func (t *waggleAttTopic) handleBundle(ctx context.Context, from peer.ID, slot primitives.Slot, b *ethpb.AttestationBundle, root [32]byte) {
	s := t.slot(slot)
	sp := s.peer(from)
	for i, idx := range b.AttesterIndices {
		v := primitives.ValidatorIndex(idx)
		vt := vote{root: root, v: v}
		sp.markHas(vt)
		vs := s.votes[vt]
		if vs.validated() || s.tombstoned(vt) || s.full(v) {
			continue
		}
		if a := vs.attempt(b.CommitteeIndex, b.Signatures[i]); a != nil {
			// TODO: peer scoring. If a.rejected, from is at fault.
			a.addSender(from)
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
		if vs == nil {
			vs = &voteState{}
			s.votes[vt] = vs
		}
		vs.attempts = append(vs.attempts, attempt{committee: b.CommitteeIndex, sig: b.Signatures[i], first: from})
		vs.running++
	}
}

func (t *waggleAttTopic) handleValResult(r valResult) {
	s := t.slots[r.att.Data.Slot]
	if s == nil {
		return
	}
	vt := vote{root: r.dataRoot, v: r.att.AttesterIndex}
	vs := s.votes[vt]
	if vs == nil {
		return
	}
	vs.running--
	if r.res != pubsub.ValidationAccept {
		// TODO: peer scoring. On ValidationReject, all senders of the attempt are at fault, as in gossipsub.
		if a := vs.attempt(r.att.CommitteeId, r.att.Signature); a != nil {
			a.rejected = r.res == pubsub.ValidationReject
		}
		return
	}
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
	vs := s.votes[vt]
	if vs.validated() || s.tombstoned(vt) || s.full(vt.v) {
		return false
	}
	if vs == nil {
		vs = &voteState{}
		s.votes[vt] = vs
	}
	if _, ok := s.roots[root]; !ok {
		s.roots[root] = rootData{data: att.Data, committee: att.CommitteeId}
	}
	vs.sig, vs.attempts, vs.asked = att.Signature, nil, nil
	s.perValidator[vt.v]++
	s.window.add(vt, time.Now())
	s.pending[vt] = struct{}{}
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
	s.window.age(now, s.tombstone)
}

func (s *slotAttestations) tombstone(vt vote) {
	if !s.votes[vt].validated() {
		return
	}
	delete(s.votes, vt)
	delete(s.pending, vt)
	s.tombstones[vt] = struct{}{}
	for _, sp := range s.peers {
		delete(sp.known, vt)
	}
}

// handleMetadata records the announced votes that we do not have as wanted, and the requested
// votes that we have as replies. Requests and replies go to the peer when it has send capacity.
// It drops available lists for a data root without a validated vote: the first vote of a root
// comes by push. It drops requests for votes that we do not have.
func (t *waggleAttTopic) handleMetadata(from peer.ID, slot primitives.Slot, md *ethpb.CommitteeAttestationPartsMetadata) {
	s := t.slot(slot)
	sp := s.peer(from)
	root := bytesutil.ToBytes32(md.DataRoot)
	if _, ok := s.roots[root]; ok {
		for _, idx := range md.Available {
			vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
			sp.markHas(vt)
			vs := s.votes[vt]
			if vs.validated() || s.tombstoned(vt) || s.full(vt.v) {
				continue
			}
			if vs == nil {
				vs = &voteState{}
				s.votes[vt] = vs
			}
			s.wanted[vt] = struct{}{}
		}
	}
	for _, idx := range md.Requests {
		vt := vote{root: root, v: primitives.ValidatorIndex(idx)}
		if !s.votes[vt].validated() || sp.has(vt) {
			continue
		}
		if sp.requested == nil {
			sp.requested = make(map[vote]struct{})
		}
		sp.requested[vt] = struct{}{}
	}
}

// canAsk reports whether we can request vt from p: we do not have the vote or validate it, it
// has fewer than RequestPeers open requests, and we did not ask p for it.
func (t *waggleAttTopic) canAsk(s *slotAttestations, vt vote, p peer.ID, now time.Time) bool {
	vs := s.votes[vt]
	if vs == nil || vs.validated() || vs.running > 0 || s.full(vt.v) {
		return false
	}
	return vs.open(now, t.cfg.RequestTimeout) < t.cfg.RequestPeers && !vs.askedPeer(p)
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
func (t *waggleAttTopic) sendList(p peer.ID, slot primitives.Slot, s *slotAttestations, root [32]byte, available, requests []uint64) {
	md := &t.scratch.meta
	t.scratch.root = root
	md.CommitteeIndex, md.DataRoot, md.Available, md.Requests = s.roots[root].committee, t.scratch.root[:], available, requests
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

// sendAsks requests from p the wanted votes that p has, oldest slot first, in up to free lists. A
// request is open from when we send it.
func (t *waggleAttTopic) sendAsks(p peer.ID, free int) int {
	now := time.Now()
	for _, slot := range t.slotOrder {
		if free == 0 {
			return 0
		}
		s := t.slots[slot]
		sp := s.peers[p]
		if sp == nil || len(s.wanted) == 0 {
			continue
		}
		votes := t.scratch.votes[:0]
		for vt := range s.wanted {
			if sp.has(vt) && t.canAsk(s, vt, p, now) {
				votes = append(votes, vt)
			}
		}
		t.scratch.votes = votes
		t.lists(votes, func(root [32]byte, ids []uint64) bool {
			if free == 0 {
				return false
			}
			t.sendList(p, slot, s, root, nil, ids)
			for _, idx := range ids {
				vs := s.votes[vote{root: root, v: primitives.ValidatorIndex(idx)}]
				vs.asked = append(vs.asked, openRequest{peer: p, at: now})
			}
			free--
			return true
		})
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
			if !s.votes[vt].validated() || sp.has(vt) {
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
				sp.markHas(vt)
			}
			t.send(p, slot, b, nil)
			free--
			return true
		})
	}
	return free
}

// sendLists sends our available lists to p, oldest slot first, in up to free lists. It returns
// true when all lists are sent.
func (t *waggleAttTopic) sendLists(p peer.ID, free int) (int, bool) {
	for _, slot := range t.slotOrder {
		s := t.slots[slot]
		votes := t.availableVotes(s, s.peers[p])
		if len(votes) == 0 {
			continue
		}
		sp := s.peer(p)
		stopped := false
		t.lists(votes, func(root [32]byte, ids []uint64) bool {
			if free == 0 {
				stopped = true
				return false
			}
			t.sendList(p, slot, s, root, ids, nil)
			for _, idx := range ids {
				sp.markTold(vote{root: root, v: primitives.ValidatorIndex(idx)})
			}
			free--
			return true
		})
		if stopped {
			return 0, false
		}
	}
	return free, true
}

// push sends the pending attestations to peers with capacity, in random order. Each
// attestation goes to at most PushPeers peers that do not have it.
func (t *waggleAttTopic) push() {
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
		for vt := range s.pending {
			if s.votes[vt].sends < t.cfg.PushPeers && !sp.knows(vt) {
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
				vs := s.votes[vt]
				vs.sends++
				if vs.sends >= t.cfg.PushPeers {
					delete(s.pending, vt)
				}
				sp.markHas(vt)
			}
			t.send(p, slot, b, nil)
			free--
			return true
		})
	}
	return free
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

// availableVotes returns the votes for our available lists to the peer: the votes in the advert
// window that the peer does not know. sp can be nil. The result is scratch space.
func (t *waggleAttTopic) availableVotes(s *slotAttestations, sp *slotPeer) []vote {
	votes := t.scratch.votes[:0]
	for _, vt := range s.window.advert() {
		if !sp.knows(vt) {
			votes = append(votes, vt)
		}
	}
	t.scratch.votes = votes
	return votes
}

// lists groups votes by data root into lists of at most maxIndicesPerMetadata validator indices,
// one root after the other, and calls emit for each list. emit returns false to stop. ids is
// valid only during the call.
func (t *waggleAttTopic) lists(votes []vote, emit func(root [32]byte, ids []uint64) bool) {
	for _, root := range t.groupRoots(votes) {
		ids := t.scratch.ids[:0]
		for _, vt := range votes {
			if vt.root != root {
				continue
			}
			ids = append(ids, uint64(vt.v))
			if len(ids) == maxIndicesPerMetadata {
				if !emit(root, ids) {
					return
				}
				ids = ids[:0]
			}
		}
		t.scratch.ids = ids
		if len(ids) > 0 && !emit(root, ids) {
			return
		}
	}
}

// groupRoots returns the data roots of votes, in order. The result is scratch space.
func (t *waggleAttTopic) groupRoots(votes []vote) [][32]byte {
	roots := t.scratch.roots[:0]
	for _, vt := range votes {
		if !slices.Contains(roots, vt.root) {
			roots = append(roots, vt.root)
		}
	}
	t.scratch.roots = roots
	return roots
}

// bundleVotes groups the validated votes by data root into bundles of at most
// maxAttestationsPerBundle, and calls send for each bundle. send returns false to stop. The
// bundle is scratch space, valid only during the call.
func (t *waggleAttTopic) bundleVotes(s *slotAttestations, votes []vote, send func(b *ethpb.AttestationBundle, root [32]byte) bool) {
	b := &t.scratch.bundle
	for _, root := range t.groupRoots(votes) {
		rd := s.roots[root]
		b.CommitteeIndex, b.AttestationData = rd.committee, rd.data
		b.AttesterIndices, b.Signatures = b.AttesterIndices[:0], b.Signatures[:0]
		for _, vt := range votes {
			if vt.root != root {
				continue
			}
			vs := s.votes[vt]
			if !vs.validated() {
				continue
			}
			b.AttesterIndices = append(b.AttesterIndices, uint64(vt.v))
			b.Signatures = append(b.Signatures, vs.sig)
			if len(b.AttesterIndices) == maxAttestationsPerBundle {
				if !send(b, root) {
					return
				}
				b.AttesterIndices, b.Signatures = b.AttesterIndices[:0], b.Signatures[:0]
			}
		}
		if len(b.AttesterIndices) > 0 && !send(b, root) {
			return
		}
	}
}

// expireRequests removes from wanted the votes that we have, and the votes that no peer can
// serve: no open request, no validation job, and no peer that has the vote and that we did not
// ask. The next flush of a peer requests the other votes from it.
// TODO: peer scoring. Peers that do not serve the attestations that they announced are at fault.
func (t *waggleAttTopic) expireRequests() {
	now := time.Now()
	for _, s := range t.slots {
		for vt := range s.wanted {
			vs := s.votes[vt]
			if vs != nil && !vs.validated() && !s.full(vt.v) && t.servable(s, vt, vs, now) {
				continue
			}
			delete(s.wanted, vt)
			if vs != nil && !vs.validated() && vs.running == 0 && len(vs.attempts) == 0 {
				delete(s.votes, vt)
			}
		}
	}
}

func (t *waggleAttTopic) servable(s *slotAttestations, vt vote, vs *voteState, now time.Time) bool {
	if vs.running > 0 || vs.open(now, t.cfg.RequestTimeout) > 0 {
		return true
	}
	for p, sp := range s.peers {
		if sp.has(vt) && !vs.askedPeer(p) {
			return true
		}
	}
	return false
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

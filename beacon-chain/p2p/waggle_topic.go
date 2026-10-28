package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sukunrt/go-waggle/pb"
	"github.com/sukunrt/go-waggle/waggle"
)

type waggleAttConfig struct {
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
}

func defaultWaggleAttConfig() waggleAttConfig {
	return waggleAttConfig{
		PushPeers:        8,
		PushInterval:     20 * time.Millisecond,
		MetadataPeers:    2,
		MetadataInterval: 100 * time.Millisecond,
		RequestPeers:     2,
		RequestTimeout:   500 * time.Millisecond,
	}
}

// validateAttestationFn sends one attestation through the classic gossip validator.
type validateAttestationFn func(ctx context.Context, topic string, att *ethpb.SingleAttestation) bool

type waggleIncoming struct {
	from peer.ID
	msg  *pb.ObjectPieces
}

type bundleKey struct {
	committee primitives.CommitteeIndex
	dataRoot  [32]byte
}

type waggleBundle struct {
	data *ethpb.AttestationData
	sigs map[primitives.ValidatorIndex][]byte
}

type waggleSlot struct {
	// seen is the dedup before validation. The key includes the data and the signature.
	seen map[[32]byte]struct{}
	// validators holds the validated attestations. After validation, (slot, validator) is the dedup key.
	validators map[primitives.ValidatorIndex]bundleKey
	bundles    map[bundleKey]*waggleBundle
}

type wagglePeer struct {
	// known holds the attestations that the peer has: sent by us, sent to us, or in its available list.
	known map[primitives.Slot]map[primitives.ValidatorIndex]struct{}
}

type slotValidator struct {
	slot      primitives.Slot
	validator primitives.ValidatorIndex
}

type openRequest struct {
	peer peer.ID
	at   time.Time
}

// waggleAttTopic runs EIP-8334 on one attestation subnet topic. The run loop owns all state.
type waggleAttTopic struct {
	name     string
	topic    *waggle.Topic
	cfg      waggleAttConfig
	validate validateAttestationFn

	cancel context.CancelFunc
	wg     sync.WaitGroup

	incoming chan waggleIncoming
	local    chan *ethpb.SingleAttestation
	sendDone chan struct{}

	slots     map[primitives.Slot]*waggleSlot
	peers     map[peer.ID]*wagglePeer
	pushQueue []slotValidator
	requests  map[slotValidator][]openRequest
}

func newWaggleAttTopic(name string, cfg waggleAttConfig) *waggleAttTopic {
	return &waggleAttTopic{
		name:     name,
		cfg:      cfg,
		incoming: make(chan waggleIncoming, 64),
		local:    make(chan *ethpb.SingleAttestation, 64),
		sendDone: make(chan struct{}, 1),
		slots:    make(map[primitives.Slot]*waggleSlot),
		peers:    make(map[peer.ID]*wagglePeer),
		requests: make(map[slotValidator][]openRequest),
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

func (t *waggleAttTopic) receive(ctx context.Context) {
	for {
		from, msg := t.topic.ReceiveMessage()
		if msg == nil {
			return
		}
		select {
		case t.incoming <- waggleIncoming{from: from, msg: msg}:
		case <-ctx.Done():
			return
		}
	}
}

func (t *waggleAttTopic) run(ctx context.Context) {
	push := time.NewTicker(t.cfg.PushInterval)
	defer push.Stop()
	meta := time.NewTicker(t.cfg.MetadataInterval)
	defer meta.Stop()
	cleanup := time.NewTicker(params.BeaconConfig().SlotDuration())
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-t.incoming:
			t.handleIncoming(ctx, in)
		case att := <-t.local:
			t.handleLocal(att)
		case <-t.sendDone:
			t.push()
		case <-push.C:
			t.push()
			t.expireRequests()
		case <-meta.C:
			t.sendMetadata()
		case <-cleanup.C:
			t.prune()
		}
	}
}

func (t *waggleAttTopic) handleIncoming(ctx context.Context, in waggleIncoming) {
	m, err := decodeWaggleMessage(in.msg)
	if err != nil {
		log.WithError(err).WithField("peer", in.from).Debug("Could not decode waggle message")
		return
	}
	if m.Bundle != nil {
		t.handleBundle(ctx, in.from, m.Slot, m.Bundle)
	}
	if m.Metadata != nil {
		t.handleMetadata(in.from, m.Slot, m.Metadata)
	}
}

// handleBundle drops attestations of validated validators and seen keys, validates the
// rest, then stores them and queues them for push.
func (t *waggleAttTopic) handleBundle(context.Context, peer.ID, primitives.Slot, *ethpb.AttestationBundle) {
}

// handleMetadata requests the attestations in available that we do not have, and
// serves the requests from our store. A request that we cannot serve is dropped.
func (t *waggleAttTopic) handleMetadata(peer.ID, primitives.Slot, *ethpb.CommitteeAttestationPartsMetadata) {
}

// handleLocal stores an attestation that is already validated and queues it for push.
func (t *waggleAttTopic) handleLocal(*ethpb.SingleAttestation) {
}

// push sends each queued attestation to PushPeers peers from PeersWithCapacity that do not have it.
func (t *waggleAttTopic) push() {
}

// sendMetadata sends our available list to MetadataPeers random peers with capacity.
func (t *waggleAttTopic) sendMetadata() {
}

// expireRequests asks another holder for each request that is older than RequestTimeout.
func (t *waggleAttTopic) expireRequests() {
}

// prune removes the state of slots that are out of the propagation window.
func (t *waggleAttTopic) prune() {
}

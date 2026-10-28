package p2p

import (
	"context"
	"strings"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sukunrt/go-waggle/waggle"
)

// waggleAttestations holds one waggle topic per attestation subnet and fork digest.
// Topics stay open while their digest is in use, because a closed topic loses the
// streams that peers opened to us. Subnet membership uses Subscribe and Unsubscribe.
type waggleAttestations struct {
	ctx     context.Context
	w       *waggle.Waggle
	suffix  string
	cfg     waggleAttConfig
	mu      sync.Mutex
	digests map[[fieldparams.VersionLength]byte]bool
	topics  map[string]*waggleAttTopic
}

func newWaggleAttestations(ctx context.Context, w *waggle.Waggle, suffix string, cfg waggleAttConfig) *waggleAttestations {
	return &waggleAttestations{
		ctx:     ctx,
		w:       w,
		suffix:  suffix,
		cfg:     cfg,
		digests: make(map[[fieldparams.VersionLength]byte]bool),
		topics:  make(map[string]*waggleAttTopic),
	}
}

func isAttestationTopic(topic string) bool {
	return strings.Contains(topic, GossipAttestationMessage+"_")
}

// EnsureDigest makes the topics for all attestation subnets of the digest.
func (a *waggleAttestations) EnsureDigest(digest [fieldparams.VersionLength]byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.digests[digest] {
		return
	}
	a.digests[digest] = true
	for subnet := range attestationSubnetCount {
		topic := attestationToTopic(subnet, digest) + a.suffix
		t := newWaggleAttTopic(topic, a.cfg)
		wt, err := a.w.NewTopic(topic, 0, t.onSendComplete)
		if err != nil {
			log.WithError(err).WithField("topic", topic).Error("Could not create waggle topic")
			continue
		}
		t.topic = wt
		t.start(a.ctx)
		a.topics[topic] = t
	}
}

func (a *waggleAttestations) topic(topic string) *waggleAttTopic {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.topics[topic]
}

// Subscribe subscribes to the waggle topic. It does nothing for an unknown topic.
func (a *waggleAttestations) Subscribe(topic string) {
	if t := a.topic(topic); t != nil {
		t.topic.Subscribe()
	}
}

// Unsubscribe unsubscribes from the waggle topic. It does nothing for an unknown topic.
func (a *waggleAttestations) Unsubscribe(topic string) {
	if t := a.topic(topic); t != nil {
		t.topic.Unsubscribe()
	}
}

// Prune closes the topics of all digests that keep does not accept.
func (a *waggleAttestations) Prune(keep func([fieldparams.VersionLength]byte) bool) {
	var closed []*waggleAttTopic
	a.mu.Lock()
	for digest := range a.digests {
		if keep(digest) {
			continue
		}
		delete(a.digests, digest)
		for subnet := range attestationSubnetCount {
			topic := attestationToTopic(subnet, digest) + a.suffix
			if t, ok := a.topics[topic]; ok {
				closed = append(closed, t)
				delete(a.topics, topic)
			}
		}
	}
	a.mu.Unlock()
	for _, t := range closed {
		t.topic.Close()
		t.stop()
	}
}

// Close closes all topics.
func (a *waggleAttestations) Close() {
	a.Prune(func([fieldparams.VersionLength]byte) bool { return false })
}

// wantedDigests returns the digests in use at epoch: the current one, the next one
// in the epoch before a fork, and the previous one in the first epoch after a fork.
func wantedDigests(epoch primitives.Epoch) map[[fieldparams.VersionLength]byte]bool {
	current := params.GetNetworkScheduleEntry(epoch)
	wanted := map[[fieldparams.VersionLength]byte]bool{
		current.ForkDigest: true,
		params.GetNetworkScheduleEntry(epoch + 1).ForkDigest: true,
	}
	if current.Epoch != params.BeaconConfig().GenesisEpoch && epoch < current.Epoch+1 {
		wanted[params.GetNetworkScheduleEntry(current.Epoch-1).ForkDigest] = true
	}
	return wanted
}

func (s *Service) pruneWaggleAttestationTopics() {
	wanted := wantedDigests(slots.ToEpoch(slots.CurrentSlot(s.genesisTime)))
	s.waggleAtt.Prune(func(d [fieldparams.VersionLength]byte) bool { return wanted[d] })
}

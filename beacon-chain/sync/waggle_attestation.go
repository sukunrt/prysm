package sync

import (
	"context"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// validateWaggleAttestation validates one attestation from waggle with the gossip rules.
// If it is valid, it goes to the pool and to gossipsub, for the peers without waggle.
func (s *Service) validateWaggleAttestation(ctx context.Context, from peer.ID, topic string, att *eth.SingleAttestation) pubsub.ValidationResult {
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore
	}
	if digest, err := p2p.ExtractGossipDigest(topic); err != nil || digest != s.currentForkDigest() {
		return pubsub.ValidationIgnore
	}
	ctx, cancel := context.WithTimeout(ctx, pubsubMessageTimeout)
	defer cancel()

	attForPool, res, err := s.validateAttestation(ctx, from, att, topic)
	if res != pubsub.ValidationAccept {
		if res == pubsub.ValidationReject {
			log.WithError(err).WithField("peer", from).Debug("Rejected attestation from waggle")
		}
		return res
	}
	if err := s.committeeIndexBeaconAttestationSubscriber(ctx, attForPool); err != nil {
		log.WithError(err).Error("Could not save attestation from waggle")
	}
	subnet, err := attestationSubnetFromTopic(topic)
	if err != nil {
		log.WithError(err).WithField("topic", topic).Error("Could not get subnet of waggle topic")
		return pubsub.ValidationAccept
	}
	if err := s.cfg.p2p.BroadcastAttestation(ctx, subnet, att); err != nil {
		log.WithError(err).Debug("Could not broadcast attestation from waggle")
	}
	return pubsub.ValidationAccept
}

// attestationSubnetFromTopic returns the subnet of a beacon_attestation_{subnet} topic.
func attestationSubnetFromTopic(topic string) (uint64, error) {
	prefix := p2p.GossipAttestationMessage + "_"
	i := strings.Index(topic, prefix)
	if i < 0 {
		return 0, errors.New("not an attestation topic")
	}
	subnet, _, _ := strings.Cut(topic[i+len(prefix):], "/")
	return strconv.ParseUint(subnet, 10, 64)
}

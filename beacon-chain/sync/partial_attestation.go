package sync

import (
	"context"
	"time"

	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

// processPartialAttestation replays one bundled attestation through the
// gossip validator, pools it if accepted, and rebroadcasts it on classic
// gossip for non-partial peers. It reports whether the attestation was
// accepted; a non-nil error means it was rejected. received is when the
// bundle's RPC arrived.
func (s *Service) processPartialAttestation(
	topic string, att *eth.SingleAttestation, received time.Time,
) (bool, error) {
	ctx, cancel := context.WithTimeout(s.ctx, pubsubMessageTimeout)
	defer cancel()

	attForPool, subnet, result, err := s.validateGossipAttestation(
		ctx, att, topic, transportBundle, "", received)
	if result == pubsub.ValidationReject {
		if err == nil {
			err = errors.New("validation failed")
		}
		return false, errors.Wrap(err, "bundled attestation rejected")
	}
	if result != pubsub.ValidationAccept {
		// Ignored: already seen or not currently verifiable. Not an error.
		return false, nil
	}
	if err := s.committeeIndexBeaconAttestationSubscriber(ctx, attForPool); err != nil {
		log.WithError(err).Error("Could not save partial attestation to pool")
	}
	if err := s.cfg.p2p.BroadcastAttestation(ctx, subnet, att); err != nil {
		log.WithError(err).Warn("Could not rebroadcast partial attestation")
	}
	return true, nil
}

package sync

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const pendingAttsLimit = 32768

// aggregatorIndexFilter defines how aggregator index should be handled in equality checks.
type aggregatorIndexFilter int

const (
	// ignoreAggregatorIndex means aggregates differing only by aggregator index are considered equal.
	ignoreAggregatorIndex aggregatorIndexFilter = iota
	// includeAggregatorIndex means aggregator index must also match for aggregates to be considered equal.
	includeAggregatorIndex
)

// This method processes pending attestations as a "known" block as arrived. With validations,
// the valid attestations get saved into the operation mem pool, and the invalid attestations gets deleted
// from the sync pending pool.
func (s *Service) processPendingAttsForBlock(ctx context.Context, bRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "processPendingAttsForBlock")
	defer span.End()

	// Confirm that the pending attestation's missing block arrived and the node processed the block.
	if !s.cfg.beaconDB.HasBlock(ctx, bRoot) || !(s.cfg.beaconDB.HasState(ctx, bRoot) || s.cfg.beaconDB.HasStateSummary(ctx, bRoot)) || !s.cfg.chain.InForkchoice(bRoot) {
		return fmt.Errorf("could not process unknown block root %#x", bRoot)
	}

	// Before a node processes pending attestations queue, it verifies
	// the attestations in the queue are still valid. Attestations will
	// be deleted from the queue if invalid (i.e. getting stalled from falling too many slots behind).
	s.validatePendingAtts(ctx, s.cfg.clock.CurrentSlot())

	s.drainPendingAttsForBlock(ctx, bRoot)

	randGen := rand.NewGenerator()
	s.pendingAttsLock.Lock()
	pendingRoots := make([][32]byte, 0, len(s.blkRootToPendingAtts))
	s.pendingQueueLock.RLock()
	for r := range s.blkRootToPendingAtts {
		if !s.seenPendingBlocks[r] && !s.cfg.chain.InForkchoice(r) && !s.cfg.chain.BlockBeingSynced(r) {
			pendingRoots = append(pendingRoots, r)
		}
	}
	s.pendingQueueLock.RUnlock()
	s.pendingAttsLock.Unlock()

	//  Request the blocks for the pending attestations that could not be processed.
	return s.sendBatchRootRequest(ctx, pendingRoots, randGen)
}

// drainPendingAttsForBlock processes every attestation queued against the block
// root and removes the queue, repeating until nothing is left.
//
// Taking the batch and dropping the key in the same critical section is the
// point: processing an item verifies a signature and hands it to forkchoice, so
// a batch takes long enough that more votes for the same block arrive while it
// runs. Deleting the key afterwards, as this used to, threw those away
// unprocessed. An available attestation is gossiped once, during its own slot,
// so a vote dropped here never comes back and is simply missing from the
// Goldfish seat count for that slot.
func (s *Service) drainPendingAttsForBlock(ctx context.Context, bRoot [32]byte) {
	for {
		s.pendingAttsLock.Lock()
		attestations := s.blkRootToPendingAtts[bRoot]
		delete(s.blkRootToPendingAtts, bRoot)
		s.pendingAttsLock.Unlock()
		if len(attestations) == 0 {
			return
		}
		s.processAttestations(ctx, attestations)
	}
}

// processAttestations processes a list of attestations.
// It assumes (for logging purposes only) that all attestations pertain to the same block.
func (s *Service) processAttestations(ctx context.Context, attestations []any) {
	if len(attestations) == 0 {
		return
	}

	firstAttestation := attestations[0]
	var blockRoot []byte
	switch v := firstAttestation.(type) {
	case ethpb.Att:
		blockRoot = v.GetData().BeaconBlockRoot
	case ethpb.SignedAggregateAttAndProof:
		blockRoot = v.AggregateAttestationAndProof().AggregateVal().GetData().BeaconBlockRoot
	case *ethpb.AvailableAttestation:
		blockRoot = v.GetData().BeaconBlockRoot
	default:
		log.Warnf("Unexpected attestation type %T, skipping processing", v)
		return
	}

	validAggregates := make([]ethpb.SignedAggregateAttAndProof, 0, len(attestations))
	startAggregate := time.Now()
	atts := make([]ethpb.Att, 0, len(attestations))
	availableAtts := make([]*ethpb.AvailableAttestation, 0, len(attestations))
	aggregateAttAndProofCount := 0
	for _, att := range attestations {
		switch v := att.(type) {
		case ethpb.Att:
			atts = append(atts, v)
		case *ethpb.AvailableAttestation:
			availableAtts = append(availableAtts, v)
		case ethpb.SignedAggregateAttAndProof:
			aggregateAttAndProofCount++
			// Avoid processing multiple aggregates only differing by aggregator index.
			if slices.ContainsFunc(validAggregates, func(other ethpb.SignedAggregateAttAndProof) bool {
				return pendingAggregatesAreEqual(v, other, ignoreAggregatorIndex)
			}) {
				continue
			}

			if err := s.processAggregate(ctx, v); err != nil {
				log.WithError(err).Debug("Pending aggregate attestation could not be processed")
				continue
			}

			validAggregates = append(validAggregates, v)
		default:
			log.Warnf("Unexpected attestation type %T, skipping", v)
		}
	}
	durationAggregateAttAndProof := time.Since(startAggregate)

	startAtts := time.Now()
	for _, bucket := range bucketAttestationsByData(atts) {
		s.processAttestationBucket(ctx, bucket)
	}

	durationAtts := time.Since(startAtts)

	for _, att := range availableAtts {
		s.processAvailableAttestation(ctx, att)
	}

	log.WithFields(logrus.Fields{
		"blockRoot":                       fmt.Sprintf("%#x", blockRoot),
		"totalCount":                      len(attestations),
		"aggregateAttAndProofCount":       aggregateAttAndProofCount,
		"uniqueAggregateAttAndProofCount": len(validAggregates),
		"attCount":                        len(atts),
		"availableAttCount":               len(availableAtts),
		"durationTotal":                   durationAggregateAttAndProof + durationAtts,
		"durationAggregateAttAndProof":    durationAggregateAttAndProof,
		"durationAtts":                    durationAtts,
	}).Debug("Verified and saved pending attestations to pool")
}

// attestationBucket groups attestations with the same AttestationData for batch processing.
type attestationBucket struct {
	dataHash     [32]byte
	data         *ethpb.AttestationData
	attestations []ethpb.Att
}

// processAttestationBucket processes a bucket of attestations with shared AttestationData.
func (s *Service) processAttestationBucket(ctx context.Context, bucket *attestationBucket) {
	if bucket == nil || len(bucket.attestations) == 0 {
		return
	}

	data := bucket.data

	// Shared validations for the entire bucket.
	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(data.BeaconBlockRoot)) {
		log.WithError(blockchain.ErrNotDescendantOfFinalized).WithField("root", fmt.Sprintf("%#x", data.BeaconBlockRoot)).Debug("Failed forkchoice check for bucket")
		return
	}

	preState, err := s.cfg.chain.AttestationTargetState(ctx, data.Target)
	if err != nil {
		log.WithError(err).Debug("Failed to get attestation prestate for bucket")
		return
	}

	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, bucket.attestations[0]); err != nil {
		log.WithError(err).Debug("Failed FFG consistency check for bucket")
		return
	}

	// Collect valid attestations for both single and electra formats.
	// Broadcast takes single format but attestation pool and batch signature verification take electra format.
	forBroadcast := make([]ethpb.Att, 0, len(bucket.attestations))
	forPool := make([]ethpb.Att, 0, len(bucket.attestations))

	for _, att := range bucket.attestations {
		committee, err := helpers.BeaconCommitteeFromState(ctx, preState, data.Slot, att.GetCommitteeIndex())
		if err != nil {
			log.WithError(err).Debug("Failed to get committee from state")
			continue
		}

		valid, err := validateAttesterData(ctx, att, committee)
		if err != nil {
			log.WithError(err).Debug("Failed attester data validation")
			continue
		}
		if valid != pubsub.ValidationAccept {
			log.Debug("Pending attestation rejected due to invalid data")
			continue
		}

		var conv ethpb.Att
		if att.Version() >= version.Electra {
			single, ok := att.(*ethpb.SingleAttestation)
			if !ok {
				log.Debugf("Wrong type: expected %T, got %T", &ethpb.SingleAttestation{}, att)
				continue
			}
			conv = single.ToAttestationElectra(committee)
		} else {
			conv = att
		}

		forBroadcast = append(forBroadcast, att)
		forPool = append(forPool, conv)
	}

	if len(forPool) == 0 {
		return
	}

	verified := s.batchVerifyAttestationSignatures(ctx, forPool, preState)
	verifiedSet := make(map[ethpb.Att]struct{}, len(verified))
	for _, att := range verified {
		verifiedSet[att] = struct{}{}
	}

	for i, poolAtt := range forPool {
		if _, ok := verifiedSet[poolAtt]; ok {
			s.processVerifiedAttestation(ctx, forBroadcast[i], poolAtt, preState)
		}
	}
}

// batchVerifyAttestationSignatures attempts batch verification, with individual fallback on failure.
func (s *Service) batchVerifyAttestationSignatures(
	ctx context.Context,
	attestations []ethpb.Att,
	preState state.ReadOnlyBeaconState,
) []ethpb.Att {
	const fallbackMsg = "batch verification failed, using individual checks"

	set, err := blocks.AttestationSignatureBatch(ctx, preState, attestations)
	if err != nil {
		log.WithError(err).Debug(fallbackMsg)
		return s.fallbackToIndividualVerification(ctx, attestations, preState)
	}

	ok, err := set.Verify()
	if err != nil || !ok {
		if err != nil {
			log.WithError(err).Debug(fallbackMsg)
		} else {
			log.Debug(fallbackMsg)
		}
		return s.fallbackToIndividualVerification(ctx, attestations, preState)
	}

	return attestations
}

// fallbackToIndividualVerification verifies each attestation individually if batch verification fails.
func (s *Service) fallbackToIndividualVerification(
	ctx context.Context,
	attestations []ethpb.Att,
	preState state.ReadOnlyBeaconState,
) []ethpb.Att {
	verified := make([]ethpb.Att, 0, len(attestations))

	for _, att := range attestations {
		res, err := s.validateUnaggregatedAttWithState(ctx, att, preState)
		if err != nil {
			log.WithError(err).Debug("Individual signature verification error")
			continue
		}
		if res == pubsub.ValidationAccept {
			verified = append(verified, att)
		}
	}

	return verified
}

// saveAttestation saves an attestation to the appropriate pool.
func (s *Service) saveAttestation(att ethpb.Att) error {
	if features.Get().EnableExperimentalAttestationPool {
		return s.cfg.attestationCache.Add(att)
	}
	if att.IsAggregated() {
		return s.cfg.attPool.SaveAggregatedAttestation(att)
	}
	return s.cfg.attPool.SaveUnaggregatedAttestation(att)
}

// processVerifiedAttestation handles a signature-verified attestation.
func (s *Service) processVerifiedAttestation(
	ctx context.Context,
	broadcastAtt ethpb.Att,
	poolAtt ethpb.Att,
	preState state.ReadOnlyBeaconState,
) {
	data := broadcastAtt.GetData()

	if err := s.saveAttestation(poolAtt); err != nil {
		log.WithError(err).Debug("Failed to save unaggregated attestation")
		return
	}

	if key, err := generateUnaggregatedAttCacheKey(broadcastAtt); err != nil {
		log.WithError(err).Error("Failed to generate cache key for attestation tracking")
	} else {
		_ = s.setSeenUnaggregatedAtt(key)
	}

	valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(data.Slot))
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve active validator count")
		return
	}

	if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, broadcastAtt), broadcastAtt); err != nil {
		log.WithError(err).Debug("Failed to broadcast attestation")
	}

	var (
		eventType feed.EventType
		eventData any
	)

	switch {
	case broadcastAtt.Version() >= version.Electra:
		if sa, ok := broadcastAtt.(*ethpb.SingleAttestation); ok {
			eventType = operation.SingleAttReceived
			eventData = &operation.SingleAttReceivedData{Attestation: sa}
			break
		}
		fallthrough
	default:
		eventType = operation.UnaggregatedAttReceived
		eventData = &operation.UnAggregatedAttReceivedData{Attestation: broadcastAtt}
	}

	// Send event notification
	s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
		Type: eventType,
		Data: eventData,
	})
}

func (s *Service) processAggregate(ctx context.Context, aggregate ethpb.SignedAggregateAttAndProof) error {
	res, err := s.validateAggregatedAtt(ctx, aggregate)
	if err != nil {
		log.WithError(err).Debug("Pending aggregated attestation failed validation")
		return errors.Wrap(err, "validate aggregated att")
	}

	if res != pubsub.ValidationAccept || !s.validateBlockInAttestation(ctx, aggregate) {
		return errors.New("Pending aggregated attestation failed validation")
	}

	att := aggregate.AggregateAttestationAndProof().AggregateVal()
	if err := s.saveAttestation(att); err != nil {
		return errors.Wrap(err, "save attestation")
	}

	aggregatorIdx := aggregate.AggregateAttestationAndProof().GetAggregatorIndex()
	_ = s.setAggregatorIndexRoundSeen(slots.RoundAt(att.GetData().Slot), aggregatorIdx)

	if err := s.cfg.p2p.Broadcast(ctx, aggregate); err != nil {
		log.WithError(err).Debug("Could not broadcast aggregated attestation")
	}

	return nil
}

// This defines how pending aggregates are saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations/aggregates
// that voted for that block root. The caller of this function is responsible
// for not sending repeated aggregates to the pending queue.
func (s *Service) savePendingAggregate(agg ethpb.SignedAggregateAttAndProof) {
	root := bytesutil.ToBytes32(agg.AggregateAttestationAndProof().AggregateVal().GetData().BeaconBlockRoot)

	s.savePending(root, agg, func(other any) bool {
		a, ok := other.(ethpb.SignedAggregateAttAndProof)
		return ok && pendingAggregatesAreEqual(agg, a, includeAggregatorIndex)
	})
}

// This defines how pending attestations are saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations/aggregates
// that voted for that block root. The caller of this function is responsible
// for not sending repeated attestations to the pending queue.
func (s *Service) savePendingAtt(att ethpb.Att) {
	if att.Version() >= version.Electra && !att.IsSingle() {
		log.Debug("Non-single attestation sent to pending attestation pool. Attestation will be ignored")
		return
	}

	root := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)

	s.savePending(root, att, func(other any) bool {
		a, ok := other.(ethpb.Att)
		return ok && pendingAttsAreEqual(att, a)
	})
}

// savePendingAvailableAtt queues a Goldfish head vote whose block we have not
// seen yet, keyed by the missing block root. Without this the vote is lost for
// good: it is only ever gossiped once, during its own slot.
func (s *Service) savePendingAvailableAtt(att *ethpb.AvailableAttestation) {
	root := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)

	s.savePending(root, att, func(other any) bool {
		a, ok := other.(*ethpb.AvailableAttestation)
		return ok && pendingAvailableAttsAreEqual(att, a)
	})
}

// pendingAvailableAttsAreEqual reports whether two queued votes are the same
// vote. The seat bits name the signer, so a slot and bits match is the same
// validator voting for the same block root twice.
func pendingAvailableAttsAreEqual(a, b *ethpb.AvailableAttestation) bool {
	return a.GetData().Slot == b.GetData().Slot &&
		a.GetData().PayloadPresent == b.GetData().PayloadPresent &&
		bytes.Equal(a.GetAggregationBits(), b.GetAggregationBits())
}

// processAvailableAttestation replays a queued Goldfish head vote now that its
// block is known. The vote is verified exactly as the gossip validator would
// have verified it on arrival, then handed to forkchoice and re-broadcast so
// peers that also lacked the block get another chance to count it. Forkchoice
// counts a replay whose slot has already passed as a late vote.
func (s *Service) processAvailableAttestation(ctx context.Context, att *ethpb.AvailableAttestation) {
	arrived := time.Now()
	blockRoot := bytesutil.ToBytes32(att.GetData().BeaconBlockRoot)
	res, err := s.validateAvailableAttWithBlock(ctx, att, blockRoot, arrived)
	if res != pubsub.ValidationAccept {
		if res == pubsub.ValidationReject {
			s.dropVote(att, "signature", arrived)
		}
		// Otherwise validateAvailableAttWithBlock already named the reason.
		log.WithError(err).Debug("Pending available attestation was not accepted")
		return
	}
	if err := s.cfg.chain.ReceiveAvailableAttestation(ctx, att); err != nil {
		s.dropVote(att, "forkchoice", arrived)
		log.WithError(err).Debug("Could not record pending available attestation")
		return
	}
	s.logVote(att, voteReplayed, "", arrived)
	if err := s.cfg.p2p.Broadcast(ctx, att); err != nil {
		log.WithError(err).Debug("Could not broadcast pending available attestation")
	}
}

// We want to avoid saving duplicate items, which is the purpose of the passed-in closure.
// It is the responsibility of the caller to provide a function that correctly determines quality
// in the context of the pending queue.
func (s *Service) savePending(root [32]byte, pending any, isEqual func(other any) bool) {
	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	numOfPendingAtts := 0
	for _, v := range s.blkRootToPendingAtts {
		numOfPendingAtts += len(v)
	}
	// Exit early if we exceed the pending attestations limit.
	if numOfPendingAtts >= pendingAttsLimit {
		return
	}

	_, ok := s.blkRootToPendingAtts[root]
	if !ok {
		pendingAttCount.Inc()
		s.blkRootToPendingAtts[root] = []any{pending}
		return
	}

	// Skip if the attestation/aggregate from the same validator already exists in
	// the pending queue.
	if slices.ContainsFunc(s.blkRootToPendingAtts[root], isEqual) {
		return
	}

	pendingAttCount.Inc()
	s.blkRootToPendingAtts[root] = append(s.blkRootToPendingAtts[root], pending)
}

// pendingAggregatesAreEqual checks if two pending aggregate attestations are equal.
// The filter parameter controls whether aggregator index is considered in the equality check.
func pendingAggregatesAreEqual(a, b ethpb.SignedAggregateAttAndProof, filter aggregatorIndexFilter) bool {
	if a.Version() != b.Version() {
		return false
	}

	if filter == includeAggregatorIndex {
		if a.AggregateAttestationAndProof().GetAggregatorIndex() != b.AggregateAttestationAndProof().GetAggregatorIndex() {
			return false
		}
	}

	aAtt := a.AggregateAttestationAndProof().AggregateVal()
	bAtt := b.AggregateAttestationAndProof().AggregateVal()
	if aAtt.GetData().Slot != bAtt.GetData().Slot {
		return false
	}
	if aAtt.GetCommitteeIndex() != bAtt.GetCommitteeIndex() {
		return false
	}
	return bytes.Equal(aAtt.GetAggregationBits(), bAtt.GetAggregationBits())
}

func pendingAttsAreEqual(a, b ethpb.Att) bool {
	if a.Version() != b.Version() {
		return false
	}
	if a.GetData().Slot != b.GetData().Slot {
		return false
	}
	if a.Version() >= version.Electra {
		return a.GetAttestingIndex() == b.GetAttestingIndex()
	}
	if a.GetCommitteeIndex() != b.GetCommitteeIndex() {
		return false
	}
	return bytes.Equal(a.GetAggregationBits(), b.GetAggregationBits())
}

// This validates the pending attestations in the queue are still valid.
// If not valid, a node will remove it from the queue in place. The validity
// check specifies the pending attestation cannot fall one epoch behind
// the current slot.
func (s *Service) validatePendingAtts(ctx context.Context, slot primitives.Slot) {
	_, span := trace.StartSpan(ctx, "validatePendingAtts")
	defer span.End()

	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	for bRoot, atts := range s.blkRootToPendingAtts {
		for i := len(atts) - 1; i >= 0; i-- {
			var attSlot primitives.Slot
			switch t := atts[i].(type) {
			case ethpb.Att:
				attSlot = t.GetData().Slot
			case ethpb.SignedAggregateAttAndProof:
				attSlot = t.AggregateAttestationAndProof().AggregateVal().GetData().Slot
			case *ethpb.AvailableAttestation:
				attSlot = t.GetData().Slot
			default:
				log.Debugf("Unexpected item of type %T in pending attestation queue. Item will be removed", t)
				// Remove the pending attestation from the map in place.
				atts[i] = atts[len(atts)-1]
				atts = atts[:len(atts)-1]
				continue
			}
			if slot >= attSlot+params.BeaconConfig().SlotsPerEpoch {
				// Remove the pending attestation from the map in place.
				atts[i] = atts[len(atts)-1]
				atts = atts[:len(atts)-1]
			}
		}
		s.blkRootToPendingAtts[bRoot] = atts

		// If the pending attestations list of a given block root is empty,
		// a node will remove the key from the map to avoid dangling keys.
		if len(s.blkRootToPendingAtts[bRoot]) == 0 {
			delete(s.blkRootToPendingAtts, bRoot)
		}
	}
}

// bucketAttestationsByData groups attestations by their AttestationData hash.
func bucketAttestationsByData(attestations []ethpb.Att) map[[32]byte]*attestationBucket {
	bucketMap := make(map[[32]byte]*attestationBucket)

	for _, att := range attestations {
		data := att.GetData()
		dataHash, err := data.HashTreeRoot()
		if err != nil {
			log.WithError(err).Debug("Failed to hash attestation data, skipping attestation")
			continue
		}

		if bucket, ok := bucketMap[dataHash]; ok {
			bucket.attestations = append(bucket.attestations, att)
		} else {
			bucketMap[dataHash] = &attestationBucket{
				dataHash:     dataHash,
				data:         data,
				attestations: []ethpb.Att{att},
			}
		}
	}

	return bucketMap
}

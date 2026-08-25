package sync

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// Vote ledger outcomes. Every head vote a node sees ends on exactly one of
// them, so a run can reconcile a slot's committee seat by seat: the seats the
// committee schedule expects, minus the accepted ones, minus the dropped ones,
// are the seats that never arrived.
const (
	voteAccepted = "accepted" // handed to forkchoice off gossip
	voteReplayed = "replayed" // handed to forkchoice by the pending queue
	voteQueued   = "queued"   // parked until its block arrives; not yet counted
	voteDropped  = "dropped"  // discarded, see the reason field
)

// logVote writes one line for one head vote. arrived is when the vote entered
// validation, so the line carries both the arrival and the decision time as
// milliseconds into the vote's own slot: that is what says whether a vote was
// refused for being late or was simply never delivered.
//
// Off unless --goldfish-vote-ledger is set: the topic carries one message per
// seat holder per slot, so this is a run instrument, not a production log.
func (s *Service) logVote(
	att *ethpb.AvailableAttestation, outcome, reason string, arrived time.Time,
) {
	if !features.Get().GoldfishVoteLedger || att == nil || att.Data == nil {
		return
	}
	slot := att.Data.Slot
	start := slots.UnsafeStartTime(s.cfg.clock.GenesisTime(), slot)
	fields := logrus.Fields{
		"outcome":   outcome,
		"voteSlot":  slot,
		"seats":     att.AggregationBits.Count(),
		"blockRoot": fmt.Sprintf("%#x", bytesutil.ToBytes32(att.Data.BeaconBlockRoot)),
		"arrivedMs": arrived.Sub(start).Milliseconds(),
		"decidedMs": time.Since(start).Milliseconds(),
	}
	if reason != "" {
		fields["reason"] = reason
	}
	indices := decoupled.AvailableAttestationSeatsToValidatorIndices(
		slot, att.AggregationBits.BitIndices(), decoupled.CommitteeValidatorCount())
	if len(indices) == 1 {
		fields["validator"] = indices[0]
	}
	log.WithFields(fields).Info("Goldfish vote")
}

// logFFGVote writes one line for one FFG attestation that passed gossip
// validation. arrived is when the attestation entered validation, carried as
// milliseconds into the attestation's own slot: the same clock basis as the
// head-vote lines above, so both parse the same way.
//
// Off unless --goldfish-vote-ledger is set.
func (s *Service) logFFGVote(att ethpb.Att, arrived time.Time) {
	if !features.Get().GoldfishVoteLedger || att == nil {
		return
	}
	data := att.GetData()
	if data == nil || data.Target == nil {
		return
	}
	seats := uint64(1)
	if bits := att.GetAggregationBits(); bits != nil {
		seats = bits.Count()
	}
	start := slots.UnsafeStartTime(s.cfg.clock.GenesisTime(), data.Slot)
	fields := logrus.Fields{
		"outcome":        "gossip",
		"attSlot":        data.Slot,
		"targetRound":    data.Target.Epoch,
		"committeeIndex": att.GetCommitteeIndex(),
		"seats":          seats,
		"arrivedMs":      arrived.Sub(start).Milliseconds(),
		"blockRoot":      fmt.Sprintf("%#x", bytesutil.ToBytes32(data.BeaconBlockRoot)),
		"dataRoot":       decoupled.VoteLedgerDataRoot(att),
	}
	if att.IsSingle() {
		fields["validator"] = att.GetAttestingIndex()
	}
	log.WithFields(fields).Info("FFG vote")
}

// logFFGAggregate writes one line for one aggregate that passed validation on
// the aggregate topic. committee is the one the validation already resolved, so
// the aggregation bits are named without a second lookup. arrived carries the
// same clock basis as the FFG vote lines: milliseconds into the aggregate's own
// attestation slot.
//
// Off unless --goldfish-vote-ledger is set.
func (s *Service) logFFGAggregate(
	signed ethpb.SignedAggregateAttAndProof,
	committee []primitives.ValidatorIndex,
	arrived time.Time,
) {
	if !features.Get().GoldfishVoteLedger || signed == nil {
		return
	}
	aggregateAndProof := signed.AggregateAttestationAndProof()
	att := aggregateAndProof.AggregateVal()
	data := att.GetData()
	if data == nil || data.Target == nil {
		return
	}
	indices, err := attestation.AttestingIndices(att, committee)
	if err != nil {
		log.WithError(err).Debug("Could not name the seats of an FFG aggregate")
	}
	start := slots.UnsafeStartTime(s.cfg.clock.GenesisTime(), data.Slot)
	log.WithFields(logrus.Fields{
		"outcome":         "gossip",
		"attSlot":         data.Slot,
		"targetRound":     data.Target.Epoch,
		"committeeIndex":  att.GetCommitteeIndex(),
		"aggregatorIndex": aggregateAndProof.GetAggregatorIndex(),
		"seats":           att.GetAggregationBits().Count(),
		"arrivedMs":       arrived.Sub(start).Milliseconds(),
		"blockRoot":       fmt.Sprintf("%#x", bytesutil.ToBytes32(data.BeaconBlockRoot)),
		"dataRoot":        decoupled.VoteLedgerDataRoot(att),
		"validators":      decoupled.VoteLedgerValidators(indices),
	}).Info("FFG aggregate")
}

// dropVote counts and records a head vote the node is discarding. Every path
// that throws a vote away goes through here, so goldfish_vote_drop_total and
// the ledger can never disagree.
func (s *Service) dropVote(
	att *ethpb.AvailableAttestation, reason string, arrived time.Time,
) {
	availableAttDropCount.WithLabelValues(reason).Inc()
	s.logVote(att, voteDropped, reason, arrived)
}

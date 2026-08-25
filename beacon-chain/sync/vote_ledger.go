package sync

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	payloadattestation "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attestation"
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

// logDataColumn writes one line per data column sidecar the node takes in.
// outcome says how it arrived: "gossip" for one accepted off a column subnet,
// "local" for one the node built itself by reconstruction or from the execution
// client and never saw on gossip. arrived is milliseconds into the column's own
// slot, the same clock basis as the vote ledger's lines, so a run can ask how
// the 128 columns of a slot filled in relative to the votes cast on it.
//
// kzgCommitmentCount comes from the cell count: a column holds one cell per
// blob, and verification refuses a sidecar whose cells and commitments disagree,
// so this counts the blobs whether or not the Gloas bid commitments are attached.
//
// Off unless --goldfish-vote-ledger is set.
func (s *Service) logDataColumn(
	column blocks.VerifiedRODataColumn, outcome string, arrived time.Time,
) {
	if !features.Get().GoldfishVoteLedger {
		return
	}
	start := slots.UnsafeStartTime(s.cfg.clock.GenesisTime(), column.Slot())
	log.WithFields(logrus.Fields{
		"outcome":            outcome,
		"slot":               column.Slot(),
		"blockRoot":          fmt.Sprintf("%#x", column.BlockRoot()),
		"columnIndex":        column.Index(),
		"kzgCommitmentCount": len(column.Column()),
		"arrivedMs":          arrived.Sub(start).Milliseconds(),
	}).Info("Data column")
}

// logPTCVote writes one line per payload attestation the node takes in.
// outcome says how it arrived: "gossip" for one accepted off the PTC topic,
// including one replayed once its block landed, "local" for one this node's own
// PTC member published, which never traverses gossip. arrived is milliseconds
// into the vote's own slot, the same clock basis as the other ledger lines, so a
// run can line the PTC's verdict up against the payload it is voting on.
//
// Off unless --goldfish-vote-ledger is set.
func (s *Service) logPTCVote(
	pa payloadattestation.ROMessage, outcome string, arrived time.Time,
) {
	if !features.Get().GoldfishVoteLedger {
		return
	}
	start := slots.UnsafeStartTime(s.cfg.clock.GenesisTime(), pa.Slot())
	log.WithFields(logrus.Fields{
		"outcome":           outcome,
		"slot":              pa.Slot(),
		"blockRoot":         fmt.Sprintf("%#x", pa.BeaconBlockRoot()),
		"validatorIndex":    pa.ValidatorIndex(),
		"payloadPresent":    pa.PayloadPresent(),
		"blobDataAvailable": pa.BlobDataAvailable(),
		"arrivedMs":         arrived.Sub(start).Milliseconds(),
	}).Info("PTC vote")
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

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
	voteLocal    = "local"    // this node's own vote, handed to forkchoice by the RPC
)

// msIntoSlot is a time as milliseconds into the slot it belongs to. Negative
// for an arrival inside the early tolerance.
func msIntoSlot(genesis time.Time, slot primitives.Slot, t time.Time) float64 {
	return float64(t.Sub(slots.UnsafeStartTime(genesis, slot)).Milliseconds())
}

// recordVote takes in one head vote. arrived is when the vote entered
// validation, so the ledger line carries both the arrival and the decision time
// as milliseconds into the vote's own slot: that is what says whether a vote was
// refused for being late or was simply never delivered.
//
// The metrics are always on; the ledger line needs --goldfish-vote-ledger,
// because the topic carries one message per seat holder per slot.
func (s *Service) recordVote(
	att *ethpb.AvailableAttestation, outcome, reason string, arrived time.Time,
) {
	recordVote(s.cfg.clock.GenesisTime(), att, outcome, reason, arrived)
}

// RecordLocalVote takes in a head vote this node published itself. Own votes
// never reach the gossip subscriber, so the RPC that broadcasts one calls this
// instead.
func RecordLocalVote(genesis time.Time, att *ethpb.AvailableAttestation) {
	recordVote(genesis, att, voteLocal, "", time.Now())
}

func recordVote(
	genesis time.Time, att *ethpb.AvailableAttestation, outcome, reason string, arrived time.Time,
) {
	if att == nil || att.Data == nil {
		return
	}
	slot := att.Data.Slot
	// A queued vote is counted when it is replayed, a dropped one never.
	if outcome == voteAccepted || outcome == voteReplayed || outcome == voteLocal {
		goldfishVoteArrival.Observe(msIntoSlot(genesis, slot, arrived))
		voteSeats.add(slot, att.AggregationBits.Count())
	}
	if !features.Get().GoldfishVoteLedger {
		return
	}
	fields := logrus.Fields{
		"outcome":   outcome,
		"voteSlot":  slot,
		"seats":     att.AggregationBits.Count(),
		"blockRoot": fmt.Sprintf("%#x", bytesutil.ToBytes32(att.Data.BeaconBlockRoot)),
		"arrivedMs": int64(msIntoSlot(genesis, slot, arrived)),
		"decidedMs": int64(msIntoSlot(genesis, slot, time.Now())),
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

// recordFFGVote takes in one FFG attestation that passed gossip validation.
// arrived is when the attestation entered validation, carried as milliseconds
// into the attestation's own slot: the same clock basis as the head-vote lines
// above, so both parse the same way.
//
// The metric is always on; the ledger line needs --goldfish-vote-ledger.
func (s *Service) recordFFGVote(att ethpb.Att, arrived time.Time) {
	if att == nil {
		return
	}
	data := att.GetData()
	if data == nil || data.Target == nil {
		return
	}
	genesis := s.cfg.clock.GenesisTime()
	ffgVoteArrival.Observe(msIntoSlot(genesis, data.Slot, arrived))
	if !features.Get().GoldfishVoteLedger {
		return
	}
	seats := uint64(1)
	if bits := att.GetAggregationBits(); bits != nil {
		seats = bits.Count()
	}
	fields := logrus.Fields{
		"outcome":        "gossip",
		"attSlot":        data.Slot,
		"targetRound":    data.Target.Epoch,
		"committeeIndex": att.GetCommitteeIndex(),
		"seats":          seats,
		"arrivedMs":      int64(msIntoSlot(genesis, data.Slot, arrived)),
		"blockRoot":      fmt.Sprintf("%#x", bytesutil.ToBytes32(data.BeaconBlockRoot)),
		"dataRoot":       decoupled.VoteLedgerDataRoot(att),
	}
	if att.IsSingle() {
		fields["validator"] = att.GetAttestingIndex()
	}
	log.WithFields(fields).Info("FFG vote")
}

// recordFFGAggregate takes in one aggregate that passed validation on the
// aggregate topic. committee is the one the validation already resolved, so the
// aggregation bits are named without a second lookup. arrived carries the same
// clock basis as the FFG vote lines: milliseconds into the aggregate's own
// attestation slot.
//
// The metrics are always on; the ledger line needs --goldfish-vote-ledger.
func (s *Service) recordFFGAggregate(
	signed ethpb.SignedAggregateAttAndProof,
	committee []primitives.ValidatorIndex,
	arrived time.Time,
) {
	if signed == nil {
		return
	}
	aggregateAndProof := signed.AggregateAttestationAndProof()
	att := aggregateAndProof.AggregateVal()
	data := att.GetData()
	if data == nil || data.Target == nil {
		return
	}
	genesis := s.cfg.clock.GenesisTime()
	ffgAggregateArrival.Observe(msIntoSlot(genesis, data.Slot, arrived))
	if len(committee) > 0 {
		ffgAggregateSeatFraction.Observe(
			float64(att.GetAggregationBits().Count()) / float64(len(committee)))
	}
	if !features.Get().GoldfishVoteLedger {
		return
	}
	indices, err := attestation.AttestingIndices(att, committee)
	if err != nil {
		log.WithError(err).Debug("Could not name the seats of an FFG aggregate")
	}
	log.WithFields(logrus.Fields{
		"outcome":         "gossip",
		"attSlot":         data.Slot,
		"targetRound":     data.Target.Epoch,
		"committeeIndex":  att.GetCommitteeIndex(),
		"aggregatorIndex": aggregateAndProof.GetAggregatorIndex(),
		"seats":           att.GetAggregationBits().Count(),
		"arrivedMs":       int64(msIntoSlot(genesis, data.Slot, arrived)),
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
	log.WithFields(logrus.Fields{
		"outcome":            outcome,
		"slot":               column.Slot(),
		"blockRoot":          fmt.Sprintf("%#x", column.BlockRoot()),
		"columnIndex":        column.Index(),
		"kzgCommitmentCount": len(column.Column()),
		"arrivedMs":          int64(msIntoSlot(s.cfg.clock.GenesisTime(), column.Slot(), arrived)),
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
	log.WithFields(logrus.Fields{
		"outcome":           outcome,
		"slot":              pa.Slot(),
		"blockRoot":         fmt.Sprintf("%#x", pa.BeaconBlockRoot()),
		"validatorIndex":    pa.ValidatorIndex(),
		"payloadPresent":    pa.PayloadPresent(),
		"blobDataAvailable": pa.BlobDataAvailable(),
		"arrivedMs":         int64(msIntoSlot(s.cfg.clock.GenesisTime(), pa.Slot(), arrived)),
	}).Info("PTC vote")
}

// dropVote counts and records a head vote the node is discarding. Every path
// that throws a vote away goes through here, so goldfish_vote_drop_total and
// the ledger can never disagree.
func (s *Service) dropVote(
	att *ethpb.AvailableAttestation, reason string, arrived time.Time,
) {
	availableAttDropCount.WithLabelValues(reason).Inc()
	s.recordVote(att, voteDropped, reason, arrived)
}

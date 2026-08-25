package sync

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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

// dropVote counts and records a head vote the node is discarding. Every path
// that throws a vote away goes through here, so goldfish_vote_drop_total and
// the ledger can never disagree.
func (s *Service) dropVote(
	att *ethpb.AvailableAttestation, reason string, arrived time.Time,
) {
	availableAttDropCount.WithLabelValues(reason).Inc()
	s.logVote(att, voteDropped, reason, arrived)
}

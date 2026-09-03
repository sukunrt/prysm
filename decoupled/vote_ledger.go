package decoupled

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// LogLocalFFGVote records the node's own FFG attestation in the vote ledger.
// The gossip validator never sees the attestations this node publishes, so the
// submission paths (gRPC and REST) are the only places they can enter the run's
// ledger. Same line shape as the sync side, with outcome "local".
func LogLocalFFGVote(log logrus.FieldLogger, genesisTime time.Time, att ethpb.Att) {
	if !features.Get().GoldfishVoteLedger {
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
	start := slots.UnsafeStartTime(genesisTime, data.Slot)
	fields := logrus.Fields{
		"outcome":        "local",
		"attSlot":        data.Slot,
		"targetRound":    data.Target.Epoch,
		"committeeIndex": att.GetCommitteeIndex(),
		"seats":          seats,
		"arrivedMs":      time.Since(start).Milliseconds(),
		"blockRoot":      fmt.Sprintf("%#x", bytesutil.ToBytes32(data.BeaconBlockRoot)),
		"dataRoot":       VoteLedgerDataRoot(att),
	}
	if att.IsSingle() {
		fields["validator"] = att.GetAttestingIndex()
	}
	log.WithFields(fields).Info("FFG vote")
}

// VoteLedgerValidators renders validator indices as the goldfish vote ledger's
// validators field: a comma-separated list, no spaces, in the order given. Every
// ledger line that names seats goes through here so one parser reads them all.
func VoteLedgerValidators(indices []uint64) string {
	parts := make([]string, len(indices))
	for i, v := range indices {
		parts[i] = strconv.FormatUint(v, 10)
	}
	return strings.Join(parts, ",")
}

// VoteLedgerDataRoot renders the attestation pool's grouping key for att: two
// FFG votes can be aggregated together exactly when this value matches. Without
// it a run cannot tell a committee that agreed on one vote from a committee that
// split across several, because both look like the same count of arrivals.
//
// The pool's key starts with the attestation version, so the same vote renders
// differently once it is folded into an aggregate. The version byte is dropped
// here: a run has to join a seat's arrival line to the aggregate that carried
// it, and the remaining 31 bytes are the same for both. Dropping the byte rather
// than zeroing it also keeps all 8 characters of VoteLedgerRootPrefix meaningful.
//
// Returns the empty string when the key cannot be built, which keeps the ledger
// line rather than dropping it.
func VoteLedgerDataRoot(att ethpb.Att) string {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%#x", id[1:])
}

// VoteLedgerRootPrefix shortens a rendered root to its first 8 hex characters,
// without the 0x. Ledger fields that name several roots on one line use it; the
// full roots stay on the per-vote lines, so nothing is lost by shortening here.
func VoteLedgerRootPrefix(root string) string {
	root = strings.TrimPrefix(root, "0x")
	if len(root) > 8 {
		return root[:8]
	}
	return root
}

// SummaryPurpose is the value of the purpose field on every per-slot summary
// line. One grep on it pulls all four lines out of a node's log.
const SummaryPurpose = "goldfish-summary"

// SummaryFields is the field set every summary line starts from.
func SummaryFields(slot primitives.Slot) logrus.Fields {
	return logrus.Fields{"purpose": SummaryPurpose, "slot": slot}
}

// SummaryActive reports whether the given slot is at or after the Heze fork.
func SummaryActive(slot primitives.Slot) bool {
	return slots.ToEpoch(slot) >= params.BeaconConfig().HezeForkEpoch
}

// SummaryRoot renders the first 4 bytes of a root with the 0x prefix. Every
// summary line names roots this way, so the block and the payload line of one
// slot join on blockRoot.
func SummaryRoot(root [32]byte) string {
	return fmt.Sprintf("%#x", root[:4])
}

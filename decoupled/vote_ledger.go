package decoupled

import (
	"fmt"
	"strconv"
	"strings"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
)

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

package decoupled

import (
	"strconv"
	"strings"
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

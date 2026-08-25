package field_params_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// testFieldParametersMatchConfig asserts that the active chain config and the
// compiled-in field parameters agree. params.VerifyPreset is the same check the
// beacon node and the validator run at startup, so this exercises the real
// thing rather than a copy of its rules.
func testFieldParametersMatchConfig(t *testing.T) {
	require.NoError(t, params.VerifyPreset(params.BeaconConfig()))
}

//go:build decoupled

package params_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestE2EDecoupledConfigMatchesPreset pins the decoupled e2e config to the
// compiled-in decoupled field parameters: only non-preset-checked values may
// be overridden, or both e2e nodes refuse to start.
func TestE2EDecoupledConfigMatchesPreset(t *testing.T) {
	require.NoError(t, params.VerifyPreset(params.E2EDecoupledTestConfig()))
}

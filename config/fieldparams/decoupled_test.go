//go:build decoupled

package field_params_test

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestFieldParametersValues(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// OverrideBeaconConfig rather than SetActiveWithUndo: the decoupled config
	// reuses mainnet's fork-version schedule, which configset rejects.
	params.OverrideBeaconConfig(params.DecoupledConfig())
	require.Equal(t, "decoupled", fieldparams.Preset)
	testFieldParametersMatchConfig(t)
}

package heze_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// TestUpgradeToHeze checks the consensus-only fork bump: the fork field moves
// to the Heze version while the state keeps its pre-Heze shape and contents.
func TestUpgradeToHeze(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 4
	params.OverrideBeaconConfig(cfg)

	preState, _ := util.DeterministicGenesisStateGloas(t, 32)
	slot, err := slots.EpochStart(cfg.HezeForkEpoch)
	require.NoError(t, err)
	require.NoError(t, preState.SetSlot(slot))
	preVersion := preState.Fork().CurrentVersion
	preValidators := preState.NumValidators()

	post, err := heze.UpgradeToHeze(preState)
	require.NoError(t, err)

	require.DeepEqual(t, preVersion, post.Fork().PreviousVersion)
	require.DeepEqual(t, cfg.HezeForkVersion, post.Fork().CurrentVersion)
	require.Equal(t, cfg.HezeForkEpoch, post.Fork().Epoch)
	// Shape and contents are untouched: still a Gloas state.
	require.Equal(t, version.Gloas, post.Version())
	require.Equal(t, preValidators, post.NumValidators())
	require.Equal(t, slot, post.Slot())
}

// TestUpgradeToHeze_FromFulu covers the schedule without Gloas: the bump works
// from whatever the previous scheduled fork is.
func TestUpgradeToHeze_FromFulu(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 4
	params.OverrideBeaconConfig(cfg)

	preState, _ := util.DeterministicGenesisStateFulu(t, 32)
	slot, err := slots.EpochStart(cfg.HezeForkEpoch)
	require.NoError(t, err)
	require.NoError(t, preState.SetSlot(slot))
	preVersion := preState.Fork().CurrentVersion

	post, err := heze.UpgradeToHeze(preState)
	require.NoError(t, err)

	require.DeepEqual(t, preVersion, post.Fork().PreviousVersion)
	require.DeepEqual(t, cfg.HezeForkVersion, post.Fork().CurrentVersion)
	require.Equal(t, version.Fulu, post.Version())
}

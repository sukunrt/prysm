package params_test

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// presetConfig returns the chain config that matches the preset this test
// binary was compiled with, so the test works under every build tag.
func presetConfig(t *testing.T) *params.BeaconChainConfig {
	switch fieldparams.Preset {
	case "mainnet":
		return params.MainnetConfig().Copy()
	case "minimal":
		return params.MinimalSpecConfig()
	default:
		t.Fatalf("no chain config for preset %q", fieldparams.Preset)

		return nil
	}
}

func TestVerifyPreset(t *testing.T) {
	t.Run("config matching the compiled preset", func(t *testing.T) {
		require.NoError(t, params.VerifyPreset(presetConfig(t)))
	})

	t.Run("mismatched SLOTS_PER_EPOCH", func(t *testing.T) {
		cfg := presetConfig(t)
		// Halving SlotsPerEpoch also halves the two derived lengths, so the
		// error must name all three problems.
		cfg.SlotsPerEpoch /= 2

		err := params.VerifyPreset(cfg)
		require.ErrorContains(t, "SLOTS_PER_EPOCH", err)
		require.ErrorContains(t, "fieldparams.SlotsPerEpoch", err)
		require.ErrorContains(t, "fieldparams.Eth1DataVotesLength", err)
		require.ErrorContains(t, "fieldparams.PreviousEpochAttestationsLength", err)
		// The error has to say which preset the binary was built with.
		require.ErrorContains(t, fieldparams.Preset, err)
	})

	t.Run("mismatched vector length", func(t *testing.T) {
		cfg := presetConfig(t)
		cfg.EpochsPerHistoricalVector++

		err := params.VerifyPreset(cfg)
		require.ErrorContains(t, "EPOCHS_PER_HISTORICAL_VECTOR", err)
		require.ErrorContains(t, "fieldparams.RandaoMixesLength", err)
	})

	t.Run("a config from another preset is rejected", func(t *testing.T) {
		other := params.MainnetConfig().Copy()
		if fieldparams.Preset == "mainnet" {
			other = params.MinimalSpecConfig()
		}

		require.ErrorContains(t, "does not match", params.VerifyPreset(other))
	})
}

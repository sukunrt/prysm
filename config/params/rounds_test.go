package params_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestVerifyRounds(t *testing.T) {
	t.Run("shipped configs are the identity", func(t *testing.T) {
		for _, cfg := range []*params.BeaconChainConfig{
			params.MainnetConfig(), params.MinimalSpecConfig(), params.E2ETestConfig(),
		} {
			require.NoError(t, params.VerifyRounds(cfg))
			require.Equal(t, cfg.SlotsPerEpoch, cfg.SlotsPerRound, cfg.ConfigName)
		}
	})

	t.Run("a shorter round that divides the epoch is fine", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.SlotsPerRound = 8
		require.NoError(t, params.VerifyRounds(cfg))
	})

	t.Run("zero is rejected", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.SlotsPerRound = 0
		require.ErrorContains(t, "SLOTS_PER_ROUND=0", params.VerifyRounds(cfg))
	})

	t.Run("a round that does not divide the epoch is rejected", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.SlotsPerRound = 7
		require.ErrorContains(t, "does not divide SLOTS_PER_EPOCH=32", params.VerifyRounds(cfg))
	})
}

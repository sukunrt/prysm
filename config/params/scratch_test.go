package params_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestVerifyScratchSpace(t *testing.T) {
	t.Run("shipped configs are in bounds", func(t *testing.T) {
		for _, cfg := range []*params.BeaconChainConfig{
			params.MainnetConfig(), params.MinimalSpecConfig(), params.E2ETestConfig(),
		} {
			require.NoError(t, params.VerifyScratchSpace(cfg))
		}
	})

	t.Run("the maximum itself is accepted", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.ConsensusBlockScratchSpace = params.MaxScratchSpace
		cfg.GoldfishScratchSpace = params.MaxScratchSpace
		require.NoError(t, params.VerifyScratchSpace(cfg))
	})

	t.Run("an oversized block scratch space is rejected", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.ConsensusBlockScratchSpace = params.MaxScratchSpace + 1
		require.ErrorContains(t, "CONSENSUS_BLOCK_SCRATCH_SPACE=65537", params.VerifyScratchSpace(cfg))
	})

	t.Run("an oversized goldfish scratch space is rejected", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.GoldfishScratchSpace = params.MaxScratchSpace + 1
		require.ErrorContains(t, "GOLDFISH_SCRATCH_SPACE=65537", params.VerifyScratchSpace(cfg))
	})
}

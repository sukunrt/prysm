package params_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHezeShape(t *testing.T) {
	t.Run("gloas scheduled before heze", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.GloasForkEpoch = 2
		cfg.HezeForkEpoch = 4
		cfg.InitializeForkSchedule()
		entry := cfg.HezeShape()
		require.Equal(t, version.Gloas, entry.VersionEnum)
		require.DeepEqual(t, [4]byte(entry.ForkVersion), [4]byte(cfg.GloasForkVersion))
	})

	t.Run("gloas unscheduled falls back to fulu", func(t *testing.T) {
		cfg := params.MainnetConfig().Copy()
		cfg.FuluForkEpoch = 2
		cfg.GloasForkEpoch = cfg.FarFutureEpoch
		cfg.HezeForkEpoch = 6
		cfg.InitializeForkSchedule()
		entry := cfg.HezeShape()
		require.Equal(t, version.Fulu, entry.VersionEnum)
		require.DeepEqual(t, [4]byte(entry.ForkVersion), [4]byte(cfg.FuluForkVersion))
	})
}

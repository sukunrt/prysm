package main

import (
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// presetConfigs maps every preset in proto/ssz_proto_library.bzl to the chain
// config its SSZ sizes are derived from. Adding a preset without adding its
// config here fails TestPresetSizesMatchChainConfigs.
var presetConfigs = map[string]func() *params.BeaconChainConfig{
	"mainnet":   params.MainnetConfig,
	"minimal":   params.MinimalSpecConfig,
	"decoupled": params.DecoupledConfig,
}

// TestPresetSizesMatchChainConfigs pins every SSZ size in ssz_proto_library.bzl
// that is derived from a chain config value to its spec formula.
//
// The .bzl file is the only place several of these live: the Gloas sizes
// (proposer_lookahead_size, ptc_window.size, builder_pending_payments.size)
// have no Go constant to check them against, so nothing else in the repo would
// notice if a preset's arithmetic drifted.
func TestPresetSizesMatchChainConfigs(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	t.Chdir(root)

	ps, err := loadPresets()
	require.NoError(t, err)

	for _, name := range ps.names {
		t.Run(name, func(t *testing.T) {
			newCfg, ok := presetConfigs[name]
			require.Equal(t, true, ok, "preset "+name+" has no entry in presetConfigs")

			cfg := newCfg()
			slotsPerEpoch := uint64(cfg.SlotsPerEpoch)
			eth1Votes := slotsPerEpoch * uint64(cfg.EpochsPerEth1VotingPeriod)
			num := func(v uint64) string { return strconv.FormatUint(v, 10) }

			want := map[string]string{
				"eth1_data_votes.size":                num(eth1Votes),
				"previous_epoch_attestations.max":     num(slotsPerEpoch * cfg.MaxAttestations),
				"current_epoch_attestations.max":      num(slotsPerEpoch * cfg.MaxAttestations),
				"proposer_lookahead_size":             num((uint64(cfg.MinSeedLookahead) + 1) * slotsPerEpoch),
				"ptc_window.size":                     num((2 + uint64(cfg.MinSeedLookahead)) * slotsPerEpoch),
				"builder_pending_payments.size":       num(2 * slotsPerEpoch),
				"slashings.size":                      num(uint64(cfg.EpochsPerSlashingsVector)),
				"execution_payload_availability.size": num(uint64(cfg.SlotsPerHistoricalRoot) / 8),
				"block_roots.size":                    num(uint64(cfg.SlotsPerHistoricalRoot)) + ",32",
				"state_roots.size":                    num(uint64(cfg.SlotsPerHistoricalRoot)) + ",32",
				"randao_mixes.size":                   num(uint64(cfg.EpochsPerHistoricalVector)) + ",32",
				"sync_committee_bits.size":            num(cfg.SyncCommitteeSize),
				"max_committees_per_slot.size":        num(cfg.MaxCommitteesPerSlot),
			}

			dict := ps.dicts[name]
			for key, w := range want {
				got, ok := dict[key]
				require.Equal(t, true, ok, name+" is missing key "+key)
				require.Equal(t, w, got, name+"["+key+"]")
			}
		})
	}
}

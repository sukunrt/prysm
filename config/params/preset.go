package params

import (
	"fmt"
	"strings"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
)

// presetCheck ties one chain-config expression to the compile-time constant
// that must equal it.
type presetCheck struct {
	expr  string // chain-config expression, e.g. "SLOTS_PER_EPOCH * MAX_ATTESTATIONS"
	want  uint64 // its value in the running chain config
	konst string // name of the fieldparams constant derived from it
	got   uint64 // the value compiled into this binary
}

// VerifyPreset checks that cfg agrees with the SSZ sizes this binary was built
// with.
//
// The preset is fixed at compile time by the `minimal` build tag and
// determines every SSZ array length, while cfg can come from an arbitrary
// --chain-config-file. A disagreement does not crash anything: the node simply
// computes wrong hash tree roots and silently forks off. So it has to be a loud
// startup failure instead.
func VerifyPreset(cfg *BeaconChainConfig) error {
	slotsPerEpoch := uint64(cfg.SlotsPerEpoch)
	histRoot := uint64(cfg.SlotsPerHistoricalRoot)
	eth1Votes := slotsPerEpoch * uint64(cfg.EpochsPerEth1VotingPeriod)
	atts := slotsPerEpoch * cfg.MaxAttestations

	checks := []presetCheck{
		{"SLOTS_PER_EPOCH", slotsPerEpoch,
			"SlotsPerEpoch", fieldparams.SlotsPerEpoch},
		{"SLOTS_PER_HISTORICAL_ROOT", histRoot,
			"BlockRootsLength", fieldparams.BlockRootsLength},
		{"SLOTS_PER_HISTORICAL_ROOT", histRoot,
			"StateRootsLength", fieldparams.StateRootsLength},
		{"HISTORICAL_ROOTS_LIMIT", cfg.HistoricalRootsLimit,
			"HistoricalRootsLength", fieldparams.HistoricalRootsLength},
		{"EPOCHS_PER_HISTORICAL_VECTOR", uint64(cfg.EpochsPerHistoricalVector),
			"RandaoMixesLength", fieldparams.RandaoMixesLength},
		{"EPOCHS_PER_SLASHINGS_VECTOR", uint64(cfg.EpochsPerSlashingsVector),
			"SlashingsLength", fieldparams.SlashingsLength},
		{"VALIDATOR_REGISTRY_LIMIT", cfg.ValidatorRegistryLimit,
			"ValidatorRegistryLimit", fieldparams.ValidatorRegistryLimit},
		{"SYNC_COMMITTEE_SIZE", cfg.SyncCommitteeSize,
			"SyncCommitteeLength", fieldparams.SyncCommitteeLength},
		{"SLOTS_PER_EPOCH * EPOCHS_PER_ETH1_VOTING_PERIOD", eth1Votes,
			"Eth1DataVotesLength", fieldparams.Eth1DataVotesLength},
		{"SLOTS_PER_EPOCH * MAX_ATTESTATIONS", atts,
			"PreviousEpochAttestationsLength", fieldparams.PreviousEpochAttestationsLength},
		{"SLOTS_PER_EPOCH * MAX_ATTESTATIONS", atts,
			"CurrentEpochAttestationsLength", fieldparams.CurrentEpochAttestationsLength},
	}

	var bad []string
	for _, c := range checks {
		if c.want != c.got {
			bad = append(bad,
				fmt.Sprintf("%s=%d but fieldparams.%s=%d", c.expr, c.want, c.konst, c.got))
		}
	}

	if len(bad) == 0 {
		return nil
	}

	return fmt.Errorf(
		"chain config %q does not match the %q preset this binary was built with: %s; "+
			"rebuild with the build tag matching the chain config, or run a chain "+
			"config matching the %q preset",
		cfg.ConfigName, fieldparams.Preset, strings.Join(bad, "; "), fieldparams.Preset,
	)
}

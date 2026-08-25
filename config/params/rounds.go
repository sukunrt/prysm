package params

import "fmt"

// VerifyRounds checks that SLOTS_PER_ROUND is a usable round length.
//
// A round is the unit committees are reshuffled over, so it must be non-zero
// and must divide an epoch evenly; otherwise the last round of an epoch is
// short and its committees do not partition the active set. Neither failure
// crashes anything by itself - the node just computes committees nobody else
// agrees with - so it has to be a loud startup failure instead.
func VerifyRounds(cfg *BeaconChainConfig) error {
	if cfg.SlotsPerRound == 0 {
		return fmt.Errorf("chain config %q has SLOTS_PER_ROUND=0, it must be at least 1", cfg.ConfigName)
	}
	if cfg.SlotsPerEpoch%cfg.SlotsPerRound != 0 {
		return fmt.Errorf(
			"chain config %q has SLOTS_PER_ROUND=%d, which does not divide SLOTS_PER_EPOCH=%d",
			cfg.ConfigName, cfg.SlotsPerRound, cfg.SlotsPerEpoch,
		)
	}
	return nil
}

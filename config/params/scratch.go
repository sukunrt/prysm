package params

import "fmt"

// MaxScratchSpace bounds either scratch space field.
//
// The scratch bytes are meaningless padding for a stress test. The bound keeps
// a mistyped config from building a message the network drops: the block
// prefix and the vote field both stay far below MAX_PAYLOAD_SIZE at this size,
// and it matches the SSZ max on AvailableAttestation.scratch_space.
const MaxScratchSpace = 65536

// VerifyScratchSpace checks that both scratch space values are in bounds.
func VerifyScratchSpace(cfg *BeaconChainConfig) error {
	if cfg.ConsensusBlockScratchSpace > MaxScratchSpace {
		return fmt.Errorf(
			"chain config %q has CONSENSUS_BLOCK_SCRATCH_SPACE=%d, the maximum is %d",
			cfg.ConfigName, cfg.ConsensusBlockScratchSpace, MaxScratchSpace,
		)
	}
	if cfg.GoldfishScratchSpace > MaxScratchSpace {
		return fmt.Errorf(
			"chain config %q has GOLDFISH_SCRATCH_SPACE=%d, the maximum is %d",
			cfg.ConfigName, cfg.GoldfishScratchSpace, MaxScratchSpace,
		)
	}
	return nil
}

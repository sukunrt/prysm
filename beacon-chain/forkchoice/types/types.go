package types

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Checkpoint is an array version of ethpb.Checkpoint. It is used internally in
// forkchoice, while the slice version is used in the interface to legacy code
// in other packages.
//
// Epoch keeps its name to match the proto field it mirrors; its value is a
// Simplex ROUND (see plan/plan-finality-round.md).
type Checkpoint struct {
	Epoch primitives.Round
	Root  [fieldparams.RootLength]byte
}

// BlockAndCheckpoints to call the InsertOptimisticChain function
type BlockAndCheckpoints struct {
	Block               consensus_blocks.ROBlock
	JustifiedCheckpoint *ethpb.Checkpoint
	FinalizedCheckpoint *ethpb.Checkpoint
	HasPayload          bool
}

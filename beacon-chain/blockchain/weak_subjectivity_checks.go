package blockchain

import (
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

type weakSubjectivityDB interface {
	HasBlock(ctx context.Context, blockRoot [32]byte) bool
	IsFinalizedBlock(ctx context.Context, blockRoot [32]byte) bool
	BlockRoots(ctx context.Context, f *filters.QueryFilter) ([][32]byte, error)
}

type WeakSubjectivityVerifier struct {
	enabled  bool
	verified bool
	root     [32]byte
	round    primitives.Round
	slot     primitives.Slot
	db       weakSubjectivityDB
}

// NewWeakSubjectivityVerifier validates a checkpoint, and if valid, uses it to initialize a weak subjectivity verifier.
func NewWeakSubjectivityVerifier(wsc *ethpb.Checkpoint, db weakSubjectivityDB) (*WeakSubjectivityVerifier, error) {
	if wsc == nil || len(wsc.Root) == 0 || wsc.Epoch == 0 {
		log.Debug("--weak-subjectivity-checkpoint not provided")
		return &WeakSubjectivityVerifier{
			enabled: false,
		}, nil
	}
	startSlot, err := slots.RoundStart(wsc.Epoch)
	if err != nil {
		return nil, err
	}
	return &WeakSubjectivityVerifier{
		enabled:  true,
		verified: false,
		root:     bytesutil.ToBytes32(wsc.Root),
		round:    wsc.Epoch,
		db:       db,
		slot:     startSlot,
	}, nil
}

// VerifyWeakSubjectivity verifies the weak subjectivity root in the service struct.
// Reference design: https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/weak-subjectivity.md#weak-subjectivity-sync-procedure
func (v *WeakSubjectivityVerifier) VerifyWeakSubjectivity(ctx context.Context, finalizedRound primitives.Round) error {
	if v.verified || !v.enabled {
		return nil
	}
	// Two conditions are described in the specs (in rounds here, since checkpoints carry rounds):
	// IF round_number > store.finalized_checkpoint round,
	// then ASSERT during block sync that block with root block_root
	// is in the sync path at that round. Emit descriptive critical error if this assert fails,
	// then exit client process.
	// we do not handle this case ^, because we can only blocks that have been processed / are currently
	// in line for finalization, we don't have the ability to look ahead. so we only satisfy the following:
	// IF round_number <= store.finalized_checkpoint round,
	// then ASSERT that the block in the canonical chain at that round has root block_root.
	// Emit descriptive critical error if this assert fails, then exit client process.
	if v.round >= finalizedRound {
		return nil
	}
	log.Infof("Performing weak subjectivity check for root %#x in round %d", v.root, v.round)

	if !v.db.HasBlock(ctx, v.root) {
		return errors.Wrap(errWSBlockNotFound, fmt.Sprintf("missing root %#x", v.root))
	}
	if !v.db.IsFinalizedBlock(ctx, v.root) {
		return errors.Wrap(errWSBlockNotCanonical, fmt.Sprintf("root=%#x, round=%d", v.root, v.round))
	}
	endSlot := v.slot + params.BeaconConfig().SlotsPerRound - 1
	filter := filters.NewFilter().SetStartSlot(v.slot).SetEndSlot(endSlot)
	// A node should have the weak subjectivity block corresponds to the correct round in the DB.
	log.Infof("Searching block roots for weak subjectivity root=%#x, between slots %d-%d", v.root, v.slot, endSlot)
	roots, err := v.db.BlockRoots(ctx, filter)
	if err != nil {
		return errors.Wrap(err, "error while retrieving block roots to verify weak subjectivity")
	}
	if slices.Contains(roots, v.root) {
		log.Info("Weak subjectivity check has passed!!")
		v.verified = true
		return nil
	}
	return errors.Wrap(errWSBlockNotFoundInEpoch, fmt.Sprintf("root=%#x, round=%d", v.root, v.round))
}

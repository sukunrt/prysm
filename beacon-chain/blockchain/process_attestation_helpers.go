package blockchain

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// The caller of this function must have a lock on forkchoice.
func (s *Service) getRecentPreState(ctx context.Context, c *ethpb.Checkpoint) state.ReadOnlyBeaconState {
	headSlot := s.HeadSlot()
	headRound := slots.RoundAt(headSlot)
	if c.Epoch+1 < headRound || c.Epoch == 0 {
		return nil
	}
	// Only use head state if the head state is compatible with the target checkpoint.
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return nil
	}
	// The shuffling-compatibility check stays EPOCH-keyed: dependent roots are about
	// shuffling, not FFG. The checkpoint's own epoch is the epoch containing its round's
	// first slot, and we compare against the epoch just before the head's.
	headEpoch := slots.ToEpoch(headSlot)
	checkpointStart, err := slots.RoundStart(c.Epoch)
	if err != nil {
		return nil
	}
	if slots.ToEpoch(checkpointStart)+1 < headEpoch {
		return nil
	}
	headDependent, err := s.cfg.ForkChoiceStore.DependentRootForEpoch([32]byte(headRoot), headEpoch-1)
	if err != nil {
		return nil
	}
	targetDependent, err := s.cfg.ForkChoiceStore.DependentRootForEpoch([32]byte(c.Root), headEpoch-1)
	if err != nil {
		return nil
	}
	if targetDependent != headDependent {
		return nil
	}

	// If the head state alone is enough, we can return it directly read only.
	if c.Epoch <= headRound {
		st, err := s.HeadStateReadOnly(ctx)
		if err != nil {
			return nil
		}
		return st
	}
	// At this point we can only have c.Epoch > headRound.
	if !s.cfg.ForkChoiceStore.IsCanonical([32]byte(c.Root)) {
		return nil
	}
	// Advance the head state to the start of the target round.
	// This point can only be reached if c.Root == headRoot and c.Epoch > headRound.
	slot, err := slots.RoundStart(c.Epoch)
	if err != nil {
		return nil
	}
	// Try if we have already set the checkpoint cache. This will be tried again if we fail here but the check is cheap anyway.
	roundKey := strconv.FormatUint(uint64(c.Epoch), 10 /* base 10 */)
	lock := async.NewMultilock(string(c.Root) + roundKey)
	lock.Lock()
	defer lock.Unlock()
	cachedState, err := s.checkpointStateCache.StateByCheckpoint(c)
	if err != nil {
		return nil
	}
	if cachedState != nil && !cachedState.IsNil() {
		return cachedState
	}
	// If we haven't advanced yet then process the slots from head state.
	st, err := s.HeadState(ctx)
	if err != nil {
		return nil
	}
	st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, c.Root, slot)
	if err != nil {
		return nil
	}
	if err := s.checkpointStateCache.AddCheckpointState(c, st); err != nil {
		log.WithError(err).Error("Could not save checkpoint state to cache")
	}
	return st
}

// getAttPreState retrieves the att pre state by either from the cache or the DB.
// The caller of this function must have a lock on forkchoice.
func (s *Service) getAttPreState(ctx context.Context, c *ethpb.Checkpoint) (state.ReadOnlyBeaconState, error) {
	// If the attestation is recent and canonical we can use the head state to compute the shuffling.
	if st := s.getRecentPreState(ctx, c); st != nil {
		return st, nil
	}
	// Use a multilock to allow scoped holding of a mutex by a checkpoint root + round
	// allowing us to behave smarter in terms of how this function is used concurrently.
	roundKey := strconv.FormatUint(uint64(c.Epoch), 10 /* base 10 */)
	lock := async.NewMultilock(string(c.Root) + roundKey)
	lock.Lock()
	defer lock.Unlock()
	cachedState, err := s.checkpointStateCache.StateByCheckpoint(c)
	if err != nil {
		return nil, errors.Wrap(err, "could not get cached checkpoint state")
	}
	if cachedState != nil && !cachedState.IsNil() {
		return cachedState, nil
	}
	// Try the next slot cache for the early round calls, this should mostly have been covered already
	// but is cheap
	slot, err := slots.RoundStart(c.Epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute round start")
	}
	cachedState = transition.NextSlotState(c.Root, slot)
	if cachedState != nil && !cachedState.IsNil() {
		if cachedState.Slot() != slot {
			cachedState, err = transition.ProcessSlots(ctx, cachedState, slot)
			if err != nil {
				return nil, errors.Wrap(err, "could not process slots")
			}
		}
		if err := s.checkpointStateCache.AddCheckpointState(c, cachedState); err != nil {
			return nil, errors.Wrap(err, "could not save checkpoint state to cache")
		}
		return cachedState, nil
	}

	// Do not process attestations for old non viable checkpoints otherwise
	ok, err := s.cfg.ForkChoiceStore.IsViableForCheckpoint(&forkchoicetypes.Checkpoint{Root: [32]byte(c.Root), Epoch: c.Epoch})
	if err != nil {
		return nil, errors.Wrap(err, "could not check checkpoint condition in forkchoice")
	}
	if !ok {
		return nil, errors.Wrap(ErrNotCheckpoint, fmt.Sprintf("round %d root %#x", c.Epoch, c.Root))
	}

	// Fallback to state regeneration.
	log.WithFields(logrus.Fields{"round": c.Epoch, "root": fmt.Sprintf("%#x", c.Root)}).Debug("Regenerating attestation pre-state")
	baseState, err := s.cfg.StateGen.StateByRoot(ctx, bytesutil.ToBytes32(c.Root))
	if err != nil {
		return nil, errors.Wrapf(err, "could not get pre state for round %d", c.Epoch)
	}

	roundStartSlot, err := slots.RoundStart(c.Epoch)
	if err != nil {
		return nil, err
	}
	baseState, err = transition.ProcessSlotsIfPossible(ctx, baseState, roundStartSlot)
	if err != nil {
		return nil, errors.Wrapf(err, "could not process slots up to round %d", c.Epoch)
	}

	// Sharing the same state across caches is perfectly fine here, the fetching
	// of attestation prestate is by far the most accessed state fetching pattern in
	// the beacon node. An extra state instance cached isn't an issue in the bigger
	// picture.
	if err := s.checkpointStateCache.AddCheckpointState(c, baseState); err != nil {
		return nil, errors.Wrap(err, "could not save checkpoint state to cache")
	}
	return baseState, nil
}

// verifyAttTargetRound validates attestation is from the current or previous round.
func verifyAttTargetRound(_ context.Context, genesis, now time.Time, c *ethpb.Checkpoint) error {
	currentSlot := slots.At(genesis, now)
	currentRound := slots.RoundAt(currentSlot)
	var prevRound primitives.Round
	// Prevents previous round under flow
	if currentRound > 1 {
		prevRound = currentRound - 1
	}
	if c.Epoch != prevRound && c.Epoch != currentRound {
		return fmt.Errorf("target round %d does not match current round %d or prev round %d", c.Epoch, currentRound, prevRound)
	}
	return nil
}

// verifyBeaconBlock verifies beacon head block is known and not from the future.
func (s *Service) verifyBeaconBlock(ctx context.Context, data *ethpb.AttestationData) error {
	r := bytesutil.ToBytes32(data.BeaconBlockRoot)
	b, err := s.getBlock(ctx, r)
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return err
	}
	if b.Block().Slot() > data.Slot {
		return fmt.Errorf("could not process attestation for future block, block.Slot=%d > attestation.Data.Slot=%d", b.Block().Slot(), data.Slot)
	}
	return nil
}

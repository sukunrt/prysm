package helpers

import (
	"bytes"
	"context"
	"math"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// GenesisBlockRootReader is the minimal beacon DB surface needed to fetch the
// genesis block root for the spec's epoch < 2 fallback.
type GenesisBlockRootReader interface {
	GenesisBlockRoot(ctx context.Context) ([32]byte, error)
}

// ProposerDependentRootOrGenesis wraps state.ProposerDependentRoot with the
// spec-mandated genesis fallback: when that underflows (proposal epoch < 2) the
// dependent root is the genesis block root.
func ProposerDependentRootOrGenesis(ctx context.Context, db GenesisBlockRootReader, st state.ReadOnlyBeaconState, slot primitives.Slot) ([32]byte, error) {
	root, err := st.ProposerDependentRoot(slot)
	if !errors.Is(err, state.ErrProposerDependentRootUnderflow) {
		return root, err
	}
	if db == nil {
		return [32]byte{}, errors.New("genesis fallback required at epoch < 2 but db is nil")
	}
	genesisRoot, err := db.GenesisBlockRoot(ctx)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "genesis block root")
	}
	return genesisRoot, nil
}

// ParentTargetGasLimit returns the parent execution payload's gas limit, used
// as the payload-attributes fallback when the proposer has no signed
// preference. Falls back to DefaultBuilderGasLimit on pre-Gloas states or
// when no bid is cached (e.g. at genesis).
func ParentTargetGasLimit(st state.ReadOnlyBeaconState) uint64 {
	bid, err := st.LatestExecutionPayloadBid()
	if err != nil || bid == nil {
		// No cached bid (e.g. the gloas fork boundary): EL ratchets toward this
		// default, briefly nudging gas limit away from the parent's value.
		log.WithField("default", params.BeaconConfig().DefaultBuilderGasLimit).
			Debug("No parent execution payload bid; gas limit falls back to DefaultBuilderGasLimit")
		return params.BeaconConfig().DefaultBuilderGasLimit
	}
	return bid.GasLimit()
}

// BlockRootAtSlot returns the block root stored in the BeaconState for a recent slot.
// It returns an error if the requested block root is not within the slot range.
//
// Spec pseudocode definition:
//
//	def get_block_root_at_slot(state: BeaconState, slot: Slot) -> Root:
//	  """
//	  Return the block root at a recent ``slot``.
//	  """
//	  assert slot < state.slot <= slot + SLOTS_PER_HISTORICAL_ROOT
//	  return state.block_roots[slot % SLOTS_PER_HISTORICAL_ROOT]
func BlockRootAtSlot(state state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	if math.MaxUint64-slot < params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.New("slot overflows uint64")
	}
	if slot >= state.Slot() || state.Slot() > slot+params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.Errorf("slot %d out of bounds", slot)
	}
	return state.BlockRootAtIndex(uint64(slot % params.BeaconConfig().SlotsPerHistoricalRoot))
}

// StateRootAtSlot returns the cached state root at that particular slot. If no state
// root has been cached it will return a zero-hash.
func StateRootAtSlot(state state.ReadOnlyBeaconState, slot primitives.Slot) ([]byte, error) {
	if slot >= state.Slot() || state.Slot() > slot+params.BeaconConfig().SlotsPerHistoricalRoot {
		return []byte{}, errors.Errorf("slot %d out of bounds", slot)
	}
	return state.StateRootAtIndex(uint64(slot % params.BeaconConfig().SlotsPerHistoricalRoot))
}

// FFGTargetRoot returns the FFG target root for a round: the block root at
// FFG_TARGET_OFFSET_SLOTS slots before the round starts.
//
// This is the decoupled fork's replacement for the spec's get_block_root,
// which names the block at the epoch's first slot. At the default offset of 1
// a validator casting its finality vote at the start of the round's first slot
// has not seen that slot's block yet, so the target is shifted one slot back
// and every voter in the round names a block that already exists. Offset 0
// restores the spec shape, targeting the round's own first slot.
//
//	def get_ffg_target_root(state: BeaconState, round: Round) -> Root:
//	  slot = compute_start_slot_at_round(round)
//	  if slot < FFG_TARGET_OFFSET_SLOTS:
//	    return get_block_root_at_slot(state, GENESIS_SLOT)
//	  return get_block_root_at_slot(state, slot - FFG_TARGET_OFFSET_SLOTS)
//
// Round 0 has no earlier slot, so it names the anchor (genesis) block, which
// is what the unshifted rule returns there anyway.
func FFGTargetRoot(state state.ReadOnlyBeaconState, round primitives.Round) ([]byte, error) {
	s, err := slots.FFGTargetSlot(round)
	if err != nil {
		return nil, err
	}
	return BlockRootAtSlot(state, s)
}

// CheckpointEpoch converts a round-valued checkpoint index into the epoch that
// contains the round's first slot.
//
// It is one of only two sanctioned round<->epoch conversions (the other being
// the slots.RoundAt family); every other cross-unit cast is a bug.
func CheckpointEpoch(round primitives.Round) (primitives.Epoch, error) {
	s, err := slots.RoundStart(round)
	if err != nil {
		return 0, err
	}
	return slots.ToEpoch(s), nil
}

// EpochCheckpoint translates a round-valued checkpoint into the epoch-valued
// checkpoint standard beacon-API consumers expect.
//
// The epoch is taken from the checkpoint block's own slot (the round's FFG
// target, one slot before the round starts), not from the round's first slot:
// when a round boundary coincides with an epoch boundary the block at the
// epoch's first slot is the checkpoint block's child and can still reorg, so
// deriving from the target slot keeps the boundary root an ancestor-or-self of
// the checkpoint block at every FFG_TARGET_OFFSET_SLOTS. The cost is that on
// those rounds the advertised epoch lags by one until the next round.
//
// The pair is what a stock consumer reads as "everything before epoch*32 is
// justified/finalized". The raw round and round root stay available alongside
// it in the API responses that call this.
//
// Round 0 and the zero checkpoint pass through unchanged: at genesis there is
// no earlier boundary to name, and on configs where SlotsPerRound equals
// SlotsPerEpoch the translation is near-identity anyway.
func EpochCheckpoint(
	st state.ReadOnlyBeaconState,
	round primitives.Round,
	root []byte,
) (primitives.Epoch, []byte, error) {
	target, err := slots.FFGTargetSlot(round)
	if err != nil {
		return 0, nil, err
	}
	epoch := slots.ToEpoch(target)
	if epoch == 0 {
		return 0, root, nil
	}
	boundary, err := slots.EpochStart(epoch)
	if err != nil {
		return 0, nil, err
	}
	// BlockRootAtSlot's window is [st.Slot()-SlotsPerHistoricalRoot, st.Slot()),
	// strict on the upper end. When the boundary is not inside it, or the state
	// carries no root there, the checkpoint's own root is the newest root known
	// justified/finalized; it is a descendant of the boundary block, so naming it
	// never over-claims.
	boundaryRoot, err := BlockRootAtSlot(st, boundary)
	if err != nil || bytes.Equal(boundaryRoot, params.BeaconConfig().ZeroHash[:]) {
		return epoch, root, nil
	}
	return epoch, boundaryRoot, nil
}

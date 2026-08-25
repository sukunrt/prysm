package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/client/beacon"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestService_VerifyWeakSubjectivityRoot(t *testing.T) {
	b := util.NewBeaconBlock()
	b.Block.Slot = 1792480
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	forkBlock := util.NewBeaconBlock()
	forkBlock.Block.Slot = b.Block.Slot
	forkBlock.Block.ProposerIndex = 1
	forkRoot, err := forkBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	blockEpoch := slots.RoundAt(b.Block.Slot)
	childSlot, err := slots.RoundStart(blockEpoch + 1)
	require.NoError(t, err)
	childBlock := util.NewBeaconBlock()
	childBlock.Block.Slot = childSlot
	childBlock.Block.ParentRoot = r[:]
	childRoot, err := childBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	tests := []struct {
		wsVerified     bool
		disabled       bool
		wantErr        error
		checkpt        *ethpb.Checkpoint
		finalizedEpoch primitives.Round
		name           string
	}{
		{
			name:     "nil root and epoch",
			disabled: true,
		},
		{
			name:           "not yet to verify, ws epoch higher than finalized epoch",
			checkpt:        &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'a'}, 32), Epoch: blockEpoch},
			finalizedEpoch: blockEpoch - 1,
		},
		{
			name:           "can't find the block in DB",
			checkpt:        &ethpb.Checkpoint{Root: bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength), Epoch: 1},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSBlockNotFound,
		},
		{
			name:           "can't find the block corresponds to ws epoch in DB",
			checkpt:        &ethpb.Checkpoint{Root: r[:], Epoch: blockEpoch - 2}, // Root belongs in epoch 1.
			finalizedEpoch: blockEpoch - 1,
			wantErr:        errWSBlockNotFoundInEpoch,
		},
		{
			name:           "block in db but not canonical",
			checkpt:        &ethpb.Checkpoint{Root: forkRoot[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSBlockNotCanonical,
		},
		{
			name:           "canonical block from next epoch fails epoch range",
			checkpt:        &ethpb.Checkpoint{Root: childRoot[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSBlockNotFoundInEpoch,
		},
		{
			name:           "can verify and pass",
			checkpt:        &ethpb.Checkpoint{Root: r[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
		},
		{
			name:           "not yet to verify, equal epoch",
			checkpt:        &ethpb.Checkpoint{Root: r[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testServiceWithDB(t)
			beaconDB := s.cfg.BeaconDB
			util.SaveBlock(t, t.Context(), beaconDB, b)
			util.SaveBlock(t, t.Context(), beaconDB, forkBlock)
			util.SaveBlock(t, t.Context(), beaconDB, childBlock)
			require.NoError(t, beaconDB.SaveGenesisBlockRoot(t.Context(), bytesutil.ToBytes32(b.Block.ParentRoot)))
			require.NoError(t, beaconDB.SaveFinalizedCheckpoint(t.Context(), &ethpb.Checkpoint{Root: childRoot[:], Epoch: blockEpoch + 1}))
			wv, err := NewWeakSubjectivityVerifier(tt.checkpt, beaconDB)
			require.NoError(t, err)
			s.cfg.WeakSubjectivityCheckpt = tt.checkpt
			s.wsVerifier = wv
			require.Equal(t, !tt.disabled, wv.enabled)
			require.NoError(t, s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: tt.finalizedEpoch}))
			cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
			err = s.wsVerifier.VerifyWeakSubjectivity(t.Context(), cp.Epoch)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

// TestWeakSubjectivityCheckpointRoundTrip pins the producer/consumer contract for
// --weak-subjectivity-checkpoint: what prysmctl prints must be what the beacon
// node reads back. The numeric half is a ROUND, so the verifier's search window
// -- RoundStart(round) through RoundStart(round)+SlotsPerRound-1 -- has to contain
// the block the producer named. At 8 slots per round inside a 32-slot epoch an
// epoch-valued producer misses that window entirely and the node exits with
// errWSBlockNotFoundInEpoch.
func TestWeakSubjectivityCheckpointRoundTrip(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)

	// The weak subjectivity block sits at slot 100: round 12, epoch 3.
	const blockSlot = primitives.Slot(100)
	blockRoot := [32]byte{0xab}

	// Producer side: what the GetWeakSubjectivity handler and prysmctl emit.
	wsd := &beacon.WeakSubjectivityData{
		BlockRoot: blockRoot,
		Round:     slots.RoundAt(blockSlot),
	}

	// Consumer side: the beacon node's flag parser and verifier.
	cp, err := helpers.ParseWeakSubjectivityInputString(wsd.CheckpointString())
	require.NoError(t, err)
	v, err := NewWeakSubjectivityVerifier(cp, nil)
	require.NoError(t, err)

	endSlot := v.slot + params.BeaconConfig().SlotsPerRound - 1
	require.Equal(t, true, v.slot <= blockSlot && blockSlot <= endSlot,
		"verifier window [%d,%d] must contain the weak subjectivity block at slot %d",
		v.slot, endSlot, blockSlot)
	require.Equal(t, blockRoot, v.root)
}

package interop

import (
	"math/big"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/detect"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPremineGenesis_Electra(t *testing.T) {
	one := uint64(1)

	genesis := types.NewBlockWithHeader(&types.Header{
		Time:          uint64(time.Now().Unix()),
		Extra:         make([]byte, 32),
		BaseFee:       big.NewInt(1),
		ExcessBlobGas: &one,
		BlobGasUsed:   &one,
	})
	_, err := NewPreminedGenesis(t.Context(), time.Unix(int64(genesis.Time()), 0), 10, 10, version.Electra, genesis)
	require.NoError(t, err)
}

// TestPremineGenesis_Heze is the step-4 acceptance check: genesis is built
// directly as a Heze state, it survives an SSZ round trip through the fork
// detector, and the genesis bid describes the geth genesis block.
func TestPremineGenesis_Heze(t *testing.T) {
	one := uint64(1)
	gb := types.NewBlockWithHeader(&types.Header{
		Time:          uint64(time.Now().Unix()),
		Extra:         make([]byte, 32),
		BaseFee:       big.NewInt(1),
		ExcessBlobGas: &one,
		BlobGasUsed:   &one,
		GasLimit:      30000000,
	})
	st, err := NewPreminedGenesis(t.Context(), time.Unix(int64(gb.Time()), 0), 256, 0, version.Heze, gb)
	require.NoError(t, err)
	require.Equal(t, version.Heze, st.Version())
	require.DeepEqual(t, params.BeaconConfig().HezeForkVersion, st.Fork().CurrentVersion)
	require.DeepEqual(t, params.BeaconConfig().HezeForkVersion, st.Fork().PreviousVersion)

	// process_withdrawals returns early unless these two agree, and the first
	// block's bid must name the genesis block hash as its parent.
	bid, err := st.LatestExecutionPayloadBid()
	require.NoError(t, err)
	require.Equal(t, gb.Hash(), gethcommon.Hash(bid.BlockHash()))
	require.Equal(t, gb.ParentHash(), gethcommon.Hash(bid.ParentBlockHash()))
	require.Equal(t, gb.GasLimit(), bid.GasLimit())
	lbh, err := st.LatestBlockHash()
	require.NoError(t, err)
	require.Equal(t, gb.Hash(), gethcommon.Hash(lbh))
	matches, err := st.LatestBlockHashMatchesBidBlockHash()
	require.NoError(t, err)
	require.Equal(t, true, matches)

	// The first block's empty parent_execution_requests must hash to the bid's
	// execution requests root.
	emptyRequestsRoot, err := enginev1.EmptyExecutionRequestsHashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, emptyRequestsRoot, bid.ExecutionRequestsRoot())

	// Slot 0 carries a payload; every later slot does not.
	avail, err := st.ExecutionPayloadAvailability(0)
	require.NoError(t, err)
	require.Equal(t, uint64(1), avail)
	avail, err = st.ExecutionPayloadAvailability(1)
	require.NoError(t, err)
	require.Equal(t, uint64(0), avail)

	// The derived Gloas fields are populated.
	window, err := st.PTCWindow()
	require.NoError(t, err)
	require.Equal(t, int(params.BeaconConfig().SlotsPerEpoch)*(2+int(params.BeaconConfig().MinSeedLookahead)), len(window))
	payments, err := st.BuilderPendingPayments()
	require.NoError(t, err)
	require.Equal(t, int(params.BeaconConfig().SlotsPerEpoch)*2, len(payments))
	lookahead, err := st.ProposerLookahead()
	require.NoError(t, err)
	require.Equal(t, int(params.BeaconConfig().SlotsPerEpoch)*2, len(lookahead))
	require.Equal(t, false, isAllZero(lookahead))

	// SSZ round trip through the fork detector.
	enc, err := st.MarshalSSZ()
	require.NoError(t, err)
	cf, err := detect.FromState(enc)
	require.NoError(t, err)
	require.Equal(t, version.Heze, cf.Fork)
	got, err := cf.UnmarshalBeaconState(enc)
	require.NoError(t, err)
	require.Equal(t, version.Heze, got.Version())
	wantRoot, err := st.HashTreeRoot(t.Context())
	require.NoError(t, err)
	gotRoot, err := got.HashTreeRoot(t.Context())
	require.NoError(t, err)
	require.Equal(t, wantRoot, gotRoot)
}

func isAllZero(indices []primitives.ValidatorIndex) bool {
	for _, i := range indices {
		if i != 0 {
			return false
		}
	}
	return true
}

// TestPremineGenesis_HezeSlot1 walks the Heze genesis to slot 1 and runs the
// first block's parent-payload and bid processing against the genesis bid. It
// is the unit-level stand-in for "the chain produces and processes blocks past
// slot 1"; a full node plus execution client run belongs to the e2e suite.
func TestPremineGenesis_HezeSlot1(t *testing.T) {
	one := uint64(1)
	gb := types.NewBlockWithHeader(&types.Header{
		Time:          uint64(time.Now().Unix()),
		Extra:         make([]byte, 32),
		BaseFee:       big.NewInt(1),
		ExcessBlobGas: &one,
		BlobGasUsed:   &one,
		GasLimit:      30000000,
	})
	st, err := NewPreminedGenesis(t.Context(), time.Unix(int64(gb.Time()), 0), 256, 0, version.Heze, gb)
	require.NoError(t, err)

	st, err = transition.ProcessSlots(t.Context(), st, 1)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(1), st.Slot())

	// process_slot unsets the next slot's availability and leaves genesis alone.
	avail, err := st.ExecutionPayloadAvailability(0)
	require.NoError(t, err)
	require.Equal(t, uint64(1), avail)

	parentRoot, err := helpers.BlockRootAtSlot(st, 0)
	require.NoError(t, err)
	randaoMix, err := helpers.RandaoMix(st, 0)
	require.NoError(t, err)
	emptyRequestsRoot, err := enginev1.EmptyExecutionRequestsHashTreeRoot()
	require.NoError(t, err)

	blk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       1,
			ParentRoot: parentRoot,
			StateRoot:  make([]byte, fieldparams.RootLength),
			Body: &ethpb.BeaconBlockBodyGloas{
				RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot: make([]byte, fieldparams.RootLength),
					BlockHash:   make([]byte, fieldparams.RootLength),
				},
				Graffiti: make([]byte, fieldparams.RootLength),
				SyncAggregate: &ethpb.SyncAggregate{
					SyncCommitteeBits:      make([]byte, fieldparams.SyncCommitteeLength/8),
					SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
				},
				ParentExecutionRequests: &enginev1.ExecutionRequestsGloas{},
				SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBid{
					Message: &ethpb.ExecutionPayloadBid{
						ParentBlockHash:       gb.Hash().Bytes(),
						ParentBlockRoot:       parentRoot,
						BlockHash:             make([]byte, fieldparams.RootLength),
						PrevRandao:            randaoMix,
						FeeRecipient:          make([]byte, fieldparams.FeeRecipientLength),
						BuilderIndex:          params.BeaconConfig().BuilderIndexSelfBuild,
						Slot:                  1,
						ExecutionRequestsRoot: emptyRequestsRoot[:],
					},
					Signature: common.InfiniteSignature[:],
				},
			},
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	})
	require.NoError(t, err)

	// The parent is the genesis block, so the genesis bid is settled: its empty
	// execution requests must hash to the bid's execution_requests_root.
	require.NoError(t, gloas.ProcessParentExecutionPayload(t.Context(), st, blk.Block()))
	// And the bid's parent_block_hash must match latest_block_hash.
	require.NoError(t, gloas.ProcessExecutionPayloadBid(st, blk.Block()))

	bid, err := st.LatestExecutionPayloadBid()
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(1), bid.Slot())
}

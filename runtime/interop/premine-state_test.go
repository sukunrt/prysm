package interop

import (
	"math/big"
	"testing"
	"time"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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

// TestPremineGenesis_HezeGenesisBlockRoot pins the genesis block root: the block a node
// reconstructs from the state must be the one the state's latest_block_header commits to, or the
// first proposer's parent root does not match and no block is ever built.
func TestPremineGenesis_HezeGenesisBlockRoot(t *testing.T) {
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

	genesisBlk, err := coreblocks.NewGenesisBlockForState(t.Context(), st)
	require.NoError(t, err)
	want, err := genesisBlk.Block().HashTreeRoot()
	require.NoError(t, err)

	st, err = transition.ProcessSlots(t.Context(), st, 1)
	require.NoError(t, err)
	got, err := helpers.BlockRootAtSlot(st, 0)
	require.NoError(t, err)
	require.DeepEqual(t, want[:], got)
}

// TestPremineGenesis_HezeBuilderSeeding covers the genesis builder registry: deposit entries
// carrying 0xB0 withdrawal credentials must be onboarded as builders rather than validators, so
// an external builder can bid on a chain whose EL has no builder deposit contract.
func TestPremineGenesis_HezeBuilderSeeding(t *testing.T) {
	const nvals = 256
	amount := params.BeaconConfig().MinDepositAmount * 10
	execAddr := bytesutil.ToBytes20([]byte("buildoor-exec-addr"))

	privs, pubs, err := DeterministicallyGenerateKeys(0, nvals)
	require.NoError(t, err)
	dds, roots, err := DepositDataFromKeysWithExecCreds(privs, pubs, 0)
	require.NoError(t, err)

	builderKey, err := bls.RandKey()
	require.NoError(t, err)

	t.Run("valid builder deposit is onboarded", func(t *testing.T) {
		dd, root := builderDepositData(t, builderKey, execAddr, amount, true)
		st, err := preminedHezeGenesis(t, append(dds, dd), append(roots, root))
		require.NoError(t, err)

		require.Equal(t, nvals, st.NumValidators())
		builders, err := st.Builders()
		require.NoError(t, err)
		require.Equal(t, 1, len(builders))
		require.DeepEqual(t, builderKey.PublicKey().Marshal(), builders[0].Pubkey)
		require.DeepEqual(t, execAddr[:], builders[0].ExecutionAddress)
		require.Equal(t, primitives.Gwei(amount), builders[0].Balance)
		require.Equal(t, primitives.Epoch(0), builders[0].DepositEpoch)
		require.Equal(t, params.BeaconConfig().FarFutureEpoch, builders[0].WithdrawableEpoch)
		require.DeepEqual(t, []byte{params.BeaconConfig().PayloadBuilderVersion}, builders[0].Version)

		// The deposit must not linger in the queue, and it must not have funded a validator.
		pending, err := st.PendingDeposits()
		require.NoError(t, err)
		require.Equal(t, 0, len(pending))
		_, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(builderKey.PublicKey().Marshal()))
		require.Equal(t, false, ok)

		// Bid-eligible at slot 0. VerifyBuilderActive is a thin wrapper over IsActiveBuilder,
		// so this is the check RequireBidBuilderActive applies on both the gossip and the
		// builder-API path, and nothing has finalized yet.
		active, err := st.IsActiveBuilder(0)
		require.NoError(t, err)
		require.Equal(t, true, active)
		require.Equal(t, primitives.Round(0), st.FinalizedCheckpoint().Epoch)

		bid := primitives.Gwei(amount - params.BeaconConfig().MinDepositAmount)
		canCover, err := st.CanBuilderCoverBid(0, bid)
		require.NoError(t, err)
		require.Equal(t, true, canCover)
	})

	t.Run("builder deposit with a bad signature is dropped", func(t *testing.T) {
		dd, root := builderDepositData(t, builderKey, execAddr, amount, false)
		st, err := preminedHezeGenesis(t, append(dds, dd), append(roots, root))
		require.NoError(t, err)

		require.Equal(t, nvals, st.NumValidators())
		builders, err := st.Builders()
		require.NoError(t, err)
		require.Equal(t, 0, len(builders))
	})

	t.Run("no builder entries leaves the registry empty", func(t *testing.T) {
		st, err := preminedHezeGenesis(t, dds, roots)
		require.NoError(t, err)

		require.Equal(t, nvals, st.NumValidators())
		builders, err := st.Builders()
		require.NoError(t, err)
		require.Equal(t, 0, len(builders))
	})
}

func preminedHezeGenesis(
	t *testing.T,
	dds []*ethpb.Deposit_Data,
	roots [][]byte,
) (state.BeaconState, error) {
	t.Helper()
	one := uint64(1)
	gb := types.NewBlockWithHeader(&types.Header{
		Time:          uint64(time.Now().Unix()),
		Extra:         make([]byte, 32),
		BaseFee:       big.NewInt(1),
		ExcessBlobGas: &one,
		BlobGasUsed:   &one,
		GasLimit:      30000000,
	})
	return NewPreminedGenesis(t.Context(), time.Unix(int64(gb.Time()), 0), uint64(len(dds)), 0,
		version.Heze, gb, WithDepositData(dds, roots))
}

// builderDepositData mints the deposit_data.json entry a builder is seeded with: 0xB0 prefix,
// eleven zero bytes, then the twenty-byte execution address.
func builderDepositData(
	t *testing.T,
	key common.SecretKey,
	execAddr [20]byte,
	amount uint64,
	validSig bool,
) (*ethpb.Deposit_Data, []byte) {
	t.Helper()
	creds := make([]byte, 12)
	creds[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	creds = append(creds, execAddr[:]...)

	msg := &ethpb.DepositMessage{
		PublicKey:             key.PublicKey().Marshal(),
		WithdrawalCredentials: creds,
		Amount:                amount,
	}
	mr, err := msg.HashTreeRoot()
	require.NoError(t, err)
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	sr, err := (&ethpb.SigningData{ObjectRoot: mr[:], Domain: domain}).HashTreeRoot()
	require.NoError(t, err)

	sig := make([]byte, fieldparams.BLSSignatureLength)
	if validSig {
		sig = key.Sign(sr[:]).Marshal()
	}
	dd := &ethpb.Deposit_Data{
		PublicKey:             msg.PublicKey,
		WithdrawalCredentials: creds,
		Amount:                amount,
		Signature:             sig,
	}
	root, err := dd.HashTreeRoot()
	require.NoError(t, err)
	return dd, root[:]
}

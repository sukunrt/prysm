//go:build !minimal

package validator

import (
	"encoding/binary"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestValidatorStatus_Active(t *testing.T) {
	// This test breaks if it doesn't use mainnet config
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	ctx := t.Context()

	pubkey := generatePubkey(1)

	depData := &ethpb.Deposit_Data{
		PublicKey:             pubkey,
		Signature:             bytesutil.PadTo([]byte("hi"), 96),
		WithdrawalCredentials: bytesutil.PadTo([]byte("hey"), 32),
	}

	deposit := &ethpb.Deposit{
		Data: depData,
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")
	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)

	root, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, depositCache.InsertDeposit(ctx, deposit, 0 /*blockNum*/, 0, root))

	// Active because activation epoch <= current epoch < exit epoch.
	activeEpoch := helpers.ActivationExitEpoch(0)

	block := util.NewBeaconBlock()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	st := &ethpb.BeaconState{
		GenesisTime: uint64(time.Unix(0, 0).Unix()),
		Slot:        10000,
		Validators: []*ethpb.Validator{{
			ActivationEpoch:   activeEpoch,
			ExitEpoch:         params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
			PublicKey:         pubkey},
		}}
	stateObj, err := state_native.InitializeFromProtoUnsafePhase0(st)
	require.NoError(t, err)

	timestamp := time.Unix(int64(params.BeaconConfig().Eth1FollowDistance), 0).Unix()
	p := &mockExecution.Chain{
		TimesByHeight: map[int]uint64{
			int(params.BeaconConfig().Eth1FollowDistance): uint64(timestamp),
		},
	}
	vs := &Server{
		ChainStartFetcher: p,
		BlockFetcher:      p,
		Eth1InfoFetcher:   p,
		DepositFetcher:    depositCache,
		HeadFetcher:       &mockChain.ChainService{State: stateObj, Root: genesisRoot[:]},
	}
	req := &ethpb.ValidatorStatusRequest{
		PublicKey: pubkey,
	}
	resp, err := vs.ValidatorStatus(t.Context(), req)
	require.NoError(t, err, "Could not get validator status")

	expected := &ethpb.ValidatorStatusResponse{
		Status:          ethpb.ValidatorStatus_ACTIVE,
		ActivationEpoch: 5,
	}
	if !proto.Equal(resp, expected) {
		t.Errorf("Wanted %v, got %v", expected, resp)
	}
}

// pubKey is a helper to generate a well-formed public key.
func generatePubkey(i uint64) []byte {
	pubKey := make([]byte, params.BeaconConfig().BLSPubkeyLength)
	binary.LittleEndian.PutUint64(pubKey, i)
	return pubKey
}

// TestServer_CheckDoppelGanger_RecencyGateIsEpochWide pins the unit of the
// recency gate against the unit of the evidence it protects. The evidence is the
// epoch participation arrays of the head state and of a state one epoch back, so
// the gate must be epoch-wide. At 8 slots per round inside a 32-slot epoch a
// round-keyed gate is 4x narrower: a validator whose last attestation target is
// 3 rounds old clears it, and its own participation bit is then read back as a
// doppelganger.
func TestServer_CheckDoppelGanger_RecencyGateIsEpochWide(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 32
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)

	// Head slot 127 is round 15 and epoch 3.
	gs, keys := util.DeterministicGenesisStateAltair(t, 64)
	hs := gs.Copy()
	require.NoError(t, hs.SetSlot(127))
	ps := gs.Copy()
	require.NoError(t, ps.SetSlot(95))

	// The validator's own attestation is in the head state's evidence.
	currentIndices := make([]byte, 64)
	currentIndices[0] = 1
	require.NoError(t, hs.SetCurrentParticipationBits(currentIndices))

	rb := mockstategen.NewReplayerBuilder()
	rb.SetMockStateForSlot(ps, 95)
	vs := &Server{
		HeadFetcher:     &mockChain.ChainService{State: hs},
		SyncChecker:     &mockSync.Sync{IsSyncing: false},
		ReplayerBuilder: rb,
	}

	// Round 12 starts at slot 96, which lives in epoch 3 -- the head's own epoch.
	pubKey := keys[0].PublicKey().Marshal()
	got, err := vs.CheckDoppelGanger(t.Context(), &ethpb.DoppelGangerRequest{
		ValidatorRequests: []*ethpb.DoppelGangerRequest_ValidatorRequest{
			{PublicKey: pubKey, Epoch: 12, SignedRoot: []byte{'A'}},
		},
	})
	require.NoError(t, err)
	require.DeepEqual(t, &ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
			{PublicKey: pubKey, DuplicateExists: false},
		},
	}, got)
}

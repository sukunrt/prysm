//go:build !minimal

package validator

import (
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

func TestAttestationDataAtSlot_HandlesFarAwayJustifiedEpoch(t *testing.T) {
	// Scenario:
	//
	// State slot = 10000
	// Last justified slot = epoch start of 1500
	// HistoricalRootsLimit = 8192
	//
	// More background: https://github.com/prysmaticlabs/prysm/issues/2153
	// This test breaks if it doesn't use mainnet config

	// Ensure HistoricalRootsLimit matches scenario
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig()
	cfg.HistoricalRootsLimit = 8192
	params.OverrideBeaconConfig(cfg)

	block := util.NewBeaconBlock()
	block.Block.Slot = 10000
	epochBoundaryBlock := util.NewBeaconBlock()
	var err error
	epochBoundaryBlock.Block.Slot, err = slots.EpochStart(slots.ToEpoch(10000))
	require.NoError(t, err)
	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot, err = slots.EpochStart(slots.ToEpoch(1500))
	require.NoError(t, err)
	justifiedBlock.Block.Slot -= 2 // Imagine two skip block
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	justifiedBlockRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash justified block")

	slot := primitives.Slot(10000)
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.RoundAt(1500),
		Root:  justifiedBlockRoot[:],
	}
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(justifiedCheckpoint))

	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache:      cache.NewAttestationDataCache(),
			HeadFetcher:           &mock.ChainService{TargetRoot: blockRoot, Root: blockRoot[:], State: beaconState},
			GenesisTimeFetcher:    &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           10000,
	}
	res, err := attesterServer.GetAttestationData(t.Context(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &ethpb.AttestationData{
		Slot:            req.Slot,
		BeaconBlockRoot: blockRoot[:],
		Source: &ethpb.Checkpoint{
			Epoch: slots.RoundAt(1500),
			Root:  justifiedBlockRoot[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: 312,
			Root:  blockRoot[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func TestProposeAvailableAttestation(t *testing.T) {
	head := util.NewBeaconBlock()
	head.Block.Slot = 999
	head.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)

	validators := make([]*ethpb.Validator, 64)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey:             make([]byte, 48),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
		}
	}

	sk, err := bls.RandKey()
	require.NoError(t, err)
	sig := sk.Sign([]byte("dummy_test_data"))

	t.Run("Heze-Decoupled", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig().Copy()
		config.ElectraForkEpoch = 0
		config.GloasForkEpoch = 0
		config.HezeForkEpoch = 0
		params.OverrideBeaconConfig(config)

		attSlot := params.BeaconConfig().SlotsPerEpoch + 1
		state, err := util.NewBeaconStateGloas()
		require.NoError(t, err)
		require.NoError(t, state.SetSlot(attSlot))
		require.NoError(t, state.SetValidators(validators))
		cs := &mock.ChainService{State: state, BlockSlot: attSlot - 1}
		broadcaster := &mockp2p.MockBroadcaster{}
		server := &Server{
			HeadFetcher:             cs,
			P2P:                     broadcaster,
			AttPool:                 attestations.NewPool(),
			OperationNotifier:       (&mock.ChainService{}).OperationNotifier(),
			TimeFetcher:             cs,
			AttestationStateFetcher: cs,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
			ForkchoiceFetcher:       cs,
			// The gossip subscription skips messages this node published, so
			// the local vote reaches forkchoice through this receiver.
			AvailableAttestationReceiver: cs,
		}
		req := &ethpb.AvailableAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AvailableAttestationData{
				Slot:            attSlot,
				BeaconBlockRoot: root[:],
				PayloadPresent:  false,
			},
		}
		_, err = server.proposeAvailableAtt(t.Context(), req)
		assert.NoError(t, err)
		assert.Equal(t, true, broadcaster.BroadcastCalled.Load())
		require.Equal(t, 1, len(broadcaster.BroadcastMessages))
		assert.Equal(t, true, proto.Equal(req, broadcaster.BroadcastMessages[0]))
		require.Equal(t, 1, len(cs.AvailableAttestations))
		assert.Equal(t, true, proto.Equal(req, cs.AvailableAttestations[0]))
	})
}

func TestGetAvailableAttestationData_OK(t *testing.T) {
	block := util.NewBeaconBlock()

	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	block.Block.Slot = slot

	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	params.BeaconConfig().HezeForkEpoch = 0

	attesterServer := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
		CoreService: &core.Service{
			ChainInfoFetcher: &mock.ChainService{
				MockCanonicalRoots: map[primitives.Slot][32]byte{
					slot: blockRoot,
				},
				MockCanonicalFull: map[primitives.Slot]bool{
					slot: true,
				},
			},
		},
	}

	req := &ethpb.AvailableAttestationDataRequest{
		Slot: slot,
	}
	res, err := attesterServer.GetAvailableAttestationData(t.Context(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &ethpb.AvailableAttestationData{
		Slot:            3*params.BeaconConfig().SlotsPerEpoch + 1,
		BeaconBlockRoot: blockRoot[:],
		PayloadPresent:  true,
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

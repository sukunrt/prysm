package blockchain

import (
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// DomainDataDiagnosticFixture exposes only the test fixture pieces needed by
// the external-package TCP diagnostic. Keeping construction in package
// blockchain lets the diagnostic reuse the real checkpoint cache without a
// production export or an import cycle through the validator RPC package.
type DomainDataDiagnosticFixture struct {
	Service       *Service
	Checkpoint    *ethpb.Checkpoint
	State         state.BeaconState
	MemoizedCount uint64
	SyncSlot1Cold bool
	SingletonKey  []byte
}

// NewDomainDataDiagnosticFixture builds the same 120k slot-zero checkpoint
// state used by the existing count-load diagnostics and verifies the scalar
// used by the memoized control before timed work begins.
func NewDomainDataDiagnosticFixture(t *testing.T) DomainDataDiagnosticFixture {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.HezeForkEpoch = 0
	cfg.SlotsPerRound = 8
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	pb := updateHeadDiagnosticGenesis(t)
	pb.InactivityScores = make([]uint64, updateHeadDiagnosticValidatorCount)
	// Give validator 0 a unique valid key and put it in the current sync committee.
	// The generic large fixture otherwise uses the same zero key everywhere,
	// which cannot meaningfully validate the cold sync-index result.
	secretBytes := make([]byte, params.BeaconConfig().BLSSecretKeyLength)
	secretBytes[len(secretBytes)-1] = 1
	secretKey, err := bls.SecretKeyFromBytes(secretBytes)
	require.NoError(t, err)
	singletonKey := secretKey.PublicKey().Marshal()
	copy(pb.Validators[0].PublicKey, singletonKey)
	copy(pb.CurrentSyncCommittee.Pubkeys[0], singletonKey)
	checkpointState, err := util.NewBeaconStateHeze(func(dst *ethpb.BeaconStateHeze) error {
		*dst = *pb
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, updateHeadDiagnosticValidatorCount, checkpointState.NumValidators())
	require.Equal(t, uint64(0), uint64(checkpointState.Slot()))

	service, _ := minimalTestService(t, WithFinalizedStateAtStartUp(checkpointState))
	require.NoError(t, service.saveGenesisData(t.Context(), checkpointState))
	cp := &ethpb.Checkpoint{Epoch: 0, Root: append([]byte(nil), service.originBlockRoot[:]...)}
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp, checkpointState))
	warmState, err := service.AttestationTargetState(t.Context(), cp)
	require.NoError(t, err)
	require.Equal(t, uint64(0), uint64(warmState.Slot()))
	require.Equal(t, updateHeadDiagnosticValidatorCount, warmState.NumValidators())
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), checkpointState, 0))
	memoizedCount, err := helpers.ActiveValidatorCount(t.Context(), checkpointState, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(updateHeadDiagnosticValidatorCount), memoizedCount)
	aggregateKey, err := checkpointState.AggregateKeyFromIndices([]uint64{0})
	require.NoError(t, err)
	require.DeepEqual(t, singletonKey, aggregateKey.Marshal())
	_, err = service.syncCommitteeHeadState.Get(1)
	require.Equal(t, true, errors.Is(err, cache.ErrNotFound))

	return DomainDataDiagnosticFixture{
		Service:       service,
		Checkpoint:    cp,
		State:         checkpointState,
		MemoizedCount: memoizedCount,
		SyncSlot1Cold: true,
		SingletonKey:  append([]byte(nil), singletonKey...),
	}
}

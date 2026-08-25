package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"go.uber.org/mock/gomock"
)

// setRoundLength pins SLOTS_PER_ROUND for the test so round boundaries are at
// slots 0, 8, 16, ...
func setRoundLength(t *testing.T, slotsPerRound primitives.Slot) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerRound = slotsPerRound
	params.OverrideBeaconConfig(cfg)
}

func attData(slot primitives.Slot, root string) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: bytesutil.PadTo([]byte(root), 32),
		Source:          &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte("source"), 32)},
		Target:          &ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte("target"), 32)},
	}
}

func TestRoundHeadCacheEmpty(t *testing.T) {
	setRoundLength(t, 8)

	c := &roundHeadCache{}
	require.IsNil(t, c.frozen(0))
	require.IsNil(t, c.frozen(9))
}

func TestRoundHeadCacheReusedWithinRound(t *testing.T) {
	setRoundLength(t, 8)

	c := &roundHeadCache{}
	c.freeze(8, attData(8, "roundstart"))

	for _, slot := range []primitives.Slot{8, 9, 15} {
		got := c.frozen(slot)
		require.NotNil(t, got)
		require.DeepEqual(t, bytesutil.PadTo([]byte("roundstart"), 32), got.BeaconBlockRoot)
		// The vote is for its own slot even though the head is the round's.
		require.Equal(t, slot, got.Slot)
	}
}

func TestRoundHeadCacheResetsAcrossRoundBoundary(t *testing.T) {
	setRoundLength(t, 8)

	c := &roundHeadCache{}
	c.freeze(9, attData(9, "roundone"))

	// The next round has nothing frozen yet, and neither has the previous one.
	require.IsNil(t, c.frozen(16))
	require.IsNil(t, c.frozen(7))

	c.freeze(16, attData(16, "roundtwo"))
	got := c.frozen(20)
	require.NotNil(t, got)
	require.DeepEqual(t, bytesutil.PadTo([]byte("roundtwo"), 32), got.BeaconBlockRoot)
	require.IsNil(t, c.frozen(15))
}

func TestRoundHeadCacheReturnsCopies(t *testing.T) {
	setRoundLength(t, 8)

	stored := attData(8, "roundstart")
	c := &roundHeadCache{}
	c.freeze(8, stored)

	// Mutating either the source or a returned value must not move the freeze.
	stored.BeaconBlockRoot = bytesutil.PadTo([]byte("mutated"), 32)
	got := c.frozen(9)
	require.NotNil(t, got)
	require.DeepEqual(t, bytesutil.PadTo([]byte("roundstart"), 32), got.BeaconBlockRoot)

	got.BeaconBlockRoot = bytesutil.PadTo([]byte("mutated"), 32)
	again := c.frozen(10)
	require.NotNil(t, again)
	require.DeepEqual(t, bytesutil.PadTo([]byte("roundstart"), 32), again.BeaconBlockRoot)
}

func TestGetAttestationDataHeadAtRoundStart(t *testing.T) {
	setRoundLength(t, 8)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	reset := features.InitWithReset(&features.Flags{DecoupledFFGHeadAtRoundStart: true})
	defer reset()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)
	v := &validator{validatorClient: client}

	client.EXPECT().AttestationData(gomock.Any(), &ethpb.AttestationDataRequest{
		Slot:           8,
		CommitteeIndex: 0,
	}).Return(attData(8, "roundstart"), nil).Times(1)
	client.EXPECT().AttestationData(gomock.Any(), &ethpb.AttestationDataRequest{
		Slot:           16,
		CommitteeIndex: 0,
	}).Return(attData(16, "nextround"), nil).Times(1)

	got, err := v.getAttestationData(t.Context(), 8, 0)
	require.NoError(t, err)
	require.DeepEqual(t, bytesutil.PadTo([]byte("roundstart"), 32), got.BeaconBlockRoot)

	// Later slot of the same round: the frozen head, not another node call.
	got, err = v.getAttestationData(t.Context(), 12, 0)
	require.NoError(t, err)
	require.DeepEqual(t, bytesutil.PadTo([]byte("roundstart"), 32), got.BeaconBlockRoot)
	require.Equal(t, primitives.Slot(12), got.Slot)

	// New round: the freeze is dropped and the node is asked again.
	got, err = v.getAttestationData(t.Context(), 16, 0)
	require.NoError(t, err)
	require.DeepEqual(t, bytesutil.PadTo([]byte("nextround"), 32), got.BeaconBlockRoot)
}

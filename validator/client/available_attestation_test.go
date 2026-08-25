package client

import (
	"context"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
)

func TestSubmitAvailableAttestation_Ok(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	// Keep the validator count below the committee size so validator 0 always has seats.
	cfg.MinGenesisActiveValidatorCount = 64
	params.OverrideBeaconConfig(cfg)

	validator, m, validatorKey, finish := setup(t, false)
	defer finish()
	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.pubkeyToStatus[pubKey] = &validatorStatus{
		publicKey: pubKey[:],
		status: &ethpb.ValidatorStatusResponse{
			Status: ethpb.ValidatorStatus_ACTIVE,
		},
		index: 42,
	}

	const slot = primitives.Slot(1)
	attData := &ethpb.AvailableAttestationData{
		Slot:            slot,
		PayloadPresent:  false,
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
	}

	m.validatorClient.EXPECT().AvailableAttestationData(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&ethpb.AvailableAttestationDataRequest{}),
	).Return(attData, nil)

	var submitted *ethpb.AvailableAttestation
	m.validatorClient.EXPECT().ProposeAvailableAttestation(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&ethpb.AvailableAttestation{}),
	).Do(func(_ context.Context, att *ethpb.AvailableAttestation) {
		submitted = att
	}).Return(&ethpb.AttestResponse{}, nil)

	validator.SubmitAvailableAttestation(t.Context(), slot, pubKey)
	require.NotNil(t, submitted, "no attestation was proposed")

	expectedBits := bitfield.NewBitvector512()
	for _, s := range decoupled.AvailableAttestationSeats(slot, 42, cfg.MinGenesisActiveValidatorCount) {
		expectedBits.SetBitAt(s, true)
	}

	root, err := signing.ComputeSigningRoot(attData, availableAttDomain)
	require.NoError(t, err)
	sig, err := validator.km.Sign(t.Context(), &validatorpb.SignRequest{
		PublicKey:   pubKey[:],
		SigningRoot: root[:],
	})
	require.NoError(t, err)

	assert.DeepEqual(t, &ethpb.AvailableAttestation{
		AggregationBits: expectedBits,
		Data:            attData,
		Signature:       sig.Marshal(),
	}, submitted)
}

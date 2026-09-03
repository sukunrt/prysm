package beacon_api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	testhelpers "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/test-helpers"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestAvailableAttestationData(t *testing.T) {
	ctx := t.Context()
	slot := uint64(42)
	beaconBlockRoot := testhelpers.FillByteSlice(32, 0xab)
	endpoint := fmt.Sprintf("/eth/v1/validator/available_attestation_data?slot=%d", slot)

	jsonHeader := http.Header{"Content-Type": []string{api.JsonMediaType}}
	sszHeader := http.Header{"Content-Type": []string{api.OctetStreamMediaType}}

	jsonResp, err := json.Marshal(structs.GetAvailableAttestationDataResponse{
		Version: version.String(version.Heze),
		Data: &structs.AvailableAttestationData{
			Slot:            fmt.Sprintf("%d", slot),
			PayloadPresent:  true,
			BeaconBlockRoot: hexutil.Encode(beaconBlockRoot),
		},
	})
	require.NoError(t, err)

	sszResp, err := (&ethpb.AvailableAttestationData{
		Slot:            primitives.Slot(slot),
		PayloadPresent:  true,
		BeaconBlockRoot: beaconBlockRoot,
	}).MarshalSSZ()
	require.NoError(t, err)

	for _, tt := range []struct {
		name   string
		body   []byte
		header http.Header
	}{
		{name: "json response", body: jsonResp, header: jsonHeader},
		{name: "ssz response", body: sszResp, header: sszHeader},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			handler := mock.NewMockHandler(ctrl)
			handler.EXPECT().GetSSZ(gomock.Any(), endpoint).Return(tt.body, tt.header, nil).Times(1)

			client := &beaconApiValidatorClient{handler: handler}
			data, err := client.availableAttestationData(ctx, primitives.Slot(slot))
			require.NoError(t, err)
			require.NotNil(t, data)
			assert.Equal(t, primitives.Slot(slot), data.Slot)
			assert.Equal(t, true, data.PayloadPresent)
			assert.Equal(t, hexutil.Encode(beaconBlockRoot), hexutil.Encode(data.BeaconBlockRoot))
		})
	}
}

func TestAvailableAttestationData_NilData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockHandler(ctrl)

	jsonHeader := http.Header{"Content-Type": []string{api.JsonMediaType}}
	handler.EXPECT().GetSSZ(gomock.Any(), gomock.Any()).Return([]byte("{}"), jsonHeader, nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.availableAttestationData(t.Context(), 1)
	require.ErrorContains(t, "available attestation data is nil", err)
}

func TestAvailableAttestationData_EndpointError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := mock.NewMockHandler(ctrl)

	handler.EXPECT().GetSSZ(gomock.Any(), gomock.Any()).Return(nil, nil, errors.New("boom")).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.availableAttestationData(t.Context(), 1)
	require.ErrorContains(t, "boom", err)
}

func TestProposeAvailableAttestation(t *testing.T) {
	bits := bitfield.NewBitvector512()
	bits.SetBitAt(5, true)
	att := &ethpb.AvailableAttestation{
		AggregationBits: bits,
		Data: &ethpb.AvailableAttestationData{
			Slot:            99,
			PayloadPresent:  true,
			BeaconBlockRoot: testhelpers.FillByteSlice(32, 0x11),
		},
		Signature:    testhelpers.FillByteSlice(96, 0x22),
		ScratchSpace: testhelpers.FillByteSlice(100, 0x33),
	}
	wantRoot, err := att.Data.HashTreeRoot()
	require.NoError(t, err)
	elem, err := att.MarshalSSZ()
	require.NoError(t, err)
	// The SSZ body is List[AvailableAttestation]: a one-entry offset table, then the element.
	sszBody := append([]byte{4, 0, 0, 0}, elem...)
	jsonBody, err := json.Marshal([]*structs.AvailableAttestation{structs.AvailableAttestationFromConsensus(att)})
	require.NoError(t, err)
	headers := map[string]string{api.VersionHeader: version.String(version.Heze)}

	t.Run("valid sends SSZ", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(
			gomock.Any(),
			availableAttestationsEndpoint,
			headers,
			bytes.NewBuffer(sszBody),
		).Return(nil, nil, nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		resp, err := client.proposeAvailableAttestation(t.Context(), att)
		require.NoError(t, err)
		assert.DeepEqual(t, wantRoot[:], resp.AttestationDataRoot)
	})

	t.Run("the SSZ body is a one-element list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		var posted []byte
		handler.EXPECT().PostSSZ(gomock.Any(), availableAttestationsEndpoint, headers, gomock.Any()).
			DoAndReturn(func(_ any, _ string, _ map[string]string, body *bytes.Buffer) ([]byte, http.Header, error) {
				posted = body.Bytes()
				return nil, nil, nil
			}).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		_, err := client.proposeAvailableAttestation(t.Context(), att)
		require.NoError(t, err)

		require.Equal(t, true, len(posted) > 4)
		require.Equal(t, uint32(4), binary.LittleEndian.Uint32(posted[:4]))
		got := &ethpb.AvailableAttestation{}
		require.NoError(t, got.UnmarshalSSZ(posted[4:]))
		assert.DeepEqual(t, att.ScratchSpace, got.ScratchSpace)
		assert.DeepEqual(t, att.Signature, got.Signature)
	})

	t.Run("falls back to JSON on 415", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(gomock.Any(), availableAttestationsEndpoint, gomock.Any(), gomock.Any()).
			Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
		handler.EXPECT().Post(
			gomock.Any(),
			availableAttestationsEndpoint,
			headers,
			bytes.NewBuffer(jsonBody),
			nil,
		).Return(nil).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		resp, err := client.proposeAvailableAttestation(t.Context(), att)
		require.NoError(t, err)
		assert.DeepEqual(t, wantRoot[:], resp.AttestationDataRoot)
	})

	t.Run("non-415 error does not fall back", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		handler := mock.NewMockHandler(ctrl)
		handler.EXPECT().PostSSZ(gomock.Any(), availableAttestationsEndpoint, gomock.Any(), gomock.Any()).
			Return(nil, nil, errors.New("bad request")).Times(1)

		client := &beaconApiValidatorClient{handler: handler}
		_, err := client.proposeAvailableAttestation(t.Context(), att)
		require.ErrorContains(t, "bad request", err)
	})

	t.Run("nil attestation", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		_, err := client.proposeAvailableAttestation(t.Context(), nil)
		require.ErrorContains(t, "available attestation is nil", err)
	})

	t.Run("nil data", func(t *testing.T) {
		client := &beaconApiValidatorClient{}
		_, err := client.proposeAvailableAttestation(t.Context(), &ethpb.AvailableAttestation{AggregationBits: bits})
		require.ErrorContains(t, "available attestation is nil", err)
	})
}

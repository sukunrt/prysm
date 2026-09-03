package beacon

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/prysm/v1alpha1/validator"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testAvailableAttestation(slot primitives.Slot) *eth.AvailableAttestation {
	bits := bitfield.NewBitvector512()
	bits.SetBitAt(1, true)
	return &eth.AvailableAttestation{
		AggregationBits: bits,
		Data: &eth.AvailableAttestationData{
			Slot:            slot,
			PayloadPresent:  true,
			BeaconBlockRoot: bytesutil.PadTo([]byte("root"), 32),
		},
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
}

func availableAttestationsServer(delegate eth.BeaconNodeValidatorServer, syncing bool) *Server {
	return &Server{
		SyncChecker:             &mockSync.Sync{IsSyncing: syncing},
		HeadFetcher:             &chainMock.ChainService{},
		TimeFetcher:             &chainMock.ChainService{},
		OptimisticModeFetcher:   &chainMock.ChainService{},
		V1Alpha1ValidatorServer: delegate,
	}
}

// availableAttestationsSSZList builds the SSZ List[AvailableAttestation] body:
// the offset table, then the elements.
func availableAttestationsSSZList(t *testing.T, atts ...*eth.AvailableAttestation) []byte {
	t.Helper()
	table := make([]byte, 4*len(atts))
	var elems []byte
	for i, att := range atts {
		binary.LittleEndian.PutUint32(table[i*4:], uint32(len(table)+len(elems)))
		enc, err := att.MarshalSSZ()
		require.NoError(t, err)
		elems = append(elems, enc...)
	}
	return append(table, elems...)
}

func availableAttestationsRequest(t *testing.T, body []byte, ssz bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/eth/v1/beacon/pool/available_attestations",
		bytes.NewReader(body))
	req.Header.Set(api.VersionHeader, "heze")
	if ssz {
		req.Header.Set("Content-Type", api.OctetStreamMediaType)
	} else {
		req.Header.Set("Content-Type", api.JsonMediaType)
	}
	return req
}

func TestSubmitAvailableAttestations_JSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	att := testAvailableAttestation(9)
	delegate := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	delegate.EXPECT().ProposeAvailableAttestation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, got *eth.AvailableAttestation) (*eth.AttestResponse, error) {
			assert.Equal(t, primitives.Slot(9), got.Data.Slot)
			assert.Equal(t, true, got.Data.PayloadPresent)
			assert.DeepEqual(t, att.Data.BeaconBlockRoot, got.Data.BeaconBlockRoot)
			assert.DeepEqual(t, []byte(att.AggregationBits), []byte(got.AggregationBits))
			assert.DeepEqual(t, att.Signature, got.Signature)
			return &eth.AttestResponse{}, nil
		})

	body, err := json.Marshal([]*structs.AvailableAttestation{
		structs.AvailableAttestationFromConsensus(att),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	availableAttestationsServer(delegate, false).
		SubmitAvailableAttestations(w, availableAttestationsRequest(t, body, false))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitAvailableAttestations_SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	att := testAvailableAttestation(9)
	delegate := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	delegate.EXPECT().ProposeAvailableAttestation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, got *eth.AvailableAttestation) (*eth.AttestResponse, error) {
			assert.Equal(t, primitives.Slot(9), got.Data.Slot)
			assert.DeepEqual(t, att.Signature, got.Signature)
			return &eth.AttestResponse{}, nil
		})

	body := availableAttestationsSSZList(t, att)
	require.Equal(t, 4+205, len(body))

	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	availableAttestationsServer(delegate, false).
		SubmitAvailableAttestations(w, availableAttestationsRequest(t, body, true))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubmitAvailableAttestations_SSZTwoElements(t *testing.T) {
	ctrl := gomock.NewController(t)
	delegate := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	var got []*eth.AvailableAttestation
	delegate.EXPECT().ProposeAvailableAttestation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, a *eth.AvailableAttestation) (*eth.AttestResponse, error) {
			got = append(got, a)
			return &eth.AttestResponse{}, nil
		}).Times(2)

	first := testAvailableAttestation(9)
	first.ScratchSpace = make([]byte, 100)
	second := testAvailableAttestation(10)
	second.ScratchSpace = make([]byte, 7)
	body := availableAttestationsSSZList(t, first, second)

	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	availableAttestationsServer(delegate, false).
		SubmitAvailableAttestations(w, availableAttestationsRequest(t, body, true))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 2, len(got))
	require.Equal(t, primitives.Slot(9), got[0].Data.Slot)
	require.Equal(t, 100, len(got[0].ScratchSpace))
	require.Equal(t, primitives.Slot(10), got[1].Data.Slot)
	require.Equal(t, 7, len(got[1].ScratchSpace))
}

func TestSubmitAvailableAttestations_BadRequests(t *testing.T) {
	validJSON, err := json.Marshal([]*structs.AvailableAttestation{
		structs.AvailableAttestationFromConsensus(testAvailableAttestation(9)),
	})
	require.NoError(t, err)

	t.Run("missing version header", func(t *testing.T) {
		req := availableAttestationsRequest(t, validJSON, false)
		req.Header.Del(api.VersionHeader)
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}

		availableAttestationsServer(nil, false).SubmitAvailableAttestations(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, api.VersionHeader+" header is required", w.Body.String())
	})
	t.Run("unparseable version header", func(t *testing.T) {
		req := availableAttestationsRequest(t, validJSON, false)
		req.Header.Set(api.VersionHeader, "nonsense")
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}

		availableAttestationsServer(nil, false).SubmitAvailableAttestations(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("pre-heze version header", func(t *testing.T) {
		req := availableAttestationsRequest(t, validJSON, false)
		req.Header.Set(api.VersionHeader, "gloas")
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}

		availableAttestationsServer(nil, false).SubmitAvailableAttestations(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "Available attestations require the Heze fork", w.Body.String())
	})
	t.Run("syncing", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		availableAttestationsServer(nil, true).
			SubmitAvailableAttestations(w, availableAttestationsRequest(t, validJSON, false))
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
	t.Run("empty json array", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		availableAttestationsServer(nil, false).
			SubmitAvailableAttestations(w, availableAttestationsRequest(t, []byte("[]"), false))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "no data submitted", w.Body.String())
	})
	t.Run("misaligned ssz body", func(t *testing.T) {
		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		availableAttestationsServer(nil, false).
			SubmitAvailableAttestations(w, availableAttestationsRequest(t, make([]byte, 100), true))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "Invalid SSZ available attestation list size", w.Body.String())
	})
	t.Run("bad json element is an indexed failure", func(t *testing.T) {
		bad := structs.AvailableAttestationFromConsensus(testAvailableAttestation(9))
		bad.Signature = "0xdeadbeef"
		body, err := json.Marshal([]*structs.AvailableAttestation{bad})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		w.Body = &bytes.Buffer{}
		availableAttestationsServer(nil, false).
			SubmitAvailableAttestations(w, availableAttestationsRequest(t, body, false))
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.StringContains(t, "Signature", w.Body.String())
	})
}

func TestSubmitAvailableAttestations_DelegateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	delegate := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	delegate.EXPECT().ProposeAvailableAttestation(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.InvalidArgument, "incorrect available attestation signature"))

	body, err := json.Marshal([]*structs.AvailableAttestation{
		structs.AvailableAttestationFromConsensus(testAvailableAttestation(9)),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	availableAttestationsServer(delegate, false).
		SubmitAvailableAttestations(w, availableAttestationsRequest(t, body, false))
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.StringContains(t, "incorrect available attestation signature", w.Body.String())
	assert.Equal(t, false, bytes.Contains(w.Body.Bytes(), []byte("rpc error")))
}

// TestSubmitAvailableAttestations_FillsScratchSpace runs the REST handler into
// a real validator.Server, the delegate the node wires up, so the fill site in
// proposeAvailableAtt is reached. A gomock delegate stops short of it.
func TestSubmitAvailableAttestations_FillsScratchSpace(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = 0
	cfg.GoldfishScratchSpace = 77
	params.OverrideBeaconConfig(cfg)

	sk, err := bls.RandKey()
	require.NoError(t, err)
	att := testAvailableAttestation(0)
	att.Data.PayloadPresent = false
	att.Signature = sk.Sign([]byte("scratch")).Marshal()

	broadcaster := &mockp2p.MockBroadcaster{}
	delegate := &validator.Server{
		SyncChecker: &mockSync.Sync{IsSyncing: false},
		TimeFetcher: &chainMock.ChainService{},
		P2P:         broadcaster,
	}

	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	availableAttestationsServer(delegate, false).SubmitAvailableAttestations(
		w, availableAttestationsRequest(t, availableAttestationsSSZList(t, att), true))
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	sent, ok := broadcaster.BroadcastMessages[0].(*eth.AvailableAttestation)
	require.Equal(t, true, ok)
	require.Equal(t, 77, len(sent.ScratchSpace))
}

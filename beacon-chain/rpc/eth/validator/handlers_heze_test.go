package validator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func availableAttestationServer(chain *mockChain.ChainService, syncing bool) *Server {
	return &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: syncing},
		HeadFetcher:           chain,
		TimeFetcher:           chain,
		OptimisticModeFetcher: chain,
		CoreService: &core.Service{
			GenesisTimeFetcher: chain,
			ForkchoiceFetcher:  chain,
			HeadFetcher:        chain,
			ChainInfoFetcher:   chain,
		},
	}
}

func setHezeForkEpoch(t *testing.T, epoch primitives.Epoch) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.HezeForkEpoch = epoch
	params.OverrideBeaconConfig(cfg)
}

func TestGetAvailableAttestationData(t *testing.T) {
	root := bytesutil.PadTo([]byte("head-root"), 32)
	slot := primitives.Slot(5)

	fullChain := func() *mockChain.ChainService {
		return &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
		}
	}

	t.Run("pre-heze slot is a bad request", func(t *testing.T) {
		setHezeForkEpoch(t, 100)
		s := availableAttestationServer(fullChain(), false)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data?slot=5", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.StringContains(t, "heze fork", writer.Body.String())
		assert.Equal(t, "", writer.Header().Get(api.VersionHeader))
	})
	t.Run("missing slot is a bad request", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		s := availableAttestationServer(fullChain(), false)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
	})
	t.Run("syncing", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		s := availableAttestationServer(fullChain(), true)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data?slot=5", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
	})
	t.Run("ok json", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		s := availableAttestationServer(fullChain(), false)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data?slot=5", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)

		resp := &structs.GetAvailableAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, "heze", resp.Version)
		assert.Equal(t, "5", resp.Data.Slot)
		assert.Equal(t, true, resp.Data.PayloadPresent)
		assert.Equal(t, hexutil.Encode(root), resp.Data.BeaconBlockRoot)
		assert.Equal(t, version.String(version.Heze), writer.Header().Get(api.VersionHeader))
	})
	t.Run("payload not present", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		chain := fullChain()
		chain.MockCanonicalFull = map[primitives.Slot]bool{slot: false}
		s := availableAttestationServer(chain, false)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data?slot=5", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)

		resp := &structs.GetAvailableAttestationDataResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, false, resp.Data.PayloadPresent)
	})
	t.Run("ok ssz", func(t *testing.T) {
		setHezeForkEpoch(t, 0)
		s := availableAttestationServer(fullChain(), false)

		request := httptest.NewRequest(http.MethodGet,
			"http://example.com/eth/v1/validator/available_attestation_data?slot=5", nil)
		request.Header.Set("Accept", api.OctetStreamMediaType)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetAvailableAttestationData(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		assert.Equal(t, version.String(version.Heze), writer.Header().Get(api.VersionHeader))
		assert.Equal(t, 41, writer.Body.Len())

		data := &ethpbalpha.AvailableAttestationData{}
		require.NoError(t, data.UnmarshalSSZ(writer.Body.Bytes()))
		assert.Equal(t, primitives.Slot(5), data.Slot)
		assert.Equal(t, true, data.PayloadPresent)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
	})
}

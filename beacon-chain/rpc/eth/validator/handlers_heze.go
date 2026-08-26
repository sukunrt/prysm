package validator

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpbalpha "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// GetAvailableAttestationData produces available attestation data for the requested slot.
func (s *Server) GetAvailableAttestationData(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetAvailableAttestationData")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	_, slot, ok := shared.UintFromQuery(w, r, "slot", true)
	if !ok {
		return
	}

	req := &ethpbalpha.AvailableAttestationDataRequest{Slot: primitives.Slot(slot)}
	data, rpcErr := s.CoreService.GetAvailableAttestationData(ctx, req)
	if rpcErr != nil {
		httputil.HandleError(w, rpcErr.Err.Error(), core.ErrorReasonToHTTP(rpcErr.Reason))
		return
	}

	// Set after the error branch so a failed request does not carry a fork header.
	w.Header().Set(api.VersionHeader, version.String(version.Heze))

	if httputil.RespondWithSsz(r) {
		sszData, err := data.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "Could not marshal available attestation data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszData)
		return
	}

	httputil.WriteJson(w, &structs.GetAvailableAttestationDataResponse{
		Version: version.String(version.Heze),
		Data:    structs.AvailableAttestationDataFromConsensus(data),
	})
}

package beacon

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubmitAvailableAttestations submits available attestations to the node's pool.
func (s *Server) SubmitAvailableAttestations(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.SubmitAvailableAttestations")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}
	v, err := version.FromString(versionHeader)
	if err != nil {
		httputil.HandleError(w, "Could not parse "+api.VersionHeader+": "+err.Error(), http.StatusBadRequest)
		return
	}
	if v < version.Heze {
		httputil.HandleError(w, "Available attestations require the Heze fork", http.StatusBadRequest)
		return
	}

	var atts []*eth.AvailableAttestation
	var failures []*server.IndexedError
	var decodeErr error
	if httputil.IsRequestSsz(r) {
		atts, failures, decodeErr = decodeAvailableAttestationsSSZ(r.Body)
	} else {
		atts, failures, decodeErr = decodeAvailableAttestationsJSON(r.Body)
	}
	if decodeErr != nil {
		httputil.HandleError(w, decodeErr.Error(), http.StatusBadRequest)
		return
	}
	if len(atts) == 0 && len(failures) == 0 {
		httputil.HandleError(w, "no data submitted", http.StatusBadRequest)
		return
	}

	// The delegate is the same gRPC submit path: signature check, Heze gating, payload_present
	// rule, broadcast, local forkchoice delivery and the vote ledger line.
	for i, att := range atts {
		if att == nil {
			continue
		}
		if _, err := s.V1Alpha1ValidatorServer.ProposeAvailableAttestation(ctx, att); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: grpcErrorMessage(err)})
		}
	}

	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
	}
}

func decodeAvailableAttestationsSSZ(r io.Reader) ([]*eth.AvailableAttestation, []*server.IndexedError, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not read request body")
	}
	sszSize := (&eth.AvailableAttestation{}).SizeSSZ()
	if len(body) == 0 || len(body)%sszSize != 0 {
		return nil, nil, errors.New("Invalid SSZ available attestation list size")
	}
	n := len(body) / sszSize
	atts := make([]*eth.AvailableAttestation, n)
	var failures []*server.IndexedError
	for i := range n {
		a := &eth.AvailableAttestation{}
		if err := a.UnmarshalSSZ(body[i*sszSize : (i+1)*sszSize]); err != nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Could not decode SSZ available attestation: " + err.Error(),
			})
			continue
		}
		atts[i] = a
	}
	return atts, failures, nil
}

// decodeAvailableAttestationsJSON decodes a JSON array of AvailableAttestation from body. Returns
// one slot per element in the input (nil for elements that failed to convert), plus per-index
// conversion failures.
func decodeAvailableAttestationsJSON(r io.Reader) ([]*eth.AvailableAttestation, []*server.IndexedError, error) {
	var jsonAtts []*structs.AvailableAttestation
	if err := json.NewDecoder(r).Decode(&jsonAtts); err != nil {
		return nil, nil, errors.Wrap(err, "could not decode request body")
	}
	atts := make([]*eth.AvailableAttestation, len(jsonAtts))
	var failures []*server.IndexedError
	for i, att := range jsonAtts {
		ca, err := att.ToConsensus()
		if err != nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Could not convert available attestation: " + err.Error(),
			})
			continue
		}
		atts[i] = ca
	}
	return atts, failures, nil
}

// grpcErrorMessage unwraps a gRPC status message so an indexed failure carries the delegate's own
// text rather than the "rpc error: code = ..." envelope.
func grpcErrorMessage(err error) string {
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return st.Message()
	}
	return err.Error()
}

package beacon

import (
	"encoding/binary"
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

var errAvailableAttestationList = errors.New("Invalid SSZ available attestation list size")

// decodeAvailableAttestationsSSZ decodes a List[AvailableAttestation] body.
// The element is variable-size, so the body is an offset table followed by the
// elements, not a concatenation of fixed-size elements.
func decodeAvailableAttestationsSSZ(r io.Reader) ([]*eth.AvailableAttestation, []*server.IndexedError, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not read request body")
	}
	offsets, err := availableAttestationOffsets(body)
	if err != nil {
		return nil, nil, err
	}
	atts := make([]*eth.AvailableAttestation, len(offsets))
	var failures []*server.IndexedError
	for i, start := range offsets {
		end := len(body)
		if i+1 < len(offsets) {
			end = int(offsets[i+1])
		}
		a := &eth.AvailableAttestation{}
		if err := a.UnmarshalSSZ(body[start:end]); err != nil {
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

// availableAttestationOffsets reads the offset table of an SSZ list of
// variable-size elements. The first offset is the table length, so it gives
// the element count.
func availableAttestationOffsets(body []byte) ([]uint32, error) {
	if len(body) < 4 {
		return nil, errAvailableAttestationList
	}
	first := binary.LittleEndian.Uint32(body[:4])
	if first < 4 || first%4 != 0 || uint64(first) > uint64(len(body)) {
		return nil, errAvailableAttestationList
	}
	offsets := make([]uint32, first/4)
	prev := uint32(0)
	for i := range offsets {
		o := binary.LittleEndian.Uint32(body[i*4 : i*4+4])
		if o < prev || uint64(o) > uint64(len(body)) {
			return nil, errAvailableAttestationList
		}
		offsets[i], prev = o, o
	}
	return offsets, nil
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

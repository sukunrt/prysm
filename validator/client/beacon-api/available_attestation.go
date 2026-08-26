package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const availableAttestationsEndpoint = "/eth/v1/beacon/pool/available_attestations"

func (c *beaconApiValidatorClient) availableAttestationData(ctx context.Context, slot primitives.Slot) (*ethpb.AvailableAttestationData, error) {
	endpoint := fmt.Sprintf("/eth/v1/validator/available_attestation_data?slot=%d", slot)
	// Prefer SSZ; GetSSZ negotiates and the server may answer JSON, which we decode below.
	// Freshness options steer the read toward a node that already imported the announced head.
	data, header, err := c.handler.GetSSZ(ctx, endpoint, availableAttestationFreshnessOptions(ctx)...)
	if err != nil {
		return nil, errors.Wrap(err, "could not get available attestation data")
	}
	if strings.Contains(header.Get("Content-Type"), api.OctetStreamMediaType) {
		d := &ethpb.AvailableAttestationData{}
		if err := d.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal ssz available attestation data")
		}
		return d, nil
	}
	var resp structs.GetAvailableAttestationDataResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, errors.Wrap(err, "could not decode available attestation data")
	}
	if resp.Data == nil {
		return nil, errors.New("available attestation data is nil")
	}
	return resp.Data.ToConsensus()
}

func (c *beaconApiValidatorClient) proposeAvailableAttestation(
	ctx context.Context, att *ethpb.AvailableAttestation,
) (*ethpb.AttestResponse, error) {
	if att == nil || att.Data == nil {
		return nil, errors.New("available attestation is nil")
	}
	// The data root is not carried on the wire in either direction; the beacon node recomputes it.
	root, err := att.Data.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute available attestation data root")
	}
	headers := map[string]string{api.VersionHeader: version.String(version.Heze)}

	// Prefer SSZ; fall back to JSON if the beacon node does not accept octet-stream request bodies.
	// The SSZ body is the List[AvailableAttestation] encoding, here a single fixed-size element.
	sszBody, err := att.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal available attestation ssz")
	}
	if _, _, err = c.handler.PostSSZ(ctx, availableAttestationsEndpoint, headers, bytes.NewBuffer(sszBody)); err == nil {
		return &ethpb.AttestResponse{AttestationDataRoot: root[:]}, nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) || errJson.Code != http.StatusUnsupportedMediaType {
		return nil, err
	}
	log.WithError(err).Warn("Beacon node does not accept SSZ available attestations, falling back to JSON")

	jsonBody, err := json.Marshal([]*structs.AvailableAttestation{structs.AvailableAttestationFromConsensus(att)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal available attestation")
	}
	if err := c.handler.Post(ctx, availableAttestationsEndpoint, headers, bytes.NewBuffer(jsonBody), nil); err != nil {
		return nil, err
	}
	return &ethpb.AttestResponse{AttestationDataRoot: root[:]}, nil
}

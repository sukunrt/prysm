package beacon_api

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) submitAggregateSelectionProof(
	ctx context.Context,
	in *ethpb.AggregateSelectionRequest,
	index primitives.ValidatorIndex,
	committeeLength uint64,
	attDataRoot []byte,
) (*ethpb.AggregateSelectionResponse, error) {
	if err := c.validateAggregateSelectionRequest(ctx, in, committeeLength, attDataRoot); err != nil {
		return nil, err
	}

	aggregateAttestationResponse, err := c.aggregateAttestation(ctx, in.Slot, attDataRoot, in.CommitteeIndex)
	if err != nil {
		return nil, err
	}

	var attData *structs.Attestation
	if err := json.Unmarshal(aggregateAttestationResponse.Data, &attData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal aggregate attestation data")
	}
	if attData == nil {
		return nil, errors.New("aggregate attestation is nil")
	}

	aggregatedAttestation, err := attData.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert aggregate attestation json to proto")
	}

	return &ethpb.AggregateSelectionResponse{
		AggregateAndProof: &ethpb.AggregateAttestationAndProof{
			AggregatorIndex: index,
			Aggregate:       aggregatedAttestation,
			SelectionProof:  in.SlotSignature,
		},
	}, nil
}

func (c *beaconApiValidatorClient) submitAggregateSelectionProofElectra(
	ctx context.Context,
	in *ethpb.AggregateSelectionRequest,
	index primitives.ValidatorIndex,
	committeeLength uint64,
	attDataRoot []byte,
) (*ethpb.AggregateSelectionElectraResponse, error) {
	if err := c.validateAggregateSelectionRequest(ctx, in, committeeLength, attDataRoot); err != nil {
		return nil, err
	}

	aggregateAttestationResponse, err := c.aggregateAttestationElectra(ctx, in.Slot, attDataRoot, in.CommitteeIndex)
	if err != nil {
		return nil, err
	}

	var attData *structs.AttestationElectra
	if err := json.Unmarshal(aggregateAttestationResponse.Data, &attData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal aggregate attestation electra data")
	}
	if attData == nil {
		return nil, errors.New("aggregate attestation is nil")
	}

	aggregatedAttestation, err := attData.ToConsensus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert aggregate attestation json to proto")
	}

	return &ethpb.AggregateSelectionElectraResponse{
		AggregateAndProof: &ethpb.AggregateAttestationAndProofElectra{
			AggregatorIndex: index,
			Aggregate:       aggregatedAttestation,
			SelectionProof:  in.SlotSignature,
		},
	}, nil
}

// validateAggregateSelectionRequest checks the node and the caller are fit to aggregate. The
// attestation data root is supplied by the caller — the root of the data it signed at attestation
// time — and is never re-derived here: a fresh fetch would name the current head, while the pool
// holds the head as it was at signing time.
func (c *beaconApiValidatorClient) validateAggregateSelectionRequest(
	ctx context.Context,
	in *ethpb.AggregateSelectionRequest,
	committeeLength uint64,
	attDataRoot []byte,
) error {
	isOptimistic, err := c.isOptimistic(ctx)
	if err != nil {
		return err
	}

	// An optimistic validator MUST NOT participate in attestation. (i.e., sign across the DOMAIN_BEACON_ATTESTER, DOMAIN_SELECTION_PROOF or DOMAIN_AGGREGATE_AND_PROOF domains).
	if isOptimistic {
		return errors.New("the node is currently optimistic and cannot serve validators")
	}

	isAggregator, err := helpers.IsAggregator(committeeLength, in.SlotSignature)
	if err != nil {
		return errors.Wrap(err, "failed to get aggregator status")
	}
	if !isAggregator {
		return errors.New("validator is not an aggregator")
	}

	if len(attDataRoot) == 0 {
		return errors.New("attestation data root of the signed attestation is required")
	}

	return nil
}

func (c *beaconApiValidatorClient) aggregateAttestation(
	ctx context.Context,
	slot primitives.Slot,
	attestationDataRoot []byte,
	committeeIndex primitives.CommitteeIndex,
) (*structs.AggregateAttestationResponse, error) {
	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(slot), 10))
	params.Add("attestation_data_root", hexutil.Encode(attestationDataRoot))
	params.Add("committee_index", strconv.FormatUint(uint64(committeeIndex), 10))
	endpoint := apiutil.BuildURL("/eth/v2/validator/aggregate_attestation", params)

	var aggregateAttestationResponse structs.AggregateAttestationResponse
	err := c.handler.Get(ctx, endpoint, &aggregateAttestationResponse)
	if err != nil {
		return nil, err
	}

	return &aggregateAttestationResponse, nil
}

func (c *beaconApiValidatorClient) aggregateAttestationElectra(
	ctx context.Context,
	slot primitives.Slot,
	attestationDataRoot []byte,
	committeeIndex primitives.CommitteeIndex,
) (*structs.AggregateAttestationResponse, error) {
	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(slot), 10))
	params.Add("attestation_data_root", hexutil.Encode(attestationDataRoot))
	params.Add("committee_index", strconv.FormatUint(uint64(committeeIndex), 10))
	endpoint := apiutil.BuildURL("/eth/v2/validator/aggregate_attestation", params)

	var aggregateAttestationResponse structs.AggregateAttestationResponse
	if err := c.handler.Get(ctx, endpoint, &aggregateAttestationResponse); err != nil {
		return nil, err
	}

	return &aggregateAttestationResponse, nil
}

package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// AvailableAttestationReceiver interface defines the methods of the chain service
// for receiving validated available attestations.
type AvailableAttestationReceiver interface {
	ReceiveAvailableAttestation(ctx context.Context, att *ethpb.AvailableAttestation) error
}

// ReceiveAvailableAttestation records a validated available attestation as a
// Goldfish head vote in forkchoice. The attestation carries exactly one signer,
// spread over every available committee seat that signer holds in the slot.
func (s *Service) ReceiveAvailableAttestation(ctx context.Context, att *ethpb.AvailableAttestation) error {
	if att == nil || att.Data == nil || att.AggregationBits == nil {
		return errors.New("nil available attestation")
	}
	if slots.ToEpoch(att.Data.Slot) < params.BeaconConfig().HezeForkEpoch {
		return nil
	}
	seats := att.AggregationBits.Count()
	if seats == 0 {
		return errors.New("available attestation has no seats")
	}
	st, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head state for available attestation")
	}
	indices := decoupled.AvailableAttestationSeatsToValidatorIndices(
		att.Data.Slot, att.AggregationBits.BitIndices(), uint64(st.NumValidators()))
	if len(indices) != 1 {
		return errors.New("available attestation does not have exactly one signer")
	}
	root := bytesutil.ToBytes32(att.Data.BeaconBlockRoot)
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.cfg.ForkChoiceStore.InsertAvailableAttestation(att.Data.Slot, indices[0], seats, root, att.Data.PayloadPresent)
	return nil
}

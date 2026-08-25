package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/async"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

// availableAttestationDueComponent returns the slot-component basis points for the
// attestation due time.
func availableAttestationDueComponent(slot primitives.Slot) primitives.BP {
	cfg := params.BeaconConfig()
	return cfg.AvailableAttestationDueBPSHeze
}

// waitUntilAvailableAttestationDueOrValidBlock waits until (a) or (b) whichever comes first:
//
//	(a) the validator has received a valid block that is the same slot as input slot
//	(b) the configured attestation due time has transpired
func (v *validator) waitUntilAvailableAttestationDueOrValidBlock(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitUntilAvailableAttestationDueOrValidBlock")
	defer span.End()

	finalTime, err := v.slotComponentDeadline(slot, availableAttestationDueComponent(slot))
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for attestation deadline")
		return
	}
	wait := prysmTime.Until(finalTime)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()

	ch := make(chan primitives.Slot, 1)
	sub := v.slotFeed.Subscribe(ch)
	defer sub.Unsubscribe()

	for {
		select {
		case s := <-ch:
			if features.Get().AttestTimely {
				if slot <= s {
					return
				}
			}
		case <-ctx.Done():
			tracing.AnnotateError(span, ctx.Err())
			return
		case <-sub.Err():
			log.Error("Subscriber closed, exiting goroutine")
			return
		case <-t.C:
			return
		}
	}
}

func (v *validator) SubmitAvailableAttestation(
	ctx context.Context,
	slot primitives.Slot,
	pubKey [fieldparams.BLSPubkeyLength]byte,
) {
	ctx, span := trace.StartSpan(ctx, "validator.SubmitAvailableAttestation")
	defer span.End()
	span.SetAttributes(trace.StringAttribute("validator", fmt.Sprintf("%#x", pubKey)))

	epoch := slots.ToEpoch(slot)
	if epoch < params.BeaconConfig().HezeForkEpoch {
		return
	}

	st, ok := v.pubkeyToStatus[pubKey]
	if !ok {
		log.Error("INVALID PUBKEY")
		return
	}
	if !isActiveForDuties(st.status, epoch) {
		log.Error("validator is not active for duties")
		return
	}

	validatorCount := params.BeaconConfig().MinGenesisActiveValidatorCount
	seats := decoupled.AvailableAttestationSeats(slot, st.index, validatorCount)
	if len(seats) == 0 {
		log.Error("validator not scheduled for duty!")
		return
	}

	v.waitUntilAvailableAttestationDueOrValidBlock(ctx, slot)

	var b strings.Builder
	if err := b.WriteByte(byte(iface.RoleAvailableAttester)); err != nil {
		log.WithError(err).Error("Could not write role byte for lock key")
		tracing.AnnotateError(span, err)
		return
	}
	_, err := b.Write(pubKey[:])
	if err != nil {
		log.WithError(err).Error("Could not write pubkey bytes for lock key")
		tracing.AnnotateError(span, err)
		return
	}

	lock := async.NewMultilock(b.String())
	lock.Lock()
	defer lock.Unlock()

	// fmtKey := fmt.Sprintf("%#x", pubKey[:])
	// log := log.WithField("pubkey", fmt.Sprintf("%#x", bytesutil.Trunc(pubKey[:]))).WithField("slot", slot)

	data, err := v.getAvailableAttestationData(ctx, slot)
	if err != nil {
		log.WithError(err).Error("Couldn't write available attestation: failed to get attestation data")
		return
	}
	sign, _, err := v.signAvailableAtt(ctx, pubKey, data, slot)
	if err != nil {
		log.WithError(err).Error("Couldn't sign available attestation")
		return
	}
	ab := bitfield.NewBitvector512()
	for _, s := range seats {
		ab.SetBitAt(s, true)
	}

	att := &ethpb.AvailableAttestation{
		AggregationBits: ab,
		Data:            data,
		Signature:       sign,
	}
	_, err = v.validatorClient.ProposeAvailableAttestation(ctx, att)
	if err != nil {
		log.WithError(err).Error("failed to propose available attestation")
	}
}

var availableAttDomain []byte

func init() {
	const availableAttDomainString = "decoupled-mock-available-attestation"
	var ad = sha256.Sum256([]byte(availableAttDomainString))
	availableAttDomain = ad[:]
}

// Given validator's public key, this function returns the signature of an available attestation data and its signing root.
func (v *validator) signAvailableAtt(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, data *ethpb.AvailableAttestationData, slot primitives.Slot) ([]byte, [32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "validator.signAvailableAtt")
	defer span.End()

	root, err := signing.ComputeSigningRoot(data, availableAttDomain)
	if err != nil {
		return nil, [32]byte{}, err
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     root[:],
		SignatureDomain: availableAttDomain,
		Object:          &validatorpb.SignRequest_AvailableAttestationData{AvailableAttestationData: data},
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, err
	}

	return sig.Marshal(), root, nil
}

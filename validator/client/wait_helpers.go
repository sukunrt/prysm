package client

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// sinceSlotStartTime returns the elapsed time between the start of the provided slot and now.
func (v *validator) sinceSlotStartTime(slot primitives.Slot) (time.Duration, error) {
	sinceSlotStartTime, err := slots.SinceSlotStart(slot, v.genesisTime, prysmTime.Now())
	if err != nil {
		return 0, fmt.Errorf("since slot start: %w", err)
	}

	return sinceSlotStartTime.Round(time.Millisecond), nil
}

// slotComponentDeadline returns the absolute time corresponding to the provided slot component.
func (v *validator) slotComponentDeadline(slot primitives.Slot, component primitives.BP) (time.Time, error) {
	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		return time.Time{}, err
	}
	delay := params.BeaconConfig().SlotComponentDuration(component)
	return startTime.Add(delay), nil
}

func (v *validator) waitUntilSlotComponent(ctx context.Context, slot primitives.Slot, component primitives.BP) {
	ctx, span := trace.StartSpan(ctx, v.slotComponentSpanName(component))
	defer span.End()

	finalTime, err := v.slotComponentDeadline(slot, component)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for slot component deadline")
		return
	}
	wait := prysmTime.Until(finalTime)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
		return
	case <-t.C:
		return
	}
}

// ffgVoteJitter returns a random delay in [0, bound). A non-positive bound yields
// no delay.
func ffgVoteJitter(bound time.Duration) time.Duration {
	if bound <= 0 {
		return 0
	}
	return time.Duration(rand.NewGenerator().Int63n(int64(bound)))
}

// waitSlotStartJitter waits until a bounded random offset from the start of the slot
// has elapsed. It replaces waitUntilAttestationDueOrValidBlock when the FFG vote is
// cast at slot start: the vote no longer waits for the slot's block, and the jitter
// keeps a large validator set from publishing in a single burst.
func (v *validator) waitSlotStartJitter(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitSlotStartJitter")
	defer span.End()

	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).
			Error("Slot overflows, unable to jitter the FFG vote")
		return
	}
	wait := prysmTime.Until(startTime.Add(ffgVoteJitter(features.Get().DecoupledFFGVoteJitter)))
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
	case <-t.C:
	}
}

// latePublishDelay returns how far into the slot the given proposer holds its block
// back. A zero bps disables the knob; everyNth selects the subset of proposer
// indices that publish late, 1 (the default) meaning every proposer.
func latePublishDelay(idx primitives.ValidatorIndex, bps, everyNth uint64) time.Duration {
	if bps == 0 {
		return 0
	}
	if everyNth == 0 {
		everyNth = 1
	}
	if uint64(idx)%everyNth != 0 {
		return 0
	}

	return params.BeaconConfig().SlotComponentDuration(primitives.BP(bps))
}

// waitLatePublish holds a block back until the configured fraction of the slot has
// passed, for the configured subset of proposers, and logs the fact so that a
// measurement run can tell which slots were published late. It is a no-op unless
// --decoupled-late-block-publish-bps is set.
func (v *validator) waitLatePublish(
	ctx context.Context,
	slot primitives.Slot,
	pubKey [fieldparams.BLSPubkeyLength]byte,
) {
	cfg := features.Get()
	if cfg.DecoupledLateBlockPublishBPS == 0 {
		return
	}
	ctx, span := trace.StartSpan(ctx, "validator.waitLatePublish")
	defer span.End()

	duty, err := v.duty(pubKey)
	if err != nil {
		log.WithError(err).WithField("slot", slot).
			Debug("No duty for the proposer, publishing the block on time")
		return
	}
	delay := latePublishDelay(
		duty.ValidatorIndex, cfg.DecoupledLateBlockPublishBPS, cfg.DecoupledLateBlockPublishEveryNth)
	if delay == 0 {
		return
	}
	startTime, err := slots.StartTime(v.genesisTime, slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).
			Error("Slot overflows, publishing the block on time")
		return
	}
	wait := prysmTime.Until(startTime.Add(delay))
	log.WithFields(logrus.Fields{
		"slot":          slot,
		"proposerIndex": duty.ValidatorIndex,
		"delay":         delay,
		"remainingWait": wait,
	}).Warn("Publishing block late")
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
	case <-t.C:
	}
}

// waitForPayloadAvailableOrDeadline blocks until the execution_payload_available
// event for slot is received or the payload attestation deadline is reached,
// whichever comes first.
func (v *validator) waitForPayloadAvailableOrDeadline(ctx context.Context, slot primitives.Slot) {
	ctx, span := trace.StartSpan(ctx, "validator.waitForPayloadAvailableOrDeadline")
	defer span.End()

	deadline, err := v.slotComponentDeadline(slot, params.BeaconConfig().PayloadAttestationDueBPS)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Slot overflows, unable to wait for payload attestation deadline")
		return
	}
	available := v.payloadAvailability.waiter(slot)
	wait := prysmTime.Until(deadline)
	if wait <= 0 {
		return
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		tracing.AnnotateError(span, ctx.Err())
	case <-available:
	case <-t.C:
	}
}

func (v *validator) slotComponentSpanName(component primitives.BP) string {
	cfg := params.BeaconConfig()
	switch component {
	case cfg.AttestationDueBPS:
		return "validator.waitAttestationWindow"
	case cfg.AttestationDueBPSGloas:
		return "validator.waitAttestationWindow"
	case cfg.AggregateDueBPS:
		return "validator.waitAggregateWindow"
	case cfg.AggregateDueBPSGloas:
		return "validator.waitAggregateWindow"
	case cfg.SyncMessageDueBPS:
		return "validator.waitSyncMessageWindow"
	case cfg.SyncMessageDueBPSGloas:
		return "validator.waitSyncMessageWindow"
	case cfg.ContributionDueBPS:
		return "validator.waitContributionWindow"
	case cfg.ContributionDueBPSGloas:
		return "validator.waitContributionWindow"
	case cfg.ProposerReorgCutoffBPS:
		return "validator.waitProposerReorgWindow"
	case cfg.PayloadAttestationDueBPS:
		return "validator.waitPayloadAttestationWindow"
	default:
		return "validator.waitSlotComponent"
	}
}

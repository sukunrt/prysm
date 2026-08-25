package client

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"golang.org/x/sync/errgroup"
)

// The length of total subscribeDuties can be large,
// so need to limit the number of workers to avoid too many goroutines being created.
const subnetSubscriptionAggregatorWorkers = 16

// dutyAtSlot pairs a duty with one of the slots it attests at. The aggregator
// selection proof is over the slot, so each repeat slot is decided on its own.
type dutyAtSlot struct {
	duty *ethpb.ValidatorDuty
	slot primitives.Slot
}

// subscribeToSubnets iterates through each validator duty, signs each slot, and asks beacon node
// to eagerly subscribe to subnets so that the aggregator has attestations to aggregate.
func (v *validator) subscribeToSubnets(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "validator.subscribeToSubnets")
	defer span.End()

	total := len(duties.CurrentEpochDuties) + len(duties.NextEpochDuties)
	// A duty is subscribed to once per attesting slot, and a duty attests once per round.
	total *= int(params.BeaconConfig().SlotsPerEpoch / params.BeaconConfig().SlotsPerRound)
	subscribeDuties := make([]dutyAtSlot, 0, total)
	req := &ethpb.CommitteeSubnetsSubscribeRequest{
		Slots:            make([]primitives.Slot, 0, total),
		CommitteeIds:     make([]primitives.CommitteeIndex, 0, total),
		ValidatorIndices: make([]primitives.ValidatorIndex, 0, total),
		CommitteesAtSlot: make([]uint64, 0, total),
	}

	if err := v.aggSelector.RefreshSelectionProofs(ctx); err != nil {
		return fmt.Errorf("could not prepare aggregated selection proofs: %w", err)
	}

	for _, set := range [][]*ethpb.ValidatorDuty{duties.CurrentEpochDuties, duties.NextEpochDuties} {
		for _, duty := range set {
			if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
				continue
			}
			// The subnet cache on the node is keyed by slot, so every slot the duty
			// attests at needs its own subscription.
			for _, slot := range slots.RoundRepeats(duty.AttesterSlot) {
				subscribeDuties = append(subscribeDuties, dutyAtSlot{duty: duty, slot: slot})
				req.Slots = append(req.Slots, slot)
				req.CommitteeIds = append(req.CommitteeIds, duty.CommitteeIndex)
				req.ValidatorIndices = append(req.ValidatorIndices, duty.ValidatorIndex)
				req.CommitteesAtSlot = append(req.CommitteesAtSlot, duty.CommitteesAtSlot)
			}
		}
	}

	// Run concurrently to check if each validator is an aggregator.
	// Maximum goroutine: subnetSubscriptionAggregatorWorkers (16)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(subnetSubscriptionAggregatorWorkers)
	req.IsAggregator = make([]bool, len(subscribeDuties))
	for i, d := range subscribeDuties {
		g.Go(func() error {
			agg, err := v.isAggregator(gctx, d.duty.CommitteeLength, d.slot, bytesutil.ToBytes48(d.duty.PublicKey))
			if err != nil {
				return fmt.Errorf("could not check if validator is an aggregator: %w", err)
			}
			req.IsAggregator[i] = agg
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("could not check if a validator is an aggregator: %w", err)
	}

	if _, err := v.validatorClient.SubscribeCommitteeSubnets(ctx, req); err != nil {
		return fmt.Errorf("could not subscribe to committee subnets: %w", err)
	}
	return nil
}

func validatorSubnetSubscriptionKey(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) [64]byte {
	return bytesutil.ToBytes64(append(bytesutil.Bytes32(uint64(slot)), bytesutil.Bytes32(uint64(committeeIndex))...))
}

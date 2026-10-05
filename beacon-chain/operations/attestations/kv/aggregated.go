package kv

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	attaggregation "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// AggregateUnaggregatedAttestations is retained for callers that process the pool on a tick.
// Singles are now aggregated as they arrive.
func (c *AttCaches) AggregateUnaggregatedAttestations(ctx context.Context) error {
	return nil
}

// SaveAggregatedAttestation saves an aggregated attestation in cache.
func (c *AttCaches) SaveAggregatedAttestation(att ethpb.Att) error {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	if !att.IsAggregated() {
		return errors.New("attestation is not aggregated")
	}
	has, err := c.HasAggregatedAttestation(att)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	seen, err := c.hasSeenBit(att)
	if err != nil {
		return err
	}
	if seen {
		return nil
	}

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}
	copiedAtt := att.Clone()

	c.aggregatedAttLock.Lock()
	defer c.aggregatedAttLock.Unlock()
	atts, ok := c.aggregatedAtt[id]
	if !ok {
		atts := []ethpb.Att{copiedAtt}
		c.aggregatedAtt[id] = atts
		addCoverage(c.aggregatedCoverage, id, copiedAtt)
		return nil
	}

	candidates := make([]ethpb.Att, 0, len(atts)+1)
	for _, candidate := range atts {
		candidates = append(candidates, candidate.Clone())
	}
	merged, err := attaggregation.Aggregate(append(candidates, copiedAtt))
	if err != nil {
		return err
	}
	c.aggregatedAtt[id] = merged
	rebuildCoverage(c.aggregatedCoverage, id, merged)

	return nil
}

// SaveAggregatedAttestations saves a list of aggregated attestations in cache.
func (c *AttCaches) SaveAggregatedAttestations(atts []ethpb.Att) error {
	for _, att := range atts {
		if err := c.SaveAggregatedAttestation(att); err != nil {
			log.WithError(err).Debug("Could not save aggregated attestation")
			if err := c.DeleteAggregatedAttestation(att); err != nil {
				log.WithError(err).Debug("Could not delete aggregated attestation")
			}
		}
	}
	return nil
}

// AggregatedAttestations returns the aggregated attestations in cache.
func (c *AttCaches) AggregatedAttestations() []ethpb.Att {
	c.aggregatedAttLock.RLock()
	defer c.aggregatedAttLock.RUnlock()

	atts := make([]ethpb.Att, 0)

	for _, a := range c.aggregatedAtt {
		for _, att := range a {
			atts = append(atts, att.Clone())
		}
	}
	for _, att := range c.runningAtt {
		atts = append(atts, att.Clone())
	}

	return atts
}

// AggregatedAttestationsBySlotIndex returns the aggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) AggregatedAttestationsBySlotIndex(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.Attestation {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.AggregatedAttestationsBySlotIndex")
	defer span.End()

	atts := make([]*ethpb.Attestation, 0)

	c.aggregatedAttLock.RLock()
	defer c.aggregatedAttLock.RUnlock()
	for _, as := range c.aggregatedAtt {
		if as[0].Version() == version.Phase0 && slot == as[0].GetData().Slot && committeeIndex == as[0].GetData().CommitteeIndex {
			for _, a := range as {
				att, ok := a.(*ethpb.Attestation)
				// This will never fail in practice because we asserted the version
				if ok {
					atts = append(atts, att.Copy())
				}
			}
		}
	}
	for _, a := range c.runningAtt {
		if a.Version() == version.Phase0 && slot == a.GetData().Slot && committeeIndex == a.GetData().CommitteeIndex {
			if att, ok := a.(*ethpb.Attestation); ok {
				atts = append(atts, att.Copy())
			}
		}
	}

	return atts
}

// AggregatedAttestationsBySlotIndexElectra returns the aggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) AggregatedAttestationsBySlotIndexElectra(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.AttestationElectra {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.AggregatedAttestationsBySlotIndexElectra")
	defer span.End()

	atts := make([]*ethpb.AttestationElectra, 0)

	c.aggregatedAttLock.RLock()
	defer c.aggregatedAttLock.RUnlock()
	for _, as := range c.aggregatedAtt {
		if as[0].Version() >= version.Electra && slot == as[0].GetData().Slot && as[0].CommitteeBitsVal().BitAt(uint64(committeeIndex)) {
			for _, a := range as {
				att, ok := ethpb.AttestationElectraFromAtt(a)
				if ok {
					atts = append(atts, att.Copy())
				}
			}
		}
	}
	for _, a := range c.runningAtt {
		if a.Version() >= version.Electra && slot == a.GetData().Slot && a.CommitteeBitsVal().BitAt(uint64(committeeIndex)) {
			if att, ok := ethpb.AttestationElectraFromAtt(a); ok {
				atts = append(atts, att.Copy())
			}
		}
	}

	return atts
}

// DeleteAggregatedAttestation deletes the aggregated attestations in cache.
func (c *AttCaches) DeleteAggregatedAttestation(att ethpb.Att) error {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	if !att.IsAggregated() {
		return errors.New("attestation is not aggregated")
	}

	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()
	if err := c.insertSeenBit(att); err != nil {
		return err
	}
	c.aggregatedAttLock.Lock()
	defer c.aggregatedAttLock.Unlock()
	if fullID, ok := c.singleByData[id]; ok {
		if single := c.unAggregatedAtt[fullID]; single != nil {
			if bit, ok := singleBit(single); ok && att.GetAggregationBits().BitAt(bit) {
				delete(c.unAggregatedAtt, fullID)
				delete(c.singleByData, id)
			}
		}
	}
	if running := c.runningAtt[id]; running != nil {
		contains, err := att.GetAggregationBits().Contains(running.GetAggregationBits())
		if err != nil {
			return fmt.Errorf("running aggregation bits contain: %w", err)
		}
		if contains {
			if err := c.insertSeenAggregatedAtt(running); err != nil {
				return fmt.Errorf("insert running att: %w", err)
			}
			delete(c.runningAtt, id)
			delete(c.runningSig, id)
		}
	}
	attList, ok := c.aggregatedAtt[id]
	if !ok {
		return nil
	}

	filtered := make([]ethpb.Att, 0)
	for _, a := range attList {
		contains, err := att.GetAggregationBits().Contains(a.GetAggregationBits())
		if err != nil {
			return fmt.Errorf("aggregation bits contain: %w", err)
		}

		if contains {
			if err := c.insertSeenAggregatedAtt(a); err != nil {
				return fmt.Errorf("insert aggregated att: %w", err)
			}

			continue
		}

		// If the attestation in the cache doesn't contain the bits of the attestation to delete, we keep it in the cache.
		filtered = append(filtered, a)
	}

	if len(filtered) == 0 {
		delete(c.aggregatedAtt, id)
		delete(c.aggregatedCoverage, id)
		return nil
	}

	c.aggregatedAtt[id] = filtered
	rebuildCoverage(c.aggregatedCoverage, id, filtered)
	return nil
}

// HasAggregatedAttestation checks if the input attestations has already existed in cache.
func (c *AttCaches) HasAggregatedAttestation(att ethpb.Att) (bool, error) {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return false, err
	}
	if bit, single := singleBit(att); single {
		id, err := attestation.NewId(att, attestation.Data)
		if err != nil {
			return false, fmt.Errorf("could not create attestation ID: %w", err)
		}
		c.aggregatedAttLock.RLock()
		running := c.runningAtt[id]
		has := (running != nil && running.GetAggregationBits().BitAt(bit)) || c.aggregatedCoverage[id].BitAt(bit)
		c.aggregatedAttLock.RUnlock()
		if has {
			return true, nil
		}
		c.blockAttLock.RLock()
		has = c.blockCoverage[id].BitAt(bit)
		c.blockAttLock.RUnlock()
		if has {
			return true, nil
		}
		c.seenAggregatedAttLock.RLock()
		has = c.seenAggregatedCoverage[id].BitAt(bit)
		c.seenAggregatedAttLock.RUnlock()
		if has {
			return true, nil
		}
		return c.hasSeenSingleBit(id, bit)
	}

	has, err := c.hasAggregatedAtt(att)
	if err != nil {
		return false, fmt.Errorf("has aggregated att: %w", err)
	}

	if has {
		return true, nil
	}

	has, err = c.hasBlockAtt(att)
	if err != nil {
		return false, fmt.Errorf("has block att: %w", err)
	}

	if has {
		return true, nil
	}

	has, err = c.hasSeenAggregatedAtt(att)
	if err != nil {
		return false, fmt.Errorf("has seen aggregated att: %w", err)
	}

	if has {
		savedBySeenAggregatedCache.Inc()
		return true, nil
	}

	return false, nil
}

// hasAggregatedAtt checks if the attestation bits are contained in the aggregated attestation cache.
func (c *AttCaches) hasAggregatedAtt(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, fmt.Errorf("could not create attestation ID: %w", err)
	}

	c.aggregatedAttLock.RLock()
	defer c.aggregatedAttLock.RUnlock()

	cacheAtts, ok := c.aggregatedAtt[id]
	if running := c.runningAtt[id]; running != nil {
		contains, err := running.GetAggregationBits().Contains(att.GetAggregationBits())
		if err != nil {
			return false, fmt.Errorf("running aggregation bits contains: %w", err)
		}
		if contains {
			return true, nil
		}
	}
	if bit, single := singleBit(att); single {
		return c.aggregatedCoverage[id].BitAt(bit), nil
	}
	if !ok {
		return false, nil
	}

	for _, cacheAtt := range cacheAtts {
		contains, err := cacheAtt.GetAggregationBits().Contains(att.GetAggregationBits())
		if err != nil {
			return false, fmt.Errorf("aggregation bits contains: %w", err)
		}

		if contains {
			return true, nil
		}
	}

	return false, nil
}

// hasBlockAtt checks if the attestation bits are contained in the block attestation cache.
func (c *AttCaches) hasBlockAtt(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, fmt.Errorf("could not create attestation ID: %w", err)
	}

	c.blockAttLock.RLock()
	defer c.blockAttLock.RUnlock()

	cacheAtts, ok := c.blockAtt[id]
	if bit, single := singleBit(att); single {
		return c.blockCoverage[id].BitAt(bit), nil
	}
	if !ok {
		return false, nil
	}

	for _, cacheAtt := range cacheAtts {
		contains, err := cacheAtt.GetAggregationBits().Contains(att.GetAggregationBits())
		if err != nil {
			return false, fmt.Errorf("aggregation bits contains: %w", err)
		}

		if contains {
			return true, nil
		}
	}

	return false, nil
}

// hasSeenAggregatedAtt checks if the attestation bits are contained in the seen aggregated cache.
func (c *AttCaches) hasSeenAggregatedAtt(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, fmt.Errorf("could not create attestation ID: %w", err)
	}

	c.seenAggregatedAttLock.RLock()
	defer c.seenAggregatedAttLock.RUnlock()

	cacheAtts, ok := c.seenAggregatedAtt[id]
	if bit, single := singleBit(att); single {
		return c.seenAggregatedCoverage[id].BitAt(bit), nil
	}
	if !ok {
		return false, nil
	}

	for _, cacheAtt := range cacheAtts {
		contains, err := cacheAtt.GetAggregationBits().Contains(att.GetAggregationBits())
		if err != nil {
			return false, fmt.Errorf("aggregation bits contains: %w", err)
		}

		if contains {
			return true, nil
		}
	}

	return false, nil
}

// AggregatedAttestationCount returns the number of aggregated attestation keys in the pool.
func (c *AttCaches) AggregatedAttestationCount() int {
	c.aggregatedAttLock.RLock()
	defer c.aggregatedAttLock.RUnlock()
	count := len(c.aggregatedAtt)
	for id := range c.runningAtt {
		if _, ok := c.aggregatedAtt[id]; !ok {
			count++
		}
	}
	return count
}

// insertSeenAggregatedAtt inserts an attestation into the seen aggregated cache.
func (c *AttCaches) insertSeenAggregatedAtt(att ethpb.Att) error {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return fmt.Errorf("new ID: %w", err)
	}

	c.seenAggregatedAttLock.Lock()
	defer c.seenAggregatedAttLock.Unlock()

	cacheAtts, ok := c.seenAggregatedAtt[id]
	if !ok {
		c.seenAggregatedAtt[id] = []ethpb.Att{att.Clone()}
		addCoverage(c.seenAggregatedCoverage, id, att)
		return nil
	}

	// Check if attestation is already contained
	for _, cacheAtt := range cacheAtts {
		contains, err := cacheAtt.GetAggregationBits().Contains(att.GetAggregationBits())
		if err != nil {
			return fmt.Errorf("aggregation bits contains: %w", err)
		}

		if contains {
			return nil
		}
	}

	c.seenAggregatedAtt[id] = append(cacheAtts, att.Clone())
	addCoverage(c.seenAggregatedCoverage, id, att)
	return nil
}

// SeenAggregatedAttestationCount returns the number of keys in the seen aggregated cache.
func (c *AttCaches) SeenAggregatedAttestationCount() int {
	c.seenAggregatedAttLock.RLock()
	defer c.seenAggregatedAttLock.RUnlock()
	return len(c.seenAggregatedAtt)
}

// DeleteSeenAggregatedAttestationsBefore deletes all attestations from the seen cache
// with a slot less than the provided slot.
func (c *AttCaches) DeleteSeenAggregatedAttestationsBefore(expirySlot primitives.Slot) {
	c.seenAggregatedAttLock.Lock()
	defer c.seenAggregatedAttLock.Unlock()

	// The attestation ID contains the slot, so all attestations under the same ID
	// share the same slot. We only need to check the first attestation's slot
	// to determine whether to delete the entire entry.
	for id, atts := range c.seenAggregatedAtt {
		if len(atts) == 0 || atts[0].GetData().Slot < expirySlot {
			delete(c.seenAggregatedAtt, id)
			delete(c.seenAggregatedCoverage, id)
		}
	}
}

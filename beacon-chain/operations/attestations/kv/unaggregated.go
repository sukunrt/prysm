package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// SaveUnaggregatedAttestation saves an unaggregated attestation in cache.
func (c *AttCaches) SaveUnaggregatedAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}
	if c.BeforeRetentionCutoff(att.GetData().Slot) {
		return nil
	}
	if att.IsAggregated() {
		return errors.New("attestation is aggregated")
	}

	bits := att.GetAggregationBits()
	indices := bits.BitIndices()
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}
	if len(indices) == 0 {
		seen, err := c.hasSeenBit(att)
		if err != nil || seen {
			return err
		}
		fullID, err := attestation.NewId(att, attestation.Full)
		if err != nil {
			return errors.Wrap(err, "could not create attestation ID")
		}
		c.unAggregateAttLock.Lock()
		if c.BeforeRetentionCutoff(att.GetData().Slot) {
			c.unAggregateAttLock.Unlock()
			return nil
		}
		c.unAggregatedAtt[fullID] = att.Clone()
		c.unAggregateAttLock.Unlock()
		return nil
	}
	bit := uint64(indices[0])
	groupLock := &c.singleGroupLocks[id[1]^id[2]^id[3]]
	groupLock.Lock()
	defer groupLock.Unlock()

	for {
		c.lockSingleState()
		if c.BeforeRetentionCutoff(att.GetData().Slot) {
			c.unlockSingleState()
			return nil
		}
		covered, err := c.singleCoveredLocked(id, bit)
		if err != nil || covered {
			c.unlockSingleState()
			return err
		}
		running := c.runningAtt[id]
		fullID, hasFirst := c.singleByData[id]
		first := c.unAggregatedAtt[fullID]
		if hasFirst && first == nil {
			delete(c.singleByData, id)
			hasFirst = false
		}
		if hasFirst && first.GetAggregationBits().BitAt(bit) {
			c.unlockSingleState()
			return nil
		}
		if running == nil && !hasFirst {
			fullID, err := attestation.NewId(att, attestation.Full)
			if err == nil {
				c.unAggregatedAtt[fullID] = att.Clone()
				c.singleByData[id] = fullID
			}
			c.unlockSingleState()
			if err != nil {
				return errors.Wrap(err, "could not create attestation ID")
			}
			return nil
		}
		base := running
		baseSig := c.runningSig[id]
		if base == nil {
			base = first
			baseSig = nil
		}
		c.unlockSingleState()

		if c.beforeSingleAggregate != nil {
			c.beforeSingleAggregate()
		}
		next, nextSig, err := addSingleToRunning(base, baseSig, att, bit)
		if err != nil {
			return err
		}

		c.lockSingleState()
		if c.BeforeRetentionCutoff(att.GetData().Slot) {
			c.unlockSingleState()
			return nil
		}
		covered, err = c.singleCoveredLocked(id, bit)
		if err != nil || covered {
			c.unlockSingleState()
			return err
		}
		if running != nil {
			if c.runningAtt[id] != running {
				c.unlockSingleState()
				continue
			}
			c.runningAtt[id] = next
			c.runningSig[id] = nextSig
		} else {
			if c.singleByData[id] != fullID || c.unAggregatedAtt[fullID] != first || c.runningAtt[id] != nil {
				c.unlockSingleState()
				continue
			}
			delete(c.unAggregatedAtt, fullID)
			delete(c.singleByData, id)
			c.runningAtt[id] = next
			c.runningSig[id] = nextSig
		}
		c.unlockSingleState()
		return nil
	}

}

func (c *AttCaches) lockSingleState() {
	c.unAggregateAttLock.Lock()
	c.aggregatedAttLock.Lock()
	c.blockAttLock.RLock()
	c.seenAggregatedAttLock.RLock()
	c.seenBitLock.Lock()
}

func (c *AttCaches) unlockSingleState() {
	c.seenBitLock.Unlock()
	c.seenAggregatedAttLock.RUnlock()
	c.blockAttLock.RUnlock()
	c.aggregatedAttLock.Unlock()
	c.unAggregateAttLock.Unlock()
}

func (c *AttCaches) singleCoveredLocked(id attestation.Id, bit uint64) (bool, error) {
	if running := c.runningAtt[id]; running != nil && running.GetAggregationBits().BitAt(bit) {
		return true, nil
	}
	if c.aggregatedCoverage[id].BitAt(bit) || c.blockCoverage[id].BitAt(bit) || c.seenAggregatedCoverage[id].BitAt(bit) {
		return true, nil
	}
	return c.hasSeenSingleBit(id, bit)
}

func addSingleToRunning(running ethpb.Att, runningSig bls.Signature, single ethpb.Att, bit uint64) (ethpb.Att, bls.Signature, error) {
	if runningSig == nil {
		var err error
		runningSig, err = bls.SignatureFromBytesNoValidation(running.GetSignature())
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not decode running signature")
		}
	}
	singleSig, err := bls.SignatureFromBytesNoValidation(single.GetSignature())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not decode single signature")
	}
	next := running.Clone()
	next.GetAggregationBits().SetBitAt(bit, true)
	nextSig := bls.AggregateSignatures([]bls.Signature{runningSig, singleSig})
	next.SetSignature(nextSig.Marshal())
	return next, nextSig, nil
}

// SaveUnaggregatedAttestations saves a list of unaggregated attestations in cache.
func (c *AttCaches) SaveUnaggregatedAttestations(atts []ethpb.Att) error {
	for _, att := range atts {
		if err := c.SaveUnaggregatedAttestation(att); err != nil {
			return err
		}
	}

	return nil
}

// UnaggregatedAttestations returns all the unaggregated attestations in cache.
func (c *AttCaches) UnaggregatedAttestations() []ethpb.Att {
	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()
	unAggregatedAtts := c.unAggregatedAtt
	atts := make([]ethpb.Att, 0, len(unAggregatedAtts))
	for _, att := range unAggregatedAtts {
		seen, err := c.hasSeenBit(att)
		if err != nil {
			log.WithError(err).Debug("Could not check if unaggregated attestation's bit has been seen. Attestation will not be returned")
			continue
		}
		if !seen {
			atts = append(atts, att.Clone())
		}
	}
	return atts
}

// UnaggregatedAttestationsBySlotIndex returns the unaggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) UnaggregatedAttestationsBySlotIndex(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.Attestation {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.UnaggregatedAttestationsBySlotIndex")
	defer span.End()

	atts := make([]*ethpb.Attestation, 0)

	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()

	unAggregatedAtts := c.unAggregatedAtt
	for _, a := range unAggregatedAtts {
		if a.Version() == version.Phase0 && slot == a.GetData().Slot && committeeIndex == a.GetData().CommitteeIndex {
			att, ok := a.(*ethpb.Attestation)
			// This will never fail in practice because we asserted the version
			if ok {
				atts = append(atts, att.Copy())
			}
		}
	}

	return atts
}

// UnaggregatedAttestationsBySlotIndexElectra returns the unaggregated attestations in cache,
// filtered by committee index and slot.
func (c *AttCaches) UnaggregatedAttestationsBySlotIndexElectra(
	ctx context.Context,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) []*ethpb.AttestationElectra {
	_, span := trace.StartSpan(ctx, "operations.attestations.kv.UnaggregatedAttestationsBySlotIndexElectra")
	defer span.End()

	atts := make([]*ethpb.AttestationElectra, 0)

	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()

	unAggregatedAtts := c.unAggregatedAtt
	for _, a := range unAggregatedAtts {
		if a.Version() >= version.Electra && slot == a.GetData().Slot && a.CommitteeBitsVal().BitAt(uint64(committeeIndex)) {
			att, ok := ethpb.AttestationElectraFromAtt(a)
			if ok {
				atts = append(atts, att.Copy())
			}
		}
	}

	return atts
}

// DeleteUnaggregatedAttestation deletes the unaggregated attestations in cache.
func (c *AttCaches) DeleteUnaggregatedAttestation(att ethpb.Att) error {
	if att == nil || att.IsNil() {
		return nil
	}
	if att.IsAggregated() {
		return errors.New("attestation is aggregated")
	}

	id, err := attestation.NewId(att, attestation.Full)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}
	dataID, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}

	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()
	if err := c.insertSeenBit(att); err != nil {
		log.WithError(err).Debug("Could not insert seen bit of unaggregated attestation. Attestation will be deleted")
	}
	delete(c.unAggregatedAtt, id)
	if mappedID, ok := c.singleByData[dataID]; ok {
		if mapped := c.unAggregatedAtt[mappedID]; mapped != nil {
			if bit, single := singleBit(att); single && mapped.GetAggregationBits().BitAt(bit) {
				delete(c.unAggregatedAtt, mappedID)
				delete(c.singleByData, dataID)
			}
		} else {
			delete(c.singleByData, dataID)
		}
	}

	return nil
}

// DeleteSeenUnaggregatedAttestations deletes the unaggregated attestations in cache
// that have been already processed once. Returns number of attestations deleted.
func (c *AttCaches) DeleteSeenUnaggregatedAttestations() (int, error) {
	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()

	count := 0
	for r, att := range c.unAggregatedAtt {
		if att == nil || att.IsNil() || att.IsAggregated() {
			continue
		}
		seen, err := c.hasSeenBit(att)
		if err != nil {
			log.WithError(err).Debug("Could not check if unaggregated attestation's bit has been seen. Attestation will be deleted")
			seen = true
		}
		if seen {
			delete(c.unAggregatedAtt, r)
			if id, err := attestation.NewId(att, attestation.Data); err == nil {
				if c.singleByData[id] == r {
					delete(c.singleByData, id)
				}
			}
			count++
		}
	}
	return count, nil
}

// UnaggregatedAttestationCount returns the number of unaggregated attestation keys in the pool.
func (c *AttCaches) UnaggregatedAttestationCount() int {
	c.unAggregateAttLock.RLock()
	defer c.unAggregateAttLock.RUnlock()
	return len(c.unAggregatedAtt)
}

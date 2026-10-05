package attestations

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// pruneExpired prunes attestations pool on every slot interval.
func (s *Service) pruneExpired() {
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	offset := time.Duration(secondsPerSlot-1) * time.Second
	slotTicker := slots.NewSlotTickerWithOffset(s.genesisTime, offset, secondsPerSlot)
	defer slotTicker.Done()
	startTicker := slots.NewSlotTickerWithOffset(s.genesisTime, 0, secondsPerSlot)
	defer startTicker.Done()
	for {
		select {
		case <-startTicker.C():
			s.pruneCurrentSlot()
		case <-slotTicker.C():
			if s.hezeActive() {
				s.updateMetrics()
				continue
			}
			s.pruneExpiredAtts()
			s.updateMetrics()
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

func (s *Service) hezeActive() bool {
	return slots.ToEpoch(slots.CurrentSlot(s.genesisTime)) >= params.BeaconConfig().HezeForkEpoch
}

// pruneExpiredExperimental prunes attestations on every prune interval.
func (s *Service) pruneExpiredExperimental() {
	ticker := time.NewTicker(s.cfg.pruneInterval)
	defer ticker.Stop()
	slotTicker := slots.NewSlotTickerWithOffset(s.genesisTime, 0, params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()
	for {
		select {
		case <-slotTicker.C():
			s.pruneCurrentSlot()
		case <-ticker.C:
			if s.hezeActive() {
				continue
			}
			expirySlot, err := s.expirySlot()
			if err != nil {
				log.WithError(err).Error("Could not get expiry slot")
				continue
			}
			numExpired := s.cfg.Cache.PruneBefore(expirySlot)
			s.updateMetricsExperimental(numExpired)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// pruneCurrentSlot applies Heze's short proposal retention at slot start.
func (s *Service) pruneCurrentSlot() bool {
	current := slots.CurrentSlot(s.genesisTime)
	if !s.hezeActive() {
		return false
	}
	cutoff := primitives.Slot(0)
	if current > 3 {
		cutoff = current - 3
	}
	if features.Get().EnableExperimentalAttestationPool {
		numExpired := s.cfg.Cache.PruneRetainedBefore(cutoff)
		s.updateMetricsExperimental(numExpired)
	} else {
		counts, err := s.cfg.Pool.PruneBefore(cutoff)
		if err != nil {
			log.WithError(err).Error("Could not prune old proposal attestations")
		}
		expiredAggregatedAtts.Add(float64(counts.Aggregated))
		expiredUnaggregatedAtts.Add(float64(counts.Unaggregated))
		expiredBlockAtts.Add(float64(counts.Block))
	}
	return true
}

// This prunes expired attestations from the pool.
func (s *Service) pruneExpiredAtts() {
	if s.pruneCurrentSlot() {
		return
	}
	aggregatedAtts := s.cfg.Pool.AggregatedAttestations()
	for _, att := range aggregatedAtts {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteAggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired aggregated attestation")
			}
			expiredAggregatedAtts.Inc()
		}
	}

	if _, err := s.cfg.Pool.DeleteSeenUnaggregatedAttestations(); err != nil {
		log.WithError(err).Error("Cannot delete seen attestations")
	}

	for _, att := range s.cfg.Pool.UnaggregatedAttestations() {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired unaggregated attestation")
			}
			expiredUnaggregatedAtts.Inc()
		}
	}

	blockAtts := s.cfg.Pool.BlockAttestations()
	for _, att := range blockAtts {
		if s.expired(att.GetData().Slot) {
			if err := s.cfg.Pool.DeleteBlockAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired block attestation")
			}
			expiredBlockAtts.Inc()
		}
	}

	expirySlot, err := s.expirySlot()
	if err != nil {
		log.WithError(err).Error("Could not get expiry slot for seen aggregated attestations")
		return
	}

	s.cfg.Pool.DeleteSeenAggregatedAttestationsBefore(expirySlot)
}

// Return true if the input slot has been expired.
// Expired is defined as one epoch behind than current time.
func (s *Service) expired(providedSlot primitives.Slot) bool {
	providedEpoch := slots.ToEpoch(providedSlot)
	currSlot := slots.CurrentSlot(s.genesisTime)
	currEpoch := slots.ToEpoch(currSlot)
	if currEpoch < params.BeaconConfig().DenebForkEpoch {
		return s.expiredPreDeneb(providedSlot)
	}
	return providedEpoch+1 < currEpoch
}

// Handles expiration of attestations before deneb.
func (s *Service) expiredPreDeneb(slot primitives.Slot) bool {
	expirationSlot := slot + params.BeaconConfig().SlotsPerEpoch
	expirationTime := s.genesisTime.Add(time.Duration(expirationSlot.Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	return expirationTime.Before(time.Now())
}

// Attestations for a slot before the returned slot are considered expired.
func (s *Service) expirySlot() (primitives.Slot, error) {
	currSlot := slots.CurrentSlot(s.genesisTime)
	currEpoch := slots.ToEpoch(currSlot)
	if currEpoch == 0 {
		return 0, nil
	}
	if currEpoch < params.BeaconConfig().DenebForkEpoch {
		// Safe to subtract because we exited early for epoch 0.
		return currSlot - 31, nil
	}
	return slots.EpochStart(currEpoch - 1)
}

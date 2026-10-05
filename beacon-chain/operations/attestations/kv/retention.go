package kv

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// PrunedCounts reports proposal candidates retired by kind.
type PrunedCounts struct {
	Aggregated   uint64
	Unaggregated uint64
	Block        uint64
}

// PruneBefore retires proposal candidates before slot without changing fork choice.
func (c *AttCaches) PruneBefore(slot primitives.Slot) (PrunedCounts, error) {
	c.unAggregateAttLock.Lock()
	defer c.unAggregateAttLock.Unlock()
	c.aggregatedAttLock.Lock()
	defer c.aggregatedAttLock.Unlock()
	c.blockAttLock.Lock()
	defer c.blockAttLock.Unlock()
	c.seenAggregatedAttLock.Lock()
	defer c.seenAggregatedAttLock.Unlock()

	if slot <= primitives.Slot(c.retentionCutoff.Load()) {
		return PrunedCounts{}, nil
	}
	var removed PrunedCounts
	for id, atts := range c.aggregatedAtt {
		if len(atts) == 0 || atts[0].GetData().Slot >= slot {
			continue
		}
		removed.Aggregated += uint64(len(atts))
		delete(c.aggregatedAtt, id)
		delete(c.aggregatedCoverage, id)
	}
	for id, a := range c.runningAtt {
		if a.GetData().Slot >= slot {
			continue
		}
		removed.Aggregated++
		delete(c.runningAtt, id)
		delete(c.runningSig, id)
	}
	for id, a := range c.unAggregatedAtt {
		if a.GetData().Slot >= slot {
			continue
		}
		removed.Unaggregated++
		delete(c.unAggregatedAtt, id)
	}
	for id, fullID := range c.singleByData {
		if _, ok := c.unAggregatedAtt[fullID]; !ok {
			delete(c.singleByData, id)
		}
	}
	for id, atts := range c.blockAtt {
		if len(atts) == 0 || atts[0].GetData().Slot >= slot {
			continue
		}
		removed.Block += uint64(len(atts))
		delete(c.blockAtt, id)
		delete(c.blockCoverage, id)
	}
	for id, atts := range c.seenAggregatedAtt {
		if len(atts) == 0 || atts[0].GetData().Slot < slot {
			delete(c.seenAggregatedAtt, id)
			delete(c.seenAggregatedCoverage, id)
		}
	}
	c.retentionCutoff.Store(uint64(slot))
	return removed, nil
}

// BeforeRetentionCutoff reports whether a candidate can no longer be stored.
func (c *AttCaches) BeforeRetentionCutoff(slot primitives.Slot) bool {
	return slot < primitives.Slot(c.retentionCutoff.Load())
}

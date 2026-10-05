package kv

import (
	"github.com/OffchainLabs/go-bitfield"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
)

func (c *AttCaches) insertSeenBit(att ethpb.Att) error {
	c.seenBitLock.Lock()
	defer c.seenBitLock.Unlock()
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return errors.Wrap(err, "could not create attestation ID")
	}
	v, ok := c.seenSingleAtt.Get(id.String())
	bits := bitfield.NewBitlist(att.GetAggregationBits().Len())
	if ok {
		var valid bool
		bits, valid = v.(bitfield.Bitlist)
		if !valid {
			return errors.New("could not convert single bitlist type")
		}
		bits = append(bitfield.Bitlist(nil), bits...)
	}
	for _, bit := range att.GetAggregationBits().BitIndices() {
		bits.SetBitAt(uint64(bit), true)
	}
	if att.GetAggregationBits().Count() == 1 {
		c.seenSingleAtt.Set(id.String(), bits, cache.DefaultExpiration)
		return nil
	}

	v, ok = c.seenAtt.Get(id.String())
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		alreadyExists := false
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.GetAggregationBits()); err != nil {
				return err
			} else if c {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			seenBits = append(seenBits, append(bitfield.Bitlist(nil), att.GetAggregationBits()...))
		}
		c.seenAtt.Set(id.String(), seenBits, cache.DefaultExpiration /* one epoch */)
		c.seenSingleAtt.Set(id.String(), bits, cache.DefaultExpiration)
		return nil
	}

	c.seenAtt.Set(id.String(), []bitfield.Bitlist{append(bitfield.Bitlist(nil), att.GetAggregationBits()...)}, cache.DefaultExpiration /* one epoch */)
	c.seenSingleAtt.Set(id.String(), bits, cache.DefaultExpiration)
	return nil
}

func (c *AttCaches) hasSeenBit(att ethpb.Att) (bool, error) {
	id, err := attestation.NewId(att, attestation.Data)
	if err != nil {
		return false, errors.Wrap(err, "could not create attestation ID")
	}
	if att.GetAggregationBits().Count() == 1 {
		return c.hasSeenSingleBit(id, uint64(att.GetAggregationBits().BitIndices()[0]))
	}

	v, ok := c.seenAtt.Get(id.String())
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return false, errors.New("could not convert to bitlist type")
		}
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.GetAggregationBits()); err != nil {
				return false, err
			} else if c {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *AttCaches) hasSeenSingleBit(id attestation.Id, bit uint64) (bool, error) {
	v, ok := c.seenSingleAtt.Get(id.String())
	if !ok {
		return false, nil
	}
	bits, valid := v.(bitfield.Bitlist)
	if !valid {
		return false, errors.New("could not convert single bitlist type")
	}
	return bits.BitAt(bit), nil
}

func singleBit(att ethpb.Att) (uint64, bool) {
	if att.GetAggregationBits().Count() != 1 {
		return 0, false
	}
	return uint64(att.GetAggregationBits().BitIndices()[0]), true
}

func addCoverage(coverage map[attestation.Id]bitfield.Bitlist, id attestation.Id, att ethpb.Att) {
	bits := att.GetAggregationBits()
	mask := coverage[id]
	if mask == nil {
		mask = bitfield.NewBitlist(bits.Len())
	} else {
		mask = append(bitfield.Bitlist(nil), mask...)
	}
	for _, bit := range bits.BitIndices() {
		mask.SetBitAt(uint64(bit), true)
	}
	coverage[id] = mask
}

func rebuildCoverage(coverage map[attestation.Id]bitfield.Bitlist, id attestation.Id, atts []ethpb.Att) {
	delete(coverage, id)
	for _, att := range atts {
		addCoverage(coverage, id, att)
	}
}

// Package kv includes a key-value store implementation
// of an attestation cache used to satisfy important use-cases
// such as aggregation in a beacon node runtime.
package kv

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/attmap"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/patrickmn/go-cache"
)

// AttCaches defines the caches used to satisfy attestation pool interface.
// These caches are KV store for various attestations
// such are unaggregated, aggregated or attestations within a block.
type AttCaches struct {
	retentionCutoff        atomic.Uint64
	beforeSingleAggregate  func()
	beforePeerCommit       func()
	aggregatedAttLock      sync.RWMutex
	aggregatedAtt          map[attestation.Id][]ethpb.Att
	runningAtt             map[attestation.Id]ethpb.Att
	runningSig             map[attestation.Id]bls.Signature
	aggregatedCoverage     map[attestation.Id]bitfield.Bitlist
	unAggregateAttLock     sync.RWMutex
	unAggregatedAtt        map[attestation.Id]ethpb.Att
	singleByData           map[attestation.Id]attestation.Id
	singleGroupLocks       [256]sync.Mutex
	forkchoiceAtt          *attmap.Attestations
	blockAttLock           sync.RWMutex
	blockAtt               map[attestation.Id][]ethpb.Att
	blockCoverage          map[attestation.Id]bitfield.Bitlist
	seenAtt                *cache.Cache
	seenSingleAtt          *cache.Cache
	seenBitLock            sync.Mutex
	seenAggregatedAttLock  sync.RWMutex
	seenAggregatedAtt      map[attestation.Id][]ethpb.Att
	seenAggregatedCoverage map[attestation.Id]bitfield.Bitlist
}

// NewAttCaches initializes a new attestation pool consists of multiple KV store in cache for
// various kind of attestations.
func NewAttCaches() *AttCaches {
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(2*secsInEpoch*time.Second, 2*secsInEpoch*time.Second)
	pool := &AttCaches{
		unAggregatedAtt:        make(map[attestation.Id]ethpb.Att),
		singleByData:           make(map[attestation.Id]attestation.Id),
		aggregatedAtt:          make(map[attestation.Id][]ethpb.Att),
		runningAtt:             make(map[attestation.Id]ethpb.Att),
		runningSig:             make(map[attestation.Id]bls.Signature),
		aggregatedCoverage:     make(map[attestation.Id]bitfield.Bitlist),
		forkchoiceAtt:          attmap.New(),
		blockAtt:               make(map[attestation.Id][]ethpb.Att),
		blockCoverage:          make(map[attestation.Id]bitfield.Bitlist),
		seenAtt:                c,
		seenSingleAtt:          cache.New(2*secsInEpoch*time.Second, 2*secsInEpoch*time.Second),
		seenAggregatedAtt:      make(map[attestation.Id][]ethpb.Att),
		seenAggregatedCoverage: make(map[attestation.Id]bitfield.Bitlist),
	}

	return pool
}

// saveForkchoiceAttestation saves a forkchoice attestation.
func (c *AttCaches) saveForkchoiceAttestation(att ethpb.Att) error {
	return c.forkchoiceAtt.Save(att)
}

// SaveForkchoiceAttestations saves forkchoice attestations.
func (c *AttCaches) SaveForkchoiceAttestations(att []ethpb.Att) error {
	return c.forkchoiceAtt.SaveMany(att)
}

// ForkchoiceAttestations returns all forkchoice attestations.
func (c *AttCaches) ForkchoiceAttestations() []ethpb.Att {
	return c.forkchoiceAtt.GetAll()
}

// DeleteForkchoiceAttestation deletes a forkchoice attestation.
func (c *AttCaches) DeleteForkchoiceAttestation(att ethpb.Att) error {
	return c.forkchoiceAtt.Delete(att)
}

// ForkchoiceAttestationCount returns the number of forkchoice attestation keys.
func (c *AttCaches) ForkchoiceAttestationCount() int {
	return c.forkchoiceAtt.Count()
}

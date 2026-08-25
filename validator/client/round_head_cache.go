package client

import (
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/proto"
)

// roundHeadCache freezes the attestation data the beacon node returned for the
// first FFG vote of a round so that the round's remaining votes name the same
// head. It backs the head-at-round-start setting of --decoupled-ffg-head-source;
// with head-at-vote-time, the default, nothing reads it.
//
// The freeze is anchored at the round's start slot whenever the client holds an
// attester duty there, and at the client's first vote of the round otherwise.
// The zero value is ready to use.
type roundHeadCache struct {
	mu    sync.Mutex
	round primitives.Round
	data  *ethpb.AttestationData
}

// frozen returns the data frozen for the round holding slot, with its slot field
// rewritten to slot. It returns nil when the cache is empty or holds another
// round, which is how a round boundary resets the freeze.
func (c *roundHeadCache) frozen(slot primitives.Slot) *ethpb.AttestationData {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.data == nil || c.round != slots.RoundAt(slot) {
		return nil
	}
	data := proto.Clone(c.data).(*ethpb.AttestationData)
	data.Slot = slot

	return data
}

// freeze stores data as the answer for every remaining vote of the round holding
// slot, dropping whatever an earlier round left behind.
func (c *roundHeadCache) freeze(slot primitives.Slot, data *ethpb.AttestationData) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.round = slots.RoundAt(slot)
	c.data = proto.Clone(data).(*ethpb.AttestationData)
}

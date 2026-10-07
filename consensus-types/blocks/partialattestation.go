package blocks

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// PeerKind records how a peer entered a group's peer state.
type PeerKind uint8

const (
	// PeerUnknown is the zero value: the library added the peer from the mesh.
	// The first flush promotes it to PeerMesh.
	PeerUnknown PeerKind = iota
	// PeerMesh is a mesh peer: it is pushed every pending vote.
	PeerMesh
	// PeerRemote is an untracked peer that sent us bundles or metadata. It is
	// never pushed to.
	PeerRemote
)

// PartialAttestationPeerState tracks, per committee, which validators' slot
// attestations a peer is known to have (its claims plus everything we pushed
// to it), by global validator index. The slot is the partial-messages group.
// Owned by the gossipsub event loop.
type PartialAttestationPeerState struct {
	Kind      PeerKind
	Available map[primitives.CommitteeIndex]map[uint64]struct{}
}

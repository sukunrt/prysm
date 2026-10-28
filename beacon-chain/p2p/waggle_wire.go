package p2p

import (
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sukunrt/go-waggle/pb"
)

// maxAttestationsPerBundle is MAX_ATTESTATIONS_PER_BUNDLE from EIP-8334.
const maxAttestationsPerBundle = 50

var (
	errWaggleObjectID      = errors.New("object id is not an 8 byte slot")
	errWaggleEmptyMessage  = errors.New("message has no bundle and no metadata")
	errWaggleBundleSize    = errors.New("bundle has more than the maximum attestations")
	errWaggleBundleEmpty   = errors.New("bundle has no attestations")
	errWaggleBundleLengths = errors.New("bundle indices and signatures have different lengths")
	errWaggleBundleDup     = errors.New("bundle lists a validator twice")
	errWaggleBundleSlot    = errors.New("bundle slot is not the object slot")
)

// waggleMessage is one waggle ObjectPieces on an attestation topic. The object ID is
// the slot, pieces is an AttestationBundle and pieces metadata is a
// CommitteeAttestationPartsMetadata. Bundle and Metadata can both be set.
type waggleMessage struct {
	Slot     primitives.Slot
	Bundle   *ethpb.AttestationBundle
	Metadata *ethpb.CommitteeAttestationPartsMetadata
}

func (m waggleMessage) encode() (*pb.ObjectPieces, error) {
	msg := &pb.ObjectPieces{ObjectID: binary.LittleEndian.AppendUint64(nil, uint64(m.Slot))}
	if m.Bundle != nil {
		b, err := m.Bundle.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal bundle")
		}
		msg.Pieces = b
	}
	if m.Metadata != nil {
		b, err := m.Metadata.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal metadata")
		}
		msg.PiecesMetadata = b
	}
	return msg, nil
}

// decodeWaggleMessage decodes msg and does the structural checks. It does not validate attestations.
func decodeWaggleMessage(msg *pb.ObjectPieces) (waggleMessage, error) {
	if len(msg.ObjectID) != 8 {
		return waggleMessage{}, errWaggleObjectID
	}
	m := waggleMessage{Slot: primitives.Slot(binary.LittleEndian.Uint64(msg.ObjectID))}
	if len(msg.Pieces) > 0 {
		b := &ethpb.AttestationBundle{}
		if err := b.UnmarshalSSZ(msg.Pieces); err != nil {
			return waggleMessage{}, errors.Wrap(err, "unmarshal bundle")
		}
		if err := checkBundle(b, m.Slot); err != nil {
			return waggleMessage{}, err
		}
		m.Bundle = b
	}
	if len(msg.PiecesMetadata) > 0 {
		md := &ethpb.CommitteeAttestationPartsMetadata{}
		if err := md.UnmarshalSSZ(msg.PiecesMetadata); err != nil {
			return waggleMessage{}, errors.Wrap(err, "unmarshal metadata")
		}
		m.Metadata = md
	}
	if m.Bundle == nil && m.Metadata == nil {
		return waggleMessage{}, errWaggleEmptyMessage
	}
	return m, nil
}

func checkBundle(b *ethpb.AttestationBundle, slot primitives.Slot) error {
	n := len(b.AttesterIndices)
	switch {
	case n == 0:
		return errWaggleBundleEmpty
	case n > maxAttestationsPerBundle:
		return errWaggleBundleSize
	case n != len(b.Signatures):
		return errWaggleBundleLengths
	case b.AttestationData.Slot != slot:
		return errWaggleBundleSlot
	}
	seen := make(map[uint64]struct{}, n)
	for _, v := range b.AttesterIndices {
		if _, ok := seen[v]; ok {
			return errWaggleBundleDup
		}
		seen[v] = struct{}{}
	}
	return nil
}

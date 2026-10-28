package p2p

import (
	"encoding/binary"
	"slices"

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

// encode writes the object ID, the bundle and the metadata into one buffer.
func (m waggleMessage) encode() (*pb.ObjectPieces, error) {
	var bn, mn int
	if m.Bundle != nil {
		bn = m.Bundle.SizeSSZ()
	}
	if m.Metadata != nil {
		mn = m.Metadata.SizeSSZ()
	}
	buf := make([]byte, 8, 8+bn+mn)
	binary.LittleEndian.PutUint64(buf, uint64(m.Slot))
	msg := &pb.ObjectPieces{ObjectID: buf[:8:8]}
	var err error
	if m.Bundle != nil {
		if msg.Pieces, err = m.Bundle.MarshalSSZTo(buf[8 : 8 : 8+bn]); err != nil {
			return nil, errors.Wrap(err, "marshal bundle")
		}
	}
	if m.Metadata != nil {
		if msg.PiecesMetadata, err = m.Metadata.MarshalSSZTo(buf[8+bn : 8+bn : 8+bn+mn]); err != nil {
			return nil, errors.Wrap(err, "marshal metadata")
		}
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
	var buf [maxAttestationsPerBundle]uint64
	ids := buf[:n]
	copy(ids, b.AttesterIndices)
	slices.Sort(ids)
	for i := 1; i < n; i++ {
		if ids[i] == ids[i-1] {
			return errWaggleBundleDup
		}
	}
	return nil
}

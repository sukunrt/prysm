package encoder

import (
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// A padded gossip block is [magic][N][N scratch bytes][ssz bytes] before
// snappy. SSZ has no header, but a Gloas block is a variable-size container,
// so its first 4 bytes are the offset of its first variable field, which can
// never exceed MAX_PAYLOAD_SIZE. So the magic is not a valid offset and marks
// a padded message without ambiguity.
const (
	scratchMagic     = 0xFFFFFFFF
	scratchHeaderLen = 8
)

// scratchPadded reports whether v is a gossip block that carries the prefix.
//
// The two gossip sides do not see the same Go type. The broadcaster passes the
// proto; the receiver decodes into the consensus-types wrapper that
// types.BlockMap builds. Both must answer true, or a padded block from a peer
// fails to decode and the validator rejects it.
func scratchPadded(v any) bool {
	switch m := v.(type) {
	case *ethpb.SignedBeaconBlockGloas:
		return true
	case interfaces.ReadOnlySignedBeaconBlock:
		return m.Version() >= version.Gloas
	default:
		return false
	}
}

func addScratchPrefix(b []byte) ([]byte, error) {
	n := params.BeaconConfig().ConsensusBlockScratchSpace
	if n == 0 {
		return b, nil
	}
	if n > params.MaxScratchSpace {
		return nil, errors.Errorf(
			"CONSENSUS_BLOCK_SCRATCH_SPACE is %d bytes, the maximum is %d", n, params.MaxScratchSpace)
	}
	out := make([]byte, scratchHeaderLen+int(n)+len(b))
	binary.LittleEndian.PutUint32(out, scratchMagic)
	binary.LittleEndian.PutUint32(out[4:], uint32(n))
	copy(out[scratchHeaderLen:], bytesutil.RandomBytes(int(n)))
	copy(out[scratchHeaderLen+int(n):], b)
	return out, nil
}

func stripScratchPrefix(b []byte) ([]byte, error) {
	if len(b) < 4 || binary.LittleEndian.Uint32(b) != scratchMagic {
		return b, nil
	}
	if len(b) < scratchHeaderLen {
		return nil, errors.Errorf("scratch prefix is truncated: %d bytes", len(b))
	}
	n := binary.LittleEndian.Uint32(b[4:])
	if uint64(n) > params.MaxScratchSpace {
		return nil, errors.Errorf(
			"scratch prefix is %d bytes, the maximum is %d", n, params.MaxScratchSpace)
	}
	if scratchHeaderLen+int(n) > len(b) {
		return nil, errors.Errorf(
			"scratch prefix of %d bytes overruns a message of %d bytes", n, len(b))
	}
	return b[scratchHeaderLen+int(n):], nil
}

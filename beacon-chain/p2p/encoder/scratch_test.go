package encoder_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/golang/snappy"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func setBlockScratchSpace(t *testing.T, n uint64) {
	cfg := params.BeaconConfig().Copy()
	cfg.ConsensusBlockScratchSpace = n
	params.OverrideBeaconConfig(cfg)
	types.InitializeDataMaps()
}

// gloasBlockWrapper builds the target the same way decodePubsubMessage does.
func gloasBlockWrapper(t *testing.T) interfaces.ReadOnlySignedBeaconBlock {
	f, ok := types.BlockMap[bytesutil.ToBytes4(params.BeaconConfig().GloasForkVersion)]
	require.Equal(t, true, ok)
	b, err := f()
	require.NoError(t, err)
	return b
}

// TestScratch_GossipSeam is the seam the network runs: the broadcaster hands
// EncodeGossip the proto, the receiver decodes into the consensus-types
// wrapper. Both sides must agree that a Gloas block carries the prefix.
func TestScratch_GossipSeam(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	e := &encoder.SszNetworkEncoder{}

	for _, n := range []uint64{0, 1000} {
		setBlockScratchSpace(t, n)

		blk := util.NewBeaconBlockGloas()
		want, err := blk.MarshalSSZ()
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = e.EncodeGossip(buf, blk)
		require.NoError(t, err)

		wrapped := gloasBlockWrapper(t)
		require.NoError(t, e.DecodeGossip(buf.Bytes(), wrapped))
		got, err := wrapped.MarshalSSZ()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)

		bare := &ethpb.SignedBeaconBlockGloas{}
		require.NoError(t, e.DecodeGossip(buf.Bytes(), bare))
		got, err = bare.MarshalSSZ()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	}
}

func TestScratch_ZeroIsUnchanged(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setBlockScratchSpace(t, 0)

	blk := util.NewBeaconBlockGloas()
	want, err := blk.MarshalSSZ()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = (&encoder.SszNetworkEncoder{}).EncodeGossip(buf, blk)
	require.NoError(t, err)

	got, err := snappy.Decode(nil, buf.Bytes())
	require.NoError(t, err)
	require.DeepEqual(t, want, got)
}

func TestScratch_GrowsTheWire(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	e := &encoder.SszNetworkEncoder{}
	const n = 4000

	setBlockScratchSpace(t, 0)
	plain := new(bytes.Buffer)
	_, err := e.EncodeGossip(plain, util.NewBeaconBlockGloas())
	require.NoError(t, err)

	setBlockScratchSpace(t, n)
	padded := new(bytes.Buffer)
	_, err = e.EncodeGossip(padded, util.NewBeaconBlockGloas())
	require.NoError(t, err)

	grew := padded.Len() - plain.Len()
	require.Equal(t, true, grew >= n, "compressed size grew by %d, want at least %d", grew, n)
}

func TestScratch_NonBlockTypeIsNotPadded(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setBlockScratchSpace(t, 1000)

	msg := &ethpb.Fork{PreviousVersion: []byte("fooo"), CurrentVersion: []byte("barr"), Epoch: 9001}
	want, err := msg.MarshalSSZ()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = (&encoder.SszNetworkEncoder{}).EncodeGossip(buf, msg)
	require.NoError(t, err)

	got, err := snappy.Decode(nil, buf.Bytes())
	require.NoError(t, err)
	require.DeepEqual(t, want, got)
}

func TestScratch_BadPrefixIsAnError(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setBlockScratchSpace(t, 0)
	e := &encoder.SszNetworkEncoder{}

	prefixed := func(header []byte, body int) []byte {
		return snappy.Encode(nil, append(header, make([]byte, body)...))
	}
	magic := func(n uint32) []byte {
		h := make([]byte, 8)
		binary.LittleEndian.PutUint32(h, 0xFFFFFFFF)
		binary.LittleEndian.PutUint32(h[4:], n)
		return h
	}

	t.Run("truncated header", func(t *testing.T) {
		err := e.DecodeGossip(prefixed(magic(0)[:4], 1), gloasBlockWrapper(t))
		require.ErrorContains(t, "scratch prefix is truncated", err)
	})
	t.Run("length overruns the message", func(t *testing.T) {
		err := e.DecodeGossip(prefixed(magic(1000), 10), gloasBlockWrapper(t))
		require.ErrorContains(t, "overruns", err)
	})
	t.Run("length above the bound", func(t *testing.T) {
		err := e.DecodeGossip(prefixed(magic(params.MaxScratchSpace+1), 10), gloasBlockWrapper(t))
		require.ErrorContains(t, "the maximum is", err)
	})
}

func TestScratch_ConfigAboveBoundFailsEncode(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	setBlockScratchSpace(t, params.MaxScratchSpace+1)

	_, err := (&encoder.SszNetworkEncoder{}).EncodeGossip(new(bytes.Buffer), util.NewBeaconBlockGloas())
	require.ErrorContains(t, "CONSENSUS_BLOCK_SCRATCH_SPACE", err)
}

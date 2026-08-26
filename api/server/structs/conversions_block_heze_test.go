package structs

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func availableAttestationConsensus() *eth.AvailableAttestation {
	bits := bitfield.NewBitvector512()
	bits.SetBitAt(3, true)
	return &eth.AvailableAttestation{
		AggregationBits: bits,
		Data: &eth.AvailableAttestationData{
			Slot:            7,
			PayloadPresent:  true,
			BeaconBlockRoot: bytesOf(0xaa, 32),
		},
		Signature: bytesOf(0xbb, 96),
	}
}

func bytesOf(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestAvailableAttestationDataRoundTrip(t *testing.T) {
	consensus := availableAttestationConsensus().Data
	s := AvailableAttestationDataFromConsensus(consensus)
	require.Equal(t, "7", s.Slot)
	require.Equal(t, true, s.PayloadPresent)
	require.Equal(t, hexutil.Encode(consensus.BeaconBlockRoot), s.BeaconBlockRoot)

	back, err := s.ToConsensus()
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(7), back.Slot)
	require.Equal(t, true, back.PayloadPresent)
	require.DeepEqual(t, consensus.BeaconBlockRoot, back.BeaconBlockRoot)
}

func TestAvailableAttestationRoundTrip(t *testing.T) {
	consensus := availableAttestationConsensus()
	s := AvailableAttestationFromConsensus(consensus)
	require.Equal(t, hexutil.Encode(consensus.AggregationBits), s.AggregationBits)
	require.Equal(t, hexutil.Encode(consensus.Signature), s.Signature)
	require.NotNil(t, s.Data)

	back, err := s.ToConsensus()
	require.NoError(t, err)
	require.DeepEqual(t, []byte(consensus.AggregationBits), []byte(back.AggregationBits))
	require.DeepEqual(t, consensus.Signature, back.Signature)
	require.Equal(t, primitives.Slot(7), back.Data.Slot)

	// The consensus container must survive an SSZ marshal, which rejects any bits length != 64.
	_, err = back.MarshalSSZ()
	require.NoError(t, err)
}

func TestAvailableAttestationToConsensusErrors(t *testing.T) {
	valid := AvailableAttestationFromConsensus(availableAttestationConsensus())

	t.Run("nil receiver", func(t *testing.T) {
		var a *AvailableAttestation
		_, err := a.ToConsensus()
		require.ErrorIs(t, err, errNilValue)
	})
	t.Run("short aggregation bits", func(t *testing.T) {
		a := *valid
		a.AggregationBits = hexutil.Encode(bytesOf(0x01, 32))
		_, err := a.ToConsensus()
		require.ErrorContains(t, "AggregationBits", err)
	})
	t.Run("bad signature", func(t *testing.T) {
		a := *valid
		a.Signature = "not-hex"
		_, err := a.ToConsensus()
		require.ErrorContains(t, "Signature", err)
	})
	t.Run("nil data", func(t *testing.T) {
		a := *valid
		a.Data = nil
		_, err := a.ToConsensus()
		require.ErrorContains(t, "Data", err)
	})
}

func TestAvailableAttestationDataToConsensusErrors(t *testing.T) {
	valid := AvailableAttestationDataFromConsensus(availableAttestationConsensus().Data)

	t.Run("nil receiver", func(t *testing.T) {
		var d *AvailableAttestationData
		_, err := d.ToConsensus()
		require.ErrorIs(t, err, errNilValue)
	})
	t.Run("bad slot", func(t *testing.T) {
		d := *valid
		d.Slot = "abc"
		_, err := d.ToConsensus()
		require.ErrorContains(t, "Slot", err)
	})
	t.Run("short root", func(t *testing.T) {
		d := *valid
		d.BeaconBlockRoot = hexutil.Encode(bytesOf(0x01, 16))
		_, err := d.ToConsensus()
		require.ErrorContains(t, "BeaconBlockRoot", err)
	})
}

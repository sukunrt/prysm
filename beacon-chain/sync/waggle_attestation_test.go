package sync

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAttestationSubnetFromTopic(t *testing.T) {
	digest := [4]byte{1, 2, 3, 4}
	t.Run("with suffix", func(t *testing.T) {
		subnet, err := attestationSubnetFromTopic(fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, 42) + "/ssz_snappy")
		require.NoError(t, err)
		assert.Equal(t, uint64(42), subnet)
	})
	t.Run("without suffix", func(t *testing.T) {
		subnet, err := attestationSubnetFromTopic(fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, 7))
		require.NoError(t, err)
		assert.Equal(t, uint64(7), subnet)
	})
	t.Run("not an attestation topic", func(t *testing.T) {
		_, err := attestationSubnetFromTopic(fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest))
		require.NotNil(t, err)
	})
}

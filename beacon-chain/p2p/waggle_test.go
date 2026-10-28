package p2p

import (
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sukunrt/go-waggle/waggle"
)

func TestIsAttestationTopic(t *testing.T) {
	digest := [fieldparams.VersionLength]byte{1, 2, 3, 4}
	assert.Equal(t, true, isAttestationTopic(attestationToTopic(3, digest)+"/ssz_snappy"))
	assert.Equal(t, false, isAttestationTopic(fmt.Sprintf(AggregateAndProofSubnetTopicFormat, digest)))
	assert.Equal(t, false, isAttestationTopic(syncCommitteeToTopic(3, digest)))
	assert.Equal(t, false, isAttestationTopic(fmt.Sprintf(BlockSubnetTopicFormat, digest)))
}

func TestWaggleAttestations(t *testing.T) {
	const suffix = "/ssz_snappy"
	d1 := [fieldparams.VersionLength]byte{1}
	d2 := [fieldparams.VersionLength]byte{2}
	setup := func(t *testing.T) *waggleAttestations {
		w := waggle.NewWaggle()
		a := newWaggleAttestations(w, suffix)
		t.Cleanup(func() {
			a.Close()
			w.Close()
		})
		return a
	}

	t.Run("ensureDigest makes all subnet topics once", func(t *testing.T) {
		a := setup(t)
		a.EnsureDigest(d1)
		a.EnsureDigest(d1)
		require.Equal(t, int(attestationSubnetCount), len(a.topics))
		for subnet := range attestationSubnetCount {
			assert.NotNil(t, a.topic(attestationToTopic(subnet, d1)+suffix))
		}
	})

	t.Run("subscribe and unsubscribe", func(t *testing.T) {
		a := setup(t)
		a.EnsureDigest(d1)
		topic := attestationToTopic(0, d1) + suffix
		a.Subscribe(topic)
		a.Unsubscribe(topic)
		a.Subscribe(topic)
		a.Subscribe("unknown")
		a.Unsubscribe("unknown")
	})

	t.Run("prune closes only rejected digests", func(t *testing.T) {
		a := setup(t)
		a.EnsureDigest(d1)
		a.EnsureDigest(d2)
		a.Prune(func(d [fieldparams.VersionLength]byte) bool { return d == d2 })
		require.Equal(t, int(attestationSubnetCount), len(a.topics))
		assert.Equal(t, true, a.topic(attestationToTopic(0, d1)+suffix) == nil)
		assert.NotNil(t, a.topic(attestationToTopic(0, d2)+suffix))
		a.Subscribe(attestationToTopic(0, d1) + suffix)

		a.EnsureDigest(d1)
		require.Equal(t, 2*int(attestationSubnetCount), len(a.topics))
	})
}

func TestWantedDigests(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 10
	cfg.BellatrixForkEpoch = 20
	cfg.CapellaForkEpoch = cfg.FarFutureEpoch
	cfg.DenebForkEpoch = cfg.FarFutureEpoch
	cfg.ElectraForkEpoch = cfg.FarFutureEpoch
	cfg.FuluForkEpoch = cfg.FarFutureEpoch
	cfg.GloasForkEpoch = cfg.FarFutureEpoch
	cfg.InitializeForkSchedule()

	phase0 := params.GetNetworkScheduleEntry(0).ForkDigest
	altair := params.GetNetworkScheduleEntry(10).ForkDigest
	require.NotEqual(t, phase0, altair)

	tests := []struct {
		epoch primitives.Epoch
		want  [][fieldparams.VersionLength]byte
	}{
		{epoch: 5, want: [][fieldparams.VersionLength]byte{phase0}},
		{epoch: 9, want: [][fieldparams.VersionLength]byte{phase0, altair}},
		{epoch: 10, want: [][fieldparams.VersionLength]byte{phase0, altair}},
		{epoch: 11, want: [][fieldparams.VersionLength]byte{altair}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("epoch %d", tt.epoch), func(t *testing.T) {
			got := wantedDigests(tt.epoch)
			require.Equal(t, len(tt.want), len(got))
			for _, d := range tt.want {
				assert.Equal(t, true, got[d])
			}
		})
	}
}

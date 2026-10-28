package p2p

import (
	"fmt"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/sukunrt/go-waggle/pb"
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
		a := newWaggleAttestations(t.Context(), w, suffix, DefaultWaggleAttConfig(), time.Now)
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

func TestWaggleMessage(t *testing.T) {
	bundle := func(slot primitives.Slot, indices ...uint64) *ethpb.AttestationBundle {
		sigs := make([][]byte, len(indices))
		for i := range sigs {
			sigs[i] = make([]byte, fieldparams.BLSSignatureLength)
		}
		return &ethpb.AttestationBundle{
			CommitteeIndex: 1,
			AttestationData: &ethpb.AttestationData{
				Slot:            slot,
				BeaconBlockRoot: make([]byte, fieldparams.RootLength),
				Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
			},
			AttesterIndices: indices,
			Signatures:      sigs,
		}
	}
	encode := func(t *testing.T, m waggleMessage) *pb.ObjectPieces {
		msg, err := m.encode()
		require.NoError(t, err)
		return msg
	}

	t.Run("round trip", func(t *testing.T) {
		in := waggleMessage{
			Slot:     5,
			Bundle:   bundle(5, 10, 20),
			Metadata: &ethpb.CommitteeAttestationPartsMetadata{CommitteeIndex: 1, Available: []uint64{10, 20}, Requests: []uint64{30}},
		}
		out, err := decodeWaggleMessage(encode(t, in))
		require.NoError(t, err)
		assert.Equal(t, in.Slot, out.Slot)
		assert.DeepEqual(t, in.Bundle, out.Bundle)
		assert.DeepEqual(t, in.Metadata, out.Metadata)
	})

	t.Run("metadata only", func(t *testing.T) {
		in := waggleMessage{Slot: 5, Metadata: &ethpb.CommitteeAttestationPartsMetadata{Available: []uint64{1}, Requests: []uint64{}}}
		out, err := decodeWaggleMessage(encode(t, in))
		require.NoError(t, err)
		assert.Equal(t, true, out.Bundle == nil)
		assert.NotNil(t, out.Metadata)
	})

	t.Run("errors", func(t *testing.T) {
		tooMany := make([]uint64, maxAttestationsPerBundle+1)
		for i := range tooMany {
			tooMany[i] = uint64(i)
		}
		uneven := bundle(5, 1, 2)
		uneven.Signatures = uneven.Signatures[:1]
		tests := []struct {
			name string
			msg  *pb.ObjectPieces
			err  error
		}{
			{name: "short object id", msg: &pb.ObjectPieces{ObjectID: []byte{1}}, err: errWaggleObjectID},
			{name: "empty", msg: encode(t, waggleMessage{Slot: 5}), err: errWaggleEmptyMessage},
			{name: "no attestations", msg: encode(t, waggleMessage{Slot: 5, Bundle: bundle(5)}), err: errWaggleBundleEmpty},
			{name: "too many attestations", msg: encode(t, waggleMessage{Slot: 5, Bundle: bundle(5, tooMany...)}), err: errWaggleBundleSize},
			{name: "uneven lengths", msg: encode(t, waggleMessage{Slot: 5, Bundle: uneven}), err: errWaggleBundleLengths},
			{name: "wrong slot", msg: encode(t, waggleMessage{Slot: 5, Bundle: bundle(6, 1)}), err: errWaggleBundleSlot},
			{name: "duplicate validator", msg: encode(t, waggleMessage{Slot: 5, Bundle: bundle(5, 1, 1)}), err: errWaggleBundleDup},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := decodeWaggleMessage(tt.msg)
				require.ErrorIs(t, err, tt.err)
			})
		}
	})
}

func TestWaggleAttConfigWithDefaults(t *testing.T) {
	t.Run("zero gets defaults", func(t *testing.T) {
		got := WaggleAttConfig{}.withDefaults()
		want := DefaultWaggleAttConfig()
		want.PushPeers, want.MetadataPeers = 0, 0
		assert.DeepEqual(t, want, got)
	})
	t.Run("set values stay", func(t *testing.T) {
		in := WaggleAttConfig{PushPeers: 3, PushInterval: time.Second, MetadataPeers: 4, MetadataInterval: time.Second, RequestPeers: 5, RequestTimeout: time.Second, MaxValidations: 7}
		assert.DeepEqual(t, in, in.withDefaults())
	})
}

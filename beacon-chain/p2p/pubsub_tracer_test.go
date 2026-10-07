package p2p

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
)

func TestRecordRPCBytes(t *testing.T) {
	topic := "/eth2/00000000/" + GossipAttestationMessage + "_3/ssz_snappy"
	rpc := &pubsub.RPC{}
	rpc.Publish = []*pubsubpb.Message{{Topic: &topic, Data: make([]byte, 40)}}
	rpc.Partial = &pubsubpb.PartialMessagesExtension{
		TopicID:        &topic,
		PartialMessage: make([]byte, 100),
		PartsMetadata:  make([]byte, 10),
	}
	ihave := &pubsubpb.ControlIHave{TopicID: &topic, MessageIDs: []string{"id1", "id2"}}
	iwant := &pubsubpb.ControlIWant{MessageIDs: []string{"id3"}}
	idontwant := &pubsubpb.ControlIDontWant{MessageIDs: []string{"id4"}}
	rpc.Control = &pubsubpb.ControlMessage{
		Ihave:     []*pubsubpb.ControlIHave{ihave},
		Iwant:     []*pubsubpb.ControlIWant{iwant},
		Idontwant: []*pubsubpb.ControlIDontWant{idontwant},
	}

	counters := []prometheus.Counter{
		pubsubRPCBytes.WithLabelValues("sent", "bundle", "attestation"),
		pubsubRPCBytes.WithLabelValues("sent", "metadata", "attestation"),
		pubsubRPCBytes.WithLabelValues("sent", "publish", "attestation"),
		pubsubRPCBytes.WithLabelValues("sent", "ihave", "attestation"),
		pubsubRPCBytes.WithLabelValues("sent", "iwant", "unknown"),
		pubsubRPCBytes.WithLabelValues("sent", "idontwant", "unknown"),
	}
	want := []int{100, 10, proto.Size(rpc.Publish[0]), proto.Size(ihave), proto.Size(iwant),
		proto.Size(idontwant)}
	before := make([]uint64, len(counters))
	for i, c := range counters {
		before[i] = counterValue(c)
	}

	recordRPCBytes("sent", rpc)

	for i, c := range counters {
		require.Equal(t, uint64(want[i]), counterValue(c)-before[i])
	}
}

func TestRecordRPCBytesNilControl(t *testing.T) {
	recordRPCBytes("recv", &pubsub.RPC{})
}

func TestTopicFamily(t *testing.T) {
	require.Equal(t, "attestation", topicFamily("/eth2/00/"+GossipAttestationMessage+"_1/ssz"))
	require.Equal(t, "data_column",
		topicFamily("/eth2/00/"+GossipDataColumnSidecarMessage+"_1/ssz"))
	require.Equal(t, "other", topicFamily("/eth2/00/beacon_block/ssz"))
}

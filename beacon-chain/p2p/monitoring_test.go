package p2p

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	libp2pmetrics "github.com/libp2p/go-libp2p/core/metrics"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestRecordBandwidthLogsWithTheLedgerOn(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{GoldfishVoteLedger: true})
	defer reset()
	hook := logTest.NewGlobal()

	s := &Service{bandwidth: libp2pmetrics.NewBandwidthCounter()}
	s.bandwidth.LogSentMessageStream(7, "/meshsub/1.2.0", "peer")

	// go-flow-metrics folds a mark into the snapshot on its own sweeper tick.
	require.Eventually(t, func() bool {
		s.recordBandwidth()
		entry := hook.LastEntry()
		return entry != nil && entry.Message == "P2P bandwidth" &&
			entry.Data["meshsubOut"] == int64(7)
	}, 10*time.Second, 200*time.Millisecond, "the bandwidth line never carried the mark")
}

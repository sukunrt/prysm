package sync

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

type attestationLogEntry struct {
	slot               primitives.Slot
	committeeIndex     primitives.CommitteeIndex
	sinceSlotStartTime time.Duration
	validationTime     time.Duration
	peerSuffix         string
	peerGossipScore    float64
	beaconBlockRoot    [32]byte
	targetEpoch        primitives.Epoch
	isAggregate        bool
}

func (s *Service) processAttestationLogs() {
	if s.cfg.dataDir == "" {
		// No data dir configured, drain channel but don't log to file.
		for {
			select {
			case <-s.attestationLogCh:
			case <-s.ctx.Done():
				return
			}
		}
	}

	logPath := filepath.Join(s.cfg.dataDir, "attestation_logs.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.WithError(err).Error("Could not open attestation log file")
		return
	}
	s.attestationLogFile = f

	logger := slog.New(slog.NewJSONHandler(f, nil))

	for {
		select {
		case entry := <-s.attestationLogCh:
			logger.Info("attestation_received",
				"slot", uint64(entry.slot),
				"committeeIndex", uint64(entry.committeeIndex),
				"sinceSlotStartMs", entry.sinceSlotStartTime.Milliseconds(),
				"validationTimeMs", entry.validationTime.Milliseconds(),
				"peerSuffix", entry.peerSuffix,
				"peerGossipScore", entry.peerGossipScore,
				"beaconBlockRoot", fmt.Sprintf("%#x", entry.beaconBlockRoot),
				"targetEpoch", uint64(entry.targetEpoch),
				"isAggregate", entry.isAggregate,
			)
		case <-s.ctx.Done():
			return
		}
	}
}

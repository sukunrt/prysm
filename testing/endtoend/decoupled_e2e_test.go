package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_DecoupledConfig runs the e2e suite under the decoupled preset
// (mainnet SSZ sizes, 8-slot epochs), Electra through Fulu, 10 epochs.
//
// Evaluators dropped relative to the minimal run, following the mainnet-preset
// precedent in e2eMainnet:
//   - ValidatorSyncParticipation: SyncCommitteeSize (512) exceeds the 256
//     genesis validators.
//   - ValidatorsVoteWithTheMajority, ProcessesDepositsInBlocks,
//     ActivatesDepositedValidators: mainnet EpochsPerEth1VotingPeriod (64)
//     puts an eth1-data voting period far beyond the run length; deposits
//     still activate via execution requests and are checked by
//     DepositedValidatorsAreActive.
func TestEndToEnd_DecoupledConfig(t *testing.T) {
	cfg := params.E2EDecoupledTestConfig()
	cfg = types.InitForkCfg(version.Electra, version.Fulu, cfg)
	// Set Fulu fork at epoch 2 for a quick fork transition test
	cfg.FuluForkEpoch = 2
	// Update BlobSchedule to use the new FuluForkEpoch for BPO testing
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: cfg.DenebForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlock)},
		{Epoch: cfg.ElectraForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)},
		// BPO (Blob Parameter Optimization) schedule for Fulu
		{Epoch: cfg.FuluForkEpoch + 1, MaxBlobsPerBlock: 15},
		{Epoch: cfg.FuluForkEpoch + 2, MaxBlobsPerBlock: 21},
	}
	cfg.InitializeForkSchedule()

	r := e2eMinimal(t, cfg,
		types.WithEpochs(10),
		types.WithExitEpoch(4), // Minimum due to ShardCommitteePeriod=4
		types.WithLargeBlobs(), // Use large blob transactions for BPO testing
		withoutEvaluators(
			ev.ValidatorSyncParticipation.Name,
			ev.ValidatorsVoteWithTheMajority.Name,
			ev.ProcessesDepositsInBlocks.Name,
			ev.ActivatesDepositedValidators.Name,
		),
	)
	r.run()
}

// TestEndToEnd_DecoupledHezeConfig is TestEndToEnd_DecoupledConfig with the
// Heze fork scheduled at epoch 6. Heze is a consensus-only fork (fork-field
// bump, digest rotation, available-attestation stream); the run proves the
// chain keeps proposing, attesting, and finalizing across the fork — the
// data12 shadow sim failure mode (post-fork signature mismatch) regresses
// here. Gloas stays unscheduled: the e2e harness has no Gloas evaluators, so
// this exercises the Fulu-shaped Heze path.
func TestEndToEnd_DecoupledHezeConfig(t *testing.T) {
	cfg := params.E2EDecoupledTestConfig()
	cfg = types.InitForkCfg(version.Electra, version.Fulu, cfg)
	// Set Fulu fork at epoch 2 for a quick fork transition test
	cfg.FuluForkEpoch = 2
	// Update BlobSchedule to use the new FuluForkEpoch for BPO testing
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: cfg.DenebForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlock)},
		{Epoch: cfg.ElectraForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)},
		// BPO (Blob Parameter Optimization) schedule for Fulu
		{Epoch: cfg.FuluForkEpoch + 1, MaxBlobsPerBlock: 15},
		{Epoch: cfg.FuluForkEpoch + 2, MaxBlobsPerBlock: 21},
	}
	cfg.HezeForkEpoch = 6
	cfg.InitializeForkSchedule()

	r := e2eMinimal(t, cfg,
		types.WithEpochs(10),
		types.WithExitEpoch(4), // Minimum due to ShardCommitteePeriod=4
		types.WithLargeBlobs(), // Use large blob transactions for BPO testing
		withoutEvaluators(
			ev.ValidatorSyncParticipation.Name,
			ev.ValidatorsVoteWithTheMajority.Name,
			ev.ProcessesDepositsInBlocks.Name,
			ev.ActivatesDepositedValidators.Name,
		),
	)
	r.run()
}

// withoutEvaluators drops the named evaluators from the run.
func withoutEvaluators(names ...string) types.E2EConfigOpt {
	return func(c *types.E2EConfig) {
		drop := make(map[string]bool, len(names))
		for _, n := range names {
			drop[n] = true
		}
		kept := c.Evaluators[:0]
		for _, e := range c.Evaluators {
			if !drop[e.Name] {
				kept = append(kept, e)
			}
		}
		c.Evaluators = kept
	}
}

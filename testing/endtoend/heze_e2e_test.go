package endtoend

import (
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_HezeGenesis runs the e2e suite on a chain whose genesis state is
// already a Heze state. There is no fork transition anywhere in the run: the
// mainnet-preset e2e config schedules every fork, Gloas and Heze included, at
// epoch 0.
//
// SLOTS_PER_EPOCH stays at the mainnet 32 while SLOTS_PER_ROUND is 8, so an
// epoch holds four rounds and every validator attests four times per epoch. At
// the e2e config's 6-second slots an epoch takes 3.2 minutes.
//
// The run is five epochs rather than four. Justification is skipped for the
// first two epochs (the spec returns early while the current epoch is at most
// GENESIS_EPOCH + 1), so the first finalized checkpoint only appears during
// epoch 4, and the evaluator loop stops at epoch EpochsToRun-1.
//
// Evaluators dropped relative to the stock minimal run:
//   - VerifyBlockGraffiti, FeeRecipientIsPresent, ValidatorsVoteWithTheMajority,
//     ProcessesDepositsInBlocks: all read blocks over ListBeaconBlocks, and
//     BeaconBlockContainer has no arm for a Gloas-shaped block.
//   - ValidatorSyncParticipation: reads blocks the same way, and the mainnet
//     SyncCommitteeSize (512) exceeds the 256 genesis validators anyway.
//   - ActivatesDepositedValidators: mainnet EpochsPerEth1VotingPeriod (64) puts
//     the eth1 voting period far beyond the run; post-genesis deposits arrive as
//     execution requests instead.
//
// Three evaluators are added: ChainProducesBlocks, which fails a run whose
// chain never left genesis, AvailableAttestationsFlow, which proves the Heze
// available attestation topic carries traffic on every node, and
// AttestationsInEveryRound, which proves committee attestations happen in all
// four rounds of an epoch rather than only the first.
func TestEndToEnd_HezeGenesis(t *testing.T) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	// Four rounds to the 32-slot epoch. SLOTS_PER_EPOCH is untouched, so every
	// SSZ array keeps its mainnet length and no preset change is needed.
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(5),
		withoutEvaluators(hezeDroppedEvaluators...),
		withEvaluators(
			ev.ChainProducesBlocks,
			ev.AvailableAttestationsFlow,
			ev.AttestationsInEveryRound,
		),
	)
	r.run()
}

// hezeDroppedEvaluators are the stock minimal evaluators the Heze runs cannot
// use; the reason for each is on TestEndToEnd_HezeGenesis.
var hezeDroppedEvaluators = []string{
	ev.VerifyBlockGraffiti.Name,
	ev.FeeRecipientIsPresent.Name,
	ev.ValidatorsVoteWithTheMajority.Name,
	ev.ProcessesDepositsInBlocks.Name,
	ev.ValidatorSyncParticipation.Name,
	ev.ActivatesDepositedValidators.Name,
}

// TestEndToEnd_HezeGenesisSlotStartFFG is the Heze run with the FFG vote cast
// at the start of the slot instead of at the attestation due time.
//
// The vote then names the previous slot's block as head, so is_matching_head is
// missed every slot: 14/64 of the attestation reward, which the task's charter
// allows to be wrong. The FFG target is not affected - it is the block at
// StartSlot(E)-1, which a voter at the start of any slot of epoch E has already
// seen - and participation is measured on the target, so the relaxed floor here
// only leaves slack for the vote's jitter. It is relaxed for this run only; the
// default TestEndToEnd_HezeGenesis keeps the stock expectation.
func TestEndToEnd_HezeGenesisSlotStartFFG(t *testing.T) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(5),
		types.WithSlotStartFFGVote(),
		withoutEvaluators(append(slices.Clone(hezeDroppedEvaluators),
			ev.ValidatorsParticipatingAtEpoch(2).Name)...),
		withEvaluators(
			ev.ValidatorsParticipatingAtEpochWithFloor(2, 0.95),
			ev.ChainProducesBlocks,
			ev.AvailableAttestationsFlow,
			ev.AttestationsInEveryRound,
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

// withEvaluators adds evaluators to the run.
func withEvaluators(evals ...types.Evaluator) types.E2EConfigOpt {
	return func(c *types.E2EConfig) {
		c.Evaluators = append(c.Evaluators, evals...)
	}
}

// TestEndToEnd_HezeGenesisShort is the cheap shakeout tier of the Heze run: one
// epoch, no sync or deposit phase, and only the evaluators that fire that early.
// Three minutes instead of nineteen. It is a smoke test, not a result.
func TestEndToEnd_HezeGenesisShort(t *testing.T) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(1),
		func(c *types.E2EConfig) {
			c.TestSync = false
			c.TestDeposits = false
			c.TestFeature = false
		},
		withoutEvaluators(hezeDroppedEvaluators...),
		withEvaluators(ev.ChainProducesBlocks, ev.AvailableAttestationsFlow),
	)
	r.run()
}

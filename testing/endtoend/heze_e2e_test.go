package endtoend

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/components/eth1"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
// The run is five epochs rather than four. Justification is skipped while the
// state slot is at or below EpochStart(2) -- the spec's genesis guard, which the
// plan deliberately keeps epoch-based -- so nothing justifies before slot 64.
// Round 8 is the first to justify, one round later at slot 72, and finalizes two
// rounds after it closes, by slot 80: inside epoch 2. The extra epochs are
// headroom for the evaluators, which stop at epoch EpochsToRun-1.
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
//
// The stock FinalizationOccurs is swapped for FinalizationOccursInRounds and
// JustificationAdvancesEveryRound: FFG now runs once per round, so finality has
// to be judged in rounds and its per-round progression asserted directly. The
// one-epoch Short run only drops the stock evaluator - neither replacement can
// fire that early.
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
			ev.FinalizationOccursInRounds(3),
			ev.JustificationAdvancesEveryRound,
		),
	)
	r.run()
}

// hezeDroppedEvaluators are the stock minimal evaluators the Heze runs cannot
// use; the reason for each is on TestEndToEnd_HezeGenesis.
var hezeDroppedEvaluators = []string{
	// Replaced by hezeFinalityEvaluators: this one reads the chain head's
	// round-valued checkpoints as epochs.
	ev.FinalizationOccurs(0).Name,
	ev.VerifyBlockGraffiti.Name,
	ev.FeeRecipientIsPresent.Name,
	ev.ValidatorsVoteWithTheMajority.Name,
	ev.ProcessesDepositsInBlocks.Name,
	ev.ValidatorSyncParticipation.Name,
	ev.ActivatesDepositedValidators.Name,
}

// TestEndToEnd_HezeGenesisCheckpointSync is the cold-start witness: a beacon
// node that joins the chain late, from a finalized checkpoint rather than from
// genesis, and has to reach the head and hold it.
//
// The goldfish design deliberately deviates from the spec so that a node
// joining late can move: the empty-vote-slot gate abstains instead of vetoing
// the walk, because a node importing history has no available attestation for
// any slot it imports and none will ever arrive. Nothing else in the suite
// exercises that: every other node either starts at genesis or is already at
// the tip. This run is the witness for it.
//
// It takes five epochs because the checkpoint has to be real. The Heze chain
// finalizes nothing before slot 80 -- the epoch-2 genesis guard holds
// justification off until slot 64, and finalization trails by two rounds -- so a
// node that checkpoint-synced any earlier would receive the genesis state as its
// "finalized" state and would in fact be syncing from genesis. The extra epochs
// also leave the checkpoint far enough behind head to be worth syncing to.
// testCheckpointSync fails the run when the origin block slot it reads out of
// the joining node's own log is zero, so a silent fall back to genesis sync
// cannot pass here.
//
// The run is otherwise leaner than TestEndToEnd_HezeGenesis: no deposit phase
// and no genesis-sync node, since the joining node is the whole point. The
// transaction generator still runs (TestFeature keeps it on) so the chain the
// node syncs carries payloads and blobs.
//
// If this run ever fails at all_nodes_have_same_head after the joining node
// has already matched the head once, look at the initial-sync handoff before
// suspecting the checkpoint. Initial sync hands over at whatever slot it
// reached, and when that is a few slots behind the head the joining node has
// to import the gap through the pending-block queue, which calls
// processPendingPayloadEnvelope synchronously. Two things keep that path
// moving on a Gloas chain, and a regression in either one shows up here as a
// multi-minute "Processed pending block and cleared it in cache" duration:
// the envelope fetches its block's data columns by root when its slot is
// already behind the head, because the gossip that carried them happened
// while this node was still syncing, and the import itself is capped at three
// slots so a wait for data that never arrives cannot hold the queue.
func TestEndToEnd_HezeGenesisCheckpointSync(t *testing.T) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(5),
		types.WithCheckpointSync(),
		func(c *types.E2EConfig) {
			// The joining node is the witness; a second genesis-syncing node
			// and the deposit phase would only add wall time.
			c.TestSync = false
			c.TestDeposits = false
		},
		withoutEvaluators(hezeDroppedEvaluators...),
		withEvaluators(
			ev.ChainProducesBlocks,
			ev.AvailableAttestationsFlow,
			ev.AttestationsInEveryRound,
			ev.FinalizationOccursInRounds(3),
			ev.JustificationAdvancesEveryRound,
		),
	)
	r.run()
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
			ev.FinalizationOccursInRounds(3),
			ev.JustificationAdvancesEveryRound,
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

// hezeELWitness builds the direct execution-layer witness runs: no sync or
// deposit phase, and no evaluator loop. The whole run is the witness: as soon
// as the EL produces its first engine-built block, the test itself sends a
// value transfer from the pre-funded miner account and a cell-proof blob
// transaction from the genesis-funded blob account (geth reserves each sender
// to one txpool subpool, so they cannot share an account), then asserts the
// receipts, the credited recipient balance, and consumed blob gas a few slots
// later. Total wall time is node startup plus a handful of slots.
//
// The witness is direct because every other evaluator passes on a chain that
// carries no transactions at all, or whose every transaction fails with
// status 0 (as happened when Amsterdam repriced plain transfers past the
// tools' hardcoded 21000 gas).
func hezeELWitness(t *testing.T, wantTransfer, wantBlob bool) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(1),
		func(c *types.E2EConfig) {
			c.TestSync = false
			c.TestDeposits = false
			c.TestFeature = false
			// The witness run replaces the evaluator loop entirely.
			c.Evaluators = nil
		},
	)
	r.runBase([]runEvent{r.hezeELWitnessRun(wantTransfer, wantBlob)})
}

// hezeELStateWindow is the send-and-verify tier of the EL checks: for
// windowSlots slots after chain start the test itself sends one value
// transfer and one single-blob transaction per slot from two genesis-funded
// accounts, and after the window it verifies the resulting chain state
// exactly - every receipt, every balance to the wei, both nonces, the blob
// gas, and that every pair landed in its own block. No generator, no
// funding transfers, no miner account: nothing here depends on warm-up.
func hezeELStateWindow(t *testing.T, windowSlots int) {
	cfg := params.E2EMainnetTestConfig()
	cfg = types.InitForkCfg(version.Heze, version.Heze, cfg)
	cfg.SlotsPerRound = 8

	r := e2eMinimal(t, cfg,
		types.WithEpochs(1),
		func(c *types.E2EConfig) {
			c.TestSync = false
			c.TestDeposits = false
			c.TestFeature = false
			// The window run judges the chain itself; no evaluator loop.
			c.Evaluators = nil
		},
	)
	r.runBase([]runEvent{r.hezeELStateWindowRun(windowSlots)})
}

func (r *testRunner) hezeELStateWindowRun(windowSlots int) runEvent {
	return func() error {
		defer func() {
			log.Info("EL state window finished, cleaning up")
			r.comHandler.done()
		}()
		ctxAllNodesReady, cancel := context.WithTimeout(r.comHandler.ctx, allNodesStartTimeout)
		defer cancel()
		if err := helpers.ComponentsStarted(ctxAllNodesReady, r.comHandler.required()); err != nil {
			return errors.Wrap(err, "components take too long to start")
		}
		r.comHandler.printPIDs(r.t.Logf)
		defer helpers.LogOutput(r.t)
		return r.hezeELStateWindowCheck(windowSlots)
	}
}

// hezeELStateWindowCheck sends the per-slot traffic and verifies the state.
func (r *testRunner) hezeELStateWindowCheck(windowSlots int) error {
	ctx := r.comHandler.ctx
	client, err := helpers.MinerRPCClient()
	if err != nil {
		return errors.Wrap(err, "el window: dial miner rpc")
	}
	transferKey := eth1.BlobV0SenderKey()
	blobKey := eth1.BlobSenderKey()

	// Submit every transaction to every EL node: blocks are proposed by all
	// of them in turn, and blob transactions do not gossip fast enough to
	// reach a peer's payload in the same slot (eth/68 only announces blob
	// transactions; the peer then pulls and proof-verifies them). The test
	// verifies inclusion and state, not gossip latency.
	clients := []*ethclient.Client{client}
	for i := 1; i < e2e.TestParams.BeaconNodeCount; i++ {
		c, err := ethclient.Dial(e2e.TestParams.Eth1RPCURL(i).String())
		if err != nil {
			return errors.Wrapf(err, "el window: dial eth1 node %d", i)
		}
		defer c.Close()
		clients = append(clients, c)
	}
	sendAll := func(tx *ethtypes.Transaction, what string) error {
		for i, c := range clients {
			err := c.SendTransaction(ctx, tx)
			if err != nil && !strings.Contains(err.Error(), "already known") {
				return errors.Wrapf(err, "%s to node %d", what, i)
			}
		}
		return nil
	}

	// Chain start: the first engine-built EL block exists.
	for {
		head, err := client.BlockNumber(ctx)
		if err == nil && head >= 1 {
			break
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "el window: EL never produced a block")
		case <-time.After(500 * time.Millisecond):
		}
	}

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return errors.Wrap(err, "el window: chain id")
	}
	genesisNumber := big.NewInt(0)
	transferStartBal, err := client.BalanceAt(ctx, transferKey.Address, genesisNumber)
	if err != nil {
		return errors.Wrap(err, "el window: transfer sender genesis balance")
	}
	blobStartBal, err := client.BalanceAt(ctx, blobKey.Address, genesisNumber)
	if err != nil {
		return errors.Wrap(err, "el window: blob sender genesis balance")
	}
	if transferStartBal.Sign() == 0 || blobStartBal.Sign() == 0 {
		return errors.New("el window: sender accounts are not funded in genesis")
	}

	slot := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	transferTo := ethcommon.HexToAddress("0x00000000000000000000000000000000e2ee2ee1")
	transferValue := big.NewInt(1_000_000_000_000_000_000) // 1 ETH
	blobTo := ethcommon.HexToAddress("0x00000000000000000000000000000000b10bb10b")
	transferHashes := make([]ethcommon.Hash, 0, windowSlots)
	blobHashes := make([]ethcommon.Hash, 0, windowSlots)

	sendBlob := func(nonce uint64, tip, gasPrice *big.Int, payload string) (ethcommon.Hash, error) {
		blobTx, err := eth1.New4844CellTx(nonce, &blobTo, 500_000, chainID, tip, gasPrice,
			big.NewInt(0), nil, big.NewInt(1_000_000), []byte(payload), make(ethtypes.AccessList, 0))
		if err != nil {
			return ethcommon.Hash{}, errors.Wrap(err, "build blob tx")
		}
		signed, err := ethtypes.SignTx(blobTx, ethtypes.NewCancunSigner(chainID), blobKey.PrivateKey)
		if err != nil {
			return ethcommon.Hash{}, errors.Wrap(err, "sign blob tx")
		}
		if err := sendAll(signed, "send blob tx"); err != nil {
			return ethcommon.Hash{}, err
		}
		return signed.Hash(), nil
	}

	// Prime the blob path before the measured window: a fresh geth loads its
	// KZG trusted setup lazily while admitting the first blob transaction it
	// ever sees, which can push that blob one block late. The primer pays
	// that one-time cost outside the window; it is verified and accounted
	// for like everything else the test sends.
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return errors.Wrap(err, "el window: primer gas price")
	}
	tip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return errors.Wrap(err, "el window: primer gas tip")
	}
	primerHash, err := sendBlob(0, tip, gasPrice, "heze el window primer blob")
	if err != nil {
		return errors.Wrap(err, "el window: primer")
	}
	primerDeadline := time.Now().Add(4 * slot)
	var primer *ethtypes.Receipt
	for {
		primer, err = client.TransactionReceipt(ctx, primerHash)
		if err == nil {
			break
		}
		if time.Now().After(primerDeadline) {
			return errors.Errorf("el window: primer blob %#x never included", primerHash)
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "el window: waiting for primer blob")
		case <-time.After(500 * time.Millisecond):
		}
	}
	if primer.Status != ethtypes.ReceiptStatusSuccessful {
		return errors.Errorf("el window: primer blob failed: status %d", primer.Status)
	}

	// One transfer and one blob transaction per slot for the whole window.
	// Each pair goes out right after a block import - the start of a slot -
	// so the pool has the entire slot to admit the blob (cell-proof
	// verification makes blob admission lag plain transactions) before the
	// next payload is sealed. Sending on a fixed sleep instead lets the send
	// phase drift into the end of the slot, where the transfer still catches
	// the payload but the blob does not.
	head, err := client.BlockNumber(ctx)
	if err != nil {
		return errors.Wrap(err, "el window: head before window")
	}
	waitNextBlock := func() error {
		for {
			h, err := client.BlockNumber(ctx)
			if err == nil && h > head {
				head = h
				return nil
			}
			select {
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "el window: chain stopped mid-window")
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	sendHeads := make([]uint64, 0, windowSlots)
	for i := range windowSlots {
		sendHeads = append(sendHeads, head)
		gasPrice, err = client.SuggestGasPrice(ctx)
		if err != nil {
			return errors.Wrapf(err, "el window: gas price, slot %d", i)
		}
		tip, err = client.SuggestGasTipCap(ctx)
		if err != nil {
			return errors.Wrapf(err, "el window: gas tip, slot %d", i)
		}
		gas, err := client.EstimateGas(ctx, ethereum.CallMsg{
			From: transferKey.Address, To: &transferTo, Value: transferValue,
		})
		if err != nil {
			return errors.Wrapf(err, "el window: estimate transfer gas, slot %d", i)
		}
		tx := ethtypes.NewTransaction(uint64(i), transferTo, transferValue, gas, gasPrice, nil)
		signed, err := ethtypes.SignTx(tx, ethtypes.NewLondonSigner(chainID), transferKey.PrivateKey)
		if err != nil {
			return errors.Wrapf(err, "el window: sign transfer %d", i)
		}
		if err := sendAll(signed, fmt.Sprintf("el window: send transfer %d", i)); err != nil {
			return err
		}
		transferHashes = append(transferHashes, signed.Hash())
		// The primer holds blob nonce 0.
		blobHash, err := sendBlob(uint64(i)+1, tip, gasPrice, fmt.Sprintf("heze el window blob %d", i))
		if err != nil {
			return errors.Wrapf(err, "el window: blob %d", i)
		}
		blobHashes = append(blobHashes, blobHash)

		if err := waitNextBlock(); err != nil {
			return err
		}
	}

	// The last pair needs its block: up to two more slots of inclusion grace.
	deadline := time.Now().Add(2 * slot)
	receiptOf := func(h ethcommon.Hash, what string, i int) (*ethtypes.Receipt, error) {
		for {
			receipt, err := client.TransactionReceipt(ctx, h)
			if err == nil {
				return receipt, nil
			}
			if time.Now().After(deadline) {
				return nil, errors.Errorf("el window: %s %d (%#x) never included", what, i, h)
			}
			select {
			case <-ctx.Done():
				return nil, errors.Wrapf(ctx.Err(), "el window: waiting for %s %d", what, i)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	// Verification: the exact post-state of everything the window sent. Block
	// placement is judged by promptness, not by one-pair-per-block: at round
	// starts (SlotsPerRound=8) the Heze design deliberately replaces the tip
	// with the stable-root proposal, and a reorged pair legally re-lands
	// merged into the winning block.
	transferFees := new(big.Int)
	blobFees := new(big.Int)
	blobFees.Add(blobFees, new(big.Int).Mul(new(big.Int).SetUint64(primer.GasUsed), primer.EffectiveGasPrice))
	blobFees.Add(blobFees, new(big.Int).Mul(new(big.Int).SetUint64(primer.BlobGasUsed), primer.BlobGasPrice))
	blobGasTotal := primer.BlobGasUsed
	blobGasByBlock := make(map[uint64]uint64, windowSlots)
	const inclusionGraceBlocks = 2
	for i := range windowSlots {
		tr, err := receiptOf(transferHashes[i], "transfer", i)
		if err != nil {
			return err
		}
		if tr.Status != ethtypes.ReceiptStatusSuccessful {
			return errors.Errorf("el window: transfer %d failed: status %d in block %s", i, tr.Status, tr.BlockNumber)
		}
		transferFees.Add(transferFees, new(big.Int).Mul(new(big.Int).SetUint64(tr.GasUsed), tr.EffectiveGasPrice))

		br, err := receiptOf(blobHashes[i], "blob tx", i)
		if err != nil {
			return err
		}
		if br.Status != ethtypes.ReceiptStatusSuccessful {
			return errors.Errorf("el window: blob tx %d failed: status %d in block %s", i, br.Status, br.BlockNumber)
		}
		if br.BlobGasUsed != uint64(ethparams.BlobTxBlobGasPerBlob) {
			return errors.Errorf("el window: blob tx %d consumed %d blob gas, want exactly one blob (%d)",
				i, br.BlobGasUsed, ethparams.BlobTxBlobGasPerBlob)
		}
		blobGasTotal += br.BlobGasUsed
		blobGasByBlock[br.BlockNumber.Uint64()] += br.BlobGasUsed
		blobFees.Add(blobFees, new(big.Int).Mul(new(big.Int).SetUint64(br.GasUsed), br.EffectiveGasPrice))
		blobFees.Add(blobFees, new(big.Int).Mul(new(big.Int).SetUint64(br.BlobGasUsed), br.BlobGasPrice))

		// Prompt inclusion: both transactions of the pair are canonical
		// within the block after next from where they were sent.
		for what, rc := range map[string]*ethtypes.Receipt{"transfer": tr, "blob tx": br} {
			if rc.BlockNumber.Uint64() > sendHeads[i]+inclusionGraceBlocks {
				return errors.Errorf("el window: %s %d sent at head %d only included in block %s",
					what, i, sendHeads[i], rc.BlockNumber)
			}
		}
	}
	// Every blob-carrying block's header must account for exactly the blobs
	// this test put there: no other blob source exists in this run.
	for number, want := range blobGasByBlock {
		header, err := client.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			return errors.Wrapf(err, "el window: header of block %d", number)
		}
		if header.BlobGasUsed == nil || *header.BlobGasUsed != want {
			return errors.Errorf("el window: block %d header blob gas does not match the %d blob gas sent there",
				number, want)
		}
	}

	// Balance sheet, to the wei.
	wantRecipient := new(big.Int).Mul(transferValue, big.NewInt(int64(windowSlots)))
	gotRecipient, err := client.BalanceAt(ctx, transferTo, nil)
	if err != nil {
		return errors.Wrap(err, "el window: recipient balance")
	}
	if gotRecipient.Cmp(wantRecipient) != 0 {
		return errors.Errorf("el window: recipient holds %s wei, want %s", gotRecipient, wantRecipient)
	}
	wantTransferBal := new(big.Int).Sub(transferStartBal, wantRecipient)
	wantTransferBal.Sub(wantTransferBal, transferFees)
	gotTransferBal, err := client.BalanceAt(ctx, transferKey.Address, nil)
	if err != nil {
		return errors.Wrap(err, "el window: transfer sender balance")
	}
	if gotTransferBal.Cmp(wantTransferBal) != 0 {
		return errors.Errorf("el window: transfer sender holds %s wei, want %s (start - values - fees)",
			gotTransferBal, wantTransferBal)
	}
	wantBlobBal := new(big.Int).Sub(blobStartBal, blobFees)
	gotBlobBal, err := client.BalanceAt(ctx, blobKey.Address, nil)
	if err != nil {
		return errors.Wrap(err, "el window: blob sender balance")
	}
	if gotBlobBal.Cmp(wantBlobBal) != 0 {
		return errors.Errorf("el window: blob sender holds %s wei, want %s (start - fees - blob fees)",
			gotBlobBal, wantBlobBal)
	}
	wantNonces := map[string]uint64{"transfer": uint64(windowSlots), "blob": uint64(windowSlots) + 1}
	for who, key := range map[string]*keystore.Key{"transfer": transferKey, "blob": blobKey} {
		nonce, err := client.NonceAt(ctx, key.Address, nil)
		if err != nil {
			return errors.Wrapf(err, "el window: %s sender nonce", who)
		}
		if nonce != wantNonces[who] {
			return errors.Errorf("el window: %s sender nonce %d, want %d", who, nonce, wantNonces[who])
		}
	}

	log.WithField("pairs", windowSlots).WithField("blobGas", blobGasTotal).
		WithField("blobBlocks", len(blobGasByBlock)).Info("EL state window verified")
	return nil
}

// hezeELWitnessRun is the whole run for the EL witness tests. It stands in
// for defaultEndToEndRun so the test ends a few slots after chain start
// instead of waiting out the epoch-shaped evaluator ticker.
func (r *testRunner) hezeELWitnessRun(wantTransfer, wantBlob bool) runEvent {
	return func() error {
		defer func() {
			log.Info("EL witness finished, cleaning up")
			r.comHandler.done()
		}()
		ctxAllNodesReady, cancel := context.WithTimeout(r.comHandler.ctx, allNodesStartTimeout)
		defer cancel()
		if err := helpers.ComponentsStarted(ctxAllNodesReady, r.comHandler.required()); err != nil {
			return errors.Wrap(err, "components take too long to start")
		}
		r.comHandler.printPIDs(r.t.Logf)
		// Written before the deferred done() above cancels the components.
		defer helpers.LogOutput(r.t)
		return r.hezeELDirectCheck(wantTransfer, wantBlob)
	}
}

// hezeELDirectCheck sends the witness transactions and asserts their effects.
func (r *testRunner) hezeELDirectCheck(wantTransfer, wantBlob bool) error {
	ctx := r.comHandler.ctx
	client, err := helpers.MinerRPCClient()
	if err != nil {
		return errors.Wrap(err, "el witness: dial miner rpc")
	}
	keyPath, err := e2e.TestParams.Paths.MinerKeyPath()
	if err != nil {
		return errors.Wrap(err, "el witness: miner key path")
	}
	minerKey, err := helpers.KeyFromPath(keyPath, eth1.KeystorePassword)
	if err != nil {
		return errors.Wrap(err, "el witness: decrypt miner key")
	}

	// Chain start: the first engine-built EL block exists.
	for {
		head, err := client.BlockNumber(ctx)
		if err == nil && head >= 1 {
			break
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "el witness: EL never produced a block")
		case <-time.After(500 * time.Millisecond):
		}
	}

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return errors.Wrap(err, "el witness: chain id")
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return errors.Wrap(err, "el witness: gas price")
	}

	var transferHash, blobHash ethcommon.Hash
	transferTo := ethcommon.HexToAddress("0x00000000000000000000000000000000e2ee2ee1")
	transferValue := big.NewInt(1_000_000_000_000_000_000) // 1 ETH
	if wantTransfer {
		nonce, err := client.PendingNonceAt(ctx, minerKey.Address)
		if err != nil {
			return errors.Wrap(err, "el witness: miner pending nonce")
		}
		// Estimated, not hardcoded: Amsterdam repriced a plain transfer
		// from 21000 to ~207k gas.
		gas, err := client.EstimateGas(ctx, ethereum.CallMsg{
			From: minerKey.Address, To: &transferTo, Value: transferValue,
		})
		if err != nil {
			return errors.Wrap(err, "el witness: estimate transfer gas")
		}
		tx := ethtypes.NewTransaction(nonce, transferTo, transferValue, gas, gasPrice, nil)
		signed, err := ethtypes.SignTx(tx, ethtypes.NewLondonSigner(chainID), minerKey.PrivateKey)
		if err != nil {
			return errors.Wrap(err, "el witness: sign transfer")
		}
		if err := client.SendTransaction(ctx, signed); err != nil {
			return errors.Wrap(err, "el witness: send transfer")
		}
		transferHash = signed.Hash()
		log.WithField("hash", transferHash.Hex()).WithField("gas", gas).
			Info("EL witness sent value transfer")
	}
	if wantBlob {
		// Heze schedules Fulu at genesis, so blob sidecars are V1 cell proofs.
		blobKey := eth1.BlobSenderKey()
		nonce, err := client.PendingNonceAt(ctx, blobKey.Address)
		if err != nil {
			return errors.Wrap(err, "el witness: blob sender pending nonce")
		}
		tip, err := client.SuggestGasTipCap(ctx)
		if err != nil {
			return errors.Wrap(err, "el witness: gas tip")
		}
		blobTo := ethcommon.HexToAddress("0x00000000000000000000000000000000b10bb10b")
		tx, err := eth1.New4844CellTx(nonce, &blobTo, 500_000, chainID, tip, gasPrice,
			big.NewInt(0), nil, big.NewInt(1_000_000), []byte("heze el witness blob"),
			make(ethtypes.AccessList, 0))
		if err != nil {
			return errors.Wrap(err, "el witness: build blob tx")
		}
		signed, err := ethtypes.SignTx(tx, ethtypes.NewCancunSigner(chainID), blobKey.PrivateKey)
		if err != nil {
			return errors.Wrap(err, "el witness: sign blob tx")
		}
		if err := client.SendTransaction(ctx, signed); err != nil {
			return errors.Wrap(err, "el witness: send blob tx")
		}
		blobHash = signed.Hash()
		log.WithField("hash", blobHash.Hex()).Info("EL witness sent blob transaction")
	}

	// Two slots is the inclusion budget: sent during slot N, included by the
	// proposer of N+1. A third slot absorbs a single missed proposal.
	budget := 3 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second
	deadline := time.Now().Add(budget)
	receiptBy := func(h ethcommon.Hash, name string) (*ethtypes.Receipt, error) {
		for {
			receipt, err := client.TransactionReceipt(ctx, h)
			if err == nil {
				return receipt, nil
			}
			if time.Now().After(deadline) {
				return nil, errors.Errorf("el witness: %s %#x not included within %s", name, h, budget)
			}
			select {
			case <-ctx.Done():
				return nil, errors.Wrapf(ctx.Err(), "el witness: waiting for %s receipt", name)
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	if wantTransfer {
		receipt, err := receiptBy(transferHash, "transfer")
		if err != nil {
			return err
		}
		if receipt.Status != ethtypes.ReceiptStatusSuccessful {
			return errors.Errorf(
				"el witness: transfer failed: status %d, block %s, gasUsed %d",
				receipt.Status, receipt.BlockNumber, receipt.GasUsed)
		}
		bal, err := client.BalanceAt(ctx, transferTo, nil)
		if err != nil {
			return errors.Wrap(err, "el witness: recipient balance")
		}
		if bal.Cmp(transferValue) != 0 {
			return errors.Errorf("el witness: transfer succeeded but recipient holds %s wei, want %s",
				bal, transferValue)
		}
		log.WithField("block", receipt.BlockNumber).WithField("gasUsed", receipt.GasUsed).
			Info("EL witness transfer credited")
	}
	if wantBlob {
		receipt, err := receiptBy(blobHash, "blob tx")
		if err != nil {
			return err
		}
		if receipt.Status != ethtypes.ReceiptStatusSuccessful {
			return errors.Errorf(
				"el witness: blob tx failed: status %d, block %s, gasUsed %d",
				receipt.Status, receipt.BlockNumber, receipt.GasUsed)
		}
		if receipt.BlobGasUsed == 0 {
			return errors.New("el witness: blob tx succeeded but consumed no blob gas")
		}
		// The including block's header must account for the blob gas too.
		header, err := client.HeaderByNumber(ctx, receipt.BlockNumber)
		if err != nil {
			return errors.Wrap(err, "el witness: header of blob block")
		}
		if header.BlobGasUsed == nil || *header.BlobGasUsed == 0 {
			return errors.Errorf("el witness: block %s consumed no blob gas", receipt.BlockNumber)
		}
		log.WithField("block", receipt.BlockNumber).WithField("blobGasUsed", receipt.BlobGasUsed).
			Info("EL witness blob landed")
	}
	return nil
}

// TestEndToEnd_HezeGenesisTransactions: a value transfer sent at chain start
// is included within two slots and credits its recipient.
func TestEndToEnd_HezeGenesisTransactions(t *testing.T) {
	hezeELWitness(t, true, false)
}

// TestEndToEnd_HezeGenesisBlobTx: a blob transaction sent at chain start is
// included within two slots and consumes blob gas.
func TestEndToEnd_HezeGenesisBlobTx(t *testing.T) {
	hezeELWitness(t, false, true)
}

// TestEndToEnd_HezeGenesisEL: both execution-layer witnesses together.
func TestEndToEnd_HezeGenesisEL(t *testing.T) {
	hezeELWitness(t, true, true)
}

// TestEndToEnd_HezeGenesisELFlowShort: six slots, each carrying one value
// transfer and one blob transaction, with the whole resulting chain state
// verified after the window.
func TestEndToEnd_HezeGenesisELFlowShort(t *testing.T) {
	hezeELStateWindow(t, 6)
}

// TestEndToEnd_HezeGenesisELFlowEpoch: the same send-and-verify window held
// open for a full epoch.
func TestEndToEnd_HezeGenesisELFlowEpoch(t *testing.T) {
	hezeELStateWindow(t, int(params.E2EMainnetTestConfig().SlotsPerEpoch))
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

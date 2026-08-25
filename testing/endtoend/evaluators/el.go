package evaluators

import (
	"context"
	"fmt"
	"math/big"

	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// The rest of the suite only proves consensus wiring: a chain whose every
// transaction fails with status 0 (as happened when Amsterdam repriced
// plain transfers past the tools' hardcoded 21000 gas), or that never
// carries a transaction at all, still finalizes and looks green. These
// evaluators scan the recent EL blocks and fail the run when execution
// effects are missing.

// ELTransactionsCredit requires that transactions are included at all,
// that not every one of them failed, and that at least one value-carrying
// transfer succeeded and its recipient's balance shows it.
var ELTransactionsCredit = types.Evaluator{
	Name:       "el_transactions_credit_%d",
	Policy:     policies.AllEpochs,
	Evaluation: elTransactionsCredit,
}

// ELBlobsLand requires that at least one blob transaction landed: some
// recent block consumed blob gas.
var ELBlobsLand = types.Evaluator{
	Name:       "el_blobs_land_%d",
	Policy:     policies.AllEpochs,
	Evaluation: elBlobsLand,
}

// elScan is one pass over the recent EL blocks, shared by the evaluators.
type elScan struct {
	start, head   uint64
	txCount       int
	statusOK      int
	statusFailed  int
	creditedValue bool
	blobLanded    bool
}

func scanRecentELBlocks() (*elScan, error) {
	rpcClient, err := rpc.DialHTTP(fmt.Sprintf("http://127.0.0.1:%d", e2e.TestParams.Ports.Eth1RPCPort))
	if err != nil {
		return nil, errors.Wrap(err, "failed to dial eth1 rpc")
	}
	defer rpcClient.Close()
	client := ethclient.NewClient(rpcClient)
	ctx := context.Background()

	head, err := client.BlockNumber(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get eth1 head")
	}
	sc := &elScan{head: head, start: 1}
	if head > 32 {
		sc.start = head - 32
	}

	for n := sc.start; n <= head; n++ {
		block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(n))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get eth1 block %d", n)
		}
		if bg := block.BlobGasUsed(); bg != nil && *bg > 0 {
			sc.blobLanded = true
		}
		for _, tx := range block.Transactions() {
			sc.txCount++
			receipt, err := client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get receipt for tx %#x", tx.Hash())
			}
			if receipt.Status != ethtypes.ReceiptStatusSuccessful {
				sc.statusFailed++
				continue
			}
			sc.statusOK++
			if tx.Value().Sign() > 0 && tx.To() != nil && !sc.creditedValue {
				bal, err := client.BalanceAt(ctx, *tx.To(), nil)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to get balance of %#x", *tx.To())
				}
				if bal.Sign() > 0 {
					sc.creditedValue = true
				}
			}
		}
	}
	return sc, nil
}

func (sc *elScan) String() string {
	return fmt.Sprintf("eth1 blocks %d-%d: %d txs, %d ok, %d failed",
		sc.start, sc.head, sc.txCount, sc.statusOK, sc.statusFailed)
}

func elTransactionsCredit(_ *types.EvaluationContext, _ ...*grpc.ClientConn) error {
	sc, err := scanRecentELBlocks()
	if err != nil {
		return err
	}
	if sc.txCount == 0 {
		return fmt.Errorf("no transactions in %s: the transaction flow is dead", sc)
	}
	if sc.statusOK == 0 {
		return fmt.Errorf("every transaction failed (%s): execution burns gas without effects", sc)
	}
	if !sc.creditedValue {
		return fmt.Errorf("no successful value transfer credited its recipient (%s)", sc)
	}
	return nil
}

func elBlobsLand(_ *types.EvaluationContext, _ ...*grpc.ClientConn) error {
	sc, err := scanRecentELBlocks()
	if err != nil {
		return err
	}
	if !sc.blobLanded {
		return fmt.Errorf("no blob gas consumed (%s): blob transactions are not landing", sc)
	}
	return nil
}

// Command blobsend drives an enclave's execution layer with real traffic, so a
// measurement run has data column sidecars to carry instead of empty payloads.
//
//	go run ./kurtosis/blobsend -rpc http://127.0.0.1:33221 -interval 6s
//
// Two things are deliberate. Every transaction carries a generous gas limit:
// Amsterdam reprices state access (a bare transfer is ~207k, not 21000), and a
// tool that hardcodes the pre-Amsterdam cost has its funding transfers die
// out-of-gas, which is how earlier runs ended up with no blob transactions and
// therefore no columns at all. And blobs and transfers are sent from two
// different prefunded accounts, because geth pins a sender to a single txpool
// subpool and the two kinds of traffic would evict each other.
//
// Blob sidecars are built with cell proofs (BlobSidecarVersion1), which is what
// a Fulu/PeerDAS txpool accepts.
package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"log"
	"math/big"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/components/eth1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// The ethereum-package prefunded dev accounts (m/44'/60'/0'/0/{0,1}); index 0
// sends blobs, index 1 sends transfers.
const (
	blobKeyHex     = "bcdf20249abf0ed6d944c0288fad489e33f66b3960d9e6229c1cd214ed3bbe31"
	transferKeyHex = "39725efee3fb28614de3bacaffe4cc4bd8c436257e2c8bb887c4b5c4be45e76d"
)

func main() {
	rpcURL := flag.String("rpc", "http://127.0.0.1:8545", "execution layer JSON-RPC endpoint")
	interval := flag.Duration("interval", 6*time.Second, "delay between sends")
	duration := flag.Duration("duration", 0, "stop after this long (0 = run forever)")
	blobs := flag.Int("blobs", 2, "blobs per blob transaction")
	gas := flag.Uint64("gas", 500_000, "gas limit; Amsterdam prices a transfer at ~207k")
	flag.Parse()

	ctx := context.Background()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}

	client, err := ethclient.DialContext(ctx, *rpcURL)
	if err != nil {
		log.Fatalf("dial %s: %v", *rpcURL, err)
	}
	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("chain id: %v", err)
	}
	blobKey, err := crypto.HexToECDSA(blobKeyHex)
	if err != nil {
		log.Fatalf("blob key: %v", err)
	}
	transferKey, err := crypto.HexToECDSA(transferKeyHex)
	if err != nil {
		log.Fatalf("transfer key: %v", err)
	}
	signer := types.LatestSignerForChainID(chainID)
	blobFrom := crypto.PubkeyToAddress(blobKey.PublicKey)
	transferFrom := crypto.PubkeyToAddress(transferKey.PublicKey)
	log.Printf("chain %s: blobs from %s, transfers from %s", chainID, blobFrom, transferFrom)

	// One blob transaction and one transfer per tick, each from its own account.
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	var sent, failed int
	for {
		select {
		case <-ctx.Done():
			log.Printf("done: %d sent, %d failed", sent, failed)
			return
		case <-ticker.C:
		}
		head, err := client.HeaderByNumber(ctx, nil)
		if err != nil {
			log.Printf("head: %v", err)
			failed++
			continue
		}
		tip := big.NewInt(1_000_000_000) // 1 gwei
		feeCap := new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(4)), tip)
		blobFeeCap := big.NewInt(1_000_000_000)
		if head.ExcessBlobGas != nil {
			// Overpay by a wide margin rather than track the exact schedule.
			blobFeeCap = new(big.Int).Mul(blobFeeCap, big.NewInt(100))
		}

		for _, s := range []struct {
			name string
			send func() (common.Hash, error)
		}{
			{"blob", func() (common.Hash, error) {
				nonce, err := client.PendingNonceAt(ctx, blobFrom)
				if err != nil {
					return common.Hash{}, err
				}
				to := common.HexToAddress("0x000000000000000000000000000000000000b10b")
				data := make([]byte, (*blobs-1)*fieldparams.BlobSize+1)
				copy(data, head.Hash().Bytes()) // unique per block, so no duplicate blobs
				tx, err := eth1.New4844CellTx(nonce, &to, *gas, chainID, tip, feeCap,
					big.NewInt(0), nil, blobFeeCap, data, nil)
				if err != nil {
					return common.Hash{}, err
				}
				return sign(ctx, client, signer, blobKey, tx)
			}},
			{"transfer", func() (common.Hash, error) {
				nonce, err := client.PendingNonceAt(ctx, transferFrom)
				if err != nil {
					return common.Hash{}, err
				}
				to := common.HexToAddress("0x0000000000000000000000000000000000c0ffee")
				tx := types.NewTx(&types.DynamicFeeTx{
					ChainID: chainID, Nonce: nonce, To: &to, Gas: *gas,
					GasTipCap: tip, GasFeeCap: feeCap, Value: big.NewInt(1_000_000_000_000_000),
				})
				return sign(ctx, client, signer, transferKey, tx)
			}},
		} {
			hash, err := s.send()
			if err != nil {
				log.Printf("%s: %v", s.name, err)
				failed++
				continue
			}
			sent++
			log.Printf("%s sent %s", s.name, hash)
			go report(client, s.name, hash)
		}
	}
}

func sign(ctx context.Context, client *ethclient.Client, signer types.Signer,
	key *ecdsa.PrivateKey, tx *types.Transaction) (common.Hash, error) {
	signed, err := types.SignTx(tx, signer, key)
	if err != nil {
		return common.Hash{}, err
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, err
	}
	return signed.Hash(), nil
}

// report waits for the receipt and logs what the execution layer did with the
// transaction: a run's evidence that value moved and blob gas was consumed.
func report(client *ethclient.Client, name string, hash common.Hash) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	for {
		receipt, err := client.TransactionReceipt(ctx, hash)
		if err == nil {
			log.Printf("%s %s: status %d block %d gas %d blobgas %d",
				name, hash, receipt.Status, receipt.BlockNumber, receipt.GasUsed,
				receipt.BlobGasUsed)
			return
		}
		select {
		case <-ctx.Done():
			log.Printf("%s %s: no receipt", name, hash)
			return
		case <-time.After(2 * time.Second):
		}
	}
}

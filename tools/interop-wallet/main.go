// Command interop-wallet creates a validator wallet pre-loaded with the
// deterministic interop keys that a premined genesis state is built from.
//
// prysmctl's generate-genesis derives its validator set with
// interop.DeterministicallyGenerateKeys, so validator index N in the genesis
// state and key N here are the same key. Nothing needs to be transferred
// between the two: both sides recompute the keys from the index.
//
// Split a key range across several validator clients with --start-index:
//
//	interop-wallet --num-keys=2 --start-index=0 --wallet-dir=w1 --password=...
//	interop-wallet --num-keys=2 --start-index=2 --wallet-dir=w2 --password=...
//
// These keys are derived from a public algorithm with no secret input, so they
// are only ever suitable for local devnets.
package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"

	"github.com/OffchainLabs/prysm/v7/io/file"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager/local"
)

var (
	numKeys    = flag.Uint64("num-keys", 0, "Number of validator keys to import")
	startIndex = flag.Uint64("start-index", 0, "First validator index to import")
	walletDir  = flag.String("wallet-dir", "", "Directory to create the wallet in")
	password   = flag.String("password", "", "Wallet password")
)

func main() {
	flag.Parse()
	if *numKeys == 0 || *walletDir == "" || *password == "" {
		log.Fatal("--num-keys, --wallet-dir and --password are required")
	}

	ctx := context.Background()
	privKeys, pubKeys, err := interop.DeterministicallyGenerateKeys(*startIndex, *numKeys)
	if err != nil {
		log.Fatalf("could not generate interop keys: %v", err)
	}

	w := wallet.New(&wallet.Config{
		WalletDir:      *walletDir,
		KeymanagerKind: keymanager.Local,
		WalletPassword: *password,
	})
	if err := w.SaveWallet(); err != nil {
		log.Fatalf("could not save wallet: %v", err)
	}

	km, err := local.NewKeymanager(ctx, &local.SetupConfig{Wallet: w})
	if err != nil {
		log.Fatalf("could not create keymanager: %v", err)
	}

	privBytes := make([][]byte, len(privKeys))
	pubBytes := make([][]byte, len(pubKeys))
	for i := range privKeys {
		privBytes[i] = privKeys[i].Marshal()
		pubBytes[i] = pubKeys[i].Marshal()
	}
	if err := km.ImportKeypairs(ctx, privBytes, pubBytes); err != nil {
		log.Fatalf("could not import keypairs: %v", err)
	}

	passwordFile := filepath.Join(*walletDir, wallet.DefaultWalletPasswordFile)
	if err := file.WriteFile(passwordFile, []byte(*password)); err != nil {
		log.Fatalf("could not write password file: %v", err)
	}

	log.Printf("imported %d keys (index %d..%d) into %s",
		*numKeys, *startIndex, *startIndex+*numKeys-1, *walletDir)
	log.Printf("password file: %s", passwordFile)
}

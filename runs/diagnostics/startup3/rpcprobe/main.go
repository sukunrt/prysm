// rpcprobe measures low-rate real Prysm RPC latency during a local startup load.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type event struct {
	Event       string  `json:"event"`
	Method      string  `json:"method,omitempty"`
	Slot        uint64  `json:"slot,omitempty"`
	OffsetMS    float64 `json:"offset_ms,omitempty"`
	DurationMS  float64 `json:"duration_ms,omitempty"`
	OK          bool    `json:"ok,omitempty"`
	GenesisUnix int64   `json:"genesis_unix,omitempty"`
}

func main() {
	genesisPath := flag.String("genesis", "", "path to Heze genesis.ssz")
	configPath := flag.String("config", "", "path to chain config.yaml")
	endpoint := flag.String("endpoint", "127.0.0.1:4000", "local Prysm beacon gRPC endpoint")
	flag.Parse()
	if *genesisPath == "" || *configPath == "" {
		fatalf("-genesis and -config are required")
	}
	if err := params.LoadChainConfigFile(*configPath, nil); err != nil {
		fatalf("load config: %v", err)
	}
	raw, err := os.ReadFile(*genesisPath)
	if err != nil {
		fatalf("read genesis: %v", err)
	}
	pb := &ethpb.BeaconStateHeze{}
	if err := pb.UnmarshalSSZ(raw); err != nil {
		fatalf("decode genesis: %v", err)
	}
	st, err := state_native.InitializeFromProtoUnsafeHeze(pb)
	if err != nil {
		fatalf("initialize genesis: %v", err)
	}
	committee, err := st.CurrentSyncCommittee()
	if err != nil || len(committee.Pubkeys) == 0 {
		fatalf("read current sync committee: %v", err)
	}
	genesis := st.GenesisTime()
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(event{Event: "ready", GenesisUnix: genesis.Unix()}); err != nil {
		fatalf("write readiness: %v", err)
	}
	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer conn.Close()
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	start, _ := slots.StartTime(genesis, 1)
	start = start.Add(300 * time.Millisecond)
	end, _ := slots.StartTime(genesis, 3)
	end = end.Add(12 * time.Second)
	if wait := time.Until(start); wait > 0 {
		time.Sleep(wait)
	}

	var outputMu sync.Mutex
	probe := func(method string) {
		for due := start; !due.After(end); {
			if wait := time.Until(due); wait > 0 {
				time.Sleep(wait)
			}
			if time.Now().After(end) {
				return
			}
			began := time.Now()
			probeSlot := uint64(began.Sub(genesis) / (time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second))
			if probeSlot < 1 {
				probeSlot = 1
			} else if probeSlot > 3 {
				probeSlot = 3
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			var err error
			if method == "DomainData" {
				_, err = client.DomainData(ctx, &ethpb.DomainRequest{Epoch: 0, Domain: []byte{0x02, 0, 0, 0}})
			} else {
				_, err = client.GetSyncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{PublicKey: committee.Pubkeys[0], Slot: primitives.Slot(probeSlot)})
			}
			cancel()
			slotStart, _ := slots.StartTime(genesis, primitives.Slot(probeSlot))
			outputMu.Lock()
			_ = enc.Encode(event{Event: "probe", Method: method, Slot: probeSlot, OffsetMS: float64(began.Sub(slotStart).Microseconds()) / 1000, DurationMS: float64(time.Since(began).Microseconds()) / 1000, OK: err == nil})
			outputMu.Unlock()
			due = nextProbeDue(due, time.Now(), 500*time.Millisecond)
		}
	}
	var wg sync.WaitGroup
	for _, method := range []string{"DomainData", "GetSyncSubcommitteeIndex"} {
		wg.Add(1)
		go func() { defer wg.Done(); probe(method) }()
	}
	wg.Wait()
}

func nextProbeDue(previous, now time.Time, interval time.Duration) time.Time {
	next := previous.Add(interval)
	if next.After(now) {
		return next
	}
	missed := now.Sub(next)/interval + 1
	return next.Add(missed * interval)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "rpcprobe: "+format+"\n", args...)
	os.Exit(1)
}

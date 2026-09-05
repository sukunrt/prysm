// ffgsource prepares valid genesis-root FFG votes and submits them to one Prysm
// beacon node. It is intended only for isolated local consensus test networks.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	bip39 "github.com/tyler-smith/go-bip39"
	eth2util "github.com/wealdtech/go-eth2-util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

const workers = 64

type options struct {
	genesis, config, mnemonic, endpoint, slots, exclude string
	perSlot                                             int
	spread, startOffset                                 time.Duration
}

type prepared struct {
	slot primitives.Slot
	att  *ethpb.SingleAttestation
}

type slotResult struct {
	Slot               uint64 `json:"slot"`
	Planned            int    `json:"planned"`
	Accepted           int    `json:"accepted"`
	Errors             int    `json:"errors"`
	FirstAttemptOffset int64  `json:"first_attempt_offset_ms"`
	LastAttemptOffset  int64  `json:"last_attempt_offset_ms"`
}

type summary struct {
	Prepared      int          `json:"prepared"`
	PreparationMS int64        `json:"preparation_ms"`
	GenesisUnix   uint64       `json:"genesis_unix"`
	Results       []slotResult `json:"results"`
}

type preparationEvent struct {
	Event         string `json:"event"`
	Prepared      int    `json:"prepared"`
	PreparationMS int64  `json:"preparation_ms"`
	GenesisUnix   int64  `json:"genesis_unix"`
}

func main() {
	var o options
	flag.StringVar(&o.genesis, "genesis", "", "path to Heze genesis.ssz")
	flag.StringVar(&o.config, "config", "", "path to chain config.yaml")
	flag.StringVar(&o.mnemonic, "mnemonic-file", "", "path to mnemonic text or generated mnemonics.yaml")
	flag.StringVar(&o.endpoint, "endpoint", "127.0.0.1:4000", "local Prysm beacon gRPC endpoint")
	flag.StringVar(&o.slots, "slots", "1,2,3", "comma-separated slots")
	flag.StringVar(&o.exclude, "exclude-selection", "", "optional proposerkeys selection JSON")
	flag.IntVar(&o.perSlot, "per-slot", 2500, "votes prepared per slot")
	flag.DurationVar(&o.spread, "spread", 2*time.Second, "submission spread from each slot start")
	flag.DurationVar(&o.startOffset, "start-offset", 0, "offset from each slot start for the first submission")
	flag.Parse()
	if o.genesis == "" || o.config == "" || o.mnemonic == "" || o.perSlot <= 0 || o.spread < 0 {
		fatalf("-genesis, -config, -mnemonic-file and positive -per-slot are required")
	}

	started := time.Now()
	st, err := loadState(o.config, o.genesis)
	if err != nil {
		fatalf("load genesis: %v", err)
	}
	if o.startOffset < -400*time.Millisecond || o.startOffset > time.Second {
		fatalf("-start-offset must be between -400ms and 1s")
	}
	if o.startOffset < 0 && -o.startOffset > params.BeaconConfig().MaximumGossipClockDisparityDuration() {
		fatalf("early -start-offset exceeds configured MAXIMUM_GOSSIP_CLOCK_DISPARITY")
	}
	wantedSlots, err := parseSlots(o.slots)
	if err != nil {
		fatalf("parse slots: %v", err)
	}
	mnemonic, err := readMnemonic(o.mnemonic)
	if err != nil {
		fatalf("read mnemonic: %v", err)
	}
	excluded, err := readExcluded(o.exclude)
	if err != nil {
		fatalf("read exclusion: %v", err)
	}
	votes, err := prepareVotes(context.Background(), st, wantedSlots, o.perSlot, mnemonic, excluded)
	if err != nil {
		fatalf("prepare votes: %v", err)
	}
	prepDuration := time.Since(started)
	genesisTime := st.GenesisTime()
	if time.Now().After(genesisTime.Add(-10 * time.Second)) {
		fatalf("preparation finished too late: genesis=%s remaining=%s", genesisTime.Format(time.RFC3339), time.Until(genesisTime))
	}
	mnemonic = ""
	if err := json.NewEncoder(os.Stderr).Encode(preparationEvent{Event: "prepared", Prepared: len(votes), PreparationMS: prepDuration.Milliseconds(), GenesisUnix: genesisTime.Unix()}); err != nil {
		fatalf("write preparation event: %v", err)
	}

	conn, err := grpc.NewClient(o.endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer conn.Close()
	client := ethpb.NewBeaconNodeValidatorClient(conn)
	results := submit(context.Background(), client, votes, genesisTime, o.spread, o.startOffset)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary{Prepared: len(votes), PreparationMS: prepDuration.Milliseconds(), GenesisUnix: uint64(st.GenesisTime().Unix()), Results: results}); err != nil {
		fatalf("write summary: %v", err)
	}
}

func loadState(configPath, genesisPath string) (state.BeaconState, error) {
	if err := params.LoadChainConfigFile(configPath, nil); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(genesisPath)
	if err != nil {
		return nil, err
	}
	pb := &ethpb.BeaconStateHeze{}
	if err := pb.UnmarshalSSZ(raw); err != nil {
		return nil, err
	}
	return state_native.InitializeFromProtoUnsafeHeze(pb)
}

func parseSlots(input string) ([]primitives.Slot, error) {
	var out []primitives.Slot
	seen := make(map[primitives.Slot]bool)
	for _, part := range strings.Split(input, ",") {
		var slot uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &slot); err != nil {
			return nil, fmt.Errorf("invalid slot %q", part)
		}
		if slot == 0 || seen[primitives.Slot(slot)] {
			return nil, fmt.Errorf("slot must be unique and nonzero: %d", slot)
		}
		seen[primitives.Slot(slot)] = true
		out = append(out, primitives.Slot(slot))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func readMnemonic(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var entries []string
	if yaml.Unmarshal(raw, &entries) == nil && len(entries) == 1 {
		return strings.TrimSpace(entries[0]), nil
	}
	var records []struct {
		Mnemonic string `yaml:"mnemonic"`
	}
	if yaml.Unmarshal(raw, &records) == nil && len(records) == 1 && strings.TrimSpace(records[0].Mnemonic) != "" {
		return strings.TrimSpace(records[0].Mnemonic), nil
	}
	value := strings.TrimSpace(string(raw))
	if value == "" || strings.Contains(value, "\n") {
		return "", fmt.Errorf("expected one mnemonic")
	}
	return value, nil
}

func readExcluded(path string) (map[uint64]bool, error) {
	out := make(map[uint64]bool)
	if path == "" {
		return out, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value struct {
		Selected []uint64 `json:"selected_indices"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	for _, index := range value.Selected {
		out[index] = true
	}
	return out, nil
}

func prepareVotes(ctx context.Context, st state.BeaconState, wanted []primitives.Slot, perSlot int, mnemonic string, excluded map[uint64]bool) ([]prepared, error) {
	seed := bip39.NewSeed(mnemonic, "")
	genesisBlock, err := coreblocks.NewGenesisBlockForState(ctx, st)
	if err != nil {
		return nil, err
	}
	genesisRoot, err := genesisBlock.Block().HashTreeRoot()
	if err != nil {
		return nil, err
	}
	domain, err := signing.Domain(st.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	source := st.CurrentJustifiedCheckpoint()
	if source == nil {
		return nil, fmt.Errorf("genesis state has no current justified checkpoint")
	}
	votes := make([]prepared, 0, len(wanted)*perSlot)
	for _, slot := range wanted {
		if slot < 1 || slot > 3 || slots.ToEpoch(slot) != 0 {
			return nil, fmt.Errorf("slot %d is outside the supported startup slots 1..3 in genesis epoch", slot)
		}
		chosen := 0
		committeeCount := helpers.SlotCommitteeCount(uint64(st.NumValidators()))
		committees := make([][]primitives.ValidatorIndex, committeeCount)
		for committeeID := uint64(0); committeeID < committeeCount; committeeID++ {
			committee, err := helpers.BeaconCommitteeFromState(ctx, st, slot, primitives.CommitteeIndex(committeeID))
			if err != nil {
				return nil, err
			}
			committees[committeeID] = committee
		}
		for seat := 0; chosen < perSlot; seat++ {
			found := false
			for committeeID, committee := range committees {
				if seat >= len(committee) {
					continue
				}
				found = true
				validatorIndex := committee[seat]
				if excluded[uint64(validatorIndex)] {
					continue
				}
				key, err := eth2util.PrivateKeyFromSeedAndPath(seed, fmt.Sprintf("m/12381/3600/%d/0/0", validatorIndex))
				if err != nil {
					return nil, err
				}
				expected := st.PubkeyAtIndex(validatorIndex)
				if !strings.EqualFold(fmt.Sprintf("%x", key.PublicKey().Marshal()), fmt.Sprintf("%x", expected[:])) {
					return nil, fmt.Errorf("derived public key mismatch at validator %d", validatorIndex)
				}
				data := &ethpb.AttestationData{
					Slot:            slot,
					BeaconBlockRoot: genesisRoot[:],
					Source:          &ethpb.Checkpoint{Epoch: source.Epoch, Root: append([]byte(nil), source.Root...)},
					Target:          &ethpb.Checkpoint{Root: genesisRoot[:]},
				}
				root, err := signing.ComputeSigningRoot(data, domain)
				if err != nil {
					return nil, err
				}
				votes = append(votes, prepared{slot: slot, att: &ethpb.SingleAttestation{CommitteeId: primitives.CommitteeIndex(committeeID), AttesterIndex: validatorIndex, Data: data, Signature: key.Sign(root[:]).Marshal()}})
				chosen++
				if chosen >= perSlot {
					break
				}
			}
			if !found {
				break
			}
		}
		if chosen != perSlot {
			return nil, fmt.Errorf("slot %d has only %d selectable committee members", slot, chosen)
		}
	}
	return votes, nil
}

func submit(ctx context.Context, client ethpb.BeaconNodeValidatorClient, votes []prepared, genesis time.Time, spread, startOffset time.Duration) []slotResult {
	bySlot := make(map[primitives.Slot][]*ethpb.SingleAttestation)
	for _, vote := range votes {
		bySlot[vote.slot] = append(bySlot[vote.slot], vote.att)
	}
	slotIDs := make([]primitives.Slot, 0, len(bySlot))
	for slot := range bySlot {
		slotIDs = append(slotIDs, slot)
	}
	sort.Slice(slotIDs, func(i, j int) bool { return slotIDs[i] < slotIDs[j] })
	results := make([]slotResult, len(slotIDs))
	var slotsWG sync.WaitGroup
	for resultIndex, slot := range slotIDs {
		resultIndex, slot := resultIndex, slot
		slotsWG.Add(1)
		go func() {
			defer slotsWG.Done()
			atts := bySlot[slot]
			start, err := slots.StartTime(genesis, slot)
			if err != nil {
				results[resultIndex] = slotResult{Slot: uint64(slot), Planned: len(atts), Errors: len(atts)}
				return
			}
			firstDue := start.Add(startOffset)
			if wait := time.Until(firstDue); wait > 0 {
				time.Sleep(wait)
			}
			jobs := make(chan *ethpb.SingleAttestation)
			var wg sync.WaitGroup
			var mu sync.Mutex
			result := slotResult{Slot: uint64(slot), Planned: len(atts)}
			first, last := int64(1<<62), int64(-1<<62)
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for att := range jobs {
						offset := time.Since(start).Milliseconds()
						callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
						_, err := client.ProposeAttestationElectra(callCtx, att)
						cancel()
						mu.Lock()
						if offset < first {
							first = offset
						}
						if offset > last {
							last = offset
						}
						if err == nil {
							result.Accepted++
						} else {
							result.Errors++
						}
						mu.Unlock()
					}
				}()
			}
			for i, att := range atts {
				due := firstDue.Add(time.Duration(i) * spread / time.Duration(len(atts)))
				if wait := time.Until(due); wait > 0 {
					time.Sleep(wait)
				}
				jobs <- att
			}
			close(jobs)
			wg.Wait()
			result.FirstAttemptOffset, result.LastAttemptOffset = first, last
			results[resultIndex] = result
		}()
	}
	slotsWG.Wait()
	return results
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ffgsource: "+format+"\n", args...)
	os.Exit(1)
}

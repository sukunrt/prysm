package evaluators

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AvailableAttestationsFlow checks that the Heze available attestation stream is
// live: every beacon node must have accepted messages on the global
// available_attestation gossip topic. It runs from the first epoch on, so that
// the one-epoch short run exercises it too.
var AvailableAttestationsFlow = types.Evaluator{
	Name:       "available_attestations_flow_%d",
	Policy:     policies.AllEpochs,
	Evaluation: availableAttestationsFlow,
}

// AttestationsInEveryRound checks that committee attestations are produced in
// every round of an epoch, not only in the epoch's first round. With
// SLOTS_PER_ROUND shorter than SLOTS_PER_EPOCH a validator attests once per
// round, so attestation data slots must cover every round offset.
var AttestationsInEveryRound = types.Evaluator{
	Name:       "attestations_in_every_round_%d",
	Policy:     policies.AfterNthEpoch(1),
	Evaluation: attestationsInEveryRound,
}

// ChainProducesBlocks checks that the chain actually advanced past genesis.
// Every other early evaluator passes on a blockless chain, so without this an
// execution layer that never returns a payload looks green.
var ChainProducesBlocks = types.Evaluator{
	Name:       "chain_produces_blocks_%d",
	Policy:     policies.AllEpochs,
	Evaluation: chainProducesBlocks,
}

func chainProducesBlocks(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	// The evaluator ticker fires in the middle of an epoch, so half an epoch
	// of slots has gone by at the earliest call. Ask for half of that again,
	// which leaves room for missed proposals but not for an empty chain.
	want := primitives.Slot(params.BeaconConfig().SlotsPerEpoch / 4)
	for i, conn := range conns {
		client := eth.NewBeaconChainClient(conn)
		head, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
		if err != nil {
			return errors.Wrapf(err, "failed to get chain head of beacon node %d", i)
		}
		if head.HeadSlot < want {
			return fmt.Errorf(
				"beacon node %d head is at slot %d, want at least %d: the chain is not producing blocks",
				i, head.HeadSlot, want,
			)
		}
	}
	return nil
}

func availableAttestationsFlow(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	client := eth.NewBeaconChainClient(conns[0])
	head, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	digest := params.ForkDigest(head.HeadEpoch)
	topic := fmt.Sprintf(p2p.AvailableAttestationTopicFormat, digest)
	metric := fmt.Sprintf("p2p_message_received_total{topic=%q}", topic+"/ssz_snappy")
	for i := range conns {
		page, err := beaconMetricsPage(i)
		if err != nil {
			return err
		}
		count, ok := labelledMetricValue(page, metric)
		if !ok {
			return fmt.Errorf(
				"beacon node %d exposes no %s counter, so no available attestation reached it",
				i, metric,
			)
		}
		if count <= 0 {
			return fmt.Errorf(
				"beacon node %d received %v available attestations on %s, want more than 0",
				i, count, topic,
			)
		}
	}
	return nil
}

func attestationsInEveryRound(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	client := eth.NewBeaconChainClient(conns[0])
	head, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	cfg := params.BeaconConfig()
	roundsPerEpoch := uint64(cfg.SlotsPerEpoch / cfg.SlotsPerRound)
	epoch := head.HeadEpoch - 1
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return errors.Wrapf(err, "could not compute the first slot of epoch %d", epoch)
	}

	// Attested slots grouped by their offset round within their own epoch. The
	// blocks of one epoch carry attestations for a window of SLOTS_PER_EPOCH
	// slots, so a healthy run covers every round offset here.
	perRound := make(map[uint64]map[primitives.Slot]bool, roundsPerEpoch)
	for slot := start; slot < start+cfg.SlotsPerEpoch; slot++ {
		atts, err := blockAttestations(slot)
		if err != nil {
			return err
		}
		for _, att := range atts {
			attSlot, err := strconv.ParseUint(att.Data.Slot, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "could not parse the attestation slot %q", att.Data.Slot)
			}
			round := (attSlot % uint64(cfg.SlotsPerEpoch)) / uint64(cfg.SlotsPerRound)
			if perRound[round] == nil {
				perRound[round] = make(map[primitives.Slot]bool)
			}
			perRound[round][primitives.Slot(attSlot)] = true
		}
	}

	counts := make([]string, 0, roundsPerEpoch)
	for round := range roundsPerEpoch {
		counts = append(counts, fmt.Sprintf("round %d: %d slots", round, len(perRound[round])))
	}
	for round := range roundsPerEpoch {
		if len(perRound[round]) == 0 {
			return fmt.Errorf(
				"blocks of epoch %d carry no attestation for round %d of an epoch (%s)",
				epoch, round, strings.Join(counts, ", "),
			)
		}
	}
	return nil
}

// blockAttestations returns the attestations carried by the canonical block at
// the given slot, or nothing at all if that slot has no block.
func blockAttestations(slot primitives.Slot) ([]*structs.AttestationElectra, error) {
	url := fmt.Sprintf(
		"http://127.0.0.1:%d/eth/v2/beacon/blocks/%d/attestations",
		e2e.TestParams.Ports.PrysmBeaconNodeHTTPPort, slot,
	)
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "could not request the attestations of slot %d", slot)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // A missed slot has no block.
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "could not read the attestations of slot %d", slot)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attestations of slot %d: status %d: %s", slot, resp.StatusCode, body)
	}
	container := &structs.GetBlockAttestationsV2Response{}
	if err := json.Unmarshal(body, container); err != nil {
		return nil, errors.Wrapf(err, "could not decode the attestations of slot %d", slot)
	}
	var atts []*structs.AttestationElectra
	if err := json.Unmarshal(container.Data, &atts); err != nil {
		return nil, errors.Wrapf(err, "could not decode the attestation list of slot %d", slot)
	}
	return atts, nil
}

// beaconMetricsPage fetches the prometheus page of the given beacon node.
func beaconMetricsPage(index int) (string, error) {
	port := e2e.TestParams.Ports.PrysmBeaconNodeMetricsPort + index
	url := fmt.Sprintf("http://localhost:%d/metrics", port)
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "could not read the metrics of beacon node %d", index)
	}
	defer func() { _ = resp.Body.Close() }()
	page, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "could not read the metrics body of beacon node %d", index)
	}
	return string(page), nil
}

// labelledMetricValue reads the value of one fully labelled prometheus sample.
// valueOfTopic cannot be used here: it relies on the metric name repeating in
// the HELP and TYPE comments, and those carry no labels.
func labelledMetricValue(page, sample string) (float64, bool) {
	for line := range strings.Lines(page) {
		name, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found || name != sample {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

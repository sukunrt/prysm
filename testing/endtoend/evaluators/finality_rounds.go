package evaluators

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
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

// FinalizationOccursInRounds is the rounds twin of FinalizationOccurs: with FFG
// keyed to rounds, steady state is the finalized checkpoint two rounds behind
// the head's round and the justified checkpoint one round behind, whatever the
// epoch length is. Run it only after the round-2 genesis guard has cleared, so
// that every round in the window is one the gadget was allowed to justify. The
// guard ends at round 2, so at 8/32 any epoch from 1 on qualifies; the call
// sites stay at a later epoch because the other evaluators need the room.
var FinalizationOccursInRounds = func(epoch primitives.Epoch) types.Evaluator {
	return types.Evaluator{
		Name:       "finalizes_in_rounds_at_epoch_%d",
		Policy:     policies.AfterNthEpoch(epoch),
		Evaluation: finalizationOccursInRounds,
	}
}

// JustificationAdvancesEveryRound checks the per-round justification rate the
// whole plan exists for: over one finished epoch, the justified checkpoint's
// round must go up by exactly one at every round boundary. The endpoint check
// in FinalizationOccursInRounds still passes when a round is skipped and the
// next one catches up, so a gap is only visible from the boundaries themselves.
var JustificationAdvancesEveryRound = types.Evaluator{
	Name:       "justification_advances_every_round_%d",
	Policy:     policies.AfterNthEpoch(3),
	Evaluation: justificationAdvancesEveryRound,
}

func finalizationOccursInRounds(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	client := eth.NewBeaconChainClient(conns[0])
	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	// The chain head's checkpoint fields carry ROUNDS; the wall-clock side is the
	// round the head slot falls in.
	currentRound := slots.RoundAt(chainHead.HeadSlot)
	if currentRound < 2 {
		return fmt.Errorf("head slot %d is only in round %d, too early to judge finality",
			chainHead.HeadSlot, currentRound)
	}

	if want := currentRound - 2; chainHead.FinalizedEpoch != want {
		return fmt.Errorf("expected finalized round %d at head slot %d, received %d",
			want, chainHead.HeadSlot, chainHead.FinalizedEpoch)
	}
	if want := currentRound - 1; chainHead.JustifiedEpoch != want {
		return fmt.Errorf("expected justified round %d at head slot %d, received %d",
			want, chainHead.HeadSlot, chainHead.JustifiedEpoch)
	}
	if chainHead.PreviousJustifiedEpoch+1 != chainHead.JustifiedEpoch {
		return fmt.Errorf(
			"there should be no gap between the justified rounds, received current %d and previous %d",
			chainHead.JustifiedEpoch, chainHead.PreviousJustifiedEpoch,
		)
	}
	return nil
}

func justificationAdvancesEveryRound(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	client := eth.NewBeaconChainClient(conns[0])
	head, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	cfg := params.BeaconConfig()
	epoch := head.HeadEpoch - 1
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return errors.Wrapf(err, "could not compute the first slot of epoch %d", epoch)
	}

	// One sample per round boundary of the epoch, plus the boundary that closes
	// it, so all of the epoch's rounds are covered by a pair. The state at the
	// first slot of round R has just justified round R-1, so consecutive samples
	// must differ by exactly one.
	roundsPerEpoch := uint64(cfg.SlotsPerEpoch / cfg.SlotsPerRound)
	justified := make([]primitives.Round, 0, roundsPerEpoch+1)
	boundaries := make([]primitives.Slot, 0, roundsPerEpoch+1)
	for k := range roundsPerEpoch + 1 {
		slot := start + primitives.Slot(k).Mul(uint64(cfg.SlotsPerRound))
		round, err := justifiedRoundAtSlot(slot)
		if err != nil {
			return err
		}
		boundaries = append(boundaries, slot)
		justified = append(justified, round)
	}

	for k := 1; k < len(justified); k++ {
		if justified[k] == justified[k-1]+1 {
			continue
		}
		return fmt.Errorf(
			"justified round did not advance by one over the round boundary at slot %d: "+
				"round %d at slot %d, round %d at slot %d (epoch %d boundaries %v, rounds %v)",
			boundaries[k], justified[k-1], boundaries[k-1], justified[k], boundaries[k],
			epoch, boundaries, justified,
		)
	}
	return nil
}

// justifiedRoundAtSlot reads the current justified checkpoint of the state at
// the given slot. The checkpoint's `epoch` field carries a ROUND.
func justifiedRoundAtSlot(slot primitives.Slot) (primitives.Round, error) {
	url := fmt.Sprintf(
		"http://127.0.0.1:%d/eth/v1/beacon/states/%d/finality_checkpoints",
		e2e.TestParams.Ports.PrysmBeaconNodeHTTPPort, slot,
	)
	resp, err := http.Get(url)
	if err != nil {
		return 0, errors.Wrapf(err, "could not request the finality checkpoints of slot %d", slot)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, errors.Wrapf(err, "could not read the finality checkpoints of slot %d", slot)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("finality checkpoints of slot %d: status %d: %s", slot, resp.StatusCode, body)
	}
	container := &structs.GetFinalityCheckpointsResponse{}
	if err := json.Unmarshal(body, container); err != nil {
		return 0, errors.Wrapf(err, "could not decode the finality checkpoints of slot %d", slot)
	}
	if container.Data == nil || container.Data.CurrentJustified == nil {
		return 0, fmt.Errorf("finality checkpoints of slot %d carry no current justified checkpoint", slot)
	}
	round, err := strconv.ParseUint(container.Data.CurrentJustified.Epoch, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "could not parse the justified round %q at slot %d",
			container.Data.CurrentJustified.Epoch, slot)
	}
	return primitives.Round(round), nil
}

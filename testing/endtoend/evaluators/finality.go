package evaluators

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// FinalizationOccurs is an evaluator to make sure finalization is performing as it should.
// Requires to be run after at least 4 epochs have passed.
var FinalizationOccurs = func(epoch primitives.Epoch) types.Evaluator {
	return types.Evaluator{
		Name:       "finalizes_at_epoch_%d",
		Policy:     policies.AfterNthEpoch(epoch),
		Evaluation: finalizationOccurs,
	}
}

func finalizationOccurs(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	conn := conns[0]
	client := eth.NewBeaconChainClient(conn)
	chainHead, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	// The chain head's checkpoint fields carry ROUNDS, so the wall-clock side is the
	// head slot's round. Under the shipped configs (SLOTS_PER_ROUND == SLOTS_PER_EPOCH)
	// this is numerically what the evaluator always asserted.
	currentRound := slots.RoundAt(chainHead.HeadSlot)
	finalizedRound := chainHead.FinalizedEpoch

	expectedFinalizedRound := currentRound - 2
	if expectedFinalizedRound != finalizedRound {
		return fmt.Errorf(
			"expected finalized round to be %d, received: %d",
			expectedFinalizedRound,
			finalizedRound,
		)
	}
	previousJustifiedRound := chainHead.PreviousJustifiedEpoch
	currentJustifiedRound := chainHead.JustifiedEpoch
	if previousJustifiedRound+1 != currentJustifiedRound {
		return fmt.Errorf(
			"there should be no gaps between current and previous justified rounds, received current %d and previous %d",
			currentJustifiedRound,
			previousJustifiedRound,
		)
	}
	if currentJustifiedRound+1 != currentRound {
		return fmt.Errorf(
			"there should be no gaps between current round and current justified round, received current %d and justified %d",
			currentRound,
			currentJustifiedRound,
		)
	}
	return nil
}

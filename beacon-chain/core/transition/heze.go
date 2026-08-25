package transition

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
)

// processRoundHeze describes the per round operations performed on a Heze beacon
// state. Heze splits what other forks do once per epoch in two: justification and
// finalization run here, once per ROUND, and everything else stays in
// processEpochHeze at epoch cadence (plan-finality-round step 2.2).
//
// The participation arrays must rotate once per round, or round R would be
// justified on round R-1's target bits. The rotation is skipped when this round
// boundary is also an epoch boundary, because processEpochHeze runs it there at its
// original late position — after rewards and penalties have read the pre-rotation
// arrays. Under the shipped configs (SlotsPerRound == SlotsPerEpoch) every round
// boundary is an epoch boundary, so the rotation never happens here and the whole
// sequence is value-identical to processEpochGloas.
func processRoundHeze(ctx context.Context, st state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "heze.ProcessRound")
	defer span.End()

	if st == nil || st.IsNil() {
		return errors.New("nil state")
	}
	vp, bp, err := electra.InitializePrecomputeValidators(ctx, st)
	if err != nil {
		return err
	}
	if _, bp, err = electra.ProcessEpochParticipation(ctx, st, bp, vp); err != nil {
		return err
	}
	if _, err = precompute.ProcessJustificationAndFinalizationPreCompute(st, bp); err != nil {
		return errors.Wrap(err, "could not process justification")
	}
	if coreTime.CanProcessEpoch(st) {
		return nil
	}
	_, err = electra.ProcessParticipationFlagUpdates(st)
	return err
}

// processEpochHeze describes the per epoch operations performed on a Heze beacon
// state. It is processEpochGloas minus justification and finalization, which
// processRoundHeze owns; every remaining call keeps its Gloas order.
//
// The precompute is re-run here rather than shared with processRoundHeze because the
// two are separate ProcessSlotsCore hooks. That is value-safe — both precompute
// passes are pure reads of the validator registry, the inactivity scores and the
// participation arrays, none of which justification and finalization mutate (it
// writes only the checkpoints and the justification bits) — at the cost of a second
// full-registry scan at every epoch boundary, plus one scan per round in between.
// Acceptable at simulation scale.
func processEpochHeze(ctx context.Context, st state.BeaconState) error {
	_, span := trace.StartSpan(ctx, "heze.ProcessEpoch")
	defer span.End()

	if st == nil || st.IsNil() {
		return errors.New("nil state")
	}
	vp, bp, err := electra.InitializePrecomputeValidators(ctx, st)
	if err != nil {
		return err
	}
	vp, bp, err = electra.ProcessEpochParticipation(ctx, st, bp, vp)
	if err != nil {
		return err
	}
	st, vp, err = electra.ProcessInactivityScores(ctx, st, vp)
	if err != nil {
		return errors.Wrap(err, "could not process inactivity updates")
	}
	st, err = electra.ProcessRewardsAndPenaltiesPrecompute(st, bp, vp)
	if err != nil {
		return errors.Wrap(err, "could not process rewards and penalties")
	}
	if err := electra.ProcessRegistryUpdates(ctx, st); err != nil {
		return errors.Wrap(err, "could not process registry updates")
	}
	if err := electra.ProcessSlashings(ctx, st); err != nil {
		return err
	}
	st, err = electra.ProcessEth1DataReset(st)
	if err != nil {
		return err
	}
	active := primitives.Gwei(bp.ActiveCurrentEpoch)
	if err = electra.ProcessPendingDeposits(ctx, st, active); err != nil {
		return err
	}
	if err = electra.ProcessPendingConsolidations(ctx, st); err != nil {
		return err
	}
	if err = gloas.ProcessBuilderPendingPayments(ctx, st); err != nil {
		return err
	}
	if err = electra.ProcessEffectiveBalanceUpdates(st); err != nil {
		return err
	}
	st, err = electra.ProcessSlashingsReset(st)
	if err != nil {
		return err
	}
	st, err = electra.ProcessRandaoMixesReset(st)
	if err != nil {
		return err
	}
	st, err = electra.ProcessHistoricalDataUpdate(st)
	if err != nil {
		return err
	}
	st, err = electra.ProcessParticipationFlagUpdates(st)
	if err != nil {
		return err
	}
	_, err = electra.ProcessSyncCommitteeUpdates(ctx, st)
	if err != nil {
		return err
	}
	if err := gloas.ProcessProposerLookahead(ctx, st); err != nil {
		return err
	}
	return gloas.ProcessPTCWindow(ctx, st)
}

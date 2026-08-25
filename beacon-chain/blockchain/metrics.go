package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	beaconSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_slot",
		Help: "Latest slot of the beacon chain state",
	})
	beaconHeadSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_head_slot",
		Help: "Slot of the head block of the beacon chain",
	})
	clockTimeSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_clock_time_slot",
		Help: "The current slot based on the genesis time and current clock",
	})

	// The FFG checkpoints carry ROUNDS. The four *_epoch gauges keep their names
	// and emit the EPOCH the checkpoint's round starts in, so existing dashboards
	// and scrape tooling stay meaningful; the four *_round gauges next to them
	// carry the raw round, which is what actually advances per round.
	headFinalizedEpoch = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "head_finalized_epoch",
		Help: "Epoch containing the first slot of the head state's last finalized round",
	})
	headFinalizedRound = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "head_finalized_round",
		Help: "Last finalized round of the head state",
	})
	headFinalizedRoot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "head_finalized_root",
		Help: "Last finalized root of the head state",
	})
	beaconFinalizedEpoch = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_finalized_epoch",
		Help: "Epoch containing the first slot of the processed state's last finalized round",
	})
	beaconFinalizedRound = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_finalized_round",
		Help: "Last finalized round of the processed state",
	})
	beaconFinalizedRoot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_finalized_root",
		Help: "Last finalized root of the processed state",
	})
	beaconCurrentJustifiedEpoch = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_current_justified_epoch",
		Help: "Epoch containing the first slot of the processed state's current justified round",
	})
	beaconCurrentJustifiedRound = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_current_justified_round",
		Help: "Current justified round of the processed state",
	})
	beaconCurrentJustifiedRoot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_current_justified_root",
		Help: "Current justified root of the processed state",
	})
	beaconPrevJustifiedEpoch = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_previous_justified_epoch",
		Help: "Epoch containing the first slot of the processed state's previous justified round",
	})
	beaconPrevJustifiedRound = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_previous_justified_round",
		Help: "Previous justified round of the processed state",
	})
	beaconPrevJustifiedRoot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_previous_justified_root",
		Help: "Previous justified root of the processed state",
	})
	finalityLatencySlots = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "finality_latency_slots",
		Help: "Slots between the current slot and the first slot of the last finalized round",
	})
	activeValidatorCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_current_active_validators",
		Help: "Current total active validators",
	})
	validatorsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "validator_count",
		Help: "The total number of validators",
	}, []string{"state"})
	validatorsBalance = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "validators_total_balance",
		Help: "The total balance of validators, in GWei",
	}, []string{"state"})
	validatorsEffectiveBalance = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "validators_total_effective_balance",
		Help: "The total effective balance of validators, in GWei",
	}, []string{"state"})
	currentEth1DataDepositCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "current_eth1_data_deposit_count",
		Help: "The current eth1 deposit count in the last processed state eth1data field.",
	})
	processedDepositsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_processed_deposits_total",
		Help: "Total number of deposits processed",
	})
	stateTrieReferences = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_references",
		Help: "The number of states a particular field is shared with.",
	}, []string{"state"})
	// The participation arrays rotate once per ROUND, so these four measure the
	// previous round's votes, not the previous epoch's. They are renamed rather
	// than kept for continuity: the number changed meaning, so the name must too.
	prevRoundActiveBalances = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_prev_round_active_gwei",
		Help: "The total amount of ether, in gwei, that was active for voting of previous round",
	})
	prevRoundSourceBalances = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_prev_round_source_gwei",
		Help: "The total amount of ether, in gwei, used in voting attestation source of previous round",
	})
	prevRoundTargetBalances = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_prev_round_target_gwei",
		Help: "The total amount of ether, in gwei, used in voting attestation target of previous round",
	})
	prevRoundHeadBalances = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_prev_round_head_gwei",
		Help: "The total amount of ether, in gwei, used in voting attestation head of previous round",
	})
	reorgCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_reorgs_total",
		Help: "Count the number of times beacon chain has a reorg",
	})
	LateBlockAttemptedReorgCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_late_block_attempted_reorgs",
		Help: "Count the number of times a proposer served by this beacon has attempted a late block reorg",
	})
	lateBlockFailedAttemptFirstThreshold = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_failed_reorg_attempts_first_threshold",
		Help: "Count the number of times a proposer served by this beacon attempted a late block reorg but desisted in the first threshold",
	})
	lateBlockFailedAttemptSecondThreshold = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_failed_reorg_attempts_second_threshold",
		Help: "Count the number of times a proposer served by this beacon attempted a late block reorg but desisted in the second threshold",
	})
	saveOrphanedAttCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "saved_orphaned_att_total",
		Help: "Count the number of times an orphaned attestation is saved",
	})
	attestationInclusionDelay = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "attestation_inclusion_delay_slots",
			Help:    "The number of slots between att.Slot and block.Slot",
			Buckets: []float64{1, 2, 3, 4, 6, 32, 64},
		},
	)
	syncHeadStateMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sync_head_state_miss",
		Help: "The number of sync head state requests that are not present in the cache.",
	})
	syncHeadStateHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "sync_head_state_hit",
		Help: "The number of sync head state requests that are present in the cache.",
	})
	newPayloadValidNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_payload_valid_node_count",
		Help: "Count the number of valid nodes after newPayload EE call",
	})
	newPayloadOptimisticNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_payload_optimistic_node_count",
		Help: "Count the number of optimistic nodes after newPayload EE call",
	})
	newPayloadInvalidNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_payload_invalid_node_count",
		Help: "Count the number of invalid nodes after newPayload EE call",
	})
	forkchoiceUpdatedValidNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "forkchoice_updated_valid_node_count",
		Help: "Count the number of valid nodes after forkchoiceUpdated EE call",
	})
	forkchoiceUpdatedOptimisticNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "forkchoice_updated_optimistic_node_count",
		Help: "Count the number of optimistic nodes after forkchoiceUpdated EE call",
	})
	forkchoiceUpdatedInvalidNodeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "forkchoice_updated_invalid_node_count",
		Help: "Count the number of invalid nodes after forkchoiceUpdated EE call",
	})
	txsPerSlotCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "txs_per_slot_count",
		Help: "Count the number of txs per slot",
	})
	consolidationRequestCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "consolidation_request_count",
		Help: "Count the number of consolidation requests",
	})
	onBlockProcessingTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "on_block_processing_milliseconds",
		Help: "Total time in milliseconds to complete a call to postBlockProcess()",
	})
	stateTransitionProcessingTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "state_transition_processing_milliseconds",
		Help: "Total time to call a state transition in validateStateTransition()",
	})
	chainServiceProcessingTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "chain_service_processing_milliseconds",
		Help: "Total time to call a chain service in ReceiveBlock()",
	})
	dataAvailWaitedTime = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "da_waited_time_milliseconds",
		Help: "Total time spent waiting for a data availability check in ReceiveBlock()",
	})
	processAttsElapsedTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "process_attestations_milliseconds",
			Help:    "Captures latency for process attestations (forkchoice) in milliseconds",
			Buckets: []float64{1, 5, 20, 100, 500, 1000},
		},
	)
	newAttHeadElapsedTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "new_att_head_milliseconds",
			Help:    "Captures latency for new attestation head in milliseconds",
			Buckets: []float64{1, 5, 20, 100, 500, 1000},
		},
	)
	newBlockHeadElapsedTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "new_block_head_milliseconds",
			Help:    "Captures latency for new block head in milliseconds",
			Buckets: []float64{1, 5, 20, 100, 500, 1000},
		},
	)
	reorgDistance = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reorg_distance",
			Help:    "Captures distance of reorgs. Distance is defined as the number of blocks between the old head and the new head",
			Buckets: []float64{1, 2, 4, 8, 16, 32, 64},
		},
	)
	reorgDepth = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "reorg_depth",
			Help:    "Captures depth of reorgs. Depth is defined as the number of blocks between the head and the common ancestor",
			Buckets: []float64{1, 2, 4, 8, 16, 32},
		},
	)
	commitmentCount = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "commitment_count_max_21",
			Help:    "The number of blob KZG commitments per block.",
			Buckets: []float64{1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21},
		},
	)
	maxBlobsPerBlock = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "max_blobs_per_block",
			Help: "The maximum number of blobs allowed in a block.",
		},
	)
	beaconExecutionPayloadEnvelopeValidTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_execution_payload_envelope_valid_total",
		Help: "Count the number of execution payload envelopes that were processed successfully.",
	})
	beaconExecutionPayloadEnvelopeInvalidTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_execution_payload_envelope_invalid_total",
		Help: "Count the number of execution payload envelopes that failed processing.",
	})
	beaconExecutionPayloadEnvelopeProcessingDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_execution_payload_envelope_processing_duration_seconds",
			Help:    "Captures end-to-end processing time for execution payload envelopes.",
			Buckets: prometheus.DefBuckets,
		},
	)
	beaconLatePayloadTaskTriggeredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_late_payload_task_triggered_total",
		Help: "Count the number of times late payload tasks fired.",
	})
	goroutineCountGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "beacon_goroutine_count",
		Help: "Goroutine count sampled once per slot.",
	}, []string{"kind"})
)

// reportCheckpointRound sets the pair of gauges describing one round-valued FFG
// checkpoint: the raw round, and the epoch its first slot falls in.
func reportCheckpointRound(round primitives.Round, epochGauge, roundGauge prometheus.Gauge) {
	roundGauge.Set(float64(round))
	epoch, err := helpers.CheckpointEpoch(round)
	if err != nil {
		log.WithError(err).Error("Could not compute the checkpoint round's epoch")
		return
	}
	epochGauge.Set(float64(epoch))
}

// reportSlotMetrics reports slot related metrics.
func reportSlotMetrics(stateSlot, headSlot, clockSlot primitives.Slot, finalizedCheckpoint *ethpb.Checkpoint) {
	clockTimeSlot.Set(float64(clockSlot))
	beaconSlot.Set(float64(stateSlot))
	beaconHeadSlot.Set(float64(headSlot))
	if finalizedCheckpoint == nil {
		return
	}
	reportCheckpointRound(finalizedCheckpoint.Epoch, headFinalizedEpoch, headFinalizedRound)
	headFinalizedRoot.Set(float64(bytesutil.ToLowInt64(finalizedCheckpoint.Root)))
	// Finality latency is measured against the wall clock, not against the head:
	// a chain that stops finalizing must make this gauge grow, and at the moment
	// finality does advance the two readings agree.
	start, err := slots.RoundStart(finalizedCheckpoint.Epoch)
	if err != nil {
		log.WithError(err).Error("Could not compute the finalized round's start slot")
		return
	}
	if clockSlot >= start {
		finalityLatencySlots.Set(float64(clockSlot - start))
	}
}

// reportRoundMetrics reports the whole-state metrics, once per round. It runs at
// round cadence because the FFG checkpoints and the participation arrays it reads
// now move once per round; the validator census it also reports is an epoch-shaped
// quantity that simply gets sampled more often.
func reportRoundMetrics(ctx context.Context, postState, headState state.BeaconState) error {
	currentEpoch := slots.ToEpoch(postState.Slot())

	// Validator instances
	pendingInstances := 0
	activeInstances := 0
	slashingInstances := 0
	slashedInstances := 0
	exitingInstances := 0
	exitedInstances := 0
	// Validator balances
	pendingBalance := uint64(0)
	activeBalance := uint64(0)
	activeEffectiveBalance := uint64(0)
	exitingBalance := uint64(0)
	exitingEffectiveBalance := uint64(0)
	slashingBalance := uint64(0)
	slashingEffectiveBalance := uint64(0)

	for i := 0; i < postState.NumValidators(); i++ {
		validator, err := postState.ValidatorAtIndexReadOnly(primitives.ValidatorIndex(i))
		if err != nil {
			log.WithError(err).Error("Could not load validator")
			continue
		}
		bal, err := postState.BalanceAtIndex(primitives.ValidatorIndex(i))
		if err != nil {
			log.WithError(err).Error("Could not load validator balance")
			continue
		}
		if validator.Slashed() {
			if currentEpoch < validator.ExitEpoch() {
				slashingInstances++
				slashingBalance += bal
				slashingEffectiveBalance += validator.EffectiveBalance()
			} else {
				slashedInstances++
			}
			continue
		}
		if validator.ExitEpoch() != params.BeaconConfig().FarFutureEpoch {
			if currentEpoch < validator.ExitEpoch() {
				exitingInstances++
				exitingBalance += bal
				exitingEffectiveBalance += validator.EffectiveBalance()
			} else {
				exitedInstances++
			}
			continue
		}
		if currentEpoch < validator.ActivationEpoch() {
			pendingInstances++
			pendingBalance += bal
			continue
		}
		activeInstances++
		activeBalance += bal
		activeEffectiveBalance += validator.EffectiveBalance()
	}
	activeInstances += exitingInstances + slashingInstances
	activeBalance += exitingBalance + slashingBalance
	activeEffectiveBalance += exitingEffectiveBalance + slashingEffectiveBalance

	activeValidatorCount.Set(float64(activeInstances))
	validatorsCount.WithLabelValues("Pending").Set(float64(pendingInstances))
	validatorsCount.WithLabelValues("Active").Set(float64(activeInstances))
	validatorsCount.WithLabelValues("Exiting").Set(float64(exitingInstances))
	validatorsCount.WithLabelValues("Exited").Set(float64(exitedInstances))
	validatorsCount.WithLabelValues("Slashing").Set(float64(slashingInstances))
	validatorsCount.WithLabelValues("Slashed").Set(float64(slashedInstances))
	validatorsBalance.WithLabelValues("Pending").Set(float64(pendingBalance))
	validatorsBalance.WithLabelValues("Active").Set(float64(activeBalance))
	validatorsBalance.WithLabelValues("Exiting").Set(float64(exitingBalance))
	validatorsBalance.WithLabelValues("Slashing").Set(float64(slashingBalance))
	validatorsEffectiveBalance.WithLabelValues("Active").Set(float64(activeEffectiveBalance))
	validatorsEffectiveBalance.WithLabelValues("Exiting").Set(float64(exitingEffectiveBalance))
	validatorsEffectiveBalance.WithLabelValues("Slashing").Set(float64(slashingEffectiveBalance))

	// Last justified round
	reportCheckpointRound(postState.CurrentJustifiedCheckpoint().Epoch,
		beaconCurrentJustifiedEpoch, beaconCurrentJustifiedRound)
	beaconCurrentJustifiedRoot.Set(float64(bytesutil.ToLowInt64(postState.CurrentJustifiedCheckpoint().Root)))

	// Last previous justified round
	reportCheckpointRound(postState.PreviousJustifiedCheckpoint().Epoch,
		beaconPrevJustifiedEpoch, beaconPrevJustifiedRound)
	beaconPrevJustifiedRoot.Set(float64(bytesutil.ToLowInt64(postState.PreviousJustifiedCheckpoint().Root)))

	// Last finalized round
	reportCheckpointRound(postState.FinalizedCheckpointRound(),
		beaconFinalizedEpoch, beaconFinalizedRound)
	beaconFinalizedRoot.Set(float64(bytesutil.ToLowInt64(postState.FinalizedCheckpoint().Root)))
	currentEth1DataDepositCount.Set(float64(postState.Eth1Data().DepositCount))
	processedDepositsCount.Set(float64(postState.Eth1DepositIndex() + 1))

	var b *precompute.Balance
	var v []*precompute.Validator
	var err error

	if headState.Version() == version.Phase0 {
		v, b, err = precompute.New(ctx, headState)
		if err != nil {
			return err
		}
		_, b, err = precompute.ProcessAttestations(ctx, headState, v, b)
		if err != nil {
			return err
		}
	} else if headState.Version() >= version.Altair {
		v, b, err = altair.InitializePrecomputeValidators(ctx, headState)
		if err != nil {
			return err
		}
		_, b, err = altair.ProcessEpochParticipation(ctx, headState, b, v)
		if err != nil {
			return err
		}
	} else {
		return errors.Errorf("invalid state type provided: %T", headState.ToProtoUnsafe())
	}

	prevRoundActiveBalances.Set(float64(b.ActivePrevEpoch))
	prevRoundSourceBalances.Set(float64(b.PrevEpochAttested))
	prevRoundTargetBalances.Set(float64(b.PrevEpochTargetAttested))
	prevRoundHeadBalances.Set(float64(b.PrevEpochHeadAttested))

	refMap := postState.FieldReferencesCount()
	for name, val := range refMap {
		stateTrieReferences.WithLabelValues(name).Set(float64(val))
	}
	postState.RecordStateMetrics()

	return nil
}

func reportAttestationInclusion(blk interfaces.ReadOnlyBeaconBlock) {
	for _, att := range blk.Body().Attestations() {
		attestationInclusionDelay.Observe(float64(blk.Slot() - att.GetData().Slot))
	}
}

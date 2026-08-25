package doublylinkedtree

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	headSlotNumber = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "doublylinkedtree_head_slot",
			Help: "The slot number of the current head.",
		},
	)
	nodeCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "doublylinkedtree_node_count",
			Help: "The number of nodes in the doubly linked tree based store structure.",
		},
	)
	headChangesCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_head_changed_count",
			Help: "The number of times head changes.",
		},
	)
	calledHeadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_head_requested_count",
			Help: "The number of times someone called head.",
		},
	)
	processedBlockCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_block_processed_count",
			Help: "The number of times a block is processed for fork choice.",
		},
	)
	processedAttestationCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_attestation_processed_count",
			Help: "The number of times an attestation is processed for fork choice.",
		},
	)
	prunedCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_pruned_count",
			Help: "The number of times pruning happened.",
		},
	)
	payloadInsertedCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forkchoice_payload_inserted_count",
			Help: "The number of payloads inserted into forkchoice.",
		},
	)
	payloadEmptyNodeCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forkchoice_payload_empty_node_count",
			Help: "The number of empty payload nodes currently tracked in forkchoice.",
		},
	)
	payloadFullNodeCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "forkchoice_payload_full_node_count",
			Help: "The number of full payload nodes currently tracked in forkchoice.",
		},
	)
	ptcVoteCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "forkchoice_ptc_vote_count",
			Help: "The number of PTC votes recorded by forkchoice.",
		},
	)
	justifiedRoundAdvanceTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "justified_round_advance_total",
			Help: "Count of the times the store's justified checkpoint moved to a later round.",
		},
	)
	finalizedRoundAdvanceTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "finalized_round_advance_total",
			Help: "Count of the times the store's finalized checkpoint moved to a later round.",
		},
	)
	goldfishSeatFraction = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "goldfish_seat_fraction",
			Help: "Fraction of the available committee seats whose votes for a slot arrived before the next slot started.",
		},
	)
	goldfishGateStopCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_gate_stop_total",
			Help: "The number of times the Goldfish walk stopped because no child cleared the majority gate.",
		},
	)
	goldfishLateVoteCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_late_vote_total",
			Help: "The number of available attestation votes that arrived after their slot had ended.",
		},
	)
	goldfishGateRetreatCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_gate_retreat",
			Help: "The number of times the Goldfish walk moved the head back to an ancestor of the previous head.",
		},
	)
	goldfishRoundProposalCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_round_proposal_total",
			Help: "The number of times the round's distinguished proposal started the Goldfish walk.",
		},
	)
	goldfishProposalConflictCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_round_proposal_conflict_total",
			Help: "The number of rounds that saw more than one round start block.",
		},
	)
	goldfishEquivocationCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "goldfish_equivocation_total",
			Help: "The number of validators observed signing two different available attestations for the same slot.",
		},
	)
)

func updatePayloadNodeMetrics(s *Store) {
	payloadEmptyNodeCount.Set(float64(len(s.emptyNodeByRoot)))
	payloadFullNodeCount.Set(float64(len(s.fullNodeByRoot)))
}

package doublylinkedtree

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// goldfishVoteRetention is how many slots behind the current one available
// attestation votes are kept for. The walk only ever reads the previous slot;
// the extra slack absorbs late arrivals and clock disparity.
const goldfishVoteRetention = primitives.Slot(3)

// goldfishVote is one validator's available attestation vote for one slot.
type goldfishVote struct {
	root           [32]byte
	payloadPresent bool
	// seats is the number of available committee seats the signer holds in the
	// vote's slot. The committee is a fixed size list with repeats, so a
	// validator holding k seats counts k in both score and denominator.
	seats uint64
}

// goldfishVotes holds the available attestation votes that drive the Goldfish
// head walk, keyed by slot and then by validator index.
//
// This is deliberately NOT the `f.votes` store: those are epoch granular and
// never expire, while an available attestation vote is only read during the
// slot after the one it names.
//
// The caller must hold the forkchoice write lock for every method here.
type goldfishVotes struct {
	votes map[primitives.Slot]map[primitives.ValidatorIndex]goldfishVote
	// equivocators are validators that signed two different available
	// attestations for the same slot. Their first vote stays in `votes` so
	// they keep counting in the viability denominator.
	equivocators map[primitives.Slot]map[primitives.ValidatorIndex]bool
}

func newGoldfishVotes() *goldfishVotes {
	return &goldfishVotes{
		votes:        make(map[primitives.Slot]map[primitives.ValidatorIndex]goldfishVote),
		equivocators: make(map[primitives.Slot]map[primitives.ValidatorIndex]bool),
	}
}

// insert records a validator's vote for the given slot. A second vote with
// different content moves the validator to the slot's equivocation set; the
// first vote is kept so the validator still counts in the denominator.
func (g *goldfishVotes) insert(
	slot primitives.Slot, index primitives.ValidatorIndex, v goldfishVote,
) {
	if g.equivocators[slot][index] {
		return
	}
	byIndex, ok := g.votes[slot]
	if !ok {
		byIndex = make(map[primitives.ValidatorIndex]goldfishVote)
		g.votes[slot] = byIndex
	}
	previous, ok := byIndex[index]
	if !ok {
		byIndex[index] = v
		return
	}
	if previous.root == v.root && previous.payloadPresent == v.payloadPresent {
		return
	}
	eq, ok := g.equivocators[slot]
	if !ok {
		eq = make(map[primitives.ValidatorIndex]bool)
		g.equivocators[slot] = eq
	}
	eq[index] = true
	goldfishEquivocationCount.Inc()
}

// prune drops every slot that is more than goldfishVoteRetention behind the
// given slot.
func (g *goldfishVotes) prune(current primitives.Slot) {
	// An equivocation is only ever recorded for a slot that already holds a
	// vote, so the two maps have the same slot keys.
	for slot := range g.votes {
		if slot+goldfishVoteRetention < current {
			delete(g.votes, slot)
			delete(g.equivocators, slot)
		}
	}
}

// seats returns the number of committee seats that voted for the given slot,
// equivocators included. This is the electorate the majority gate divides.
func (g *goldfishVotes) seats(slot primitives.Slot) uint64 {
	total := uint64(0)
	for _, v := range g.votes[slot] {
		total += v.seats
	}
	return total
}

// voters returns the number of validators that hold a vote for the given slot.
// One gossip message carries one validator's seats, so this counts the
// validators the node heard from.
func (g *goldfishVotes) voters(slot primitives.Slot) uint64 {
	return uint64(len(g.votes[slot]))
}

// goldfishProposals records the round start blocks that arrived during their
// own round. It is the stub of the spec's store.round_proposals and
// store.round_proposal_conflicts: the finality gadget that would distinguish a
// proposal by the frozen stable root does not exist here, so the round's single
// round start block plays that role.
//
// The caller must hold the forkchoice write lock for every method here.
type goldfishProposals struct {
	byRound map[primitives.Round][32]byte
	// conflicts are the rounds that saw more than one round start block. The
	// spec's rule is that two proposals distinguish neither.
	conflicts map[primitives.Round]bool
}

func newGoldfishProposals() *goldfishProposals {
	return &goldfishProposals{
		byRound:   make(map[primitives.Round][32]byte),
		conflicts: make(map[primitives.Round]bool),
	}
}

// insert records a round start block for its round.
func (p *goldfishProposals) insert(round primitives.Round, root [32]byte) {
	previous, ok := p.byRound[round]
	if !ok {
		p.byRound[round] = root
		return
	}
	if previous != root && !p.conflicts[round] {
		p.conflicts[round] = true
		goldfishProposalConflictCount.Inc()
	}
}

// distinguished returns the round's proposal root, if the round saw exactly one
// round start block.
func (p *goldfishProposals) distinguished(round primitives.Round) ([32]byte, bool) {
	if p.conflicts[round] {
		return [32]byte{}, false
	}
	root, ok := p.byRound[round]
	return root, ok
}

// prune drops every round before the given one.
func (p *goldfishProposals) prune(current primitives.Round) {
	for round := range p.byRound {
		if round < current {
			delete(p.byRound, round)
			delete(p.conflicts, round)
		}
	}
}

// recordRoundProposal notes a block that starts the current round, mirroring
// the spec's update_round_proposals: a block is a candidate proposal only for
// its own round, and only while that round is the current one.
func (s *Store) recordRoundProposal(n *Node) {
	if !goldfishActiveAt(n.slot) || !slots.IsRoundStart(n.slot) {
		return
	}
	round := slots.RoundAt(n.slot)
	if slots.RoundAt(s.currentSlot()) != round {
		return
	}
	s.goldfishProposals.insert(round, n.root)
}

// InsertAvailableAttestation records a validated available attestation vote.
// The caller MUST hold the forkchoice write lock.
func (f *ForkChoice) InsertAvailableAttestation(
	slot primitives.Slot,
	index primitives.ValidatorIndex,
	seats uint64,
	root [32]byte,
	payloadPresent bool,
) {
	if seats == 0 {
		return
	}
	s := f.store
	if slot < s.currentSlot() {
		goldfishLateVoteCount.Inc()
	}
	s.goldfishVotes.insert(slot, index, goldfishVote{
		root:           root,
		payloadPresent: payloadPresent,
		seats:          seats,
	})
}

// goldfishActive reports whether the Goldfish head vote is the head rule, that
// is whether the wall clock has reached the Heze fork. Shipped configs keep
// HezeForkEpoch at the far future epoch, so this is false everywhere but on
// the devnet, e2e and simulation configs.
func (s *Store) goldfishActive() bool {
	return goldfishActiveAt(s.currentSlot())
}

// goldfishActiveAt reports whether the given slot is at or after the Heze fork.
func goldfishActiveAt(slot primitives.Slot) bool {
	return slots.ToEpoch(slot) >= params.BeaconConfig().HezeForkEpoch
}

// goldfishNewSlot prunes the vote store and reports the votes heard for the slot
// that just ended, as a metric and as the Goldfish votes summary line. Called
// from NewSlot, that is at the slot boundary, which is exactly the "before the
// next slot start" cutoff. A vote that arrives after the tick is not counted.
func (s *Store) goldfishNewSlot(slot primitives.Slot) {
	if slot > 0 {
		seats := s.goldfishVotes.seats(slot - 1)
		goldfishSeatFraction.Set(float64(seats) / float64(decoupled.AvailableAttestationCommitteeSize))
		fields := decoupled.SummaryFields(slot - 1)
		fields["votes"] = s.goldfishVotes.voters(slot - 1)
		fields["seats"] = seats
		fields["committeeSeats"] = uint64(decoupled.AvailableAttestationCommitteeSize)
		log.WithFields(fields).Info("Goldfish votes")
	}
	s.goldfishVotes.prune(slot)
	s.goldfishProposals.prune(slots.RoundAt(slot))
}

// goldfishScores holds one walk's available-attestation scores. Scores are
// accumulated bottom up, once per vote, so the cost is the number of voters
// times the depth of the tree rather than depth times children times voters.
//
// A node here is the spec's ForkChoiceNode: `pending` is keyed by the consensus
// node (PAYLOAD_STATUS_PENDING) and `payload` by the empty or full payload node
// (PAYLOAD_STATUS_EMPTY / PAYLOAD_STATUS_FULL).
type goldfishScores struct {
	pending map[*Node]uint64
	payload map[*PayloadNode]uint64
	// equivocatorSeats is the seat count of every validator that equivocated in
	// the scored slot. The spec adds it to every child's score: an equivocator
	// supports nothing in particular, and it must not be able to keep a child
	// below the gate.
	equivocatorSeats uint64
	// threshold is half the seats that voted, equivocators included. A child is
	// viable when its score is strictly greater.
	threshold uint64
	// noVotes reports that the scored slot recorded no vote at all, so the gate
	// has no electorate to divide and must not veto anything. This is a
	// deviation from the spec, which compares score > threshold unconditionally
	// and so refuses every child of an empty store.
	//
	// The spec is written for a node at the tip, where the previous slot always
	// has voters. A node importing history has none and never will: available
	// attestations live in gossip for one slot and are not carried by blocks,
	// so no vote for slot N ever arrives after slot N+3. Vetoing on an empty
	// store pins such a node's head at the justified root forever - it imports
	// the whole chain and still reports the checkpoint as its head, which stalls
	// initial sync, because the sync state machine reads HeadSlot to decide what
	// it still needs.
	noVotes bool
}

func (sc *goldfishScores) nodeScore(n *Node) uint64 {
	return sc.pending[n] + sc.equivocatorSeats
}

func (sc *goldfishScores) payloadScore(p *PayloadNode) uint64 {
	return sc.payload[p] + sc.equivocatorSeats
}

// goldfishScoresForSlot scores every node between the justified root and the
// blocks named by the given slot's available attestation votes. It implements
// the spec's get_available_attestation_score and
// get_available_majority_threshold together, in one bottom up pass.
func (s *Store) goldfishScoresForSlot(voteSlot primitives.Slot, justified *Node) *goldfishScores {
	sc := &goldfishScores{
		pending: make(map[*Node]uint64),
		payload: make(map[*PayloadNode]uint64),
	}
	equivocators := s.goldfishVotes.equivocators[voteSlot]
	seats := uint64(0)
	for index, v := range s.goldfishVotes.votes[voteSlot] {
		seats += v.seats
		if equivocators[index] {
			sc.equivocatorSeats += v.seats
			continue
		}
		s.creditGoldfishVote(sc, voteSlot, v, justified)
	}
	sc.threshold = seats / 2
	sc.noVotes = seats == 0
	return sc
}

// creditGoldfishVote adds a vote's seats to the node it supports and to every
// ancestor of that node up to the justified root.
func (s *Store) creditGoldfishVote(
	sc *goldfishScores, voteSlot primitives.Slot, v goldfishVote, justified *Node,
) {
	en, ok := s.emptyNodeByRoot[v.root]
	if !ok || en == nil {
		return
	}
	n := en.node
	// get_available_vote_payload_status: a vote whose block precedes its own
	// slot decides full or empty by the payload_present bit, a same slot vote
	// decides nothing and supports the pending node only.
	if n.slot < voteSlot {
		supported := en
		if v.payloadPresent {
			supported = s.fullNodeByRoot[v.root]
		}
		if supported != nil {
			sc.payload[supported] += v.seats
		}
	}
	for n != nil {
		sc.pending[n] += v.seats
		if n == justified {
			return
		}
		p := n.parent
		if p == nil {
			return
		}
		sc.payload[p] += v.seats
		n = p.node
	}
}

// goldfishNodeViable implements is_available_attestation_viable for a beacon
// block (a PAYLOAD_STATUS_PENDING node).
func goldfishNodeViable(n *Node, sc *goldfishScores, current primitives.Slot) bool {
	if n.slot == current {
		// The current slot's proposal passes through: its own slot's votes are
		// only read one slot later, so it scores zero by construction. A round
		// start proposal is the exception, it has to earn its votes.
		return !slots.IsRoundStart(n.slot)
	}
	if sc.noVotes {
		return true
	}
	return sc.nodeScore(n) > sc.threshold
}

// goldfishPayloadViable implements is_available_attestation_viable for a
// payload decision node (PAYLOAD_STATUS_EMPTY / PAYLOAD_STATUS_FULL).
func goldfishPayloadViable(p *PayloadNode, sc *goldfishScores, current primitives.Slot) bool {
	if p.node.slot+1 == current {
		// is_ptc_decision_node: the previous slot's payload decision passes
		// through and is settled by the tiebreaker instead.
		return true
	}
	if sc.noVotes {
		return true
	}
	return sc.payloadScore(p) > sc.threshold
}

// goldfishPayloadTiebreaker mirrors the spec's get_payload_status_tiebreaker:
// full beats empty except when the previous slot's payload decision is being
// made and the chain should not extend the payload.
func (s *Store) goldfishPayloadTiebreaker(p *PayloadNode, current primitives.Slot) uint8 {
	if p.node.slot+1 == current {
		if !p.full {
			return 1
		}
		if s.shouldExtendPayload(p) {
			return 2
		}
		return 0
	}
	if p.full {
		return 2
	}
	return 1
}

// goldfishBestPayload picks between the empty and full payload node of n, over
// the ones that clear the gate. hadCandidates reports whether a refused node
// had blocks below it, that is whether the gate is what stopped the walk.
func (s *Store) goldfishBestPayload(
	n *Node, sc *goldfishScores, current primitives.Slot,
) (best *PayloadNode, hadCandidates bool) {
	bestScore, bestTiebreak := uint64(0), uint8(0)
	for _, p := range [2]*PayloadNode{s.emptyNodeByRoot[n.root], s.fullNodeByRoot[n.root]} {
		if p == nil {
			continue
		}
		hadCandidates = hadCandidates || len(p.children) > 0
		if !goldfishPayloadViable(p, sc, current) {
			continue
		}
		score := sc.payloadScore(p)
		tiebreak := s.goldfishPayloadTiebreaker(p, current)
		if best == nil || score > bestScore || (score == bestScore && tiebreak > bestTiebreak) {
			best, bestScore, bestTiebreak = p, score, tiebreak
		}
	}
	return best, hadCandidates
}

// goldfishBestChild picks the winning block among the blocks that build on the
// given payload node, over the ones that clear the gate. Ties break on the
// larger root, as in the spec's max key.
func (s *Store) goldfishBestChild(
	p *PayloadNode,
	sc *goldfishScores,
	current primitives.Slot,
	justifiedEpoch, currentEpoch primitives.Round,
) (best *Node, hadCandidates bool) {
	bestScore := uint64(0)
	for _, child := range p.children {
		if child == nil || !child.leadsToViableHead(justifiedEpoch, currentEpoch) {
			continue
		}
		hadCandidates = true
		if !goldfishNodeViable(child, sc, current) {
			continue
		}
		score := sc.nodeScore(child)
		if best == nil || score > bestScore ||
			(score == bestScore && bytes.Compare(child.root[:], best.root[:]) > 0) {
			best, bestScore = child, score
		}
	}
	return best, hadCandidates
}

// goldfishRoundProposal implements the proposal arm of the spec's get_head
// phase 2. At a round start slot the round's distinguished proposal starts the
// walk, which is how a round start block becomes head at all: the ordinary walk
// refuses it (is_available_attestation_viable) because a proposal cannot have
// earned votes yet.
//
// The spec distinguishes the proposal at the round's available vote action,
// against the frozen stable root and the live available confirmed head. Neither
// exists here: the stable root is stubbed as the justified root, so the checks
// that remain are the ones that do not need the gadget - exactly one round start
// block was received during its own round, it is the block of this slot, it
// descends from the stable root, and it is viable for the head. Two round start
// blocks distinguish neither, as in the spec. Unlike the spec this decision is
// not frozen, so a second proposal arriving later in the slot withdraws the
// first one's distinction rather than being ignored; that is the conservative
// direction, the round start slot is then orphaned as it was before the stub.
func (s *Store) goldfishRoundProposal(justified *Node, current primitives.Slot) *Node {
	if !slots.IsRoundStart(current) {
		return nil
	}
	root, ok := s.goldfishProposals.distinguished(slots.RoundAt(current))
	if !ok {
		return nil
	}
	en, ok := s.emptyNodeByRoot[root]
	if !ok || en == nil {
		return nil
	}
	n := en.node
	if n.slot != current || !isGoldfishAncestor(n, justified) {
		return nil
	}
	if !n.leadsToViableHead(s.justifiedCheckpoint.Epoch, slots.RoundAt(current)) {
		return nil
	}
	return n
}

// goldfishDescend runs phase 2 of the spec's get_head: from the stable root
// (here the justified root) or from the round's distinguished proposal, follow
// the available chain while a child clears the previous slot's majority gate.
func (s *Store) goldfishDescend(
	from *Node, sc *goldfishScores, current primitives.Slot,
) *Node {
	justifiedEpoch := s.justifiedCheckpoint.Epoch
	currentEpoch := slots.RoundAt(current)
	head := from
	for {
		p, hadPayloads := s.goldfishBestPayload(head, sc, current)
		if p == nil {
			if hadPayloads {
				goldfishGateStopCount.Inc()
			}
			return head
		}
		child, hadChildren := s.goldfishBestChild(p, sc, current, justifiedEpoch, currentEpoch)
		if child == nil {
			if hadChildren {
				goldfishGateStopCount.Inc()
			}
			return head
		}
		head = child
	}
}

// goldfishHead returns the head chosen by the Goldfish walk. It replaces the
// best descendant walk once the Heze fork is active.
func (s *Store) goldfishHead() ([32]byte, error) {
	justified, err := s.justifiedNode()
	if err != nil {
		return [32]byte{}, err
	}
	current := s.currentSlot()
	// The walk reads the previous slot's votes. At genesis there is no previous
	// slot, so there is no electorate and the gate does not veto.
	sc := &goldfishScores{
		pending: map[*Node]uint64{},
		payload: map[*PayloadNode]uint64{},
		noVotes: true,
	}
	if current > 0 {
		sc = s.goldfishScoresForSlot(current-1, justified)
	}
	start := justified
	if proposal := s.goldfishRoundProposal(justified, current); proposal != nil {
		start = proposal
		goldfishRoundProposalCount.Inc()
	}
	head := s.goldfishDescend(start, sc, current)
	s.allTipsAreInvalid = false
	previous := s.headNode
	if head != previous {
		if previous != nil && isGoldfishAncestor(previous, head) {
			goldfishGateRetreatCount.Inc()
		}
		headChangesCount.Inc()
		headSlotNumber.Set(float64(head.slot))
		s.headNode = head
	}
	return head.root, nil
}

// isGoldfishAncestor reports whether ancestor is n or one of its ancestors.
func isGoldfishAncestor(n, ancestor *Node) bool {
	for n != nil && n.slot > ancestor.slot {
		if n.parent == nil {
			return false
		}
		n = n.parent.node
	}
	return n == ancestor
}

// isCanonicalGoldfish reports whether n is on the chain leading to the head the
// walk chose. The best descendant pointers no longer describe that chain, so
// this walks parents from the head instead.
func (s *Store) isCanonicalGoldfish(n *Node) bool {
	if s.headNode == nil {
		return false
	}
	return isGoldfishAncestor(s.headNode, n)
}

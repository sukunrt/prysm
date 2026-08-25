package doublylinkedtree

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// applyProposerBoostScore applies the current proposer boost scores to the
// relevant nodes.
func (f *ForkChoice) applyProposerBoostScore() error {
	s := f.store
	proposerScore := uint64(0)
	s.removePreviousProposerBoost()
	// After Heze the current-slot passthrough in the Goldfish walk is what
	// admits a fresh block, so the boost weight is not applied. The boost root
	// itself is still recorded at insert time: it is the only record of which
	// block arrived timely this slot, and the walk's payload tiebreaker reads
	// it through shouldExtendPayload.
	if !s.goldfishActive() && s.shouldApplyProposerBoost() {
		proposerScore = s.applyNewProposerBoost()
	}
	s.previousProposerBoostRoot = s.proposerBoostRoot
	s.previousProposerBoostScore = proposerScore
	return nil
}

func (s *Store) removePreviousProposerBoost() {
	if s.previousProposerBoostRoot == params.BeaconConfig().ZeroHash {
		return
	}
	previousNode, ok := s.emptyNodeByRoot[s.previousProposerBoostRoot]
	if !ok || previousNode == nil {
		log.WithError(errInvalidProposerBoostRoot).Errorf("invalid prev root %#x", s.previousProposerBoostRoot)
		return
	}
	n := previousNode.node
	if n.balance < s.previousProposerBoostScore {
		log.Errorf("invalid proposer boost score %d for node balance %d", s.previousProposerBoostScore, n.balance)
		n.balance = 0
		return
	}
	n.balance -= s.previousProposerBoostScore
}

// applyNewProposerBoost applies the new proposer boost and returns the new proposer boost score.
func (s *Store) applyNewProposerBoost() uint64 {
	if s.proposerBoostRoot == params.BeaconConfig().ZeroHash {
		return 0
	}
	currentNode, ok := s.emptyNodeByRoot[s.proposerBoostRoot]
	if !ok || currentNode == nil {
		log.WithError(errInvalidProposerBoostRoot).Errorf("invalid current root %#x", s.proposerBoostRoot)
		return 0
	}
	proposerScore := (s.committeeWeight * params.BeaconConfig().ProposerScoreBoost) / 100
	currentNode.node.balance += proposerScore
	return proposerScore
}

// ProposerBoost of fork choice store.
func (s *Store) proposerBoost() [fieldparams.RootLength]byte {
	return s.proposerBoostRoot
}

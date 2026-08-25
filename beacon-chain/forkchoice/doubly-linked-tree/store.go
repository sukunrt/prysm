package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// head starts from justified root and then follows the best descendant links
// to find the best block for head.
func (s *Store) head(ctx context.Context) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.head")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}

	// After Heze the head is the Goldfish walk over the available attestation
	// votes, not the best descendant of the justified node.
	if s.goldfishActive() {
		return s.goldfishHead()
	}

	jn, err := s.justifiedNode()
	if err != nil {
		return [32]byte{}, err
	}

	// If the justified node doesn't have a best descendant,
	// the best node is itself.
	bestDescendant := jn.bestDescendant
	if bestDescendant == nil {
		bestDescendant = jn
	}
	currentRound := slots.RoundsSinceGenesis(s.genesisTime)
	if !bestDescendant.viableForHead(s.justifiedCheckpoint.Epoch, currentRound) {
		s.allTipsAreInvalid = true
		return [32]byte{}, fmt.Errorf("head at slot %d with weight %d is not eligible, finalizedRound, justified Round %d, %d != %d, %d",
			bestDescendant.slot, bestDescendant.weight/10e9, bestDescendant.finalizedEpoch, bestDescendant.justifiedEpoch, s.finalizedCheckpoint.Epoch, s.justifiedCheckpoint.Epoch)
	}
	s.allTipsAreInvalid = false

	// Update metrics.
	if bestDescendant != s.headNode {
		headChangesCount.Inc()
		headSlotNumber.Set(float64(bestDescendant.slot))
		s.headNode = bestDescendant
	}

	return bestDescendant.root, nil
}

// justifiedNode returns the store's justified node, which is where every head
// walk starts.
func (s *Store) justifiedNode() (*Node, error) {
	if ej := s.emptyNodeByRoot[s.justifiedCheckpoint.Root]; ej != nil {
		return ej.node, nil
	}
	// If the justifiedCheckpoint is from genesis, then the root is zeroHash. In
	// this case it should be the root of the forkchoice tree.
	if s.justifiedCheckpoint.Epoch == genesisRound {
		return s.treeRootNode, nil
	}
	return nil, errors.WithMessage(errUnknownJustifiedRoot, fmt.Sprintf("%#x", s.justifiedCheckpoint.Root))
}

// insert registers a new block node to the fork choice store's node list.
// It then updates the new node's parent with the best child and descendant node.
func (s *Store) insert(ctx context.Context,
	roblock consensus_blocks.ROBlock,
	justifiedEpoch, finalizedEpoch primitives.Round,
) (*PayloadNode, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.insert")
	defer span.End()

	root := roblock.Root()
	// Return if the block has been inserted into Store before.
	if n, ok := s.emptyNodeByRoot[root]; ok {
		return n, nil
	}

	block := roblock.Block()
	slot := block.Slot()
	var parent *PayloadNode
	blockHash := &[32]byte{}
	var gasLimit uint64
	if block.Version() >= version.Gloas {
		if err := s.resolveParentPayloadStatus(block, &parent, blockHash, &gasLimit); err != nil {
			return nil, err
		}
	} else {
		if block.Version() >= version.Bellatrix {
			execution, err := block.Body().Execution()
			if err != nil {
				return nil, err
			}
			copy(blockHash[:], execution.BlockHash())
			gasLimit = execution.GasLimit()
		}
		parentRoot := block.ParentRoot()
		en := s.emptyNodeByRoot[parentRoot]
		parent = s.fullNodeByRoot[parentRoot]
		if parent == nil && en != nil {
			// pre-Gloas only full parents are allowed.
			return nil, errInvalidParentRoot
		}
	}

	n := &Node{
		slot:                        slot,
		proposerIndex:               block.ProposerIndex(),
		root:                        root,
		parent:                      parent,
		justifiedEpoch:              justifiedEpoch,
		unrealizedJustifiedEpoch:    justifiedEpoch,
		finalizedEpoch:              finalizedEpoch,
		unrealizedFinalizedEpoch:    finalizedEpoch,
		blockHash:                   *blockHash,
		payloadAvailabilityVote:     bitfield.NewBitvector512(),
		payloadDataAvailabilityVote: bitfield.NewBitvector512(),
		payloadAttesters:            bitfield.NewBitvector512(),
	}
	// Set the node's target checkpoint. The decoupled fork's FFG target for
	// round R is the block at slot RoundStart(R) - FFG_TARGET_OFFSET_SLOTS, so it
	// is the deepest ancestor at or before that slot. The state side computes the
	// same root in helpers.FFGTargetRoot off the same slots.FFGTargetSlot; the two
	// must agree or VerifyLmdFfgConsistency rejects every vote.
	targetSlot, err := slots.FFGTargetSlot(slots.RoundAt(slot))
	if err != nil {
		return nil, err
	}
	switch {
	case parent == nil:
		// The anchor: round 0 underflows, and a checkpoint sync anchor has no
		// ancestor left. Both fall back to the anchor root itself.
		n.target = n
	case slot <= targetSlot:
		// Offset 0 only: a block exactly at its round's first slot is its own target.
		// At offset 1 the target sits one slot earlier, so this arm never fires.
		n.target = n
	case slots.RoundAt(slot) == slots.RoundAt(parent.node.slot):
		n.target = parent.node.target
	default:
		// A new round starting at a later slot than RoundStart (empty round start):
		// the parent is still the deepest block at or before the target slot.
		n.target = parent.node
	}
	var ret *PayloadNode
	optimistic := true
	if parent != nil {
		optimistic = n.parent.optimistic
	}
	// Make the empty node.It's optimistic status equals it's parent's status.
	pn := &PayloadNode{
		node:       n,
		optimistic: optimistic,
		timestamp:  time.Now(),
		children:   make([]*Node, 0),
	}
	s.emptyNodeByRoot[root] = pn
	ret = pn
	if block.Version() < version.Gloas {
		// Make also the full node, this is optimistic until the engine returns the execution payload validation.
		fn := &PayloadNode{
			node:       n,
			optimistic: true,
			timestamp:  time.Now(),
			full:       true,
			gasLimit:   gasLimit,
		}
		ret = fn
		s.fullNodeByRoot[root] = fn
	} else if parent == nil && slot == 0 {
		// A Gloas genesis block commits to the execution genesis block, which is present by
		// definition, so genesis is full. No payload envelope is ever imported for it, and
		// nothing else creates its full node, so make it here.
		s.fullNodeByRoot[root] = &PayloadNode{
			node:       n,
			optimistic: true,
			timestamp:  time.Now(),
			full:       true,
			gasLimit:   gasLimit,
			children:   make([]*Node, 0),
		}
	}

	if parent == nil {
		if s.treeRootNode == nil {
			s.treeRootNode = n
			s.headNode = n
			s.highestReceivedNode = n
		} else {
			delete(s.emptyNodeByRoot, root)
			delete(s.fullNodeByRoot, root)
			updatePayloadNodeMetrics(s)
			return nil, errInvalidParentRoot
		}
	} else {
		parent.children = append(parent.children, n)
		// Apply proposer boost
		now := time.Now()
		if now.Before(s.genesisTime) {
			return ret, nil
		}
		currentSlot := slots.CurrentSlot(s.genesisTime)
		sss, err := slots.SinceSlotStart(currentSlot, s.genesisTime, now)
		if err != nil {
			return nil, fmt.Errorf("could not determine time since current slot started: %w", err)
		}
		bps := params.BeaconConfig().AttestationDueBPS
		if block.Version() >= version.Gloas {
			bps = params.BeaconConfig().AttestationDueBPSGloas
		}
		boostThreshold := params.BeaconConfig().SlotComponentDuration(bps)
		isFirstBlock := s.proposerBoostRoot == [32]byte{}
		if currentSlot == slot && sss < boostThreshold && isFirstBlock {
			depEpoch := slots.ToEpoch(currentSlot)
			if depEpoch > 0 {
				depEpoch--
			}
			depRoot, err := s.dependentRootForEpoch(root, depEpoch)
			if err != nil {
				return nil, errors.Wrap(err, "could not get block dependent root.")
			}
			headDepRoot, err := s.dependentRoot(depEpoch)
			if err != nil {
				return nil, errors.Wrap(err, "could not get head dependent root.")
			}
			if depRoot == headDepRoot {
				s.proposerBoostRoot = root
			}
		}

		// Update best descendants
		jEpoch := s.justifiedCheckpoint.Epoch
		fEpoch := s.finalizedCheckpoint.Epoch
		if err := s.updateBestDescendantConsensusNode(ctx, s.treeRootNode, jEpoch, fEpoch, slots.RoundAt(currentSlot)); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"slot": slot,
				"root": root,
			}).Error("Could not update best descendant")
		}
	}
	// A block that starts the current round can be the round's distinguished
	// proposal, which is the only way a round start block becomes head.
	s.recordRoundProposal(n)

	// Update metrics.
	processedBlockCount.Inc()
	nodeCount.Set(float64(len(s.emptyNodeByRoot)))
	updatePayloadNodeMetrics(s)

	// Only update received block slot if it's within epoch from current time.
	if slot+params.BeaconConfig().SlotsPerEpoch > slots.CurrentSlot(s.genesisTime) {
		s.receivedBlocksLastEpoch[slot%params.BeaconConfig().SlotsPerEpoch] = slot
	}
	// Update highest slot tracking.
	if slot > s.highestReceivedNode.slot {
		s.highestReceivedNode = n
	}

	return ret, nil
}

// pruneFinalizedNodeByRootMap prunes the `nodeByRoot` maps
// starting from `node` down to the finalized Node or to a leaf of the Fork
// choice store.
func (s *Store) pruneFinalizedNodeByRootMap(ctx context.Context, node, finalizedNode *Node) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if node == finalizedNode {
		// The finalized node becomes the anchor: whatever its FFG target was,
		// that block is being pruned away, so the anchor becomes its own
		// target the way a genesis or checkpoint sync anchor is.
		node.target = node
		return nil
	}
	for _, child := range s.allConsensusChildren(node) {
		if err := s.pruneFinalizedNodeByRootMap(ctx, child, finalizedNode); err != nil {
			return err
		}
	}
	en := s.emptyNodeByRoot[node.root]
	en.children = nil
	delete(s.emptyNodeByRoot, node.root)
	fn := s.fullNodeByRoot[node.root]
	if fn != nil {
		fn.children = nil
		delete(s.fullNodeByRoot, node.root)
	}
	updatePayloadNodeMetrics(s)
	return nil
}

// pruneHorizonSlot returns the slot below which fork choice stops keeping nodes.
//
// Pruning is a memory optimization, not a correctness mechanism: head selection is
// gated by the justified and finalized checkpoints, so nothing below finality can
// win whether or not its node is still in the tree. What pruning must not do is
// delete nodes that live lookups still reach.
//
// dependentRootForEpoch is epoch-keyed -- shuffling is an epoch concept -- and its
// callers ask for the boundary of currentEpoch-1, so the tree has to reach that far
// back. Per-round finality lands two rounds behind head, which at 8 slots per round
// inside a 32-slot epoch is well within a single epoch; cutting at the finalized
// round would put that epoch boundary below the tree root on every prune and every
// insert would fail. So keep the cut on the epoch rhythm and trail it by two
// epochs from the epoch holding the finalized round's first slot -- the distance
// the epoch-keyed world got for free when finality itself lagged two epochs.
//
// This is how FAR BACK the horizon sits. Where the cut lands relative to a round's
// FFG target -- the offset-dependent geometry -- is a separate question, handled by
// the incompatible-children pass in prune.
func pruneHorizonSlot(finalizedRound primitives.Round) (primitives.Slot, error) {
	roundStart, err := slots.RoundStart(finalizedRound)
	if err != nil {
		return 0, err
	}
	epochStart, err := slots.EpochStart(slots.ToEpoch(roundStart))
	if err != nil {
		return 0, err
	}
	trail := params.BeaconConfig().SlotsPerEpoch.Mul(2)
	if epochStart < trail {
		return 0, nil
	}
	return epochStart - trail, nil
}

// prune prunes the fork choice store. It removes all nodes that compete with the finalized root.
// This function does not prune for invalid optimistically synced nodes, it deals only with pruning upon finalization
// TODO: Gloas, to ensure that chains up to a full node are found, we may want to consider pruning only up to the latest full block that was finalized
func (s *Store) prune(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.Prune")
	defer span.End()

	// The tree walk below used to be the only thing that noticed a cancelled
	// context, which made the check depend on whether there was anything to cut.
	// Now that the horizon can leave the tree untouched, check up front.
	if err := ctx.Err(); err != nil {
		return err
	}

	finalizedRoot := s.finalizedCheckpoint.Root
	finalizedRound := s.finalizedCheckpoint.Epoch
	fen, ok := s.emptyNodeByRoot[finalizedRoot]
	if !ok || fen == nil {
		return errors.WithMessage(errUnknownFinalizedRoot, fmt.Sprintf("%#x", finalizedRoot))
	}
	fn := fen.node
	// return early if we haven't changed the finalized checkpoint
	if fn.parent == nil {
		return nil
	}
	s.finalizedPayloadBlockHash = s.checkpointPayloadHashForRoot(finalizedRoot)

	// Cut the tree back to the pruning horizon. The new tree root is the deepest
	// ancestor of the finalized node at or below the horizon, so the horizon only
	// ever moves along the finalized chain.
	boundSlot, err := pruneHorizonSlot(finalizedRound)
	if err != nil {
		return errors.Wrap(err, "could not compute prune horizon")
	}
	newRoot := fn
	for newRoot.slot > boundSlot && newRoot.parent != nil {
		newRoot = newRoot.parent.node
	}
	if newRoot.parent != nil {
		// Save the dependent root below the new tree root, since it is about to go.
		s.finalizedDependentRoot = newRoot.parent.node.root

		// Prune nodeByRoot starting from root
		if err := s.pruneFinalizedNodeByRootMap(ctx, s.treeRootNode, newRoot); err != nil {
			return err
		}

		newRoot.parent = nil
		s.treeRootNode = newRoot

		prunedCount.Inc()
	}

	// Prune all children of the finalized checkpoint block that are incompatible
	// with it. A chain's FFG target for the finalized round is its deepest block at
	// or before slots.FFGTargetSlot(round), so a child of the finalized node is
	// compatible exactly when it sits strictly after that slot: a child at or before
	// it would be the round's target on its own chain, not the finalized root. Both
	// offsets fall out of this one expression -- at offset 1 the bound is the round's
	// first slot, at offset 0 it is one slot later.
	targetSlot, err := slots.FFGTargetSlot(finalizedRound)
	if err != nil {
		return errors.Wrap(err, "could not compute ffg target slot")
	}
	firstCompatibleSlot := targetSlot + 1
	if fn.slot+1 >= firstCompatibleSlot {
		return nil
	}

	remaining := fen.children[:0]
	for _, child := range fen.children {
		if child != nil && child.slot < firstCompatibleSlot {
			if err := s.pruneFinalizedNodeByRootMap(ctx, child, fn); err != nil {
				return errors.Wrap(err, "could not prune incompatible finalized child")
			}
			continue
		}
		remaining = append(remaining, child)
	}
	fen.children = remaining
	ffn := s.fullNodeByRoot[finalizedRoot]
	if ffn == nil {
		return nil
	}
	remaining = ffn.children[:0]
	for _, child := range ffn.children {
		if child != nil && child.slot < firstCompatibleSlot {
			if err := s.pruneFinalizedNodeByRootMap(ctx, child, fn); err != nil {
				return errors.Wrap(err, "could not prune incompatible finalized child")
			}
			continue
		}
		remaining = append(remaining, child)
	}
	ffn.children = remaining
	return nil
}

// tips returns a list of possible heads from fork choice store, it returns the
// roots and the slots of the leaf nodes.
func (s *Store) tips() ([][32]byte, []primitives.Slot) {
	var roots [][32]byte
	var slots []primitives.Slot

	for root, n := range s.emptyNodeByRoot {
		if !s.hasConsensusChildren(n.node) {
			roots = append(roots, root)
			slots = append(slots, n.node.slot)
		}
	}
	return roots, slots
}

func (f *ForkChoice) HighestReceivedBlockRoot() [32]byte {
	if f.store.highestReceivedNode == nil {
		return [32]byte{}
	}
	return f.store.highestReceivedNode.root
}

// HighestReceivedBlockSlot returns the highest slot received by the forkchoice
func (f *ForkChoice) HighestReceivedBlockSlot() primitives.Slot {
	if f.store.highestReceivedNode == nil {
		return 0
	}
	return f.store.highestReceivedNode.slot
}

// ReceivedBlocksLastEpoch returns the number of blocks received in the last epoch
func (f *ForkChoice) ReceivedBlocksLastEpoch() (uint64, error) {
	count := uint64(0)
	lowerBound := slots.CurrentSlot(f.store.genesisTime)
	var err error
	if lowerBound > fieldparams.SlotsPerEpoch {
		lowerBound, err = lowerBound.SafeSub(fieldparams.SlotsPerEpoch)
		if err != nil {
			return 0, err
		}
	}

	for _, s := range f.store.receivedBlocksLastEpoch {
		if s != 0 && lowerBound <= s {
			count++
		}
	}
	return count, nil
}

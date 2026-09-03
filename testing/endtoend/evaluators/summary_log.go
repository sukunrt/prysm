package evaluators

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/decoupled"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SummaryLogLines checks the four per-slot Goldfish summary lines on every
// beacon node's log: that each is written, parses, and agrees with the others.
// When the run also carries --goldfish-vote-ledger the summary counts are
// cross-checked against the per-vote ledger lines.
var SummaryLogLines = types.Evaluator{
	Name:       "summary_log_lines_%d",
	Policy:     policies.AllEpochs,
	Evaluation: summaryLogLines,
}

const (
	goldfishVotesLine   = "Goldfish votes"
	ffgVotesLine        = "FFG votes"
	blockReceivedLine   = "Block received"
	payloadReceivedLine = "Payload received"

	goldfishVoteLedgerLine = "Goldfish vote"
	ffgVoteLedgerLine      = "FFG vote"
	ffgIncludedLedgerLine  = "FFG vote included"
)

// logLine is one parsed log line, by field name.
type logLine map[string]string

func (l logLine) num(key string) (int64, error) {
	value, ok := l[key]
	if !ok {
		return 0, fmt.Errorf("%q line has no %s field", l["msg"], key)
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "%s of the %q line is not a number", key, l["msg"])
	}
	return n, nil
}

// nodeLog holds the lines of one beacon node: the summary lines by message and
// slot, and the ledger lines by message.
type nodeLog struct {
	summary map[string]map[primitives.Slot][]logLine
	ledger  map[string][]logLine
}

func (n *nodeLog) at(message string, slot primitives.Slot) []logLine {
	return n.summary[message][slot]
}

// one returns the single line of the given message and slot, or an error naming
// how many there were instead.
func (n *nodeLog) one(message string, slot primitives.Slot) (logLine, error) {
	lines := n.at(message, slot)
	if len(lines) != 1 {
		return nil, fmt.Errorf("slot %d has %d %q lines, want exactly 1", slot, len(lines), message)
	}
	return lines[0], nil
}

// logfmtFields splits one logfmt line into its key=value pairs. The e2e log is
// written by a TextFormatter with ForceFormatting off, so a value that holds a
// space is double quoted; helpers.FindFollowingTextInFile splits on spaces and
// cannot read those.
func logfmtFields(line string) logLine {
	fields := make(logLine)
	i := 0
	for i < len(line) {
		if line[i] == ' ' {
			i++
			continue
		}
		keyStart := i
		for i < len(line) && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i == len(line) || line[i] == ' ' {
			continue
		}
		key := line[keyStart:i]
		i++
		var value strings.Builder
		if i < len(line) && line[i] == '"' {
			for i++; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) {
					i++
				}
				value.WriteByte(line[i])
			}
			i++
		} else {
			for ; i < len(line) && line[i] != ' '; i++ {
				value.WriteByte(line[i])
			}
		}
		fields[key] = value.String()
	}
	return fields
}

// readNodeLog scans one beacon node's log once and keeps the summary and the
// ledger lines. The evaluator opens its own handle; helpers.LogOutput opens,
// reads and closes its own.
func readNodeLog(index int) (*nodeLog, error) {
	name := path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, index))
	file, err := os.Open(name) // #nosec G304 -- the path is built from the test params.
	if err != nil {
		return nil, errors.Wrapf(err, "could not open the log of beacon node %d", index)
	}
	defer func() {
		_ = file.Close()
	}()
	node := &nodeLog{
		summary: make(map[string]map[primitives.Slot][]logLine),
		ledger:  make(map[string][]logLine),
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		summary := strings.Contains(line, "purpose="+decoupled.SummaryPurpose)
		if !summary && !strings.Contains(line, `msg="Goldfish vote"`) &&
			!strings.Contains(line, `msg="FFG vote`) {
			continue
		}
		fields := logfmtFields(line)
		message := fields["msg"]
		if !summary {
			switch message {
			case goldfishVoteLedgerLine, ffgVoteLedgerLine, ffgIncludedLedgerLine:
				node.ledger[message] = append(node.ledger[message], fields)
			}
			continue
		}
		slot, err := fields.num("slot")
		if err != nil {
			return nil, errors.Wrapf(err, "beacon node %d", index)
		}
		bySlot, ok := node.summary[message]
		if !ok {
			bySlot = make(map[primitives.Slot][]logLine)
			node.summary[message] = bySlot
		}
		s := primitives.Slot(slot)
		bySlot[s] = append(bySlot[s], fields)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrapf(err, "could not read the log of beacon node %d", index)
	}
	return node, nil
}

func summaryLogLines(_ *types.EvaluationContext, conns ...*grpc.ClientConn) error {
	client := eth.NewBeaconChainClient(conns[0])
	head, err := client.GetChainHead(context.Background(), &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get chain head")
	}
	// Slots 0 and 1 have no full committee history; the last slot may still be
	// open.
	if head.HeadSlot < 4 {
		return nil
	}
	first, last := primitives.Slot(2), head.HeadSlot-2

	nodes := make([]*nodeLog, e2e.TestParams.BeaconNodeCount)
	for i := range nodes {
		if nodes[i], err = readNodeLog(i); err != nil {
			return err
		}
	}
	for i, node := range nodes {
		for slot := first; slot <= last; slot++ {
			if err := checkNodeSlot(node, slot); err != nil {
				return errors.Wrapf(err, "beacon node %d", i)
			}
		}
		if err := checkLedger(node, first, last); err != nil {
			return errors.Wrapf(err, "beacon node %d", i)
		}
	}
	return checkAcrossNodes(nodes, first, last)
}

// checkNodeSlot runs the shape checks of one node's four lines for one slot.
func checkNodeSlot(node *nodeLog, slot primitives.Slot) error {
	if err := checkGoldfishVotes(node, slot); err != nil {
		return err
	}
	if err := checkFFGVotes(node, slot); err != nil {
		return err
	}
	block, err := checkBlockReceived(node, slot)
	if err != nil {
		return err
	}
	return checkPayloadReceived(node, slot, block)
}

func checkGoldfishVotes(node *nodeLog, slot primitives.Slot) error {
	line, err := node.one(goldfishVotesLine, slot)
	if err != nil {
		return err
	}
	votes, err := line.num("votes")
	if err != nil {
		return err
	}
	seats, err := line.num("seats")
	if err != nil {
		return err
	}
	committeeSeats, err := line.num("committeeSeats")
	if err != nil {
		return err
	}
	if seats <= 0 || seats > committeeSeats {
		return fmt.Errorf(
			"slot %d: %q has seats %d, want more than 0 and at most committeeSeats %d",
			slot, goldfishVotesLine, seats, committeeSeats)
	}
	if 3*seats < 2*committeeSeats {
		return fmt.Errorf(
			"slot %d: %q has seats %d, want at least two thirds of committeeSeats %d",
			slot, goldfishVotesLine, seats, committeeSeats)
	}
	if votes > seats {
		return fmt.Errorf("slot %d: %q has votes %d and seats %d, want votes at most seats",
			slot, goldfishVotesLine, votes, seats)
	}
	return nil
}

func checkFFGVotes(node *nodeLog, slot primitives.Slot) error {
	line, err := node.one(ffgVotesLine, slot)
	if err != nil {
		return err
	}
	subnets, err := line.num("subnets")
	if err != nil {
		return err
	}
	votes, err := line.num("votes")
	if err != nil {
		return err
	}
	seats, err := line.num("seats")
	if err != nil {
		return err
	}
	perSubnet, total, err := parsePerSubnet(line["perSubnet"])
	if err != nil {
		return errors.Wrapf(err, "slot %d", slot)
	}
	if int64(len(perSubnet)) != subnets {
		return fmt.Errorf("slot %d: %q lists %d subnets in perSubnet %q, want subnets %d",
			slot, ffgVotesLine, len(perSubnet), line["perSubnet"], subnets)
	}
	if total != votes {
		return fmt.Errorf("slot %d: %q perSubnet %q sums to %d, want votes %d",
			slot, ffgVotesLine, line["perSubnet"], total, votes)
	}
	if seats < votes {
		return fmt.Errorf("slot %d: %q has seats %d and votes %d, want seats at least votes",
			slot, ffgVotesLine, seats, votes)
	}
	return nil
}

// parsePerSubnet reads the subnet:votes pairs of the FFG votes line and returns
// them by subnet together with their sum.
func parsePerSubnet(value string) (map[uint64]int64, int64, error) {
	pairs := make(map[uint64]int64)
	if value == "" {
		return pairs, 0, nil
	}
	total := int64(0)
	for _, pair := range strings.Split(value, ",") {
		subnet, count, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, 0, fmt.Errorf("perSubnet pair %q is not subnet:votes", pair)
		}
		s, err := strconv.ParseUint(subnet, 10, 64)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "perSubnet pair %q has no subnet number", pair)
		}
		n, err := strconv.ParseInt(count, 10, 64)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "perSubnet pair %q has no vote count", pair)
		}
		pairs[s] = n
		total += n
	}
	return pairs, total, nil
}

// checkBlockReceived returns the slot's block line, or nil when the node did not
// see the block on gossip.
func checkBlockReceived(node *nodeLog, slot primitives.Slot) (logLine, error) {
	lines := node.at(blockReceivedLine, slot)
	if len(lines) == 0 {
		return nil, nil
	}
	if len(lines) > 1 {
		return nil, fmt.Errorf("slot %d has %d %q lines, want at most 1",
			slot, len(lines), blockReceivedLine)
	}
	line := lines[0]
	arrived, err := line.num("arrivedMs")
	if err != nil {
		return nil, err
	}
	slotMs := params.BeaconConfig().SlotDuration().Milliseconds()
	if arrived < 0 || arrived >= slotMs {
		return nil, fmt.Errorf("slot %d: %q has arrivedMs %d, want 0 to %d",
			slot, blockReceivedLine, arrived, slotMs)
	}
	size, err := line.num("bytes")
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, fmt.Errorf("slot %d: %q has bytes %d, want more than 0",
			slot, blockReceivedLine, size)
	}
	attestations, err := line.num("attestations")
	if err != nil {
		return nil, err
	}
	ffgSeats, err := line.num("ffgSeats")
	if err != nil {
		return nil, err
	}
	cfg := params.BeaconConfig()
	// lint:ignore uintcast -- preset bounds, far below int64.
	most := attestations * int64(cfg.MaxValidatorsPerCommittee) * int64(cfg.MaxCommitteesPerSlot)
	if ffgSeats > most {
		return nil, fmt.Errorf("slot %d: %q has ffgSeats %d over %d attestations, want at most %d",
			slot, blockReceivedLine, ffgSeats, attestations, most)
	}
	return line, nil
}

func checkPayloadReceived(node *nodeLog, slot primitives.Slot, block logLine) error {
	lines := node.at(payloadReceivedLine, slot)
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > 1 {
		return fmt.Errorf("slot %d has %d %q lines, want at most 1",
			slot, len(lines), payloadReceivedLine)
	}
	line := lines[0]
	size, err := line.num("payloadBytes")
	if err != nil {
		return err
	}
	if size <= 0 {
		return fmt.Errorf("slot %d: %q has payloadBytes %d, want more than 0",
			slot, payloadReceivedLine, size)
	}
	if block == nil {
		return nil
	}
	payloadArrived, err := line.num("arrivedMs")
	if err != nil {
		return err
	}
	blockArrived, err := block.num("arrivedMs")
	if err != nil {
		return err
	}
	if payloadArrived < blockArrived {
		return fmt.Errorf(
			"slot %d: %q arrivedMs %d is before %q arrivedMs %d",
			slot, payloadReceivedLine, payloadArrived, blockReceivedLine, blockArrived)
	}
	return nil
}

// checkAcrossNodes asserts that a slot whose payload any node saw was also
// carried by a block on all but one node. The one allowed gap is the proposer,
// whose own block never traverses gossip.
func checkAcrossNodes(nodes []*nodeLog, first, last primitives.Slot) error {
	for slot := first; slot <= last; slot++ {
		payloads, blocks := 0, 0
		for _, node := range nodes {
			if len(node.at(payloadReceivedLine, slot)) > 0 {
				payloads++
			}
			if len(node.at(blockReceivedLine, slot)) > 0 {
				blocks++
			}
		}
		if payloads == 0 {
			continue
		}
		if payloads != len(nodes) {
			return fmt.Errorf("slot %d: %d of %d nodes wrote a %q line, want all of them",
				slot, payloads, len(nodes), payloadReceivedLine)
		}
		if blocks < len(nodes)-1 {
			return fmt.Errorf("slot %d: %d of %d nodes wrote a %q line, want at least %d",
				slot, blocks, len(nodes), blockReceivedLine, len(nodes)-1)
		}
	}
	return nil
}

// checkLedger cross-checks the summary counts against the per-vote ledger lines.
// It is a no-op on a run without --goldfish-vote-ledger.
func checkLedger(node *nodeLog, first, last primitives.Slot) error {
	if len(node.ledger) == 0 {
		return nil
	}
	if err := checkGoldfishLedger(node, first, last); err != nil {
		return err
	}
	if err := checkFFGLedger(node, first, last); err != nil {
		return err
	}
	return checkIncludedLedger(node, first, last)
}

func checkGoldfishLedger(node *nodeLog, first, last primitives.Slot) error {
	counted := make(map[primitives.Slot]int64)
	seated := make(map[primitives.Slot]int64)
	for _, entry := range node.ledger[goldfishVoteLedgerLine] {
		switch entry["outcome"] {
		case "accepted", "replayed", "local":
		default:
			continue
		}
		slot, err := entry.num("voteSlot")
		if err != nil {
			return err
		}
		seats, err := entry.num("seats")
		if err != nil {
			return err
		}
		counted[primitives.Slot(slot)]++
		seated[primitives.Slot(slot)] += seats
	}
	for slot := first; slot <= last; slot++ {
		line, err := node.one(goldfishVotesLine, slot)
		if err != nil {
			return err
		}
		votes, err := line.num("votes")
		if err != nil {
			return err
		}
		if votes != counted[slot] {
			return fmt.Errorf("slot %d: %q has votes %d, the ledger holds %d head votes",
				slot, goldfishVotesLine, votes, counted[slot])
		}
		seats, err := line.num("seats")
		if err != nil {
			return err
		}
		if seats != seated[slot] {
			return fmt.Errorf("slot %d: %q has seats %d, the ledger holds %d head vote seats",
				slot, goldfishVotesLine, seats, seated[slot])
		}
	}
	return nil
}

// checkFFGLedger compares the FFG votes line with the ledger's gossip lines for
// the same slot. A vote that arrived within 50 ms of the aggregation deadline
// can land on either side of the tick, so that many lines are tolerated.
func checkFFGLedger(node *nodeLog, first, last primitives.Slot) error {
	cfg := params.BeaconConfig()
	dueMs := cfg.SlotComponentDuration(cfg.AggregateDueBPSGloas).Milliseconds()
	counted := make(map[primitives.Slot]int64)
	borderline := make(map[primitives.Slot]int64)
	for _, entry := range node.ledger[ffgVoteLedgerLine] {
		if entry["outcome"] != "gossip" {
			continue
		}
		slot, err := entry.num("attSlot")
		if err != nil {
			return err
		}
		arrived, err := entry.num("arrivedMs")
		if err != nil {
			return err
		}
		if arrived < dueMs {
			counted[primitives.Slot(slot)]++
		}
		if arrived > dueMs-50 && arrived < dueMs+50 {
			borderline[primitives.Slot(slot)]++
		}
	}
	for slot := first; slot <= last; slot++ {
		line, err := node.one(ffgVotesLine, slot)
		if err != nil {
			return err
		}
		votes, err := line.num("votes")
		if err != nil {
			return err
		}
		if difference(votes, counted[slot]) > borderline[slot] {
			return fmt.Errorf(
				"slot %d: %q has votes %d, the ledger holds %d in-time FFG votes, %d of them borderline",
				slot, ffgVotesLine, votes, counted[slot], borderline[slot])
		}
	}
	return nil
}

func checkIncludedLedger(node *nodeLog, first, last primitives.Slot) error {
	seated := make(map[primitives.Slot]int64)
	for _, entry := range node.ledger[ffgIncludedLedgerLine] {
		slot, err := entry.num("blockSlot")
		if err != nil {
			return err
		}
		seats, err := entry.num("seats")
		if err != nil {
			return err
		}
		seated[primitives.Slot(slot)] += seats
	}
	for slot := first; slot <= last; slot++ {
		lines := node.at(blockReceivedLine, slot)
		if len(lines) == 0 {
			continue
		}
		ffgSeats, err := lines[0].num("ffgSeats")
		if err != nil {
			return err
		}
		if ffgSeats != seated[slot] {
			return fmt.Errorf(
				"slot %d: %q has ffgSeats %d, the ledger holds %d included FFG vote seats",
				slot, blockReceivedLine, ffgSeats, seated[slot])
		}
	}
	return nil
}

func difference(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

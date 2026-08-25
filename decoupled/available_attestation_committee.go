package decoupled

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const (
	domain        = "decoupled_mock_goldfish_committee"
	committeeSize = 512
)

func offset(slot primitives.Slot, validatorCount uint64) uint64 {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(slot)))
	sm := h.Sum(nil)
	return (binary.BigEndian.Uint64(sm[:8]) % validatorCount)
}

func AvailableAttestationSeats(slot primitives.Slot, index primitives.ValidatorIndex, validatorCount uint64) []uint64 {
	off := offset(slot, validatorCount)
	st := (uint64(index) + validatorCount - off) % validatorCount
	var seats []uint64
	for pos := st; pos < committeeSize; pos += validatorCount {
		seats = append(seats, pos)
	}
	return seats
}

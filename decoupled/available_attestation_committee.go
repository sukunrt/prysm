package decoupled

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const (
	domain                            = "decoupled_mock_goldfish_committee"
	AvailableAttestationCommitteeSize = 512
)

var AvailableAttDomain []byte

func init() {
	var ad = sha256.Sum256([]byte(domain))
	AvailableAttDomain = ad[:]
}

func offset(slot primitives.Slot, validatorCount uint64) uint64 {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(slot)))
	sm := h.Sum(nil)
	return (binary.BigEndian.Uint64(sm[:8]) % validatorCount)
}

// CommitteeValidatorCount is the electorate the mock available committee is
// drawn from: the genesis validator set, not the live registry. The publisher
// and every receiver must resolve seats against the same number, and the
// registry grows whenever a deposit is processed.
func CommitteeValidatorCount() uint64 {
	return params.BeaconConfig().MinGenesisActiveValidatorCount
}

func AvailableAttestationSeats(slot primitives.Slot, index primitives.ValidatorIndex, validatorCount uint64) []uint64 {
	if uint64(index) >= validatorCount {
		// A validator that joined after genesis is outside the mock committee.
		// Without this it would wrap onto another validator's seats and its
		// signature would be checked against the wrong public key.
		return nil
	}
	off := offset(slot, validatorCount)
	st := (uint64(index) + validatorCount - off) % validatorCount
	var seats []uint64
	for pos := st; pos < AvailableAttestationCommitteeSize; pos += validatorCount {
		seats = append(seats, pos)
	}
	return seats
}

func AvailableAttestationSeatsToValidatorIndices(slot primitives.Slot, seats []int, validatorCount uint64) []primitives.ValidatorIndex {
	off := offset(slot, validatorCount)
	var validatorIndices []primitives.ValidatorIndex
	for _, s := range seats {
		vi := primitives.ValidatorIndex((uint64(s) + off) % validatorCount)
		validatorIndices = append(validatorIndices, vi)
	}
	slices.Sort(validatorIndices)
	validatorIndices = slices.Compact(validatorIndices)

	return validatorIndices
}

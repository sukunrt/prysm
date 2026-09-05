package decoupled

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
	"slices"

	"github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	hlru "github.com/hashicorp/golang-lru"
)

const (
	domain                            = "decoupled_mock_goldfish_committee"
	AvailableAttestationCommitteeSize = 512
)

var AvailableAttDomain []byte
var committeeCache *hlru.Cache

func init() {
	var ad = sha256.Sum256([]byte(domain))
	AvailableAttDomain = ad[:]
	committeeCache = lru.New(128)
}

func updateCacheAndGetCommittee(slot primitives.Slot, validatorCount uint64) availableAttestationCommittee {
	key := lruKey{Slot: slot, ValidatorCount: validatorCount}
	aac, ok := committeeCache.Get(key)
	if ok {
		return aac.(availableAttestationCommittee)
	}

	sd := seed(slot, validatorCount)
	r := rand.New(rand.NewChaCha8(sd))
	validatorSeats := make([]primitives.ValidatorIndex, AvailableAttestationCommitteeSize)
	for i := range len(validatorSeats) {
		validatorSeats[i] = primitives.ValidatorIndex(r.Uint64N(validatorCount))
	}
	ac := availableAttestationCommittee{
		slot:            slot,
		validatorCount:  validatorCount,
		seed:            sd,
		validatorToSeat: map[primitives.ValidatorIndex][]uint64{},
		seatToValidator: map[uint64]primitives.ValidatorIndex{},
	}
	for s, v := range validatorSeats {
		ac.validatorToSeat[v] = append(ac.validatorToSeat[v], uint64(s))
		ac.seatToValidator[uint64(s)] = v
	}
	committeeCache.Add(key, ac)
	return ac
}

func seed(slot primitives.Slot, validatorCount uint64) [32]byte {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(slot)))
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(validatorCount)))
	var seed [32]byte
	h.Sum(seed[:0])
	return seed
}

// TotalValidatorCount is the total validator count from which the committee is chosen
func TotalValidatorCount() uint64 {
	return params.BeaconConfig().MinGenesisActiveValidatorCount
}

func AvailableAttestationSeats(slot primitives.Slot, index primitives.ValidatorIndex, validatorCount uint64) []uint64 {
	if uint64(index) >= validatorCount || validatorCount == 0 {
		// A validator that joined after genesis is outside the mock committee.
		// Without this it would wrap onto another validator's seats and its
		// signature would be checked against the wrong public key.
		return nil
	}
	ac := updateCacheAndGetCommittee(slot, validatorCount)
	return slices.Clone(ac.validatorToSeat[index])
}

func AvailableAttestationSeatsToValidatorIndices(slot primitives.Slot, seats []int, validatorCount uint64) []primitives.ValidatorIndex {
	if validatorCount == 0 {
		return nil
	}
	ac := updateCacheAndGetCommittee(slot, validatorCount)

	validatorIndices := make([]primitives.ValidatorIndex, 0, len(seats))
	for _, s := range seats {
		if s < 0 || s >= AvailableAttestationCommitteeSize {
			return nil
		}
		ss := uint64(s)
		v, ok := ac.seatToValidator[ss]
		if !ok {
			continue
		}
		validatorIndices = append(validatorIndices, v)
	}
	slices.Sort(validatorIndices)
	return slices.Compact(validatorIndices)
}

type lruKey struct {
	Slot           primitives.Slot
	ValidatorCount uint64
}

type availableAttestationCommittee struct {
	slot            primitives.Slot
	validatorCount  uint64
	seed            [32]byte
	validatorToSeat map[primitives.ValidatorIndex][]uint64
	seatToValidator map[uint64]primitives.ValidatorIndex
}

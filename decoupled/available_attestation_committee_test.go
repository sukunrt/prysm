package decoupled

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAvailableAttestationSeats(t *testing.T) {
	for _, validatorCount := range []uint64{1, 100, 512, 1000, 2048} {
		t.Run(fmt.Sprintf("validatorCount-%d", validatorCount), func(t *testing.T) {
			for _, slot := range []primitives.Slot{0, 1, 100} {
				seatOwners := make(map[uint64]uint64)
				for idx := range validatorCount {
					seats := AvailableAttestationSeats(slot, primitives.ValidatorIndex(idx), validatorCount)
					for _, s := range seats {
						require.Equal(t, true, s < AvailableAttestationCommitteeSize, "seat %d out of range", s)
						owner, taken := seatOwners[s]
						require.Equal(t, false, taken, "seat %d owned by validators %d and %d", s, owner, idx)
						seatOwners[s] = idx
					}
				}
				require.Equal(t, AvailableAttestationCommitteeSize, len(seatOwners), "not every seat is owned at slot %d", slot)
			}
		})
	}
}

func TestAvailableAttestationSeatRoundTrip(t *testing.T) {
	for _, validatorCount := range []uint64{1, 100, 512, 1000, 2048} {
		t.Run(fmt.Sprintf("validatorCount-%d", validatorCount), func(t *testing.T) {
			for _, slot := range []primitives.Slot{0, 1, 100} {
				for v := range validatorCount {
					seats := AvailableAttestationSeats(slot, primitives.ValidatorIndex(v), validatorCount)
					var seatInts []int
					for _, s := range seats {
						seatInts = append(seatInts, int(s))
					}
					vv := AvailableAttestationSeatsToValidatorIndices(slot, seatInts, validatorCount)
					if len(seats) == 0 {
						require.Equal(t, 0, len(vv), "seats empty but validator indices non empty")
						continue
					}
					require.Equal(t, 1, len(vv), "too many validator indices")
					require.Equal(t, v, uint64(vv[0]), "round trip failed")
				}
			}
		})
	}
}

func TestAvailableAttestationSeatMultiple(t *testing.T) {
	for _, validatorCount := range []uint64{1, 100, 512, 1000, 2048} {
		t.Run(fmt.Sprintf("validatorCount-%d", validatorCount), func(t *testing.T) {
			for _, slot := range []primitives.Slot{0, 1, 100} {
				rs := rand.NewPCG(0, 1)
				var seatInts []int
				indices := make(map[primitives.ValidatorIndex]struct{})
				for range 30 {
					vi := primitives.ValidatorIndex(rs.Uint64() % validatorCount)
					seats := AvailableAttestationSeats(slot, vi, validatorCount)
					if len(seats) == 0 {
						continue
					}
					indices[vi] = struct{}{}
					for _, s := range seats {
						seatInts = append(seatInts, int(s))
					}
				}
				validatorIndices := slices.Collect(maps.Keys(indices))
				slices.Sort(validatorIndices)
				vv := AvailableAttestationSeatsToValidatorIndices(slot, seatInts, validatorCount)
				if slices.Compare(validatorIndices, vv) != 0 {
					t.Fatal("mismatch between indices and roundtrip indices")
				}
			}
		})
	}
}

func TestAvailableAttestationSeats_IndexOutsideTheCommittee(t *testing.T) {
	// A validator that joined after genesis is outside the mock committee. It
	// must get no seats rather than wrap onto an existing validator's, or the
	// receiver would check its signature against the wrong public key.
	const validatorCount = 256
	require.Equal(t, 0, len(AvailableAttestationSeats(3, validatorCount, validatorCount)))
	require.Equal(t, 0, len(AvailableAttestationSeats(3, validatorCount+7, validatorCount)))
}

func TestAvailableAttestationSeats_NotInValidatorOrder(t *testing.T) {
	const validatorCount = uint64(1000)
	for _, slot := range []primitives.Slot{0, 1, 100} {
		t.Run(fmt.Sprintf("slot-%d", slot), func(t *testing.T) {
			owners := make([]primitives.ValidatorIndex, AvailableAttestationCommitteeSize)
			for seat := range owners {
				indices := AvailableAttestationSeatsToValidatorIndices(slot, []int{seat}, validatorCount)
				require.Equal(t, 1, len(indices), "seat %d must have one owner", seat)
				require.Equal(t, true, uint64(indices[0]) < validatorCount, "seat %d has an invalid owner", seat)
				owners[seat] = indices[0]
			}
			require.Equal(t, false, slices.IsSorted(owners), "seat owners follow validator index order")

			consecutive := true
			for i := 1; i < len(owners); i++ {
				if uint64(owners[i]) != (uint64(owners[i-1])+1)%validatorCount {
					consecutive = false
					break
				}
			}
			require.Equal(t, false, consecutive, "seat owners form a consecutive run, including wraparound")
		})
	}
}

func TestAvailableAttestationSeats_ZeroValidators(t *testing.T) {
	require.Equal(t, 0, len(AvailableAttestationSeats(0, 0, 0)))
	require.Equal(t, 0, len(AvailableAttestationSeatsToValidatorIndices(0, []int{0}, 0)))
}

func TestAvailableAttestationSeatsToValidatorIndices_InvalidSeats(t *testing.T) {
	for _, seats := range [][]int{{-1}, {AvailableAttestationCommitteeSize}, {0, -1}, {0, AvailableAttestationCommitteeSize}} {
		t.Run(fmt.Sprint(seats), func(t *testing.T) {
			require.Equal(t, 0, len(AvailableAttestationSeatsToValidatorIndices(0, seats, 1000)))
		})
	}
}

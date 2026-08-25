package decoupled

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestAvailableAttestationSeats(t *testing.T) {
	tests := []struct {
		name           string
		validatorCount uint64
	}{
		{name: "validator count below committee size", validatorCount: 100},
		{name: "validator count equal to committee size", validatorCount: 512},
		{name: "validator count above committee size", validatorCount: 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, slot := range []primitives.Slot{0, 1, 100} {
				seatOwners := make(map[uint64]uint64)
				for idx := range tt.validatorCount {
					seats := AvailableAttestationSeats(slot, primitives.ValidatorIndex(idx), tt.validatorCount)
					if tt.validatorCount <= committeeSize {
						require.Equal(t, true, len(seats) > 0, "validator %d has no seats", idx)
					}
					for _, s := range seats {
						require.Equal(t, true, s < committeeSize, "seat %d out of range", s)
						owner, taken := seatOwners[s]
						require.Equal(t, false, taken, "seat %d owned by validators %d and %d", s, owner, idx)
						seatOwners[s] = idx
					}
				}
				require.Equal(t, committeeSize, len(seatOwners), "not every seat is owned at slot %d", slot)
			}
		})
	}
}

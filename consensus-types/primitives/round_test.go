package primitives_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mathprysm "github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRound_Mul(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Round
		panicMsg string
	}{
		{a: 0, b: 1, res: 0},
		{a: 1 << 32, b: 1, res: 1 << 32},
		{a: 1 << 32, b: 100, res: 429496729600},
		{a: 1 << 32, b: 1 << 31, res: 9223372036854775808},
		{a: 1 << 32, b: 1 << 32, res: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: 1 << 63, b: 2, res: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Round(%v).Mul(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Round
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Round(tt.a).Mul(tt.b)
				})
				_, err := primitives.Round(tt.a).SafeMul(tt.b)
				require.ErrorContains(t, tt.panicMsg, err)
			} else {
				res = primitives.Round(tt.a).Mul(tt.b)
			}
			require.Equal(t, tt.res, res)
		})
	}
}

func TestRound_Div(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Round
		panicMsg string
	}{
		{a: 0, b: 1, res: 0},
		{a: 1, b: 0, res: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 1 << 32, b: 1 << 32, res: 1},
		{a: 429496729600, b: 1 << 32, res: 100},
		{a: 33, b: 8, res: 4},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Round(%v).Div(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Round
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Round(tt.a).Div(tt.b)
				})
				_, err := primitives.Round(tt.a).SafeDiv(tt.b)
				require.ErrorContains(t, tt.panicMsg, err)
			} else {
				res = primitives.Round(tt.a).Div(tt.b)
			}
			require.Equal(t, tt.res, res)
		})
	}
}

func TestRound_Add(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Round
		panicMsg string
	}{
		{a: 0, b: 1, res: 1},
		{a: 1 << 32, b: 100, res: 4294967396},
		{a: math.MaxUint64, b: 0, res: math.MaxUint64},
		{a: math.MaxUint64, b: 1, res: 0, panicMsg: mathprysm.ErrAddOverflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Round(%v).Add(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Round
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Round(tt.a).Add(tt.b)
				})
				_, err := primitives.Round(tt.a).SafeAdd(tt.b)
				require.ErrorContains(t, tt.panicMsg, err)
			} else {
				res = primitives.Round(tt.a).Add(tt.b)
			}
			require.Equal(t, tt.res, res)
		})
	}
}

func TestRound_Sub(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Round
		panicMsg string
	}{
		{a: 1, b: 0, res: 1},
		{a: 0, b: 1, res: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
		{a: 1 << 32, b: 100, res: 4294967196},
		{a: math.MaxUint64, b: math.MaxUint64, res: 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Round(%v).Sub(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Round
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Round(tt.a).Sub(tt.b)
				})
				_, err := primitives.Round(tt.a).SafeSub(tt.b)
				require.ErrorContains(t, tt.panicMsg, err)
			} else {
				res = primitives.Round(tt.a).Sub(tt.b)
			}
			require.Equal(t, tt.res, res)
		})
	}
}

func TestRound_Mod(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Round
		panicMsg string
	}{
		{a: 1, b: 0, res: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 0, b: 1, res: 0},
		{a: 1 << 32, b: 17, res: 1},
		{a: 33, b: 8, res: 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Round(%v).Mod(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Round
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Round(tt.a).Mod(tt.b)
				})
				_, err := primitives.Round(tt.a).SafeMod(tt.b)
				require.ErrorContains(t, tt.panicMsg, err)
			} else {
				res = primitives.Round(tt.a).Mod(tt.b)
			}
			require.Equal(t, tt.res, res)
		})
	}
}

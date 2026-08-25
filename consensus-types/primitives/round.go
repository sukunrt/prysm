package primitives

import (
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/math"
)

var _ ssz.HashRoot = (Round)(0)
var _ ssz.Marshaler = (*Round)(nil)
var _ ssz.Unmarshaler = (*Round)(nil)

// Round represents a single Simplex round. A round is a whole number of slots
// and divides an epoch; committees are reshuffled once per round.
//
// Round is deliberately a distinct type rather than an alias of Epoch, so that
// mixing the two does not compile.
type Round uint64

// Mul multiplies round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (r Round) Mul(x uint64) Round {
	res, err := r.SafeMul(x)
	if err != nil {
		panic(err.Error()) // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return res
}

// SafeMul multiplies round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (r Round) SafeMul(x uint64) (Round, error) {
	res, err := math.Mul64(uint64(r), x)
	return Round(res), err
}

// Div divides round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (r Round) Div(x uint64) Round {
	res, err := r.SafeDiv(x)
	if err != nil {
		panic(err.Error()) // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return res
}

// SafeDiv divides round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (r Round) SafeDiv(x uint64) (Round, error) {
	res, err := math.Div64(uint64(r), x)
	return Round(res), err
}

// Add increases round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (r Round) Add(x uint64) Round {
	res, err := r.SafeAdd(x)
	if err != nil {
		panic(err.Error()) // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return res
}

// SafeAdd increases round by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (r Round) SafeAdd(x uint64) (Round, error) {
	res, err := math.Add64(uint64(r), x)
	return Round(res), err
}

// Sub subtracts x from the round.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (r Round) Sub(x uint64) Round {
	res, err := r.SafeSub(x)
	if err != nil {
		panic(err.Error()) // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return res
}

// SafeSub subtracts x from the round.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (r Round) SafeSub(x uint64) (Round, error) {
	res, err := math.Sub64(uint64(r), x)
	return Round(res), err
}

// Mod returns result of `round % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (r Round) Mod(x uint64) Round {
	res, err := r.SafeMod(x)
	if err != nil {
		panic(err.Error()) // lint:nopanic -- Panic is communicated in the godoc commentary.
	}
	return res
}

// SafeMod returns result of `round % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (r Round) SafeMod(x uint64) (Round, error) {
	res, err := math.Mod64(uint64(r), x)
	return Round(res), err
}

// HashTreeRoot --
func (r Round) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(r)
}

// HashTreeRootWith --
func (r Round) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutUint64(uint64(r))
	return nil
}

// UnmarshalSSZ --
func (r *Round) UnmarshalSSZ(buf []byte) error {
	if len(buf) != r.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", r.SizeSSZ(), len(buf))
	}
	*r = Round(UnmarshalUint64(buf))
	return nil
}

// MarshalSSZTo --
func (r *Round) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (r *Round) MarshalSSZ() ([]byte, error) {
	marshalled := MarshalUint64([]byte{}, uint64(*r))
	return marshalled, nil
}

// SizeSSZ --
func (r *Round) SizeSSZ() int {
	return 8
}

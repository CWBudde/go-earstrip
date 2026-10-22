// Package verify holds the invariant checks of the tests, shared by the package tests, the fuzz targets and the
// oracle tests (which run them on the C++ results to show that the checks themselves are right).
package verify

import (
	"math/big"
	"math/bits"
)

// Int128 is a signed 128-bit integer in two's complement, for the exact areas of test/test.cpp's __int128.
type Int128 struct {
	Hi int64
	Lo uint64
}

// I128 widens v.
func I128(v int64) Int128 { return Int128{Hi: v >> 63, Lo: uint64(v)} }

// Mul64 returns a*b exactly.
func Mul64(a, b int64) Int128 {
	neg := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(abs64(a), abs64(b))
	r := Int128{Hi: int64(hi), Lo: lo}
	if neg {
		r = r.Neg()
	}
	return r
}

func abs64(v int64) uint64 {
	if v < 0 {
		return uint64(-v) // also right for math.MinInt64
	}
	return uint64(v)
}

// Add returns a+b (wrapping like __int128).
func (a Int128) Add(b Int128) Int128 {
	lo, carry := bits.Add64(a.Lo, b.Lo, 0)
	return Int128{Hi: a.Hi + b.Hi + int64(carry), Lo: lo}
}

// Sub returns a-b.
func (a Int128) Sub(b Int128) Int128 { return a.Add(b.Neg()) }

// Neg returns -a.
func (a Int128) Neg() Int128 {
	lo, borrow := bits.Sub64(0, a.Lo, 0)
	return Int128{Hi: -a.Hi - int64(borrow), Lo: lo}
}

// Sign returns -1, 0 or 1.
func (a Int128) Sign() int {
	switch {
	case a.Hi < 0:
		return -1
	case a.Hi == 0 && a.Lo == 0:
		return 0
	}
	return 1
}

// Abs returns |a|.
func (a Int128) Abs() Int128 {
	if a.Sign() < 0 {
		return a.Neg()
	}
	return a
}

// Big returns a as a big.Int.
func (a Int128) Big() *big.Int {
	v := new(big.Int).SetInt64(a.Hi)
	v.Lsh(v, 64)
	return v.Add(v, new(big.Int).SetUint64(a.Lo))
}

func (a Int128) String() string { return a.Big().String() }

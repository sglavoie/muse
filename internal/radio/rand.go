package radio

import (
	cryptorand "crypto/rand"
	"errors"
	"math"
	"math/big"
)

// randPrecision is the denominator of a crypto-backed draw. 2^53 is the
// largest power of two every float64 can represent exactly, so the quotient
// covers [0, 1) without rounding two adjacent integers onto the same float.
const randPrecision = 1 << 53

// errNoRand is returned by FailingRand so callers can exercise the
// deterministic fallback without wiring a fake entropy reader.
var errNoRand = errors.New("radio: no randomness available")

// RandSource supplies uniform floats in [0, 1) for weighted batch selection.
// Injecting it is what makes a planned batch reproducible in tests: production
// uses CryptoRand, tests use FixedRand or SequenceRand.
//
// A source that fails is not an error condition for the planner. Selection
// falls back to taking the highest-scored eligible candidate, which is the
// same behavior radio has always had when the entropy pool is unavailable.
type RandSource interface {
	Float64() (float64, error)
}

// RandFunc adapts a function to RandSource.
type RandFunc func() (float64, error)

// Float64 implements RandSource.
func (f RandFunc) Float64() (float64, error) { return f() }

// CryptoRand returns the production RandSource, backed by crypto/rand.
func CryptoRand() RandSource { return cryptoRand{} }

type cryptoRand struct{}

// Float64 implements RandSource.
func (cryptoRand) Float64() (float64, error) {
	n, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetUint64(randPrecision))
	if err != nil {
		return 0, err
	}
	return float64(n.Uint64()) / float64(randPrecision), nil
}

// FixedRand returns a RandSource that always yields v. Use it to pin which
// slice of the cumulative weight distribution a test lands in.
func FixedRand(v float64) RandSource {
	return RandFunc(func() (float64, error) { return v, nil })
}

// SequenceRand returns a RandSource that yields values in order and then
// repeats the final value, so a test never runs out of draws mid-batch. With
// no values it degrades to always yielding 0.
func SequenceRand(values ...float64) RandSource {
	i := 0
	return RandFunc(func() (float64, error) {
		if len(values) == 0 {
			return 0, nil
		}
		v := values[i]
		if i < len(values)-1 {
			i++
		}
		return v, nil
	})
}

// FailingRand returns a RandSource that always fails, exercising the
// deterministic highest-score fallback.
func FailingRand() RandSource {
	return RandFunc(func() (float64, error) { return 0, errNoRand })
}

// unitInterval folds an arbitrary float onto [0, 1). A source that returns
// NaN, an infinity, or an out-of-range value would otherwise make every
// cumulative-weight comparison degenerate, silently collapsing the weighted
// draw onto one candidate.
func unitInterval(v float64) (float64, bool) {
	if math.IsNaN(v) {
		return 0, false
	}
	if v < 0 || math.IsInf(v, -1) {
		return 0, true
	}
	if v >= 1 || math.IsInf(v, 1) {
		return math.Nextafter(1, 0), true
	}
	return v, true
}

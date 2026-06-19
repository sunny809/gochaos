// Package delayx provides delay distribution implementations for the gmock server.
//
// Each distribution converts user-specified parameters (percentiles, ranges, etc.)
// into a time.Duration sample suitable for simulating network latency. All
// distributions accept a randx.RNG so that output is reproducible when a seed
// is configured.
package delayx

import (
	"fmt"
	"math"
	"time"

	"github.com/sunny809/gochaos/internal/randx"
)

// PhiInverse returns the inverse of the standard normal CDF at probability p.
// That is, PhiInverse(p) = x such that P(Z <= x) = p for Z ~ N(0,1).
//
// The implementation uses the relationship between the normal CDF and the
// error function: Phi(x) = (1 + erf(x/sqrt2)) / 2, so
// PhiInverse(p) = sqrt2 * erfinv(2p - 1).
//
// Panics if p is not in (0, 1).
func PhiInverse(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

// LognormalFromPercentiles computes the mu and sigma parameters of a lognormal
// distribution from the given percentile values (in milliseconds).
//
// The lognormal distribution models network latency with a long tail:
// X = exp(mu + sigma * Z), where Z ~ N(0,1).
//
// At least p50 and one higher percentile (p95 or p99) must be non-zero.
// When both p95 and p99 are provided, p99 is used (it captures the tail more
// accurately and produces a better fit for the long-tail distribution).
//
// Returns an error if:
//   - p50 is zero or negative
//   - neither p95 nor p99 is provided
//   - any higher percentile is less than or equal to p50
//   - any value is negative
func LognormalFromPercentiles(p50, p95, p99 int) (mu, sigma float64, err error) {
	if p50 <= 0 {
		return 0, 0, fmt.Errorf("lognormal: p50 must be positive, got %d", p50)
	}
	if p95 < 0 {
		return 0, 0, fmt.Errorf("lognormal: p95 must be non-negative, got %d", p95)
	}
	if p99 < 0 {
		return 0, 0, fmt.Errorf("lognormal: p99 must be non-negative, got %d", p99)
	}

	// At least one higher percentile is required to determine sigma.
	higher := p99
	if higher == 0 {
		higher = p95
	}
	if higher == 0 {
		return 0, 0, fmt.Errorf("lognormal: at least one higher percentile (p95 or p99) is required")
	}
	if higher <= p50 {
		return 0, 0, fmt.Errorf("lognormal: higher percentile (%d) must be greater than p50 (%d)", higher, p50)
	}

	// mu = ln(p50) because PhiInverse(0.50) = 0
	mu = math.Log(float64(p50))

	// Determine which probability to use for sigma calculation.
	prob := 0.99
	if p99 == 0 {
		prob = 0.95
	}

	// sigma = (ln(higher) - mu) / PhiInverse(prob)
	sigma = (math.Log(float64(higher)) - mu) / PhiInverse(prob)

	if sigma <= 0 {
		return 0, 0, fmt.Errorf("lognormal: computed sigma is non-positive (%.6f), check percentile values", sigma)
	}

	return mu, sigma, nil
}

// Sample generates a single sample from the lognormal distribution with the
// given mu and sigma parameters using the provided RNG.
//
// The result is clamped to >= 0 as a defensive measure, although the
// lognormal distribution is always non-negative by definition.
// Returns the sampled delay as a time.Duration in milliseconds.
func Sample(mu, sigma float64, rng randx.RNG) time.Duration {
	z := rng.NormFloat64()
	x := math.Exp(mu + sigma*z)
	if x < 0 {
		x = 0
	}
	return time.Duration(x) * time.Millisecond
}

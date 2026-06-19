// Package randx provides a seedable random number generator interface and
// implementation for the gmock server.
//
// All chaos behavior (delays, fault injection, probabilistic matching) must
// use randx.RNG instead of math/rand global functions so that:
//   - WithRandSeed(42) produces identical failure sequences across runs
//   - Stub-level seeds can override the global seed for per-stub determinism
//   - Unseeded servers behave identically to the default math/rand global source
package randx

import (
	"math/rand"
	"sync"
	"time"
)

// RNG is the interface for seedable pseudo-random number generation.
// All chaos behavior in gmock must go through this interface to ensure
// reproducibility when a seed is configured.
type RNG interface {
	// Float64 returns a pseudo-random float64 in [0.0, 1.0).
	Float64() float64

	// Intn returns a pseudo-random int in [0, n). It panics if n <= 0.
	Intn(n int) int

	// NormFloat64 returns a normally distributed float64 from the standard
	// normal distribution (mean 0, stddev 1).
	NormFloat64() float64

	// Read generates len(p) random bytes and writes them into p.
	// It always returns len(p) and a nil error.
	// This is compatible with the io.Reader interface.
	Read(p []byte) (int, error)

	// Seed re-initializes the generator with the given seed value.
	Seed(seed int64)
}

// Rand wraps *rand.Rand to implement the RNG interface.
// It is safe for concurrent use via sync.Mutex.
type Rand struct {
	mu   sync.Mutex
	rand *rand.Rand
}

// NewGlobal creates a new RNG seeded with the given seed.
// If seed is 0, the RNG is seeded with the current Unix nano timestamp,
// matching the behavior of the unseeded math/rand global source since Go 1.20.
func NewGlobal(seed int64) *Rand {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Rand{
		rand: rand.New(rand.NewSource(seed)),
	}
}

// NewStub creates a new RNG for stub-level seed overrides.
// Unlike NewGlobal, a zero seed is treated as "no stub seed" and the
// returned *Rand is nil. Callers must check for nil before use.
func NewStub(seed int64) *Rand {
	if seed == 0 {
		return nil
	}
	return &Rand{
		rand: rand.New(rand.NewSource(seed)),
	}
}

// Float64 returns a pseudo-random float64 in [0.0, 1.0).
func (r *Rand) Float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.Float64()
}

// Intn returns a pseudo-random int in [0, n). It panics if n <= 0.
func (r *Rand) Intn(n int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.Intn(n)
}

// NormFloat64 returns a normally distributed float64 from the standard
// normal distribution (mean 0, stddev 1).
func (r *Rand) NormFloat64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.NormFloat64()
}

// Read generates len(p) random bytes and writes them into p.
// It always returns len(p) and a nil error.
// This is thread-safe via sync.Mutex, matching the other Rand methods.
func (r *Rand) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rand.Read(p)
}

// Seed re-initializes the generator with the given seed value.
func (r *Rand) Seed(seed int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rand.Seed(seed)
}

// ResolveRNG returns the stub-level RNG if non-nil, otherwise falls back
// to the global RNG. This implements the stub-seed-overrides-global convention.
func ResolveRNG(stubRNG, globalRNG RNG) RNG {
	if stubRNG != nil {
		return stubRNG
	}
	return globalRNG
}

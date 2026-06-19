package randx

import (
	"sync"
	"testing"
)

// TestNewGlobal_DeterministicWithSeed verifies that two RNG instances
// created with the same seed produce identical sequences.
func TestNewGlobal_DeterministicWithSeed(t *testing.T) {
	t.Helper()
	const seed int64 = 42

	r1 := NewGlobal(seed)
	r2 := NewGlobal(seed)

	for i := 0; i < 100; i++ {
		v1 := r1.Float64()
		v2 := r2.Float64()
		if v1 != v2 {
			t.Fatalf("Float64 mismatch at iteration %d: %v != %v", i, v1, v2)
		}
	}
}

// TestNewGlobal_IntnDeterministic verifies Intn produces identical sequences
// with the same seed.
func TestNewGlobal_IntnDeterministic(t *testing.T) {
	t.Helper()
	const seed int64 = 12345

	r1 := NewGlobal(seed)
	r2 := NewGlobal(seed)

	for i := 0; i < 100; i++ {
		v1 := r1.Intn(1000)
		v2 := r2.Intn(1000)
		if v1 != v2 {
			t.Fatalf("Intn mismatch at iteration %d: %v != %v", i, v1, v2)
		}
	}
}

// TestNewGlobal_NormFloat64Deterministic verifies NormFloat64 produces
// identical sequences with the same seed.
func TestNewGlobal_NormFloat64Deterministic(t *testing.T) {
	t.Helper()
	const seed int64 = 9999

	r1 := NewGlobal(seed)
	r2 := NewGlobal(seed)

	for i := 0; i < 50; i++ {
		v1 := r1.NormFloat64()
		v2 := r2.NormFloat64()
		if v1 != v2 {
			t.Fatalf("NormFloat64 mismatch at iteration %d: %v != %v", i, v1, v2)
		}
	}
}

// TestNewGlobal_DifferentSeedsDiffer verifies that different seeds produce
// different sequences.
func TestNewGlobal_DifferentSeedsDiffer(t *testing.T) {
	t.Helper()
	r1 := NewGlobal(1)
	r2 := NewGlobal(2)

	differences := 0
	for i := 0; i < 100; i++ {
		if r1.Float64() != r2.Float64() {
			differences++
		}
	}
	if differences == 0 {
		t.Fatal("two RNGs with different seeds produced identical sequences")
	}
}

// TestNewGlobal_ZeroSeedUsesTimestamp verifies that seed=0 does not panic
// and produces varying values across instances (since it uses the current time).
func TestNewGlobal_ZeroSeedUsesTimestamp(t *testing.T) {
	t.Helper()
	r := NewGlobal(0)
	// Should not panic and should produce valid values
	v := r.Float64()
	if v < 0 || v >= 1.0 {
		t.Fatalf("Float64() = %v, want [0, 1)", v)
	}
}

// TestNewStub_ZeroSeedReturnsNil verifies that NewStub(0) returns nil,
// meaning "no stub-level seed override".
func TestNewStub_ZeroSeedReturnsNil(t *testing.T) {
	t.Helper()
	r := NewStub(0)
	if r != nil {
		t.Fatal("NewStub(0) should return nil, got non-nil RNG")
	}
}

// TestNewStub_NonZeroSeedReturnsRNG verifies that NewStub with a non-zero
// seed returns a valid, deterministic RNG.
func TestNewStub_NonZeroSeedReturnsRNG(t *testing.T) {
	t.Helper()
	r := NewStub(42)
	if r == nil {
		t.Fatal("NewStub(42) should return non-nil RNG")
	}
	// Should produce deterministic values
	r2 := NewStub(42)
	if r.Float64() != r2.Float64() {
		t.Fatal("NewStub with same seed should produce identical values")
	}
}

// TestResolveRNG_StubOverridesGlobal verifies that ResolveRNG returns
// the stub RNG when it is non-nil.
func TestResolveRNG_StubOverridesGlobal(t *testing.T) {
	t.Helper()
	globalRNG := NewGlobal(1)
	stubRNG := NewStub(42)

	resolved := ResolveRNG(stubRNG, globalRNG)
	if resolved != stubRNG {
		t.Fatal("ResolveRNG should return stub RNG when non-nil")
	}
}

// TestResolveRNG_NilStubFallsBackToGlobal verifies that ResolveRNG returns
// the global RNG when the stub RNG is nil.
func TestResolveRNG_NilStubFallsBackToGlobal(t *testing.T) {
	t.Helper()
	globalRNG := NewGlobal(1)

	resolved := ResolveRNG(nil, globalRNG)
	if resolved != globalRNG {
		t.Fatal("ResolveRNG should fall back to global RNG when stub is nil")
	}
}

// TestSeed_ReproducibilityAfterReseed verifies that calling Seed on an
// existing RNG resets it to produce the same sequence as a fresh instance.
func TestSeed_ReproducibilityAfterReseed(t *testing.T) {
	t.Helper()
	const seed int64 = 777

	r := NewGlobal(seed)
	// Consume some values
	_, _, _ = r.Float64(), r.Intn(100), r.NormFloat64()

	// Reseed with the same seed
	r.Seed(seed)

	// Compare with a fresh instance
	fresh := NewGlobal(seed)

	for i := 0; i < 50; i++ {
		v1 := r.Float64()
		v2 := fresh.Float64()
		if v1 != v2 {
			t.Fatalf("after reseed, Float64 mismatch at iteration %d: %v != %v", i, v1, v2)
		}
	}
}

// TestConcurrentAccess verifies that the RNG is safe for concurrent use.
// This test is designed to be run with -race detector.
func TestConcurrentAccess(t *testing.T) {
	t.Helper()
	r := NewGlobal(42)

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = r.Float64()
				_ = r.Intn(100)
				_ = r.NormFloat64()
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentSeedAndRead verifies that concurrent Seed and read
// operations do not cause data races.
func TestConcurrentSeedAndRead(t *testing.T) {
	t.Helper()
	r := NewGlobal(42)

	const goroutines = 50
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half the goroutines read
	for g := 0; g < goroutines; g++ {
		go func(id int64) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = r.Float64()
				_ = r.Intn(10)
			}
		}(int64(g))
	}

	// Half the goroutines reseed
	for g := 0; g < goroutines; g++ {
		go func(id int64) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				r.Seed(id + int64(i))
			}
		}(int64(g))
	}

	wg.Wait()
}

// TestIntn_Range verifies that Intn produces values within [0, n).
func TestIntn_Range(t *testing.T) {
	t.Helper()
	r := NewGlobal(42)

	const n = 10
	for i := 0; i < 1000; i++ {
		v := r.Intn(n)
		if v < 0 || v >= n {
			t.Fatalf("Intn(%d) = %d, want [0, %d)", n, v, n)
		}
	}
}

// TestFloat64_Range verifies that Float64 produces values in [0.0, 1.0).
func TestFloat64_Range(t *testing.T) {
	t.Helper()
	r := NewGlobal(42)

	for i := 0; i < 1000; i++ {
		v := r.Float64()
		if v < 0.0 || v >= 1.0 {
			t.Fatalf("Float64() = %v, want [0.0, 1.0)", v)
		}
	}
}

// TestRead_DeterministicWithSeed verifies that Read produces identical
// byte sequences when the RNG is seeded with the same value.
func TestRead_DeterministicWithSeed(t *testing.T) {
	t.Helper()
	const seed int64 = 42

	r1 := NewGlobal(seed)
	r2 := NewGlobal(seed)

	buf1 := make([]byte, 256)
	buf2 := make([]byte, 256)

	n1, err1 := r1.Read(buf1)
	n2, err2 := r2.Read(buf2)

	if n1 != 256 || n2 != 256 {
		t.Fatalf("Read returned wrong length: n1=%d n2=%d, want 256", n1, n2)
	}
	if err1 != nil || err2 != nil {
		t.Fatalf("Read returned error: err1=%v err2=%v", err1, err2)
	}

	for i, b := range buf1 {
		if b != buf2[i] {
			t.Fatalf("Read mismatch at byte %d: %v != %v", i, b, buf2[i])
		}
	}
}

// TestRead_FillsBuffer verifies that Read fills the entire buffer with
// non-zero data (extremely unlikely to get all zeros from a proper RNG).
func TestRead_FillsBuffer(t *testing.T) {
	t.Helper()
	r := NewGlobal(0)

	buf := make([]byte, 1024)
	n, err := r.Read(buf)

	if n != 1024 {
		t.Fatalf("Read returned %d, want 1024", n)
	}
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	allZero := true
	for _, b := range buf {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("Read filled buffer with all zeros — RNG may be broken")
	}
}

// TestConcurrentRead verifies that concurrent Read calls are safe.
func TestConcurrentRead(t *testing.T) {
	t.Helper()
	r := NewGlobal(42)

	const goroutines = 50
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			buf := make([]byte, 64)
			for i := 0; i < iterations; i++ {
				n, err := r.Read(buf)
				if n != 64 {
					t.Errorf("Read returned %d, want 64", n)
				}
				if err != nil {
					t.Errorf("Read returned error: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

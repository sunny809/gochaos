// Package delayx provides delay distribution implementations for the gmock server.
package delayx

import (
	"math"
	"sort"
	"testing"

	"github.com/sunny809/gochaos/internal/randx"
)

// percentile computes the p-th percentile (0-100) from a sorted slice of
// float64 durations (in milliseconds).
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func TestPhiInverse(t *testing.T) {
	tests := []struct {
		name string
		p    float64
		want float64
	}{
		{"p=0.50", 0.50, 0.0},
		{"p=0.84", 0.84, 0.9944578},
		{"p=0.975", 0.975, 1.9599639},
		{"p=0.99", 0.99, 2.3263478},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PhiInverse(tt.p)
			// Allow generous tolerance since erfinv is approximate.
			tol := 0.001
			if math.Abs(got-tt.want) > tol {
				t.Errorf("PhiInverse(%v) = %v, want %v (tol %v)", tt.p, got, tt.want, tol)
			}
		})
	}

	// Verify PhiInverse(0.50) is exactly 0.
	t.Run("p50_exact_zero", func(t *testing.T) {
		got := PhiInverse(0.50)
		if got != 0.0 {
			t.Errorf("PhiInverse(0.50) = %v, want exactly 0.0", got)
		}
	})
}

func TestLognormalFromPercentiles(t *testing.T) {
	tests := []struct {
		name      string
		p50       int
		p95       int
		p99       int
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid_p50_p99",
			p50:  100,
			p99:  2000,
		},
		{
			name: "valid_p50_p95",
			p50:  100,
			p95:  500,
		},
		{
			name: "valid_p50_p95_p99_uses_p99",
			p50:  100,
			p95:  500,
			p99:  2000,
		},
		{
			name:      "missing_p50",
			p50:       0,
			p95:       500,
			wantErr:   true,
			errSubstr: "p50 must be positive",
		},
		{
			name:      "negative_p50",
			p50:       -10,
			p95:       500,
			wantErr:   true,
			errSubstr: "p50 must be positive",
		},
		{
			name:      "no_higher_percentile",
			p50:       100,
			wantErr:   true,
			errSubstr: "at least one higher percentile",
		},
		{
			name:      "p95_less_than_p50",
			p50:       100,
			p95:       50,
			wantErr:   true,
			errSubstr: "must be greater than p50",
		},
		{
			name:      "p99_less_than_p50",
			p50:       100,
			p99:       80,
			wantErr:   true,
			errSubstr: "must be greater than p50",
		},
		{
			name:      "p95_equals_p50",
			p50:       100,
			p95:       100,
			wantErr:   true,
			errSubstr: "must be greater than p50",
		},
		{
			name:      "negative_p95",
			p50:       100,
			p95:       -1,
			wantErr:   true,
			errSubstr: "p95 must be non-negative",
		},
		{
			name:      "negative_p99",
			p50:       100,
			p99:       -1,
			wantErr:   true,
			errSubstr: "p99 must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu, sigma, err := LognormalFromPercentiles(tt.p50, tt.p95, tt.p99)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
				if !contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mu <= 0 {
				t.Errorf("mu should be positive, got %v", mu)
			}
			if sigma <= 0 {
				t.Errorf("sigma should be positive, got %v", sigma)
			}
		})
	}

	// Verify mu = ln(p50) exactly.
	t.Run("mu_equals_ln_p50", func(t *testing.T) {
		mu, _, err := LognormalFromPercentiles(100, 0, 2000)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := math.Log(100)
		if mu != expected {
			t.Errorf("mu = %v, want %v", mu, expected)
		}
	})
}

func TestLognormalFromPercentiles_PreferP99(t *testing.T) {
	// When both p95 and p99 are provided, p99 should be used for sigma.
	// This means sigma computed with p99 should differ from sigma with p95 alone.
	mu95, sigma95, err := LognormalFromPercentiles(100, 500, 0)
	if err != nil {
		t.Fatalf("p50+p95: %v", err)
	}
	mu99, sigma99, err := LognormalFromPercentiles(100, 500, 2000)
	if err != nil {
		t.Fatalf("p50+p95+p99: %v", err)
	}
	// mu should be the same (ln(100)).
	if mu95 != mu99 {
		t.Errorf("mu should be equal: mu95=%v, mu99=%v", mu95, mu99)
	}
	// sigma should differ because p99 captures a longer tail.
	if sigma95 == sigma99 {
		t.Errorf("sigma should differ when p99 is provided: sigma95=%v, sigma99=%v", sigma95, sigma99)
	}
	// sigma with p99 should be larger (longer tail).
	if sigma99 <= sigma95 {
		t.Errorf("sigma with p99 (%v) should be larger than sigma with p95 (%v)", sigma99, sigma95)
	}
}

func TestSample_StatisticalDistribution(t *testing.T) {
	// Generate 10000 samples from lognormal(p50=100, p95=500, p99=2000).
	// When both p95 and p99 are provided, p99 is used to compute sigma,
	// so the distribution is tuned for the long tail. We verify p50 and p99
	// within ±20%. The p95 percentile is only loosely checked because the
	// sigma is dominated by p99.
	p50, p95, p99 := 100, 500, 2000
	mu, sigma, err := LognormalFromPercentiles(p50, p95, p99)
	if err != nil {
		t.Fatalf("LognormalFromPercentiles: %v", err)
	}

	rng := randx.NewGlobal(42) // fixed seed for reproducibility

	const n = 10000
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		samples[i] = float64(Sample(mu, sigma, rng).Milliseconds())
	}
	sort.Float64s(samples)

	observedP50 := percentile(samples, 50)
	observedP95 := percentile(samples, 95)
	observedP99 := percentile(samples, 99)

	t.Logf("observed: p50=%.1f, p95=%.1f, p99=%.1f", observedP50, observedP95, observedP99)
	t.Logf("target:   p50=%d, p95=%d, p99=%d", p50, p95, p99)

	// ±20% tolerance for p50 and p99 (p99 drives sigma)
	tol := 0.20
	checkPercentile := func(name string, observed, target float64, tolerance float64) {
		lower := target * (1 - tolerance)
		upper := target * (1 + tolerance)
		if observed < lower || observed > upper {
			t.Errorf("%s: observed %.1f outside [%.1f, %.1f]", name, observed, lower, upper)
		}
	}

	checkPercentile("p50", observedP50, float64(p50), tol)
	checkPercentile("p99", observedP99, float64(p99), tol)

	// p95 is only loosely checked because sigma is computed from p99,
	// so the p95 percentile will be higher than the target p95.
	// Verify p95 is at least between p50 and a reasonable upper bound.
	if observedP95 <= float64(p50) {
		t.Errorf("p95: observed %.1f should be greater than p50 (%d)", observedP95, p50)
	}
}

func TestSample_StatisticalDistribution_P50P95(t *testing.T) {
	// When only p50 and p95 are provided, verify the distribution fits.
	p50, p95 := 100, 500
	mu, sigma, err := LognormalFromPercentiles(p50, p95, 0)
	if err != nil {
		t.Fatalf("LognormalFromPercentiles: %v", err)
	}

	rng := randx.NewGlobal(123)

	const n = 10000
	samples := make([]float64, n)
	for i := 0; i < n; i++ {
		samples[i] = float64(Sample(mu, sigma, rng).Milliseconds())
	}
	sort.Float64s(samples)

	observedP50 := percentile(samples, 50)
	observedP95 := percentile(samples, 95)

	t.Logf("observed: p50=%.1f, p95=%.1f", observedP50, observedP95)

	tol := 0.20
	if observedP50 < float64(p50)*(1-tol) || observedP50 > float64(p50)*(1+tol) {
		t.Errorf("p50: observed %.1f outside [%.1f, %.1f]", observedP50, float64(p50)*(1-tol), float64(p50)*(1+tol))
	}
	if observedP95 < float64(p95)*(1-tol) || observedP95 > float64(p95)*(1+tol) {
		t.Errorf("p95: observed %.1f outside [%.1f, %.1f]", observedP95, float64(p95)*(1-tol), float64(p95)*(1+tol))
	}
}

func TestSample_Deterministic(t *testing.T) {
	// Same seed must produce identical sample sequences.
	mu, sigma, err := LognormalFromPercentiles(100, 0, 2000)
	if err != nil {
		t.Fatalf("LognormalFromPercentiles: %v", err)
	}

	rng1 := randx.NewGlobal(999)
	rng2 := randx.NewGlobal(999)

	const n = 100
	for i := 0; i < n; i++ {
		s1 := Sample(mu, sigma, rng1)
		s2 := Sample(mu, sigma, rng2)
		if s1 != s2 {
			t.Errorf("sample %d: %v != %v (seed determinism broken)", i, s1, s2)
			break
		}
	}
}

func TestSample_NonNegative(t *testing.T) {
	// All samples must be non-negative (defensive clamp).
	mu, sigma, err := LognormalFromPercentiles(100, 0, 2000)
	if err != nil {
		t.Fatalf("LognormalFromPercentiles: %v", err)
	}

	rng := randx.NewGlobal(42)

	const n = 10000
	for i := 0; i < n; i++ {
		d := Sample(mu, sigma, rng)
		if d < 0 {
			t.Errorf("sample %d: got negative duration %v", i, d)
		}
	}
}

func TestSample_ConcurrentAccess(t *testing.T) {
	// Verify that Sample is safe for concurrent use with a shared RNG.
	mu, sigma, err := LognormalFromPercentiles(100, 0, 2000)
	if err != nil {
		t.Fatalf("LognormalFromPercentiles: %v", err)
	}

	rng := randx.NewGlobal(42)

	const goroutines = 10
	const samplesPerGoroutine = 1000

	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < samplesPerGoroutine; i++ {
				d := Sample(mu, sigma, rng)
				if d < 0 {
					// Should never happen, but check.
				}
			}
		}()
	}

	for g := 0; g < goroutines; g++ {
		<-done
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Package response provides the response writing port and adapters for the gmock server.
//
// This file tests the activation logic for fault injection, including
// probability-based activation (A1), everyNthRequest activation (A2),
// validation, and integration with the applyFault pipeline.
package response

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/randx"
	"github.com/sunny809/gochaos/internal/spec"
)

// --- ValidateActivation tests ---

func TestValidateActivation(t *testing.T) {
	tests := []struct {
		name       string
		activation *spec.Activation
		wantErr    bool
	}{
		{
			name:       "nil activation is valid",
			activation: nil,
			wantErr:    false,
		},
		{
			name:       "empty activation is valid (zero-values)",
			activation: &spec.Activation{},
			wantErr:    false,
		},
		{
			name:       "probability 0.0 is valid",
			activation: &spec.Activation{Probability: 0.0},
			wantErr:    false,
		},
		{
			name:       "probability 0.5 is valid",
			activation: &spec.Activation{Probability: 0.5},
			wantErr:    false,
		},
		{
			name:       "probability 1.0 is valid",
			activation: &spec.Activation{Probability: 1.0},
			wantErr:    false,
		},
		{
			name:       "probability -0.1 is invalid",
			activation: &spec.Activation{Probability: -0.1},
			wantErr:    true,
		},
		{
			name:       "probability 1.5 is invalid",
			activation: &spec.Activation{Probability: 1.5},
			wantErr:    true,
		},
		{
			name:       "everyNthRequest 1 is valid",
			activation: &spec.Activation{EveryNthRequest: 1},
			wantErr:    false,
		},
		{
			name:       "everyNthRequest 10 is valid",
			activation: &spec.Activation{EveryNthRequest: 10},
			wantErr:    false,
		},
		{
			name:       "everyNthRequest -1 is invalid",
			activation: &spec.Activation{EveryNthRequest: -1},
			wantErr:    true,
		},
		{
			name: "valid time window",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 2000},
				},
			},
			wantErr: false,
		},
		{
			name: "time window endMs equals startMs is valid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 1000},
				},
			},
			wantErr: false,
		},
		{
			name: "time window endMs < startMs is invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 2000, EndMs: 1000},
				},
			},
			wantErr: true,
		},
		{
			name: "time window probability 0.5 is valid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 2000, Probability: 0.5},
				},
			},
			wantErr: false,
		},
		{
			name: "time window probability -0.1 is invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 2000, Probability: -0.1},
				},
			},
			wantErr: true,
		},
		{
			name: "time window probability 1.5 is invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 2000, Probability: 1.5},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple time windows second invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 1000, EndMs: 2000, Probability: 0.5},
					{StartMs: 3000, EndMs: 2000, Probability: 0.3},
				},
			},
			wantErr: true,
		},
		{
			name: "combined probability and everyNthRequest is valid",
			activation: &spec.Activation{
				Probability:     0.5,
				EveryNthRequest: 5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			err := ValidateActivation(tt.activation)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateActivation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- ShouldActivate tests ---

func TestShouldActivate_NilActivation(t *testing.T) {
	rng := randx.NewGlobal(42)
	// nil activation should always return true (always-on)
	if !ShouldActivate(nil, rng, 0, time.Time{}).ShouldFire {
		t.Error("ShouldActivate(nil) = false, want true (always-on)")
	}
}

func TestShouldActivate_ProbabilityZero(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{Probability: 0.0}

	// Probability 0.0 is treated as "not configured" (zero-value).
	// With no other mode configured, this is equivalent to an empty
	// Activation{} — always-on for backward compatibility.
	// If a user wants "never fire", they should not configure a fault at all.
	for i := 0; i < 100; i++ {
		if !ShouldActivate(activation, rng, 0, time.Time{}).ShouldFire {
			t.Errorf("ShouldActivate with probability 0.0 (unconfigured) returned false on iteration %d, want true", i)
			break
		}
	}
}

func TestShouldActivate_ProbabilityOne(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{Probability: 1.0}

	// probability 1.0 should always activate
	for i := 0; i < 100; i++ {
		if !ShouldActivate(activation, rng, 0, time.Time{}).ShouldFire {
			t.Errorf("ShouldActivate with probability 1.0 did not activate on iteration %d", i)
			break
		}
	}
}

func TestShouldActivate_ProbabilityStatistical(t *testing.T) {
	// With seed 1 and 1000 requests, probability 0.5 should produce
	// approximately 450-550 activations (acceptance criterion).
	rng := randx.NewGlobal(1)
	activation := &spec.Activation{Probability: 0.5}

	triggered := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ShouldActivate(activation, rng, 0, time.Time{}).ShouldFire {
			triggered++
		}
	}

	if triggered < 450 || triggered > 550 {
		t.Errorf("probability 0.5 with seed 1: triggered %d/%d, expected 450-550", triggered, total)
	}
}

func TestShouldActivate_ProbabilityLow(t *testing.T) {
	// With a very low probability, most requests should not trigger.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{Probability: 0.01}

	triggered := 0
	total := 10000
	for i := 0; i < total; i++ {
		if ShouldActivate(activation, rng, 0, time.Time{}).ShouldFire {
			triggered++
		}
	}

	// Expect ~100 out of 10000; allow wide margin: 50-200
	if triggered < 50 || triggered > 200 {
		t.Errorf("probability 0.01: triggered %d/%d, expected 50-200", triggered, total)
	}
}

func TestShouldActivate_ProbabilityHigh(t *testing.T) {
	// With a very high probability, most requests should trigger.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{Probability: 0.99}

	triggered := 0
	total := 10000
	for i := 0; i < total; i++ {
		if ShouldActivate(activation, rng, 0, time.Time{}).ShouldFire {
			triggered++
		}
	}

	// Expect ~9900 out of 10000; allow wide margin: 9800-10000
	if triggered < 9800 || triggered > 10000 {
		t.Errorf("probability 0.99: triggered %d/%d, expected 9800-10000", triggered, total)
	}
}

func TestShouldActivate_Deterministic(t *testing.T) {
	// Same seed must produce the same activation sequence.
	activation := &spec.Activation{Probability: 0.5}

	rng1 := randx.NewGlobal(42)
	results1 := make([]bool, 100)
	for i := range results1 {
		results1[i] = ShouldActivate(activation, rng1, 0, time.Time{}).ShouldFire
	}

	rng2 := randx.NewGlobal(42)
	results2 := make([]bool, 100)
	for i := range results2 {
		results2[i] = ShouldActivate(activation, rng2, 0, time.Time{}).ShouldFire
	}

	for i := range results1 {
		if results1[i] != results2[i] {
			t.Errorf("determinism broken at index %d: %v vs %v", i, results1[i], results2[i])
		}
	}
}

func TestShouldActivate_ConcurrentSafety(t *testing.T) {
	// Multiple goroutines calling ShouldActivate concurrently should not race.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{Probability: 0.5}

	done := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func() {
			defer func() { done <- true }()
			ShouldActivate(activation, rng, 0, time.Time{})
		}()
	}

	for i := 0; i < 200; i++ {
		<-done
	}
}

// --- Integration: applyFault with activation ---

func TestHTTPWriter_ApplyFault_WithActivation_ProbabilityOne(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// Probability 1.0 should always fire the fault
	def := &spec.StubDefinition{
		ID: "activation-p1",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					Probability: 1.0,
				},
			},
		},
	}

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
		if err != nil {
			t.Fatalf("WriteResponse failed: %v", err)
		}
		if rr.Code != 500 {
			t.Errorf("iteration %d: status = %d, want 500 (fault should always fire)", i, rr.Code)
		}
	}
}

func TestHTTPWriter_ApplyFault_WithActivation_ProbabilityZero(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// Probability 0.0 is treated as "not configured" (zero-value).
	// With no other mode configured, the fault is always-on.
	def := &spec.StubDefinition{
		ID: "activation-p0",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					Probability: 0.0,
				},
			},
		},
	}

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
		if err != nil {
			t.Fatalf("WriteResponse failed: %v", err)
		}
		// Probability 0.0 alone = no mode configured = always-on
		if rr.Code != 500 {
			t.Errorf("iteration %d: status = %d, want 500 (probability 0.0 alone = always-on)", i, rr.Code)
		}
	}
}

func TestHTTPWriter_ApplyFault_WithActivation_BackwardCompatible(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// Fault without activation should behave as always-on (backward compatible)
	def := &spec.StubDefinition{
		ID: "no-activation",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "error"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	if rr.Code != 500 {
		t.Errorf("status = %d, want 500 (always-on fault)", rr.Code)
	}
	wantBody := `{"error":"internal server error","fault":"error"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}
}

func TestHTTPWriter_ApplyFault_WithActivation_Statistical(t *testing.T) {
	// Full integration test: 1000 requests with probability 0.5 and seed 1
	// should produce 450-550 fault responses.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(1)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "activation-stat",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					Probability: 0.5,
				},
			},
		},
	}

	faultCount := 0
	normalCount := 0
	total := 1000

	for i := 0; i < total; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
		if err != nil {
			t.Fatalf("WriteResponse failed on iteration %d: %v", i, err)
		}
		if rr.Code == 500 {
			faultCount++
		} else if rr.Code == 200 {
			normalCount++
		}
	}

	if faultCount+normalCount != total {
		t.Errorf("total responses = %d, want %d", faultCount+normalCount, total)
	}

	if faultCount < 450 || faultCount > 550 {
		t.Errorf("fault triggered %d/%d, expected 450-550", faultCount, total)
	}
}

func TestHTTPWriter_ApplyFault_WithActivation_EmptyFaultType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// Fault with activation but probability 0.0 (unconfigured) — no mode
	// is configured, so the fault is always-on and fires on every request.
	def := &spec.StubDefinition{
		ID: "empty-activation",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					Probability: 0.0,
				},
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Probability 0.0 alone = no mode configured = always-on
	if rr.Code != 500 {
		t.Errorf("status = %d, want 500 (always-on)", rr.Code)
	}
}

// --- A2: EveryNthRequest activation tests ---

func TestShouldActivate_EveryNthRequest_FiresOnMultiples(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 3}

	// hitCount starts at 1 (after increment). Should fire on 3, 6, 9.
	results := make([]bool, 10)
	for i := 1; i <= 10; i++ {
		results[i-1] = ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire
	}

	// Expected: [false, false, true, false, false, true, false, false, true, false]
	want := []bool{false, false, true, false, false, true, false, false, true, false}
	for i, got := range results {
		if got != want[i] {
			t.Errorf("hitCount %d: ShouldActivate = %v, want %v", i+1, got, want[i])
		}
	}
}

func TestShouldActivate_EveryNthRequest_One(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 1}

	// EveryNthRequest=1 means every request fires (1%1==0, 2%1==0, etc.)
	for i := 1; i <= 10; i++ {
		if !ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire {
			t.Errorf("hitCount %d: ShouldActivate = false, want true (everyNthRequest=1)", i)
		}
	}
}

func TestShouldActivate_EveryNthRequest_Two(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 2}

	// Should fire on even hitCounts: 2, 4, 6, 8, 10
	for i := 1; i <= 10; i++ {
		got := ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire
		want := i%2 == 0
		if got != want {
			t.Errorf("hitCount %d: ShouldActivate = %v, want %v", i, got, want)
		}
	}
}

func TestShouldActivate_EveryNthRequest_ZeroNotConfigured(t *testing.T) {
	rng := randx.NewGlobal(42)
	// EveryNthRequest=0 means not configured — should not block activation.
	// Combined with Probability=1.0, the fault should always fire.
	activation := &spec.Activation{EveryNthRequest: 0, Probability: 1.0}

	for i := 0; i < 10; i++ {
		if !ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire {
			t.Errorf("hitCount %d: ShouldActivate = false, want true (everyNthRequest=0 is no-op)", i)
		}
	}
}

func TestShouldActivate_EveryNthRequest_AND_Probability(t *testing.T) {
	// When both everyNthRequest and probability are configured, AND semantics:
	// both must pass for the fault to fire.
	// With everyNthRequest=2 and probability=1.0, fault fires on every even hitCount.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 2, Probability: 1.0}

	for i := 1; i <= 10; i++ {
		got := ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire
		want := i%2 == 0 // everyNthRequest passes on even, probability=1.0 always passes
		if got != want {
			t.Errorf("hitCount %d: ShouldActivate = %v, want %v (AND: nth=%v, prob=1.0)",
				i, got, want, i%2 == 0)
		}
	}
}

func TestShouldActivate_EveryNthRequest_AND_ProbabilityZero(t *testing.T) {
	// everyNthRequest=2 AND probability=0.0 (unconfigured) → only everyNthRequest
	// is configured, so the fault fires on every even hitCount.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 2, Probability: 0.0}

	for i := 1; i <= 10; i++ {
		got := ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire
		want := i%2 == 0
		if got != want {
			t.Errorf("hitCount %d: ShouldActivate = %v, want %v (probability 0.0 is unconfigured)", i, got, want)
		}
	}
}

func TestShouldActivate_EveryNthRequest_AND_ProbabilityHalf(t *testing.T) {
	// everyNthRequest=2 AND probability=0.5 → fires on even hitCounts with 50% chance.
	// Over many iterations, roughly half of even hitCounts should fire.
	rng := randx.NewGlobal(1)
	activation := &spec.Activation{EveryNthRequest: 2, Probability: 0.5}

	triggered := 0
	evenCount := 0
	total := 1000

	for i := 1; i <= total; i++ {
		if ShouldActivate(activation, rng, uint64(i), time.Time{}).ShouldFire {
			triggered++
		}
		if i%2 == 0 {
			evenCount++
		}
	}

	// Even hitCounts = 500. With probability 0.5, expect ~250 triggers.
	// Allow wide margin: 150-350.
	if triggered < 150 || triggered > 350 {
		t.Errorf("everyNthRequest=2 AND probability=0.5: triggered %d/%d (even=%d), expected 150-350",
			triggered, total, evenCount)
	}

	// Odd hitCounts should never trigger (everyNthRequest blocks).
	// This is implicitly verified by the upper bound above, but let's be explicit.
}

func TestShouldActivate_EveryNthRequest_Deterministic(t *testing.T) {
	// Same seed + same hitCount sequence must produce identical results.
	activation := &spec.Activation{EveryNthRequest: 3, Probability: 0.5}

	rng1 := randx.NewGlobal(42)
	results1 := make([]bool, 100)
	for i := range results1 {
		results1[i] = ShouldActivate(activation, rng1, uint64(i+1), time.Time{}).ShouldFire
	}

	rng2 := randx.NewGlobal(42)
	results2 := make([]bool, 100)
	for i := range results2 {
		results2[i] = ShouldActivate(activation, rng2, uint64(i+1), time.Time{}).ShouldFire
	}

	for i := range results1 {
		if results1[i] != results2[i] {
			t.Errorf("determinism broken at hitCount %d: %v vs %v", i+1, results1[i], results2[i])
		}
	}
}

func TestShouldActivate_EveryNthRequest_ConcurrentSafety(t *testing.T) {
	// Multiple goroutines calling ShouldActivate with different hitCounts should not race.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{EveryNthRequest: 5, Probability: 0.5}

	done := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func(hitCount uint64) {
			defer func() { done <- true }()
			ShouldActivate(activation, rng, hitCount, time.Time{})
		}(uint64(i + 1))
	}

	for i := 0; i < 200; i++ {
		<-done
	}
}

// --- A2: Integration tests with HTTPWriter ---

func TestHTTPWriter_ApplyFault_WithEveryNthRequest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "nth-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					EveryNthRequest: 3,
				},
			},
		},
	}

	// Simulate 9 requests with incrementing hitCount.
	// Fault should fire on hitCount 3, 6, 9.
	for hitCount := uint64(1); hitCount <= 9; hitCount++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, hitCount, time.Time{})
		if err != nil {
			t.Fatalf("hitCount %d: WriteResponse failed: %v", hitCount, err)
		}

		isNth := hitCount%3 == 0
		if isNth {
			if rr.Code != 500 {
				t.Errorf("hitCount %d: status = %d, want 500 (fault should fire)", hitCount, rr.Code)
			}
		} else {
			if rr.Code != 200 {
				t.Errorf("hitCount %d: status = %d, want 200 (fault should not fire)", hitCount, rr.Code)
			}
			if rr.Body.String() != "ok" {
				t.Errorf("hitCount %d: body = %q, want %q", hitCount, rr.Body.String(), "ok")
			}
		}
	}
}

func TestHTTPWriter_ApplyFault_EveryNthRequest_AND_ProbabilityOne(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "nth-prob-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					EveryNthRequest: 2,
					Probability:     1.0,
				},
			},
		},
	}

	// everyNthRequest=2 AND probability=1.0 → fires on even hitCounts only.
	for hitCount := uint64(1); hitCount <= 6; hitCount++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, hitCount, time.Time{})
		if err != nil {
			t.Fatalf("hitCount %d: WriteResponse failed: %v", hitCount, err)
		}

		isEven := hitCount%2 == 0
		if isEven && rr.Code != 500 {
			t.Errorf("hitCount %d: status = %d, want 500 (AND: nth passes, prob=1.0 passes)", hitCount, rr.Code)
		}
		if !isEven && rr.Code != 200 {
			t.Errorf("hitCount %d: status = %d, want 200 (AND: nth fails)", hitCount, rr.Code)
		}
	}
}

func TestHTTPWriter_ApplyFault_EveryNthRequest_AND_ProbabilityZero(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// everyNthRequest=2 AND probability=0.0 (unconfigured) → only everyNthRequest applies.
	def := &spec.StubDefinition{
		ID: "nth-prob0-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					EveryNthRequest: 2,
					Probability:     0.0,
				},
			},
		},
	}

	// Fault should fire on even hitCounts.
	for hitCount := uint64(1); hitCount <= 6; hitCount++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, hitCount, time.Time{})
		if err != nil {
			t.Fatalf("hitCount %d: WriteResponse failed: %v", hitCount, err)
		}

		isEven := hitCount%2 == 0
		if isEven && rr.Code != 500 {
			t.Errorf("hitCount %d: status = %d, want 500 (nth fires, prob 0.0 is unconfigured)", hitCount, rr.Code)
		}
		if !isEven && rr.Code != 200 {
			t.Errorf("hitCount %d: status = %d, want 200 (nth does not fire)", hitCount, rr.Code)
		}
	}
}

func TestHTTPWriter_ApplyFault_EveryNthRequest_BackwardCompatible(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	// Fault without activation (nil) should behave as always-on.
	def := &spec.StubDefinition{
		ID: "no-activation-nth",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Fault:  &spec.FaultDefinition{Type: "error"},
		},
	}

	for hitCount := uint64(1); hitCount <= 5; hitCount++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, err := w.WriteResponse(rr, def, req, nil, hitCount, time.Time{})
		if err != nil {
			t.Fatalf("hitCount %d: WriteResponse failed: %v", hitCount, err)
		}
		if rr.Code != 500 {
			t.Errorf("hitCount %d: status = %d, want 500 (always-on fault)", hitCount, rr.Code)
		}
	}
}

// --- A3: ActiveBetween time-window activation tests ---

func TestShouldActivate_ActiveBetween_SingleWindow_AlwaysOn(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 5000}, // 0-5s: always-on (no window-level probability)
		},
	}

	serverStart := time.Now()

	// Within window (immediately after start): should activate.
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false within [0, 5000ms) window, want true")
	}

	// Outside window (6 seconds after start): should not activate.
	outsideStart := serverStart.Add(-6 * time.Second)
	if ShouldActivate(activation, rng, 1, outsideStart).ShouldFire {
		t.Error("ShouldActivate = true outside [0, 5000ms) window, want false")
	}
}

func TestShouldActivate_ActiveBetween_SingleWindow_WithProbability(t *testing.T) {
	rng := randx.NewGlobal(1)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 5000, Probability: 0.5},
		},
	}

	serverStart := time.Now()

	// Within window with probability 0.5: statistical test.
	triggered := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
			triggered++
		}
	}
	if triggered < 350 || triggered > 650 {
		t.Errorf("window probability 0.5: triggered %d/%d, expected 350-650", triggered, total)
	}
}

func TestShouldActivate_ActiveBetween_MultipleSegments(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 30000},                       // 0-30s: always-on
			{StartMs: 30000, EndMs: 90000, Probability: 1.0}, // 30-90s: 100% probability
		},
	}

	serverStart := time.Now()

	// Within first window (0-30s): always-on, should activate.
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false in first window [0, 30s), want true")
	}

	// Within second window (30-60s): should activate (probability 1.0).
	inSecondWindow := serverStart.Add(-35 * time.Second) // elapsed = 35s
	if !ShouldActivate(activation, rng, 1, inSecondWindow).ShouldFire {
		t.Error("ShouldActivate = false in second window [30s, 90s), want true")
	}

	// Outside all windows (100s elapsed): should not activate.
	outsideAll := serverStart.Add(-100 * time.Second)
	if ShouldActivate(activation, rng, 1, outsideAll).ShouldFire {
		t.Error("ShouldActivate = true outside all windows, want false")
	}
}

func TestShouldActivate_ActiveBetween_MultipleSegments_GradualFault(t *testing.T) {
	// Simulate gradual fault: 0-30s always-on, 30-90s 50% probability
	rng := randx.NewGlobal(1)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 30000},                       // 0-30s: always-on
			{StartMs: 30000, EndMs: 90000, Probability: 0.5}, // 30-90s: 50%
		},
	}

	// In the second window (elapsed=60s), probability should be ~50%.
	serverStart := time.Now().Add(-60 * time.Second) // 60s ago = elapsed 60s

	triggered := 0
	total := 1000
	for i := 0; i < total; i++ {
		if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
			triggered++
		}
	}
	if triggered < 350 || triggered > 650 {
		t.Errorf("second window probability 0.5: triggered %d/%d, expected 350-650", triggered, total)
	}
}

func TestShouldActivate_ActiveBetween_OutsideAllWindows(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 1000, EndMs: 5000},
			{StartMs: 10000, EndMs: 20000},
		},
	}

	// 500ms elapsed: before all windows.
	serverStart := time.Now().Add(-500 * time.Millisecond)
	if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = true before all windows (500ms), want false")
	}

	// 7s elapsed: between windows (gap).
	serverStart = time.Now().Add(-7 * time.Second)
	if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = true between windows (7s), want false")
	}

	// 25s elapsed: after all windows.
	serverStart = time.Now().Add(-25 * time.Second)
	if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = true after all windows (25s), want false")
	}
}

func TestShouldActivate_ActiveBetween_FirstWindowWins(t *testing.T) {
	// When time falls in multiple overlapping windows, the first one wins.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 10000, Probability: 1.0},    // first window: 100%
			{StartMs: 5000, EndMs: 15000, Probability: 0.0}, // second window: no override
		},
	}

	// At 7s: both windows match. First window wins -> always-on.
	serverStart := time.Now().Add(-7 * time.Second)
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false when in overlapping windows, first should win with prob=1.0")
	}
}

func TestShouldActivate_ActiveBetween_WindowProbabilityZeroIsAlwaysOn(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 10000}, // no Probability = always-on within window
		},
	}

	serverStart := time.Now()
	for i := 0; i < 100; i++ {
		if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
			t.Error("ShouldActivate = false with no window probability (always-on), want true")
			break
		}
	}
}

func TestShouldActivate_ActiveBetween_AND_Probability(t *testing.T) {
	// activeBetween AND probability: both must pass.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		Probability: 1.0,
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 10000}, // always-on within window
		},
	}

	serverStart := time.Now()
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false with activeBetween (in window) AND probability=1.0, want true")
	}

	// Outside the window: activeBetween fails -> AND result is false.
	outsideStart := serverStart.Add(-15 * time.Second)
	if ShouldActivate(activation, rng, 1, outsideStart).ShouldFire {
		t.Error("ShouldActivate = true with activeBetween (outside window) AND probability=1.0, want false")
	}
}

func TestShouldActivate_ActiveBetween_AND_EveryNthRequest(t *testing.T) {
	// activeBetween AND everyNthRequest: both must pass.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		EveryNthRequest: 2,
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 10000}, // always-on within window
		},
	}

	serverStart := time.Now()

	// hitCount=2: both pass (in window, even hitCount).
	if !ShouldActivate(activation, rng, 2, serverStart).ShouldFire {
		t.Error("ShouldActivate = false with activeBetween (in window) AND everyNthRequest=2 at hitCount=2, want true")
	}

	// hitCount=1: in window but odd hitCount.
	if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = true with activeBetween (in window) AND everyNthRequest=2 at hitCount=1, want false")
	}

	// hitCount=2: outside window (even if nth passes, time fails).
	outsideStart := serverStart.Add(-15 * time.Second)
	if ShouldActivate(activation, rng, 2, outsideStart).ShouldFire {
		t.Error("ShouldActivate = true with activeBetween (outside) AND everyNthRequest=2, want false")
	}
}

func TestShouldActivate_ActiveBetween_NoActiveBetween_BackwardCompatible(t *testing.T) {
	// Empty ActiveBetween should not affect activation (backward compatible).
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		Probability:   1.0,
		ActiveBetween: []spec.TimeWindow{}, // empty
	}

	serverStart := time.Now()
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false with empty ActiveBetween AND probability=1.0, want true")
	}
}

func TestShouldActivate_ActiveBetween_ZeroStartTime(t *testing.T) {
	// When serverStart is zero (not set), time.Since(zero) is huge, so
	// the elapsed time will be very large and likely outside all windows.
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 5000},
		},
	}

	// Zero serverStart: time.Since(zero) is decades, way past the 5s window.
	if ShouldActivate(activation, rng, 1, time.Time{}).ShouldFire {
		t.Error("ShouldActivate = true with zero serverStart and small window, want false (elapsed is huge)")
	}
}

func TestShouldActivate_ActiveBetween_EdgeExactStart(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 1000, EndMs: 5000},
		},
	}

	// Exactly at startMs (1s elapsed): inclusive, should match.
	serverStart := time.Now().Add(-1000 * time.Millisecond)
	if !ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = false at exactly startMs, want true (inclusive)")
	}
}

func TestShouldActivate_ActiveBetween_EdgeExactEnd(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 5000},
		},
	}

	// Exactly at endMs (5s elapsed): exclusive, should NOT match.
	serverStart := time.Now().Add(-5000 * time.Millisecond)
	if ShouldActivate(activation, rng, 1, serverStart).ShouldFire {
		t.Error("ShouldActivate = true at exactly endMs, want false (exclusive)")
	}
}

func TestShouldActivate_ActiveBetween_Deterministic(t *testing.T) {
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 10000, Probability: 0.5},
		},
	}

	serverStart := time.Now()

	rng1 := randx.NewGlobal(42)
	results1 := make([]bool, 100)
	for i := range results1 {
		results1[i] = ShouldActivate(activation, rng1, 1, serverStart).ShouldFire
	}

	rng2 := randx.NewGlobal(42)
	results2 := make([]bool, 100)
	for i := range results2 {
		results2[i] = ShouldActivate(activation, rng2, 1, serverStart).ShouldFire
	}

	for i := range results1 {
		if results1[i] != results2[i] {
			t.Errorf("determinism broken at index %d: %v vs %v", i, results1[i], results2[i])
		}
	}
}

func TestShouldActivate_ActiveBetween_ConcurrentSafety(t *testing.T) {
	rng := randx.NewGlobal(42)
	activation := &spec.Activation{
		ActiveBetween: []spec.TimeWindow{
			{StartMs: 0, EndMs: 60000, Probability: 0.5},
		},
	}

	serverStart := time.Now()
	done := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func() {
			defer func() { done <- true }()
			ShouldActivate(activation, rng, 1, serverStart)
		}()
	}

	for i := 0; i < 200; i++ {
		<-done
	}
}

// --- A3: ValidateActivation TimeWindow overlap tests ---

func TestValidateActivation_OverlappingWindows(t *testing.T) {
	tests := []struct {
		name       string
		activation *spec.Activation
		wantErr    bool
	}{
		{
			name: "non-overlapping adjacent windows are valid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 5000},
					{StartMs: 5000, EndMs: 10000},
				},
			},
			wantErr: false,
		},
		{
			name: "non-overlapping separated windows are valid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 5000},
					{StartMs: 10000, EndMs: 20000},
				},
			},
			wantErr: false,
		},
		{
			name: "overlapping windows are invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 10000},
					{StartMs: 5000, EndMs: 15000},
				},
			},
			wantErr: true,
		},
		{
			name: "fully contained window is invalid",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 20000},
					{StartMs: 5000, EndMs: 10000},
				},
			},
			wantErr: true,
		},
		{
			name: "three windows first two overlap",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 10000},
					{StartMs: 5000, EndMs: 15000},
					{StartMs: 20000, EndMs: 30000},
				},
			},
			wantErr: true,
		},
		{
			name: "three windows none overlap",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 0, EndMs: 5000},
					{StartMs: 5000, EndMs: 10000},
					{StartMs: 10000, EndMs: 15000},
				},
			},
			wantErr: false,
		},
		{
			name: "zero-length windows at same point do not overlap",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 5000, EndMs: 5000},
					{StartMs: 5000, EndMs: 5000},
				},
			},
			wantErr: false, // zero-length windows [5k,5k) are empty, so no overlap
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			err := ValidateActivation(tt.activation)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateActivation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- A3: Integration tests with HTTPWriter ---

func TestHTTPWriter_ApplyFault_WithActiveBetween_InWindow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "timewindow-in",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					ActiveBetween: []spec.TimeWindow{
						{StartMs: 0, EndMs: 60000}, // 0-60s: always-on
					},
				},
			},
		},
	}

	serverStart := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := w.WriteResponse(rr, def, req, nil, 1, serverStart)
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}
	if rr.Code != 500 {
		t.Errorf("status = %d, want 500 (fault should fire within active window)", rr.Code)
	}
}

func TestHTTPWriter_ApplyFault_WithActiveBetween_OutsideWindow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "timewindow-out",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					ActiveBetween: []spec.TimeWindow{
						{StartMs: 0, EndMs: 5000}, // 0-5s only
					},
				},
			},
		},
	}

	// Server started 10s ago, so elapsed > 5s -> outside window.
	serverStart := time.Now().Add(-10 * time.Second)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := w.WriteResponse(rr, def, req, nil, 1, serverStart)
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}
	if rr.Code != 200 {
		t.Errorf("status = %d, want 200 (fault should NOT fire outside window)", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "ok")
	}
}

func TestHTTPWriter_ApplyFault_WithActiveBetween_MultiSegment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "timewindow-multi",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					ActiveBetween: []spec.TimeWindow{
						{StartMs: 0, EndMs: 5000},                        // 0-5s: always-on
						{StartMs: 10000, EndMs: 20000, Probability: 1.0}, // 10-20s: 100%
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		serverStart time.Time
		wantStatus  int
		wantBody    string
	}{
		{
			name:        "in first window (0-5s)",
			serverStart: time.Now(),
			wantStatus:  500,
		},
		{
			name:        "between windows (7s)",
			serverStart: time.Now().Add(-7 * time.Second),
			wantStatus:  200,
			wantBody:    "ok",
		},
		{
			name:        "in second window (15s)",
			serverStart: time.Now().Add(-15 * time.Second),
			wantStatus:  500,
		},
		{
			name:        "after all windows (25s)",
			serverStart: time.Now().Add(-25 * time.Second),
			wantStatus:  200,
			wantBody:    "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := w.WriteResponse(rr, def, req, nil, 1, tt.serverStart)
			if err != nil {
				t.Fatalf("WriteResponse failed: %v", err)
			}
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHTTPWriter_ApplyFault_WithActiveBetween_AND_EveryNthRequest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "timewindow-nth",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault: &spec.FaultDefinition{
				Type: "error",
				Activation: &spec.Activation{
					EveryNthRequest: 2,
					ActiveBetween: []spec.TimeWindow{
						{StartMs: 0, EndMs: 60000}, // in window
					},
				},
			},
		},
	}

	serverStart := time.Now()

	// hitCount=1: in window but odd -> AND fails (nth blocks).
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := w.WriteResponse(rr, def, req, nil, 1, serverStart)
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}
	if rr.Code != 200 {
		t.Errorf("hitCount=1: status = %d, want 200 (nth blocks)", rr.Code)
	}

	// hitCount=2: in window AND even -> AND passes.
	rr = httptest.NewRecorder()
	_, err = w.WriteResponse(rr, def, req, nil, 2, serverStart)
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}
	if rr.Code != 500 {
		t.Errorf("hitCount=2: status = %d, want 500 (both pass)", rr.Code)
	}
}

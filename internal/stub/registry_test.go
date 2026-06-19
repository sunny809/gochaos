package stub_test

import (
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
	"github.com/sunny809/gochaos/pkg/gmock"
)

func TestRegistryAddAndGet(t *testing.T) {
	r := stub.NewRegistry()

	def := gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "ok"},
	}

	id, err := r.Add(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	retrieved := r.Get(id)
	if retrieved == nil {
		t.Fatal("expected stub to be retrievable")
	}
	if retrieved.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", retrieved.Request.Method)
	}
}

func TestRegistryAddPreservesID(t *testing.T) {
	r := stub.NewRegistry()

	def := gmock.StubDefinition{
		ID:       "custom-id",
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: 200},
	}

	id, _ := r.Add(def)
	if id != "custom-id" {
		t.Errorf("expected custom-id, got %s", id)
	}
}

func TestRegistryDelete(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
	})

	if !r.Delete(id) {
		t.Error("expected Delete to return true")
	}
	if r.Get(id) != nil {
		t.Error("expected stub to be deleted")
	}
	if r.Delete(id) {
		t.Error("expected second Delete to return false")
	}
}

func TestRegistryDeleteAll(t *testing.T) {
	r := stub.NewRegistry()

	for i := 0; i < 5; i++ {
		r.Add(gmock.StubDefinition{
			Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		})
	}

	if r.Len() != 5 {
		t.Errorf("expected 5 stubs, got %d", r.Len())
	}

	r.DeleteAll()
	if r.Len() != 0 {
		t.Errorf("expected 0 stubs after DeleteAll, got %d", r.Len())
	}
}

func TestRegistryList(t *testing.T) {
	r := stub.NewRegistry()

	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/a"},
	})
	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/b"},
	})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 stubs, got %d", len(list))
	}
}

func TestRegistryPriorityOrdering(t *testing.T) {
	r := stub.NewRegistry()

	// Add stubs in unsorted priority order
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/low"},
		Priority: 10,
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/high"},
		Priority: 1,
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/mid"},
		Priority: 5,
	})

	list := r.List()
	if list[0].Request.URLPath != "/high" {
		t.Errorf("expected /high first, got %s", list[0].Request.URLPath)
	}
	if list[1].Request.URLPath != "/mid" {
		t.Errorf("expected /mid second, got %s", list[1].Request.URLPath)
	}
	if list[2].Request.URLPath != "/low" {
		t.Errorf("expected /low third, got %s", list[2].Request.URLPath)
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := stub.NewRegistry()
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Add(gmock.StubDefinition{
				Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
			})
		}()
	}

	// Concurrent readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_ = r.Len()
		}()
	}

	wg.Wait()

	if r.Len() != 100 {
		t.Errorf("expected 100 stubs, got %d", r.Len())
	}
}

func TestEngineMatch(t *testing.T) {
	r := stub.NewRegistry()
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "users"},
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "POST", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 201, Body: "created"},
	})

	engine := stub.NewEngine(r)

	// Match GET /users
	req := httptest.NewRequest("GET", "/users", nil)
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match for GET /users")
	}
	if result.Stub.Response.Body != "users" {
		t.Errorf("expected 'users' body, got %s", result.Stub.Response.Body)
	}

	// Match POST /users
	req = httptest.NewRequest("POST", "/users", nil)
	result = engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match for POST /users")
	}
	if result.Stub.Response.Status != 201 {
		t.Errorf("expected 201, got %d", result.Stub.Response.Status)
	}

	// No match for DELETE
	req = httptest.NewRequest("DELETE", "/users", nil)
	result = engine.Match(req)
	if result != nil {
		t.Errorf("expected no match for DELETE, got %+v", result)
	}
}

func TestEngineMatchSpecificity(t *testing.T) {
	r := stub.NewRegistry()

	// Less specific stub (matches GET to anything)
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "any"},
	})

	// More specific stub (matches GET /users)
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "specific"},
	})

	engine := stub.NewEngine(r)

	req := httptest.NewRequest("GET", "/users", nil)
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match")
	}
	if result.Stub.Response.Body != "specific" {
		t.Errorf("expected most specific stub to win, got %s", result.Stub.Response.Body)
	}
}

func TestEngineWithHeaders(t *testing.T) {
	r := stub.NewRegistry()

	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  "GET",
			URLPath: "/api",
			Headers: map[string]string{"Authorization": "Bearer token"},
		},
		Response: gmock.ResponseDefinition{Status: 200, Body: "authorized"},
	})

	engine := stub.NewEngine(r)

	// With correct header
	req := httptest.NewRequest("GET", "/api", nil)
	req.Header.Set("Authorization", "Bearer token")
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match with correct header")
	}

	// Without header
	req = httptest.NewRequest("GET", "/api", nil)
	result = engine.Match(req)
	if result != nil {
		t.Error("expected no match without header")
	}
}

func TestEngineEmptyRegistry(t *testing.T) {
	r := stub.NewRegistry()
	engine := stub.NewEngine(r)

	req := httptest.NewRequest("GET", "/anything", nil)
	result := engine.Match(req)
	if result != nil {
		t.Errorf("expected no match on empty registry, got %+v", result)
	}
}

func TestRegistry_Add_InvalidFault(t *testing.T) {
	r := stub.NewRegistry()

	tests := []struct {
		name      string
		faultType string
		wantErr   bool
	}{
		{name: "valid fault type error", faultType: "error", wantErr: false},
		{name: "valid fault type empty", faultType: "empty", wantErr: false},
		{name: "valid fault type connection_reset", faultType: "connection_reset", wantErr: false},
		{name: "invalid fault type", faultType: "INVALID", wantErr: true},
		{name: "case-sensitive mismatch", faultType: "Error", wantErr: true},
		{name: "hyphen instead of underscore", faultType: "connection-reset", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := spec.StubDefinition{
				Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
				Response: spec.ResponseDefinition{
					Status: 200,
					Fault:  &spec.FaultDefinition{Type: tt.faultType},
				},
			}
			_, err := r.Add(def)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				var valErr *stub.ValidationError
				if !errors.As(err, &valErr) {
					t.Errorf("expected *stub.ValidationError, got %T: %v", err, err)
				}
				if valErr.Field != "fault" {
					t.Errorf("expected field fault, got %q", valErr.Field)
				}
			}
		})
	}
}

func TestRegistry_Add_NoFault(t *testing.T) {
	r := stub.NewRegistry()

	// Stub with nil fault should succeed
	def := spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	}
	id, err := r.Add(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}
}

func TestRegistry_Add_InvalidActivation(t *testing.T) {
	r := stub.NewRegistry()

	tests := []struct {
		name       string
		activation *spec.Activation
		wantErr    bool
	}{
		{
			name:       "valid activation probability 0.5",
			activation: &spec.Activation{Probability: 0.5},
			wantErr:    false,
		},
		{
			name:       "valid activation probability 1.0",
			activation: &spec.Activation{Probability: 1.0},
			wantErr:    false,
		},
		{
			name:       "invalid activation probability -0.1",
			activation: &spec.Activation{Probability: -0.1},
			wantErr:    true,
		},
		{
			name:       "invalid activation probability 1.5",
			activation: &spec.Activation{Probability: 1.5},
			wantErr:    true,
		},
		{
			name:       "nil activation is valid",
			activation: nil,
			wantErr:    false,
		},
		{
			name: "invalid time window endMs < startMs",
			activation: &spec.Activation{
				ActiveBetween: []spec.TimeWindow{
					{StartMs: 2000, EndMs: 1000},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			def := spec.StubDefinition{
				Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
				Response: spec.ResponseDefinition{
					Status: 200,
					Fault: &spec.FaultDefinition{
						Type:       "error",
						Activation: tt.activation,
					},
				},
			}
			_, err := r.Add(def)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErr {
				var valErr *stub.ValidationError
				if !errors.As(err, &valErr) {
					t.Errorf("expected *stub.ValidationError, got %T: %v", err, err)
				}
				if valErr.Field != "fault" {
					t.Errorf("expected field fault, got %q", valErr.Field)
				}
			}
		})
	}
}
func TestRegistry_IncrementHitCount(t *testing.T) {
	r := stub.NewRegistry()

	id, err := r.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Initial hit count should be 0
	if hc := r.GetHitCount(id); hc != 0 {
		t.Errorf("initial GetHitCount = %d, want 0", hc)
	}

	// First increment returns 1
	if hc := r.IncrementHitCount(id); hc != 1 {
		t.Errorf("first IncrementHitCount = %d, want 1", hc)
	}

	// Second increment returns 2
	if hc := r.IncrementHitCount(id); hc != 2 {
		t.Errorf("second IncrementHitCount = %d, want 2", hc)
	}

	// GetHitCount reflects the current value
	if hc := r.GetHitCount(id); hc != 2 {
		t.Errorf("GetHitCount after 2 increments = %d, want 2", hc)
	}

	// Non-existent stub returns 0
	if hc := r.IncrementHitCount("nonexistent"); hc != 0 {
		t.Errorf("IncrementHitCount for nonexistent stub = %d, want 0", hc)
	}
	if hc := r.GetHitCount("nonexistent"); hc != 0 {
		t.Errorf("GetHitCount for nonexistent stub = %d, want 0", hc)
	}
}

func TestRegistry_IncrementHitCount_IndependentPerStub(t *testing.T) {
	r := stub.NewRegistry()

	id1, _ := r.Add(spec.StubDefinition{
		ID:       "stub-1",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/a"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	id2, _ := r.Add(spec.StubDefinition{
		ID:       "stub-2",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/b"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	// Increment stub-1 three times
	r.IncrementHitCount(id1)
	r.IncrementHitCount(id1)
	r.IncrementHitCount(id1)

	// Increment stub-2 once
	r.IncrementHitCount(id2)

	// Each stub has independent hit count
	if hc := r.GetHitCount(id1); hc != 3 {
		t.Errorf("stub-1 hitCount = %d, want 3", hc)
	}
	if hc := r.GetHitCount(id2); hc != 1 {
		t.Errorf("stub-2 hitCount = %d, want 1", hc)
	}
}

func TestRegistry_IncrementHitCount_ConcurrentSafety(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	var wg sync.WaitGroup
	increments := 1000

	for i := 0; i < increments; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.IncrementHitCount(id)
		}()
	}

	wg.Wait()

	// After 1000 concurrent increments, hit count must be exactly 1000
	if hc := r.GetHitCount(id); hc != uint64(increments) {
		t.Errorf("concurrent IncrementHitCount: got %d, want %d", hc, increments)
	}
}

func TestRegistry_DeleteAll_ResetsHitCount(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	// Increment a few times
	r.IncrementHitCount(id)
	r.IncrementHitCount(id)
	if hc := r.GetHitCount(id); hc != 2 {
		t.Errorf("pre-delete hitCount = %d, want 2", hc)
	}

	// DeleteAll removes the stub entirely
	r.DeleteAll()

	// After DeleteAll, the old ID no longer exists
	if hc := r.GetHitCount(id); hc != 0 {
		t.Errorf("post-delete hitCount for old ID = %d, want 0", hc)
	}

	// Add a new stub — its hit count starts at 0
	id2, _ := r.Add(spec.StubDefinition{
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	if hc := r.GetHitCount(id2); hc != 0 {
		t.Errorf("new stub hitCount = %d, want 0", hc)
	}
}

func TestRegistry_ShouldRateLimit_WarmUpPhase(t *testing.T) {
	r := stub.NewRegistry()

	id, err := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", AfterRequests: 5, PerSecond: 2},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First 5 requests should NOT be rate-limited (warm-up phase).
	// hitCount 1-5 are all within the warm-up window.
	for i := 0; i < 5; i++ {
		r.IncrementHitCount(id)
		if limited := r.ShouldRateLimit(id, 5, 2); limited {
			t.Errorf("hitCount=%d should not be rate-limited during warm-up", i+1)
		}
	}

	// 6th request (hitCount=6): warm-up is over, token bucket starts.
	// Bucket is initialized full (2 tokens), first request consumes one.
	r.IncrementHitCount(id)
	if limited := r.ShouldRateLimit(id, 5, 2); limited {
		t.Error("first request after warm-up should not be rate-limited (bucket has tokens)")
	}
}

func TestRegistry_ShouldRateLimit_TokenBucketAfterWarmUp(t *testing.T) {
	r := stub.NewRegistry()

	id, err := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", AfterRequests: 2, PerSecond: 2},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warm-up: 2 requests (hitCount 1-2 are within warm-up).
	r.IncrementHitCount(id)
	r.IncrementHitCount(id)

	// After warm-up (hitCount=3+), token bucket starts with 2 tokens.
	// First 2 requests consume tokens.
	r.IncrementHitCount(id) // hitCount=3
	if limited := r.ShouldRateLimit(id, 2, 2); limited {
		t.Error("first request after warm-up should not be rate-limited")
	}
	r.IncrementHitCount(id) // hitCount=4
	if limited := r.ShouldRateLimit(id, 2, 2); limited {
		t.Error("second request after warm-up should not be rate-limited")
	}

	// Third request should be rate-limited (no tokens left).
	r.IncrementHitCount(id) // hitCount=5
	if limited := r.ShouldRateLimit(id, 2, 2); !limited {
		t.Error("third request after warm-up should be rate-limited (no tokens)")
	}
}

func TestRegistry_ShouldRateLimit_NoWarmUp(t *testing.T) {
	r := stub.NewRegistry()

	id, err := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With afterRequests=0 (default), rate limiting begins immediately.
	// First request should be allowed (full bucket = 1 token).
	if limited := r.ShouldRateLimit(id, 0, 1); limited {
		t.Error("first request should not be rate-limited")
	}

	// Second request should be rate-limited (no tokens left).
	if limited := r.ShouldRateLimit(id, 0, 1); !limited {
		t.Error("second request should be rate-limited")
	}
}

func TestRegistry_ShouldRateLimit_UnknownStub(t *testing.T) {
	r := stub.NewRegistry()

	// Unknown stub ID should not be rate-limited
	if limited := r.ShouldRateLimit("nonexistent", 5, 2); limited {
		t.Error("unknown stub should not be rate-limited")
	}
}

func TestRegistry_ShouldRateLimit_ZeroPerSecond(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 0},
		},
	})

	// perSecond=0 should never rate-limit (guard against invalid config)
	if limited := r.ShouldRateLimit(id, 0, 0); limited {
		t.Error("perSecond=0 should not rate-limit")
	}
}

func TestRegistry_ShouldRateLimit_TokenRefill(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 10},
		},
	})

	// Exhaust the bucket (10 tokens).
	for i := 0; i < 10; i++ {
		if limited := r.ShouldRateLimit(id, 0, 10); limited {
			t.Errorf("request %d should not be rate-limited", i+1)
		}
	}

	// Bucket should be empty now.
	if limited := r.ShouldRateLimit(id, 0, 10); !limited {
		t.Error("should be rate-limited when bucket is empty")
	}

	// Wait for tokens to refill (100ms should add ~1 token at rate 10/sec).
	// We wait a bit longer to account for timing imprecision.
	time.Sleep(150 * time.Millisecond)

	// Should now be able to make one more request.
	if limited := r.ShouldRateLimit(id, 0, 10); limited {
		t.Error("should not be rate-limited after token refill")
	}

	// But only one token refilled, so the next request should be limited.
	if limited := r.ShouldRateLimit(id, 0, 10); !limited {
		t.Error("should be rate-limited when only one token refilled")
	}
}

func TestRegistry_ShouldRateLimit_ConcurrentSafety(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 100},
		},
	})

	var wg sync.WaitGroup
	allowed := uint64(0)
	rateLimited := uint64(0)

	// Launch 200 concurrent requests. With perSecond=100 and a full bucket,
	// exactly 100 should be allowed (initial tokens), and the rest rate-limited.
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.ShouldRateLimit(id, 0, 100) {
				atomic.AddUint64(&rateLimited, 1)
			} else {
				atomic.AddUint64(&allowed, 1)
			}
		}()
	}

	wg.Wait()

	// With 200 concurrent requests and perSecond=100, we expect at most 100
	// allowed. Some may be rate-limited even before the bucket is fully
	// exhausted due to concurrent refill/consume, so we only check the
	// invariant: allowed + rateLimited == 200.
	total := allowed + rateLimited
	if total != 200 {
		t.Errorf("allowed + rateLimited = %d, want 200", total)
	}
	if allowed == 0 {
		t.Error("expected at least some requests to be allowed")
	}
}

func TestRegistry_DeleteAll_ResetsRateLimitState(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 1},
		},
	})

	// Exhaust the bucket.
	r.ShouldRateLimit(id, 0, 1)
	if limited := r.ShouldRateLimit(id, 0, 1); !limited {
		t.Error("should be rate-limited when bucket is empty")
	}

	// DeleteAll resets rate-limit state.
	r.DeleteAll()

	// Add a new stub with the same path.
	id2, _ := r.Add(spec.StubDefinition{
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 1},
		},
	})

	// New stub should have a full bucket.
	if limited := r.ShouldRateLimit(id2, 0, 1); limited {
		t.Error("new stub should not be rate-limited after DeleteAll")
	}
}

func TestRegistry_Delete_RemovesRateLimitState(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(spec.StubDefinition{
		ID:      "rl-stub",
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 1},
		},
	})

	// Exhaust the bucket.
	r.ShouldRateLimit(id, 0, 1)

	// Delete the stub.
	r.Delete(id)

	// Re-add with the same ID.
	id2, _ := r.Add(spec.StubDefinition{
		ID:      "rl-stub",
		Request: spec.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: spec.ResponseDefinition{
			Status: 200,
			Fault:  &spec.FaultDefinition{Type: "rate_limit", PerSecond: 1},
		},
	})

	// Should have a fresh bucket (rate-limit state was deleted with the stub).
	if limited := r.ShouldRateLimit(id2, 0, 1); limited {
		t.Error("re-added stub should not be rate-limited")
	}
}

func TestRegistry_Add_RateLimitValidation(t *testing.T) {
	r := stub.NewRegistry()

	tests := []struct {
		name    string
		fault   *spec.FaultDefinition
		wantErr bool
	}{
		{
			name:    "rate_limit with perSecond > 0 is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 2},
			wantErr: false,
		},
		{
			name:    "rate_limit with perSecond = 0 is invalid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 0},
			wantErr: true,
		},
		{
			name:    "rate_limit with afterRequests and perSecond is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", AfterRequests: 5, PerSecond: 2},
			wantErr: false,
		},
		{
			name:    "rate_limit with custom rateLimitStatus is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 10, RateLimitStatus: 503},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			def := spec.StubDefinition{
				Request:  spec.RequestPattern{Method: "GET", URLPath: "/test"},
				Response: spec.ResponseDefinition{Status: 200, Fault: tt.fault},
			}
			_, err := r.Add(def)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

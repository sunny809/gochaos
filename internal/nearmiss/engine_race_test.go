package nearmiss_test

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/spec"
)

// normalizeResults returns a deep copy with each Breakdown sorted by Dimension.
// We do this because stub.BuildMatcher iterates over Headers/QueryParams maps,
// whose iteration order is intentionally randomized by the Go runtime. The
// per-call dimension order therefore varies, but the *set* of dimensions and
// their scores must be invariant for the same (req, stubs).
func normalizeResults(in []spec.NearMissResult) []spec.NearMissResult {
	out := make([]spec.NearMissResult, len(in))
	for i, r := range in {
		bd := make([]spec.DimensionScore, len(r.Breakdown))
		copy(bd, r.Breakdown)
		sort.SliceStable(bd, func(a, b int) bool { return bd[a].Dimension < bd[b].Dimension })
		r.Breakdown = bd
		out[i] = r
	}
	return out
}

// buildRealisticStubs constructs a 50-stub registry that mixes method, path,
// header, query, and body matchers — enough variation to exercise every
// dimension Compute walks, and enough volume to surface races if any exist.
func buildRealisticStubs() []spec.StubDefinition {
	stubs := make([]spec.StubDefinition, 0, 50)
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for i := 0; i < 50; i++ {
		method := methods[i%len(methods)]
		s := spec.StubDefinition{
			ID:   fmt.Sprintf("stub-%02d", i),
			Name: fmt.Sprintf("Stub-%02d", i),
			Request: spec.RequestPattern{
				Method:  method,
				URLPath: fmt.Sprintf("/api/v1/resource/%d", i),
				Headers: map[string]string{
					"X-Tenant-Id": fmt.Sprintf("tenant-%d", i%7),
					"X-Trace-Id":  fmt.Sprintf("trace-%d", i),
				},
				QueryParams: map[string]string{
					"page": fmt.Sprintf("%d", (i%5)+1),
					"size": "20",
				},
			},
			Response: spec.ResponseDefinition{Status: 200},
		}
		// Roughly half the stubs also constrain the body. Mix of exact and regex
		// so body matchers exercise their full diagnose path.
		if i%2 == 0 {
			s.Request.Body = &spec.BodyPattern{ExactMatch: fmt.Sprintf(`{"id":%d}`, i)}
		} else {
			s.Request.Body = &spec.BodyPattern{RegexMatch: fmt.Sprintf(`"id":\s*%d`, i)}
		}
		// Sprinkle Accept on a third of stubs.
		if i%3 == 0 {
			s.Request.Accept = "application/json"
		}
		stubs = append(stubs, s)
	}
	return stubs
}

// TestEngine_ConcurrentCompute spins up 32 goroutines, each driving the same
// *Engine instance through 50 Compute calls against a shared 50-stub registry.
// Per-goroutine the request varies (different headers/body) so dimensions
// exercise concurrent body reads. Two assertions:
//   - The race detector must not fire (-race must be set on the test runner).
//   - Compute is deterministic: identical (req, stubs) inputs must yield
//     byte-equal results regardless of goroutine.
func TestEngine_ConcurrentCompute(t *testing.T) {
	const (
		goroutines = 32
		iterations = 50
	)

	stubs := buildRealisticStubs()
	engine := nearmiss.NewEngine(nearmiss.WithTopN(10)) // shared instance

	// Build a deterministic baseline against the canonical request used inside
	// the loop. Every goroutine that re-uses the same request body must match.
	baselineReq := httptest.NewRequest("POST", "/api/v1/resource/999?page=1&size=20",
		strings.NewReader(`{"id":7}`))
	baselineReq.Header.Set("X-Tenant-Id", "tenant-zzz")
	baselineReq.Header.Set("X-Trace-Id", "trace-shared")
	baselineReq.Header.Set("Accept", "application/json")
	baseline := normalizeResults(engine.Compute(baselineReq, stubs))

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				// Every goroutine alternates between the canonical request
				// (used for determinism comparison) and a varying request
				// (used to exercise concurrent body reads / header churn).
				if it%2 == 0 {
					req := httptest.NewRequest("POST", "/api/v1/resource/999?page=1&size=20",
						strings.NewReader(`{"id":7}`))
					req.Header.Set("X-Tenant-Id", "tenant-zzz")
					req.Header.Set("X-Trace-Id", "trace-shared")
					req.Header.Set("Accept", "application/json")

					got := normalizeResults(engine.Compute(req, stubs))
					if !reflect.DeepEqual(got, baseline) {
						t.Errorf("goroutine %d iter %d: non-deterministic result\n got=%+v\nwant=%+v",
							g, it, got, baseline)
						return
					}
				} else {
					body := fmt.Sprintf(`{"id":%d,"g":%d,"i":%d}`, it, g, it)
					req := httptest.NewRequest("POST",
						fmt.Sprintf("/api/v1/resource/%d?page=%d", g, (it%5)+1),
						strings.NewReader(body))
					req.Header.Set("X-Tenant-Id", fmt.Sprintf("tenant-%d", g%7))
					req.Header.Set("X-Trace-Id", fmt.Sprintf("trace-%d-%d", g, it))
					_ = engine.Compute(req, stubs) // result content varies by req
				}
			}
		}()
	}

	wg.Wait()
}

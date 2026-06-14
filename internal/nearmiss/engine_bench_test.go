package nearmiss_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/nearmiss"
)

// BenchmarkCompute_100Stubs measures the per-call cost of Engine.Compute
// against a 100-stub registry that mixes method/path/header/query/body matchers
// (i.e. realistic, not all simple). Stubs are built once before the loop;
// only Compute runs inside the timed region.
//
// Acceptance target (sprint planning, not CI-gated): mean < ~250 µs, p99 < 500
// µs on Apple Silicon dev hardware. Hard ceiling: < 1 ms mean.
//
// Measured baseline (Intel Core i5-5257U @ 2.70GHz, darwin/amd64, go1.26.0,
// -count=5, recorded 2026-06-14):
//
//	BenchmarkCompute_100Stubs-4   	     921	   1196871 ns/op	  570513 B/op	    6752 allocs/op
//	BenchmarkCompute_100Stubs-4   	    1165	   1094190 ns/op	  571260 B/op	    6753 allocs/op
//	BenchmarkCompute_100Stubs-4   	     993	   1190413 ns/op	  569972 B/op	    6752 allocs/op
//	BenchmarkCompute_100Stubs-4   	    1166	   1228885 ns/op	  570902 B/op	    6753 allocs/op
//	BenchmarkCompute_100Stubs-4   	     992	   1397957 ns/op	  570807 B/op	    6753 allocs/op
//
//	Median of 5 runs: ~1.20 ms (1,196,871 ns/op).
//
// This is materially lower than the original T4 reading on the same laptop
// (~2.5–4.5 ms across 3 -count=1 runs in spring 2026). The variance is real
// — this Broadwell host is thermally constrained and background load swings
// per-call cost by 2–4×. Treat the median above as the honest baseline; if a
// future run on the same hardware comes in dramatically higher, suspect
// thermal/load conditions before assuming a code regression.
//
// TODO: Apple Silicon baseline pending — re-run this benchmark on M-series
// hardware and update both this comment and doc.go's "Performance baseline"
// section. PO target was sub-1 ms mean (and ideally < 250 µs) on Apple
// Silicon. Do NOT fabricate numbers; the slot stays empty until measured.
//
// Allocation profile: ~6.7k allocs/op is dominated by stub.BuildMatcher being
// invoked once per stub per Compute (composite + per-header / per-query
// matcher constructors) — not by the engine. The engine adds only the result
// slice (pre-sized) and per-stub DimensionScore entries. If we hoisted matcher
// construction out of Compute (e.g. cache compiled matchers on Registry),
// allocs/op would collapse and ns/op would drop with it; that is a deferred
// optimization tracked separately from the diagnostic engine itself.
func BenchmarkCompute_100Stubs(b *testing.B) {
	// Reuse the realistic builder from the race test, scaled to 100 stubs.
	stubs := buildRealisticStubs()
	stubs = append(stubs, buildRealisticStubs()...) // 100 stubs total
	for i := range stubs {
		// Make IDs unique across both halves.
		if i >= 50 {
			stubs[i].ID = stubs[i].ID + "-b"
			stubs[i].Name = stubs[i].Name + "-b"
		}
	}

	engine := nearmiss.NewEngine(nearmiss.WithTopN(5))

	// One canonical request that misses on most stubs but partially matches
	// many — the "interesting" case for the diagnostic engine.
	body := `{"id":7,"payload":"hello"}`
	req := httptest.NewRequest("POST", "/api/v1/resource/999?page=1&size=20",
		strings.NewReader(body))
	req.Header.Set("X-Tenant-Id", "tenant-zzz")
	req.Header.Set("X-Trace-Id", "trace-shared")
	req.Header.Set("Accept", "application/json")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// httptest.NewRequest re-reads body via strings.NewReader; cheaper to
		// reset the body reader by constructing a fresh request per iteration
		// — but that would inflate measurements with httptest overhead. Body
		// matchers restore req.Body via io.NopCloser, so reusing the request
		// is safe and is what production callers do.
		_ = engine.Compute(req, stubs)
	}
}

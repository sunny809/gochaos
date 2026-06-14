// Package nearmiss implements the diagnostic engine that explains why a
// request did not match any registered stub.
//
// The engine is invoked AFTER primary matching has already concluded that no
// stub fully matched the incoming request. Given a request and a snapshot of
// the registered stubs, it produces a per-stub, per-dimension breakdown of
// what matched, what did not, and why — sorted best-candidate-first so
// callers can surface the most useful explanation.
//
// # Resolved design decisions
//
// The four open questions raised during sprint planning were resolved as
// follows. They are encoded directly in the engine's behaviour and tests; do
// not silently change them.
//
//  1. Empty list on full match (no sentinel field).
//     If a stub fully matches, the engine omits it from the result list.
//     `len(Compute(...)) == 0` therefore means "no near-miss to report" —
//     the registry could be empty, or some stub matched. Callers distinguish
//     using their existing primary-match result. Adding an `ExactMatch` flag
//     would bloat every result row for the common miss case.
//
//  2. Sum of raw matcher scores; expose MaxScore for normalization.
//     Each matcher already encodes its own dimensional weight (method=10,
//     path-exact=30, body-exact=20, query=3, etc.). The engine sums those
//     raw values into NearMissResult.Score and aggregates the per-matcher
//     MaxScore values into NearMissResult.MaxScore. Callers wanting a
//     percent compute Score/MaxScore themselves; the engine never throws
//     away signal by re-normalizing.
//
//  3. Always score every dimension; never short-circuit.
//     The diff is the value of the feature — skipping body when method/path
//     miss strips diagnostic information when the user most needs it. Cost
//     is bounded: the engine is invoked only after a miss, body matchers
//     restore req.Body, and a perf benchmark guards the budget.
//
//  4. Tie-breaker is registry order (priority asc, registration order asc).
//     The engine uses sort.SliceStable on Score descending, so equal-total
//     candidates retain whatever order the caller provided. Registry.List
//     already returns stubs in (priority asc, sequence asc) order, so the
//     engine inherits the right tiebreak for free.
//
// # Concurrency
//
// Engine is stateless after construction and safe for concurrent use. Each
// Compute call operates on its own arguments — Compute holds no locks and
// the caller is responsible for snapshotting the registry (Registry.List
// already copies under RLock).
//
// # Allocation discipline
//
// Compute pre-sizes the result slice with min(len(stubs), topN) capacity,
// references stubs by ID/Name only (no nested copies), and relies on body
// matchers' built-in truncation for Actual fields. The engine never holds
// onto req.Body across calls.
//
// # Performance baseline
//
// Recorded by BenchmarkCompute_100Stubs (see engine_bench_test.go) — kept
// here so future contributors can spot regressions without rerunning the
// benchmark.
//
//	Hardware:    Intel Core i5-5257U @ 2.70GHz (darwin/amd64, Darwin 21.6.0)
//	Go version:  go1.26.0
//	Workload:    100-stub registry (method+path+headers+query+body mix), TopN=5
//	Recorded:    2026-06-14, -count=5 run
//
//	median ns/op: ~1.20 ms (1,196,871 ns/op across 5 runs)
//	B/op:         ~570 KB
//	allocs/op:    ~6753
//
// This median is roughly half the T4-era reading on the same hardware
// (~2.5–4.5 ms). The Broadwell host is thermally constrained and background
// load swings per-call cost by 2–4× between runs; the number above is the
// honest median, not a low-water mark. If a future run on the same machine
// comes in materially worse, suspect thermal/load conditions before assuming
// a code regression.
//
// The mean still exceeds the sprint-planning soft target (mean < 250 µs, p99
// < 500 µs on Apple Silicon) on this 2015-era Broadwell laptop, but is now
// hovering around the 1 ms hard ceiling rather than 2–4× over it.
//
// TODO: Apple Silicon baseline pending — re-run BenchmarkCompute_100Stubs on
// M-series hardware and update this section. PO target was sub-1 ms on Apple
// Silicon (ideally well under 250 µs based on the typical 5–10× speedup vs.
// this CPU). Numbers MUST be measured, not extrapolated.
//
// Alloc count is dominated by stub.BuildMatcher being invoked once per stub
// per Compute — a deferred optimization (cache compiled matchers on the
// registry) would collapse both allocs/op and ns/op together.
package nearmiss

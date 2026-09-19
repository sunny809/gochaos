# Changelog

All notable changes to gmock are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

No changes yet.

## [0.2.0] — 2026-09-19

### Added

- **Fault timeline** — Declare an expected sequence of faults, record which ones
  actually fired, and export fired events to a replayable artifact. Includes
  timeline types in `internal/spec` with public aliases in `pkg/gmock`, a timeline
  runner with validation and per-event counters, `server.TimelineLoad` /
  `server.TimelineReplay` load-and-replay APIs, and Tutorial 4 covering the full
  declare → record → replay → report flow.

- **C1: Async post-response callbacks** — Fire a request to a callback URL after
  the response is sent, with SSRF protection (private-CIDR block) to stop callbacks
  from targeting internal network ranges.

- **Chaos report** — Serialize fault-injection results to JUnit XML and JSON
  (`internal/report`), a new admin `report` endpoint, and the `gmock report` CLI
  command. JSON reports render empty entries as arrays rather than null.

- **Tutorials 1–4** — Step-by-step walkthroughs in `examples/tutorial/`: 10-minute
  chaos experiment, CI-gateable chaos assertion, multi-service correlated failures,
  and fault-timeline replay.

- **BDD test suite** — 47 P2+P3 Gherkin scenarios covering server, stubs, matchers,
  faults, and admin endpoints (`test/bdd/`).

- **Community + GitHub professionalization (G2-G5)** — Public "Why gmock"
  positioning document, issue templates, community outreach plan, and 4 blog post
  drafts (AI sprints, code review, architecture, seedable RNG).

### Changed

- README overhaul — 5-second hook, comparison table, ADR highlights.
- Test harness consolidation — shared `testutil`, poll-based waits, content-keyed
  binary cache, build tags.
- `ARCHITECTURE.md` rewrite and ADR-003 fix; package comments consolidated.
- golangci-lint: 0 issues across the tree.

### Fixed

- Timeline-fired `rate_limit` events are routed through the 429 response path.
- `DelayDefinition` field name corrected; variable redeclaration removed in
  Tutorial 3.
- Code-review findings applied — atomic SSRF bypass check, `sync.Once` CIDR
  computation, template cache, request-body drain, indentation.

## [0.1.1] — 2026-06-27

First tagged release. Covers Sprint 01–06: the core mock server, near-miss
diagnostics, the full chaos depth, observability, the release pipeline, and the
first observability-stack extensions.

### Added

- **D2-D3: K8s liveness + readiness probes** — `GET /__admin/health/live` returns 200
  (simple "alive" check). `GET /__admin/health/ready` returns 200 + stubCount, or 503
  during shutdown. Both endpoints registered under `/__admin/` prefix.

- **D4: Graceful shutdown timeout** — `WithShutdownTimeout(duration)` server option
  controls the maximum wait for in-flight requests during shutdown. Default: 30 seconds.
  When timeout expires, remaining connections are force-closed.

- **M1-M2: Server metrics** — 8 expvar counters tracking requests (total/matched/unmatched),
  faults injected, delays applied, near-miss queries, stub count, and admin operations.
  Exposed via `GET /__admin/metrics` as JSON.

- **M3: Main-path benchmark** — `BenchmarkFullPipeline` measures end-to-end throughput
  with 100 stubs and 100 concurrent goroutines. Target: ≥10K req/sec on 8-core.

- **Prometheus metrics endpoint** — Zero-dependency Prometheus text format export of
  all 8 gmock metrics at `GET /__admin/metrics/prometheus`. Optional user-facing path
  via `WithPrometheusEndpoint("/metrics")`. `WritePrometheus(w io.Writer)` method on
  `Metrics`.

- **Chaos scenario library repository (`gochaos/scenarios`)** — 8 pre-built chaos
  scenarios covering database outages, network degradation, API rate limiting, and
  multi-fault burst patterns. CLI tool `gm scenario` supports `list`, `info`, and
  `export`.

- **Prometheus+Grafana observability stack (`gochaos/prometheus`)** — Grafana overview
  dashboard (7 panels), Docker Compose example (gmock + Prometheus + Grafana), and
  dashboards auto-provisioning.

- **O1: FaultInjectionLog** — Ring buffer (default 1000 entries) records every fault
  injection event with stub ID, fault type, activation mode, request method/path, and
  timestamp. Concurrent-safe; bounded memory.

- **O2: Admin API for fault log** — `GET /__admin/fault-log` lists all recorded fault
  injections; `DELETE /__admin/fault-log` clears the log. Both return JSON with count.

- **O3: VerifyFaultsInjected API** — `server.VerifyFaultsInjected(pattern, count)`
  asserts that faults matching the pattern were injected at least `count` times.
  Pattern fields (`StubID`, `FaultType`, `ActivationMode`) are optional; empty fields
  match all entries. Returns `FaultVerificationResult` with `Matched`, `ActualCount`,
  and `Errors`.

- **Phase 1: Chaos depth + reproducibility (A1-A11)** — Covers 90% of production HTTP
  failure categories with seedable RNG, new fault types, delay distributions, and
  activation modes.

  - **A11: Seedable RNG** — `WithRandSeed(seed int64)` server option makes all chaos
    behavior (delays, fault injection, probabilistic matching) fully deterministic and
    reproducible across runs. Internal `randx` package provides a concurrent-safe `RNG`
    interface wrapping `math/rand` with `sync.Mutex`. Library-only; CLI `--seed` flag
    planned for a future release.
  - **A1: Probability activation** — `activation.probability` field on `FaultDefinition`
    controls the chance a fault fires per request (0.0–1.0). Uses the seedable RNG.
  - **A2: Every Nth request activation** — `activation.everyNthRequest` fires the fault
    on every Nth matching request. Per-stub atomic hit counter ensures correctness under
    concurrent load.
  - **A3: Time-window activation** — `activation.activeBetween` restricts faults to
    specified time windows (ms since server start). Overlapping windows are rejected at
    validation time.
  - **A4: Lognormal delay distribution** — `delay.type: "lognormal"` derives `mu`/`sigma`
    from `p50`, `p95`, `p99` percentiles. Internal `delayx` package.
  - **A5: Malformed response fault** — `fault.type: "malformed"` sends an invalid HTTP
    response (Content-Length mismatch). Hijack-based with fallback to truncated response.
  - **A6: Random data fault** — `fault.type: "random_data"` sends N random binary bytes
    then closes the connection. `dataLength` field (default 256 bytes). Hijack-based with
    fallback to 500 + hex body.
  - **A7: Timeout delay** — `delay.type: "timeout"` blocks until the client disconnects
    or the server shuts down. Tests client-side timeout handling.
  - **A8: Slow close fault** — `fault.type: "slow_close"` sends the complete normal
    response, then holds the connection open for `delayMs` (default 1000) before FIN.
  - **A9: Dribble delay** — `delay.type: "dribble"` sends the body in `chunks` equal
    parts with `totalDuration/chunks` ms between chunks.
  - **A10: Rate limit simulation** — `fault.type: "rate_limit"` uses a token bucket to
    return 429 (or custom `rateLimitStatus`) with `Retry-After` when empty. Supports
    `afterRequests` warm-up and `perSecond` refill.

- **V1: Full-dimension verification** — `Verify()` and `VerifyNotCalled()` now support
  `BodyPattern`, `Headers`, `QueryParams`, `Cookies`, and `URLPathRegex` matching
  against logged requests. `VerificationResult` exposes the asserted pattern.

- **Near-miss diagnostics (P0)** — `POST /__admin/nearmiss` returns per-stub diagnostic
  breakdowns for stubs that nearly matched. 404 responses embed a `nearMisses` array
  with stub ID, name, score, maxScore, and topMissReason.

- **Response templating** — text/template-based response body rendering with custom
  functions (`request`, `randomInt`, `randomUUID`, `now`, `base64`). 100% test coverage.

- **Response delay injection** — `DelayDefinition` in stub responses actually delays the
  response. Supports `fixed` and `random` delay types.

- **Base64 binary response body** — `Base64Body` field on `ResponseDefinition` for
  returning binary content (images, protobuf, PDFs).

- **Redirect response helper** — `WithRedirect(status, location)` for 3xx redirect stubs.

- **Cookie matching** — `Cookies` field on `RequestPattern` supporting exact, `~regex`,
  `*` (any), `!` (absent) patterns.

- **Content negotiation** — `Accept` field on `RequestPattern` with wildcard and quality
  value support.

- **CORS support** — `WithCORSEnabled()` and `WithCORS(opts)` options and `--cors` CLI
  flag.

- **Gzip response compression** — Automatic gzip when the client sends
  `Accept-Encoding: gzip`. Configurable via `WithGzip()`.

- **Go Example functions** — `ExampleServer`, `ExampleServer_verify`,
  `ExampleWithRedirect`, `ExampleWithCORSEnabled` for pkg.go.dev documentation.

- **Documentation** — `near-miss-diagnostics.md`, `go-library-api.md`,
  `fault-injection.md`, `response-delays.md`, `admin-api.md`; sprint process docs in
  `docs/sprints/` with 8 role templates.

### Changed

- README simplified to focus on features; tagline updated to "Fault-Burst Generator
  for Resilience Testing"; chaos-first quick start; comparison table moved below the
  features section.
- Updated `testdata/stubs.yaml` with full-featured example stubs.
- Updated README with documentation for all new HTTP protocol features.

### Fixed

- `DelayDefinition` and `FaultDefinition` structs were defined but never applied in
  the response pipeline — delays are now actively applied.

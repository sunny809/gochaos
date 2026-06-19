# Changelog

All notable changes to gmock are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Phase 1: Chaos depth + reproducibility (A1-A11)** — Covers 90% of
  production HTTP failure categories with seedable RNG, new fault types, delay
  distributions, and activation modes.

  - **A11: Seedable RNG** — `WithRandSeed(seed int64)` server option makes all
    chaos behavior (delays, fault injection, probabilistic matching) fully
    deterministic and reproducible across runs. Internal `randx` package
    provides a concurrent-safe `RNG` interface wrapping `math/rand` with
    `sync.Mutex`. Library-only; CLI `--seed` flag is planned for a future
    release.

  - **A1: Probability activation** — `activation.probability` field on
    `FaultDefinition` controls the chance a fault fires per request (0.0--1.0).
    Uses the seedable RNG for reproducibility.

  - **A2: Every Nth request activation** — `activation.everyNthRequest` field
    fires the fault on every Nth matching request. Per-stub atomic hit counter
    ensures correctness under concurrent load.

  - **A3: Time-window activation** — `activation.activeBetween` field restricts
    faults to specified time windows (ms since server start). Multiple windows
    supported; each window can override the top-level probability. Overlapping
    windows are rejected at validation time.

  - **A7: Timeout delay** — `delay.type: "timeout"` blocks indefinitely until
    the client disconnects or the server shuts down. Tests client-side timeout
    handling.

  - **A5: Malformed response fault** — `fault.type: "malformed"` sends an
    invalid HTTP response (Content-Length mismatch). Hijack-based with fallback
    to truncated response when Hijacker is unavailable.

  - **A6: Random data fault** — `fault.type: "random_data"` sends N random
    binary bytes then closes the connection. Configurable via `dataLength`
    field (default 256 bytes). Hijack-based with fallback to 500 + hex body.

  - **A4: Lognormal delay distribution** — `delay.type: "lognormal"` generates
    delays from a lognormal distribution parameterized by `p50`, `p95`, `p99`
    percentile values. Derives `mu` and `sigma` automatically; uses seedable
    RNG for reproducibility. Internal `delayx` package.

  - **A8: Slow close fault** — `fault.type: "slow_close"` sends the complete
    normal response, then holds the TCP connection open for `delayMs`
    milliseconds (default 1000) before sending FIN. Simulates half-open
    connections.

  - **A9: Dribble delay** — `delay.type: "dribble"` sends the response body in
    `chunks` equal parts with `totalDuration/chunks` ms between each chunk.
    Each chunk is flushed immediately. Client disconnection stops remaining
    chunks.

  - **A10: Rate limit simulation** — `fault.type: "rate_limit"` uses a
    token-bucket algorithm to return 429 (or custom `rateLimitStatus`) with
    `Retry-After` header when the bucket is empty. Supports `afterRequests`
    warm-up phase and `perSecond` refill rate. Per-stub token bucket with
    independent state.

- **New public types**: `Activation`, `TimeWindow` (aliases in `pkg/gmock`)

- **New FaultDefinition fields**: `dataLength`, `delayMs`, `activation`,
  `afterRequests`, `perSecond`, `rateLimitStatus`

- **New DelayDefinition fields**: `p50`, `p95`, `p99`, `chunks`,
  `totalDuration`

- **Advanced chaos documentation**: `docs/features/advanced-chaos.md`

### Changed

- Repo homepage now shows README instead of GitHub conventions.
- README simplified to focus on features, removed internal dev docs from navigation.
- Internal development docs (`docs/`, `.github/README.md`) excluded from public repo.

### Added

- **V1: Full-dimension verification** -- `Verify()` and `VerifyNotCalled()` now support
  `BodyPattern`, `Headers`, `QueryParams`, `Cookies`, and `URLPathRegex` matching
  against logged requests. `VerificationResult` exposes the asserted pattern in
  `BodyPattern`, `HeaderPattern`, and `QueryParamPattern` fields.

- Implemented near-miss diagnostics engine (engine-only; admin endpoint pending in N2).

- **P0: Near-miss admin endpoint + 404 surface** — `POST /__admin/nearmiss` endpoint
  accepts a JSON body (`method`, `path`, `headers`, `body`) and returns per-stub
  diagnostic breakdowns for stubs that nearly matched. 404 responses now embed a
  `nearMisses` array with stub ID, name, score, maxScore, and topMissReason so
  clients can see *which* stubs were close and *why* without calling the admin
  endpoint. 9 unit tests + 9 integration tests (with 3 bad-request subtests)
  cover all mismatch dimensions, edge cases, and error handling.

- **Response templating** — `internal/templating/engine.go` with text/template-based
  response body rendering. Supports custom template functions (`request`, `randomInt`,
  `randomUUID`, `now`, `base64`). 100% test coverage.

- **Response delay injection** — `DelayDefinition` in stub responses now actually delays
  the response. Supports `fixed` and `random` delay types. (#12)

- **Base64 binary response body** — New `Base64Body` field on `ResponseDefinition`
  for returning binary content (images, protobuf, PDFs). (#13)

- **Redirect response helper** — `WithRedirect(status, location)` function for
  creating 3xx redirect stubs without manually setting headers. (#14)

- **Cookie matching** — New `Cookies` field on `RequestPattern` for matching
  requests by cookie name and value. Supports exact, `~regex`, `*` (any), `!` (absent)
  patterns. (#15)

- **Content negotiation** — New `Accept` field on `RequestPattern` for proper
  media type negotiation. Supports wildcards (`*/*`, `type/*`) and quality values. (#16)

- **CORS support** — `WithCORSEnabled()` and `WithCORS(opts)` options for automatic
  CORS preflight handling and response headers. `--cors` flag for CLI. (#17)

- **Gzip response compression** — Automatic gzip compression when the client sends
  `Accept-Encoding: gzip`. Configurable via `WithGzip()`. (#18)

- **Unit tests** — Added comprehensive unit tests for `internal/admin` (18 tests),
  `internal/log` (12 tests), and `config` (11 tests) packages. (#19)

- **Go Example functions** — `ExampleServer`, `ExampleServer_verify`,
  `ExampleWithRedirect`, `ExampleWithCORSEnabled` for pkg.go.dev documentation. (#19)

- **CHANGELOG.md** — Project changelog following Keep a Changelog format.

### Changed

- Updated `testdata/stubs.yaml` with full-featured example stubs
- Updated README with documentation for all new HTTP protocol features

### Fixed

- `DelayDefinition` and `FaultDefinition` structs were defined but never applied
  in the response pipeline — now delays are actively applied

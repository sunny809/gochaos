# Changelog

All notable changes to gmock are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

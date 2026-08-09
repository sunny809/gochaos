# Architecture Overview

This document describes the high-level architecture of `gmock`, a Go-native HTTP mock server and fault-burst generator.

## Design Philosophy

- **Library first**: All core logic lives in `internal/` and `pkg/gmock/`. The CLI (`cmd/gmock/`) is a thin wrapper.
- **Hexagonal / Ports-and-Adapters**: Domain logic (matching, stub registry) is decoupled from transport (HTTP server, CLI).
- **Concurrent safety**: `sync.RWMutex` for all shared state. Separate locks for independent concerns (e.g. `rateLimitMu` vs registry `mu`).
- **Minimal dependencies**: Core library uses only the Go stdlib + `github.com/PaesslerAG/jsonpath`.

## Package Structure

```
gmock/
├── cmd/gmock/           # CLI entry point (cobra)
├── pkg/gmock/           # Public API surface (re-exports from internal/spec via type aliases)
├── config/              # YAML/JSON stub file loading
├── internal/
│   ├── spec/            # Canonical type definitions (StubDefinition, etc.) — no imports from other internal packages
│   ├── stub/            # Stub registry (flat map + RWMutex) + matching engine
│   ├── matcher/         # Individual request matchers (method, path, headers, body, etc.)
│   ├── response/        # HTTP response writer: delays, fault injection, gzip, CORS
│   ├── admin/           # Admin REST API handlers (mounted under /__admin/)
│   ├── callback/        # Async post-response callback dispatch with SSRF protection
│   ├── callbacklog/     # Callback dispatch event log (ring buffer)
│   ├── delayx/          # Delay distribution math (lognormal percentile conversion)
│   ├── faultlog/        # Fault injection event log (ring buffer)
│   ├── log/             # Request log (ring buffer)
│   ├── nearmiss/        # Near-miss diagnostics engine
│   ├── randx/           # Seedable RNG interface + implementations
│   ├── report/          # Chaos evidence export (JUnit XML, JSON)
│   ├── templating/      # Response body template rendering (text/template + custom funcs)
│   └── timeline/        # Fault timeline: ordered, deterministic fault replay
├── test/integration/    # End-to-end tests
└── testdata/            # Fixture files
```

## Dependency Direction

```
internal/spec  ←  internal/*  ←  pkg/gmock  ←  cmd/gmock
  (types)         (logic)       (public API)    (CLI)
```

- `internal/spec` defines all canonical types. Other `internal/*` packages import it directly — never `pkg/gmock` (avoids import cycles).
- `pkg/gmock` re-exports `internal/spec` types via type aliases (`type StubDefinition = spec.StubDefinition`) and wires the internal components into a public `Server` interface.
- `cmd/gmock` depends only on `pkg/gmock` and CLI libraries (cobra, yaml.v3).

## Key Interfaces

### Server (public API)

```go
type Server interface {
    Start() error
    Stop() error
    Stub(def StubDefinition) string
    URL() string
    AdminURL() string
    Verify(pattern RequestPattern, count int) VerificationResult
    VerifyFaultsInjected(pattern FaultPattern, count int) FaultVerificationResult
    VerifyCallbacks(pattern CallbackPattern, count int) CallbackVerificationResult
    RequestLog() []LoggedRequest
    UnmatchedRequests() []LoggedRequest
}
```

Implemented by `pkg/gmock/mockServer`. See [Go Library API](../docs/go-library-api.md) for the full interface.

### Matcher

```go
type Matcher interface {
    Match(req *http.Request) (bool, int)
    String() string
}
```

Each matcher returns `(matched bool, score int)`. Score is used for ranking — the highest-scoring matched stub wins. See [ADR-003](adrs/adr-003-matcher-scoring.md).

### Writer (response transport)

```go
type Writer interface {
    WriteResponse(rw http.ResponseWriter, req *http.Request, resp spec.ResponseDefinition, stubID string) (FaultInjectionInfo, error)
}
```

Implemented by `response.HTTPWriter`. Handles delays, fault injection, gzip, binary bodies, and CORS.

## Request Flow

```
HTTP Request
    |
    v
[Server Handler]  (pkg/gmock/server.go — serveMock)
    |
    v
[Matcher Engine]  (internal/stub — Engine.Match)
    |-- Builds CompositeMatcher from each stub's RequestPattern
    |-- Scores all stubs; highest score wins (ties broken by priority+order)
    |
    v
[Rate Limiter]    (internal/stub — ShouldRateLimit, token bucket)
    |-- If rate-limited: return 429/503 immediately
    |
    v
[Response Writer] (internal/response — HTTPWriter.WriteResponse)
    |-- Apply delay (fixed/random/lognormal/timeout/dribble)
    |-- Apply fault if configured (error/empty/connection_reset/etc.)
    |-- Render template body (internal/templating)
    |-- Compress if gzip accepted
    |-- Write response (or close connection for connection_reset)
    |
    v
[Async Callbacks] (internal/callback — Dispatcher, fire-and-forget goroutine)
    |-- SSRF check (DNS resolution at dispatch time, block private IPs)
    |-- POST to callback URL with rendered body
    |
    v
[Logging]         (internal/log, faultlog, callbacklog — ring buffers)
    |
    v
HTTP Response
```

## Admin API

The admin API (`internal/admin`) is mounted under `/__admin/` by default (configurable via `WithAdminPort` for a separate port):

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/__admin/mappings` | Create a stub |
| `GET` | `/__admin/mappings` | List all stubs |
| `GET` | `/__admin/mappings/{id}` | Get stub by ID |
| `DELETE` | `/__admin/mappings/{id}` | Delete stub by ID |
| `DELETE` | `/__admin/mappings` | Delete all stubs |
| `POST` | `/__admin/reset` | Reset all server state |
| `GET` | `/__admin/requests` | View request log |
| `DELETE` | `/__admin/requests` | Clear request log |
| `GET` | `/__admin/fault-log` | View fault injection log |
| `DELETE` | `/__admin/fault-log` | Clear fault injection log |
| `GET` | `/__admin/callbacks` | View callback dispatch events |
| `DELETE` | `/__admin/callbacks` | Clear callback dispatch log |
| `GET` | `/__admin/health` | Health check |
| `GET` | `/__admin/health/live` | K8s liveness probe |
| `GET` | `/__admin/health/ready` | K8s readiness probe |
| `GET` | `/__admin/metrics` | Server metrics (8 counters) |
| `POST` | `/__admin/nearmiss` | Near-miss diagnostics |
| `GET` | `/__admin/report` | Export chaos evidence (JUnit XML or JSON) |

## Concurrency Model

- **Stub registry**: Flat map guarded by a single `sync.RWMutex`. See [ADR-004](adrs/adr-004-sharded-registry.md) for why sharding was rejected.
- **Rate limiting**: Separate `rateLimitMu` mutex to avoid holding the read lock during token-bucket computation.
- **Request/callback/fault logging**: Ring buffers with atomic indices — lock-free reads.
- **Callback dispatch**: Fire-and-forget goroutines tracked by `sync.WaitGroup` for graceful shutdown. Each goroutine has a `recover()` to catch panics.
- **Template caching**: Bounded cache (100 entries) with `sync.RWMutex`.

## Technology Decisions

| Decision | Choice | Rationale | ADR |
|----------|--------|-----------|-----|
| HTTP Router | `http.ServeMux` (Go 1.22+) | Native pattern matching, zero deps | [ADR-005](adrs/adr-005-stdlib-mux.md) |
| JSONPath | `github.com/PaesslerAG/jsonpath` | Only non-stdlib dep in core | — |
| Config | YAML/JSON + functional options | `gmock.WithPort(8080)`, not struct mutation | [ADR-002](adrs/adr-002-functional-options.md) |
| Logging | `log/slog` | Standard structured logging since Go 1.21 | — |
| CLI | `github.com/spf13/cobra` | Go CLI standard. CLI-only dep. | — |
| Templating | `text/template` not `html/template` | Avoids HTML escaping in JSON responses | [ADR-006](adrs/adr-006-text-template.md) |
| Registry | Flat map + RWMutex | Registry size is small (<10K stubs); sharding adds complexity | [ADR-004](adrs/adr-004-sharded-registry.md) |
| Matcher scoring | `(bool, int)` return | Enables ranking + near-miss diagnostics | [ADR-003](adrs/adr-003-matcher-scoring.md) |

## Security Considerations

- **SSRF protection** (`internal/callback`): DNS resolution at dispatch time (not registration), blocks private/reserved IP ranges (RFC 1918, CGNAT, loopback, link-local). Fail-closed on resolution failure.
- **Admin API**: Same port by default; configurable separate port via `WithAdminPort`. No authentication/authorization in core.
- **Template injection**: `text/template` is sandboxed — no file system or network access from templates.

## Performance Targets

- 1,000+ concurrent connections
- <1ms p99 latency for stub matching (excluding network)
- <50MB RSS at idle

## See Also

- [Package Documentation](README.md) — user-facing feature guides
- [ADRs](adrs/README.md) — architecture decision records
- [Go Library API](../docs/go-library-api.md) — complete public API reference
- [CLI Reference](../docs/cli.md) — all commands and flags

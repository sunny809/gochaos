# Architecture Overview

This document describes the high-level architecture of `gmock`, a Go-native HTTP mock server.

## Design Philosophy

- **Library first**: All core logic lives in `internal/` and `pkg/gmock/`. The CLI (`cmd/gmock/`) is a thin wrapper.
- **Hexagonal / Ports-and-Adapters**: The domain logic (matching, stub registry) is decoupled from transport (HTTP server, CLI).
- **Concurrent safety**: `sync.RWMutex` for all shared state — never a single global lock.
- **Minimal dependencies**: Core library uses only the Go stdlib + `github.com/PaesslerAG/jsonpath`.

## Package Structure

```
gmock/
├── cmd/gmock/           # CLI entry point (cobra)
├── pkg/gmock/           # Public API surface (re-exports from internal/spec)
├── internal/
│   ├── spec/            # Canonical type definitions (StubDefinition, etc.)
│   ├── stub/            # Stub registry + matching engine (sync.RWMutex)
│   ├── matcher/         # Individual request matchers
│   ├── admin/           # Admin REST API handlers
│   ├── server/          # HTTP server lifecycle
│   └── log/             # Request logging (ring buffer)
├── config/              # YAML/JSON stub file loading
├── test/integration/    # End-to-end tests
└── testdata/            # Fixture files
```

## Key Interfaces

### Matcher

```go
type Matcher interface {
    Match(req *http.Request) (bool, int)
    String() string
}
```

Each matcher returns `(matched bool, score int)`. Score is used for ranking and future near-miss analysis.

### StubRepository

```go
type StubRepository interface {
    Add(stub spec.StubDefinition) error
    Remove(id string) error
    Find(req *http.Request) (*spec.StubDefinition, error)
    List() []spec.StubDefinition
    Clear()
}
```

Implemented by `internal/stub/registry.go` using a sync.RWMutex (64 shards, keyed by hash of request signature).

### ResponseDelayer

```go
type ResponseDelayer interface {
    Delay(ctx context.Context, d spec.DelayDefinition) error
}
```

Respects `ctx.Done()` for cancellable delays.

## Request Flow

```
HTTP Request
    |
    v
[Server Handler]
    |
    v
[Matcher Chain]  -- method, path, headers, query, body, cookies
    |
    v
[Stub Registry]  -- sharded lookup, priority-ordered
    |
    v
[Response Builder] -- delay, template (planned), gzip
    |
    v
[Admin Logger]     -- ring buffer, async
    |
    v
HTTP Response
```

## Build Slices

Development proceeds in numbered slices:

- ✅ Slice 1: Core types in `internal/spec` + public aliases
- ✅ Slice 2: Stub registry + matching engine (priority-ordered, concurrent-safe)
- ✅ Slice 3: HTTP server + handler pipeline
- ✅ Slice 4: Admin REST API
- ✅ Slice 5: CLI (cobra)
- ✅ Slice 6: Response templating (`text/template` + custom funcs)
- 🔜 Slice 7: Fault/delay injection
- 🔜 Slice 8: Proxy recording
- 🔜 Slice 9: Stateful scenarios
- 🔜 Slice 10: Near-miss diagnostics
- ✅ Slice 11: Verification API (basic exists, body matching planned)
- 🔜 Slice 12: Hot-reload via fsnotify
- 🔜 Slice 13: Polish + GitHub Actions

## Concurrency Model

- **Mutex-protected stub registry**: `sync.RWMutex` guards all registry operations. Priority-ordered retrieval via insertion sort (O(n) for nearly-sorted data).
- **Request logging**: Lock-free ring buffer using `atomic` indices.
- **Response pooling**: `sync.Pool` for transient response objects to reduce GC pressure.

## Technology Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| HTTP Router | `http.ServeMux` (Go 1.22+) | Native pattern matching, zero deps. Rejected `gorilla/mux`. |
| JSONPath | `github.com/PaesslerAG/jsonpath` | Only non-stdlib dep in core. Pre-evaluated for correctness. |
| Config | YAML/JSON + functional options | `gmock.WithPort(8080)`, not struct mutation. |
| Logging | `log/slog` | Standard structured logging since Go 1.21. |
| CLI | `github.com/spf13/cobra` | Go CLI standard. CLI-only dep. |

## Security Considerations

- Admin API is exposed on the same port by default (configurable via `AdminPort`).
- No authentication/authorization layer in core (planned as middleware, use separate port in production).

## Performance Targets

- 1,000+ concurrent connections
- <1ms p99 latency for stub matching (excluding network)
- <50MB RSS at idle

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) and [GitHub Conventions](../.github/README.md).

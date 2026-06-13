# ADR-002: Functional Options for Server Configuration

## Status

Accepted

## Context

gmock is both a library and a CLI. Library users need a clean, extensible way to configure the server without exposing internal fields. Struct mutation is error-prone and breaks encapsulation. As the server gains more features (CORS, gzip, proxy, delay injection), the configuration surface grows.

We need a pattern that:
- Keeps internals hidden
- Allows backward-compatible additions
- Reads clearly at call sites
- Works well with Go's type system

## Decision

Use the functional options pattern for all server configuration:

```go
server := gmock.NewServer(
    gmock.WithPort(8080),
    gmock.WithCORSEnabled(),
    gmock.WithVerbose(),
)
```

Each option is a function `func(*ServerConfig)` that mutates the configuration after construction but before `Start()`.

Key rules:
- `NewServer` accepts variadic `Option` functions
- `ServerConfig` is unexported (or documented as read-only after construction)
- Options are applied in order; later options override earlier ones
- Default values are provided by `DefaultConfig()`

## Consequences

**Positive:**
- Clean public API with no exposed internals
- Backward-compatible additions (new options don't change `NewServer` signature)
- Self-documenting code at call sites
- Compile-time safety: invalid configurations are impossible to express

**Negative:**
- Slightly more boilerplate (each option is a small function)
- Options are applied in order; order-dependent bugs possible (documented in godoc)
- No compile-time validation of mutually exclusive options (runtime error instead)
- Discoverability is slightly lower than a config struct (IDE autocomplete shows functions, not fields)

## Alternatives Considered

- **Config struct**: Rejected because it exposes fields and encourages zero-value misuse. Users might set `Port: 0` thinking it means "default" when it actually means "random port."
- **Builder pattern**: Rejected because it adds verbosity without clear benefit over functional options in Go
- **Functional options returning error**: Rejected because it complicates the API (`NewServer` would return `(Server, error)`) for a case that rarely fails

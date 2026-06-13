# ADR-001: Hexagonal Architecture

## Status

Accepted

## Context

gmock serves two distinct user personas:
1. **Go developers** who embed the mock server in tests and need a clean, stable library API
2. **DevOps/QA engineers** who run the mock server as a standalone service and interact via CLI or HTTP

These personas have different needs (compile-time stability vs. runtime configurability) but share the same domain logic. Without clear boundaries, internal implementation details leak into the public API, making future refactoring risky and breaking downstream consumers.

Additionally, the project needs to avoid import cycles: the public API package (`pkg/gmock`) must reference canonical types, and internal packages must also reference those same types without creating circular dependencies.

## Decision

Adopt a hexagonal (ports-and-adapters) architecture with three concentric layers:

1. **Core domain** (`internal/spec`) — Canonical type definitions (StubDefinition, RequestPattern, etc.)
2. **Internal implementations** (`internal/stub`, `internal/matcher`, `internal/admin`, etc.) — Business logic that imports `internal/spec` directly
3. **Public API** (`pkg/gmock`) — Re-exports types from `internal/spec` via type aliases, providing a stable surface for external consumers
4. **Adapters** (`cmd/gmock`) — CLI and HTTP adapters that consume the public API

Key rules:
- Internal packages import `internal/spec` directly, never `pkg/gmock`
- `pkg/gmock` re-exports via `type Foo = spec.Foo` aliases (not new types)
- The CLI is a thin wrapper; all domain logic lives in `internal/` and `pkg/gmock/`

## Consequences

**Positive:**
- Clear separation between public API and implementation details
- Internal refactorings do not break external consumers
- No import cycles: `internal/spec` is the root of the dependency graph
- Type aliases ensure the public API and internal types are the same type (no conversion overhead)
- Easy to add new adapters (e.g., a future Web UI) without touching core logic

**Negative:**
- Slightly more packages to navigate
- New types require updates in two places (`internal/spec` and `pkg/gmock`)
- Type aliases can confuse Go doc navigation (clicking a type alias jumps to the underlying type)

## Alternatives Considered

- **Single package with exported/unexported types**: Rejected because it couples public API stability with internal refactoring freedom
- **Separate module for public API**: Rejected because it adds release complexity for marginal benefit
- **Code generation for type re-exports**: Rejected because Go type aliases are sufficient and simpler

# ADR-005: http.ServeMux over gorilla/mux

## Status

Accepted

## Context

gmock needs HTTP routing for both mock requests (wildcard paths) and admin API (fixed paths). Go 1.22 introduced enhanced `http.ServeMux` with pattern matching (`GET /path/{id}`).

## Decision

Use `http.ServeMux` (Go 1.22+) for all routing. Do not add `github.com/gorilla/mux` as a dependency.

## Consequences

**Positive:**
- Zero additional dependencies
- Standard library guarantees (no third-party maintenance risk)
- Pattern syntax is sufficient for gmock's needs

**Negative:**
- No middleware chaining (must implement manually)
- No regex-based path matching (we use `urlPathRegex` in stub config instead)
- Less flexible than gorilla/mux for complex routing

## Alternatives Considered

- `github.com/go-chi/chi`: Rejected — excellent but adds a dependency for marginal benefit
- `github.com/gorilla/mux`: Rejected — mature but effectively unmaintained; adds dependency

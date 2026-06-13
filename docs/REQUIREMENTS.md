# gochaos — Requirements & Product Vision

## One-Line Pitch

> **A Go-native HTTP mock server — embeddable in tests, runnable as a CLI.**

## Problem

Microservice integration testing today has a gap in the Go ecosystem:

- **gock / httpmock**: Intercept at `http.RoundTripper` — not real servers, no admin API, no dynamic responses
- **WireMock**: Java-heavy, thread-pool model limits concurrency, not embeddable in Go tests
- **Toxiproxy**: TCP-level, not HTTP-aware, no stub matching

gochaos fills this gap: a real HTTP server, embeddable in Go tests or runnable standalone, with WireMock-inspired request matching and a full admin API.

## Design Goals

1. **Easy to configure** — Fluent Go API + YAML/JSON stub files + hot-reload (planned)
2. **Easy to manage** — Full RESTful Admin API for runtime stub control
3. **Easy to troubleshoot** — Request log + near-miss diagnostics (planned) explaining why a stub didn't match
4. **Minimal dependencies** — Core library uses only the Go stdlib + a small JSONPath package
5. **Production-quality** — Apache 2.0, race-tested, structured logging

## Shipped Features

- Core stub registry with concurrent-safe CRUD
- Request matching (method, path, header, query, body — exact/regex/JSONPath)
- Cookie matching
- Content negotiation (Accept header)
- Response templating (text/template with custom functions)
- HTTP server with handler pipeline
- Admin REST API (CRUD stubs, view requests, reset)
- Request logging (ring buffer)
- Verification API
- CLI binary (cobra: start, stub, reset, requests)
- YAML/JSON stub file loading
- Priority-ordered matching
- Response delay injection (fixed & random)
- Binary response body (base64)
- Redirect response helper
- CORS support (preflight + headers)
- Gzip response compression

## Future Features (Roadmap)

- Fault injection (connection resets, malformed responses)
- Proxy recording mode
- Stateful scenarios
- Near-miss diagnostics
- Hot-reload of stub files
- Web UI dashboard

## Target Users

- **Go developers** writing integration tests that need a real HTTP mock server
- **CI pipelines** needing a lightweight, reliable mock for non-Go services
- **Teams** that want WireMock-like functionality without the JVM dependency

## Non-Goals

- Not a performance testing tool (use k6, Locust)
- Not a TCP-level proxy (use Toxiproxy)
- Not an API gateway or reverse proxy
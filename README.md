# gochaos

> A lightweight HTTP mock server — runs anywhere, zero dependencies, WireMock-compatible API. Embeddable in Go tests, or run as a standalone CLI for **CI/CD integration testing, cross-language API mocking, and chaos engineering**.

[![Go Reference](https://pkg.go.dev/badge/github.com/sunny809/gochaos.svg)](https://pkg.go.dev/github.com/sunny809/gochaos)
[![Go Report Card](https://goreportcard.com/badge/github.com/sunny809/gochaos)](https://goreportcard.com/report/github.com/sunny809/gochaos)
[![CI](https://github.com/sunny809/gochaos/actions/workflows/ci.yml/badge.svg)](https://github.com/sunny809/gochaos/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/sunny809/gochaos/branch/main/graph/badge.svg)](https://codecov.io/gh/sunny809/gochaos)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Release](https://img.shields.io/github/release/sunny809/gochaos.svg)](https://github.com/sunny809/gochaos/releases/latest)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org/doc/devel/release.html)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue?logo=docker)](https://github.com/sunny809/gochaos/pkgs/container/gochaos)

`gochaos` is a lightweight HTTP/REST mock server inspired by [WireMock](https://wiremock.org/), built natively in Go. Unlike `gock` and `httpmock` which only intercept at the `http.RoundTripper` level, gochaos runs as a **real HTTP server** with a **REST admin API**, **response templating**, **request verification**, **near-miss diagnostics**, and **fault injection** — both as an embeddable Go library and a standalone CLI for **any language team**.

## Comparison

| Feature | gochaos | gock / httpmock | WireMock |
|---------|---------|-----------------|----------|
| **Real HTTP server** | ✅ Yes | ❌ RoundTripper | ✅ Yes (JVM) |
| **Standalone CLI / Docker** | ✅ Yes | ❌ Library only | ✅ Yes (JVM) |
| **Embeddable in Go tests** | ✅ Yes | ✅ Yes | ❌ JVM only |
| **REST admin API** | ✅ Yes | ❌ No | ✅ Yes |
| **Near-miss diagnostics** | ✅ Yes | ❌ No | ✅ Yes |
| **Fault injection** | ✅ Yes | ❌ No | ❌ Limited |
| **Callback/Webhook** | 🔜 Planned | ❌ No | ✅ Yes |
| **Startup time** | ~5ms | ~1ms | ~2-5s |
| **Memory** | ~10MB | ~5MB | ~200-500MB |

## Installation

### CLI Binary

```bash
# Install with go install
go install github.com/sunny809/gochaos/cmd/gmock@latest

# Or download a prebuilt binary from the latest release
# https://github.com/sunny809/gochaos/releases/latest
```

### Go Library

```bash
go get github.com/sunny809/gochaos/pkg/gmock
```

Requires Go 1.22 or newer (uses the enhanced `net/http.ServeMux` pattern matching).

## Quick Start

### As a CLI

```bash
# Install
go install github.com/sunny809/gochaos/cmd/gmock@latest

# Start with stubs from a YAML file
gmock start --port 8080 --stubs ./testdata/stubs.yaml

# In another terminal, use it
curl http://localhost:8080/api/users
#=> {"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}

# Inspect via the admin API
curl http://localhost:8080/__admin/health
curl http://localhost:8080/__admin/mappings
curl http://localhost:8080/__admin/requests

# Or use the CLI client
gmock stub list --admin-url http://localhost:8080
gmock stub create ./new-stub.json --admin-url http://localhost:8080
gmock reset --admin-url http://localhost:8080
```

### As a Go Library

```go
package main

import (
    "fmt"
    "net/http"

    "github.com/sunny809/gochaos/pkg/gmock"
)

func main() {
    // Create and start a server on a random port
    server := gmock.NewServer(gmock.WithPort(0))
    if err := server.Start(); err != nil {
        panic(err)
    }
    defer server.Stop()

    // Register a stub
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodGet,
            URLPath: "/api/users",
        },
        Response: gmock.ResponseDefinition{
            Status:  http.StatusOK,
            Headers: map[string]string{"Content-Type": "application/json"},
            Body:    `{"users":[]}`,
        },
    })

    // Point your code under test at server.URL()
    resp, _ := http.Get(server.URL() + "/api/users")
    // ... assert response

    // Verify the request happened
    result := server.Verify(gmock.RequestPattern{
        Method:  "GET",
        URLPath: "/api/users",
    }, 1)
    if !result.Matched {
        fmt.Printf("expected 1 request, got %d\n", result.ActualCount)
    }
}
```

## Stub File Format

YAML or JSON. Multiple stubs under a `mappings` array:

```yaml
mappings:
  - name: list-users
    request:
      method: GET
      urlPath: /api/users
    response:
      status: 200
      headers:
        Content-Type: application/json
      body: '{"users":[]}'

  - name: get-user-by-id
    request:
      method: GET
      urlPathRegex: ^/api/users/\d+$
    response:
      status: 200
      body: '{"id":1,"name":"Alice"}'
```

## Request Matching

`gochaos` matches requests on multiple dimensions. The best-scoring matching stub wins:

| Dimension | Field | Patterns |
|-----------|-------|----------|
| Method | `request.method` | `GET`, `POST`, etc. (case-insensitive) |
| Path | `request.urlPath` | Exact match |
| Path | `request.urlPathRegex` | Go regex |
| Accept | `request.accept` | Media type negotiation (`application/json`, `*/*`) |
| Headers | `request.headers` | exact, `~regex`, `*` (any), `!` (absent) |
| Cookies | `request.cookies` | Same as headers |
| Query | `request.queryParams` | Same as headers |
| Body | `request.body.exactMatch` | Byte-for-byte match |
| Body | `request.body.regexMatch` | Regex |
| Body | `request.body.jsonPath` | JSONPath expression |

Priority can be set via `priority: 1` on a stub (lower = higher precedence).

## Response Features

### Response Delays

Simulate network latency with fixed, random, lognormal, timeout, or dribble delays:

```yaml
response:
  status: 200
  body: '{"slow": true}'
  delay:
    type: fixed
    value: 2000  # 2 seconds
```

```yaml
response:
  status: 200
  body: '{"realistic": true}'
  delay:
    type: lognormal
    p50: 50
    p95: 200
    p99: 500
```

```go
server.Stub(gmock.StubDefinition{
    Response: gmock.ResponseDefinition{
        Status: 200,
        Body: "delayed",
        Delay: &gmock.DelayDefinition{Type: "random", Min: 100, Max: 500},
    },
})
```

### Binary Responses (Base64)

Return binary content like images or protobuf using `base64Body`:

```yaml
response:
  status: 200
  headers:
    Content-Type: image/png
  base64Body: "iVBORw0KGgoAAAANSUhEUg..."
```

### Redirect Responses

Create redirect stubs with the `WithRedirect` helper:

```go
server.Stub(gmock.StubDefinition{
    Request:  gmock.RequestPattern{Method: "GET", URLPath: "/old-path"},
    Response: gmock.WithRedirect(http.StatusMovedPermanently, "/new-path"),
})
```

### CORS Support

Enable CORS with default permissive settings or customize:

```go
// Quick enable (allows all origins)
server := gmock.NewServer(gmock.WithCORSEnabled())

// Custom CORS configuration
server := gmock.NewServer(gmock.WithCORS(gmock.CORSOptions{
    AllowedOrigins: []string{"https://myapp.com"},
    AllowedMethods: []string{"GET", "POST"},
    AllowedHeaders: []string{"Content-Type", "Authorization"},
    AllowCredentials: true,
    MaxAge: 3600,
}))
```

CLI: `gmock start --cors`

### Gzip Compression

Response bodies are automatically compressed when the client sends `Accept-Encoding: gzip`. Disable with:

```go
server := gmock.NewServer(gmock.WithGzip(false))
```

## Admin API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/__admin/mappings` | Create stub |
| `GET` | `/__admin/mappings` | List all stubs |
| `GET` | `/__admin/mappings/{id}` | Get stub by ID |
| `DELETE` | `/__admin/mappings/{id}` | Delete stub by ID |
| `DELETE` | `/__admin/mappings` | Delete all stubs |
| `POST` | `/__admin/reset` | Reset all server state |
| `GET` | `/__admin/requests` | View request log (filter: `matched`, `unmatched`) |
| `DELETE` | `/__admin/requests` | Clear request log |
| `GET` | `/__admin/health` | Health check |

## Documentation

| For | Document |
|-----|----------|
| 🚀 **Getting Started** | [Feature Overview](docs/features/getting-started.md) |
| 📖 **CLI Reference** | [docs/cli.md](docs/cli.md) — all commands and flags |
| 🌐 **Admin API** | [docs/admin-api.md](docs/admin-api.md) — REST API with curl examples |
| 🎯 **Stub Matching** | [Feature Guide](docs/features/stub-matching.md) — 8 matching dimensions |
| ⏱️ **Response Delays** | [Feature Guide](docs/features/response-delays.md) — fixed and random delays |
| 💥 **Fault Injection** | [Feature Guide](docs/features/fault-injection.md) — error, empty, connection reset |
| 🔥 **Advanced Chaos** | [Feature Guide](docs/features/advanced-chaos.md) — 7 fault types, 5 delay distributions, activation modes, seedable RNG |
| 🔄 **Response Templating** | [Feature Guide](docs/features/response-templating.md) — dynamic responses |
| 🌍 **CORS** | [Feature Guide](docs/features/cors.md) — cross-origin support |
| ✅ **Verification** | [Feature Guide](docs/features/verification.md) — request assertions |
| 📦 **YAML/JSON Stubs** | [Feature Guide](docs/features/yaml-stubs.md) — file-based stubs |
| 🔐 **Gzip** | [Feature Guide](docs/features/gzip-compression.md) — compression support |
| 🧪 **Examples** | [examples/](examples/) — runnable Go examples |
| 🚀 **CI/CD** | [docs/ci-integration.md](docs/ci-integration.md) — GitHub Actions, GitLab CI, Jenkins |

## CI/CD & Cross-Language Usage

`gochaos` is uniquely suited for CI/CD pipelines because of its minimal resource footprint:

| Metric | gochaos | WireMock (JVM) | Benefit for CI |
|--------|---------|----------------|----------------|
| Startup time | ~5ms | ~2-5s | Pipeline starts faster |
| Memory | ~10MB | ~200-500MB | Run more parallel jobs |
| Image size | ~15MB (scratch) | ~200MB+ (JDK) | Pull in < 1s |
| Dependencies | None (single binary) | JDK 17+ | No runtime to install |

### Use from any language

```bash
# Start with Docker (no Go installation needed)
docker run --rm -p 8080:8080 ghcr.io/sunny809/gochaos:latest --stubs ./stubs.yaml

# Manage via REST API (from any language)
curl -s http://localhost:8080/__admin/health | jq .
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/health"},"response":{"status":200,"body":"{\"status\":\"ok\"}"}}'

# Verify requests
curl http://localhost:8080/__admin/requests | jq .
```

### CI examples

**GitHub Actions** — [See full guide](docs/ci-integration.md):
```yaml
services:
  gochaos:
    image: ghcr.io/sunny809/gochaos:latest
    ports: ["8080:8080"]
```

**GitLab CI**:
```yaml
services:
  - name: ghcr.io/sunny809/gochaos:latest
    alias: mock-server
```

**Jenkins** — [See full guide](docs/ci-integration.md)

---

## Features

- Concurrent-safe stub registry with priority-ordered matching
- 8-dimensional request matching (method, path, headers, query, body, cookies, accept)
- Response templating with `text/template` (`{{.Request.Method}}`, `{{randomUUID}}`, `{{randomInt}}`, `{{now}}`)
- **7 fault injection types**: `error` (500), `empty` (0-byte), `connection_reset` (TCP RST), `malformed` (invalid HTTP), `random_data` (garbage bytes + close), `slow_close` (delayed FIN), `rate_limit` (token bucket + 429/503)
- **5 delay distributions**: `fixed`, `random`, `timeout` (infinite hang), `lognormal` (p50/p95/p99 parameterized), `dribble` (chunked body with inter-chunk delays)
- **3 activation modes** (AND semantics): `probability` (0.0--1.0), `everyNthRequest`, `activeBetween` (time windows with per-window probability override)
- **Seedable RNG**: `WithRandSeed(42)` makes all chaos behavior reproducible across runs
- Binary response body (base64)
- Redirect response helper
- CORS support (preflight + actual requests)
- Gzip response compression
- Request logging with ring buffer (default 1000 entries)
- Request verification API (Verify, VerifyNotCalled)
- Near-miss diagnostics (unmatched request -> closest stub + per-dimension why)
- YAML/JSON stub file loading

## Building & Testing

```bash
# Build
go build ./...

# Run all tests with race detection
go test -race ./...

# Build the CLI binary
go build -o gmock ./cmd/gmock
```

## License

Apache 2.0 — see [LICENSE](LICENSE).

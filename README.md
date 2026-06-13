# gochaos

> A Go-native HTTP mock server — embeddable in tests, runnable as a CLI

[![Go Reference](https://pkg.go.dev/badge/github.com/sunny809/gochaos.svg)](https://pkg.go.dev/github.com/sunny809/gochaos)
[![Go Report Card](https://goreportcard.com/badge/github.com/sunny809/gochaos)](https://goreportcard.com/report/github.com/sunny809/gochaos)
[![CI](https://github.com/sunny809/gochaos/actions/workflows/ci.yml/badge.svg)](https://github.com/sunny809/gochaos/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/sunny809/gochaos/branch/main/graph/badge.svg)](https://codecov.io/gh/sunny809/gochaos)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Release](https://img.shields.io/github/release/sunny809/gochaos.svg)](https://github.com/sunny809/gochaos/releases/latest)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org/doc/devel/release.html)

`gochaos` is a lightweight HTTP/REST mock server inspired by [WireMock](https://wiremock.org/), built natively in Go. It fills a gap in the Go ecosystem: existing solutions like `gock` and `httpmock` only intercept at the `http.RoundTripper` level — they're not real servers, have no admin API, no dynamic responses. `gochaos` is both an **embeddable library** for Go tests and a **standalone CLI binary** for CI pipelines and non-Go teams.

## Table of Contents

- [Design Goals](#design-goals)
- [Installation](#installation)
- [Quick Start](#quick-start)
  - [As a CLI](#as-a-cli)
  - [As a Go Library](#as-a-go-library)
- [Stub File Format](#stub-file-format)
- [Request Matching](#request-matching)
- [Response Features](#response-features)
  - [Response Delays](#response-delays)
  - [Binary Responses (Base64)](#binary-responses-base64)
  - [Redirect Responses](#redirect-responses)
  - [CORS Support](#cors-support)
  - [Gzip Compression](#gzip-compression)
- [Admin API](#admin-api)
- [Roadmap](#roadmap)
- [Project Layout](#project-layout)
- [Building & Testing](#building--testing)
- [Contributing](#contributing)
- [License](#license)

## Design Goals

- **Easy to configure** — Fluent Go API + YAML/JSON stub files + (planned) hot-reload
- **Easy to manage** — Full RESTful Admin API for runtime stub control
- **Easy to troubleshoot** — Request log + (planned) near-miss diagnostics explaining *why* a stub didn't match
- **Minimal dependencies** — Core library uses only the Go stdlib + a small JSONPath package
- **Production-ready open-source standards** — Apache 2.0, race-tested, structured logging

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

Simulate network latency with fixed or random delays:

```yaml
response:
  status: 200
  body: '{"slow": true}'
  delay:
    type: fixed
    value: 2000  # 2 seconds
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

## Roadmap

Built incrementally as "build slices" for fast feedback. **Currently shipped:**

- Core stub registry with concurrent-safe CRUD
- Request matching (method, path, header, query, body — exact/regex/JSONPath)
- Cookie matching
- Content negotiation (Accept header)
- HTTP server with handler pipeline
- Admin REST API
- Request logging (ring buffer)
- Verification API
- CLI binary (start, stub, reset, requests)
- YAML/JSON stub file loading
- Priority-ordered matching
- Response delay injection (fixed & random)
- Binary response body (base64)
- Redirect response helper
- CORS support (preflight + headers)
- Gzip response compression

**Planned:**

- Fault injection (connection resets, malformed responses)
- Proxy recording mode
- Stateful scenarios
- Near-miss diagnostics
- Hot-reload of stub files
- Web UI dashboard

## Project Layout

```
gmock/
├── cmd/gmock/           # CLI entry point (cobra)
├── pkg/gmock/           # Public library API
├── internal/
│   ├── spec/            # Canonical type definitions
│   ├── stub/            # Stub registry + matching engine
│   ├── matcher/         # Individual request matchers
│   ├── admin/           # Admin API handlers
│   ├── log/             # Request logging (ring buffer)
│   ├── server/          # (reserved for future server abstractions)
│   ├── templating/      # (Slice 6)
│   ├── fault/           # (Slice 7)
│   ├── proxy/           # (Slice 8)
│   ├── scenario/        # (Slice 9)
│   └── nearmiss/        # (Slice 10)
├── config/              # Stub file loading (YAML/JSON)
├── test/integration/    # End-to-end tests
├── testdata/            # Test fixture files
├── examples/            # Runnable examples
└── docs/                # Documentation
```

## Building & Testing

```bash
# Build
go build ./...

# Run all tests with race detection
go test -race ./...

# Build the CLI binary
go build -o gmock ./cmd/gmock

# Or use the Makefile
make test
make build
make ci
```

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache 2.0 — see [LICENSE](LICENSE).

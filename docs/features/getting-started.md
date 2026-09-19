# Getting Started

> gochaos is a Go-native HTTP mock server. Use it as an embedded library in Go tests,
> or as a standalone CLI server for cross-language integration testing.

## Library Mode (Embedded in Go Tests)

```go
package my_test

import (
    "net/http"
    "testing"

    "github.com/sunny809/gochaos/pkg/gmock"
)

func TestWithMockServer(t *testing.T) {
    // 1. Create a server on a random port
    server := gmock.NewServer(gmock.WithPort(0))
    if err := server.Start(); err != nil {
        t.Fatal(err)
    }
    defer server.Stop()

    // 2. Register a stub
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodGet,
            URLPath: "/api/hello",
        },
        Response: gmock.ResponseDefinition{
            Status: http.StatusOK,
            Body:   `{"message":"Hello, World!"}`,
        },
    })

    // 3. Make a real HTTP request to the mock
    resp, err := http.Get(server.URL() + "/api/hello")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    // 4. Verify the response
    if resp.StatusCode != http.StatusOK {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}
```

**Key concepts:**

| Concept | Explanation |
|---------|-------------|
| `gmock.WithPort(0)` | Port 0 = random available port (essential for tests) |
| `server.URL()` | Returns the base URL like `http://127.0.0.1:54321` |
| `server.Stub()` | Registers a stub and returns its UUID |
| Real HTTP | No RoundTripper interception — real TCP connections |

## CLI Mode (Standalone Server)

```bash
# Start server
gmock start --port 8080

# Register a stub via Admin API
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/hello"},"response":{"status":200,"body":"Hello!"}}'

# Test it
curl http://localhost:8080/api/hello
```

## Full Example

See [examples/basic/](../examples/basic/main.go) for a complete, runnable example.

## Next Steps

- [Stub Matching](stub-matching.md) — Request pattern matching in detail
- [CLI Reference](../cli.md) — All CLI commands and flags
- [Admin API](../admin-api.md) — REST API reference
- [Response Templating](response-templating.md) — Dynamic response generation

# Go Library API Reference

> gochaos exposes a clean Go API from the `gmock` package (`github.com/sunny809/gochaos/pkg/gmock`).
> This reference covers all exported types, constructors, and options.

---

## Quick Start

```go
import "github.com/sunny809/gochaos/pkg/gmock"

server := gmock.NewServer(gmock.WithPort(0))
server.Start()
defer server.Stop()

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

resp, _ := http.Get(server.URL() + "/api/hello")
```

---

## Server Creation

### NewServer

```go
func NewServer(opts ...Option) *Server
```

Creates a new mock server instance. The server is not started until `.Start()` is called.
Options configure the server — see [Options Reference](#options-reference) below.

### Server Methods

#### Start

```go
func (s *Server) Start() error
```

Starts the HTTP server. Returns an error if the port is already in use.
After starting, `s.URL()` returns the base URL.

#### Stop

```go
func (s *Server) Stop()
```

Stops the server gracefully. Equivalent to `Shutdown()` with the default timeout.

#### Shutdown

```go
func (s *Server) Shutdown() error
```

Shuts down the server with the configured shutdown timeout (default 30s).
In-flight requests are given time to complete before force-close.

#### URL

```go
func (s *Server) URL() string
```

Returns the base URL of the running server (e.g., `http://127.0.0.1:54321`).
Panics if called before `Start()`.

#### Stub

```go
func (s *Server) Stub(def StubDefinition) (string, error)
```

Registers a stub definition. Returns the stub's UUID and any validation error.

#### RemoveStub

```go
func (s *Server) RemoveStub(id string) error
```

Removes a registered stub by UUID. Returns an error if the stub doesn't exist.

#### RemoveAllStubs

```go
func (s *Server) RemoveAllStubs()
```

Removes all registered stubs.

#### Reset

```go
func (s *Server) Reset()
```

Resets the server to its initial state: removes all stubs, clears request log,
clears fault log, resets all counters.

---

## Type Definitions

### StubDefinition

```go
type StubDefinition struct {
    Name     string
    Priority int
    Request  RequestPattern
    Response ResponseDefinition
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Name` | string | ❌ | Optional human-readable name for debugging |
| `Priority` | int | ❌ | Lower values = higher priority. Default 0. |
| `Request` | RequestPattern | ✅ | The request pattern to match against |
| `Response` | ResponseDefinition | ✅ | The response to return when matched |

### RequestPattern

```go
type RequestPattern struct {
    Method         string
    URLPath        string
    URLPathRegex   string
    Headers        map[string]string
    QueryParams    map[string]string
    Cookies        map[string]string
    Body           *BodyPattern
    Accept         string
}
```

| Field | Type | Description |
|-------|------|-------------|
| `Method` | string | HTTP method: GET, POST, PUT, DELETE, etc. Empty = wildcard |
| `URLPath` | string | Exact URL path match |
| `URLPathRegex` | string | Regex URL path match (compiled at registration time) |
| `Headers` | map[string]string | Header name → regex value pattern |
| `QueryParams` | map[string]string | Query param name → regex value pattern |
| `Cookies` | map[string]string | Cookie name → regex value pattern |
| `Body` | *BodyPattern | Body matching — exact, regex, or JSONPath |
| `Accept` | string | Media type negotiation |

### BodyPattern

```go
type BodyPattern struct {
    ExactMatch string
    RegexMatch string
    JSONPath   string
}
```

Choose one strategy:

| Field | Strategy | Example |
|-------|----------|---------|
| `ExactMatch` | Body must be byte-for-byte identical | `{"name":"Alice"}` |
| `RegexMatch` | Body must match regex | `"name":"[A-Z][a-z]+"` |
| `JSONPath` | Body must satisfy JSONPath expression | `$.users[?(@.age > 18)]` |

### ResponseDefinition

```go
type ResponseDefinition struct {
    Status            int
    Headers           map[string]string
    Body              string
    BodyFile          string
    BodyBase64        string
    Fault             *FaultDefinition
    Delay             *DelayDefinition
    TransformResponse bool
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Status` | int | 200 | HTTP status code |
| `Headers` | map[string]string | — | Response headers |
| `Body` | string | — | Response body string |
| `BodyFile` | string | — | Path to response body file (read at registration time) |
| `BodyBase64` | string | — | Base64-encoded binary body (decoded at response time) |
| `Fault` | *FaultDefinition | nil | Fault injection configuration |
| `Delay` | *DelayDefinition | nil | Response delay configuration |
| `TransformResponse` | bool | false | Enable response templating via text/template |

### FaultDefinition

```go
type FaultDefinition struct {
    Type       string
    Config     map[string]any
    Activation *Activation
}
```

| Field | Type | Description |
|-------|------|-------------|
| `Type` | string | Fault type: `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit` |
| `Config` | map[string]any | Type-specific configuration (e.g., `size` for random_data, `capacity` for rate_limit) |
| `Activation` | *Activation | When the fault fires — probability, Nth-request, or time-window |

See [Fault Injection](features/fault-injection.md) for all 7 types.
See [Advanced Chaos](features/advanced-chaos.md) for activation modes and configuration.

### DelayDefinition

```go
type DelayDefinition struct {
    Type      string
    Value     int
    Min       int
    Max       int
    P50       int
    P95       int
    P99       int
    ChunkSize int
    Interval  int
}
```

| Field | Type | Applies to | Description |
|-------|------|-----------|-------------|
| `Type` | string | all | Delay type: `fixed`, `random`, `lognormal`, `timeout`, `dribble` |
| `Value` | int | `fixed` | Delay in milliseconds |
| `Min` | int | `random` | Minimum delay in milliseconds |
| `Max` | int | `random` | Maximum delay in milliseconds |
| `P50` | int | `lognormal` | Median latency in milliseconds |
| `P95` | int | `lognormal` | 95th percentile in milliseconds |
| `P99` | int | `lognormal` | 99th percentile in milliseconds |
| `ChunkSize` | int | `dribble` | Bytes per chunk |
| `Interval` | int | `dribble` | Milliseconds between chunks |

See [Response Delays](features/response-delays.md) for usage examples.

### Activation

```go
type Activation struct {
    Probability      float64
    EveryNthRequest  int
    ActiveBetween    *TimeWindow
}
```

| Field | Type | Description |
|-------|------|-------------|
| `Probability` | float64 | Fault fires with this probability (0.0–1.0). 0 = never, 1 = always |
| `EveryNthRequest` | int | Fault fires every N requests (e.g., 3 = fires on 3rd, 6th, 9th) |
| `ActiveBetween` | *TimeWindow | Fault fires only within this time window |

All modes can be combined (AND logic — ALL must pass).

### TimeWindow

```go
type TimeWindow struct {
    StartMs int64
    EndMs   int64
}
```

| Field | Type | Description |
|-------|------|-------------|
| `StartMs` | int64 | Window start in milliseconds since Unix epoch |
| `EndMs` | int64 | Window end in milliseconds since Unix epoch |

---

## Verification API

### Verify

```go
func (s *Server) Verify(pattern RequestPattern, count int) VerificationResult
```

Asserts that at least `count` requests matching `pattern` were received.

### VerifyNotCalled

```go
func (s *Server) VerifyNotCalled(pattern RequestPattern) VerificationResult
```

Asserts that NO request matching `pattern` was received.

### VerificationResult

```go
type VerificationResult struct {
    ExpectedCount int
    ActualCount   int
    Matched       bool
    Errors        []string
}
```

### VerifyFaultsInjected

```go
func (s *Server) VerifyFaultsInjected(pattern FaultPattern, minCount int) (int, bool, error)
```

Checks that at least `minCount` faults matching `pattern` were injected.
Returns `(actualCount, actualCount >= minCount, error)`.

### FaultPattern

```go
type FaultPattern struct {
    StubID         string
    FaultType      string
    ActivationMode string
}
```

All fields are optional — empty fields match all entries.

See [Verification & Request Log](features/verification.md) for usage examples.

---

## Request Log API

```go
func (s *Server) RequestLog() []LoggedRequest
func (s *Server) UnmatchedRequests() []LoggedRequest
func (s *Server) ClearRequestLog()
```

### LoggedRequest

```go
type LoggedRequest struct {
    Method      string
    Path        string
    QueryString string
    Headers     map[string][]string
    Body        string
    ReceivedAt  time.Time
}
```

---

## Near-Miss API

```go
func (s *Server) ComputeNearMiss(pattern RequestPattern) (*NearMissResult, error)
```

Compares a request pattern against all registered stubs and returns the near-miss breakdown.

### NearMissResult / NearMissEntry / ScoreBreakdown

```go
type NearMissResult struct {
    NearMisses []NearMissEntry
}

type NearMissEntry struct {
    StubID          string
    Name            string
    Priority        int
    ScoreBreakdown  map[string]DimensionScore
    TotalScore      int
    MaxPossibleScore int
    MatchingRatio   float64
}

type DimensionScore struct {
    Score     int
    MaxScore  int
    Matched   bool
    Expected  string
    Actual    string
}
```

See [Near-Miss Diagnostics](features/near-miss-diagnostics.md) for usage examples.

---

## Options Reference

Configure the server at creation time via functional options:

```go
server := gmock.NewServer(
    gmock.WithPort(8080),
    gmock.WithRandSeed(42),
    gmock.WithMaxRequests(5000),
)
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithPort(port)` | int | 8080 | Server port. Use 0 for random (recommended in tests) |
| `WithRandSeed(seed)` | int64 | 0 (crypto/rand) | Seed for deterministic RNG. Same seed → same fault sequence |
| `WithMaxRequests(n)` | int | 1000 | Request log ring buffer capacity |
| `WithShutdownTimeout(d)` | time.Duration | 30s | Max time to wait for in-flight requests during shutdown |
| `WithAdminPort(port)` | int | — | Separate port for the admin API. If omitted, shares main port |
| `WithPrometheusEndpoint(path)` | string | — | Enable Prometheus metrics at the given path (e.g., `/metrics`). Always available at `/__admin/metrics/prometheus` regardless. |

---

## Complete API Surface Summary

| Category | Functions/Methods |
|----------|------------------|
| **Server lifecycle** | `NewServer`, `Start`, `Stop`, `Shutdown`, `URL` |
| **Stub management** | `Stub`, `RemoveStub`, `RemoveAllStubs`, `Reset` |
| **Request log** | `RequestLog`, `UnmatchedRequests`, `ClearRequestLog` |
| **Verification** | `Verify`, `VerifyNotCalled`, `VerifyFaultsInjected` |
| **Fault log** | `FaultLog`, `ClearFaultLog` |
| **Near-miss** | `ComputeNearMiss` |
| **Options** | `WithPort`, `WithRandSeed`, `WithMaxRequests`, `WithShutdownTimeout`, `WithAdminPort`, `WithPrometheusEndpoint` |

---

## Import Path

```
github.com/sunny809/gochaos/pkg/gmock
```

Minimum Go version: **1.22** (uses enhanced `http.ServeMux`).
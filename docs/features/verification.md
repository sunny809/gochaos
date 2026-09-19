# Verification & Request Log

> gochaos records every incoming request in a ring buffer log. Use the verification
> API to assert expected requests in tests, or inspect the log for debugging.

## Request Verification API

### Verify — Assert Minimum Count

Check that at least `count` requests matching the pattern were received.

```go
result := server.Verify(gmock.RequestPattern{
    Method:  "GET",
    URLPath: "/api/users/alice",
}, 1)

if !result.Matched {
    t.Errorf("expected at least 1 matching request, got %d", result.ActualCount)
}
```

### VerifyNotCalled — Assert Zero Count

Check that NO request matching the pattern was received.

```go
result := server.VerifyNotCalled(gmock.RequestPattern{
    Method:  "DELETE",
    URLPath: "/api/users/alice",
})
if !result.Matched {
    t.Errorf("expected DELETE to not be called, but it was")
}
```

### Verification Pattern

The verification pattern checks method and path (exact or regex):

```go
type VerificationPattern struct {
    Method       string
    URLPath      string
    URLPathRegex string
}
```

Verification does NOT check headers, body, or query params (future enhancement).

### VerificationResult

```go
type VerificationResult struct {
    ExpectedCount int      // The count you asserted
    ActualCount   int      // What was actually received
    Matched       bool     // Did the assertion pass?
    Errors        []string // Human-readable errors on failure
}
```

## Fault Injection Verification API

When using probabilistic, Nth-request, or time-window fault activation, you need
to verify that faults actually fired. The fault log records every fault injection
event for CI-gateable assertions.

### VerifyFaultsInjected — Assert Fault Count

Check that at least `count` faults matching the pattern were injected.

```go
// Verify at least 2 connection_reset faults were injected
result := server.VerifyFaultsInjected(gmock.FaultPattern{
    FaultType: "connection_reset",
}, 2)

if !result.Matched {
    t.Errorf("expected at least 2 faults, got %d", result.ActualCount)
}
```

### FaultPattern Structure

Pattern fields are optional — empty fields match all entries.

```go
type FaultPattern struct {
    StubID         string // Match by stub ID (optional)
    FaultType      string // Match by fault type: "error", "connection_reset", "timeout", etc.
    ActivationMode string // Match by mode: "always", "probability", "nth_request", "time_window"
}
```

### FaultVerificationResult

```go
type FaultVerificationResult struct {
    ExpectedCount int          // The count you asserted
    ActualCount   int          // What was actually injected
    Matched       bool         // Did the assertion pass?
    Errors        []string     // Human-readable errors on failure
    Pattern       FaultPattern // The pattern used for verification
}
```

### Example: CI Gate for Fault Injection

```go
func TestCircuitBreakerTrip(t *testing.T) {
    server := gmock.NewServer(gmock.WithPort(0))
    server.Start()
    defer server.Stop()

    // Stub with 50% probability connection reset
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{Method: "GET", URLPath: "/api/downstream"},
        Response: gmock.ResponseDefinition{Status: 200, Body: `{"ok":true}`},
        Fault: &gmock.FaultDefinition{
            Type: "connection_reset",
            Activation: &gmock.Activation{
                Probability: 0.5,
            },
        },
    })

    // Run 100 requests through your circuit breaker
    for i := 0; i < 100; i++ {
        resp, _ := http.Get(server.URL() + "/api/downstream")
        // ... your SUT's resilience logic ...
    }

    // CI assertion: at least 10 faults must have fired
    result := server.VerifyFaultsInjected(gmock.FaultPattern{
        FaultType: "connection_reset",
    }, 10)

    if !result.Matched {
        t.Errorf("circuit breaker test invalid: expected >=10 faults, got %d",
            result.ActualCount)
    }
}
```

### Why Fault Verification Matters

Unlike WireMock where faults are always-on, gochaos faults are **conditional**:
- `probability: 0.5` — fires 50% of the time
- `everyNthRequest: 5` — fires every 5th request
- `activeBetween` — fires only during specific time windows

Without fault verification, a test could pass with zero faults injected (e.g.,
RNG never triggered probability). `VerifyFaultsInjected` makes chaos tests
CI-gateable and reproducible.

## Fault Injection Log

### Admin API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/__admin/fault-log` | List all fault injection events |
| `DELETE` | `/__admin/fault-log` | Clear fault injection log |

### Response Format

```bash
# List fault log
curl http://localhost:8080/__admin/fault-log
#=> {"entries":[...], "count":2}

# Clear fault log
curl -X DELETE http://localhost:8080/__admin/fault-log
#=> {"cleared":true, "count":2}
```

Each entry in the fault log contains:

```json
{
  "stubId": "stub-123",
  "faultType": "connection_reset",
  "activatedAt": "2026-06-20T10:30:45Z",
  "requestMethod": "GET",
  "requestPath": "/api/downstream",
  "activationMode": "probability"
}
```

### Max Entries

The fault log is a fixed-size ring buffer. Default: 1000 entries (same as request log).

```go
server := gmock.NewServer(gmock.WithMaxRequests(5000))
```

When full, oldest entries are overwritten.

## Request Log

### Access the Log

```go
// All logged requests (oldest first)
allLogs := server.RequestLog()

// Only unmatched requests
unmatched := server.UnmatchedRequests()
```

### LoggedRequest Structure

```go
type LoggedRequest struct {
    Method      string     // "GET", "POST", etc.
    Path        string     // "/api/users"
    QueryString string     // "page=1&sort=name"
    Headers     HeadersMap // Map of header name → values
    Body        string     // Request body (best-effort capture)
    ReceivedAt  time.Time  // Timestamp in UTC
}
```

### Max Requests

The request log is a fixed-size ring buffer. Default: 1000 entries.

```go
// Configure at server creation
server := gmock.NewServer(
    gmocks.WithPort(0),
    gmocks.WithMaxRequests(5000),
)
```

When the buffer is full, the oldest entries are overwritten.

## Testing Patterns

### Pattern 1: Assert Request Was Made

```go
func TestUserCreated(t *testing.T) {
    server := gmock.NewServer(gmock.WithPort(0))
    server.Start()
    defer server.Stop()

    // Register stub
    server.Stub(gmock.StubDefinition{
        Request:  gmock.RequestPattern{Method: "POST", URLPath: "/api/users"},
        Response: gmock.ResponseDefinition{Status: 201, Body: `{"id":1}`},
    })

    // Make request
    http.Post(server.URL()+"/api/users", "application/json",
        strings.NewReader(`{"name":"Alice"}`))

    // Verify
    result := server.Verify(gmock.RequestPattern{
        Method:  "POST",
        URLPath: "/api/users",
    }, 1)
    if !result.Matched {
        t.Fatal("expected POST /api/users")
    }
}
```

### Pattern 2: Assert Request Was NOT Made

```go
func TestDeleteNotCalled(t *testing.T) {
    // ... setup ...

    result := server.VerifyNotCalled(gmock.RequestPattern{
        Method:  "DELETE",
        URLPath: "/api/users/alice",
    })
    if !result.Matched {
        t.Error("DELETE should not have been called")
    }
}
```

### Pattern 3: Inspect Request Details

```go
func TestRequestHeadersLogged(t *testing.T) {
    // ... setup ...

    log := server.RequestLog()
    if len(log) != 1 {
        t.Fatalf("expected 1 request, got %d", len(log))
    }
    entry := log[0]
    if entry.Method != "POST" {
        t.Errorf("expected POST, got %s", entry.Method)
    }
    if entry.Path != "/api/users" {
        t.Errorf("expected /api/users, got %s", entry.Path)
    }
}
```

## Full Example

See [examples/verification/](../examples/verification/main_test.go) for a complete,
runnable example with 5 test functions.

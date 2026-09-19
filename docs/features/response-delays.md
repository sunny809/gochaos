# Response Delays

> gochaos supports **5 delay distributions** to simulate realistic network latency.
> Delays are applied *before* the response is written, affecting the total round-trip time.
> In combination with faults, delays let you simulate slow failures and degraded performance.

## Delay Types

| Type | Behaviour | Use Case |
|------|-----------|----------|
| `fixed` | Constant delay | Testing timeout configurations |
| `random` | Uniform random delay in [min, max] | Testing jitter tolerance |
| `lognormal` | Statistically realistic delay with configurable p50/p95/p99 | Simulating real-world network latency |
| `timeout` | Response never returns (connection hangs until client timeout) | Testing client-side timeout handling |
| `dribble` | Response sent in slow chunks | Testing streaming and partial-data handling |

---

### Fixed Delay

Responds after a constant delay. Useful for testing timeout configurations and
simulating consistently slow APIs.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/slow",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"delayed response"}`,
        Delay: &gmock.DelayDefinition{
            Type:  "fixed",
            Value: 500, // 500 milliseconds
        },
    },
})
```

### Random Delay

Responds after a random delay within a uniform range. Useful for testing jitter
tolerance and real-world network variability.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/jitter",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"variable delay"}`,
        Delay: &gmock.DelayDefinition{
            Type: "random",
            Min:  100, // 100ms minimum
            Max:  300, // 300ms maximum
        },
    },
})
```

### Lognormal Delay

Responds after a statistically realistic delay based on a lognormal distribution.
Configured via percentiles (p50, p95, p99) — the median, 95th percentile, and
99th percentile latency targets.

**Why lognormal?** Real-world network latency follows a lognormal distribution:
most requests are fast (p50), but a long tail of slow requests exists (p95, p99).
Lognormal delay reproduces this pattern accurately.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/realistic",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"realistic latency"}`,
        Delay: &gmock.DelayDefinition{
            Type: "lognormal",
            P50:  50,     // median latency: 50ms
            P95:  200,    // 95th percentile: 200ms
            P99:  1000,   // 99th percentile: 1000ms (1s)
        },
    },
})
```

**Expected behaviour**: ~50% of requests complete within 50ms, ~5% take longer
than 200ms, ~1% take longer than 1000ms.

### Timeout Delay

The response never returns. The HTTP handler hangs indefinitely, causing the
client to hit its own timeout.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/hang",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"you will never see this"}`,
        Delay: &gmock.DelayDefinition{
            Type: "timeout",
        },
    },
})
```

**Client behaviour**:
- Go default HTTP client → `context deadline exceeded` (after default timeout)
- Curl → `curl: (28) Connection timed out after X milliseconds`
- Browser → spinning spinner until the page timeout

**Note**: This consumes a goroutine per hanging request. Each goroutine remains
allocated until the client disconnects or the server shuts down. Use with caution
in tests — consider a `context.WithTimeout` on the client side.

### Dribble Delay

Sends the response in slow chunks, simulating a slow network stream. The full
response is delivered, but one chunk at a time with a delay between each.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/dribble",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   "This is a slow response that arrives in chunks",
        Delay: &gmock.DelayDefinition{
            Type:     "dribble",
            ChunkSize: 10,   // bytes per chunk
            Interval:  200,  // milliseconds between chunks
        },
    },
})
```

**Expected behaviour**: Each 10-byte chunk arrives 200ms apart. For a 50-byte body:
- 0ms: first 10 bytes
- 200ms: next 10 bytes
- 400ms: next 10 bytes
- 600ms: next 10 bytes
- 800ms: final 10 bytes
- Total time: ~800ms

---

## Delay + Fault Combination

Delays and faults can be combined for realistic chaos testing. The delay is
applied first, then the fault:

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/chaos",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"chaos"}`,
        Fault:  &gmock.FaultDefinition{Type: "error"},
        Delay:  &gmock.DelayDefinition{
            Type:  "fixed",
            Value: 1000, // 1 second delay, THEN error
        },
    },
})
```

This simulates a slow fail — the client waits, then gets a 500 error.

---

## Delay Definition Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | ✅ | Delay type: `fixed`, `random`, `lognormal`, `timeout`, `dribble` |
| `value` | int | for `fixed` | Delay in milliseconds |
| `min` | int | for `random` | Minimum delay in milliseconds |
| `max` | int | for `random` | Maximum delay in milliseconds |
| `p50` | int | for `lognormal` | Median latency in milliseconds |
| `p95` | int | for `lognormal` | 95th percentile latency in milliseconds |
| `p99` | int | for `lognormal` | 99th percentile latency in milliseconds |
| `chunkSize` | int | for `dribble` | Bytes per chunk |
| `interval` | int | for `dribble` | Milliseconds between chunks |

---

## Delay via Admin API

```bash
# Fixed delay
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/slow"},"response":{"status":200,"body":"\"delayed\"","delay":{"type":"fixed","value":500}}}'

# Lognormal delay
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/realistic"},"response":{"status":200,"body":"\"ok\"","delay":{"type":"lognormal","p50":50,"p95":200,"p99":1000}}}'

# Timeout (hang indefinitely)
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/hang"},"response":{"status":200,"body":"\"never\"","delay":{"type":"timeout"}}}'
```

## Delay via YAML

```yaml
- name: fixed-delay
  request:
    method: GET
    urlPath: /api/slow
  response:
    status: 200
    body: '{"message":"delayed"}'
    delay:
      type: fixed
      value: 500

- name: lognormal-latency
  request:
    method: GET
    urlPath: /api/realistic
  response:
    status: 200
    body: '{"ok":true}'
    delay:
      type: lognormal
      p50: 50
      p95: 200
      p99: 1000
```

---

## Latency Testing Patterns

| Pattern | Delay Type | What It Tests |
|---------|-----------|---------------|
| Consistently slow API | `fixed` (500ms) | Client timeout configurations |
| Variable network | `random` (50-200ms) | Jitter tolerance, request coalescing |
| Realistic tail latency | `lognormal` (p50=10, p95=200, p99=1000) | Circuit breaker thresholds |
| Hanging connection | `timeout` | Client-side timeout handling, goroutine leaks |
| Slow stream | `dribble` (chunk=64, interval=100) | Streaming clients, progress indicators |
| Slow failure | `fixed` delay + `error` fault | Timeout + error handling together |

## Full Example

See [examples/chaos/](../examples/chaos/main_test.go) for a complete, runnable
example combining delays with faults and activation modes.
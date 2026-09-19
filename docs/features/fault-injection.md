# Fault Injection

> gochaos supports **7 fault types** to simulate production HTTP failure modes.
> Faults bypass gzip compression and normal response headers/body. They can be
> combined with delay distributions and activation modes for realistic chaos testing.

## Fault Types

| Type | What Happens | Client Sees | Use Case |
|------|-------------|-------------|----------|
| `error` | Returns HTTP 500 with JSON error body | HTTP 500 response | Test retry logic |
| `empty` | Returns empty response (no body, no headers) | HTTP 200 with 0 body | Test empty response handling |
| `connection_reset` | Closes TCP connection (Hijack + Close) | Connection reset / EOF | Test connection pool resilience |
| `malformed` | Sends invalid HTTP response bytes | Parse error / partial data | Test protocol-level error handling |
| `random_data` | Sends random bytes and closes | Garbage data / decode error | Test stream corruption resilience |
| `slow_close` | Sends full response, delays TCP FIN | Response arrives, TCP lingers | Test half-open connection handling |
| `rate_limit` | Returns 429 + Retry-After (token bucket) | HTTP 429 / 503 | Test backoff and rate-limit handling |

---

### Error Fault

Returns HTTP 500 with a JSON error body.

```
Status: 500
Content-Type: application/json
Body: {"error":"internal server error","fault":"error"}
```

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/unstable",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{Type: "error"},
    },
})
```

### Empty Fault

Returns an empty response: no body, no headers, status 200 (Go default).
The response is flushed immediately if the writer supports `http.Flusher`.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/empty",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{Type: "empty"},
    },
})
```

### Connection Reset Fault

Hijacks the underlying TCP connection and closes it, causing the client to
receive TCP RST (connection reset by peer).

**Client behavior**: Different clients handle this differently:
- Go `http.Get` default client → `EOF`
- Curl → `curl: (56) Recv failure: Connection reset by peer`
- Python requests → `ConnectionError: [Errno 54] Connection reset by peer`

**Fallback**: When `http.Hijacker` is not available (wrapped ResponseWriters in proxies),
falls back to HTTP 500 + `Connection: close`.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/reset",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{Type: "connection_reset"},
    },
})
```

### Malformed Response Fault

Sends invalid HTTP response data to the client. The response is intentionally
corrupted to test protocol-level error handling.

**Note**: This fault requires `http.Hijacker`. If the ResponseWriter doesn't support
hijacking, the fault falls back to a 200 response.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/malformed",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{Type: "malformed"},
    },
})
```

### Random Data Fault

Sends N bytes of random data and closes the TCP connection. Tests how the client
handles unexpected binary data on the wire.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/garbage",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type: "random_data",
            Config: map[string]any{
                "size": 1024, // bytes of random data to send
            },
        },
    },
})
```

### Slow Close Fault

Sends a valid HTTP response but delays closing the TCP connection. Simulates
half-open connections where the server sends data but the TCP handshake lingers.

**Why this matters**: Half-open connections can accumulate in connection pools,
eventually exhausting file descriptors or goroutine slots.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/slow-close",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"delayed close"}`,
        Fault: &gmock.FaultDefinition{Type: "slow_close"},
    },
})
```

### Rate Limit Fault

Returns HTTP 429 (Too Many Requests) with a Retry-After header. Uses a **token
bucket** algorithm to simulate rate-limited APIs.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/rate-limited",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type: "rate_limit",
            Config: map[string]any{
                "capacity": 5,      // burst capacity
                "refillRate": 1,    // tokens per second
                "refillInterval": 1, // seconds between refills
            },
        },
    },
})
```

**Client sees**:
```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
Content-Type: application/json

{"error":"rate limit exceeded","retryAfter":1}
```

---

## Fault Activation Modes

Faults don't have to fire on every request. Use activation modes to control *when*
a fault triggers:

### Probability
Fires on a percentage of requests:

```go
Fault: &gmock.FaultDefinition{
    Type: "error",
    Activation: &gmock.Activation{
        Probability: 0.3, // 30% chance
    },
},
```

### Nth Request
Fires on every Nth request (e.g., every 5th):

```go
Fault: &gmock.FaultDefinition{
    Type: "connection_reset",
    Activation: &gmock.Activation{
        EveryNthRequest: 5,
    },
},
```

### Time Window
Fires only within a specific time window (ms since epoch):

```go
Fault: &gmock.FaultDefinition{
    Type: "error",
    Activation: &gmock.Activation{
        ActiveBetween: &gmock.TimeWindow{
            StartMs: 0,           // immediately
            EndMs:   60000,       // for 60 seconds
        },
    },
},
```

### Combined
Multiple modes can be combined (AND logic — ALL must pass):

```go
Fault: &gmock.FaultDefinition{
    Type: "error",
    Activation: &gmock.Activation{
        Probability:      0.5,
        EveryNthRequest:  3,
    },
},
```

For full details, see [Advanced Chaos](advanced-chaos.md).

---

## Fault + Delay Combination

Delays are applied **before** faults. This lets you simulate slow failures:

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/slow-error",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"delayed"}`,
        Fault: &gmock.FaultDefinition{Type: "error"},
        Delay: &gmock.DelayDefinition{
            Type:  "fixed",
            Value: 1000, // 1 second delay, THEN error
        },
    },
})
```

---

## Fault Definition Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | ✅ | Fault type: `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit` |
| `config` | object | ❌ | Type-specific configuration (see Advanced Chaos for details) |
| `activation` | object | ❌ | Activation mode: probability, everyNthRequest, activeBetween |

### Valid Fault Types

| Type | Config Fields | Requires Hijacker? |
|------|---------------|:------------------:|
| `error` | — | ❌ |
| `empty` | — | ❌ |
| `connection_reset` | — | ✅ (fallback to 500) |
| `malformed` | — | ✅ (fallback to 200) |
| `random_data` | `size` (int, default 1024) | ✅ (fallback to 200) |
| `slow_close` | — | ✅ (fallback to 200) |
| `rate_limit` | `capacity`, `refillRate`, `refillInterval` | ❌ |

---

## Fault Injection via Admin API

```bash
# Error fault
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/error"},"response":{"fault":{"type":"error"}}}'

# Rate limit fault with config
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/limited"},"response":{"fault":{"type":"rate_limit","config":{"capacity":3,"refillRate":1,"refillInterval":1}}}}'

# Probabilistic connection reset
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/unstable"},"response":{"fault":{"type":"connection_reset","activation":{"probability":0.3}}}}'
```

## Fault Injection via YAML

```yaml
- name: error-fault
  request:
    method: GET
    urlPath: /api/unstable
  response:
    fault:
      type: error

- name: rate-limited
  request:
    method: GET
    urlPath: /api/limited
  response:
    status: 200
    body: '{"ok":true}'
    fault:
      type: rate_limit
      config:
        capacity: 5
        refillRate: 1
        refillInterval: 1

- name: delayed-error
  request:
    method: GET
    urlPath: /api/slow-error
  response:
    delay:
      type: fixed
      value: 1000
    fault:
      type: error
```

---

## Chaos Testing Patterns

| Pattern | Fault Type | Activation | What It Tests |
|---------|-----------|------------|---------------|
| Service unavailable | `error` | — | Retry logic, circuit breakers |
| Empty response | `empty` | — | Defensive parsing, null handling |
| Connection drop | `connection_reset` | — | Connection pool recovery |
| Protocol corruption | `malformed` / `random_data` | — | Protocol-level error handling |
| Half-open connections | `slow_close` | — | TCP connection lifecycle |
| Rate-limit backoff | `rate_limit` | — | Exponential backoff |
| Intermittent failures | `error` | `probability: 0.3` | Jitter tolerance |
| Periodic failures | `connection_reset` | `everyNthRequest: 5` | Periodic failure handling |
| Deploy-window failures | `error` | `activeBetween: [T, T+300s]` | Time-windowed resilience |
| Slow failures | `error` + `delay` | — | Timeout + error handling |

## Full Example

See [examples/chaos/](../examples/chaos/main_test.go) for a complete, runnable
chaos test suite with fault injection patterns.
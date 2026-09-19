# Advanced Chaos: Fault Bursts, Delay Distributions, and Activation Modes

> Phase 1 of gmock's chaos depth roadmap covers 90% of production HTTP failure
> categories. Before this, gmock could inject faults and delays, but they were
> always-on: every matching request got the same fault. Real failures are not
> always-on. They are burst-shaped, probabilistic, and time-windowed. This
> document covers the seven fault types, five delay distributions, three
> activation modes, and the seedable RNG that makes all of it reproducible.

## From Always-On to Real Chaos Engineering

A mock server that returns a 500 on every request is a stub, not a chaos
engineering tool. Production failures follow patterns:

- A downstream service starts rate-limiting after a burst of requests.
- A connection pool encounters a half-open connection that refuses to close.
- Latency spikes during a deploy window, then recovers.
- One in fifty requests returns garbage because a proxy corrupts the stream.

gmock's Phase 1 features let you model these patterns with three building
blocks:

1. **Fault types** -- what goes wrong (7 types, up from 3)
2. **Delay distributions** -- how long things take to go wrong (5 types, up from 2)
3. **Activation modes** -- *when* things go wrong (3 modes, new)

Combined with a seedable RNG, every chaos scenario is reproducible in CI.

---

## Fault Types

| Type | What Happens | Client Sees | Use Case |
|------|-------------|-------------|----------|
| `error` | Returns HTTP 500 + JSON error body | HTTP 500 response | Test retry logic |
| `empty` | Returns 0-byte response | HTTP 200, empty body | Test empty response handling |
| `connection_reset` | Hijacks TCP + sends RST | Connection reset / EOF | Test connection pool resilience |
| `malformed` | Sends invalid HTTP response | Parse error / partial data | Test protocol-level error handling |
| `random_data` | Sends N random bytes + closes | Garbage data / decode error | Test stream corruption resilience |
| `slow_close` | Sends full response, delays FIN | Response arrives but TCP lingers | Test half-open connection handling |
| `rate_limit` | Returns 429 + Retry-After (token bucket) | HTTP 429 / 503 | Test backoff and rate-limit handling |

### error

Returns HTTP 500 with a JSON error body. The simplest fault type -- useful for
testing retry logic, circuit breakers, and fallback paths.

**Response shape:**

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

```yaml
- name: error-fault
  request:
    method: GET
    urlPath: /api/unstable
  response:
    fault:
      type: error
```

### empty

Returns a response with no body, no headers, and the Go default status 200.
Flushed immediately if the `http.Flusher` interface is available. Useful for
testing defensive parsing and null/empty body handling.

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

```yaml
- name: empty-fault
  request:
    method: GET
    urlPath: /api/empty
  response:
    fault:
      type: empty
```

### connection_reset

Hijacks the underlying TCP connection and closes it, sending a TCP RST
(connection reset by peer) to the client. Different HTTP clients report this
differently:

- Go `net/http` -- `EOF` or `connection reset by peer`
- curl -- `curl: (56) Recv failure: Connection reset by peer`
- Python `requests` -- `ConnectionError: [Errno 54] Connection reset by peer`

**Fallback**: When `http.Hijacker` is not available (for example, when the
`ResponseWriter` is wrapped by a proxy or middleware), gmock falls back to
HTTP 500 + `Connection: close` header.

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

```yaml
- name: connection-reset
  request:
    method: GET
    urlPath: /api/reset
  response:
    fault:
      type: connection_reset
```

### malformed

Sends a malformed HTTP response to simulate protocol-level corruption. When
Hijack is available, gmock writes an invalid HTTP response directly to the TCP
connection: a valid status line with `Content-Length: 100`, but only 15 bytes
of body data before the connection is closed. When Hijack is not available,
the fallback sends a normal HTTP 200 with `Content-Length: 100` but only a
partial body (`{"partial":`), simulating a truncated response.

This fault type tests whether your client can handle responses that violate
the HTTP protocol -- a common failure mode when reverse proxies or CDNs
corrupt the response stream.

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

```yaml
- name: malformed-response
  request:
    method: GET
    urlPath: /api/malformed
  response:
    fault:
      type: malformed
```

### random_data

Sends `dataLength` bytes of random binary data to the client, then closes the
connection. This simulates a scenario where a proxy, middleware, or corrupted
buffer injects garbage bytes into the response stream before the connection
drops.

When Hijack is available, raw random bytes are written directly to the TCP
connection. When Hijack is not available, the fallback returns HTTP 500 with
a hex-encoded random body (half the requested length, since hex encoding
doubles the byte count).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dataLength` | int | 256 | Number of random bytes to send before closing |

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/garbage",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type:       "random_data",
            DataLength: 512, // 512 bytes of random data
        },
    },
})
```

```yaml
- name: random-data
  request:
    method: GET
    urlPath: /api/garbage
  response:
    fault:
      type: random_data
      dataLength: 512
```

### slow_close

Sends the complete normal response (status, headers, body), then delays before
closing the TCP connection. This simulates a server that is slow to send FIN
after completing the response -- the client receives all data, but the TCP
connection lingers in a half-open state.

Unlike other fault types, `slow_close` does **not** short-circuit normal
response writing. The stub's configured response is served normally, then the
connection is hijacked and held open for the configured delay before closing.
The `Connection: close` header is set on the response to signal that the
connection will not be reused.

When Hijack is not available, the `Connection: close` header is still set,
which is the best available fallback.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `delayMs` | int | 1000 | Milliseconds to hold the connection open after the response |

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/slow-close",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"data":"received"}`,
        Fault: &gmock.FaultDefinition{
            Type:    "slow_close",
            DelayMs: 3000, // 3 seconds before FIN
        },
    },
})
```

```yaml
- name: slow-close
  request:
    method: GET
    urlPath: /api/slow-close
  response:
    status: 200
    body: '{"data":"received"}'
    fault:
      type: slow_close
      delayMs: 3000
```

### rate_limit

Simulates API rate limiting using a token-bucket algorithm. When the token
bucket is empty, gmock returns a 429 (or custom status) response with a
`Retry-After` header. When tokens are available, the normal stub response is
served.

The `rate_limit` fault type is handled at a higher level than other faults --
the rate-limit check happens before the response is written, so rate-limited
requests never reach the normal response pipeline.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `perSecond` | int | (required) | Maximum requests per second after warm-up. Must be > 0. |
| `afterRequests` | int | 0 | Number of requests to allow before rate limiting begins (warm-up phase). 0 = rate limiting begins immediately. |
| `rateLimitStatus` | int | 429 | HTTP status code for rate-limited responses. Set to 503 for service unavailable semantics. |

**Rate-limited response shape:**

```
Status: 429 (or custom rateLimitStatus)
Content-Type: application/json
Retry-After: 1
Body: {"error":"rate limited","fault":"rate_limit","retryAfter":1}
```

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/throttled",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"data":"ok"}`,
        Fault: &gmock.FaultDefinition{
            Type:            "rate_limit",
            PerSecond:       5,    // 5 requests/second max
            AfterRequests:   10,   // First 10 requests always succeed
            RateLimitStatus: 503,  // Return 503 instead of 429
        },
    },
})
```

```yaml
- name: rate-limited-api
  request:
    method: GET
    urlPath: /api/throttled
  response:
    status: 200
    body: '{"data":"ok"}'
    fault:
      type: rate_limit
      perSecond: 5
      afterRequests: 10
      rateLimitStatus: 503
```

#### Token-Bucket Algorithm

The rate limiter uses a classic token-bucket algorithm with these properties:

1. **Warm-up phase**: The first `afterRequests` requests are always allowed,
   regardless of the token bucket. This models real APIs that allow an initial
   burst before enforcing limits.

2. **Bucket capacity**: The bucket holds at most `perSecond` tokens. This
   means the maximum burst size after warm-up is `perSecond` requests.

3. **Refill rate**: Tokens refill at `perSecond` tokens per second,
   computed lazily on each request. The refill calculation is:
   ```
   tokens = min(perSecond, tokens + elapsed_seconds * perSecond)
   ```

4. **Initial state**: The bucket starts full (`tokens = perSecond`), so the
   first `perSecond` requests after warm-up are served immediately.

5. **Per-stub isolation**: Each stub maintains its own independent token
   bucket. Rate-limit state is cleaned up when a stub is deleted.

---

## Delay Distributions

| Type | Behavior | Parameters | Use Case |
|------|----------|------------|----------|
| `fixed` | Constant delay | `value` (ms) | Reproduce specific timeout thresholds |
| `random` | Uniform random delay in range | `min`, `max` (ms) | Simulate variable network latency |
| `timeout` | Blocks until client disconnects | (none) | Test client-side timeout handling |
| `lognormal` | Lognormal distribution | `p50`, `p95`, `p99` (ms) | Model realistic internet latency with long tails |
| `dribble` | Chunked body with inter-chunk delays | `chunks`, `totalDuration` (ms) | Test slow/incremental response handling |

### fixed

Waits a constant number of milliseconds before responding. The simplest delay
type, useful for reproducing latency-sensitive bugs and testing timeout
configurations.

| Field | Type | Description |
|-------|------|-------------|
| `value` | int | Delay in milliseconds |

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/slow",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"delayed"}`,
        Delay: &gmock.DelayDefinition{
            Type:  "fixed",
            Value: 2000, // 2 seconds
        },
    },
})
```

```yaml
- name: slow-endpoint
  request:
    method: GET
    urlPath: /api/slow
  response:
    status: 200
    body: '{"message":"delayed"}'
    delay:
      type: fixed
      value: 2000
```

### random

Waits a random number of milliseconds uniformly distributed between `min` and
`max` (inclusive). Useful for simulating real-world network variability and
testing jitter tolerance.

| Field | Type | Description |
|-------|------|-------------|
| `min` | int | Minimum delay in milliseconds |
| `max` | int | Maximum delay in milliseconds |

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/jitter",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"message":"variable"}`,
        Delay: &gmock.DelayDefinition{
            Type: "random",
            Min:  100,
            Max:  500,
        },
    },
})
```

```yaml
- name: jittery-endpoint
  request:
    method: GET
    urlPath: /api/jitter
  response:
    status: 200
    body: '{"message":"variable"}'
    delay:
      type: random
      min: 100
      max: 500
```

### timeout

Blocks indefinitely until the client disconnects or the server shuts down.
This simulates a server that never responds, forcing the client to rely on its
own timeout mechanism. The request context is used to detect client
disconnection, so the goroutine is released when the client gives up.

No parameters are needed. This is the simplest way to test that your client
has a working timeout and that your code handles context cancellation
correctly.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/hang",
    },
    Response: gmock.ResponseDefinition{
        Delay: &gmock.DelayDefinition{
            Type: "timeout",
        },
    },
})
```

```yaml
- name: hanging-endpoint
  request:
    method: GET
    urlPath: /api/hang
  response:
    delay:
      type: timeout
```

### lognormal

Generates delays from a lognormal distribution, parameterized by percentile
values. The lognormal distribution models real-world network latency well: it
has a defined minimum (no negative latencies), a concentrated mass around the
median, and a long right tail that produces occasional extreme values.

The distribution is defined as: `X = exp(mu + sigma * Z)`, where `Z ~ N(0,1)`.

You specify the distribution by providing observed percentile values from your
production monitoring. gmock derives the underlying `mu` and `sigma`
parameters automatically. At least `p50` and one higher percentile (`p95` or
`p99`) must be provided. When both `p95` and `p99` are given, `p99` is
preferred because it captures the tail more accurately.

| Field | Type | Description |
|-------|------|-------------|
| `p50` | int | Median latency in milliseconds (required) |
| `p95` | int | 95th percentile latency in milliseconds |
| `p99` | int | 99th percentile latency in milliseconds |

**Validation rules:**
- `p50` must be > 0
- At least one of `p95` or `p99` must be > 0
- The higher percentile must be greater than `p50`

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/realistic-latency",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"data":"ok"}`,
        Delay: &gmock.DelayDefinition{
            Type: "lognormal",
            P50:  50,  // 50ms median
            P95:  200, // 200ms at 95th percentile
            P99:  500, // 500ms at 99th percentile
        },
    },
})
```

```yaml
- name: realistic-latency
  request:
    method: GET
    urlPath: /api/realistic-latency
  response:
    status: 200
    body: '{"data":"ok"}'
    delay:
      type: lognormal
      p50: 50
      p95: 200
      p99: 500
```

With this configuration, approximately:
- 50% of requests wait less than 50ms
- 95% of requests wait less than 200ms
- 99% of requests wait less than 500ms
- 1% of requests experience extreme tail latency (potentially seconds)

This is far more realistic than a uniform `random` delay, which has no tail.

### dribble

Sends the response body in small chunks with configurable delays between them.
This simulates a slow or throttled connection where data arrives incrementally
rather than all at once.

The response body is split into `chunks` equal-sized parts (the last chunk
includes any remainder bytes). Each chunk is written and flushed immediately
via `http.Flusher`, then gmock waits for `totalDuration / chunks` milliseconds
before sending the next one.

If the client disconnects mid-transfer, remaining chunks are silently dropped.

| Field | Type | Description |
|-------|------|-------------|
| `chunks` | int | Number of equal-sized chunks (required, must be > 0) |
| `totalDuration` | int | Total time in milliseconds for the entire body transfer (required, must be > 0) |

The interval between chunks is `totalDuration / chunks` milliseconds.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/dribble",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"data":"this arrives slowly, chunk by chunk"}`,
        Delay: &gmock.DelayDefinition{
            Type:           "dribble",
            Chunks:         5,
            TotalDuration:  5000, // 5 seconds total, 1 second between chunks
        },
    },
})
```

```yaml
- name: dribble-response
  request:
    method: GET
    urlPath: /api/dribble
  response:
    status: 200
    body: '{"data":"this arrives slowly, chunk by chunk"}'
    delay:
      type: dribble
      chunks: 5
      totalDuration: 5000
```

---

## Activation Modes

By default, faults and delays are always-on: every request that matches the
stub triggers the fault. Activation modes let you control *when* a fault fires,
making chaos behavior burst-shaped, probabilistic, or time-windowed.

Activation is configured on the `FaultDefinition` via the `activation` field.
When `activation` is nil or all its fields are zero, the fault is always-on
(backward compatible with pre-Phase-1 behavior).

All three activation modes can be combined on a single fault. When multiple
modes are configured, they use **AND semantics**: all configured modes must
pass for the fault to fire.

| Mode | Field | Description |
|------|-------|-------------|
| Probability | `activation.probability` | Fault fires with the given probability (0.0 -- 1.0) |
| Every Nth Request | `activation.everyNthRequest` | Fault fires on every Nth matching request |
| Active Between | `activation.activeBetween` | Fault fires only during specified time windows |

### probability

The fault fires with the given probability on each matching request. A value
of 0.5 means roughly 50% of matching requests trigger the fault. A value of
1.0 means always-on (equivalent to no activation). A value of 0.0 means never
(equivalent to disabling the fault).

The probability draw uses the seedable RNG, so the same seed produces the
same sequence of fire/skip decisions.

| Field | Type | Range | Description |
|-------|------|-------|-------------|
| `probability` | float64 | [0.0, 1.0] | Chance that the fault fires per request |

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/flaky",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type: "error",
            Activation: &gmock.Activation{
                Probability: 0.3, // 30% chance of 500 error
            },
        },
    },
})
```

```yaml
- name: flaky-endpoint
  request:
    method: GET
    urlPath: /api/flaky
  response:
    fault:
      type: error
      activation:
        probability: 0.3
```

### everyNthRequest

The fault fires on every Nth request that matches the stub. A value of 3 means
the fault fires on the 3rd, 6th, 9th request, and so on. The hit count is
tracked per stub and incremented atomically, so this works correctly under
concurrent load.

| Field | Type | Description |
|-------|------|-------------|
| `everyNthRequest` | int | Fire on every Nth matching request (must be > 0) |

This mode is useful for testing intermittent failures that follow a pattern --
for example, a service that fails every 5th request due to a resource leak.

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/cyclic-failure",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type: "connection_reset",
            Activation: &gmock.Activation{
                EveryNthRequest: 5, // Every 5th request drops the connection
            },
        },
    },
})
```

```yaml
- name: cyclic-failure
  request:
    method: GET
    urlPath: /api/cyclic-failure
  response:
    fault:
      type: connection_reset
      activation:
        everyNthRequest: 5
```

### activeBetween

The fault fires only during specified time windows. Each window defines a
start and end time in milliseconds elapsed since the server started (not Unix
timestamps). This lets you model fault bursts that occur at specific points in
a test scenario.

Windows are defined via the `activeBetween` array. When multiple windows are
configured, the first matching window wins (evaluated in array order). Each
window can optionally override the top-level `probability` for fine-grained
control.

| Field | Type | Description |
|-------|------|-------------|
| `activeBetween` | []TimeWindow | List of time windows |
| `activeBetween[].startMs` | int64 | Inclusive start of the window (ms since server start) |
| `activeBetween[].endMs` | int64 | Exclusive end of the window (ms since server start) |
| `activeBetween[].probability` | float64 | Optional probability override within this window (0.0 -- 1.0). When zero (default), the fault is always-on within the window. |

**Validation rules:**
- `endMs` must be >= `startMs` for each window
- Overlapping windows are rejected (they usually indicate a configuration mistake)

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/deploy-window",
    },
    Response: gmock.ResponseDefinition{
        Fault: &gmock.FaultDefinition{
            Type: "error",
            Activation: &gmock.Activation{
                ActiveBetween: []gmock.TimeWindow{
                    // Burst of errors between 5s and 15s after server start
                    {StartMs: 5000, EndMs: 15000},
                    // Second burst with 50% probability between 30s and 40s
                    {StartMs: 30000, EndMs: 40000, Probability: 0.5},
                },
            },
        },
    },
})
```

```yaml
- name: deploy-window-faults
  request:
    method: GET
    urlPath: /api/deploy-window
  response:
    fault:
      type: error
      activation:
        activeBetween:
          - startMs: 5000
            endMs: 15000
          - startMs: 30000
            endMs: 40000
            probability: 0.5
```

### Combining Activation Modes

When multiple activation modes are configured on the same fault, all of them
must pass (AND semantics). A mode is "configured" when its field is set to a
non-zero value. Unconfigured modes default to true (no constraint).

This enables expressive patterns:

**"30% error rate, but only during the deploy window":**

```yaml
- name: deploy-chaos
  request:
    method: GET
    urlPath: /api/chaos
  response:
    fault:
      type: error
      activation:
        probability: 0.3
        activeBetween:
          - startMs: 5000
            endMs: 15000
```

The fault fires only when BOTH conditions are true: the RNG draw is < 0.3 AND
the current time is within the window. Outside the window, the fault never
fires regardless of probability.

**"Every 10th request fails, but only with 50% probability":**

```yaml
- name: nth-flaky
  request:
    method: GET
    urlPath: /api/nth-flaky
  response:
    fault:
      type: connection_reset
      activation:
        everyNthRequest: 10
        probability: 0.5
```

On every 10th request, there is a 50% chance the fault fires. On other
requests, the fault never fires.

**"Error every 5th request during a burst window":**

```yaml
- name: burst-errors
  request:
    method: GET
    urlPath: /api/burst
  response:
    fault:
      type: error
      activation:
        everyNthRequest: 5
        activeBetween:
          - startMs: 2000
            endMs: 10000
```

---

## Seedable RNG: Reproducible Chaos

All probabilistic behavior in gmock -- delay randomization, probability
activation, random data generation -- uses a seedable pseudo-random number
generator. When you set a seed, the same sequence of chaos behavior is
reproduced across runs, making failures deterministic and debuggable.

### WithRandSeed

The `WithRandSeed` server option sets the global RNG seed. When non-zero, all
chaos behavior produces identical sequences across runs.

```go
// Deterministic: same seed = same failure sequence every time
server := gmock.NewServer(
    gmock.WithPort(0),
    gmock.WithRandSeed(42),
)

// Non-deterministic: seed from clock (default behavior)
server := gmock.NewServer(gmock.WithPort(0))
```

### What the seed affects

Setting `WithRandSeed(42)` makes the following behavior deterministic:

- **Probability activation**: The same requests trigger (or skip) faults in the same order
- **Random delays**: The same delay values are drawn from the same range
- **Lognormal delays**: The same distribution samples are generated
- **Random data fault**: The same garbage bytes are written to the connection

### CLI usage

The `--seed` CLI flag is planned for a future release. Currently, the seed can
only be set via the Go library API (`WithRandSeed`). When using the CLI, chaos
behavior is non-deterministic (seeded from the clock).

### Why determinism matters

Without a seed, running the same test twice may produce different results: one
run passes, the next fails. This is the "flaky test" problem that makes chaos
testing unreliable in CI. With a seed, the same fault sequence is reproduced
every time, so a failure in CI can be reproduced locally.

---

## Response Pipeline Order

Understanding the order in which chaos features are applied helps you compose
them correctly:

1. **Delay** -- Applied first. The server waits (or in the case of `dribble`,
   configures chunked writing) before doing anything else.
2. **Activation check** -- If the fault has an `activation` configuration,
   `ShouldActivate` is called to decide whether the fault fires.
3. **Rate-limit check** -- For `rate_limit` faults, the token bucket is
   checked before `WriteResponse`. Rate-limited requests get a 429/503
   immediately and skip the normal response pipeline.
4. **Fault injection** -- If the fault is active and is not `rate_limit` or
   `slow_close`, the fault short-circuits normal response writing.
5. **Normal response** -- If no fault short-circuited, the stub's status,
   headers, and body are written (with optional gzip compression and
   dribble-mode chunking).
6. **Slow close** -- If the fault is `slow_close`, the connection is hijacked
   and held open after the response is written.

---

## FaultDefinition Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | (required) | Fault type: `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit` |
| `dataLength` | int | 256 | Number of random bytes for `random_data` fault |
| `delayMs` | int | 1000 | Delay before FIN for `slow_close` fault (ms) |
| `activation` | *Activation | nil | Controls when the fault fires. nil = always-on. |
| `afterRequests` | int | 0 | Warm-up request count for `rate_limit` fault. 0 = no warm-up. |
| `perSecond` | int | 0 | Token-bucket refill rate for `rate_limit` fault. Must be > 0 when type is `rate_limit`. |
| `rateLimitStatus` | int | 429 | HTTP status code for `rate_limit` fault responses |

## DelayDefinition Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | (required) | Delay type: `fixed`, `random`, `timeout`, `lognormal`, `dribble` |
| `value` | int | 0 | Fixed delay in ms (for `fixed` type) |
| `min` | int | 0 | Minimum delay in ms (for `random` type) |
| `max` | int | 0 | Maximum delay in ms (for `random` type) |
| `p50` | int | 0 | Median latency in ms (for `lognormal` type, required) |
| `p95` | int | 0 | 95th percentile in ms (for `lognormal` type) |
| `p99` | int | 0 | 99th percentile in ms (for `lognormal` type) |
| `chunks` | int | 0 | Number of body chunks (for `dribble` type, required) |
| `totalDuration` | int | 0 | Total transfer time in ms (for `dribble` type, required) |

## Activation Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `probability` | float64 | 0.0 | Chance the fault fires per request [0.0, 1.0] |
| `everyNthRequest` | int | 0 | Fire on every Nth matching request (must be > 0 when set) |
| `activeBetween` | []TimeWindow | [] | Time windows during which the fault is active |

## TimeWindow Reference

| Field | Type | Description |
|-------|------|-------------|
| `startMs` | int64 | Inclusive start of the window (ms since server start, required) |
| `endMs` | int64 | Exclusive end of the window (ms since server start, required) |
| `probability` | float64 | Optional probability override within this window [0.0, 1.0] |

---

## Admin API

All Phase 1 fields are available through the admin API. Use `POST
/__admin/mappings` to create stubs with advanced chaos configuration:

```bash
# Malformed response with 30% probability
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/corrupt"},
    "response": {
      "fault": {
        "type": "malformed",
        "activation": {"probability": 0.3}
      }
    }
  }'

# Lognormal delay
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/realistic"},
    "response": {
      "status": 200,
      "body": "{\"data\":\"ok\"}",
      "delay": {
        "type": "lognormal",
        "p50": 50,
        "p95": 200,
        "p99": 500
      }
    }
  }'

# Rate-limited endpoint with warm-up
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/throttled"},
    "response": {
      "status": 200,
      "body": "{\"data\":\"ok\"}",
      "fault": {
        "type": "rate_limit",
        "perSecond": 5,
        "afterRequests": 10,
        "rateLimitStatus": 503
      }
    }
  }'

# Dribble response
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/slow-stream"},
    "response": {
      "status": 200,
      "body": "chunk-by-chunk-data-arrives-slowly",
      "delay": {
        "type": "dribble",
        "chunks": 4,
        "totalDuration": 4000
      }
    }
  }'

# Time-windowed fault burst
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{
    "request": {"method": "GET", "urlPath": "/api/deploy-chaos"},
    "response": {
      "fault": {
        "type": "error",
        "activation": {
          "probability": 0.5,
          "activeBetween": [
            {"startMs": 5000, "endMs": 15000},
            {"startMs": 30000, "endMs": 40000, "probability": 0.8}
          ]
        }
      }
    }
  }'
```

---

## Complete Example: Chaos Test Suite

This example shows a Go test that uses multiple Phase 1 features together to
test a payment service's resilience:

```go
package payment_test

import (
    "net/http"
    "testing"

    "github.com/sunny809/gochaos/pkg/gmock"
)

func TestPaymentServiceResilience(t *testing.T) {
    // Deterministic seed so this test is reproducible in CI
    server := gmock.NewServer(
        gmock.WithPort(0),
        gmock.WithRandSeed(42),
    )
    if err := server.Start(); err != nil {
        t.Fatal(err)
    }
    defer server.Stop()

    // Normal response (happy path)
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodPost,
            URLPath: "/payments",
        },
        Response: gmock.ResponseDefinition{
            Status: http.StatusOK,
            Body:   `{"status":"ok"}`,
        },
    })

    // Flaky payment gateway: 20% error rate
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodPost,
            URLPath: "/payments/flaky",
        },
        Response: gmock.ResponseDefinition{
            Fault: &gmock.FaultDefinition{
                Type: "error",
                Activation: &gmock.Activation{
                    Probability: 0.2,
                },
            },
        },
    })

    // Rate-limited endpoint: allows 10 req/s after 5 warm-up requests
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodPost,
            URLPath: "/payments/throttled",
        },
        Response: gmock.ResponseDefinition{
            Status: http.StatusOK,
            Body:   `{"status":"ok"}`,
            Fault: &gmock.FaultDefinition{
                Type:          "rate_limit",
                PerSecond:     10,
                AfterRequests: 5,
            },
        },
    })

    // Realistic latency with lognormal distribution
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodGet,
            URLPath: "/payments/status",
        },
        Response: gmock.ResponseDefinition{
            Status: http.StatusOK,
            Body:   `{"status":"pending"}`,
            Delay: &gmock.DelayDefinition{
                Type: "lognormal",
                P50:  100,
                P95:  300,
                P99:  800,
            },
        },
    })

    // Slow close: full response but TCP lingers
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method:  http.MethodPost,
            URLPath: "/payments/slow-close",
        },
        Response: gmock.ResponseDefinition{
            Status: http.StatusOK,
            Body:   `{"status":"ok"}`,
            Fault: &gmock.FaultDefinition{
                Type:    "slow_close",
                DelayMs: 5000,
            },
        },
    })

    // Point your payment service at server.URL() and run your test...
    t.Logf("Mock server running at %s", server.URL())
}
```

---

## Notes and Known Limitations

- **Hijacker requirement**: The `connection_reset`, `malformed`, `random_data`,
  and `slow_close` fault types require access to `http.Hijacker` to manipulate
  the TCP connection directly. When Hijacker is not available (for example,
  when the `ResponseWriter` is wrapped by a proxy, middleware, or test
  harness), these faults fall back to less aggressive behavior (typically HTTP
  500 + `Connection: close`). The fallback is logged at warning level.

- **Rate limiting is per-stub**: Each stub with a `rate_limit` fault maintains
  its own independent token bucket. There is no global rate limiter across
  stubs.

- **Time windows use server-relative time**: `activeBetween` windows use
  milliseconds since the server started, not wall-clock or Unix timestamps.
  This makes tests deterministic but means you cannot schedule faults at a
  specific time of day.

- **Lognormal parameter derivation**: When both `p95` and `p99` are provided,
  `p99` is used to derive `sigma` because it captures the tail more accurately.
  If you need `p95` to be exact instead, provide only `p50` and `p95`.

- **Dribble and gzip**: When a dribble delay is active, gzip compression is
  bypassed because the chunked writing pattern conflicts with gzip buffering.
  The response is sent uncompressed.

- **Slow close and gzip**: When a `slow_close` fault is active, gzip
  compression is also bypassed because the connection must be flushed before
  hijacking. The `Connection: close` header is set instead.

- **Activation AND semantics**: When multiple activation modes are configured,
  all must pass. If you want OR semantics, register multiple stubs with
  different activation configurations and the same fault type.

- **EveryNthRequest is per-stub**: The hit counter tracks how many times a
  specific stub has been matched, not how many times the URL path has been
  requested. If two stubs match the same path, each has its own counter.

- **Overlapping time windows are rejected**: Validation rejects overlapping
  `activeBetween` windows because the "first match wins" semantics can be
  surprising. If you need overlapping behavior, merge the windows manually.

- **Stub-level seed**: The internal `randx` package supports per-stub RNG
  seeds (`NewStub`, `ResolveRNG`), but this is not yet exposed through the
  public API. Use `WithRandSeed` for global determinism. Per-stub seeding
  may be added in a future release.

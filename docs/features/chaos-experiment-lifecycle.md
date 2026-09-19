# Chaos Experiment Lifecycle

> How to use gmock for principled chaos engineering — from hypothesis to
> CI-gateable assertion. This guide maps the five Principles of Chaos Engineering
> to concrete gmock features and shows how they fit together in a complete
> experiment workflow.

---

## Overview: The 5-Step Lifecycle

The Principles of Chaos Engineering define five practices for running safe,
effective chaos experiments. Each principle maps to a step in the experiment
lifecycle and to specific gmock features:

| Step | Principle | What You Do | gmock Features |
|------|-----------|-------------|----------------|
| **1** | Build a Hypothesis Around Steady State | Define what "normal" looks like | Request log, Verify API |
| **2** | Vary Real-World Events | Choose a failure mode that matches reality | Fault types, delay distributions, activation modes |
| **3** | Run Experiments in Production† | Execute the experiment with guardrails | Seedable RNG, time-window activation |
| **4** | Observe the Result | Verify the experiment ran as intended | Fault log, VerifyFaultsInjected, VerifyCallbacks |
| **5** | Automate Experiments to Run Continuously | Bake experiments into CI | Deterministic seeds, CI-gateable assertions |

†gmock runs in pre-production by design (see [positioning.md](../positioning.md)).
The principles still apply: you simulate production-like conditions in a controlled
environment. Step 3's spirit is "run the experiment where it matters" — for gmock,
that means in your CI pipeline with realistic fault configurations.

---

## Step 1: Define Steady State

Before injecting chaos, you need to know what "normal" looks like for your
System Under Test (SUT). This is your **steady state hypothesis**.

### What to measure

Steady state is measured from the SUT's perspective, not the mock's. Your test
client (Go test code, k6, wrk) drives load and measures:

| Metric | Example | How gmock helps |
|--------|---------|-----------------|
| Success rate | 100% of requests return 2xx | gmock returns normal responses when faults are inactive |
| Latency | p99 < 200ms | gmock's delay distributions model the baseline |
| Request count | N requests per second | gmock's request log tracks what the SUT sent |
| Error rate | 0% 5xx errors | gmock's fault log records what was injected (not what the SUT saw — that's your test client's job) |

### Using gmock to establish baseline

```go
// 1. Start gmock with a deterministic seed
server := gmock.NewServer(gmock.WithPort(0), gmock.WithRandSeed(42))
server.Start()
defer server.Stop()

// 2. Register stubs for all downstream dependencies
server.Stub(gmock.StubDefinition{
    Request:  gmock.RequestPattern{Method: "GET", URLPath: "/api/inventory"},
    Response: gmock.ResponseDefinition{Status: 200, Body: `{"stock":100}`},
})

// 3. Run test client against SUT (which calls gmock)
// ... your test code drives load ...

// 4. Verify baseline: request log shows expected traffic
logs := server.RequestLog()
// logs[0].Path == "/api/inventory"
// logs[0].Method == "GET"
```

**Your hypothesis template**:

> Under normal conditions, the SUT receives [expected behavior] from its
> downstream dependencies. We measure this as [metrics].

---

## Step 2: Form a Hypothesis

Choose a real-world failure mode and predict how the SUT should respond.

### Fault types → real-world events

gmock's fault types model specific production failure modes:

| Fault Type | Real-World Event | What the SUT Experiences |
|------------|------------------|--------------------------|
| `error` | Application crash, 500 error from proxy | HTTP 500 with JSON error body |
| `connection_reset` | Server crash mid-request, load balancer drop | EOF / connection reset by peer |
| `random_data` | Proxy corruption, CDN buffer corruption | Garbage bytes on the wire |
| `malformed` | HTTP parser bug, protocol mismatch | Invalid HTTP response |
| `slow_close` | Server slowly dying, TCP half-open | Full response delivered, then TCP lingers |
| `empty` | Silent drop, health-check-only endpoint | Empty 200 response |
| `rate_limit` | API throttle, quota exhaustion | HTTP 429 / 503 with Retry-After |

### Activation modes → real-world patterns

| Activation | Real-World Pattern | When to Use |
|------------|-------------------|-------------|
| `probability: 0.1` | A rare, intermittent failure (1 in 10 requests) | Test jitter tolerance, connection pool resilience |
| `everyNthRequest: 5` | A resource leak that triggers every Nth request | Test periodic failure handling |
| `activeBetween: [{startMs, endMs}]` | A deploy window, a traffic spike, a timed outage | Test time-bound resilience strategies |
| Combined modes | Multiple conditions (e.g., "50% errors during deploy window") | Test complex, real-world failure cascades |

### Writing your hypothesis

Use this template:

> **If** we inject **[fault type]** into **[stub name/path]** at **[activation
> pattern]**, **then** the SUT will **[expected behavior]** **because** **[reason]**.

Examples:

- *"If we inject a 30% error rate into the payment service stub, then the
  checkout flow will retry 3 times and eventually succeed because the SUT's
  retry logic is configured for up to 5 retries with exponential backoff."*
- *"If we inject a 2-second fixed delay into the inventory service stub during
  the first 30 seconds after startup, then 15% of orders will fall back to
  cached inventory data because the circuit breaker trips after 1 second."*
- *"If we inject a `rate_limit` fault at 5 req/s into the shipping API stub,
  then the SUT will queue orders and process them when the rate limit resets."*

---

## Step 3: Inject Chaos

Configure gmock to implement your hypothesis. Three key principles:

### 1. Be deterministic with `WithRandSeed`

Probabilistic chaos is useless if it isn't reproducible. Set a seed so the same
test run produces the same fault sequence:

```go
server := gmock.NewServer(
    gmock.WithPort(0),
    gmock.WithRandSeed(42),  // Same seed = same fault sequence every time
)
```

When a CI test fails because the SUT couldn't handle a particular fault pattern,
you can reproduce the exact same sequence locally — no "works on my machine."

### 2. Constrain the blast radius with time windows

Use `activeBetween` to limit chaos to a specific time window. This is your
primary blast-radius control:

```go
// Fault fires only between 5s and 15s after server start
Fault: &gmock.FaultDefinition{
    Type: "error",
    Activation: &gmock.Activation{
        ActiveBetween: []gmock.TimeWindow{
            {StartMs: 5000, EndMs: 15000},
        },
    },
}
```

The SUT gets 5 seconds of normal behavior, then 10 seconds of chaos, then
recovers. This models a deploy-window outage and limits how long the SUT is
under stress.

### 3. Combine stubs for realistic scenarios

Real-world failures rarely affect only one service. Use multiple stubs to model
correlated failures:

```go
// Payment becomes slow
server.Stub(gmock.StubDefinition{ /* ... delay: fixed 1500ms ... */ })

// Inventory starts failing intermittently
server.Stub(gmock.StubDefinition{ /* ... error with probability: 0.3 ... */ })

// Shipping stays healthy (control group)
server.Stub(gmock.StubDefinition{ /* ... normal ... */ })
```

Each stub operates independently. The combined effect on the SUT is what you
want to test.

### Configuration reference

Build your chaos configuration from these building blocks:

```yaml
# Complete chaos stub configuration
response:
  # (Optional) Delay before responding — see response-delays.md
  delay:
    type: lognormal    # or: fixed, random, timeout, dribble
    p50: 100
    p95: 500

  # (Optional) Fault to inject — see fault-injection.md
  fault:
    type: connection_reset  # or 7 other types
    activation:
      probability: 0.3
      everyNthRequest: 10
      activeBetween:
        - startMs: 5000
          endMs: 15000
          probability: 0.5   # override within this window
```

---

## Step 4: Observe the Result

After the experiment runs, verify that (a) the chaos was injected as expected
and (b) the SUT responded as hypothesized.

### Verify what was injected

Use `VerifyFaultsInjected` to confirm that faults actually fired. This is
critical for probabilistic and Nth-request activations — without it, a passing
test could mean "no fault was injected" rather than "the SUT handled the fault
correctly."

```go
// Did the payment stub fire at least 5 errors?
result := server.VerifyFaultsInjected(gmock.FaultPattern{
    StubID:    "payment-stub",
    FaultType: "error",
}, 5)

if !result.Matched {
    t.Errorf("expected >=5 errors, got %d — test is invalid", result.ActualCount)
}
```

### Verify what the SUT sent

Use `Verify` to check that the SUT issued the right requests in response to
chaos — for example, retries, fallbacks, or circuit-breaker probes:

```go
// Did the SUT retry after circuit breaker tripped?
retries := server.Verify(gmock.RequestPattern{
    Method:  "GET",
    URLPath: "/api/inventory",
}, 3)  // At least 3 requests to inventory
```

### Inspect the fault log

The fault log gives you a full record of every fault injection event:

```bash
curl http://localhost:8080/__admin/fault-log
```

```json
{
  "entries": [
    {
      "stubId": "payment-stub",
      "faultType": "error",
      "activatedAt": "2026-07-05T10:30:45Z",
      "requestMethod": "POST",
      "requestPath": "/api/payments",
      "activationMode": "probability"
    }
  ],
  "count": 1
}
```

### Verify callbacks

If your stubs have async callbacks, use `VerifyCallbacks` to confirm they were
dispatched (or blocked by SSRF protection):

```go
result := server.VerifyCallbacks(gmock.CallbackPattern{
    StubID: "order-stub",
    Status: "delivered",
}, 1)
```

---

## Step 5: Assert & Automate

The final step makes chaos experiments a permanent part of your CI pipeline,
running continuously — not one-off manual tests.

### CI-gateable assertions

Each gmock verification API returns a result with `Matched` (bool) and `Errors`
([]string). Use these directly in Go tests:

```go
func TestPaymentServiceChaos(t *testing.T) {
    server := gmock.NewServer(
        gmock.WithPort(0),
        gmock.WithRandSeed(42),   // ← determinism
    )
    server.Start()
    defer server.Stop()

    // Step 1: Register stubs
    // ...

    // Step 2: Run test client against SUT
    // ...

    // Step 3: Assert fault injection
    if result := server.VerifyFaultsInjected(
        gmock.FaultPattern{FaultType: "error"}, 5,
    ); !result.Matched {
        t.Error(result.Errors)
    }

    // Step 4: Assert SUT behavior
    if result := server.Verify(
        gmock.RequestPattern{URLPath: "/api/inventory"}, 3,
    ); !result.Matched {
        t.Error(result.Errors)
    }
}
```

When combined with `WithRandSeed`, these tests are **deterministic**: same seed
→ same fault sequence → pass or fail on real SUT behavior, not on dice rolls.

### Pattern: Chaos test matrix

Run the same experiment with different seed values to cover more failure
sequences without manual configuration:

```go
func TestPaymentServiceMatrix(t *testing.T) {
    for _, seed := range []int64{42, 43, 44, 45} {
        t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
            server := gmock.NewServer(
                gmock.WithPort(0),
                gmock.WithRandSeed(seed),
            )
            // ... same experiment, different fault sequence ...
        })
    }
}
```

### Pattern: Continuous chaos in CI

Add chaos experiment tests to your CI pipeline as regular Go tests:

```yaml
# .github/workflows/chaos.yml
jobs:
  chaos:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-go@v5
      - run: go test -run TestPaymentServiceChaos -count=1 -race ./...
      - run: go test -run TestInventoryChaos -count=1 -race ./...
```

Each run uses the same seed, producing the same fault sequence. If a test
passes in CI and fails locally, you have a bug in your SUT, not a flaky test.

---

## Complete Example: Payment Service Chaos Experiment

Here's the full lifecycle in one test:

```go
func TestPaymentServiceWithChaos(t *testing.T) {
    // === STEP 1: Define steady state ===
    // Baseline: payment service returns 200 in <200ms.
    // We measure: success rate, latency, retry counts.

    // === STEP 3: Run the experiment (with guardrails) ===
    server := gmock.NewServer(
        gmock.WithPort(0),
        gmock.WithRandSeed(42),           // Deterministic
    )
    server.Start()
    defer server.Stop()

    // Healthy payment stub (baseline)
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{Method: "POST", URLPath: "/api/payments"},
        Response: gmock.ResponseDefinition{Status: 200, Body: `{"status":"ok"}`},
    })

    // === STEP 2: Form hypothesis ===
    // "If we inject 50% errors into the payment service for 10 seconds,
    //  the SUT will retry each failed payment at least once."

    // Faulty payment stub (higher priority, so it matches first when active)
    server.Stub(gmock.StubDefinition{
        Request: gmock.RequestPattern{
            Method: http.MethodPost,
            URLPath: "/api/payments",
            Priority: 1,                      // Higher priority than baseline
        },
        Response: gmock.ResponseDefinition{
            Fault: &gmock.FaultDefinition{
                Type: "error",
                Activation: &gmock.Activation{
                    Probability: 0.5,
                    ActiveBetween: []gmock.TimeWindow{
                        {StartMs: 2000, EndMs: 12000}, // 10-second chaos window
                    },
                },
            },
        },
    })

    // Run test client against SUT
    // for i := 0; i < 100; i++ {
    //     http.Post(sut.URL+"/checkout", ...)
    // }

    // === STEP 4: Observe ===
    // Verify faults were injected
    faultResult := server.VerifyFaultsInjected(
        gmock.FaultPattern{FaultType: "error"}, 5,
    )
    if !faultResult.Matched {
        t.Errorf("not enough faults injected: %v", faultResult.Errors)
    }

    // Verify SUT behavior (did it retry?)
    retryResult := server.Verify(
        gmock.RequestPattern{
            Method:  http.MethodPost,
            URLPath: "/api/payments",
        }, 10,
    )
    if !retryResult.Matched {
        t.Errorf("expected SUT to retry payments: %v", retryResult.Errors)
    }

    // === STEP 5: Assert & Automate ===
    // This test is CI-ready: same seed, same fault sequence, every run.
    t.Logf("Server URL: %s", server.URL())
}
```

---

## References

- [Principles of Chaos Engineering](https://principlesofchaos.org/) — the canonical reference
- [Fault Injection](fault-injection.md) — gmock's 7 fault types, 5 delay distributions, 3 activation modes
- [Advanced Chaos](advanced-chaos.md) — detailed reference for Phase 1 chaos features
- [Verification](verification.md) — request and fault verification APIs
- [Response Delays](response-delays.md) — delay distribution reference
- [positioning.md](../positioning.md) — why gmock is a "fault-burst generator," not an "SLA simulator"
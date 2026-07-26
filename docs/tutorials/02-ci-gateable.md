# Tutorial 2: CI-Gateable Chaos Experiments

## Goal

Run a chaos experiment in CI without flakiness. By the end of this tutorial
you will have a deterministic, repeatable chaos test that you can gate on in
your CI pipeline.

## Prerequisites

- Go 1.22+ installed
- 5 minutes
- Completed Tutorial 1 (or familiarity with gmock basics)

## The Problem: Flaky CI Tests

Chaos tests are inherently non-deterministic — random fault injection, timing
windows, and probabilistic activation make it hard to assert specific outcomes.
A test that passes locally may fail in CI, and a test that fails on one run may
pass on the next. This makes CI gating unreliable.

gmock solves this through three mechanisms:

1. **Deterministic RNG** — `WithRandSeed(seed)` makes all chaos behavior
   reproducible across runs.
2. **Time-window activation** — Faults fire only during specified time windows,
   creating burst-shaped failure patterns without relying on wall-clock time.
3. **Verification API** — `VerifyFaultsInjected` asserts the exact number of
   injected faults, giving CI a stable pass/fail signal.

## Solution: Deterministic Chaos with gmock

### Step 1: Seed the RNG for Determinism

```go
server := gmock.NewServer(gmock.WithPort(0), gmock.WithRandSeed(42))
```

`WithRandSeed(42)` seeds gmock's internal pseudo-random number generator. With
a non-zero seed, every probabilistic decision (fault injection, delay jitter,
etc.) produces the same sequence across runs. With seed 0 (the default), the
RNG is seeded from the current time, making behavior non-deterministic.

The seed value itself does not matter — 42 is just a convention. What matters
is that the same seed always produces the same behavior.

### Step 2: Define a Burst Window with Time-Window Activation

Time-window activation lets you specify when a fault should be active relative
to server start time. The window is defined in milliseconds elapsed since the
server started:

```go
Fault: &gmock.FaultDefinition{
    Type: "error",
    Activation: &gmock.Activation{
        ActiveBetween: []gmock.TimeWindow{
            {StartMs: 0, EndMs: 10000}, // 0–10s burst window
        },
    },
}
```

Within the `[0, 10000)`ms window, the fault is always-on (no probability gating).
Outside the window, the fault is inactive and the stub returns its normal response.

This creates a **burst shape**: failures are concentrated in the first 10 seconds,
simulating a downstream that recovers after a transient outage. For CI, the test
runs within the window so all requests trigger faults deterministically.

#### Combining with Probability

For more realistic scenarios, you can combine time-window activation with
per-window probability:

```go
ActiveBetween: []gmock.TimeWindow{
    {StartMs: 0, EndMs: 10000, Probability: 0.5}, // 50% faults during burst
},
```

With seed 42, the exact 50% sample is deterministic across runs — the same
requests always trigger faults.

### Step 3: Assert Fault Injection in CI

The `VerifyFaultsInjected` API checks that faults matching a pattern were
injected at least the specified number of times:

```go
result := server.VerifyFaultsInjected(
    gmock.FaultPattern{FaultType: "error"},
    15, // expected minimum count
)
if !result.Matched {
    t.Errorf("expected 15 fault injections, got %d: %v",
        result.ActualCount, result.Errors)
}
```

With a deterministic seed and time-window activation, this assertion is stable
across CI runs — no flakiness.

## Complete Example

The full example is at `examples/tutorial/02-ci-gateable/main_test.go`.

### Code Walkthrough

The test simulates a payments service that experiences a burst of failures:

1. **Server setup** — starts gmock with `WithRandSeed(42)` for determinism.
2. **Stub registration** — registers a payment endpoint with an "error" fault
   active in the `[0, 10s)` window after server start.
3. **SUT simulation** — a `paymentClient` with retry logic makes 5 payment
   attempts. Each attempt retries up to 2 times on 5xx errors, for a total
   of 15 requests.
4. **Assertion** — `VerifyFaultsInjected` asserts exactly 15 "error" faults
   were injected. The SUT's failure count is also verified.

### Expected Output

```
=== RUN   TestCIGateableChaosExperiment
    main_test.go:114: Payment 1: FAILED (expected during burst) — HTTP 500
    main_test.go:114: Payment 2: FAILED (expected during burst) — HTTP 500
    main_test.go:114: Payment 3: FAILED (expected during burst) — HTTP 500
    main_test.go:114: Payment 4: FAILED (expected during burst) — HTTP 500
    main_test.go:114: Payment 5: FAILED (expected during burst) — HTTP 500
    main_test.go:118: SUT results: 0 succeeded, 5 failed (expected: 0 succeeded, 5 failed)
--- PASS: TestCIGateableChaosExperiment (0.xxs)
```

## Running the Test

```bash
cd examples/tutorial/02-ci-gateable
go test -v
```

Or from the project root:

```bash
go test ./examples/tutorial/02-ci-gateable/... -v
```

## How This Enables CI Gating

| Property | Without Seed | With Seed 42 |
|----------|-------------|--------------|
| Fault injection | Random (flaky) | Deterministic |
| Same seed, same run | N/A | Always identical |
| CI assertion | `>= 1` (weak) | Exact count (strong) |
| Reproducible failures | No | Yes |

Because the test is deterministic, you can add it to your CI pipeline with
confidence:

```yaml
# .github/workflows/chaos.yml
jobs:
  chaos:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test ./examples/tutorial/02-ci-gateable/... -v
```

## What's Next?

- Try different fault types: `empty`, `connection_reset`, `malformed`
- Add delay distributions: `fixed`, `random`, `lognormal`, `dribble`
- Combine multiple time windows for gradual fault injection:

  ```go
  ActiveBetween: []gmock.TimeWindow{
      {StartMs: 0, EndMs: 5000, Probability: 1.0},    // 0–5s: 100% faults
      {StartMs: 5000, EndMs: 10000, Probability: 0.5}, // 5–10s: 50% faults
      // 10s+: no faults (recovery)
  },
  ```

- See the [verification example](../../examples/verification/main_test.go) for
  more assertion patterns.
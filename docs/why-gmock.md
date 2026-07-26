# Why gmock?

> The only CI-gated chaos mock for any microservice.

## The Problem: Resilience Testing is Broken

Most teams test their services against stable mocks. But production isn't stable --
it's bursty. A bad deploy, a saturated queue, a regional outage: these last seconds
to minutes, then resolve.

Traditional mock servers (WireMock, gock) give you two choices:
1. **Always-on chaos** -- 5% errors forever. This tests the wrong thing: it tells you
   the SUT can tolerate 5% errors *on average*, but not whether it *recovers* after
   a 90-second total outage.
2. **No chaos at all** -- your resilience code (retries, circuit breakers, fallbacks)
   is never exercised.

## Our Answer: Fault Bursts, Not Steady-State

Production failures are **burst-shaped**. gochaos models this with three activation modes:

| Mode | What it models | Example |
|------|---------------|---------|
| **Probability** | Intermittent failure (1 in 10 requests) | `probability: 0.1` |
| **Nth-request** | Resource leak that triggers every Nth request | `everyNthRequest: 5` |
| **Time-window** | Deploy-window outage, traffic spike | `activeBetween: [{startMs: 30000, endMs: 90000}]` |

## gmock vs Alternatives

| Feature | gochaos | gock / httpmock | WireMock |
|---------|---------|-----------------|----------|
| **Fault types** | 7 | 0 | 4 |
| **Delay distributions** | 5 | 0 | 3 |
| **Probabilistic faults** | ✅ | ❌ | ❌ |
| **Nth-request faults** | ✅ | ❌ | ❌ |
| **Time-window faults** | ✅ | ❌ | ❌ |
| **Seedable RNG** | ✅ | ❌ | ❌ |
| **CI-gateable fault assertions** | ✅ | ❌ | ❌ |
| **Near-miss diagnostics** | ✅ | ❌ | ✅ |
| **Real HTTP server** | ✅ | ❌ | ✅ |
| **Standalone CLI / Docker** | ✅ (~15MB) | ❌ | ✅ (~200MB) |
| **Embeddable in Go tests** | ✅ | ✅ | ❌ |
| **Startup time** | ~5ms | ~1ms | ~2-5s |
| **Memory** | ~10MB | ~5MB | ~200-500MB |

gochaos differentiator: **chaos depth** -- probabilistic/Nth-request/time-window
activation modes, seedable RNG, and CI-gateable fault assertions. WireMock can't
do any of this. gock/httpmock have no chaos features at all.

## Who This Is For

- **Go developers** writing resilience tests
- **CI/CD maintainers** needing a mock in the pipeline
- **Chaos engineers** needing reproducible fault sequences

## Who This Is NOT For

- Teams that only need stable mocks (use gock/httpmock -- they're simpler)
- Teams needing TCP-layer chaos (use Toxiproxy -- it's more powerful there)
- Teams needing a GUI (use WireMock Cloud or Mockoon)

## See Also

- [Getting Started](features/getting-started.md)
- [Chaos Experiment Lifecycle](features/chaos-experiment-lifecycle.md)
- [Architecture Decision Records](adrs/)
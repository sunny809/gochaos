# Positioning: Why "fault-burst generator" and not "SLA simulator"

> Companion document to `ROADMAP.md` §1.
> Audience: contributors, reviewers, anyone evaluating whether gochaos fits a use case.
> Status: locked-in product framing as of 2026-06-17.

## The one-line answer

**SLA is the producer's view. Resilience testing is the consumer's view. A mock
server is a tool used by consumers — it has no business pretending to *meet* an
SLA, only to *break* one on demand.**

If you only read one section, that's the entire argument. The rest of this
document defends it against the most common counter-arguments.

---

## Why this matters now

A prior vision of this project (visible in earlier ROADMAP drafts) was an
"SLA-driven chaos simulation server" — the user would declare `availability: 99.9%`,
`p99 < 200ms`, and the engine would drive responses to *match* that SLA over time.

That framing was retired in 2026-06. Three reasons, in priority order:

1. **It never shipped.** The SLA derivation engine was specified, never built.
   Working code beats unimplemented vision every time.
2. **It targets the wrong actor.** SLAs belong to the team that *runs* a service
   (capacity planning, SLO negotiation, error-budget management). The team that
   *consumes* a service doesn't care if the dependency meets its SLA — they care
   whether their own service survives when the dependency *doesn't*.
3. **It conflates two questions** that should stay separate:
   - "Does this service meet its availability target?" → answered by SLO/SLI tooling
     in production (Prometheus, Honeycomb, Datadog).
   - "Does this service survive when its dependencies fail?" → answered by chaos
     testing in pre-production. ← *This is what gochaos does.*

---

## Producer view vs. consumer view (concretely)

```
       ┌──────────────────────────────────────────────────────────────┐
       │  PRODUCER VIEW — "I run InventoryService"                    │
       │  - I publish an SLA: 99.9% available, p99 < 200ms           │
       │  - I monitor SLIs in prod: error rate, latency histograms   │
       │  - Tools: Prometheus + Grafana, error budgets, SLO alerts   │
       │  - Question: am I keeping my promise?                        │
       └──────────────────────────────────────────────────────────────┘

       ┌──────────────────────────────────────────────────────────────┐
       │  CONSUMER VIEW — "I run OrderService, which calls Inventory" │
       │  - I assume Inventory will sometimes break                   │
       │  - I write retries / timeouts / circuit breakers / fallbacks │
       │  - I want to test those resilience strategies under load     │
       │  - Tools: gochaos + k6 + Go test code                        │
       │  - Question: do I survive when Inventory misbehaves?         │
       └──────────────────────────────────────────────────────────────┘
                              ↑
                    gochaos lives here
```

A mock server in the consumer's test loop is **standing in for the producer**.
Asking "what should this mock's SLA be?" is a category error — its job is not to
have an SLA. Its job is to *break in interesting, reproducible ways* so the
consumer's resilience code gets exercised.

---

## What "fault-burst generator" means concretely

A "fault burst" has three properties, and gochaos is engineered around all three:

### 1. Burst-shaped (seconds-to-minutes, not steady-state)

Production failures are bursty. A bad deploy, a saturated queue, a regional
outage — these last seconds to minutes, then resolve. Resilience tests need to
recreate that shape.

A steady-state mock that returns 5% errors *forever* tests the wrong thing. It
tells you the SUT can tolerate 5% errors *on average*, but it doesn't tell you
whether the SUT recovers after a 90-second total outage.

gochaos supports this via **time-window faults** (A3) with multiple segments per
stub: a single YAML can express *"0-30s: normal; 30s-90s: 50% failure; 90s+:
always-on failure"* and the test client driving load through the SUT will see
exactly that shape.

### 2. Reproducible

Probabilistic chaos is worthless if it isn't deterministic. If a CI run flakes
once with the SUT misbehaving, you need to be able to re-run with the same
fault sequence to debug.

gochaos's **seedable RNG** (A11) makes every probabilistic decision derive from
a configurable seed. Same seed → same fault sequence. CI tests pass and fail
on real SUT behavior, not on dice rolls.

### 3. Observable / Assertable

Chaos tests that rely on a human eyeballing SUT logs to decide "looks OK" don't
make it into CI gates. Tests that gate CI need to assert.

gochaos's **fault-injection log** (O1-O3) records every fault that fires. Test
code can then assert:

```go
server.VerifyFaultsInjected(
    gmock.FaultPattern{Type: "error", StubID: "inventory-reserve"},
    gmock.ExpectAtLeast(3),
)
```

This is the property no other Go mock server provides — and it's a property
WireMock structurally cannot provide, because WireMock faults are always-on
(no need to record what fired). gochaos's faults are conditional, so we *must*
log, and that log becomes a first-class API.

---

## Anti-pattern: "Just simulate a stable service"

A reasonable question: *"What if I just want to mock a stable downstream and run
my tests?"*

The answer: **you're using gochaos with `probability: 0`.** A stable mock is a
chaos mock with the chaos parameter set to zero. We don't reject this use case —
the tool works perfectly fine that way.

What we *do* refuse to do is **lead with stability in the product narrative.**
The README, tagline, and docs first-page will frame gochaos as a chaos tool.
Stability is a degenerate case of chaos, not the headline feature. Reasons:

- "Yet another stable mock for Go" doesn't differentiate from gock, httpmock,
  or any other library. The space is crowded.
- "The Go mock that lets you test resilience" is a niche only gochaos occupies.
  We lead with the moat.
- Users who only need stability today often need chaos tomorrow. Onboarding them
  to a chaos-first tool means they don't need to migrate later.

---

## Anti-pattern: "Built-in load generator"

A second reasonable question: *"Why doesn't gochaos generate load itself? You
could ship a turnkey chaos test runner."*

Answer: **gochaos sits downstream of the SUT. The test client drives load.** This
is not an accident — it's a deliberate boundary.

```
┌────────────┐   load    ┌──────┐   chaos   ┌──────────┐
│ Test client│ ────────> │ SUT  │ ────────> │ gochaos  │
│ (k6/wrk/Go)│           │      │           │ (mock)   │
└────────────┘           └──────┘           └──────────┘
```

If gochaos generated load itself, it would be:
- a load tester (already solved by k6, wrk, hey)
- *and* a mock server
- competing for CPU/network with itself

That's two products in one binary, badly. We pick the side where there's no
strong incumbent in Go (chaos-aware mock) and let users compose us with the
tool of their choice for the load side.

---

## Implications for the roadmap

This positioning isn't decorative — it directly determines what's in scope:

| Roadmap item | Justified by positioning? | Why |
|---|---|---|
| A1-A3 (probabilistic / Nth / time-window faults) | ✅ Burst-shaped requires conditional activation |
| A11 (seedable RNG) | ✅ Reproducible is non-negotiable for CI |
| O1-O3 (fault-injection log + verify) | ✅ Observable / assertable for CI gating |
| M3 (inflection benchmark) | ✅ Mock must not be the bottleneck under SUT-driven load |
| Stateful scenarios | ❌ Producer-style modeling, not consumer-side resilience |
| SLA derivation engine | ❌ Wrong actor (producer view) |
| Built-in load generator | ❌ Boundary violation |
| GUI / dashboard | ❌ CI-gated chaos tests are headless |

Anything that doesn't directly serve "burst-shaped, reproducible, observable
fault generation downstream of a SUT" should be challenged at the planning
stage, not after it ships.

---

## How to challenge this positioning

Reasonable counter-arguments and how we'd respond:

**Q: "Most users will start with a stable mock and never need chaos."**
A: That's fine — `probability: 0` works. We're not refusing stability, we're
refusing to *lead* with it because that doesn't differentiate.

**Q: "What about non-HTTP protocols (gRPC, GraphQL, SOAP)?"**
A: Out of scope for v1.0. Microcks and others occupy multi-protocol. Phase 1
intentionally narrows to HTTP to get chaos depth right; we revisit only after
v1.0 ships.

**Q: "Toxiproxy already does chaos. Why duplicate?"**
A: Toxiproxy works at the TCP layer and needs root/privileges. gochaos works at
the HTTP layer, embeddable, no privileges. They're complementary — the project's
public stance is that you should layer Toxiproxy *upstream* of gochaos when you
need both.

**Q: "If chaos-as-CI-assertion is the moat, why not just call this 'chaos test
framework' instead of 'mock server'?"**
A: Because the API surface (stub registry, response definitions, admin REST,
near-miss diagnostics) is genuinely the API surface of a mock server. The chaos
features are layered on top of that API, not a replacement for it. Users who
know "mock server" pattern-match correctly; users who only know "chaos
framework" wouldn't expect a stub registry.

---

## See also

- `ROADMAP.md` §1 — short-form positioning
- `ROADMAP.md` §4 — anti-roadmap (what we explicitly don't do)
- `ROADMAP.md` Phase 1.5 — observability of chaos (O1-O3)
- `CLAUDE.md` — project guide for contributors

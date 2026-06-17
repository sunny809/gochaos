# Roadmap

> **Last updated:** 2026-06-17
> **Status:** Living document — the single source of truth for what gochaos is and where it's going.

This roadmap answers one question for the open-source community:
**why would someone choose gochaos over WireMock, gock, or Toxiproxy?**

Every item below passes two filters:

1. **Does it ship?** — A feature that doesn't build or isn't wired is worse than no feature.
2. **Does it differentiate?** — Cloning WireMock's matrix gets us "the Go WireMock", which is not defensible. We build what only gochaos does.

---

## 1. Positioning

> **gochaos is a fault-burst generator for the System Under Test.**
>
> When a test client (k6/wrk/Go test code) drives load into your service, gochaos
> stands in for that service's downstream dependencies and **rapidly reproduces
> high-failure / high-latency / high-unreliability conditions in seconds-to-minutes
> windows** — so you can verify the SUT's resilience strategy (retries, timeouts,
> circuit breakers, fallbacks) under conditions that would take hours to surface
> in production.
>
> It is the only Go HTTP mock that is (a) a real HTTP server (not a RoundTripper hack),
> (b) embeddable in Go tests, (c) ships chaos as a first-class feature with
> probabilistic / Nth-request / time-window activation modes, (d) tells you *why* a
> request didn't match (near-miss diagnostics), and (e) emits a machine-assertable
> fault-injection log so chaos tests can be CI-gated.

### Why "fault-burst generator" and not "SLA simulator"

A prior framing positioned gochaos as an SLA-driven availability simulator. We have
explicitly retired that framing. Reasons:

- **SLA is the producer's view** — "I promise p99 < 200ms". It belongs in capacity
  planning and SLO negotiation, not in the test loop of a consumer service.
- **A mock server should never have an SLA.** Its entire reason to exist is to
  *deliberately violate* SLAs in service of testing resilience.
- **Resilience is a consumer-side property.** What we test is whether the SUT (the
  consumer) survives when its dependencies misbehave — not whether the dependency
  meets an availability target.

The new framing — "fault-burst generator" — is consumer-centric, time-bounded, and
tied directly to a CI-friendly verification workflow. See [`docs/positioning.md`](docs/positioning.md)
for the long-form argument.

### Three properties that follow from the positioning

The framing imposes three engineering requirements that the roadmap directly funds:

| Property | What it means | Roadmap support |
|---|---|---|
| **Burst-shaped** (seconds-to-minutes) | Faults must be activatable in narrow time windows, by probability, by request count — not just always-on | Phase 1: A1 (probability), A2 (Nth-request), A3 (multi-segment time windows) |
| **Reproducible** | Same test run twice → same fault sequence. Probabilistic chaos must be debuggable | Phase 1: A11 (seedable RNG) |
| **Observable** | Test code must be able to assert "X faults of type Y were injected" — not just eyeball SUT logs | Phase 1.5: O1-O3 (fault-injection log + verify API) |

### Load expectations

gochaos is **not** a load tester. The test client (k6/wrk/your Go test code) is
responsible for generating load against the SUT. gochaos sits **downstream of the
SUT** and responds to whatever traffic the SUT forwards.

This imposes one non-negotiable constraint on gochaos itself: **its own throughput
must exceed the test client's load on the SUT**, otherwise gochaos becomes the
bottleneck and the SUT's measured behavior reflects mock queuing rather than the
faults we injected. Phase 4's inflection benchmark (M3) exists to verify this — its
purpose is "mock is not the bottleneck", not "we're faster than WireMock".

### Competitive Landscape (June 2026)

#### WireMock — the benchmark (7.3k stars, Java, est. 2011)

WireMock is the industry-standard API mock server. It defines what "feature-complete" means in this space. gochaos does NOT aim for feature parity — we aim to outperform on chaos depth and Go-native UX while accepting deliberate gaps elsewhere.

| Dimension | WireMock | gochaos | gochaos strategy |
|-----------|----------|---------|-----------------|
| **Fault types** | 4: `CONNECTION_RESET_BY_PEER`, `EMPTY_RESPONSE`, `MALFORMED_RESPONSE_CHUNK`, `RANDOM_DATA_THEN_CLOSE` | 3: `error`, `empty`, `connection_reset` — adding `malformed`, `random_data`, `slow_close`, `rate_limit` in Phase 1 | gochaos Phase 1 closes the gap (A5-A6, A8, A10) then exceeds with probabilistic/time-window (A1-A3) |
| **Delay distributions** | 3: `fixed`, `uniform`, **`lognormal`** (median + sigma) + `chunkedDribble` | 2: `fixed`, `random` (uniform) — adding `lognormal`, `timeout`, `dribble` in Phase 1 | WireMock has lognormal with p50/sigma — gochaos adds lognormal with p50/p95/p99 (more intuitive), plus timeout (A7) and dribble (A9) |
| **Probabilistic faults** | ❌ No — faults are always-on per stub | 🔜 Phase 1 (A1) | **gochaos advantage**: `probability: 0.05` = 5% of requests fail. WireMock can't do this. |
| **Nth-request faults** | ❌ No | 🔜 Phase 1 (A2) | **gochaos advantage**: `everyNthRequest: 10` = every 10th request fails. Essential for retry testing. |
| **Time-window faults** | ❌ No | 🔜 Phase 1 (A3) | **gochaos advantage**: `activeBetween: { startMs, endMs }` = simulate outage windows. WireMock can't. |
| **Near-miss diagnostics** | ✅ 3 endpoints: `POST /__admin/near-misses/request`, `POST /__admin/near-misses/request-pattern`, `GET /__admin/near-misses/unmatched` | 🟡 Engine works, REST route missing (P0) | WireMock has richer surface (3 endpoints vs 1). gochaos ships 1 endpoint in P0, can extend later. |
| **Stateful scenarios** | ✅ Full: `Scenario` with `requiredState` → `newState`, admin CRUD | ❌ Dead struct `ScenarioState` | **WireMock advantage**. Deliberately cut — most CI tests don't need it. Revive if ≥3 users request. |
| **Webhooks/callbacks** | ✅ `Webhooks` extension (post-serve action), with transformers | ❌ | **WireMock advantage**. Planned Phase 3 (C1). |
| **Response templating** | ✅ Handlebars (rich: helpers, partials, lambdas) | ✅ Go `text/template` + custom funcs (`randomUUID`, `now`, `request.*`) | Different template engines. WireMock's Handlebars is more mature; gochaos's Go-native templating is more natural for Go developers. |
| **Proxy recording** | ✅ Built-in: record → playback, with snapshot | ❌ Scaffold only | **WireMock advantage**. Parked — only revive if it can generate chaos profiles from real traffic. |
| **OpenAPI import** | ❌ Not built-in (3rd party extensions exist, 2-21 stars each) | 🔜 Phase 3 (B1-B3) | **gochaos advantage**: neither project ships this natively. gochaos builds it first. |
| **Docker** | ✅ `wiremock/wiremock` image (236 stars on docker repo) | ❌ | **WireMock advantage**. Phase 2 (D1). |
| **Embeddable as library** | ✅ Java JAR | ✅ Go package | Go-native = no JVM startup overhead, ~15MB binary vs ~50MB JAR + JDK. |
| **CLI** | ✅ Standalone JAR | ✅ Single binary | Go binary = no runtime dependency. |
| **K8s probes** | ✅ (via Docker health checks) | ❌ | Phase 2 (D2-D3). |
| **Admin API** | ✅ Full CRUD + scenarios + near-miss + recordings | ✅ CRUD + health, near-miss pending | WireMock has more endpoints. gochaos focuses on what differentiates. |
| **Matching depth** | 12+ dimensions (path/query/headers/cookies/body/path-params/multi-value headers/webhook matching/etc) | 8 dimensions | **WireMock advantage** in breadth. gochaos's 8 dims cover >95% of real use cases. We don't chase the long tail. |

#### Other competitors

| Project | Stars | Type | Chaos? | Real Server? | Near-Miss? | Language | gochaos vs them |
|---------|-------|------|--------|-------------|-------------|----------|-----------------|
| **gock** | 2.2k | RoundTripper | ❌ | ❌ | ❌ | Go | gochaos replaces gock when you need a real server or chaos. gock is simpler for unit tests. |
| **httpmock** | 2.1k | RoundTripper | ❌ | ❌ | ❌ | Go | Same as gock — both are RoundTripper interceptors, not mock servers. |
| **Toxiproxy** | 12.1k | TCP proxy | ✅ transport-level | ✅ | ❌ | Go | **Complementary**, not competing. Toxiproxy corrupts at the TCP level (needs root). gochaos works at the HTTP level (no privileges). Layer them together. |
| **Hoverfly** | 2.5k | API simulation | ✅ | ✅ | ❌ | Go | Hoverfly focuses on API simulation/capture-replay. gochaos focuses on chaos injection + diagnostics. Different niches. |
| **Mockoon** | 8.3k | Desktop mock | ⚠️ basic | ✅ | ❌ | TS | Mockoon is GUI-first, TypeScript. gochaos is CLI/library-first, Go. Different target users. |

#### Summary: where gochaos wins, where it doesn't

**gochaos wins when**:
1. You need **probabilistic / time-window / Nth-request chaos** — WireMock can't do any of these
2. You need a **Go-native embeddable mock server** — no JVM, no runtime, ~15MB binary
3. You want **near-miss diagnostics** in Go — the only Go mock server with this
4. You want **OpenAPI → stubs import** — neither WireMock nor any Go competitor ships this natively

**WireMock wins when**:
1. You need **stateful scenarios** (order lifecycle, state machines)
2. You need **proxy recording** (capture real traffic → replay)
3. You need **Handlebars templating** maturity
4. You're in a **Java/JVM ecosystem** already
5. You need the **full WireMock Cloud** commercial offering

**The defensible position**: gochaos is the project that takes chaos *beyond* always-on per-stub faults. WireMock defined the mock server; gochaos defines what happens when the mock *misbehaves on purpose*.

### What we are NOT

- **Not a WireMock clone.** We don't chase WireMock's every feature. Our chaos depth is the moat, not match-width.
- **Not a transport-layer chaos tool.** Toxiproxy / Chaos Mesh run at the TCP/kernel level (needs root/privileges). They are **complementary** — layer them upstream of gochaos. We stay app-layer, embeddable, rootless.
- **Not a load tester.** We don't replace k6 / wrk2 / hey.
- **Not a SLA enforcement engine.** (This was a prior vision that never shipped. The current codebase is a chaos mock server, and that's what we commit to.)

---

## 2. Current State

### What ships today (builds, tested, wired)

| Feature | Package | Status |
|---------|---------|--------|
| 8-dimension stub matching (method/path/regex/headers/cookies/query/body/jsonpath/accept) | `internal/matcher/` | ✅ |
| 3 fault types (error/empty/connection_reset) | `internal/response/fault.go` | ✅ |
| Delay (fixed/random) | `internal/response/http_writer.go` | ✅ |
| Response templating (randomUUID/now/Request.*) | `internal/templating/` | ✅ |
| CORS, gzip, base64 body | `internal/response/` | ✅ |
| Priority matching | `internal/stub/registry.go` | ✅ |
| Admin API (CRUD stubs, requests, reset, health) | `internal/admin/` | ✅ |
| Near-miss engine (Compute) | `internal/nearmiss/` | ✅ engine |
| Full-dimension verification | `pkg/gmock/server.go` | ✅ |
| CLI | `cmd/gmock/` | ✅ |
| Separate admin port | `pkg/gmock/options.go` | ✅ |
| 30+ static analysis linters | `.golangci.yml` | ✅ |

### What's broken or incomplete

| Issue | Severity | Why it matters |
|-------|----------|----------------|
| `POST /__admin/nearmiss` not routed | **P0** | The headline differentiator (near-miss) ships an engine that's unreachable over REST. |
| Near-miss not surfaced on 404 | **P0** | The diagnostic engine is most valuable exactly where it isn't called today. |
| `context.Background()` in Shutdown() | **P1** | Blocks indefinitely on hanging connections. Rolling K8s updates will stall. |
| No Docker image | **P1** | Non-Go teams can't use gochaos without `go install`. |
| No health probes (liveness/readiness) | **P1** | Can't deploy to Kubernetes. |
| Only 3 always-on fault types | **P2** | Project is named "gochaos" but chaos depth = 3 static modes. |

---

## 3. Phases

### Phase 0 — Survival *(must-ship, ~4h)*

The headline differentiator (near-miss) is unreachable over HTTP. Nothing else matters until this is fixed.

| ID | Work | Why P0 |
|----|------|--------|
| **P0.1** | Fix build break (done — was `s.nearMissEngine` in struct literal) | `go build` is the first impression. |
| **P0.2** | Route `POST /__admin/nearmiss` in admin handler | The N1 engine ships but has no REST surface. |
| **P0.3** | Surface near-miss on no-match 404 (`writeNoMatch`) | Diagnostics are most valuable when a request doesn't match. |
| **P0.4** | Near-miss integration tests | Verify the full chain works end-to-end. |

**Exit criteria:** `go test -race ./...` green; `curl -X POST /__admin/nearmiss` returns diagnostics; 404 responses include near-miss hint.

### Phase 1 — Chaos Depth *(the moat, ~15h)*

This is where gochaos stops being "a Go mock server with some faults" and becomes the **chaos-aware mock server**. The project is named `gochaos` — if chaos depth = 3 always-on fault types, the name is misleading.

#### Production HTTP Failure Model Analysis

Based on: Google SRE principles, AWS Well-Architected Reliability Pillar, Netflix Chaos Monkey taxonomy, Toxiproxy's 7 toxic types, WireMock's 4 fault types + 3 delay distributions.

**Production HTTP failures fall into 6 categories**, ranked by how often they occur:

| Rank | Failure Category | Real-world examples | Frequency | gochaos today | Gap? |
|------|-----------------|---------------------|-----------|---------------|------|
| 1 | **Slow/stuck responses** (timeout) | DB lock, GC pause, downstream hang, DNS resolution delay | ~35% of production incidents | `delay: { type: fixed }` + `delay: { type: random }` | ⚠️ No lognormal tail; no infinite timeout |
| 2 | **Intermittent 5xx errors** | Service overload, OOM kill, upstream crash, retry storm | ~25% | `fault: { type: error }` — always-on only | 🔴 No probabilistic; no time-window; no Nth-request |
| 3 | **Connection failures** | K8s pod eviction, load balancer drain, network partition, TLS handshake fail | ~15% | `fault: { type: connection_reset }` — always-on only | ⚠️ No slow close; no partial data |
| 4 | **Truncated/malformed responses** | Proxy timeout mid-stream, serialization error, chunked encoding break | ~10% | `fault: { type: empty }` — always-on only | 🔴 No malformed response; no random data; no chunked dribble |
| 5 | **Rate limiting / throttling** | API rate limit (429), queue full (503), circuit breaker open | ~10% | ❌ Not supported | 🔴 No rate limit simulation |
| 6 | **Slow connection close** | FIN_WAIT_2 stuck, keep-alive drain, proxy buffering | ~5% | ❌ Not supported | 🟡 Edge case |

**Coverage assessment**: gochaos covers ~60% of production failure frequency today (3 always-on faults + 2 delay types). After Phase 1, coverage reaches ~90%.

#### Phase 1 work items (priority = failure frequency × differentiation)

| ID | Work | Failure category | Freq | Depends on | Effort |
|----|------|-----------------|------|------------|--------|
| **A1** | Probabilistic faults: `fault: { type: error, probability: 0.1 }` | Intermittent 5xx | 25% | P0, A11 | 2h |
| **A2** | Nth-request faults: `fault: { type: error, everyNthRequest: 5 }` | Intermittent 5xx | 25% | P0 | 1.5h |
| **A3** | Multi-segment time-window faults: `activeBetween: [{startMs,endMs}, ...]` — multiple segments per stub for "0-30s normal, 30-90s 50% fail, 90s+ always fail" composition | Intermittent 5xx | 25% | P0 | 1.5h |
| **A4** | Latency distributions: `delay: { type: lognormal, p50, p95, p99 }` | Slow/stuck responses | 35% | P0 | 3h |
| **A5** | Malformed response: `fault: { type: malformed }` — sends invalid HTTP then closes | Truncated/malformed | 10% | P0 | 1h |
| **A6** | Random data then close: `fault: { type: random_data }` — sends garbage bytes then closes | Truncated/malformed | 10% | P0 | 1h |
| **A7** | Infinite timeout: `delay: { type: timeout }` — never responds, holds connection open | Slow/stuck responses | 35% | P0 | 0.5h |
| **A8** | Slow close: `fault: { type: slow_close, delayMs: 5000 }` — delays FIN after response | Slow connection close | 5% | P0 | 1h |
| **A9** | Chunked dribble: `delay: { type: dribble, chunks: 5, totalDuration: 1000 }` — response arrives in N chunks over T ms | Slow/stuck responses | 35% | P0 | 1.5h |
| **A10** | Rate limit simulation: `fault: { type: rate_limit, status: 429, afterRequests: 100, perSecond: 10 }` | Rate limiting | 10% | P0 | 2h |
| **A11** | Seedable RNG: `WithRandSeed(int64)` server option + per-stub seed override; deterministic ordering for probabilistic / lognormal / random delay | Reproducibility primitive | — | P0 | 0.5h |

**Why these 10 and not fewer** (first principles, not feature creep):

- **A1-A3** (probabilistic/Nth/time-window) are **activation modes** — they control *when* a fault triggers. Without them, all faults are always-on, which doesn't match reality. WireMock can't do any of these. A3 supports **multiple time segments per stub** so a single YAML file can express the "ramp into failure over a 2-minute window" pattern that drives most resilience tests.
- **A4** (lognormal delay) covers the #1 failure category (35% of incidents). Real latency has a long tail — uniform random doesn't capture the p99 spikes that cause cascading timeouts.
- **A5-A6** (malformed + random_data) close the gap with WireMock, which has both. They cover the "proxy timeout mid-stream" scenario (~10% of incidents).
- **A7** (infinite timeout) is the simplest yet most dangerous failure — a connection that never responds. Toxiproxy has this as `TimeoutToxic`. Go's `http.Client` has a `Timeout` field specifically because of this failure.
- **A8** (slow close) covers FIN_WAIT_2 / keep-alive drain — Toxiproxy's `SlowCloseToxic`. Subtle but real in long-polling scenarios.
- **A9** (chunked dribble) is WireMock's `ChunkedDribbleDelay` — response arrives in pieces, not all at once. Simulates streaming APIs, slow networks, and proxy buffering.
- **A10** (rate limit) is the only new fault *type* (vs activation mode). 429/503 responses are ubiquitous in cloud APIs. No mock server simulates this natively.
- **A11** (seedable RNG) is a primitive, not a feature. Without it, A1 / A4 / A6 produce non-reproducible test runs — same code, different fault sequence, undebuggable. 30 minutes of work that makes 8 hours of A-series work usable in CI.

**What's NOT included** (Occam's razor):

- **Bandwidth throttle** — Toxiproxy's `BandwidthToxic`. Narrow use case (slowloris), complex implementation (chunked sleep), and Toxiproxy already does it at the TCP layer. Layer Toxiproxy upstream of gochaos for this.
- **Slicer** — Toxiproxy's `SlicerToxic`. TCP-level packet fragmentation. Too low-level for an app-layer mock.
- **Limit data** — Toxiproxy's `LimitDataToxic`. Drops connection after N bytes. Covered by `connection_reset` + `malformed` combination.
- **Chaos profiles as framework feature** — Ship as `examples/chaos-profiles/` YAML combos instead.

**Exit criteria:** a user can simulate 90% of production HTTP failure modes via a single stub configuration, and the same test run twice produces the same fault sequence (A11).

### Phase 1.5 — Observability of Chaos *(CI-gateable assertions, ~3h)*

A1-A11 make chaos *injectable*; Phase 1.5 makes it *assertable*. Without this, chaos
tests stay at "demo grade" — a human eyeballs SUT logs and decides if it looked OK.
That doesn't pass through CI gates.

This is also the only feature in the entire roadmap that is **structurally absent
from WireMock**. WireMock's faults are always-on, so it has no need for an
"injection log" — when a stub is registered, every match injects. gochaos's faults
are conditional (probabilistic / Nth / time-window), so we *must* record what
actually fired, and that record becomes a first-class assertion surface.

| ID | Work | Why | Effort |
|----|------|-----|--------|
| **O1** | `FaultInjectionLog` ring buffer recording `{stubID, faultType, requestID, triggeredAt, requestPath}` for every fault that fired (not every request — only injected ones) | The data primitive | 1h |
| **O2** | `GET /__admin/fault-log` (read) + `DELETE /__admin/fault-log` (clear); JSON payload mirroring `LoggedRequest` shape | Non-Go consumers (curl, k6, bash) can read injection history | 1h |
| **O3** | `server.VerifyFaultsInjected(pattern, count)` library API — pattern matches on `faultType`, `stubID`, `requestPath`; count uses existing `ExpectExactly` / `ExpectAtLeast` matchers | Go test code can `assert` chaos behavior | 1h |

**Exit criteria:** a CI test can write `server.VerifyFaultsInjected(FaultPattern{Type: "error"}, gmock.ExpectAtLeast(3))` and have it pass/fail deterministically based on what gochaos actually injected during the test.

**Why after A1-A10 (not before)**: O1-O3 are wrappers around the A-series output. Building them first would mean designing a logging schema before the things being logged exist. Sequencing A → O is a 30% risk reduction at zero cost.

### Phase 2 — Deployability *(the gateway, ~2h)*

No one outside Go can use gochaos today. Docker + K8s probes unlock every CI pipeline.

| ID | Work | Depends on | Effort |
|----|------|------------|--------|
| **D1** | Docker image via GoReleaser → ghcr.io | P0 | 1h |
| **D2** | Liveness probe `GET /__admin/health/live` | P0 | 15min |
| **D3** | Readiness probe `GET /__admin/health/ready` | P0 | 15min |
| **D4** | Graceful shutdown timeout (fix `context.Background()`) | P0 | 30min |

**Why Docker is Phase 2, not later**: `docker run ghcr.io/sunny809/gochaos` is the single highest-ROI feature for adoption. It makes gochaos accessible to Java, Python, Node, and bash teams — not just Go developers.

**Exit criteria:** `docker run ghcr.io/sunny809/gochaos` starts; health probes work; `Stop()` completes within 30s.

### Phase 3 — Reach *(the growth lever, ~10h)*

Features that make gochaos the obvious choice for specific workflows.

| ID | Work | Depends on | Effort |
|----|------|------------|--------|
| **B1** | OpenAPI 3.0 parser | Phase 2 | 2h |
| **B2** | Stub generation from OpenAPI spec | B1 | 3h |
| **B3** | CLI `gmock import openapi.yaml -o stubs.yaml` | B2 | 2h |
| **C1** | Single async callback after response | Phase 0 | 3h |

**Why OpenAPI import (B1-B3)**: In 2026, 95% of API teams start from an OpenAPI spec. "Write YAML stubs by hand" is the anti-pattern. WireMock doesn't have this; Mockoon has it but no chaos.

**Why async callback (C1)**: The dominant pattern in modern integrations (Stripe webhooks, S3 notifications, OAuth device-flow). No Go mock server supports this. Only the single-callback case (C1); fan-out and callback faults deferred.

**Exit criteria:** `gmock import petstore.yaml` generates stubs; matched stub fires an async POST to a configured URL.

### Phase 4 — Maturity *(v1.0, ~9h)*

| ID | Work | Depends on | Effort |
|----|------|------------|--------|
| **M1** | Metrics system (8 expvar counters) | Phase 1 | 2h |
| **M2** | `GET /__admin/metrics` endpoint | M1 | 1h |
| **M3** | Inflection-point benchmark — purpose: prove gochaos is **not** the bottleneck under realistic SUT-driven load | M1 | 3h |
| **D5** | Rewrite README + tagline around "fault-burst generator for the SUT"; demote "yet another mock" framing to second paragraph | All above | 1h |
| **R1** | v1.0.0 release (CHANGELOG, tag, release notes, [`docs/positioning.md`](docs/positioning.md)) | All above | 2h |

**Why M3's purpose changed**: the inflection benchmark is no longer about marketing
("gochaos is fast"). It is now an internal quality gate — if gochaos's own throughput
under our standard test profile is lower than what k6 typically pushes through an
SUT, then *gochaos itself becomes a confounding variable* in chaos test results. The
benchmark exists to certify "mock is not the bottleneck", not to win benchmarks
against WireMock.

**Why D5 is in Phase 4 (not earlier)**: the new positioning is a promise. Putting it
on the README before A1-A11 + O1-O3 are shipped would mean shipping marketing ahead
of capability — the opposite of how we've decided to build this project.

**Exit criteria:** v1.0.0 tag on main; pkg.go.dev shows stable API; metrics endpoint live; benchmark results published; README and positioning doc reflect the fault-burst-generator framing.

---

## 4. Anti-roadmap (explicitly NOT doing)

Stating these publicly so contributors don't spend effort on parity-for-parity:

- **Matching parity with WireMock.** Our 8-dim matcher is already strong. We don't chase WireMock's every match dimension.
- **Kernel/transport-level chaos.** That's Toxiproxy / Chaos Mesh's job. Layer them upstream of gochaos.
- **A GUI.** WireMock Cloud and Mockoon own that. We stay CLI + library.
- **Multi-protocol (gRPC, SOAP, GraphQL).** Focus: HTTP/REST. Breadth is Microcks' positioning.
- **Stateful scenarios.** `ScenarioState` is a dead struct. Most CI tests don't need it. Only revive if ≥3 external users request it.
- **Proxy recording.** Keep only if it can record real traffic → emit a chaos profile. Otherwise drop.
- **Hot-reload.** Go tests rebuild stubs in-process. DX nicety, not survival.
- **SLA profile engine.** Retired by design. SLA is the producer's view ("I promise p99 < 200ms"); chaos testing is the consumer's view ("can my service survive when its dependencies break?"). A mock server should never have an SLA — its purpose is to deliberately violate them. See [`docs/positioning.md`](docs/positioning.md).
- **Built-in load generator.** gochaos sits *downstream* of the SUT. The test client (k6/wrk/your test code) drives load. We don't reinvent k6.
- **"Stable mock by default" UX.** A stable mock is just a chaos mock with `probability: 0` — the README will lead with chaos, not hide it. (Rationale: don't get pattern-matched as "yet another mock"; let users discover stability as a degenerate case.)

---

## 5. Reconciled slice status

| Old slice | Actual status | Notes |
|-----------|---------------|-------|
| Slice 1–5 (types, registry, server, admin, CLI) | ✅ Done | — |
| Slice 6 (templating) | ✅ Done | `internal/templating/` |
| Slice 7 (fault/delay) | ✅ Done | `internal/response/fault.go` |
| Slice 8 (proxy recording) | 🟡 Scaffold only | CLI flag exists, no ReverseProxy. Parked. |
| Slice 9 (stateful scenarios) | ✂️ Cut | `ScenarioState` is a dead struct. |
| Slice 10 (near-miss) | 🟡 Engine done; **P0.2/P0.3 pending** | REST + 404 surface are Phase 0 work. |
| Slice 11 (verification) | ✅ Done | Full-dimension verify shipped. |
| Slice 12 (hot-reload) | ⏸️ Deferred | See Anti-roadmap. |
| Slice 13 (polish + CI) | 🟡 CI exists; keep green | golangci-lint 30+ linters active. |

---

## 6. How to contribute

High-impact, low-ambiguity entry points (in priority order):

1. **P0.2** — route POST /__admin/nearmiss. Well-scoped, engine already exists.
2. **P0.3** — surface near-miss on 404. Small change in `writeNoMatch`.
3. **A11** — seedable RNG. 30 minutes, unblocks all probabilistic Phase 1 work.
4. **A1** — probabilistic faults. Self-contained package, testable in isolation.
5. **D1** — Docker image. ~10 lines in `.goreleaser.yml`.
6. **O1** — fault-injection log. Mirrors existing `RequestLog`; comes after Phase 1 work lands.

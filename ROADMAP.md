# Roadmap

> **Last updated:** 2026-06-19
> **Status:** Living document — the single source of truth for what gochaos is and where it's going.

This roadmap answers one question for the open-source community:
**why would someone choose gochaos over WireMock, gock, or Toxiproxy?**

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

### Three properties that differentiate gochaos

| Property | What it means | Status |
|---|---|---|
| **Burst-shaped** (seconds-to-minutes) | Faults activatable by probability, by request count, by time window — not just always-on | ✅ Phase 1 shipped (A1-A11) |
| **Reproducible** | Same test run twice → same fault sequence. Probabilistic chaos is debuggable | ✅ WithRandSeed shipped (A11) |
| **Observable** | Test code can assert "X faults of type Y were injected" | 📋 Phase 1.5 planned |

### Core user personas

| Persona | Pain point | gochaos solution |
|---------|-----------|-----------------|
| **Go developer** | Need to inject intermittent failures in tests | Embed gmock in Go test, use probabilistic/everyNthRequest/time-window activation |
| **CI/CD maintainer** | Non-Go team needs mock in CI pipeline | `docker run ghcr.io/sunny809/gochaos` (Phase 2) |
| **Chaos engineer** | Need reproducible chaos sequences | WithRandSeed(42) + deterministic fault ordering |
| **WireMock migrator** | Low-cost migration from Java to Go | Deferred to v1.1 (small niche) |

---

## 2. Phases

### Phase 0 — Near-Miss Surface ✅ COMPLETE

| ID | Work | Status |
|----|------|--------|
| P0.1 | Fix build break | ✅ Done |
| P0.2 | Route `POST /__admin/nearmiss` | ✅ Done |
| P0.3 | Surface near-miss on 404 | ✅ Done |
| P0.4 | Near-miss integration tests | ✅ Done |

### Phase 1 — Chaos Depth ✅ COMPLETE

The **soul of gochaos** — from "3 always-on faults" to "90% HTTP failure coverage".

| ID | Work | Status |
|----|------|--------|
| A1 | Probabilistic faults | ✅ Done |
| A2 | Nth-request faults | ✅ Done |
| A3 | Time-window faults | ✅ Done |
| A4 | Lognormal delay distribution | ✅ Done |
| A5 | Malformed response | ✅ Done |
| A6 | Random data then close | ✅ Done |
| A7 | Infinite timeout | ✅ Done |
| A8 | Slow close | ✅ Done |
| A9 | Chunked dribble | ✅ Done |
| A10 | Rate limit simulation | ✅ Done |
| A11 | Seedable RNG | ✅ Done |

**Coverage**: 7 fault types, 5 delay distributions, 3 activation modes — covers 90% of production HTTP failure modes.

### Phase 1.5 — Chaos Observability 📋 PLANNED (~3h)

Make chaos **CI-gateable** — assertions on what actually fired.

| ID | Work | Effort |
|----|------|--------|
| O1 | `FaultInjectionLog` ring buffer | 1h |
| O2 | `GET /__admin/fault-log` endpoint | 1h |
| O3 | `server.VerifyFaultsInjected(pattern, count)` API | 1h |

**Why this matters**: WireMock faults are always-on, so it has no injection log. gochaos faults are conditional (probabilistic/Nth/time-window), so we **must** record what fired for CI assertions.

### Phase 2 — Deployability 📋 NEXT (~2h)

Unlock non-Go teams via Docker + K8s.

| ID | Work | Effort |
|----|------|--------|
| D1 | Docker image via GoReleaser → ghcr.io | 1h |
| D2 | Liveness probe `GET /__admin/health/live` | 15min |
| D3 | Readiness probe `GET /__admin/health/ready` | 15min |
| D4 | Graceful shutdown timeout | 30min |

**Why Docker is highest ROI**: `docker run ghcr.io/sunny809/gochaos` makes gochaos accessible to Java, Python, Node, bash teams — not just Go developers.

### Phase 3 — Maturity 📋 PLANNED (~6h)

| ID | Work | Effort |
|----|------|--------|
| M1 | Metrics system (8 expvar counters) | 1h |
| M2 | `GET /__admin/metrics` endpoint | 30min |
| M3 | Inflection benchmark (prove mock is not bottleneck) | 2h |
| D5 | README positioning as "fault-burst generator" | 30min |
| R1 | v1.0.0 release (CHANGELOG, tag, pkg.go.dev) | 2h |

### Phase 4 — Growth (v1.1)

Features that expand reach but are **not required for v1.0 MVP**.

| ID | Work | Effort | Deferred reason |
|----|------|--------|-----------------|
| B1-B3 | OpenAPI Import | ~7h | Users can hand-write YAML; generator is nice-to-have |
| C1 | Async callback/webhook | ~3h | Low usage frequency; implementation complex (SSRF, lifecycle) |
| W1-W4 | WireMock API compatibility | ~2.5h | Migration users are niche; ROI low |
| G2-G5 | GitHub Professionalization | ~1h | Non-functional polish |

---

## 3. Anti-roadmap (explicitly NOT doing)

| Feature | Reason |
|---------|--------|
| Stateful scenarios | `ScenarioState` is dead code. Most CI tests don't need it. |
| Proxy recording | Parked. Only revive if it generates chaos profiles from real traffic. |
| Hot-reload | Go tests rebuild stubs in-process. DX nicety, not survival. |
| SLA profile engine | Retired by design. A mock server should never have an SLA — its purpose is to violate them. |
| Built-in load generator | gochaos sits downstream of SUT. Test client (k6/wrk) drives load. |
| GUI | WireMock Cloud / Mockoon own that space. We stay CLI + library. |
| Multi-protocol (gRPC, GraphQL) | Focus on HTTP/REST. Breadth is Microcks' positioning. |
| Matching parity with WireMock | Our 8-dim matcher covers >95% use cases. Don't chase long tail. |
| Kernel/transport-level chaos | That's Toxiproxy / Chaos Mesh's job. Layer them upstream. |

---

## 4. Competitive comparison

| Dimension | gochaos | WireMock | gock/httpmock |
|-----------|---------|----------|---------------|
| **Fault types** | 7 | 4 | 0 |
| **Delay distributions** | 5 (lognormal with p50/p95/p99) | 3 | 0 |
| **Probabilistic faults** | ✅ | ❌ | ❌ |
| **Nth-request faults** | ✅ | ❌ | ❌ |
| **Time-window faults** | ✅ | ❌ | ❌ |
| **Near-miss diagnostics** | ✅ | ✅ | ❌ |
| **Seedable RNG** | ✅ | ❌ | ❌ |
| **Embeddable library** | ✅ (Go) | ✅ (Java) | ✅ (RoundTripper) |
| **Real HTTP server** | ✅ | ✅ | ❌ |
| **Docker image** | 📋 Phase 2 | ✅ | ❌ |
| **Callbacks/webhooks** | 📋 v1.1 | ✅ | ❌ |
| **OpenAPI import** | 📋 v1.1 | ❌ | ❌ |
| **Stateful scenarios** | ❌ (cut) | ✅ | ❌ |

**gochaos differentiator**: **Chaos depth** — probabilistic/Nth-request/time-window activation + seedable RNG + CI-gateable assertions. WireMock can't do any of this. gock/httpmock have no chaos features at all.

---

## 5. Execution timeline

### v1.0 MVP (~4h remaining)

```
Week 1: Phase 1.5 (O1-O3)       3h  — Fault-injection log + verify API
Week 2: Phase 2 (D1-D4)         2h  — Docker + K8s probes + graceful shutdown
Week 3: Phase 3 (M1-M3, D5, R1) 6h  — Metrics + positioning + v1.0.0 release
─── v1.0.0 published ───
```

### v1.1 (post-release)

```
Week 4-5: OpenAPI Import (B1-B3)     7h
Week 6-7: Callback/Webhook (C1)      3h
Week 8:   WireMock compatibility     2.5h
Week 9:   GitHub polish (G2-G5)      1h
─── v1.1.0 published ───
```

---

## 6. Dead code cleanup (v1.0 prerequisite)

Remove these unused artifacts before v1.0 release:

| Location | Artifact | Reason |
|----------|----------|--------|
| `internal/spec/spec.go:190` | `ScenarioState` struct | Slice 9 cut, never used |
| `internal/spec/spec.go:199` | `ProxyConfig` struct | Slice 8 parked, never used |
| `internal/stub/matching.go` | `MatchWithScore`, `WeightedScore`, `RequestKey`, `ExtractRequestPattern`, `PatternsMatch` | Exported but never called |
| `pkg/gmock/server.go:313` | `RecordedStubs()` method | Always returns nil |
| `internal/matcher/matcher.go` | `AlwaysMatch`, `MatcherFunc` | Exported but unused (optional cleanup) |

---

*This roadmap reflects the Occam's razor decisions made on 2026-06-19: v1.0 MVP = Chaos core + Docker + Metrics. All expansion features deferred to v1.1.*
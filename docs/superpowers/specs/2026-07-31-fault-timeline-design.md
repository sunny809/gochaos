# Design — Fault Timeline: declare, record, replay, report

> **Status:** Approved 2026-07-31 (brainstorming session)
> **Version:** v1.1 (alongside OpenAPI Import, callbacks, WireMock compat)

## 1. Purpose

The chaos script is currently scattered: per-stub `Activation` conditions
(probability / nth_request / time_window) express *when* a single fault fires,
but there is no way to express an **ordered, multi-event burst** across stubs —
"fail requests 3-5 of POST /payments, then delay GET /inventory for 10s, then
recover" — and no way to turn a real observed fault sequence into a
**versioned CI artifact**.

This feature makes the fault burst a first-class, git-committable artifact with
four paths over one format:

| Path | API | Purpose |
|------|-----|---------|
| **Declare** | `LoadTimelineYAML` / `LoadTimeline` | Author a deterministic burst script |
| **Record** | `ExportTimeline` | Capture a real injected sequence from a run |
| **Replay** | `LoadTimeline` (same artifact) | Reproduce the exact sequence in CI |
| **Report** | `GET /__admin/report` | Emit chaos evidence as JUnit XML / JSON |

### Alignment with positioning

The three differentiating properties map one-to-one onto the four paths:

- **Burst-shaped** — the timeline is a scripted burst with an inherent end
  (events exhaust → automatic recovery). Not always-on, not open-ended.
- **Reproducible** — replay is keyed on request index, *independent of the RNG*.
  If a future gochaos version changes its RNG sequence, a recorded timeline
  replays identically — a version-level guarantee `WithRandSeed` alone cannot
  give.
- **Observable** — the JUnit/JSON report turns the fault log into CI-visible
  evidence.

### Non-goals (YAGNI)

- Timeline admin CRUD (`POST /__admin/timeline`) — add when the community asks.
- Multi-version migration, cross-process sharing, pause/fast-forward.
- Replacing per-stub `Activation` — the timeline is the *orchestration layer on
  top*; both coexist.

## 2. Artifact format

```yaml
version: 1
name: payment-outage        # optional; shown in reports
events:
  # Trigger: exactly one of request / timeMs. Until is optional, same key type.
  - at: { request: 3 }            # 3rd request matching `match`
    until: { request: 5 }         # fires for requests 3..5, then expires
    match: { method: POST, urlPath: /api/payments }   # reuses RequestPattern
    fault: { type: error }        # reuses FaultDefinition
  - at: { timeMs: 5000 }          # or: 5s after server start
    until: { timeMs: 15000 }
    match: { urlPath: /api/inventory }
    delay: { type: fixed, value: 2000 }             # reuses DelayDefinition
  - at: { request: 7 }
    match: { urlPath: /api/payments }
    fault: { type: connection_reset }
```

**Semantics**

- **"what" / "when" layering.** Stubs declare responses and resident faults;
  the timeline only declares *when to inject what*. It is a modifier on the
  response pipeline: when a request matches both a stub and an event, the stub
  supplies the response body and the event supplies fault/delay. When only the
  event matches, error-type faults apply anyway (they need no body).
- **Index semantics.** `request: N` = the Nth request matching the event's
  `match` pattern (per-event counter; other stubs' traffic does not shift it).
  `timeMs` is measured from server start.
- **Statelessness.** The runner is an ordered event-table consumer; it keeps no
  request state. This is a fault *scheduler*, not the cut `ScenarioState`
  request-response state machine.
- **Natural end.** Events expire after their window or after firing; the burst
  ends when the table is exhausted — recovery is automatic.

**Validation** (rejected at load time with a descriptive error):

- `At` required; exactly one of `At.Request` / `At.TimeMs` set.
- `Fault` XOR `Delay` — **exactly one** of the two must be set (an event
  with neither is invalid: it would consume requests and render as a fake
  delay in reports).
- `Until` must use the same key type as `At` (index window with index, time
  window with time).
- `Until` must be strictly greater than `At` (same key type): an inverted
  window (e.g. `at.request: 5`, `until.request: 3`) would silently never
  fire and is rejected at load.
- `Request >= 1`, `TimeMs >= 0`, `Match` non-empty.

## 3. Runtime & data flow

```
request → matching engine (stub match unchanged)
        → timeline runner: scan unexpired events in arrival order
            ├─ trigger hit (per-event counter reached / time window entered)
            └─ match hit → apply effect (fault/delay)
                        → record in FaultInjectionLog (with timeline ref)
        → response (stub body ± injected effect)
```

- Runner state (event counters, fired markers, clock) guarded by
  `sync.RWMutex`, consistent with the rest of the server. Runs under `-race`.
- Coexists with `Activation`: resident stub faults keep working; the timeline
  adds orchestration on top. Neither overrides the other.
- The runner records, for each fired event, the event ordinal and the counter
  value at fire time — this record is the source for `ExportTimeline`.

## 4. Record & replay

**Record.** `server.ExportTimeline()` combines the runner's fired-events record
with the recorded **stub-driven fault fires** (see `timelineRecorder`), so a
probabilistic chaos run — whose faults fire through resident stubs — exports
exactly what the client actually experienced. Triggers are written as
`request: N` (counter value at fire time), so any recorded timeline is
replayable. Consecutive fires of one event collapse into a single
`at`/`until` window. Recorded stub-driven faults have their `activation`
stripped: the event table decides when they fire on replay, so they replay
unconditionally.

Workflow: run probabilistic chaos locally (several seeds), pick a real fault
sequence worth asserting, `ExportTimeline`, commit the YAML. The committed
timeline is the CI script.

**Replay.** `LoadTimelineYAML` (declared) and `LoadTimeline` (programmatic)
share one path; replay is simply loading a recorded artifact. Determinism comes
from index keying + arrival-order matching: the same request sequence produces
the same injection sequence. When the client's request count matches the
recorded run, replay is exact per event.

**Relationship to `WithRandSeed`.** Replay does not consult the RNG. A recorded
timeline is therefore immune to RNG sequence changes between gochaos versions.

**Separation of concerns.**

- `ExportTimeline` ← runner's fired-events record **plus** recorded stub-driven
  fires. Record mode captures stub-driven fires too (the recorder mirrors the
  runner's per-key counters and adjusts for first-match-wins), so probabilistic
  chaos runs export exactly and replay deterministically.
- `GET /__admin/report` ← `FaultInjectionLog` (all injections: stub faults +
  timeline events).

## 5. Public API surface

```go
// pkg/gmock re-exports; canonical types in internal/spec
type FaultTimeline struct {
    Version int
    Name    string
    Events  []TimelineEvent
}
type TimelineEvent struct {
    At    *TimelineTrigger   // required; Request xor TimeMs
    Until *TimelineTrigger   // optional; same key type as At
    Match RequestPattern     // reuses existing pattern
    Fault *FaultDefinition   // reuses existing; exactly one of Fault/Delay
    Delay *DelayDefinition
}
type TimelineTrigger struct {
    Request int   // 1-based count of matching requests
    TimeMs  int64 // ms from server start
}

// Server methods
func (s *Server) LoadTimelineYAML(data []byte) error
func (s *Server) LoadTimeline(tl *FaultTimeline) error
func (s *Server) ExportTimeline() (*FaultTimeline, error)
```

**Admin endpoint** (internal/admin):

- `GET /__admin/report?format=junit|json` — snapshot of the fault log.
  - *JUnit XML:* one `<testsuite>`; each injection becomes a failing
    `<testcase>` (name: fault type or `delay`; classname: stub ID; failure
    message: request line + timestamp) — chaos evidence renders as a test
    suite in CI dashboards.
  - *JSON:* structured export (suite name + entries: stubId, faultType,
    activatedAt, requestMethod, requestPath, activationMode, timelineEvent,
    delayMs) for scripts.

**CLI** (thin shell):

- `gmock report --format junit` → admin endpoint → stdout.

## 6. Implementation layering

| Layer | Change |
|-------|--------|
| `internal/spec/` | `FaultTimeline`, `TimelineEvent`, `TimelineTrigger` types + validation |
| `internal/timeline/` (new) | runner: event sort, trigger checks (index/time), counters, fired-events record, export assembly — imports `internal/spec` only |
| `internal/log/` | fault-log entry gains optional timeline ref (event ordinal) |
| `internal/stub/` | unchanged (matching engine reused) |
| `internal/report/` (new) | JUnit XML / JSON serializers over the fault log |
| `internal/admin/` | `GET /__admin/report` handler |
| `pkg/gmock/` | type aliases + `LoadTimelineYAML` / `LoadTimeline` / `ExportTimeline` wiring |
| `cmd/gmock/` | `gmock report` subcommand |
| `docs/features/` | `fault-timeline.md`, `chaos-report.md` |

No new dependencies. Error wrapping per convention
(`fmt.Errorf("context: %w", err)`); `slog` for logging.

## 7. Testing

- **Unit — `internal/timeline`:** trigger boundaries (index 3; window 3..5;
  time-window enter/exit; event exhaustion → recovery); validation table;
  export assembly and window collapsing.
- **Unit — `internal/report`:** JUnit XML / JSON golden snapshots.
- **Integration — `test/integration/`:** export→replay closed loop (run
  probabilistic chaos with a seed, export, load into a fresh server, assert the
  injection sequence matches); timeline+stub superposition; `-race` clean.
- **Docs:** Tutorial 4 — "record real chaos, replay it in CI" builds on
  Tutorial 2/3 (deterministic + multi-service) and lands the declare/record/
  replay/report narrative for the community.

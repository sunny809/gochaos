# Fault Timeline: Declare, Record, Replay, Report

> The fault timeline makes a chaos burst a first-class, git-committable
> artifact. Per-stub `activation` conditions express *when a single fault
> fires*; the timeline expresses an *ordered, multi-event burst* — "fail
> requests 3-5 of POST /api/payments, then delay GET /api/inventory for 2s,
> then recover" — and turns a real observed fault sequence into a script CI can
> replay exactly. See the [design spec](../superpowers/specs/2026-07-31-fault-timeline-design.md)
> and [Tutorial 4](../tutorials/04-timeline-replay.md).

## What It Is

A timeline is an ordered schedule of fault/delay injections (the "burst
script"). Four paths run over one YAML artifact:

| Path | API | Purpose |
|------|-----|---------|
| **Declare** | `LoadTimelineYAML` / `LoadTimeline` | Author a deterministic burst script |
| **Record** | `ExportTimeline` | Capture a real injected sequence from a run |
| **Replay** | `LoadTimeline` (same artifact) | Reproduce the exact sequence in CI |
| **Report** | `GET /__admin/report` | Emit chaos evidence as JUnit XML / JSON (see [chaos-report.md](chaos-report.md)) |

The timeline is a response-pipeline *modifier*: stubs declare responses and
resident faults; the timeline only declares *when to inject what*. It is
inherently burst-shaped — events expire after their window, so recovery is
automatic.

## Declare

```yaml
version: 1
name: payment-outage          # optional; shown in reports
events:
  # Trigger: exactly one of request / timeMs. Until is optional, same key type.
  - at: { request: 3 }               # 3rd request matching `match`
    until: { request: 5 }            # fires for requests 3..5, then expires
    match: { method: POST, urlPath: /api/payments }
    fault: { type: error }
  - at: { timeMs: 5000 }             # or: 5s after server start
    until: { timeMs: 15000 }
    match: { urlPath: /api/inventory }
    delay: { type: fixed, value: 2000 }
  - at: { request: 7 }
    match: { urlPath: /api/payments }
    fault: { type: connection_reset }
```

### Trigger rules

- **`request: N`** — the Nth request matching the event's `match` pattern
  (a per-event counter; other stubs' traffic does not shift it). `N >= 1`.
- **`timeMs: T`** — T milliseconds after server start. `T >= 0`.
- **`at`** sets exactly one of the two. `until` is optional and must use the
  same key type (index window with index, time window with time).
- **Index windows** fire while the per-event counter is in `[at.request,
  until.request]`; **time windows** fire while elapsed time is in
  `[at.timeMs, until.timeMs)`. A single-fire event (no `until`) fires once and
  expires.
- **First-event-wins.** Each request is checked against events in declaration
  order; the first event whose trigger hits fires, and later events are
  re-evaluated on subsequent requests rather than silently consumed.
- **Automatic recovery.** An event is *exhausted* once it has fired past its
  window (or fired its single shot). When the whole table is exhausted the
  burst is over — no cleanup step needed.

### Validation

Rejected at load time with a descriptive error: unsupported `version`;
`at` missing or setting both/neither `request`/`timeMs`; `until` with a
different key type than `at`; `fault` and `delay` set together (they are
mutually exclusive); negative triggers.

## Record

`server.ExportTimeline()` returns a `*FaultTimeline` artifact describing every
event that fired so far:

- The runner's record of **declared timeline events** that fired, plus the
  **stub-driven fault fires** captured by the timeline recorder — so a
  probabilistic chaos run, whose faults fire through resident stubs, exports
  exactly what the client actually experienced.
- Triggers are written as `request: N` (the per-event counter value at fire
  time), so any recorded timeline is replayable — including events that were
  declared with `timeMs` triggers.
- Consecutive fires of one event collapse into a single `at`/`until` window.
- Recorded stub-driven faults have their `activation` stripped: the event
  table decides when they fire on replay, so they replay unconditionally.

Workflow: run probabilistic chaos locally (several seeds), pick a real fault
sequence worth asserting, `ExportTimeline`, commit the YAML. The committed
timeline is the CI script.

```go
tl, err := server.ExportTimeline() // after the run
// write tl to YAML, or marshal it straight into your CI test
```

## Replay

`LoadTimelineYAML` (declare or replay from a file) and `LoadTimeline`
(programmatic) share one path. Determinism comes from index keying +
first-match-wins counters: the same request sequence produces the same
injection sequence, **independent of the RNG**. If a future gochaos version
changes its RNG sequence, a recorded timeline still replays identically — a
version-level guarantee `WithRandSeed` alone cannot give.

```go
data, _ := os.ReadFile("chaos/payment-outage.yaml")
if err := server.LoadTimelineYAML(data); err != nil { ... }

// or, from a recorded artifact in memory:
if err := server.LoadTimeline(tl); err != nil { ... }
```

When the client's request count matches the recorded run, replay is exact per
event. Note that replay does **not** restore the original probability
conditions — the fired events are injected unconditionally at their recorded
positions.

## API Reference

Server methods (all also available via the admin-less Go library):

| Method | Purpose |
|--------|---------|
| `LoadTimelineYAML(data []byte) error` | Load a timeline from YAML (declare or replay) |
| `LoadTimeline(tl *FaultTimeline) error` | Load a timeline from a struct |
| `ExportTimeline() (*FaultTimeline, error)` | Record: artifact of everything that fired so far |

### YAML schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | int | yes | Artifact version, must be `1` |
| `name` | string | no | Optional label; shown in reports |
| `events` | []event | yes | Ordered event table (declaration order = scan order) |
| `events[].at` | trigger | yes | Exactly one of `request` or `timeMs` |
| `events[].at.request` | int | either/or | 1-based count of matching requests |
| `events[].at.timeMs` | int64 | either/or | ms since server start |
| `events[].until` | trigger | no | Window end; same key type as `at` |
| `events[].match` | RequestPattern | yes | `method`, `urlPath`, `urlPathRegex`, `queryParams`, `headers`, `cookies`, `body`, `accept`, `priority` — reuse the stub matcher |
| `events[].fault` | FaultDefinition | XOR with `delay` | `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit` + their parameters |
| `events[].delay` | DelayDefinition | XOR with `fault` | `fixed`, `random`, `timeout`, `lognormal`, `dribble` + their parameters |

## Relationship to Activation

The timeline is the **orchestration layer**; per-stub `activation` still
works. They coexist:

- Requests that fire no timeline event are served exactly as before — resident
  stub faults/delays (probability, nth_request, time_window) keep their
  behavior.
- When a timeline event fires, **the event's fault/delay replaces the stub's
  own** for that request (the stub still supplies status/headers/body unless
  the fault short-circuits the response).
- A fired event applies even when **no stub matched**: the server synthesizes a
  default 200 stub so error-type faults can be injected anywhere.
- Record mode is what connects the two layers: stub-driven fires on resident
  faults are captured and exported, so `ExportTimeline` documents the *actual*
  chaos your client experienced, not just what the timeline declared.

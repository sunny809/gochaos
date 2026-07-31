# Tutorial 4: Record Real Chaos, Replay It in CI

## Goal

Turn a real, observed fault sequence into a versioned CI artifact. By the end
of this tutorial you will be able to run probabilistic chaos once, export the
exact fault sequence that actually fired, and replay that sequence
deterministically in CI — with JUnit report evidence uploaded as an artifact.

## Prerequisites

- Go 1.22+ installed
- 10 minutes
- Completed Tutorial 2 (deterministic chaos, `WithRandSeed`) or familiarity
  with gmock's activation modes and `VerifyFaultsInjected`

## Why: Probabilistic Chaos Is Not CI-Safe

Tutorial 2 made chaos deterministic with `WithRandSeed(42)`: the same seed
always produces the same fault sequence. But a seed is a weak guarantee across
*versions* — if a future gochaos version changes its RNG sequence, seed 42
starts producing a different fault sequence, and your CI assertions quietly
change meaning. The fault *sequence* was never a versioned artifact.

The timeline fixes this: instead of pinning the RNG, you pin **what actually
happened**. Run the chaos once, export the fired fault sequence as a YAML
artifact, commit it. CI replays the artifact — no RNG involved at any step of
the replay. RNG changes become irrelevant.

## The Loop

```
record (local, seeded)          replay (CI, deterministic)
┌──────────────────────┐        ┌──────────────────────────────┐
│ run probabilistic    │        │ load committed timeline      │
│ chaos, observe faults│        │ + fault-free stubs           │
│ ──────────────────►  │  commit │ ────────────────────────────► │
│ ExportTimeline() →   │ artifact│ assert statuses + report    │
│ timeline YAML        │        │ upload JUnit evidence        │
└──────────────────────┘        └──────────────────────────────┘
```

1. **Record** — locally, with a seed for debuggability: stub a flaky
   downstream (say 50% errors), run your client traffic, then
   `server.ExportTimeline()`.
2. **Export & commit** — the artifact describes every injection that actually
   fired, as `at: { request: N }` triggers, with consecutive fires collapsed
   into windows. Commit it.
3. **Replay** — in CI, register the same stubs **fault-free** and
   `LoadTimeline` the committed artifact. The timeline supplies the faults at
   the recorded request positions. Same request sequence → same injection
   sequence, exactly.

## Code Walkthrough

The full example is at
`examples/tutorial/04-timeline-replay/main_test.go`. It's one test in two
phases.

### Phase 1: Record

```go
serverA := gmock.NewServer(gmock.WithPort(0), gmock.WithRandSeed(42))
serverA.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/payments"},
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Body:   `{"status":"ok"}`,
        Fault: &gmock.FaultDefinition{
            Type:       "error",
            Activation: &gmock.Activation{Probability: 0.5},
        },
    },
})
```

The stub is *flaky*: a 50% chance of an `error` fault (HTTP 500) per request.
The seed is only for debuggability — you could run unseeded and still get a
valid artifact.

Then the SUT traffic — here just 20 `http.Get` calls — and the export:

```go
recorded := make([]int, 0, 20)   // status codes the client saw
for i := 0; i < 20; i++ { /* GET /api/payments, record resp.StatusCode */ }

tl, err := serverA.ExportTimeline()
```

`ExportTimeline` combines the runner's record with the recorded stub-driven
fires, so every fault that actually fired — including the probabilistic ones
through the resident stub — lands in the artifact with its activation
stripped. The events replay unconditionally; the event table decides when.

### Phase 2: Replay

```go
serverB := gmock.NewServer(gmock.WithPort(0))
serverB.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/payments"},
    Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `{"status":"ok"}`},
})
serverB.LoadTimeline(tl)
```

Notice what's different: **no seed, and the stub is fault-free.** The timeline
is the only chaos source. The same 20 requests now produce the exact same
status sequence:

```go
for i := range recorded {
    if recorded[i] != replayed[i] {
        t.Errorf("request %d: recorded %d, replayed %d", i, recorded[i], replayed[i])
    }
}
```

And the injections are verifiable — timeline-fired faults log
`activationMode: "timeline"`, which `VerifyFaultsInjected` matches:

```go
result := serverB.VerifyFaultsInjected(gmock.FaultPattern{
    FaultType:      "error",
    ActivationMode: "timeline",
}, 1)
if !result.Matched {
    t.Errorf("expected timeline faults in replay: %v", result.Errors)
}
```

With seed 42 this run records 10 faults out of 20 requests; your run may
record a different count, but the replay always mirrors the record exactly.

## Report Evidence

The replayed injections live in the fault-injection log, so the replay server
also produces the chaos report:

```bash
curl 'http://localhost:8080/__admin/report?format=junit' > chaos-report.xml
gmock report --format junit > chaos-report.xml   # same thing via the CLI
```

Each injection becomes a failing JUnit testcase — structured evidence that the
chaos actually happened. Upload it as a CI artifact:

```yaml
- name: Export chaos evidence
  run: gmock report --format junit > chaos-report.xml
- uses: actions/upload-artifact@v4
  with:
    name: chaos-report
    path: chaos-report.xml
```

## Try It

```bash
go test ./examples/tutorial/04-timeline-replay/ -v
```

Expected: `--- PASS` with the recorded and replayed status sequences printed
side by side — identical, request for request.

## What's Next?

- Commit the exported YAML and replay it from disk with `LoadTimelineYAML`
  instead of passing the struct in memory.
- Record a multi-stub run (see Tutorial 3) and watch `ExportTimeline` produce
  one event per (method, path) pair.
- Combine timelines with resident activation faults — a fired event replaces
  the stub's fault/delay for that request; everything else behaves as before.
- See [fault-timeline.md](../features/fault-timeline.md) for the full API
  reference and [chaos-report.md](../features/chaos-report.md) for the report
  formats.

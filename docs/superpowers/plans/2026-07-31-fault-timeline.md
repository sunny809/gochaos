# Fault Timeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the fault burst a first-class, git-committable artifact — declare, record, replay, and report a deterministic fault timeline.

**Architecture:** A new `internal/timeline` package (ordered event table + per-event counters, keyed by request index or elapsed time) sits between stub matching and response writing in `serveMock` as a response-pipeline modifier. A new `internal/report` package serializes the fault injection log to JUnit XML / JSON behind `GET /__admin/report`. `pkg/gmock` re-exports types and wires `LoadTimelineYAML` / `LoadTimeline` / `ExportTimeline`; `cmd/gmock` gains `gmock report`.

**Tech Stack:** Go (std lib only), `gopkg.in/yaml.v3` (already a dependency), existing `internal/spec`, `internal/stub`, `internal/matcher`, `internal/faultlog`, `internal/response`, `internal/admin` packages.

## Global Constraints

- Types live in `internal/spec`; `pkg/gmock` re-exports via type aliases. Internal packages import `internal/spec`, never `pkg/gmock`.
- `sync.RWMutex` for all shared state; run tests with `-race`.
- Error wrapping: `fmt.Errorf("context: %w", err)`. Structured logging via `log/slog`.
- No `init()` for business logic. All files have package doc comments.
- Tests are table-driven with `t.Run` subtests.
- No new dependencies.
- Valid fault type strings are: `error`, `empty`, `connection_reset`, `malformed`, `random_data`, `slow_close`, `rate_limit`. (The design spec's YAML examples use `type: reset` — that string does NOT exist; use `connection_reset` in all code.)
- The `error` fault short-circuits with `http.StatusInternalServerError` (500) — integration tests assert on this.
- `testMu sync.Mutex` and `commonAdminURL string` already exist in `cmd/gmock` (declared in `stub_test.go` / `stub.go`) — CLI tests reuse them, never redeclare.
- Before each commit, run `gofmt -w` on every file you created or modified in that task, then `gofmt -l .` must print nothing.
- The `RequestPattern` YAML key for path is `urlPath` (tag `yaml:"urlPath,omitempty"`), NOT `path`. The design spec's YAML examples use `path:` — correct them in Task 7.
- `docs/` is gitignored — every docs file must be added with `git add -f`.
- Commit after each task. Before the final task: `gofmt -l .` must be empty, `go vet ./...` clean, `go test -race ./...` green.

---

### Task 1: Timeline types in spec + public aliases

**Files:**
- Modify: `internal/spec/spec.go` (append new sections; extend `FaultInjectionEntry`; add `ModeTimeline` const)
- Modify: `pkg/gmock/stub.go` (aliases + const)

**Interfaces:**
- Produces: `spec.FaultTimeline`, `spec.TimelineEvent`, `spec.TimelineTrigger` (canonical types), `spec.ModeTimeline` const, `spec.FaultInjectionEntry.TimelineEvent int` + `.DelayMs int` — consumed by every later task.

- [ ] **Step 1: Add the timeline types to `internal/spec/spec.go`**

Append after the Callback section (end of file):

```go
// --- Fault Timeline ---

// FaultTimeline is an ordered, deterministic schedule of fault/delay
// injections (the "burst script"). It is a git-committable artifact that can
// be declared up front, recorded from a live run, or replayed in CI.
type FaultTimeline struct {
	Version int             `json:"version" yaml:"version"`
	Name    string          `json:"name,omitempty" yaml:"name,omitempty"`
	Events  []TimelineEvent `json:"events" yaml:"events"`
}

// TimelineEvent declares one fault or delay injection on matching requests.
// At is required and sets exactly one of Request (1-based count of matching
// requests) or TimeMs (ms since server start). Until is optional and must use
// the same key type as At. Fault and Delay are mutually exclusive.
type TimelineEvent struct {
	At    *TimelineTrigger `json:"at" yaml:"at"`
	Until *TimelineTrigger `json:"until,omitempty" yaml:"until,omitempty"`
	Match RequestPattern   `json:"match" yaml:"match"`
	Fault *FaultDefinition `json:"fault,omitempty" yaml:"fault,omitempty"`
	Delay *DelayDefinition `json:"delay,omitempty" yaml:"delay,omitempty"`
}

// TimelineTrigger keys a timeline event to a request count or an elapsed time.
type TimelineTrigger struct {
	Request int   `json:"request,omitempty" yaml:"request,omitempty"`
	TimeMs  int64 `json:"timeMs,omitempty" yaml:"timeMs,omitempty"`
}
```

- [ ] **Step 2: Add `ModeTimeline` and the fault-log fields**

In the `ActivationMode` const block (after `ModeCombined`):

```go
	ModeTimeline    ActivationMode = "timeline"
```

In `FaultInjectionEntry` (after `ActivationMode` field):

```go
	// TimelineEvent is the 1-based index of the timeline event that injected
	// this entry, or 0 when the entry was not timeline-driven.
	TimelineEvent int `json:"timelineEvent,omitempty"`

	// DelayMs is the configured delay value in milliseconds when this entry
	// records a timeline delay injection (FaultType == "delay").
	DelayMs int `json:"delayMs,omitempty"`
```

- [ ] **Step 3: Add public aliases in `pkg/gmock/stub.go`**

Add to the Chaos Observability section (after `ModeCombined` const line):

```go
const (
	ModeAlways      = spec.ModeAlways
	ModeProbability = spec.ModeProbability
	ModeNthRequest  = spec.ModeNthRequest
	ModeTimeWindow  = spec.ModeTimeWindow
	ModeCombined    = spec.ModeCombined
	ModeTimeline    = spec.ModeTimeline
)
```

Add after the `FaultVerificationResult` alias:

```go
// --- Fault Timeline types ---

// FaultTimeline is an ordered, deterministic schedule of fault/delay injections.
type FaultTimeline = spec.FaultTimeline

// TimelineEvent declares one fault or delay injection on matching requests.
type TimelineEvent = spec.TimelineEvent

// TimelineTrigger keys a timeline event to a request count or an elapsed time.
type TimelineTrigger = spec.TimelineTrigger
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: compiles.

- [ ] **Step 5: Commit**

```bash
git add internal/spec/spec.go pkg/gmock/stub.go
git commit -m "feat: add fault timeline types to spec and public aliases"
```

---

### Task 2: Timeline runner — validation, counters, Check

**Files:**
- Create: `internal/timeline/timeline.go`
- Test: `internal/timeline/timeline_test.go`

**Interfaces:**
- Consumes: `spec.FaultTimeline`, `spec.TimelineEvent`, `spec.TimelineTrigger`, `spec.FaultDefinition`, `spec.DelayDefinition`, `spec.RequestPattern`; `stub.BuildMatcher` from `internal/stub`; `matcher.CompositeMatcher` from `internal/matcher`.
- Produces: `timeline.NewRunner() *Runner`, `(*Runner).Load(tl *spec.FaultTimeline) error`, `(*Runner).Check(req *http.Request, serverStart time.Time) *Fired`, `(*Runner).Clear()`, `(*Runner).Export() *spec.FaultTimeline`, `timeline.Validate(tl *spec.FaultTimeline) error`, `timeline.Fired{EventIndex, RequestCount int, Fault *spec.FaultDefinition, Delay *spec.DelayDefinition}` — consumed by Tasks 3 and 4.

- [ ] **Step 1: Write the failing tests**

`internal/timeline/timeline_test.go`:

```go
package timeline

import (
	"net/http"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// newRequest builds a GET request for the given path.
func newRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://test"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// TestValidate rejects malformed timelines (table-driven).
func TestValidate(t *testing.T) {
	valid := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 3}, Match: spec.RequestPattern{URLPath: "/a"}},
			{At: &spec.TimelineTrigger{TimeMs: 5000}, Until: &spec.TimelineTrigger{TimeMs: 15000}, Match: spec.RequestPattern{URLPath: "/b"}},
		},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid timeline rejected: %v", err)
	}

	cases := []struct {
		name string
		tl   *spec.FaultTimeline
	}{
		{"nil timeline", nil},
		{"wrong version", &spec.FaultTimeline{Version: 2, Events: valid.Events}},
		{"missing at", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{Match: spec.RequestPattern{URLPath: "/a"}}}}},
		{"at sets both keys", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{At: &spec.TimelineTrigger{Request: 1, TimeMs: 5}, Match: spec.RequestPattern{URLPath: "/a"}}}}},
		{"at sets neither key", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{At: &spec.TimelineTrigger{}, Match: spec.RequestPattern{URLPath: "/a"}}}}},
		{"at negative request", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{At: &spec.TimelineTrigger{Request: -1}, Match: spec.RequestPattern{URLPath: "/a"}}}}},
		{"fault and delay both set", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{At: &spec.TimelineTrigger{Request: 1}, Match: spec.RequestPattern{URLPath: "/a"}, Fault: &spec.FaultDefinition{Type: "error"}, Delay: &spec.DelayDefinition{Type: "fixed", Value: 10}}}}},
		{"until key type mismatch", &spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{{At: &spec.TimelineTrigger{Request: 1}, Until: &spec.TimelineTrigger{TimeMs: 5}, Match: spec.RequestPattern{URLPath: "/a"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.tl); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

// TestCheckIndexSingleShot fires exactly once at the Nth matching request.
func TestCheckIndexSingleShot(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 3}, Match: spec.RequestPattern{URLPath: "/api/payments"}, Fault: &spec.FaultDefinition{Type: "error"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	for i := 0; i < 2; i++ {
		if f := r.Check(newRequest(t, "/api/payments"), start); f != nil {
			t.Fatalf("request %d: expected no fire", i+1)
		}
	}
	f := r.Check(newRequest(t, "/api/payments"), start)
	if f == nil {
		t.Fatal("expected fire on request 3")
	}
	if f.EventIndex != 0 || f.RequestCount != 3 || f.Fault == nil || f.Fault.Type != "error" {
		t.Fatalf("unexpected fired: %+v", f)
	}
	if f := r.Check(newRequest(t, "/api/payments"), start); f != nil {
		t.Fatal("expected no fire after exhaustion")
	}
}

// TestCheckIndexWindow fires for requests in [At.Request, Until.Request].
func TestCheckIndexWindow(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 2}, Until: &spec.TimelineTrigger{Request: 4}, Match: spec.RequestPattern{URLPath: "/api/x"}, Delay: &spec.DelayDefinition{Type: "fixed", Value: 100}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	expectFire := []bool{false, true, true, true, false}
	for i, want := range expectFire {
		f := r.Check(newRequest(t, "/api/x"), start)
		if want && f == nil {
			t.Fatalf("request %d: expected fire", i+1)
		}
		if !want && f != nil {
			t.Fatalf("request %d: expected no fire, got %+v", i+1, f)
		}
	}
}

// TestCheckTimeWindow fires only inside [At.TimeMs, Until.TimeMs).
func TestCheckTimeWindow(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{TimeMs: 5000}, Until: &spec.TimelineTrigger{TimeMs: 15000}, Match: spec.RequestPattern{URLPath: "/api/t"}, Fault: &spec.FaultDefinition{Type: "connection_reset"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}

	// 10s ago: elapsed ~10s -> inside the window.
	inside := time.Now().Add(-10 * time.Second)
	if f := r.Check(newRequest(t, "/api/t"), inside); f == nil {
		t.Fatal("expected fire inside time window")
	}

	// Now: elapsed ~0s -> outside the window.
	outside := time.Now()
	if f := r.Check(newRequest(t, "/api/t"), outside); f != nil {
		t.Fatal("expected no fire outside time window")
	}

	// 30s ago: elapsed ~30s -> past the window; event must be exhausted.
	past := time.Now().Add(-30 * time.Second)
	if f := r.Check(newRequest(t, "/api/t"), past); f != nil {
		t.Fatal("expected no fire after window end")
	}
	if f := r.Check(newRequest(t, "/api/t"), inside); f != nil {
		t.Fatal("expected no fire after exhaustion")
	}
}

// TestCheckNonMatchingRequestDoesNotAdvanceCounter.
func TestCheckNonMatchingRequestDoesNotAdvanceCounter(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 2}, Match: spec.RequestPattern{URLPath: "/api/a"}, Fault: &spec.FaultDefinition{Type: "error"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	// Three requests to another path must not advance the counter.
	for i := 0; i < 3; i++ {
		if f := r.Check(newRequest(t, "/api/b"), start); f != nil {
			t.Fatalf("request %d: unexpected fire", i+1)
		}
	}
	// First matching request: counter 1 -> no fire.
	if f := r.Check(newRequest(t, "/api/a"), start); f != nil {
		t.Fatal("expected no fire on first matching request")
	}
	// Second matching request: fires.
	if f := r.Check(newRequest(t, "/api/a"), start); f == nil {
		t.Fatal("expected fire on second matching request")
	}
}

// TestCheckFirstEventWins applies only the first fired event per request,
// but advances every matching event's counter.
func TestCheckFirstEventWins(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 1}, Match: spec.RequestPattern{URLPath: "/api/z"}, Fault: &spec.FaultDefinition{Type: "error"}},
			{At: &spec.TimelineTrigger{Request: 1}, Match: spec.RequestPattern{URLPath: "/api/z"}, Fault: &spec.FaultDefinition{Type: "empty"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	f := r.Check(newRequest(t, "/api/z"), start)
	if f == nil || f.EventIndex != 0 {
		t.Fatalf("expected first event to win, got %+v", f)
	}
	// Event 0 is exhausted; event 1's counter was still advanced, so its
	// request 1 fires now.
	f = r.Check(newRequest(t, "/api/z"), start)
	if f == nil || f.EventIndex != 1 {
		t.Fatalf("expected second event to fire next, got %+v", f)
	}
}

// TestClearResetsCountersButKeepsEvents re-runs a timeline from scratch.
func TestClearResetsCountersButKeepsEvents(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 1}, Match: spec.RequestPattern{URLPath: "/api/c"}, Fault: &spec.FaultDefinition{Type: "error"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	if f := r.Check(newRequest(t, "/api/c"), start); f == nil {
		t.Fatal("expected first run fire")
	}
	if f := r.Check(newRequest(t, "/api/c"), start); f != nil {
		t.Fatal("expected exhaustion after first run")
	}

	r.Clear()

	if f := r.Check(newRequest(t, "/api/c"), start); f == nil {
		t.Fatal("expected fire after clear")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/timeline/`
Expected: FAIL — package `timeline` does not exist.

- [ ] **Step 3: Write the implementation**

`internal/timeline/timeline.go`:

```go
// Package timeline implements the fault timeline: an ordered, deterministic
// schedule of fault/delay injections. A timeline is a response-pipeline
// modifier — it declares *when* to inject *what* on top of registered stubs.
//
// See docs/superpowers/specs/2026-07-31-fault-timeline-design.md for the
// full design.
package timeline

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sunny809/gochaos/internal/matcher"
	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
)

// Fired describes a timeline event that fired for a request.
type Fired struct {
	// EventIndex is the index into the loaded timeline's Events slice.
	EventIndex int

	// RequestCount is the per-event match counter value at fire time.
	// For time-keyed events this is still the counter of matching requests.
	RequestCount int

	Fault *spec.FaultDefinition
	Delay *spec.DelayDefinition
}

// eventState is the mutable runtime state for one timeline event.
type eventState struct {
	def       spec.TimelineEvent
	matcher   *matcher.CompositeMatcher
	counter   int   // number of matching requests seen
	exhausted bool  // event will never fire again
	fired     []int // counter values at each fire (source for Export)
}

// Runner owns the loaded timeline and its per-event counters.
type Runner struct {
	mu     sync.RWMutex
	name   string
	events []*eventState
}

// NewRunner creates an empty timeline runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Load validates and installs a new timeline, replacing any previous one.
func (r *Runner) Load(tl *spec.FaultTimeline) error {
	if err := Validate(tl); err != nil {
		return err
	}
	events := make([]*eventState, 0, len(tl.Events))
	for _, e := range tl.Events {
		events = append(events, &eventState{
			def:     e,
			matcher: stub.BuildMatcher(e.Match),
		})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.name = tl.Name
	r.events = events
	return nil
}

// Clear resets counters and fired records but keeps the loaded event table,
// so a timeline can be re-run from scratch (e.g. after Reset).
func (r *Runner) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		e.counter = 0
		e.exhausted = false
		e.fired = nil
	}
}

// Check evaluates the timeline for one request and returns the first fired
// event in declaration order, or nil when no event fires. Every event whose
// pattern matches advances its own counter independently, even when an
// earlier event already fired for this request.
func (r *Runner) Check(req *http.Request, serverStart time.Time) *Fired {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsedMs := time.Since(serverStart).Milliseconds()

	var first *Fired
	for i, e := range r.events {
		if e.exhausted {
			continue
		}
		matched, _ := e.matcher.ScoreMatch(req)
		if !matched {
			continue
		}
		e.counter++
		if e.shouldFire(elapsedMs) {
			if first == nil {
				first = &Fired{
					EventIndex:   i,
					RequestCount: e.counter,
					Fault:        e.def.Fault,
					Delay:        e.def.Delay,
				}
			}
		}
	}
	return first
}

// shouldFire decides whether the event fires at its current counter/elapsed
// state and updates the exhausted flag. Callers hold the runner lock.
func (e *eventState) shouldFire(elapsedMs int64) bool {
	if e.def.At == nil {
		return false // unreachable: validation requires At
	}

	// Index-keyed trigger.
	if e.def.At.Request > 0 {
		if e.def.Until != nil && e.def.Until.Request > 0 {
			if e.counter > e.def.Until.Request {
				e.exhausted = true
				return false
			}
			return e.counter >= e.def.At.Request
		}
		if e.counter == e.def.At.Request {
			e.exhausted = true
			return true
		}
		return false
	}

	// Time-keyed trigger (At.TimeMs > 0, guaranteed by validation).
	if e.def.Until != nil && e.def.Until.TimeMs > 0 {
		if elapsedMs >= e.def.Until.TimeMs {
			e.exhausted = true
			return false
		}
		return elapsedMs >= e.def.At.TimeMs
	}
	return elapsedMs >= e.def.At.TimeMs
}

// Validate checks a FaultTimeline against the artifact rules (spec §2).
func Validate(tl *spec.FaultTimeline) error {
	if tl == nil {
		return fmt.Errorf("timeline: nil timeline")
	}
	if tl.Version != 1 {
		return fmt.Errorf("timeline: unsupported version %d (want 1)", tl.Version)
	}
	for i, e := range tl.Events {
		idx := fmt.Sprintf("events[%d]", i)
		if e.At == nil {
			return fmt.Errorf("timeline: %s: at is required", idx)
		}
		if e.At.Request < 0 || e.At.TimeMs < 0 {
			return fmt.Errorf("timeline: %s: at: negative triggers are invalid", idx)
		}
		if (e.At.Request > 0) == (e.At.TimeMs > 0) {
			return fmt.Errorf("timeline: %s: at must set exactly one of request or timeMs", idx)
		}
		if e.Fault != nil && e.Delay != nil {
			return fmt.Errorf("timeline: %s: fault and delay are mutually exclusive", idx)
		}
		if e.Until != nil {
			if e.Until.Request < 0 || e.Until.TimeMs < 0 {
				return fmt.Errorf("timeline: %s: until: negative triggers are invalid", idx)
			}
			if (e.Until.Request > 0) == (e.Until.TimeMs > 0) {
				return fmt.Errorf("timeline: %s: until must set exactly one of request or timeMs", idx)
			}
			if e.Until.Request > 0 && e.At.Request == 0 {
				return fmt.Errorf("timeline: %s: until must use the same key type as at", idx)
			}
			if e.Until.TimeMs > 0 && e.At.TimeMs == 0 {
				return fmt.Errorf("timeline: %s: until must use the same key type as at", idx)
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/timeline/`
Expected: PASS, no races.

- [ ] **Step 5: Commit**

```bash
git add internal/timeline/
git commit -m "feat: timeline runner — validation, per-event counters, Check"
```

---

### Task 3: Timeline export — fired events to artifact

**Files:**
- Modify: `internal/timeline/timeline.go` (add `Export`)
- Test: `internal/timeline/export_test.go`

**Interfaces:**
- Consumes: Task 2's `Runner` + `eventState.fired`.
- Produces: `(*Runner).Export() *spec.FaultTimeline` — consumed by Task 4.

- [ ] **Step 1: Write the failing tests**

`internal/timeline/export_test.go`:

```go
package timeline

import (
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// TestExportSingleShot emits one at trigger with no until.
func TestExportSingleShot(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 3}, Match: spec.RequestPattern{URLPath: "/api/payments"}, Fault: &spec.FaultDefinition{Type: "error"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	for i := 0; i < 3; i++ {
		r.Check(newRequest(t, "/api/payments"), start)
	}

	out := r.Export()
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 exported event, got %d", len(out.Events))
	}
	e := out.Events[0]
	if e.At == nil || e.At.Request != 3 {
		t.Fatalf("expected at.request=3, got %+v", e.At)
	}
	if e.Until != nil {
		t.Fatalf("expected no until for single shot, got %+v", e.Until)
	}
	if e.Fault == nil || e.Fault.Type != "error" {
		t.Fatalf("expected fault preserved, got %+v", e.Fault)
	}
}

// TestExportWindowCollapses consecutive fires into one at/until window.
func TestExportWindowCollapses(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{Request: 2}, Until: &spec.TimelineTrigger{Request: 4}, Match: spec.RequestPattern{URLPath: "/api/x"}, Delay: &spec.DelayDefinition{Type: "fixed", Value: 100}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	for i := 0; i < 5; i++ {
		r.Check(newRequest(t, "/api/x"), start)
	}

	out := r.Export()
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 exported event, got %d", len(out.Events))
	}
	e := out.Events[0]
	if e.At == nil || e.At.Request != 2 {
		t.Fatalf("expected at.request=2, got %+v", e.At)
	}
	if e.Until == nil || e.Until.Request != 4 {
		t.Fatalf("expected until.request=4, got %+v", e.Until)
	}
	if e.Delay == nil || e.Delay.Value != 100 {
		t.Fatalf("expected delay preserved, got %+v", e.Delay)
	}
}

// TestExportTimeEventBecomesRequestKeyed converts time triggers to the
// request counter value at fire time (spec §4).
func TestExportTimeEventBecomesRequestKeyed(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			{At: &spec.TimelineTrigger{TimeMs: 5000}, Until: &spec.TimelineTrigger{TimeMs: 15000}, Match: spec.RequestPattern{URLPath: "/api/t"}, Fault: &spec.FaultDefinition{Type: "connection_reset"}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	inside := time.Now().Add(-10 * time.Second)

	for i := 0; i < 4; i++ {
		r.Check(newRequest(t, "/api/t"), inside)
	}

	out := r.Export()
	if len(out.Events) != 1 {
		t.Fatalf("expected 1 exported event, got %d", len(out.Events))
	}
	e := out.Events[0]
	if e.At == nil || e.At.Request != 1 || e.At.TimeMs != 0 {
		t.Fatalf("expected at.request=1 (request-keyed), got %+v", e.At)
	}
	if e.Until == nil || e.Until.Request != 4 {
		t.Fatalf("expected until.request=4, got %+v", e.Until)
	}
}

// TestExportEmptyWhenNothingFired returns a versioned artifact with no events.
func TestExportEmptyWhenNothingFired(t *testing.T) {
	r := NewRunner()
	if err := r.Load(&spec.FaultTimeline{Version: 1, Events: []spec.TimelineEvent{
		{At: &spec.TimelineTrigger{Request: 1}, Match: spec.RequestPattern{URLPath: "/api/n"}, Fault: &spec.FaultDefinition{Type: "error"}},
	}}); err != nil {
		t.Fatal(err)
	}

	out := r.Export()
	if out.Version != 1 {
		t.Fatalf("expected version 1, got %d", out.Version)
	}
	if len(out.Events) != 0 {
		t.Fatalf("expected no exported events, got %d", len(out.Events))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/timeline/ -run TestExport`
Expected: FAIL — `r.Export undefined`.

- [ ] **Step 3: Write the implementation**

Append to `internal/timeline/timeline.go`:

```go
// Export assembles a FaultTimeline artifact from the events that actually
// fired (spec §4). Only fired events are included; time-keyed triggers are
// converted to request-keyed triggers using the counter value at fire time,
// and consecutive fires collapse into a single at/until window.
func (r *Runner) Export() *spec.FaultTimeline {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tl := &spec.FaultTimeline{Version: 1, Name: r.name}
	for _, e := range r.events {
		if len(e.fired) == 0 {
			continue
		}
		at := &spec.TimelineTrigger{Request: e.fired[0]}
		var until *spec.TimelineTrigger
		if last := e.fired[len(e.fired)-1]; last > e.fired[0] {
			until = &spec.TimelineTrigger{Request: last}
		}
		tl.Events = append(tl.Events, spec.TimelineEvent{
			At:    at,
			Until: until,
			Match: e.def.Match,
			Fault: e.def.Fault,
			Delay: e.def.Delay,
		})
	}
	return tl
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/timeline/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/timeline/
git commit -m "feat: timeline export — fired events to replayable artifact"
```

---

### Task 4: Server wiring — load/replay APIs + serveMock hook

**Files:**
- Modify: `pkg/gmock/server.go` (interface + struct + `NewServer` + `serveMock` + `Reset`)
- Create: `pkg/gmock/timeline.go`
- Test: `test/integration/timeline_test.go`

**Interfaces:**
- Consumes: Task 2/3 `timeline.Runner` (`NewRunner`, `Load`, `Check`, `Clear`, `Export`), `timeline.Fired`; `spec.FaultInjectionEntry`.
- Produces: `Server.LoadTimelineYAML([]byte) error`, `Server.LoadTimeline(*FaultTimeline) error`, `Server.ExportTimeline() (*FaultTimeline, error)` on the `gmock.Server` interface — the public feature surface.

- [ ] **Step 1: Add the interface methods and runner field**

In `pkg/gmock/server.go` interface (after `NearMiss`):

```go
	// LoadTimelineYAML loads a fault timeline from YAML (declare or replay).
	LoadTimelineYAML(data []byte) error

	// LoadTimeline installs a fault timeline from a struct (declare or replay).
	LoadTimeline(tl *FaultTimeline) error

	// ExportTimeline returns a fault timeline artifact describing every event
	// that fired so far (record). Triggers are request-keyed, so the artifact
	// can be loaded into another server for deterministic replay.
	ExportTimeline() (*FaultTimeline, error)
```

In the `mockServer` struct (after `responseWriter`):

```go
	timelineRunner *timeline.Runner
```

Add `timeline` to the import block of `server.go`:

```go
	"github.com/sunny809/gochaos/internal/stub"
	"github.com/sunny809/gochaos/internal/timeline"
```

In `NewServer`, add to the returned struct literal (after `responseWriter`):

```go
		timelineRunner: timeline.NewRunner(),
```

- [ ] **Step 2: Write the failing integration tests**

`test/integration/timeline_test.go`:

```go
package integration_test

import (
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestTimelineIndexSequence fires a declared timeline exactly on schedule,
// and the injections are visible to VerifyFaultsInjected.
func TestTimelineIndexSequence(t *testing.T) {
	server, stop := startServer(t)
	defer stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/payments"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
		},
	})

	err := server.LoadTimelineYAML([]byte(`
version: 1
name: sequence
events:
  - at: { request: 3 }
    until: { request: 5 }
    match: { method: GET, urlPath: /api/payments }
    fault: { type: error }
`))
	if err != nil {
		t.Fatalf("load timeline: %v", err)
	}

	// Status codes: 200, 200, 500(error), 500, 500, 200.
	want := []int{200, 200, 500, 500, 500, 200}
	for i, wantStatus := range want {
		resp, err := http.Get(server.URL() + "/api/payments")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != wantStatus {
			t.Fatalf("request %d: got status %d, want %d", i, resp.StatusCode, wantStatus)
		}
	}

	result := server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType:      "error",
		ActivationMode: "timeline",
	}, 3)
	if !result.Matched {
		t.Fatalf("expected 3 timeline faults: %v", result.Errors)
	}
}

// TestTimelineExportReplayClosedLoop: probabilistic chaos -> export -> exact
// deterministic replay on a fresh server (no RNG involved).
func TestTimelineExportReplayClosedLoop(t *testing.T) {
	const n = 30

	drive := func(server gmock.Server) []int {
		statuses := make([]int, 0, n)
		for i := 0; i < n; i++ {
			resp, err := http.Get(server.URL() + "/api/faulty")
			if err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
			resp.Body.Close()
			statuses = append(statuses, resp.StatusCode)
		}
		return statuses
	}

	// Run 1: probabilistic chaos with a seed.
	serverA, stopA := startServer(t, gmock.WithRandSeed(42))
	serverA.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/faulty"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:       "error",
				Activation: &gmock.Activation{Probability: 0.5},
			},
		},
	})
	gotA := drive(serverA)

	tl, err := serverA.ExportTimeline()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(tl.Events) == 0 {
		t.Fatal("expected exported timeline to contain fired events")
	}
	stopA()

	// Run 2: replay the recorded timeline with a fresh, fault-free stub.
	serverB, stopB := startServer(t)
	defer stopB()
	serverB.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/faulty"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
		},
	})
	if err := serverB.LoadTimeline(tl); err != nil {
		t.Fatalf("replay load: %v", err)
	}
	gotB := drive(serverB)

	for i := range gotA {
		if gotA[i] != gotB[i] {
			t.Fatalf("request %d: run A=%d run B=%d (sequences differ)", i, gotA[i], gotB[i])
		}
	}
}

// TestTimelineDelayEventIsRecorded: delay-only events appear in the fault log
// as FaultType "delay" with DelayMs set.
func TestTimelineDelayEventIsRecorded(t *testing.T) {
	server, stop := startServer(t)
	defer stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/slow"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `{"ok":true}`},
	})

	if err := server.LoadTimelineYAML([]byte(`
version: 1
events:
  - at: { request: 1 }
    match: { method: GET, urlPath: /api/slow }
    delay: { type: fixed, value: 50 }
`)); err != nil {
		t.Fatalf("load timeline: %v", err)
	}

	resp, err := http.Get(server.URL() + "/api/slow")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	result := server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType:      "delay",
		ActivationMode: "timeline",
	}, 1)
	if !result.Matched {
		t.Fatalf("expected 1 timeline delay: %v", result.Errors)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./test/integration/ -run TestTimeline`
Expected: FAIL — compile error (`LoadTimelineYAML` etc. undefined).

- [ ] **Step 4: Implement the public API**

Create `pkg/gmock/timeline.go`:

```go
package gmock

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// LoadTimelineYAML loads a fault timeline from YAML (declare or replay).
func (s *mockServer) LoadTimelineYAML(data []byte) error {
	var tl FaultTimeline
	if err := yaml.Unmarshal(data, &tl); err != nil {
		return fmt.Errorf("gmock: invalid timeline YAML: %w", err)
	}
	return s.LoadTimeline(&tl)
}

// LoadTimeline installs a fault timeline from a struct (declare or replay).
func (s *mockServer) LoadTimeline(tl *FaultTimeline) error {
	if err := s.timelineRunner.Load(tl); err != nil {
		return fmt.Errorf("gmock: invalid timeline: %w", err)
	}
	return nil
}

// ExportTimeline returns a fault timeline artifact describing every event
// that fired so far (record). Triggers are request-keyed, so the artifact can
// be loaded into another server for deterministic replay.
func (s *mockServer) ExportTimeline() (*FaultTimeline, error) {
	return s.timelineRunner.Export(), nil
}
```

- [ ] **Step 5: Hook the runner into `serveMock`**

In `pkg/gmock/server.go`, `serveMock`, insert after the `requestLog.Record` line (before the `if !matched {` 404 branch):

```go
	// Check the fault timeline (if loaded). A fired event takes precedence
	// over the stub's own fault/delay for this request, and can even apply
	// when no stub matched (error-type faults need no response body).
	if fired := s.timelineRunner.Check(r, s.startTime); fired != nil {
		s.serveTimelineFired(w, r, result, matched, stubID, fired)
		return
	}
```

Append `serveTimelineFired` at the end of `pkg/gmock/server.go`:

```go
// serveTimelineFired writes a response for a request on which a timeline
// event fired. The event's fault/delay replaces the stub's own; when no stub
// matched, a default 200 stub is synthesized so the effect still applies.
func (s *mockServer) serveTimelineFired(w http.ResponseWriter, r *http.Request, result *spec.MatchResult, matched bool, stubID string, fired *timeline.Fired) {
	def := spec.StubDefinition{Response: spec.ResponseDefinition{Status: http.StatusOK}}
	if matched {
		def = *result.Stub // copy: the event overrides the stub's effects
	}
	def.Response.Fault = fired.Fault
	def.Response.Delay = fired.Delay

	var hitCount uint64
	if matched {
		hitCount = s.registry.IncrementHitCount(result.Stub.ID)
	}

	faultInfo, err := s.responseWriter.WriteResponse(w, &def, r, corsOptsFromConfig(s.config.CORSOptions), hitCount, s.startTime)
	if err != nil {
		s.logger.Warn("failed to write timeline response", "stub", stubID, "error", err)
	}

	if def.Response.Delay != nil {
		s.metrics.faultsDelayed.Add(1)
	}
	if faultInfo.Injected {
		s.metrics.faultsInjected.Add(1)
	}

	faultType := ""
	if fired.Fault != nil {
		faultType = fired.Fault.Type
	}
	delayMs := 0
	if fired.Delay != nil {
		faultType = "delay"
		delayMs = fired.Delay.Value
	}
	s.faultLog.Record(spec.FaultInjectionEntry{
		StubID:         stubID,
		FaultType:      faultType,
		DelayMs:        delayMs,
		ActivatedAt:    time.Now(),
		RequestMethod:  r.Method,
		RequestPath:    r.URL.Path,
		ActivationMode: spec.ModeTimeline,
		TimelineEvent:  fired.EventIndex + 1,
	})
}
```

- [ ] **Step 6: Wire `Reset`**

In `pkg/gmock/server.go`, `Reset`, after `s.callbackLog.Clear()`:

```go
	s.timelineRunner.Clear()
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test -race ./pkg/gmock/ ./test/integration/`
Expected: PASS — including the three new `TestTimeline*` tests.

- [ ] **Step 8: Commit**

```bash
git add pkg/gmock/server.go pkg/gmock/timeline.go test/integration/timeline_test.go
git commit -m "feat: server wiring — LoadTimeline/ExportTimeline + serveMock hook"
```

---

### Task 5: Chaos report serializers — JUnit XML + JSON

**Files:**
- Create: `internal/report/junit.go`, `internal/report/json.go`
- Test: `internal/report/report_test.go`

**Interfaces:**
- Consumes: `spec.FaultInjectionEntry`.
- Produces: `report.JUnit(entries []spec.FaultInjectionEntry, suiteName string) []byte`, `report.JSON(entries []spec.FaultInjectionEntry, suiteName string) ([]byte, error)`, `report.JSONReport` — consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

`internal/report/report_test.go`:

```go
package report

import (
	"strings"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

func sampleEntries() []spec.FaultInjectionEntry {
	at := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	return []spec.FaultInjectionEntry{
		{
			StubID:         "stub-1",
			FaultType:      "error",
			ActivatedAt:    at,
			RequestMethod:  "POST",
			RequestPath:    "/api/payments",
			ActivationMode: spec.ModeTimeline,
			TimelineEvent:  1,
		},
		{
			StubID:         "stub-2",
			FaultType:      "delay",
			DelayMs:        2000,
			ActivatedAt:    at,
			RequestMethod:  "GET",
			RequestPath:    "/api/inventory",
			ActivationMode: spec.ModeTimeline,
			TimelineEvent:  2,
		},
	}
}

// TestJUnit renders each injection as a failing testcase.
func TestJUnit(t *testing.T) {
	out := string(JUnit(sampleEntries(), "gmock-chaos"))

	for _, want := range []string{
		`<testsuite name="gmock-chaos" tests="2" failures="2"`,
		`<testcase name="error" classname="stub-1"`,
		`<testcase name="delay" classname="stub-2"`,
		`<failure message="fault injected on POST /api/payments`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JUnit output missing %q:\n%s", want, out)
		}
	}
}

// TestJUnitEmpty renders a valid empty suite.
func TestJUnitEmpty(t *testing.T) {
	out := string(JUnit(nil, "gmock-chaos"))
	if !strings.Contains(out, `tests="0" failures="0"`) {
		t.Fatalf("expected empty suite, got:\n%s", out)
	}
}

// TestJSON renders the envelope with all entries.
func TestJSON(t *testing.T) {
	out, err := JSON(sampleEntries(), "gmock-chaos")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"suite": "gmock-chaos"`,
		`"faultType": "error"`,
		`"activationMode": "timeline"`,
		`"timelineEvent": 1`,
		`"delayMs": 2000`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("JSON output missing %q:\n%s", want, string(out))
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/report/`
Expected: FAIL — package `report` does not exist.

- [ ] **Step 3: Write the implementation**

`internal/report/junit.go`:

```go
// Package report serializes chaos evidence from the fault injection log into
// CI-consumable formats (JUnit XML and JSON).
package report

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// JUnit renders the fault injection log as a JUnit XML test suite. Each
// injection becomes a failing testcase named "<faultType> on <stubID>", so the
// report reads as chaos evidence in CI dashboards: tests == injections.
func JUnit(entries []spec.FaultInjectionEntry, suiteName string) []byte {
	type failure struct {
		Message string `xml:"message,attr"`
		Type    string `xml:"type,attr"`
		Body    string `xml:",chardata"`
	}
	type testcase struct {
		Name      string    `xml:"name,attr"`
		ClassName string    `xml:"classname,attr"`
		Failures  []failure `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Cases    []testcase `xml:"testcase"`
	}

	suite := testsuite{Name: suiteName, Tests: len(entries), Failures: len(entries)}
	for _, e := range entries {
		faultType := e.FaultType
		if faultType == "" {
			faultType = "delay"
		}
		suite.Cases = append(suite.Cases, testcase{
			Name:      faultType,
			ClassName: e.StubID,
			Failures: []failure{{
				Message: fmt.Sprintf("fault injected on %s %s at %s",
					e.RequestMethod, e.RequestPath, e.ActivatedAt.Format(time.RFC3339)),
				Type: faultType,
				Body: faultType,
			}},
		})
	}

	out, _ := xml.MarshalIndent(suite, "", "  ")
	return append([]byte(xml.Header), out...)
}
```

`internal/report/json.go`:

```go
package report

import (
	"encoding/json"

	"github.com/sunny809/gochaos/internal/spec"
)

// JSONReport is the structured report envelope for the JSON format.
type JSONReport struct {
	Suite   string                     `json:"suite"`
	Entries []spec.FaultInjectionEntry `json:"entries"`
}

// JSON renders the fault injection log as a JSON report envelope.
func JSON(entries []spec.FaultInjectionEntry, suiteName string) ([]byte, error) {
	return json.MarshalIndent(JSONReport{Suite: suiteName, Entries: entries}, "", "  ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/report/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/report/
git commit -m "feat: chaos report serializers — JUnit XML + JSON"
```

---

### Task 6: Admin report endpoint + `gmock report` CLI

**Files:**
- Create: `internal/admin/report.go`
- Modify: `internal/admin/handler.go` (route + doc comment)
- Create: `cmd/gmock/report.go`, `cmd/gmock/report_test.go`
- Modify: `cmd/gmock/root.go` (register command)

**Interfaces:**
- Consumes: Task 5 `report.JUnit` / `report.JSON`; `h.faultLog.List()`.
- Produces: `GET /__admin/report?format=junit|json` endpoint; `gmock report --format junit|json --admin-url` command.

- [ ] **Step 1: Write the failing tests**

`cmd/gmock/report_test.go` (mirrors the httptest pattern in `stub_test.go` — no gmock dependency in CLI tests):

```go
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReportCommandJUnit fetches and prints the JUnit report.
// Reuses the package-level testMu and commonAdminURL from stub_test.go / stub.go.
func TestReportCommandJUnit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__admin/report" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "junit" {
			t.Errorf("expected format=junit, got %q", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<testsuite name="gmock-chaos" tests="1" failures="1"/>`))
	}))
	defer ts.Close()

	testMu.Lock()
	defer testMu.Unlock()
	commonAdminURL = ts.URL
	commonClient = ts.Client()

	cmd := newReportCmd()
	cmd.SetArgs([]string{"--format", "junit"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "<testsuite name=\"gmock-chaos\"") {
		t.Fatalf("expected junit output, got: %s", buf.String())
	}
}

// TestReportCommandRejectsBadFormat surfaces the server's 400 response.
func TestReportCommandRejectsBadFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid format: must be junit or json"}`))
	}))
	defer ts.Close()

	testMu.Lock()
	defer testMu.Unlock()
	commonAdminURL = ts.URL
	commonClient = ts.Client()

	cmd := newReportCmd()
	cmd.SetArgs([]string{"--format", "bogus"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for bad format")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gmock/`
Expected: FAIL — `newReportCmd` undefined.

- [ ] **Step 3: Implement the CLI command**

`cmd/gmock/report.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"
)

// newReportCmd creates the `gmock report` subcommand for exporting chaos
// evidence (JUnit XML or JSON) from a running server.
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Export chaos evidence from a running server (JUnit XML or JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			adminURL, _ := cmd.Flags().GetString("admin-url")
			format, _ := cmd.Flags().GetString("format")

			u, err := url.Parse(adminURL + "/__admin/report")
			if err != nil {
				return fmt.Errorf("invalid admin URL: %w", err)
			}
			q := u.Query()
			q.Set("format", format)
			u.RawQuery = q.Encode()

			resp, err := commonClient.Get(u.String())
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
			}

			body, _ := io.ReadAll(resp.Body)
			if format == "json" {
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, body, "", "  "); err == nil {
					body = pretty.Bytes()
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	}

	cmd.Flags().String("admin-url", "http://localhost:8080",
		"Base URL of the running gmock server admin API")
	cmd.Flags().String("format", "json", "Report format: 'json' or 'junit'")

	return cmd
}
```

In `cmd/gmock/root.go`, after `cmd.AddCommand(newRequestsCmd())`:

```go
	cmd.AddCommand(newReportCmd())
```

- [ ] **Step 4: Implement the admin endpoint**

`internal/admin/report.go`:

```go
package admin

import (
	"log/slog"
	"net/http"

	"github.com/sunny809/gochaos/internal/report"
)

// reportHandler serves GET /__admin/report — chaos evidence export.
// Formats: junit (JUnit XML), json (structured envelope). Default: json.
func (h *Handler) reportHandler(w http.ResponseWriter, r *http.Request) {
	if h.metrics != nil {
		h.metrics.Add("admin_operations", 1)
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	entries := h.faultLog.List()
	switch format {
	case "junit":
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(report.JUnit(entries, "gmock-chaos")); err != nil {
			slog.Warn("failed to write junit report", "error", err)
		}
	case "json":
		body, err := report.JSON(entries, "gmock-chaos")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode report")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			slog.Warn("failed to write json report", "error", err)
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid format: must be junit or json")
	}
}
```

In `internal/admin/handler.go`, add a route case (after the `metrics` case, before `default`):

```go
	case path == Prefix+"report":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		h.reportHandler(w, r)
```

Add to the API endpoints doc comment (after the metrics line):

```go
//	GET    /__admin/report              Export chaos evidence (JUnit XML or JSON)
```

- [ ] **Step 5: Run all tests to verify they pass**

Run: `go test -race ./internal/admin/ ./cmd/gmock/ ./internal/report/ ./test/integration/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/report.go internal/admin/handler.go cmd/gmock/report.go cmd/gmock/report_test.go cmd/gmock/root.go
git commit -m "feat: GET /__admin/report endpoint + gmock report CLI"
```

---

### Task 7: Docs, spec corrections, tutorial 4, full verification

**Files:**
- Create: `docs/features/fault-timeline.md`, `docs/features/chaos-report.md`
- Create: `docs/tutorials/04-timeline-replay.md`, `examples/tutorial/04-timeline-replay/main_test.go`
- Modify: `docs/superpowers/specs/2026-07-31-fault-timeline-design.md` (correct `path:` → `urlPath:`, `reset` → `connection_reset` in the YAML examples)
- Modify: `ROADMAP.md` (Phase 4 table: add timeline row)

**Interfaces:**
- Consumes: the public API from Task 4 (`LoadTimelineYAML`, `LoadTimeline`, `ExportTimeline`) and Task 6 (`GET /__admin/report`).
- Produces: user-facing docs + a runnable tutorial example. Everything in this task is `git add -f` (docs/ is gitignored).

- [ ] **Step 1: Write the tutorial example (compilable, tested)**

`examples/tutorial/04-timeline-replay/main_test.go`:

```go
package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestRecordAndReplayChaos shows the full loop: run probabilistic chaos once,
// export the fault sequence that actually fired, then replay it exactly in CI.
func TestRecordAndReplayChaos(t *testing.T) {
	// --- Phase 1: record (exploration run, seeded for debuggability) ---
	serverA := gmock.NewServer(gmock.WithPort(0), gmock.WithRandSeed(42))
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
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

	recorded := make([]int, 0, 20)
	for i := 0; i < 20; i++ {
		resp, err := http.Get(serverA.URL() + "/api/payments")
		if err == nil {
			resp.Body.Close()
		}
		if resp != nil {
			recorded = append(recorded, resp.StatusCode)
		}
	}

	tl, err := serverA.ExportTimeline()
	if err != nil {
		t.Fatal(err)
	}
	if err := serverA.Stop(); err != nil {
		t.Fatal(err)
	}

	// --- Phase 2: replay (CI — deterministic, no RNG involved) ---
	serverB := gmock.NewServer(gmock.WithPort(0))
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	// The stub is registered fault-free; the timeline supplies the faults.
	serverB.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/payments"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `{"status":"ok"}`},
	})
	if err := serverB.LoadTimeline(tl); err != nil {
		t.Fatal(err)
	}

	replayed := make([]int, 0, 20)
	for i := 0; i < 20; i++ {
		resp, err := http.Get(serverB.URL() + "/api/payments")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		replayed = append(replayed, resp.StatusCode)
	}

	// The replayed sequence must match the recorded one request for request.
	if len(recorded) != len(replayed) {
		t.Fatalf("sequence length mismatch: recorded %d, replayed %d", len(recorded), len(replayed))
	}
	for i := range recorded {
		if recorded[i] != replayed[i] {
			t.Errorf("request %d: recorded %d, replayed %d", i, recorded[i], replayed[i])
		}
	}

	// The timeline injections are visible to the verification API.
	result := serverB.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType:      "error",
		ActivationMode: "timeline",
	}, 1)
	if !result.Matched {
		t.Errorf("expected timeline faults in replay: %v", result.Errors)
	}

	fmt.Printf("recorded: %v\nreplayed: %v\n", recorded, replayed)
}
```

Verify it compiles and passes: `go test ./examples/tutorial/04-timeline-replay/`

- [ ] **Step 2: Write `docs/features/fault-timeline.md`**

Structure (concise, feature-doc style matching `docs/features/advanced-chaos.md`):
1. **What it is** — the burst script: an ordered schedule of fault/delay injections; declare / record / replay / report over one YAML artifact.
2. **Declare** — YAML example (with correct `urlPath` keys and `connection_reset`), the `at`/`until` trigger rules, index vs time keying, first-event-wins, automatic recovery on exhaustion.
3. **Record** — `ExportTimeline()` after a seeded run; time triggers export as request indexes; consecutive fires collapse to a window.
4. **Replay** — `LoadTimelineYAML` / `LoadTimeline`; determinism is index-keyed and independent of the RNG; same request sequence → same injection sequence.
5. **API reference** — the three methods + the YAML schema table.
6. **Relationship to Activation** — timeline is the orchestration layer; resident stub faults coexist; a fired event replaces the stub's fault/delay for that request.

- [ ] **Step 3: Write `docs/features/chaos-report.md`**

Structure:
1. **What it is** — fault-log snapshot as CI-visible evidence.
2. **JUnit XML** — `GET /__admin/report?format=junit`; one failing testcase per injection; how it renders in CI dashboards.
3. **JSON** — `GET /__admin/report?format=json`; the envelope shape.
4. **CLI** — `gmock report --format junit > chaos-report.xml`.
5. **Example** — a GitHub Actions step that uploads the report as a test artifact.

- [ ] **Step 4: Fix the design spec's YAML examples**

In `docs/superpowers/specs/2026-07-31-fault-timeline-design.md`, in the §2 artifact example and §3 example, change `path: /api/payments` → `urlPath: /api/payments` and `fault: { type: reset }` → `fault: { type: connection_reset }` (two occurrences of `reset`).

- [ ] **Step 5: Write the tutorial**

`docs/tutorials/04-timeline-replay.md` — follow the structure of `docs/tutorials/02-ci-gateable.md`:
1. **Why** — probabilistic chaos is only CI-safe when the failure sequence is a versioned artifact; RNG changes shouldn't break your CI.
2. **The loop** — record locally (seeded), export, commit, replay in CI.
3. **Code walkthrough** — reference `examples/tutorial/04-timeline-replay/main_test.go` section by section.
4. **Report evidence** — curl the JUnit report and upload it as a CI artifact.
5. **Try it** — `go test ./examples/tutorial/04-timeline-replay/`.

- [ ] **Step 6: Update ROADMAP.md**

In Phase 4 (v1.1) table, after the W1-W4 row, add:

```
| T1-T3 | Fault Timeline (declare/record/replay) + chaos report | ~1 week | Recorded timelines stay CI-stable across RNG changes (spec 2026-07-31) |
```

- [ ] **Step 7: Full verification**

Run: `gofmt -l .` → must print nothing.
Run: `go vet ./...` → clean.
Run: `go test -race ./...` → all pass.

- [ ] **Step 8: Commit (docs/ is gitignored — use -f)**

```bash
git add -f docs/features/fault-timeline.md docs/features/chaos-report.md docs/tutorials/04-timeline-replay.md ROADMAP.md
git add -f examples/tutorial/04-timeline-replay/main_test.go docs/superpowers/specs/2026-07-31-fault-timeline-design.md
git commit -m "docs: fault timeline + chaos report feature docs, tutorial 4, roadmap update"
```

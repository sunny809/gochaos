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
// event in declaration order, or nil when no event fires. The scan stops at
// the first fired event: events declared after it are re-evaluated on
// subsequent requests rather than being silently consumed, so every fired
// event's effect is eventually applied to a request.
func (r *Runner) Check(req *http.Request, serverStart time.Time) *Fired {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsedMs := time.Since(serverStart).Milliseconds()

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
			e.fired = append(e.fired, e.counter)
			return &Fired{
				EventIndex:   i,
				RequestCount: e.counter,
				Fault:        e.def.Fault,
				Delay:        e.def.Delay,
			}
		}
	}
	return nil
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
	// Time single-shot: fire on the first matching request at/after
	// At.TimeMs, then exhaust (mirrors the index single-shot pattern).
	if elapsedMs >= e.def.At.TimeMs {
		e.exhausted = true
		return true
	}
	return false
}

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
		if e.Fault == nil && e.Delay == nil {
			return fmt.Errorf("timeline: %s: event must set exactly one of fault or delay", idx)
		}
		if e.Fault != nil && e.Fault.Activation != nil {
			return fmt.Errorf("timeline: %s: activation is not supported on timeline event faults (the event table decides when)", idx)
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
			if e.Until.Request > 0 && e.Until.Request <= e.At.Request {
				return fmt.Errorf("timeline: %s: until.request must be greater than at.request (inverted window)", idx)
			}
			if e.Until.TimeMs > 0 && e.Until.TimeMs <= e.At.TimeMs {
				return fmt.Errorf("timeline: %s: until.timeMs must be greater than at.timeMs (inverted window)", idx)
			}
		}
	}
	return nil
}

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
			// Window [5000, 45000): wide enough that a slow test runner cannot
			// exit the window mid-check (see TestCheckTimeWindow).
			{At: &spec.TimelineTrigger{TimeMs: 5000}, Until: &spec.TimelineTrigger{TimeMs: 45000}, Match: spec.RequestPattern{URLPath: "/api/t"}, Fault: &spec.FaultDefinition{Type: "connection_reset"}},
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

// TestExportCopiesFaultDelay: the exported artifact must not alias the loaded
// timeline's fault/delay pointers. Check returns those live pointers on every
// firing request, so mutating the exported artifact must not change what the
// runner injects next.
func TestExportCopiesFaultDelay(t *testing.T) {
	r := NewRunner()
	tl := &spec.FaultTimeline{
		Version: 1,
		Events: []spec.TimelineEvent{
			// Windows [1,3]: the events fire again after the export, so the
			// mutation test can observe the runner's live definitions.
			{At: &spec.TimelineTrigger{Request: 1}, Until: &spec.TimelineTrigger{Request: 3}, Match: spec.RequestPattern{URLPath: "/api/f"}, Fault: &spec.FaultDefinition{Type: "error"}},
			{At: &spec.TimelineTrigger{Request: 1}, Until: &spec.TimelineTrigger{Request: 3}, Match: spec.RequestPattern{URLPath: "/api/d"}, Delay: &spec.DelayDefinition{Type: "fixed", Value: 100}},
		},
	}
	if err := r.Load(tl); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r.Check(newRequest(t, "/api/f"), start)
	r.Check(newRequest(t, "/api/d"), start)

	out := r.Export()
	if len(out.Events) != 2 {
		t.Fatalf("expected 2 exported events, got %d", len(out.Events))
	}

	// Mutate the exported definitions; the runner must still inject the
	// original values on its next firing requests.
	out.Events[0].Fault.Type = "empty"
	out.Events[1].Delay.Value = 999

	f := r.Check(newRequest(t, "/api/f"), start)
	if f == nil || f.Fault == nil || f.Fault.Type != "error" {
		t.Fatalf("exported fault aliased the live definition: got %+v", f)
	}
	d := r.Check(newRequest(t, "/api/d"), start)
	if d == nil || d.Delay == nil || d.Delay.Value != 100 {
		t.Fatalf("exported delay aliased the live definition: got %+v", d)
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

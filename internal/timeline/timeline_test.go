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

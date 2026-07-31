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

// TestJSONEmpty renders an empty entries array (never null) — script
// consumers do `.entries[]` and must see an array.
func TestJSONEmpty(t *testing.T) {
	out, err := JSON(nil, "gmock-chaos")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"entries": []`) {
		t.Fatalf("expected empty entries array, got:\n%s", string(out))
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

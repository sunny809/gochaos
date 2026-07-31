package integration_test

import (
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
	"gopkg.in/yaml.v3"
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

// TestTimelineRateLimitRecorded: stub-driven rate_limit injections are
// recorded, so the exported artifact contains a rate_limit event at the
// trigger position of the rate-limited request.
func TestTimelineRateLimitRecorded(t *testing.T) {
	server, stop := startServer(t)
	defer stop()

	server.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/burst"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:          "rate_limit",
				AfterRequests: 2, // warm-up: first 2 requests are served
				PerSecond:     2, // bucket of 2 tokens then allows 2 more
			},
		},
	})

	// Tight loop: requests 1-2 warm-up, 3-4 consume the initial tokens, and
	// 5+ hit the empty bucket (the 2 tokens/s refill cannot keep up), so the
	// first 429 lands on request 5.
	firstLimited := 0
	for i := 1; i <= 8; i++ {
		resp, err := http.Get(server.URL() + "/api/burst")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && firstLimited == 0 {
			firstLimited = i
		}
	}
	if firstLimited == 0 {
		t.Fatal("expected at least one rate-limited request")
	}

	tl, err := server.ExportTimeline()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(tl.Events) != 1 {
		t.Fatalf("expected exactly 1 recorded event, got %d", len(tl.Events))
	}
	e := tl.Events[0]
	if e.Fault == nil || e.Fault.Type != "rate_limit" {
		t.Fatalf("expected rate_limit fault in exported event, got %+v", e.Fault)
	}
	if e.At == nil || e.At.Request != firstLimited {
		t.Fatalf("expected rate_limit event at trigger %d (first 429), got %+v", firstLimited, e.At)
	}
}

// TestTimelineRateLimitReplayReproduces429s: a recorded rate_limit burst
// replays as actual 429s on a fresh server. Pins the serveTimelineFired
// rate_limit branch (WriteResponse alone treats rate_limit as a no-op), so
// the artifact reproduces the 429s the client experienced, not normal
// responses. The replay server has no rate_limit stub — the 429s come from
// the loaded timeline event, not a token bucket.
func TestTimelineRateLimitReplayReproduces429s(t *testing.T) {
	const n = 8

	drive := func(server gmock.Server) []int {
		statuses := make([]int, 0, n)
		for i := 0; i < n; i++ {
			resp, err := http.Get(server.URL() + "/api/burst")
			if err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
			resp.Body.Close()
			statuses = append(statuses, resp.StatusCode)
		}
		return statuses
	}

	// Run 1: record. Tight loop so the token bucket cannot refill between
	// requests: 1-2 warm-up, 3-4 consume the initial tokens, 5+ are limited.
	serverA, stopA := startServer(t)
	serverA.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/burst"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
			Fault: &gmock.FaultDefinition{
				Type:          "rate_limit",
				AfterRequests: 2,
				PerSecond:     2,
			},
		},
	})
	gotA := drive(serverA)
	limited := 0
	for _, s := range gotA {
		if s == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		stopA()
		t.Fatal("run A: expected at least one rate-limited request")
	}

	tl, err := serverA.ExportTimeline()
	if err != nil {
		stopA()
		t.Fatalf("export: %v", err)
	}
	if len(tl.Events) == 0 {
		stopA()
		t.Fatal("expected exported timeline to contain fired events")
	}
	data, err := yaml.Marshal(tl)
	if err != nil {
		stopA()
		t.Fatalf("marshal: %v", err)
	}
	stopA()

	// Run 2: replay the artifact into a fresh, fault-free server. The 429s
	// must come from the timeline events only.
	serverB, stopB := startServer(t)
	defer stopB()
	serverB.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/burst"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
		},
	})
	if err := serverB.LoadTimelineYAML(data); err != nil {
		t.Fatalf("replay load: %v", err)
	}
	gotB := drive(serverB)

	for i := range gotA {
		if gotA[i] != gotB[i] {
			t.Fatalf("request %d: run A=%d run B=%d (sequences differ)", i, gotA[i], gotB[i])
		}
	}
}

// TestTimelineRateLimitDeclaredFires: a declared timeline event with a
// rate_limit fault produces an actual 429 (not the normal response) and
// lands in the fault log as a timeline injection. Pins the declare path,
// which previously went through the rate_limit no-op in WriteResponse.
func TestTimelineRateLimitDeclaredFires(t *testing.T) {
	server, stop := startServer(t)
	defer stop()

	server.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/declared"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK, Body: `{"ok":true}`},
	})

	if err := server.LoadTimelineYAML([]byte(`
version: 1
events:
  - at: { request: 1 }
    match: { method: GET, urlPath: /api/declared }
    fault: { type: rate_limit }
`)); err != nil {
		t.Fatalf("load timeline: %v", err)
	}

	// Request 1 fires the event -> 429; request 2 is past the single-shot
	// event -> normal 200.
	want := []int{http.StatusTooManyRequests, http.StatusOK}
	for i, wantStatus := range want {
		resp, err := http.Get(server.URL() + "/api/declared")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != wantStatus {
			t.Fatalf("request %d: got status %d, want %d", i, resp.StatusCode, wantStatus)
		}
	}

	result := server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType:      "rate_limit",
		ActivationMode: "timeline",
	}, 1)
	if !result.Matched {
		t.Fatalf("expected 1 timeline rate_limit fault: %v", result.Errors)
	}
}

// TestTimelineYAMLRoundTrip: an exported artifact, marshaled to YAML and
// loaded into a fresh server via LoadTimelineYAML, replays the identical
// injection sequence. Pins the marshal -> LoadTimelineYAML path that only
// load-from-YAML and load-from-struct tests exercise separately.
func TestTimelineYAMLRoundTrip(t *testing.T) {
	const n = 20

	drive := func(server gmock.Server) []int {
		statuses := make([]int, 0, n)
		for i := 0; i < n; i++ {
			resp, err := http.Get(server.URL() + "/api/rt")
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
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/rt"},
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

	data, err := yaml.Marshal(tl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Run 2: replay the YAML artifact with a fresh, fault-free stub.
	serverB, stopB := startServer(t)
	defer stopB()
	serverB.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/rt"},
		Response: gmock.ResponseDefinition{
			Status: http.StatusOK,
			Body:   `{"ok":true}`,
		},
	})
	if err := serverB.LoadTimelineYAML(data); err != nil {
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
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/slow"},
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

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

// Tutorial 4: Record Real Chaos, Replay It in CI
//
// This example demonstrates the fault timeline loop: run probabilistic chaos
// once (seeded for debuggability), export the fault sequence that actually
// fired, then replay that exact sequence deterministically in CI. Replay is
// keyed on request index, so it never consults the RNG.
//
// Run:
//
//	cd examples/tutorial/04-timeline-replay && go test -v
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
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/api/payments"},
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

package spec_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

func TestDimensionScoreJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		input       spec.DimensionScore
		wantContain string // substring expected in marshalled JSON
		wantOmit    string // substring that must NOT appear in marshalled JSON
	}{
		{
			name: "with reason populated",
			input: spec.DimensionScore{
				Dimension: "method",
				Matched:   false,
				Score:     0,
				MaxScore:  10,
				Expected:  "POST",
				Actual:    "GET",
				Reason:    "method GET does not equal POST",
			},
			wantContain: `"reason":"method GET does not equal POST"`,
		},
		{
			name: "matched - reason omitted",
			input: spec.DimensionScore{
				Dimension: "path",
				Matched:   true,
				Score:     30,
				MaxScore:  30,
				Expected:  "/api/users",
				Actual:    "/api/users",
			},
			wantOmit: `"reason"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := string(data)
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("marshalled JSON %q missing %q", got, tt.wantContain)
			}
			if tt.wantOmit != "" && strings.Contains(got, tt.wantOmit) {
				t.Errorf("marshalled JSON %q should not contain %q", got, tt.wantOmit)
			}

			// Round-trip back into a struct and compare
			var decoded spec.DimensionScore
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded != tt.input {
				t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, tt.input)
			}
		})
	}
}

func TestNearMissResultJSONRoundTripWithReason(t *testing.T) {
	in := spec.NearMissResult{
		StubID:   "abc",
		StubName: "test",
		Score:    35,
		MaxScore: 50,
		Reason:   "method+path nearly matched",
		Breakdown: []spec.DimensionScore{
			{Dimension: "method", Matched: true, Score: 10, MaxScore: 10},
			{Dimension: "path", Matched: false, Score: 0, MaxScore: 30, Expected: "/api/x", Actual: "/api/y", Reason: "path /api/y does not equal /api/x"},
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out spec.NearMissResult
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Breakdown) != 2 {
		t.Fatalf("breakdown len = %d, want 2", len(out.Breakdown))
	}
	if out.Breakdown[0].Reason != "" {
		t.Errorf("matched dimension Reason should be empty, got %q", out.Breakdown[0].Reason)
	}
	if out.Breakdown[1].Reason == "" {
		t.Errorf("missed dimension Reason should be populated")
	}
}

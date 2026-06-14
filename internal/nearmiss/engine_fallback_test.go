package nearmiss

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/matcher"
)

// stubMatcher is a test-only Matcher that intentionally does NOT implement
// DiagnosticMatcher. It exists to drive diagnoseComposite's generic fallback
// branch — a path that the production stub.BuildMatcher pipeline never
// produces today (every matcher it builds implements Diagnose) but that the
// engine retains as defensive support for custom user-supplied matchers
// added through future extension points.
type stubMatcher struct {
	matched bool
	score   int
	label   string
}

func (s stubMatcher) Match(req *http.Request) bool { return s.matched }
func (s stubMatcher) ScoreMatch(req *http.Request) (bool, int) {
	return s.matched, s.score
}
func (s stubMatcher) String() string { return s.label }

// Compile-time guard: stubMatcher satisfies matcher.Matcher and does NOT
// satisfy matcher.DiagnosticMatcher. The latter is enforced by the lack of
// a Diagnose method (a non-stating compile-time check is impossible without
// negative trait bounds, but accidental promotion would be caught by the
// "fallback exercised" assertions in the tests below).
var _ matcher.Matcher = stubMatcher{}

// TestDiagnoseComposite_FallbackBranch exercises the generic fallback in
// diagnoseComposite: a CompositeMatcher containing a Matcher that does NOT
// implement DiagnosticMatcher.
func TestDiagnoseComposite_FallbackBranch(t *testing.T) {
	tests := []struct {
		name      string
		matcher   stubMatcher
		wantMatch bool
		wantScore int
	}{
		{
			name:      "miss populates Reason and custom-prefixed Dimension",
			matcher:   stubMatcher{matched: false, score: 0, label: "fake-miss"},
			wantMatch: false,
			wantScore: 0,
		},
		{
			name:      "hit yields matched DimensionScore with no Reason",
			matcher:   stubMatcher{matched: true, score: 7, label: "fake-hit"},
			wantMatch: true,
			wantScore: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/x", strings.NewReader(""))
			composite := matcher.NewCompositeMatcher(tt.matcher)

			breakdown, totalScore, totalMax, fullMatch := diagnoseComposite(composite, req)

			if len(breakdown) != 1 {
				t.Fatalf("breakdown len = %d, want 1", len(breakdown))
			}
			ds := breakdown[0]

			if !strings.HasPrefix(ds.Dimension, "custom:") {
				t.Errorf("Dimension = %q, want \"custom:\" prefix", ds.Dimension)
			}
			if ds.Matched != tt.wantMatch {
				t.Errorf("Matched = %v, want %v", ds.Matched, tt.wantMatch)
			}
			if ds.Score != tt.wantScore {
				t.Errorf("Score = %d, want %d", ds.Score, tt.wantScore)
			}
			if ds.Expected != tt.matcher.label {
				t.Errorf("Expected = %q, want %q (matcher.String())", ds.Expected, tt.matcher.label)
			}
			if tt.wantMatch && ds.Reason != "" {
				t.Errorf("Reason should be empty on match, got %q", ds.Reason)
			}
			if !tt.wantMatch && ds.Reason == "" {
				t.Errorf("Reason should be populated on miss")
			}
			if totalScore != ds.Score {
				t.Errorf("totalScore = %d, want %d", totalScore, ds.Score)
			}
			if totalMax != ds.MaxScore {
				t.Errorf("totalMax = %d, want %d", totalMax, ds.MaxScore)
			}
			if fullMatch != tt.wantMatch {
				t.Errorf("fullMatch = %v, want %v", fullMatch, tt.wantMatch)
			}
		})
	}
}

// TestDiagnoseComposite_MixedDiagnosticAndFallback verifies the fallback
// coexists with DiagnosticMatcher children in the same composite — the
// diagnostic dimension keeps its native label while the non-diagnostic
// child gets a "custom:<idx>" dimension.
func TestDiagnoseComposite_MixedDiagnosticAndFallback(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/users", strings.NewReader(""))

	// Real diagnostic matcher: method matcher (matches GET).
	methodM := matcher.NewMethodMatcher("GET")
	// Non-diagnostic fallback matcher (will miss).
	fallbackM := stubMatcher{matched: false, score: 0, label: "custom-rule"}

	composite := matcher.NewCompositeMatcher(methodM, fallbackM)
	breakdown, _, _, fullMatch := diagnoseComposite(composite, req)

	if fullMatch {
		t.Fatalf("fullMatch should be false: fallback child does not match")
	}
	if len(breakdown) != 2 {
		t.Fatalf("breakdown len = %d, want 2", len(breakdown))
	}
	if breakdown[0].Dimension != "method" {
		t.Errorf("first dimension = %q, want \"method\"", breakdown[0].Dimension)
	}
	if breakdown[1].Dimension != "custom:1" {
		t.Errorf("fallback dimension = %q, want \"custom:1\"", breakdown[1].Dimension)
	}
	if breakdown[1].Reason == "" {
		t.Errorf("fallback miss should populate Reason")
	}
}

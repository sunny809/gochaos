package nearmiss_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/spec"
)

// makeReq builds a request with optional headers, query, and body.
func makeReq(t *testing.T, method, target, body string, headers map[string]string) *http.Request {
	t.Helper()
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, br)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func dimensionByName(b []spec.DimensionScore, name string) (spec.DimensionScore, bool) {
	for _, d := range b {
		if d.Dimension == name {
			return d, true
		}
	}
	return spec.DimensionScore{}, false
}

// stubGET builds a small stub definition for tests.
func stubGET(id, path string) spec.StubDefinition {
	return spec.StubDefinition{
		ID: id,
		Request: spec.RequestPattern{
			Method:  "GET",
			URLPath: path,
		},
		Response: spec.ResponseDefinition{Status: 200},
	}
}

// stubMethodOnly builds a stub keyed only on method (max score 10).
func stubMethodOnly(id, method string) spec.StubDefinition {
	return spec.StubDefinition{
		ID: id,
		Request: spec.RequestPattern{
			Method: method,
		},
		Response: spec.ResponseDefinition{Status: 200},
	}
}

// TestCompute_BestCandidateFirst — AC #1
func TestCompute_BestCandidateFirst(t *testing.T) {
	stubs := []spec.StubDefinition{
		stubGET("low", "/orders"),     // method matches (10), path miss → score 10/40
		stubGET("high", "/users"),     // method matches (10), path matches (30) → 40/40 — full match, omitted
		stubMethodOnly("only", "GET"), // method matches (10) → 10/10 — full match, omitted
		{
			ID: "mid",
			Request: spec.RequestPattern{
				Method:  "POST", // miss (0/10)
				URLPath: "/users",
			},
			Response: spec.ResponseDefinition{Status: 200},
		}, // method miss, path match → 30/40
	}
	req := makeReq(t, "GET", "/users", "", nil)

	results := nearmiss.NewEngine().Compute(req, stubs)

	if len(results) == 0 {
		t.Fatalf("expected results, got none")
	}
	// Two stubs fully matched ("high", "only"); they should be omitted.
	for _, r := range results {
		if r.StubID == "high" || r.StubID == "only" {
			t.Errorf("full-match stub %q must be omitted", r.StubID)
		}
	}
	// Best candidate (highest score) ordered first.
	if results[0].StubID != "mid" {
		t.Errorf("expected best candidate %q first, got %q (full=%+v)", "mid", results[0].StubID, results)
	}
	if results[0].Score < results[len(results)-1].Score {
		t.Errorf("results not sorted desc: %+v", results)
	}
}

// TestCompute_FullMatchOmitted — AC #2
func TestCompute_FullMatchOmitted(t *testing.T) {
	stubs := []spec.StubDefinition{
		stubGET("exact", "/users"),
		stubGET("near", "/orders"),
	}
	req := makeReq(t, "GET", "/users", "", nil)

	results := nearmiss.NewEngine().Compute(req, stubs)

	for _, r := range results {
		if r.StubID == "exact" {
			t.Errorf("fully-matching stub must be omitted, got %+v", r)
		}
	}
	// "near" should be present (method match, path miss).
	found := false
	for _, r := range results {
		if r.StubID == "near" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected non-matching stub 'near' in results: %+v", results)
	}
}

// TestCompute_TopResultPopulatedFields — AC #3
func TestCompute_TopResultPopulatedFields(t *testing.T) {
	stubs := []spec.StubDefinition{
		{
			ID:   "s1",
			Name: "Stub-1",
			Request: spec.RequestPattern{
				Method:  "POST",
				URLPath: "/api/users",
				Headers: map[string]string{"X-Tenant-Id": "acme"},
			},
			Response: spec.ResponseDefinition{Status: 201},
		},
	}
	req := makeReq(t, "POST", "/api/users", "", map[string]string{"X-Tenant-Id": "wrong"})

	results := nearmiss.NewEngine().Compute(req, stubs)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.StubID != "s1" || r.StubName != "Stub-1" {
		t.Errorf("StubID/Name = (%q,%q), want (s1,Stub-1)", r.StubID, r.StubName)
	}
	if len(r.Breakdown) == 0 {
		t.Fatalf("Breakdown empty")
	}
	for _, d := range r.Breakdown {
		if d.Dimension == "" {
			t.Errorf("Dimension empty: %+v", d)
		}
		if d.MaxScore == 0 {
			t.Errorf("MaxScore zero: %+v", d)
		}
		if d.Matched && d.Reason != "" {
			t.Errorf("Reason should be empty when Matched: %+v", d)
		}
		if !d.Matched && d.Reason == "" {
			t.Errorf("Reason should be populated on miss: %+v", d)
		}
	}
}

// TestCompute_AllRelevantDimensions — AC #4
func TestCompute_AllRelevantDimensions(t *testing.T) {
	stubs := []spec.StubDefinition{
		{
			ID: "s1",
			Request: spec.RequestPattern{
				Method:      "POST",
				URLPath:     "/api/users",
				Accept:      "application/json",
				Headers:     map[string]string{"X-Tenant-Id": "acme"},
				Cookies:     map[string]string{"session": "abc"},
				QueryParams: map[string]string{"page": "1"},
				Body:        &spec.BodyPattern{ExactMatch: "expected"},
			},
		},
	}
	// Make request differ on every dimension to force misses.
	req := makeReq(t, "GET", "/api/orders?page=2", "actual",
		map[string]string{
			"Accept":      "text/html",
			"X-Tenant-Id": "wrong",
			"Cookie":      "session=xyz",
		})

	results := nearmiss.NewEngine().Compute(req, stubs)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	wantDims := []string{"method", "path", "accept", "header:X-Tenant-Id", "cookie:session", "query:page", "body"}
	for _, name := range wantDims {
		if _, ok := dimensionByName(results[0].Breakdown, name); !ok {
			t.Errorf("dimension %q missing in breakdown: %+v", name, results[0].Breakdown)
		}
	}
}

// TestCompute_TopNTruncates — AC #5
func TestCompute_TopNTruncates(t *testing.T) {
	stubs := []spec.StubDefinition{
		stubGET("a", "/p1"),
		stubGET("b", "/p2"),
		stubGET("c", "/p3"),
		stubGET("d", "/p4"),
		stubGET("e", "/p5"),
	}
	req := makeReq(t, "GET", "/zzz", "", nil) // method matches all, path misses all

	results := nearmiss.NewEngine(nearmiss.WithTopN(2)).Compute(req, stubs)
	if len(results) != 2 {
		t.Fatalf("expected 2 results with WithTopN(2), got %d", len(results))
	}

	// WithTopN(0) → unlimited
	all := nearmiss.NewEngine(nearmiss.WithTopN(0)).Compute(req, stubs)
	if len(all) != 5 {
		t.Errorf("WithTopN(0) should be unlimited, got %d", len(all))
	}
	negative := nearmiss.NewEngine(nearmiss.WithTopN(-3)).Compute(req, stubs)
	if len(negative) != 5 {
		t.Errorf("WithTopN(-3) should be unlimited, got %d", len(negative))
	}
}

// TestCompute_EqualScorePreservesInputOrder — AC #6
func TestCompute_EqualScorePreservesInputOrder(t *testing.T) {
	// Two stubs with identical structure → identical totals on a miss.
	stubs := []spec.StubDefinition{
		stubGET("first", "/orders"),
		stubGET("second", "/invoices"),
		stubGET("third", "/payments"),
	}
	req := makeReq(t, "GET", "/no-match", "", nil)

	results := nearmiss.NewEngine().Compute(req, stubs)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// All three have score 10 (method match) and max 40 — totals are equal,
	// so input order must be preserved by stable sort.
	gotIDs := []string{results[0].StubID, results[1].StubID, results[2].StubID}
	wantIDs := []string{"first", "second", "third"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("equal-score order = %v, want %v", gotIDs, wantIDs)
	}
}

// TestCompute_EmptyAndNilSafety — AC #7
func TestCompute_EmptyAndNilSafety(t *testing.T) {
	tests := []struct {
		name  string
		stubs []spec.StubDefinition
	}{
		{"nil", nil},
		{"empty", []spec.StubDefinition{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Compute panicked: %v", r)
				}
			}()
			req := makeReq(t, "GET", "/", "", nil)
			results := nearmiss.NewEngine().Compute(req, tt.stubs)
			if results == nil {
				t.Errorf("results must be non-nil")
			}
			if len(results) != 0 {
				t.Errorf("results should be empty, got %d", len(results))
			}
		})
	}
}

// TestCompute_BodyRestoredAndPatternUnchanged — AC #8
func TestCompute_BodyRestoredAndPatternUnchanged(t *testing.T) {
	original := spec.StubDefinition{
		ID: "s1",
		Request: spec.RequestPattern{
			Method:      "POST",
			URLPath:     "/users",
			Headers:     map[string]string{"X-Tenant-Id": "acme"},
			QueryParams: map[string]string{"page": "1"},
			Body:        &spec.BodyPattern{ExactMatch: "payload"},
		},
		Response: spec.ResponseDefinition{Status: 201},
	}
	// Pre-call deep snapshot for equality assertion.
	snapshot := spec.StubDefinition{
		ID: original.ID,
		Request: spec.RequestPattern{
			Method:      original.Request.Method,
			URLPath:     original.Request.URLPath,
			Headers:     map[string]string{"X-Tenant-Id": "acme"},
			QueryParams: map[string]string{"page": "1"},
			Body:        &spec.BodyPattern{ExactMatch: "payload"},
		},
		Response: spec.ResponseDefinition{Status: 201},
	}

	bodyText := "different-payload"
	req := httptest.NewRequest("POST", "/users?page=2", strings.NewReader(bodyText))
	req.Header.Set("X-Tenant-Id", "wrong")

	results := nearmiss.NewEngine().Compute(req, []spec.StubDefinition{original})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Body must be re-readable after Compute.
	read, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("re-read req.Body: %v", err)
	}
	if string(read) != bodyText {
		t.Errorf("body after Compute = %q, want %q", string(read), bodyText)
	}

	// Original RequestPattern must be unchanged.
	if !reflect.DeepEqual(original.Request, snapshot.Request) {
		t.Errorf("Request pattern mutated:\n got=%+v\nwant=%+v", original.Request, snapshot.Request)
	}
	if original.ID != snapshot.ID {
		t.Errorf("ID mutated: %q vs %q", original.ID, snapshot.ID)
	}
}

// TestCompute_MatchAnythingStub verifies the empty-children CompositeMatcher
// is treated as a full match and thus omitted from results.
func TestCompute_MatchAnythingStub(t *testing.T) {
	stubs := []spec.StubDefinition{
		{ID: "any", Request: spec.RequestPattern{}}, // no constraints
		stubGET("named", "/needle"),                 // miss
	}
	req := makeReq(t, "POST", "/haystack", "", nil)

	results := nearmiss.NewEngine().Compute(req, stubs)
	for _, r := range results {
		if r.StubID == "any" {
			t.Errorf("match-anything stub must be omitted from near-miss results: %+v", r)
		}
	}
}

// TestEngine_StateIsolation verifies that two engines with different topN
// configurations don't interfere. Indirectly proves the engine is stateless
// across Compute calls.
func TestEngine_StateIsolation(t *testing.T) {
	stubs := []spec.StubDefinition{
		stubGET("a", "/p1"),
		stubGET("b", "/p2"),
		stubGET("c", "/p3"),
	}
	req := makeReq(t, "GET", "/none", "", nil)

	e1 := nearmiss.NewEngine(nearmiss.WithTopN(1))
	e2 := nearmiss.NewEngine(nearmiss.WithTopN(2))

	if got := len(e1.Compute(req, stubs)); got != 1 {
		t.Errorf("e1 len = %d, want 1", got)
	}
	if got := len(e2.Compute(req, stubs)); got != 2 {
		t.Errorf("e2 len = %d, want 2", got)
	}
	// Calling again should produce the same shape (no state carryover).
	if got := len(e1.Compute(req, stubs)); got != 1 {
		t.Errorf("e1 second call len = %d, want 1", got)
	}
}

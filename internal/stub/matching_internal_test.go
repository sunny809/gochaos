package stub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

// --- maxScore ---

func TestMaxScore(t *testing.T) {
	tests := []struct {
		name string
		p    spec.RequestPattern
		want int
	}{
		{
			name: "empty pattern",
			p:    spec.RequestPattern{},
			want: 0,
		},
		{
			name: "method only",
			p:    spec.RequestPattern{Method: "GET"},
			want: 10,
		},
		{
			name: "path exact",
			p:    spec.RequestPattern{URLPath: "/api"},
			want: 30,
		},
		{
			name: "path regex",
			p:    spec.RequestPattern{URLPathRegex: "/api/.*"},
			want: 15,
		},
		{
			name: "path exact takes precedence over regex",
			p:    spec.RequestPattern{URLPath: "/api", URLPathRegex: "/api/.*"},
			want: 30,
		},
		{
			name: "accept header",
			p:    spec.RequestPattern{Accept: "application/json"},
			want: 7,
		},
		{
			name: "headers (5 each)",
			p:    spec.RequestPattern{Headers: map[string]string{"A": "1", "B": "2", "C": "3"}},
			want: 15,
		},
		{
			name: "cookies (4 each)",
			p:    spec.RequestPattern{Cookies: map[string]string{"S": "1", "T": "2"}},
			want: 8,
		},
		{
			name: "query params (3 each)",
			p:    spec.RequestPattern{QueryParams: map[string]string{"q": "1", "p": "2", "r": "3", "s": "4"}},
			want: 12,
		},
		{
			name: "body exact",
			p:    spec.RequestPattern{Body: &spec.BodyPattern{ExactMatch: "hello"}},
			want: 20,
		},
		{
			name: "body regex",
			p:    spec.RequestPattern{Body: &spec.BodyPattern{RegexMatch: "hel.*"}},
			want: 10,
		},
		{
			name: "body jsonpath",
			p:    spec.RequestPattern{Body: &spec.BodyPattern{JSONPath: "$.name"}},
			want: 12,
		},
		{
			name: "body exact takes precedence over regex",
			p:    spec.RequestPattern{Body: &spec.BodyPattern{ExactMatch: "x", RegexMatch: "y"}},
			want: 20,
		},
		{
			name: "full pattern",
			p: spec.RequestPattern{
				Method:      "POST",
				URLPath:     "/api/users",
				Accept:      "application/json",
				Headers:     map[string]string{"Authorization": "Bearer .*"},
				Cookies:     map[string]string{"session": ".*"},
				QueryParams: map[string]string{"page": "1"},
				Body:        &spec.BodyPattern{JSONPath: "$.name"},
			},
			want: 10 + 30 + 7 + 5 + 4 + 3 + 12, // 71
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxScore(tt.p)
			if got != tt.want {
				t.Errorf("maxScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- WeightedScore ---

func TestWeightedScore(t *testing.T) {
	tests := []struct {
		name   string
		actual int
		max    int
		want   float64
	}{
		{"zero max returns 0", 0, 0, 0},
		{"full score", 100, 100, 1.0},
		{"half score", 50, 100, 0.5},
		{"zero actual", 0, 100, 0},
		{"rounding down", 1, 3, 0.33},
		{"rounding up", 2, 3, 0.67},
		{"actual exceeds max", 150, 100, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeightedScore(tt.actual, tt.max)
			if got != tt.want {
				t.Errorf("WeightedScore(%d, %d) = %v, want %v", tt.actual, tt.max, got, tt.want)
			}
		})
	}
}

// --- RequestKey ---

func TestRequestKey(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want string
	}{
		{
			name: "nil request",
			req:  nil,
			want: "<nil>",
		},
		{
			name: "GET request",
			req:  httptest.NewRequest(http.MethodGet, "/api/users", nil),
			want: "GET /api/users",
		},
		{
			name: "POST request with query",
			req:  httptest.NewRequest(http.MethodPost, "/api/users?page=1", nil),
			want: "POST /api/users",
		},
		{
			name: "path with special characters",
			req:  httptest.NewRequest(http.MethodDelete, "/api/users/42", nil),
			want: "DELETE /api/users/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RequestKey(tt.req)
			if got != tt.want {
				t.Errorf("RequestKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- ExtractRequestPattern ---

func TestExtractRequestPattern(t *testing.T) {
	tests := []struct {
		name        string
		req         *http.Request
		wantMethod  string
		wantPath    string
		wantHeaders map[string]string
		wantQuery   map[string]string
	}{
		{
			name:       "basic GET",
			req:        httptest.NewRequest(http.MethodGet, "/test", nil),
			wantMethod: "GET",
			wantPath:   "/test",
		},
		{
			name:       "with single header",
			req:        httptest.NewRequest(http.MethodGet, "/test", nil),
			wantMethod: "GET",
			wantPath:   "/test",
			wantHeaders: map[string]string{
				"X-Custom": "value",
			},
		},
		{
			name:       "with query params",
			req:        httptest.NewRequest(http.MethodGet, "/search?q=hello&page=2", nil),
			wantMethod: "GET",
			wantPath:   "/search",
			wantQuery: map[string]string{
				"q":    "hello",
				"page": "2",
			},
		},
	}

	// Set up headers for tests that need them.
	tests[1].req.Header.Set("X-Custom", "value")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ExtractRequestPattern(tt.req)
			if p.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", p.Method, tt.wantMethod)
			}
			if p.URLPath != tt.wantPath {
				t.Errorf("URLPath = %q, want %q", p.URLPath, tt.wantPath)
			}
			for k, v := range tt.wantHeaders {
				if p.Headers[k] != v {
					t.Errorf("Header[%q] = %q, want %q", k, p.Headers[k], v)
				}
			}
			for k, v := range tt.wantQuery {
				if p.QueryParams[k] != v {
					t.Errorf("QueryParam[%q] = %q, want %q", k, p.QueryParams[k], v)
				}
			}
		})
	}
}

func TestExtractRequestPattern_MultiValueHeaders(t *testing.T) {
	// When a header has multiple values, only the first is extracted.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Add("Accept", "text/html")
	req.Header.Add("Accept", "application/json")

	p := ExtractRequestPattern(req)
	if p.Headers["Accept"] != "text/html" {
		t.Errorf("Accept header = %q, want %q (first value)", p.Headers["Accept"], "text/html")
	}
}

// --- PatternsMatch ---

func TestPatternsMatch(t *testing.T) {
	tests := []struct {
		name      string
		pattern   spec.RequestPattern
		candidate spec.RequestPattern
		wantMatch bool
		wantScore int
	}{
		{
			name:      "both empty (no pattern criteria)",
			pattern:   spec.RequestPattern{},
			candidate: spec.RequestPattern{},
			wantMatch: false,
			wantScore: 0,
		},
		{
			name:      "method only - matches",
			pattern:   spec.RequestPattern{Method: "GET"},
			candidate: spec.RequestPattern{Method: "GET"},
			wantMatch: true,
			wantScore: 10,
		},
		{
			name:      "method only - case insensitive match",
			pattern:   spec.RequestPattern{Method: "get"},
			candidate: spec.RequestPattern{Method: "GET"},
			wantMatch: true,
			wantScore: 10,
		},
		{
			name:      "method only - no match",
			pattern:   spec.RequestPattern{Method: "GET"},
			candidate: spec.RequestPattern{Method: "POST"},
			wantMatch: false,
			wantScore: 0,
		},
		{
			name:      "path exact - matches",
			pattern:   spec.RequestPattern{URLPath: "/api"},
			candidate: spec.RequestPattern{URLPath: "/api"},
			wantMatch: true,
			wantScore: 30,
		},
		{
			name:      "path exact - no match",
			pattern:   spec.RequestPattern{URLPath: "/api"},
			candidate: spec.RequestPattern{URLPath: "/other"},
			wantMatch: false,
			wantScore: 0,
		},
		{
			name:      "path regex - matches",
			pattern:   spec.RequestPattern{URLPathRegex: "/api/.*"},
			candidate: spec.RequestPattern{URLPath: "/api/users"},
			wantMatch: true,
			wantScore: 15,
		},
		{
			name:      "path regex - no match",
			pattern:   spec.RequestPattern{URLPathRegex: "/api/.*"},
			candidate: spec.RequestPattern{URLPath: "/other"},
			wantMatch: false,
			wantScore: 0,
		},
		{
			name:      "method and path - both match",
			pattern:   spec.RequestPattern{Method: "POST", URLPath: "/api"},
			candidate: spec.RequestPattern{Method: "POST", URLPath: "/api"},
			wantMatch: true,
			wantScore: 40,
		},
		{
			name:      "method and path - partial match",
			pattern:   spec.RequestPattern{Method: "POST", URLPath: "/api"},
			candidate: spec.RequestPattern{Method: "POST", URLPath: "/other"},
			wantMatch: false,
			wantScore: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMatch, gotScore := PatternsMatch(tt.pattern, tt.candidate)
			if gotMatch != tt.wantMatch {
				t.Errorf("PatternsMatch() match = %v, want %v", gotMatch, tt.wantMatch)
			}
			if gotScore != tt.wantScore {
				t.Errorf("PatternsMatch() score = %d, want %d", gotScore, tt.wantScore)
			}
		})
	}
}

// --- MatchWithScore ---

func TestMatchWithScore(t *testing.T) {
	// Set up a registry with a few stubs.
	r := NewRegistry()
	if _, err := r.Add(spec.StubDefinition{
		ID:       "stub-1",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/api/users"},
		Response: spec.ResponseDefinition{Status: 200},
	}); err != nil {
		t.Fatalf("add stub-1: %v", err)
	}
	if _, err := r.Add(spec.StubDefinition{
		ID:       "stub-2",
		Request:  spec.RequestPattern{Method: "POST", URLPath: "/api/users"},
		Response: spec.ResponseDefinition{Status: 201},
	}); err != nil {
		t.Fatalf("add stub-2: %v", err)
	}
	if _, err := r.Add(spec.StubDefinition{
		ID:       "stub-3",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/api/other"},
		Response: spec.ResponseDefinition{Status: 200},
	}); err != nil {
		t.Fatalf("add stub-3: %v", err)
	}

	e := NewEngine(r)

	t.Run("returns all results sorted by score", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		results := e.MatchWithScore(req)

		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}

		// Results should be sorted by score descending.
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Errorf("results not sorted: score[%d]=%d > score[%d]=%d",
					i, results[i].Score, i-1, results[i-1].Score)
			}
		}

		// The top match should be stub-1 (GET /api/users — exact match).
		if results[0].Stub.ID != "stub-1" {
			t.Errorf("top match = %q, want %q", results[0].Stub.ID, "stub-1")
		}
		if !results[0].Matched {
			t.Error("top match should be Matched=true")
		}
	})

	t.Run("no stubs returns empty", func(t *testing.T) {
		emptyReg := NewRegistry()
		emptyEngine := NewEngine(emptyReg)
		results := emptyEngine.MatchWithScore(httptest.NewRequest(http.MethodGet, "/", nil))
		if len(results) != 0 {
			t.Errorf("expected 0 results for empty registry, got %d", len(results))
		}
	})
}

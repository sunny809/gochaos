package matcher_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/matcher"
)

// diagnoseRequest is a small helper to keep table-driven tests terse.
func diagnoseRequest(t *testing.T, dm matcher.DiagnosticMatcher, req *http.Request) matcher.Diagnosis {
	t.Helper()
	return dm.Diagnose(req)
}

func TestMethodMatcher_Diagnose(t *testing.T) {
	tests := []struct {
		name        string
		stub        string
		req         string
		wantMatched bool
		wantScore   int
		wantActual  string
		wantReason  string
	}{
		{"matched", "POST", "POST", true, 10, "POST", ""},
		{"miss", "POST", "GET", false, 0, "GET", "method GET does not equal POST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewMethodMatcher(tt.stub)
			d := diagnoseRequest(t, m, makeRequest(tt.req, "/"))
			if d.Dimension != "method" {
				t.Errorf("Dimension = %q, want method", d.Dimension)
			}
			if d.Matched != tt.wantMatched || d.Score != tt.wantScore {
				t.Errorf("Matched/Score = (%v,%d), want (%v,%d)", d.Matched, d.Score, tt.wantMatched, tt.wantScore)
			}
			if d.MaxScore != 10 {
				t.Errorf("MaxScore = %d, want 10", d.MaxScore)
			}
			if d.Actual != tt.wantActual {
				t.Errorf("Actual = %q, want %q", d.Actual, tt.wantActual)
			}
			if d.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", d.Reason, tt.wantReason)
			}
		})
	}
}

func TestPathExactMatcher_Diagnose(t *testing.T) {
	m := matcher.NewPathExactMatcher("/api/users")

	t.Run("matched", func(t *testing.T) {
		d := m.Diagnose(makeRequest("GET", "/api/users"))
		if !d.Matched || d.Score != 30 || d.Reason != "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("miss", func(t *testing.T) {
		d := m.Diagnose(makeRequest("GET", "/api/orders"))
		if d.Matched || d.Score != 0 {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		if d.Reason == "" || d.Expected != "/api/users" || d.Actual != "/api/orders" {
			t.Errorf("missing fields: %+v", d)
		}
	})
}

func TestPathRegexMatcher_Diagnose(t *testing.T) {
	m := matcher.MustNewPathRegexMatcher(`^/api/v[0-9]+/users$`)

	t.Run("matched", func(t *testing.T) {
		d := m.Diagnose(makeRequest("GET", "/api/v1/users"))
		if !d.Matched || d.Score != 15 || d.Reason != "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("miss", func(t *testing.T) {
		d := m.Diagnose(makeRequest("GET", "/api/users"))
		if d.Matched || d.Reason == "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		if !strings.Contains(d.Reason, "does not match regex") {
			t.Errorf("Reason missing context: %q", d.Reason)
		}
	})
}

func TestAcceptMatcher_Diagnose(t *testing.T) {
	m := matcher.NewAcceptMatcher("application/json")

	t.Run("matched", func(t *testing.T) {
		req := makeRequest("GET", "/")
		req.Header.Set("Accept", "application/json")
		d := m.Diagnose(req)
		if !d.Matched || d.Score != 7 || d.Reason != "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("miss", func(t *testing.T) {
		req := makeRequest("GET", "/")
		req.Header.Set("Accept", "text/html")
		d := m.Diagnose(req)
		if d.Matched || d.Reason == "" || d.Actual != "text/html" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})
}

func TestHeaderMatcher_Diagnose(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
		m, _ := matcher.NewHeaderMatcher("X-Tenant-Id", "acme")
		req := makeRequest("GET", "/")
		req.Header.Set("X-Tenant-Id", "acme")
		d := m.Diagnose(req)
		if !d.Matched || d.Reason != "" || d.Dimension != "header:X-Tenant-Id" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("missing header has empty Actual", func(t *testing.T) {
		m, _ := matcher.NewHeaderMatcher("X-Tenant-Id", "acme")
		d := m.Diagnose(makeRequest("GET", "/"))
		if d.Matched {
			t.Fatalf("should not match")
		}
		if d.Actual != "" {
			t.Errorf("Actual should be empty for missing header, got %q", d.Actual)
		}
		if !strings.Contains(d.Reason, "missing") {
			t.Errorf("Reason should mention missing, got %q", d.Reason)
		}
	})

	t.Run("regex miss", func(t *testing.T) {
		m, _ := matcher.NewHeaderMatcher("X-Trace", "~^trace-[0-9]+$")
		req := makeRequest("GET", "/")
		req.Header.Set("X-Trace", "abc")
		d := m.Diagnose(req)
		if d.Matched {
			t.Fatalf("should not match")
		}
		if !strings.Contains(d.Reason, "regex") {
			t.Errorf("Reason should mention regex: %q", d.Reason)
		}
	})

	t.Run("absent-required ! pattern with present header", func(t *testing.T) {
		m, _ := matcher.NewHeaderMatcher("X-Banned", "!")
		req := makeRequest("GET", "/")
		req.Header.Set("X-Banned", "yes")
		d := m.Diagnose(req)
		if d.Matched {
			t.Fatalf("should not match")
		}
		if !strings.Contains(d.Reason, "absent") {
			t.Errorf("Reason should mention absent: %q", d.Reason)
		}
	})
}

func TestCookieMatcher_Diagnose(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
		m, _ := matcher.NewCookieMatcher("session", "abc123")
		req := makeRequest("GET", "/")
		req.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
		d := m.Diagnose(req)
		if !d.Matched || d.Reason != "" || d.Dimension != "cookie:session" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("missing cookie has empty Actual", func(t *testing.T) {
		m, _ := matcher.NewCookieMatcher("session", "abc123")
		d := m.Diagnose(makeRequest("GET", "/"))
		if d.Matched || d.Actual != "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		if !strings.Contains(d.Reason, "missing") {
			t.Errorf("Reason should mention missing: %q", d.Reason)
		}
	})

	t.Run("value mismatch", func(t *testing.T) {
		m, _ := matcher.NewCookieMatcher("session", "abc123")
		req := makeRequest("GET", "/")
		req.AddCookie(&http.Cookie{Name: "session", Value: "xyz"})
		d := m.Diagnose(req)
		if d.Matched || d.Actual != "xyz" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})
}

func TestQueryParamMatcher_Diagnose(t *testing.T) {
	t.Run("matched", func(t *testing.T) {
		m, _ := matcher.NewQueryParamMatcher("page", "1")
		d := m.Diagnose(makeRequest("GET", "/?page=1"))
		if !d.Matched || d.Reason != "" || d.Dimension != "query:page" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		m, _ := matcher.NewQueryParamMatcher("page", "1")
		d := m.Diagnose(makeRequest("GET", "/"))
		if d.Matched || d.Actual != "" || !strings.Contains(d.Reason, "missing") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("value mismatch", func(t *testing.T) {
		m, _ := matcher.NewQueryParamMatcher("page", "1")
		d := m.Diagnose(makeRequest("GET", "/?page=2"))
		if d.Matched || d.Actual != "2" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})
}

// readAndCompareBody verifies that a body-matcher's Diagnose left req.Body
// re-readable with the original content intact.
func readAndCompareBody(t *testing.T, req *http.Request, want string) {
	t.Helper()
	if req.Body == nil {
		if want == "" {
			return
		}
		t.Fatalf("req.Body is nil, want %q", want)
	}
	got, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("re-read body: %v", err)
	}
	if string(got) != want {
		t.Errorf("body after Diagnose = %q, want %q", string(got), want)
	}
}

func TestBodyExactMatcher_Diagnose(t *testing.T) {
	t.Run("matched and body restored", func(t *testing.T) {
		m := matcher.NewBodyExactMatcher("hello")
		req := httptest.NewRequest("POST", "/", strings.NewReader("hello"))
		d := m.Diagnose(req)
		if !d.Matched || d.Score != 20 || d.Reason != "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		readAndCompareBody(t, req, "hello")
	})

	t.Run("miss and body restored", func(t *testing.T) {
		m := matcher.NewBodyExactMatcher("expected")
		req := httptest.NewRequest("POST", "/", strings.NewReader("actual"))
		d := m.Diagnose(req)
		if d.Matched || d.Reason == "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		if d.Actual != "actual" {
			t.Errorf("Actual = %q, want %q", d.Actual, "actual")
		}
		readAndCompareBody(t, req, "actual")
	})

	t.Run("Actual is truncated for huge bodies", func(t *testing.T) {
		m := matcher.NewBodyExactMatcher("expected")
		big := strings.Repeat("x", 500)
		req := httptest.NewRequest("POST", "/", strings.NewReader(big))
		d := m.Diagnose(req)
		// Truncated to 80 runes plus an ellipsis, so length should be <= ~85
		if len([]rune(d.Actual)) > 81+1 {
			t.Errorf("Actual not truncated: len=%d", len([]rune(d.Actual)))
		}
		if !strings.HasSuffix(d.Actual, "…") {
			t.Errorf("Actual should end with ellipsis, got %q (len=%d)", d.Actual[:20]+"...", len(d.Actual))
		}
		readAndCompareBody(t, req, big)
	})
}

func TestBodyRegexMatcher_Diagnose(t *testing.T) {
	m, err := matcher.NewBodyRegexMatcher(`^hello \w+$`)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("matched", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("hello world"))
		d := m.Diagnose(req)
		if !d.Matched || d.Score != 10 {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		readAndCompareBody(t, req, "hello world")
	})

	t.Run("miss restores body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("nope"))
		d := m.Diagnose(req)
		if d.Matched || d.Reason == "" {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		readAndCompareBody(t, req, "nope")
	})
}

func TestBodyJSONPathMatcher_Diagnose(t *testing.T) {
	m := matcher.NewBodyJSONPathMatcher("$.user.id")

	t.Run("matched", func(t *testing.T) {
		body := `{"user":{"id":"42"}}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := m.Diagnose(req)
		if !d.Matched || d.Score != 12 {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		readAndCompareBody(t, req, body)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := `not json`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := m.Diagnose(req)
		if d.Matched || !strings.Contains(d.Reason, "valid JSON") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
		readAndCompareBody(t, req, body)
	})

	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(""))
		d := m.Diagnose(req)
		if d.Matched || !strings.Contains(d.Reason, "empty") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("path resolves to nil", func(t *testing.T) {
		body := `{"user":{"id":null}}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := m.Diagnose(req)
		if d.Matched {
			t.Errorf("should not match: %+v", d)
		}
		readAndCompareBody(t, req, body)
	})

	t.Run("path resolves to empty string", func(t *testing.T) {
		body := `{"user":{"id":""}}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := m.Diagnose(req)
		if d.Matched || !strings.Contains(d.Reason, "empty string") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("path resolves to empty array", func(t *testing.T) {
		mm := matcher.NewBodyJSONPathMatcher("$.users")
		body := `{"users":[]}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := mm.Diagnose(req)
		if d.Matched || !strings.Contains(d.Reason, "empty array") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("path resolves to empty object", func(t *testing.T) {
		mm := matcher.NewBodyJSONPathMatcher("$.user")
		body := `{"user":{}}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := mm.Diagnose(req)
		if d.Matched || !strings.Contains(d.Reason, "empty object") {
			t.Errorf("unexpected diagnosis: %+v", d)
		}
	})

	t.Run("invalid jsonpath", func(t *testing.T) {
		mm := matcher.NewBodyJSONPathMatcher("$..[?(@.x")
		body := `{"x":1}`
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		d := mm.Diagnose(req)
		if d.Matched {
			t.Errorf("should not match")
		}
	})
}

// TestDiagnosticMatcher_TypeAssertion verifies the optional-interface
// consumption pattern documented on DiagnosticMatcher.
func TestDiagnosticMatcher_TypeAssertion(t *testing.T) {
	var m matcher.Matcher = matcher.NewMethodMatcher("GET")
	dm, ok := m.(matcher.DiagnosticMatcher)
	if !ok {
		t.Fatalf("MethodMatcher should satisfy DiagnosticMatcher")
	}
	d := dm.Diagnose(makeRequest("GET", "/"))
	if !d.Matched {
		t.Errorf("expected matched diagnosis")
	}

	// AlwaysMatch deliberately does NOT implement DiagnosticMatcher.
	if _, ok := matcher.AlwaysMatch.(matcher.DiagnosticMatcher); ok {
		t.Errorf("AlwaysMatch should not implement DiagnosticMatcher")
	}
}

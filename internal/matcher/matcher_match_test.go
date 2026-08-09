package matcher_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/matcher"
)

// The existing matcher_test.go covers ScoreMatch extensively. This file covers
// the Match method (which ScoreMatch does NOT call — it duplicates logic), the
// String() descriptions, and the accessor methods. These are all part of the
// public interface contract and need independent verification.

// --- Match method (independent of ScoreMatch) ---

func TestMethodMatcher_Match(t *testing.T) {
	tests := []struct {
		name      string
		stubMetod string
		reqMethod string
		want      bool
	}{
		{"exact match", "GET", "GET", true},
		{"case insensitive", "get", "GET", true},
		{"different method", "GET", "POST", false},
		{"empty stub method", "", "GET", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewMethodMatcher(tt.stubMetod)
			req := httptest.NewRequest(tt.reqMethod, "/", nil)
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathExactMatcher_Match(t *testing.T) {
	tests := []struct {
		name     string
		stubPath string
		reqPath  string
		want     bool
	}{
		{"exact match", "/api/users", "/api/users", true},
		{"different path", "/api/users", "/api/orders", false},
		{"case sensitive", "/api/Users", "/api/users", false},
		{"trailing slash matters", "/api/users", "/api/users/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewPathExactMatcher(tt.stubPath)
			req := httptest.NewRequest("GET", tt.reqPath, nil)
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathRegexMatcher_Match(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		reqPath string
		want    bool
	}{
		{"matches digits", `^/api/users/\d+$`, "/api/users/123", true},
		{"no match", `^/api/users/\d+$`, "/api/users/abc", false},
		{"partial regex", `users`, "/api/users/123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewPathRegexMatcher(tt.pattern)
			if err != nil {
				t.Fatalf("NewPathRegexMatcher: %v", err)
			}
			req := httptest.NewRequest("GET", tt.reqPath, nil)
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeaderMatcher_Match(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		pattern    string
		reqHeaders map[string]string
		want       bool
	}{
		{"exact match", "Authorization", "Bearer xyz", map[string]string{"Authorization": "Bearer xyz"}, true},
		{"different value", "Authorization", "Bearer xyz", map[string]string{"Authorization": "Bearer abc"}, false},
		{"missing header", "Authorization", "Bearer xyz", map[string]string{}, false},
		{"regex match", "User-Agent", "~Mozilla.*", map[string]string{"User-Agent": "Mozilla/5.0"}, true},
		{"wildcard with value", "X-Trace-Id", "*", map[string]string{"X-Trace-Id": "abc"}, true},
		{"wildcard missing", "X-Trace-Id", "*", map[string]string{}, false},
		{"absent expected, present", "X-Forbidden", "!", map[string]string{"X-Forbidden": "yes"}, false},
		{"absent expected, absent", "X-Forbidden", "!", map[string]string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewHeaderMatcher(tt.header, tt.pattern)
			if err != nil {
				t.Fatalf("NewHeaderMatcher: %v", err)
			}
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.reqHeaders {
				req.Header.Set(k, v)
			}
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCookieMatcher_Match(t *testing.T) {
	tests := []struct {
		name      string
		cookie    string
		pattern   string
		reqCookies []*http.Cookie
		want      bool
	}{
		{
			name:    "exact match",
			cookie:  "session",
			pattern: "abc123",
			reqCookies: []*http.Cookie{{Name: "session", Value: "abc123"}},
			want:    true,
		},
		{
			name:    "different value",
			cookie:  "session",
			pattern: "abc123",
			reqCookies: []*http.Cookie{{Name: "session", Value: "xyz789"}},
			want:    false,
		},
		{
			name:      "missing cookie",
			cookie:    "session",
			pattern:   "abc123",
			reqCookies: []*http.Cookie{},
			want:      false,
		},
		{
			name:    "regex match",
			cookie:  "session",
			pattern: "~^[a-f0-9]+$",
			reqCookies: []*http.Cookie{{Name: "session", Value: "abc123"}},
			want:    true,
		},
		{
			name:    "regex no match",
			cookie:  "session",
			pattern: "~^[0-9]+$",
			reqCookies: []*http.Cookie{{Name: "session", Value: "abc"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewCookieMatcher(tt.cookie, tt.pattern)
			if err != nil {
				t.Fatalf("NewCookieMatcher: %v", err)
			}
			req := httptest.NewRequest("GET", "/", nil)
			for _, c := range tt.reqCookies {
				req.AddCookie(c)
			}
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueryParamMatcher_Match(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		pattern string
		reqURL  string
		want    bool
	}{
		{"exact value", "page", "2", "/?page=2", true},
		{"different value", "page", "2", "/?page=3", false},
		{"missing key", "page", "2", "/", false},
		{"regex", "id", "~^\\d+$", "/?id=42", true},
		{"wildcard with value", "token", "*", "/?token=abc", true},
		{"absent required, absent", "debug", "!", "/", true},
		{"absent required, present", "debug", "!", "/?debug=1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewQueryParamMatcher(tt.key, tt.pattern)
			if err != nil {
				t.Fatalf("NewQueryParamMatcher: %v", err)
			}
			req := httptest.NewRequest("GET", tt.reqURL, nil)
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBodyExactMatcher_Match(t *testing.T) {
	m := matcher.NewBodyExactMatcher(`{"name":"Alice"}`)

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Alice"}`))
	if !m.Match(req) {
		t.Error("expected exact match")
	}

	req = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Bob"}`))
	if m.Match(req) {
		t.Error("expected no match for different body")
	}
}

func TestBodyRegexMatcher_Match(t *testing.T) {
	m, err := matcher.NewBodyRegexMatcher(`"name"\s*:\s*"Alice"`)
	if err != nil {
		t.Fatalf("NewBodyRegexMatcher: %v", err)
	}

	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Alice"}`))
	if !m.Match(req) {
		t.Error("expected regex match")
	}

	req = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Bob"}`))
	if m.Match(req) {
		t.Error("expected no match for different body")
	}
}

func TestBodyJSONPathMatcher_Match(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want bool
	}{
		{"match nested field", "$.user.name", `{"user":{"name":"Alice"}}`, true},
		{"missing field", "$.user.email", `{"user":{"name":"Alice"}}`, false},
		{"invalid JSON", "$.name", `not json`, false},
		{"empty body", "$.name", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewBodyJSONPathMatcher(tt.path)
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAcceptMatcher_Match(t *testing.T) {
	tests := []struct {
		name      string
		accept    string
		reqAccept string
		want      bool
	}{
		{"exact match", "application/json", "application/json", true},
		{"request wildcard */* matches any desired type", "application/json", "*/*", true},
		{"request type wildcard matches subtype", "application/json", "application/*", true},
		{"no match", "text/html", "application/json", false},
		// Empty Accept header means client accepts anything (per HTTP spec).
		{"empty accept header matches anything", "application/json", "", true},
		{"desired type wildcard matches any subtype", "application/*", "application/json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewAcceptMatcher(tt.accept)
			req := httptest.NewRequest("GET", "/", nil)
			if tt.reqAccept != "" {
				req.Header.Set("Accept", tt.reqAccept)
			}
			if got := m.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- CompositeMatcher.Match (AND semantics) ---

func TestCompositeMatcher_Match_AndSemantics(t *testing.T) {
	c := matcher.NewCompositeMatcher(
		matcher.NewMethodMatcher("GET"),
		matcher.NewPathExactMatcher("/api/users"),
	)

	tests := []struct {
		name    string
		method  string
		path    string
		want    bool
	}{
		{"both match", "GET", "/api/users", true},
		{"method mismatch", "POST", "/api/users", false},
		{"path mismatch", "GET", "/api/orders", false},
		{"both mismatch", "POST", "/api/orders", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := c.Match(req); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompositeMatcher_Match_Empty(t *testing.T) {
	c := matcher.NewCompositeMatcher()
	// Empty composite matches everything.
	req := httptest.NewRequest("GET", "/", nil)
	if !c.Match(req) {
		t.Error("empty composite should match any request")
	}
}

// --- String() descriptions ---

func TestMatcher_String(t *testing.T) {
	tests := []struct {
		name string
		m    matcher.Matcher
		want string
	}{
		{"method", matcher.NewMethodMatcher("GET"), "method=GET"},
		{"path exact", matcher.NewPathExactMatcher("/api"), "path exact=/api"},
		{"path regex", mustNewPathRegex(t, "/api/.*"), "path regex=/api/.*"},
		{"body exact", matcher.NewBodyExactMatcher("hello"), "body exact match"},
		{"body regex", mustNewBodyRegex(t, "hel.*"), "body regex=hel.*"},
		{"body jsonpath", matcher.NewBodyJSONPathMatcher("$.name"), "body jsonpath=$.name"},
		{"header", mustNewHeader(t, "Auth", "Bearer .*"), "header Auth=Bearer .*"},
		{"cookie", mustNewCookie(t, "session", ".*"), "cookie session=.*"},
		{"query", mustNewQuery(t, "page", "1"), "query page=1"},
		{"accept", matcher.NewAcceptMatcher("application/json"), "accept=application/json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompositeMatcher_String(t *testing.T) {
	c := matcher.NewCompositeMatcher(
		matcher.NewMethodMatcher("GET"),
		matcher.NewPathExactMatcher("/api"),
	)
	got := c.String()
	want := "Composite(method=GET && path exact=/api)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestCompositeMatcher_Matchers(t *testing.T) {
	m1 := matcher.NewMethodMatcher("GET")
	m2 := matcher.NewPathExactMatcher("/api")
	c := matcher.NewCompositeMatcher(m1, m2)

	got := c.Matchers()
	if len(got) != 2 {
		t.Fatalf("Matchers() returned %d matchers, want 2", len(got))
	}
	if got[0] != m1 || got[1] != m2 {
		t.Error("Matchers() did not return the original matchers in order")
	}
}

// --- Accessor methods ---

func TestMethodMatcher_Method(t *testing.T) {
	m := matcher.NewMethodMatcher("get")
	if got := m.Method(); got != "GET" {
		t.Errorf("Method() = %q, want %q (uppercase)", got, "GET")
	}
}

func TestPathExactMatcher_Path(t *testing.T) {
	m := matcher.NewPathExactMatcher("/api/users")
	if got := m.Path(); got != "/api/users" {
		t.Errorf("Path() = %q, want %q", got, "/api/users")
	}
}

func TestPathRegexMatcher_Raw(t *testing.T) {
	m, err := matcher.NewPathRegexMatcher("/api/.*")
	if err != nil {
		t.Fatalf("NewPathRegexMatcher: %v", err)
	}
	if got := m.Raw(); got != "/api/.*" {
		t.Errorf("Raw() = %q, want %q", got, "/api/.*")
	}
}

func TestHeaderMatcher_Accessors(t *testing.T) {
	m, err := matcher.NewHeaderMatcher("Authorization", "Bearer .*")
	if err != nil {
		t.Fatalf("NewHeaderMatcher: %v", err)
	}
	if got := m.Name(); got != "Authorization" {
		t.Errorf("Name() = %q, want %q", got, "Authorization")
	}
	if got := m.Pattern(); got != "Bearer .*" {
		t.Errorf("Pattern() = %q, want %q", got, "Bearer .*")
	}
}

func TestCookieMatcher_Accessors(t *testing.T) {
	m, err := matcher.NewCookieMatcher("session", ".*")
	if err != nil {
		t.Fatalf("NewCookieMatcher: %v", err)
	}
	if got := m.Name(); got != "session" {
		t.Errorf("Name() = %q, want %q", got, "session")
	}
	if got := m.Pattern(); got != ".*" {
		t.Errorf("Pattern() = %q, want %q", got, ".*")
	}
}

func TestQueryParamMatcher_Accessors(t *testing.T) {
	m, err := matcher.NewQueryParamMatcher("page", "1")
	if err != nil {
		t.Fatalf("NewQueryParamMatcher: %v", err)
	}
	if got := m.Key(); got != "page" {
		t.Errorf("Key() = %q, want %q", got, "page")
	}
	if got := m.Pattern(); got != "1" {
		t.Errorf("Pattern() = %q, want %q", got, "1")
	}
}

func TestAcceptMatcher_MediaType(t *testing.T) {
	m := matcher.NewAcceptMatcher("application/json")
	if got := m.MediaType(); got != "application/json" {
		t.Errorf("MediaType() = %q, want %q", got, "application/json")
	}
}

// --- AlwaysMatch ---

func TestAlwaysMatch(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if !matcher.AlwaysMatch.Match(req) {
		t.Error("AlwaysMatch should match any request")
	}
	matched, score := matcher.AlwaysMatch.ScoreMatch(req)
	if !matched {
		t.Error("AlwaysMatch.ScoreMatch should return matched=true")
	}
	if score != 0 {
		t.Errorf("AlwaysMatch.ScoreMatch score = %d, want 0", score)
	}
}

// --- MatcherFunc ---

func TestMatcherFunc_String(t *testing.T) {
	f := matcher.MatcherFunc(func(req *http.Request) (bool, int) {
		return true, 0
	})
	if got := f.String(); got != "custom matcher function" {
		t.Errorf("MatcherFunc.String() = %q, want %q", got, "custom matcher function")
	}
}

func TestNewBodyRegexMatcher_InvalidPattern(t *testing.T) {
	_, err := matcher.NewBodyRegexMatcher("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestMustNewPathRegexMatcher_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid regex")
		}
	}()
	matcher.MustNewPathRegexMatcher("[invalid")
}

func TestAcceptMatcher_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		accept    string
		reqAccept string
		want      bool
	}{
		{"q=0 means not acceptable", "application/json", "application/json;q=0", false},
		{"q=0.0 means not acceptable", "application/json", "application/json;q=0.0", false},
		{"quality value accepted", "application/json", "application/json;q=0.5", true},
		{"multiple ranges, one matches", "application/json", "text/html, application/json", true},
		{"multiple ranges, none matches", "application/json", "text/html, text/plain", false},
		{"invalid media range skipped", "application/json", "invalid", false},
		{"invalid media range followed by valid", "application/json", "invalid, application/json", true},
		{"empty accept entry in list", "application/json", ", , application/json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewAcceptMatcher(tt.accept)
			req := httptest.NewRequest("GET", "/", nil)
			if tt.reqAccept != "" {
				req.Header.Set("Accept", tt.reqAccept)
			}
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("ScoreMatch() matched = %v, want %v", matched, tt.want)
			}
			if tt.want && score != 7 {
				t.Errorf("ScoreMatch() score = %d, want 7", score)
			}
		})
	}
}

// --- helpers ---

func mustNewPathRegex(t *testing.T, pattern string) *matcher.PathRegexMatcher {
	t.Helper()
	m, err := matcher.NewPathRegexMatcher(pattern)
	if err != nil {
		t.Fatalf("NewPathRegexMatcher(%q): %v", pattern, err)
	}
	return m
}

func mustNewBodyRegex(t *testing.T, pattern string) *matcher.BodyRegexMatcher {
	t.Helper()
	m, err := matcher.NewBodyRegexMatcher(pattern)
	if err != nil {
		t.Fatalf("NewBodyRegexMatcher(%q): %v", pattern, err)
	}
	return m
}

func mustNewHeader(t *testing.T, name, pattern string) *matcher.HeaderMatcher {
	t.Helper()
	m, err := matcher.NewHeaderMatcher(name, pattern)
	if err != nil {
		t.Fatalf("NewHeaderMatcher(%q, %q): %v", name, pattern, err)
	}
	return m
}

func mustNewCookie(t *testing.T, name, pattern string) *matcher.CookieMatcher {
	t.Helper()
	m, err := matcher.NewCookieMatcher(name, pattern)
	if err != nil {
		t.Fatalf("NewCookieMatcher(%q, %q): %v", name, pattern, err)
	}
	return m
}

func mustNewQuery(t *testing.T, key, pattern string) *matcher.QueryParamMatcher {
	t.Helper()
	m, err := matcher.NewQueryParamMatcher(key, pattern)
	if err != nil {
		t.Fatalf("NewQueryParamMatcher(%q, %q): %v", key, pattern, err)
	}
	return m
}

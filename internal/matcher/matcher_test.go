package matcher_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/matcher"
)

func makeRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

func makeRequestWithBody(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, strings.NewReader(body))
}

func TestMethodMatcher(t *testing.T) {
	tests := []struct {
		name     string
		stubMethod string
		reqMethod  string
		want      bool
		wantScore int
	}{
		{"exact match", "GET", "GET", true, 10},
		{"case insensitive", "get", "GET", true, 10},
		{"different method", "GET", "POST", false, 0},
		{"empty stub method matches none", "", "GET", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewMethodMatcher(tt.stubMethod)
			req := makeRequest(tt.reqMethod, "/")
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("Match got %v, want %v", matched, tt.want)
			}
			if score != tt.wantScore {
				t.Errorf("Score got %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestPathExactMatcher(t *testing.T) {
	tests := []struct {
		name      string
		stubPath  string
		reqPath   string
		want      bool
		wantScore int
	}{
		{"exact match", "/api/users", "/api/users", true, 30},
		{"different path", "/api/users", "/api/orders", false, 0},
		{"case sensitive", "/api/Users", "/api/users", false, 0},
		{"trailing slash matters", "/api/users", "/api/users/", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewPathExactMatcher(tt.stubPath)
			req := makeRequest("GET", tt.reqPath)
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("Match got %v, want %v", matched, tt.want)
			}
			if score != tt.wantScore {
				t.Errorf("Score got %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestPathRegexMatcher(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		reqPath   string
		want      bool
		wantScore int
	}{
		{"matches users", `^/api/users/\d+$`, "/api/users/123", true, 15},
		{"doesn't match", `^/api/users/\d+$`, "/api/users/abc", false, 0},
		{"partial regex", `users`, "/api/users/123", true, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewPathRegexMatcher(tt.pattern)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := makeRequest("GET", tt.reqPath)
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("Match got %v, want %v", matched, tt.want)
			}
			if score != tt.wantScore {
				t.Errorf("Score got %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestPathRegexMatcherInvalid(t *testing.T) {
	_, err := matcher.NewPathRegexMatcher("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestHeaderMatcher(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		pattern   string
		reqHeaders map[string]string
		want      bool
		wantScore int
	}{
		{"exact match", "Authorization", "Bearer xyz", map[string]string{"Authorization": "Bearer xyz"}, true, 5},
		{"different value", "Authorization", "Bearer xyz", map[string]string{"Authorization": "Bearer abc"}, false, 0},
		{"missing header", "Authorization", "Bearer xyz", map[string]string{}, false, 0},
		{"regex match", "User-Agent", "~Mozilla.*", map[string]string{"User-Agent": "Mozilla/5.0"}, true, 5},
		{"regex no match", "User-Agent", "~^Chrome$", map[string]string{"User-Agent": "Mozilla/5.0"}, false, 0},
		{"wildcard with value", "X-Trace-Id", "*", map[string]string{"X-Trace-Id": "abc123"}, true, 5},
		{"wildcard missing", "X-Trace-Id", "*", map[string]string{}, false, 0},
		{"absent expected, present", "X-Forbidden", "!", map[string]string{"X-Forbidden": "yes"}, false, 0},
		{"absent expected, absent", "X-Forbidden", "!", map[string]string{}, true, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewHeaderMatcher(tt.header, tt.pattern)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := makeRequest("GET", "/")
			for k, v := range tt.reqHeaders {
				req.Header.Set(k, v)
			}
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("Match got %v, want %v", matched, tt.want)
			}
			if score != tt.wantScore {
				t.Errorf("Score got %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestQueryParamMatcher(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		pattern   string
		reqURL    string
		want      bool
		wantScore int
	}{
		{"exact value", "page", "2", "/?page=2", true, 3},
		{"different value", "page", "2", "/?page=3", false, 0},
		{"missing key", "page", "2", "/", false, 0},
		{"regex", "id", "~^\\d+$", "/?id=42", true, 3},
		{"wildcard with value", "token", "*", "/?token=abc", true, 3},
		{"absent required, absent", "debug", "!", "/", true, 3},
		{"absent required, present", "debug", "!", "/?debug=1", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := matcher.NewQueryParamMatcher(tt.key, tt.pattern)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := makeRequest("GET", tt.reqURL)
			matched, score := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("Match got %v, want %v", matched, tt.want)
			}
			if score != tt.wantScore {
				t.Errorf("Score got %d, want %d", score, tt.wantScore)
			}
		})
	}
}

func TestBodyExactMatcher(t *testing.T) {
	m := matcher.NewBodyExactMatcher(`{"name":"Alice"}`)

	matched, score := m.ScoreMatch(makeRequestWithBody("POST", "/", `{"name":"Alice"}`))
	if !matched || score != 20 {
		t.Errorf("exact match: got matched=%v score=%d, want true 20", matched, score)
	}

	matched, score = m.ScoreMatch(makeRequestWithBody("POST", "/", `{"name":"Bob"}`))
	if matched || score != 0 {
		t.Errorf("different body: got matched=%v score=%d, want false 0", matched, score)
	}
}

func TestBodyRegexMatcher(t *testing.T) {
	m, err := matcher.NewBodyRegexMatcher(`"name"\s*:\s*"Alice"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matched, score := m.ScoreMatch(makeRequestWithBody("POST", "/", `{"name":"Alice","age":30}`))
	if !matched || score != 10 {
		t.Errorf("regex match: got matched=%v score=%d, want true 10", matched, score)
	}

	matched, score = m.ScoreMatch(makeRequestWithBody("POST", "/", `{"name":"Bob"}`))
	if matched || score != 0 {
		t.Errorf("different body: got matched=%v score=%d, want false 0", matched, score)
	}
}

func TestBodyJSONPathMatcher(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want bool
	}{
		{"match nested field", "$.user.name", `{"user":{"name":"Alice"}}`, true},
		{"missing field returns false", "$.user.email", `{"user":{"name":"Alice"}}`, false},
		{"empty array returns false", "$.items", `{"items":[]}`, false},
		{"non-empty array matches", "$.items", `{"items":[1,2]}`, true},
		{"invalid JSON returns false", "$.name", `not json`, false},
		{"empty body returns false", "$.name", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matcher.NewBodyJSONPathMatcher(tt.path)
			req := makeRequestWithBody("POST", "/", tt.body)
			matched, _ := m.ScoreMatch(req)
			if matched != tt.want {
				t.Errorf("got %v, want %v", matched, tt.want)
			}
		})
	}
}

func TestCompositeMatcher(t *testing.T) {
	// AND of two matchers, both must match
	c := matcher.NewCompositeMatcher(
		matcher.NewMethodMatcher("GET"),
		matcher.NewPathExactMatcher("/api/users"),
	)

	matched, score := c.ScoreMatch(makeRequest("GET", "/api/users"))
	if !matched {
		t.Errorf("expected match")
	}
	if score != 40 { // 10 + 30
		t.Errorf("expected score 40, got %d", score)
	}

	matched, score = c.ScoreMatch(makeRequest("POST", "/api/users"))
	if matched {
		t.Errorf("expected no match")
	}
	// Score should still aggregate the path match (30) even though method failed
	if score != 30 {
		t.Errorf("expected partial score 30, got %d", score)
	}
}

func TestEmptyCompositeMatcher(t *testing.T) {
	c := matcher.NewCompositeMatcher()
	if !c.Match(makeRequest("GET", "/")) {
		t.Error("empty composite should match")
	}
}

func TestMatcherFunc(t *testing.T) {
	called := false
	f := matcher.MatcherFunc(func(req *http.Request) (bool, int) {
		called = true
		return req.URL.Path == "/test", 100
	})

	matched := f.Match(makeRequest("GET", "/test"))
	if !matched {
		t.Errorf("expected match")
	}
	if !called {
		t.Errorf("expected function to be called")
	}

	matched, score := f.ScoreMatch(makeRequest("GET", "/other"))
	if matched {
		t.Errorf("expected no match")
	}
	if score != 100 {
		t.Errorf("expected score 100, got %d", score)
	}
}
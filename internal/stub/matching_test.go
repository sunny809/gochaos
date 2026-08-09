package stub_test

import (
	"net/http/httptest"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
	"github.com/sunny809/gochaos/pkg/gmock"
)

// TestBuildMatcher_InvalidRegexSilentlyDropped verifies that invalid regex
// patterns in Headers, Cookies, and QueryParams are silently dropped rather
// than causing a panic. This is the current behavior — the matcher is simply
// not added, which means the stub may match more loosely than intended.
func TestBuildMatcher_InvalidRegexSilentlyDropped(t *testing.T) {
	// Invalid regex in header should not panic
	def := spec.StubDefinition{
		Request: spec.RequestPattern{
			Method:  "GET",
			URLPath: "/test",
			Headers: map[string]string{"X-Auth": "~[invalid"},
		},
		Response: spec.ResponseDefinition{Status: 200},
	}

	// BuildMatcher should not panic and should return a valid matcher
	matcher := stub.BuildMatcher(def.Request)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	// The request should still match because the invalid regex header matcher
	// was silently dropped.
	req := httptest.NewRequest("GET", "/test", nil)
	matched := matcher.Match(req)
	if !matched {
		t.Error("expected request to match when invalid regex header is silently dropped")
	}
}

func TestBuildMatcher_InvalidCookieRegexSilentlyDropped(t *testing.T) {
	def := spec.StubDefinition{
		Request: spec.RequestPattern{
			Method:  "GET",
			URLPath: "/test",
			Cookies: map[string]string{"session": "~[invalid"},
		},
		Response: spec.ResponseDefinition{Status: 200},
	}

	matcher := stub.BuildMatcher(def.Request)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	req := httptest.NewRequest("GET", "/test", nil)
	matched := matcher.Match(req)
	if !matched {
		t.Error("expected request to match when invalid regex cookie is silently dropped")
	}
}

func TestBuildMatcher_InvalidQueryParamRegexSilentlyDropped(t *testing.T) {
	def := spec.StubDefinition{
		Request: spec.RequestPattern{
			Method:      "GET",
			URLPath:     "/test",
			QueryParams: map[string]string{"filter": "~[invalid"},
		},
		Response: spec.ResponseDefinition{Status: 200},
	}

	matcher := stub.BuildMatcher(def.Request)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	req := httptest.NewRequest("GET", "/test", nil)
	matched := matcher.Match(req)
	if !matched {
		t.Error("expected request to match when invalid regex query param is silently dropped")
	}
}

func TestBuildMatcher_ValidRegexStillWorks(t *testing.T) {
	def := spec.StubDefinition{
		Request: spec.RequestPattern{
			Method:  "GET",
			URLPath: "/test",
			Headers: map[string]string{"Authorization": "~Bearer .*"},
		},
		Response: spec.ResponseDefinition{Status: 200},
	}

	matcher := stub.BuildMatcher(def.Request)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	// Request with matching header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	matched := matcher.Match(req)
	if !matched {
		t.Error("expected request to match with valid regex header")
	}

	// Request without matching header
	req2 := httptest.NewRequest("GET", "/test", nil)
	matched2 := matcher.Match(req2)
	if matched2 {
		t.Error("expected request to not match without Authorization header")
	}
}

func TestBuildMatcher_BodyRegexInvalidSilentlyDropped(t *testing.T) {
	def := spec.StubDefinition{
		Request: spec.RequestPattern{
			Method: "POST",
			Body: &gmock.BodyPattern{
				RegexMatch: "~[invalid",
			},
		},
		Response: spec.ResponseDefinition{Status: 200},
	}

	matcher := stub.BuildMatcher(def.Request)
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	req := httptest.NewRequest("POST", "/test", nil)
	matched := matcher.Match(req)
	if !matched {
		t.Error("expected request to match when invalid body regex is silently dropped")
	}
}

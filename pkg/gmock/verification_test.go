package gmock

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// V1-T4: Unit tests for verification matching helpers
// ---------------------------------------------------------------------------

// --- matchBody ---

func TestMatchBody_Exact(t *testing.T) {
	t.Run("positive match", func(t *testing.T) {
		if !matchBody(&BodyPattern{ExactMatch: "hello"}, "hello") {
			t.Error("expected exact match to succeed")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchBody(&BodyPattern{ExactMatch: "hello"}, "world") {
			t.Error("expected exact match to fail")
		}
	})
	t.Run("empty body mismatch", func(t *testing.T) {
		if matchBody(&BodyPattern{ExactMatch: "hello"}, "") {
			t.Error("expected exact match against empty body to fail")
		}
	})
}

func TestMatchBody_Regex(t *testing.T) {
	t.Run("positive match", func(t *testing.T) {
		if !matchBody(&BodyPattern{RegexMatch: "^h.*o$"}, "hello") {
			t.Error("expected regex match to succeed")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchBody(&BodyPattern{RegexMatch: "^h.*o$"}, "world") {
			t.Error("expected regex match to fail")
		}
	})
	t.Run("invalid regex does not panic", func(t *testing.T) {
		if matchBody(&BodyPattern{RegexMatch: "[invalid"}, "hello") {
			t.Error("expected invalid regex to produce no match")
		}
	})
	t.Run("regex matches empty string", func(t *testing.T) {
		if !matchBody(&BodyPattern{RegexMatch: "^$"}, "") {
			t.Error("expected regex matching empty string to succeed")
		}
	})
}

func TestMatchBody_JSONPath(t *testing.T) {
	body := `{"name":"alice","scores":[10,20],"meta":null,"empty":"","nested":{"key":"val"}}`

	t.Run("positive match string value", func(t *testing.T) {
		if !matchBody(&BodyPattern{JSONPath: "$.name"}, body) {
			t.Error("expected JSONPath $.name to match")
		}
	})
	t.Run("positive match array", func(t *testing.T) {
		if !matchBody(&BodyPattern{JSONPath: "$.scores"}, body) {
			t.Error("expected JSONPath $.scores to match")
		}
	})
	t.Run("positive match nested", func(t *testing.T) {
		if !matchBody(&BodyPattern{JSONPath: "$.nested"}, body) {
			t.Error("expected JSONPath $.nested to match")
		}
	})
	t.Run("non-matching path", func(t *testing.T) {
		if matchBody(&BodyPattern{JSONPath: "$.age"}, body) {
			t.Error("expected non-existent JSONPath to not match")
		}
	})
	t.Run("nil value does not match", func(t *testing.T) {
		if matchBody(&BodyPattern{JSONPath: "$.meta"}, body) {
			t.Error("expected JSONPath to nil to not match")
		}
	})
	t.Run("empty string does not match", func(t *testing.T) {
		if matchBody(&BodyPattern{JSONPath: "$.empty"}, body) {
			t.Error("expected JSONPath to empty string to not match")
		}
	})
	t.Run("non-JSON body does not match", func(t *testing.T) {
		if matchBody(&BodyPattern{JSONPath: "$.name"}, "not json") {
			t.Error("expected non-JSON body to not match")
		}
	})
	t.Run("empty body does not match", func(t *testing.T) {
		if matchBody(&BodyPattern{JSONPath: "$.name"}, "") {
			t.Error("expected empty body to not match")
		}
	})
}

func TestMatchBody_NilPattern(t *testing.T) {
	t.Run("nil pattern matches any body", func(t *testing.T) {
		if !matchBody(nil, "anything") {
			t.Error("expected nil pattern to match anything")
		}
	})
	t.Run("nil pattern matches empty body", func(t *testing.T) {
		if !matchBody(nil, "") {
			t.Error("expected nil pattern to match empty body")
		}
	})
}

func TestMatchBody_EmptyPattern(t *testing.T) {
	// A non-nil BodyPattern with all fields empty is treated as no constraint.
	if !matchBody(&BodyPattern{}, "anything") {
		t.Error("expected empty BodyPattern to match anything")
	}
}

// --- matchHeader ---

func headermap(k, v string) HeadersMap {
	return HeadersMap{http.CanonicalHeaderKey(k): {v}}
}

func TestMatchHeader_Exact(t *testing.T) {
	headers := HeadersMap{"X-Auth": {"token-123"}}

	t.Run("positive match", func(t *testing.T) {
		if !matchHeader("X-Auth", "token-123", headers) {
			t.Error("expected exact header match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchHeader("X-Auth", "wrong", headers) {
			t.Error("expected exact header mismatch")
		}
	})
	t.Run("canonical key normalization", func(t *testing.T) {
		if !matchHeader("x-auth", "token-123", headers) {
			t.Error("expected canonicalized header match")
		}
	})
	t.Run("missing header", func(t *testing.T) {
		if matchHeader("X-Missing", "value", headers) {
			t.Error("expected missing header to not match")
		}
	})
}

func TestMatchHeader_Regex(t *testing.T) {
	headers := HeadersMap{"X-Id": {"abc123"}, "X-Other": {""}}

	t.Run("positive match", func(t *testing.T) {
		if !matchHeader("X-Id", "~[a-z]+[0-9]+", headers) {
			t.Error("expected regex header match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchHeader("X-Id", "~^[0-9]+$", headers) {
			t.Error("expected regex header mismatch")
		}
	})
	t.Run("canonical key with regex", func(t *testing.T) {
		if !matchHeader("x-id", "~[a-z]+[0-9]+", headers) {
			t.Error("expected canonical key regex match")
		}
	})
}

func TestMatchHeader_Absent(t *testing.T) {
	t.Run("header absent returns true", func(t *testing.T) {
		if !matchHeader("X-Missing", "!", HeadersMap{"X-Other": {"val"}}) {
			t.Error("expected absent check to pass for missing header")
		}
	})
	t.Run("header present returns false", func(t *testing.T) {
		if matchHeader("X-Auth", "!", headermap("X-Auth", "val")) {
			t.Error("expected absent check to fail for present header")
		}
	})
}

func TestMatchHeader_Any(t *testing.T) {
	t.Run("non-empty value matches", func(t *testing.T) {
		if !matchHeader("X-Auth", "*", headermap("X-Auth", "val")) {
			t.Error("expected '*' to match non-empty value")
		}
	})
	t.Run("empty value does not match", func(t *testing.T) {
		headers := HeadersMap{"X-Auth": {""}}
		if matchHeader("X-Auth", "*", headers) {
			t.Error("expected '*' to not match empty value")
		}
	})
	t.Run("missing header does not match", func(t *testing.T) {
		if matchHeader("X-Missing", "*", HeadersMap{}) {
			t.Error("expected '*' to not match missing header")
		}
	})
}

// --- matchQueryParam ---

func TestMatchQueryParam_Exact(t *testing.T) {
	query := "page=2&limit=50"

	t.Run("positive match", func(t *testing.T) {
		if !matchQueryParam("page", "2", query) {
			t.Error("expected exact query param match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchQueryParam("page", "3", query) {
			t.Error("expected exact query param mismatch")
		}
	})
	t.Run("missing param", func(t *testing.T) {
		if matchQueryParam("missing", "val", query) {
			t.Error("expected missing query param to not match")
		}
	})
}

func TestMatchQueryParam_Regex(t *testing.T) {
	t.Run("positive match", func(t *testing.T) {
		if !matchQueryParam("id", "~[0-9]+", "id=123") {
			t.Error("expected regex query param match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchQueryParam("id", "~^[a-z]+$", "id=123") {
			t.Error("expected regex query param mismatch")
		}
	})
}

func TestMatchQueryParam_Absent(t *testing.T) {
	t.Run("param absent returns true", func(t *testing.T) {
		if !matchQueryParam("missing", "!", "page=1") {
			t.Error("expected absent check to pass for missing param")
		}
	})
	t.Run("param present returns false", func(t *testing.T) {
		if matchQueryParam("page", "!", "page=1") {
			t.Error("expected absent check to fail for present param")
		}
	})
}

func TestMatchQueryParam_Any(t *testing.T) {
	t.Run("non-empty value matches", func(t *testing.T) {
		if !matchQueryParam("key", "*", "key=val") {
			t.Error("expected '*' to match non-empty query param")
		}
	})
	t.Run("empty value does not match", func(t *testing.T) {
		if matchQueryParam("key", "*", "key=") {
			t.Error("expected '*' to not match empty query param value")
		}
	})
}

// --- matchCookie ---

func TestMatchCookie_Exact(t *testing.T) {
	headers := HeadersMap{"Cookie": {"session=abc; theme=dark"}}

	t.Run("positive match", func(t *testing.T) {
		if !matchCookie("session", "abc", headers) {
			t.Error("expected exact cookie match from Cookie header")
		}
	})
	t.Run("positive match second cookie", func(t *testing.T) {
		if !matchCookie("theme", "dark", headers) {
			t.Error("expected exact cookie match for second cookie")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchCookie("session", "xyz", headers) {
			t.Error("expected exact cookie mismatch")
		}
	})
}

func TestMatchCookie_Absent(t *testing.T) {
	headers := HeadersMap{"Cookie": {"session=abc"}}

	t.Run("cookie absent returns true", func(t *testing.T) {
		if !matchCookie("missing", "!", headers) {
			t.Error("expected absent check to pass for missing cookie")
		}
	})
	t.Run("cookie present returns false", func(t *testing.T) {
		if matchCookie("session", "!", headers) {
			t.Error("expected absent check to fail for present cookie")
		}
	})
}

func TestMatchCookie_FallbackToSetCookie(t *testing.T) {
	headers := HeadersMap{"Set-Cookie": {"session=xyz"}}
	if !matchCookie("session", "xyz", headers) {
		t.Error("expected match from Set-Cookie header")
	}
}

func TestMatchCookie_Any(t *testing.T) {
	t.Run("non-empty value matches", func(t *testing.T) {
		if !matchCookie("session", "*", HeadersMap{"Cookie": {"session=val"}}) {
			t.Error("expected '*' to match non-empty cookie")
		}
	})
	t.Run("missing cookie does not match", func(t *testing.T) {
		if matchCookie("missing", "*", HeadersMap{"Cookie": {"session=val"}}) {
			t.Error("expected '*' to not match missing cookie")
		}
	})
}

func TestMatchCookie_Regex(t *testing.T) {
	headers := HeadersMap{"Cookie": {"session=abc123"}}
	t.Run("positive match", func(t *testing.T) {
		if !matchCookie("session", "~[a-z]+[0-9]+", headers) {
			t.Error("expected regex cookie match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		if matchCookie("session", "~^[0-9]+$", headers) {
			t.Error("expected regex cookie mismatch")
		}
	})
}

// --- parseCookies ---

func TestParseCookies(t *testing.T) {
	t.Run("normal cookie string", func(t *testing.T) {
		result := parseCookies("session=abc; theme=dark")
		if len(result) != 2 {
			t.Fatalf("expected 2 cookies, got %d", len(result))
		}
		if result["session"] != "abc" {
			t.Errorf("expected session=abc, got %q", result["session"])
		}
		if result["theme"] != "dark" {
			t.Errorf("expected theme=dark, got %q", result["theme"])
		}
	})
	t.Run("empty string", func(t *testing.T) {
		result := parseCookies("")
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})
	t.Run("malformed no equals sign", func(t *testing.T) {
		// A segment without "=" is skipped entirely
		result := parseCookies("justtext")
		if len(result) != 0 {
			t.Errorf("expected empty map for malformed cookie string, got %d", len(result))
		}
	})
	t.Run("mixed valid and malformed", func(t *testing.T) {
		result := parseCookies("session=abc; malformed; theme=dark")
		if len(result) != 2 {
			t.Fatalf("expected 2 cookies, got %d", len(result))
		}
		if result["session"] != "abc" {
			t.Errorf("expected session=abc, got %q", result["session"])
		}
		if result["theme"] != "dark" {
			t.Errorf("expected theme=dark, got %q", result["theme"])
		}
	})
	t.Run("whitespace around tokens", func(t *testing.T) {
		result := parseCookies(" session = abc ; theme = dark ")
		if result["session"] != "abc" {
			t.Errorf("expected session=abc, got %q", result["session"])
		}
		if result["theme"] != "dark" {
			t.Errorf("expected theme=dark, got %q", result["theme"])
		}
	})
}

// --- matchPattern extension tests ---

func TestMatchPattern_URLPathRegex(t *testing.T) {
	req := LoggedRequest{Path: "/api/users/123"}

	t.Run("positive match", func(t *testing.T) {
		pat := RequestPattern{URLPathRegex: "^/api/users/"}
		if !matchPattern(pat, req) {
			t.Error("expected URLPathRegex to match")
		}
	})
	t.Run("negative match", func(t *testing.T) {
		pat := RequestPattern{URLPathRegex: "^/api/admins/"}
		if matchPattern(pat, req) {
			t.Error("expected URLPathRegex to not match")
		}
	})
	t.Run("invalid regex", func(t *testing.T) {
		pat := RequestPattern{URLPathRegex: "[invalid"}
		if matchPattern(pat, req) {
			t.Error("expected invalid URLPathRegex to not match")
		}
	})
}

func TestMatchPattern_Body(t *testing.T) {
	req := LoggedRequest{Body: `{"status":"active"}`}

	t.Run("exact body match", func(t *testing.T) {
		pat := RequestPattern{Body: &BodyPattern{ExactMatch: `{"status":"active"}`}}
		if !matchPattern(pat, req) {
			t.Error("expected body exact match")
		}
	})
	t.Run("body mismatch", func(t *testing.T) {
		pat := RequestPattern{Body: &BodyPattern{ExactMatch: `{"status":"inactive"}`}}
		if matchPattern(pat, req) {
			t.Error("expected body exact mismatch")
		}
	})
	t.Run("nil body pattern skips check", func(t *testing.T) {
		pat := RequestPattern{}
		if !matchPattern(pat, req) {
			t.Error("expected nil body pattern to skip check")
		}
	})
}

func TestMatchPattern_Headers(t *testing.T) {
	req := LoggedRequest{Headers: HeadersMap{"X-Auth": {"token-123"}, "X-Id": {"42"}}}

	t.Run("single header match", func(t *testing.T) {
		pat := RequestPattern{Headers: map[string]string{"X-Auth": "token-123"}}
		if !matchPattern(pat, req) {
			t.Error("expected header match")
		}
	})
	t.Run("multiple header match", func(t *testing.T) {
		pat := RequestPattern{Headers: map[string]string{
			"X-Auth": "token-123",
			"X-Id":   "42",
		}}
		if !matchPattern(pat, req) {
			t.Error("expected multiple header match")
		}
	})
	t.Run("header mismatch", func(t *testing.T) {
		pat := RequestPattern{Headers: map[string]string{"X-Auth": "wrong"}}
		if matchPattern(pat, req) {
			t.Error("expected header mismatch")
		}
	})
	t.Run("nil headers skips check", func(t *testing.T) {
		pat := RequestPattern{}
		if !matchPattern(pat, req) {
			t.Error("expected nil headers to skip check")
		}
	})
}

func TestMatchPattern_QueryParams(t *testing.T) {
	req := LoggedRequest{QueryString: "page=2&limit=50"}

	t.Run("query param match", func(t *testing.T) {
		pat := RequestPattern{QueryParams: map[string]string{"page": "2"}}
		if !matchPattern(pat, req) {
			t.Error("expected query param match")
		}
	})
	t.Run("multiple query param match", func(t *testing.T) {
		pat := RequestPattern{QueryParams: map[string]string{
			"page":  "2",
			"limit": "50",
		}}
		if !matchPattern(pat, req) {
			t.Error("expected multiple query param match")
		}
	})
	t.Run("query param mismatch", func(t *testing.T) {
		pat := RequestPattern{QueryParams: map[string]string{"page": "3"}}
		if matchPattern(pat, req) {
			t.Error("expected query param mismatch")
		}
	})
}

func TestMatchPattern_Cookies(t *testing.T) {
	req := LoggedRequest{Headers: HeadersMap{"Cookie": {"session=abc; theme=dark"}}}

	t.Run("cookie match", func(t *testing.T) {
		pat := RequestPattern{Cookies: map[string]string{"session": "abc"}}
		if !matchPattern(pat, req) {
			t.Error("expected cookie match")
		}
	})
	t.Run("cookie mismatch", func(t *testing.T) {
		pat := RequestPattern{Cookies: map[string]string{"session": "wrong"}}
		if matchPattern(pat, req) {
			t.Error("expected cookie mismatch")
		}
	})
}

func TestMatchPattern_Combined(t *testing.T) {
	req := LoggedRequest{
		Method:      "POST",
		Path:        "/api/reservations",
		QueryString: "location=nyc",
		Body:        `{"guests":2}`,
		Headers: HeadersMap{
			"Content-Type": {"application/json"},
			"Cookie":       {"session=abc"},
		},
	}

	pat := RequestPattern{
		Method:      "POST",
		URLPath:     "/api/reservations",
		QueryParams: map[string]string{"location": "nyc"},
		Body:        &BodyPattern{ExactMatch: `{"guests":2}`},
		Headers:     map[string]string{"Content-Type": "application/json"},
		Cookies:     map[string]string{"session": "abc"},
	}

	if !matchPattern(pat, req) {
		t.Error("expected combined pattern to match")
	}

	// Mismatch each dimension individually
	t.Run("method mismatch", func(t *testing.T) {
		p := pat
		p.Method = "GET"
		if matchPattern(p, req) {
			t.Error("expected method mismatch")
		}
	})
	t.Run("path mismatch", func(t *testing.T) {
		p := pat
		p.URLPath = "/wrong"
		if matchPattern(p, req) {
			t.Error("expected path mismatch")
		}
	})
	t.Run("body mismatch", func(t *testing.T) {
		p := pat
		p.Body = &BodyPattern{ExactMatch: `{"guests":3}`}
		if matchPattern(p, req) {
			t.Error("expected body mismatch")
		}
	})
	t.Run("header mismatch", func(t *testing.T) {
		p := pat
		p.Headers = map[string]string{"Content-Type": "text/plain"}
		if matchPattern(p, req) {
			t.Error("expected header mismatch")
		}
	})
	t.Run("query mismatch", func(t *testing.T) {
		p := pat
		p.QueryParams = map[string]string{"location": "lax"}
		if matchPattern(p, req) {
			t.Error("expected query mismatch")
		}
	})
	t.Run("cookie mismatch", func(t *testing.T) {
		p := pat
		p.Cookies = map[string]string{"session": "wrong"}
		if matchPattern(p, req) {
			t.Error("expected cookie mismatch")
		}
	})
}

// --- copyMap ---

func TestCopyMap(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := copyMap(nil)
		if result != nil {
			t.Error("expected nil result for nil input")
		}
	})
	t.Run("shallow copy", func(t *testing.T) {
		original := map[string]string{"a": "1", "b": "2"}
		result := copyMap(original)
		if len(result) != 2 || result["a"] != "1" || result["b"] != "2" {
			t.Error("expected equal copy")
		}
		// Mutate original, ensure copy unchanged
		original["a"] = "changed"
		if result["a"] != "1" {
			t.Error("expected copy to be independent of original")
		}
	})
	t.Run("empty map", func(t *testing.T) {
		result := copyMap(map[string]string{})
		if result == nil || len(result) != 0 {
			t.Error("expected non-nil empty map")
		}
	})
}

// ---------------------------------------------------------------------------
// V1-T5: Integration tests (real server)
// ---------------------------------------------------------------------------

// startTestServer is a helper to start a gmock server on a random port.
func startTestServer(t *testing.T) Server {
	t.Helper()
	server := NewServer(WithPort(0))
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	t.Cleanup(func() { server.Stop() })
	return server
}

func sendGet(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	resp.Body.Close()
}

func sendPost(t *testing.T, url, contentType, body string) {
	t.Helper()
	resp, err := http.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	resp.Body.Close()
}

func sendPostWithHeaders(t *testing.T, url, contentType, body string, headers map[string]string) {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	resp.Body.Close()
}

func TestVerify_BodyExactMatch(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	// Register a simple stub to accept requests
	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "POST",
			URLPath: "/api/events",
		},
		Response: ResponseDefinition{Status: 200},
	})

	// Send a request with body
	sendPost(t, url+"/api/events", "application/json", `{"status":"active"}`)

	// Verify with matching body
	result := server.Verify(RequestPattern{
		Method:  "POST",
		URLPath: "/api/events",
		Body:    &BodyPattern{ExactMatch: `{"status":"active"}`},
	}, 1)
	if !result.Matched {
		t.Error("expected body exact match to verify, got errors:", result.Errors)
	}

	// Verify with non-matching body
	result = server.Verify(RequestPattern{
		Method:  "POST",
		URLPath: "/api/events",
		Body:    &BodyPattern{ExactMatch: `{"status":"inactive"}`},
	}, 1)
	if result.Matched {
		t.Error("expected body exact mismatch to not verify")
	}
}

func TestVerify_HeaderExact(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "GET",
			URLPath: "/api/auth",
		},
		Response: ResponseDefinition{Status: 200},
	})

	// Send request with custom header via GET
	req, err := http.NewRequest("GET", url+"/api/auth", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Auth", "bearer-token-123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	result := server.Verify(RequestPattern{
		Method:  "GET",
		URLPath: "/api/auth",
		Headers: map[string]string{"X-Auth": "bearer-token-123"},
	}, 1)
	if !result.Matched {
		t.Error("expected header exact match to verify, got errors:", result.Errors)
	}
}

func TestVerify_QueryParamExact(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "GET",
			URLPath: "/api/items",
		},
		Response: ResponseDefinition{Status: 200},
	})

	sendGet(t, url+"/api/items?page=2&limit=50")

	result := server.Verify(RequestPattern{
		Method:      "GET",
		URLPath:     "/api/items",
		QueryParams: map[string]string{"page": "2", "limit": "50"},
	}, 1)
	if !result.Matched {
		t.Error("expected query param exact match to verify, got errors:", result.Errors)
	}
}

func TestVerify_CombinedPattern(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "POST",
			URLPath: "/api/reservations",
		},
		Response: ResponseDefinition{Status: 201},
	})

	// Send request with body, headers, query params
	body := `{"guests":2,"date":"2026-06-14"}`
	sendPostWithHeaders(t, url+"/api/reservations?source=web", "application/json", body, map[string]string{
		"X-Request-Id": "req-123",
	})

	fullPattern := RequestPattern{
		Method:      "POST",
		URLPath:     "/api/reservations",
		QueryParams: map[string]string{"source": "web"},
		Body:        &BodyPattern{ExactMatch: `{"guests":2,"date":"2026-06-14"}`},
		Headers:     map[string]string{"Content-Type": "application/json", "X-Request-Id": "req-123"},
	}

	t.Run("full pattern matches", func(t *testing.T) {
		result := server.Verify(fullPattern, 1)
		if !result.Matched {
			t.Error("expected combined pattern to verify, got errors:", result.Errors)
		}
	})

	t.Run("body mismatch fails", func(t *testing.T) {
		p := fullPattern
		p.Body = &BodyPattern{ExactMatch: `{"guests":3}`}
		result := server.Verify(p, 1)
		if result.Matched {
			t.Error("expected body mismatch to fail verification")
		}
	})

	t.Run("header mismatch fails", func(t *testing.T) {
		p := fullPattern
		p.Headers = map[string]string{"X-Request-Id": "wrong"}
		result := server.Verify(p, 1)
		if result.Matched {
			t.Error("expected header mismatch to fail verification")
		}
	})

	t.Run("query param mismatch fails", func(t *testing.T) {
		p := fullPattern
		p.QueryParams = map[string]string{"source": "mobile"}
		result := server.Verify(p, 1)
		if result.Matched {
			t.Error("expected query param mismatch to fail verification")
		}
	})
}

func TestVerifyNotCalled_RespectsBody(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "POST",
			URLPath: "/api/orders",
		},
		Response: ResponseDefinition{Status: 200},
	})

	// Send request with specific body
	sendPost(t, url+"/api/orders", "application/json", `{"item":"book"}`)

	// Body that was NOT sent should return matched=true (not called)
	result := server.VerifyNotCalled(RequestPattern{
		Method:  "POST",
		URLPath: "/api/orders",
		Body:    &BodyPattern{ExactMatch: `{"item":"laptop"}`},
	})
	if !result.Matched {
		t.Error("expected VerifyNotCalled to return true for unmatched body")
	}

	// Body that WAS sent should return matched=false (was called)
	result = server.VerifyNotCalled(RequestPattern{
		Method:  "POST",
		URLPath: "/api/orders",
		Body:    &BodyPattern{ExactMatch: `{"item":"book"}`},
	})
	if result.Matched {
		t.Error("expected VerifyNotCalled to return false for matched body")
	}
}

func TestVerify_BackwardCompat(t *testing.T) {
	// Identical to ExampleServer_verify — method+path only
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "POST",
			URLPath: "/api/events",
		},
		Response: ResponseDefinition{Status: 201},
	})

	http.Post(url+"/api/events", "application/json", nil)
	http.Post(url+"/api/events", "application/json", nil)

	// Verify exactly 2 requests
	result := server.Verify(RequestPattern{
		Method:  "POST",
		URLPath: "/api/events",
	}, 2)
	if !result.Matched {
		t.Error("backward compat: expected matched=true, got errors:", result.Errors)
	}
	if result.ActualCount != 2 {
		t.Errorf("backward compat: expected count 2, got %d", result.ActualCount)
	}
	if result.BodyPattern != nil {
		t.Error("backward compat: expected BodyPattern nil for method+path only")
	}
	if result.HeaderPattern != nil {
		t.Error("backward compat: expected HeaderPattern nil for method+path only")
	}
	if result.QueryParamPattern != nil {
		t.Error("backward compat: expected QueryParamPattern nil for method+path only")
	}

	// Verify no unmatched method
	result = server.VerifyNotCalled(RequestPattern{
		Method:  "DELETE",
		URLPath: "/api/events",
	})
	if !result.Matched {
		t.Error("backward compat: expected VerifyNotCalled matched=true")
	}
}

func TestVerify_PopulatesPatternFields(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "POST",
			URLPath: "/api/test",
		},
		Response: ResponseDefinition{Status: 200},
	})

	sendPost(t, url+"/api/test", "application/json", `{"key":"val"}`)

	t.Run("all pattern fields populated", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:      "POST",
			URLPath:     "/api/test",
			Body:        &BodyPattern{ExactMatch: `{"key":"val"}`},
			Headers:     map[string]string{"Content-Type": "application/json"},
			QueryParams: map[string]string{"q": "1"},
		}, 1)
		if result.BodyPattern == nil {
			t.Error("expected BodyPattern to be non-nil")
		}
		if result.HeaderPattern == nil {
			t.Error("expected HeaderPattern to be non-nil")
		}
		if result.QueryParamPattern == nil {
			t.Error("expected QueryParamPattern to be non-nil")
		}
	})

	t.Run("fields nil when dimensions not set", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:  "POST",
			URLPath: "/api/test",
		}, 1)
		if result.BodyPattern != nil {
			t.Error("expected BodyPattern nil when not set")
		}
		if result.HeaderPattern != nil {
			t.Error("expected HeaderPattern nil when not set")
		}
		if result.QueryParamPattern != nil {
			t.Error("expected QueryParamPattern nil when not set")
		}
	})
}

func TestVerify_URLPathRegex(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "GET",
			URLPath: "/api/users/123",
		},
		Response: ResponseDefinition{Status: 200},
	})

	sendGet(t, url+"/api/users/123")

	t.Run("matching regex", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:       "GET",
			URLPathRegex: "^/api/users/",
		}, 1)
		if !result.Matched {
			t.Error("expected URLPathRegex to verify, got errors:", result.Errors)
		}
	})

	t.Run("non-matching regex", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:       "GET",
			URLPathRegex: "^/api/admins/",
		}, 1)
		if result.Matched {
			t.Error("expected non-matching URLPathRegex to not verify")
		}
	})
}

func TestVerify_CookieMatch(t *testing.T) {
	server := startTestServer(t)
	url := server.URL()

	server.Stub(StubDefinition{
		Request: RequestPattern{
			Method:  "GET",
			URLPath: "/api/dashboard",
		},
		Response: ResponseDefinition{Status: 200},
	})

	// Send request with cookies
	req, err := http.NewRequest("GET", url+"/api/dashboard", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", "session=abc123; theme=light")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	t.Run("matching cookie", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:  "GET",
			URLPath: "/api/dashboard",
			Cookies: map[string]string{"session": "abc123"},
		}, 1)
		if !result.Matched {
			t.Error("expected cookie match to verify, got errors:", result.Errors)
		}
	})

	t.Run("non-matching cookie", func(t *testing.T) {
		result := server.Verify(RequestPattern{
			Method:  "GET",
			URLPath: "/api/dashboard",
			Cookies: map[string]string{"session": "wrong"},
		}, 1)
		if result.Matched {
			t.Error("expected non-matching cookie to not verify")
		}
	})
}

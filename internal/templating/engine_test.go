package templating

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestEngineRender(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		template string
		method   string
		path     string
		headers  map[string]string
		query    map[string]string
		wantErr  bool
		check    func(t *testing.T, got string)
	}{
		{
			name:     "basic template with request method",
			template: `Method: {{.Request.Method}}`,
			method:   http.MethodPost,
			path:     "/api/users",
			check: func(t *testing.T, got string) {
				if got != "Method: POST" {
					t.Errorf("expected 'Method: POST', got %q", got)
				}
			},
		},
		{
			name:     "request path",
			template: `Path: {{.Request.Path}}`,
			method:   http.MethodGet,
			path:     "/api/items",
			check: func(t *testing.T, got string) {
				if got != "Path: /api/items" {
					t.Errorf("expected 'Path: /api/items', got %q", got)
				}
			},
		},
		{
			name:     "request header",
			template: `Authorization: {{.Request.Header "Authorization"}}`,
			method:   http.MethodGet,
			path:     "/",
			headers:  map[string]string{"Authorization": "Bearer token123"},
			check: func(t *testing.T, got string) {
				if got != "Authorization: Bearer token123" {
					t.Errorf("expected 'Authorization: Bearer token123', got %q", got)
				}
			},
		},
		{
			name:     "request query parameter",
			template: `Page: {{.Request.Query "page"}}`,
			method:   http.MethodGet,
			path:     "/",
			query:    map[string]string{"page": "42"},
			check: func(t *testing.T, got string) {
				if got != "Page: 42" {
					t.Errorf("expected 'Page: 42', got %q", got)
				}
			},
		},
		{
			name:     "missing header returns empty string",
			template: `Missing: "{{.Request.Header "X-Missing"}}"`,
			method:   http.MethodGet,
			path:     "/",
			check: func(t *testing.T, got string) {
				if got != `Missing: ""` {
					t.Errorf("expected empty string for missing header, got %q", got)
				}
			},
		},
		{
			name:     "missing query returns empty string",
			template: `Missing: "{{.Request.Query "missing"}}"`,
			method:   http.MethodGet,
			path:     "/",
			check: func(t *testing.T, got string) {
				if got != `Missing: ""` {
					t.Errorf("expected empty string for missing query, got %q", got)
				}
			},
		},
		{
			name:     "template with syntax error",
			template: `{{.Request.Method`,
			method:   http.MethodGet,
			path:     "/",
			wantErr:  true,
			check:    nil,
		},
		{
			name:     "template with unknown field",
			template: `{{.Request.UnknownField}}`,
			method:   http.MethodGet,
			path:     "/",
			wantErr:  true,
			check:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildRequest(t, tt.method, tt.path, tt.headers, tt.query)
			got, err := engine.Render(tt.template, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got: %q", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestEngineRandomUUID(t *testing.T) {
	engine := NewEngine()
	template := `{{randomUUID}}`

	req := buildRequest(t, http.MethodGet, "/", nil, nil)
	got, err := engine.Render(template, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UUID v4 pattern: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(got) {
		t.Errorf("expected valid UUID v4, got: %q", got)
	}
}

func TestEngineNow(t *testing.T) {
	engine := NewEngine()
	template := `{{now}}`

	req := buildRequest(t, http.MethodGet, "/", nil, nil)
	got, err := engine.Render(template, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RFC3339 format: 2006-01-02T15:04:05Z07:00
	// Should contain a 'T' and end with 'Z' (UTC)
	if !strings.Contains(got, "T") || !strings.HasSuffix(got, "Z") {
		t.Errorf("expected RFC3339 format, got: %q", got)
	}
}

func TestEngineRandomInt(t *testing.T) {
	engine := NewEngine()
	template := `{{randomInt 10 20}}`

	req := buildRequest(t, http.MethodGet, "/", nil, nil)

	// Run multiple times to verify range
	for i := 0; i < 50; i++ {
		got, err := engine.Render(template, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Parse the integer value
		var parsed int
		for _, c := range got {
			if c >= '0' && c <= '9' {
				parsed = parsed*10 + int(c-'0')
			}
		}

		if parsed < 10 || parsed > 20 {
			t.Errorf("randomInt result %d out of range [10, 20]", parsed)
		}
	}
}

func TestEngineRandomIntRange(t *testing.T) {
	engine := NewEngine()

	// Test with swapped min/max (should handle gracefully)
	template := `{{randomInt 100 50}}`
	req := buildRequest(t, http.MethodGet, "/", nil, nil)

	seen := make(map[int]bool)
	for i := 0; i < 100; i++ {
		got, err := engine.Render(template, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Parse the integer value
		var parsed int
		for _, c := range got {
			if c >= '0' && c <= '9' {
				parsed = parsed*10 + int(c-'0')
			}
		}

		if parsed < 50 || parsed > 100 {
			t.Errorf("randomInt result %d out of expected range [50, 100]", parsed)
		}
		seen[parsed] = true
	}

	// Verify we get some variety (not all the same)
	if len(seen) < 2 {
		t.Error("randomInt produced the same value across 100 iterations")
	}
}

func TestEngineRandomIntSingleValue(t *testing.T) {
	engine := NewEngine()
	template := `{{randomInt 5 5}}`

	req := buildRequest(t, http.MethodGet, "/", nil, nil)
	for i := 0; i < 10; i++ {
		got, err := engine.Render(template, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "5" {
			t.Errorf("expected 5, got %q", got)
		}
	}
}

func TestEngineStaticText(t *testing.T) {
	engine := NewEngine()
	template := `Hello, world!`

	req := buildRequest(t, http.MethodGet, "/", nil, nil)
	got, err := engine.Render(template, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", got)
	}
}

func TestEngineMultipleFunctions(t *testing.T) {
	engine := NewEngine()
	template := `{{.Request.Method}} {{randomInt 1 1}} {{now}}`

	req := buildRequest(t, http.MethodPost, "/", nil, nil)
	got, err := engine.Render(template, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(got, "POST 1 ") {
		t.Errorf("expected prefix 'POST 1 ', got: %q", got)
	}
}

// buildRequest creates an HTTP request for testing.
func TestEngineCacheConcurrency(t *testing.T) {
	engine := NewEngine()
	template := `Method: {{.Request.Method}}`

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := buildRequest(t, http.MethodGet, "/", nil, nil)
			for j := 0; j < 10; j++ {
				got, err := engine.Render(template, req)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if got != "Method: GET" {
					t.Errorf("expected 'Method: GET', got %q", got)
				}
			}
		}()
	}
	wg.Wait()
}

// buildRequest creates an HTTP request for testing.
func buildRequest(t *testing.T, method, path string, headers map[string]string, query map[string]string) *http.Request {
	u, err := url.Parse("http://localhost" + path)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req
}

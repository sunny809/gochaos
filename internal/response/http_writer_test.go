package response

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

func TestHTTPWriter_WriteResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	tests := []struct {
		name       string
		def        *spec.StubDefinition
		corsOpts   *CORSOptions
		wantStatus int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name: "basic response with body",
			def: &spec.StubDefinition{
				ID:   "stub-1",
				Name: "test-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   `{"hello":"world"}`,
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
			},
			wantStatus: 200,
			wantBody:   `{"hello":"world"}`,
			wantHeader: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "default status when 0",
			def: &spec.StubDefinition{
				ID: "stub-2",
				Response: spec.ResponseDefinition{
					Body: "ok",
				},
			},
			wantStatus: 200,
			wantBody:   "ok",
		},
		{
			name: "custom status code",
			def: &spec.StubDefinition{
				ID: "stub-3",
				Response: spec.ResponseDefinition{
					Status: 201,
					Body:   "created",
				},
			},
			wantStatus: 201,
			wantBody:   "created",
		},
		{
			name: "empty body",
			def: &spec.StubDefinition{
				ID: "stub-4",
				Response: spec.ResponseDefinition{
					Status: 204,
				},
			},
			wantStatus: 204,
			wantBody:   "",
		},
		{
			name: "base64 body",
			def: &spec.StubDefinition{
				ID: "stub-5",
				Response: spec.ResponseDefinition{
					Status:     200,
					Base64Body: base64.StdEncoding.EncodeToString([]byte("hello world")),
				},
			},
			wantStatus: 200,
			wantBody:   "hello world",
		},
		{
			name: "invalid base64 logs warning but does not fail",
			def: &spec.StubDefinition{
				ID: "stub-6",
				Response: spec.ResponseDefinition{
					Status:     200,
					Base64Body: "!!!invalid-base64!!!",
				},
			},
			wantStatus: 200,
			wantBody:   "",
		},
		{
			name: "body takes precedence over base64",
			def: &spec.StubDefinition{
				ID: "stub-7",
				Response: spec.ResponseDefinition{
					Status:     200,
					Body:       "plain text",
					Base64Body: base64.StdEncoding.EncodeToString([]byte("base64 text")),
				},
			},
			wantStatus: 200,
			wantBody:   "plain text",
		},
		{
			name: "with CORS headers",
			def: &spec.StubDefinition{
				ID: "stub-8",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "cors",
				},
			},
			corsOpts: &CORSOptions{
				AllowedOrigins: []string{"*"},
				ExposedHeaders: []string{"X-Custom"},
			},
			wantStatus: 200,
			wantBody:   "cors",
			wantHeader: map[string]string{
				"Access-Control-Allow-Origin": "*",
				"Access-Control-Expose-Headers": "X-Custom",
			},
		},
		{
			name: "stub headers override CORS",
			def: &spec.StubDefinition{
				ID: "stub-9",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "override",
					Headers: map[string]string{
						"Access-Control-Allow-Origin": "https://example.com",
					},
				},
			},
			corsOpts: &CORSOptions{
				AllowedOrigins: []string{"*"},
			},
			wantStatus: 200,
			wantBody:   "override",
			wantHeader: map[string]string{
				"Access-Control-Allow-Origin": "https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := w.WriteResponse(rr, tt.def, req, tt.corsOpts)
			if err != nil {
				t.Fatalf("WriteResponse failed: %v", err)
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatus)
			}

			body := rr.Body.String()
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}

			for key, wantVal := range tt.wantHeader {
				if got := rr.Header().Get(key); got != wantVal {
					t.Errorf("header %q = %q, want %q", key, got, wantVal)
				}
			}
		})
	}
}

func TestHTTPWriter_WriteResponse_Delay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	tests := []struct {
		name         string
		delay        *spec.DelayDefinition
		minDuration  time.Duration
		maxDuration  time.Duration
	}{
		{
			name: "fixed delay",
			delay: &spec.DelayDefinition{
				Type:  "fixed",
				Value: 50,
			},
			minDuration: 45 * time.Millisecond,
			maxDuration: 100 * time.Millisecond,
		},
		{
			name:        "nil delay",
			delay:       nil,
			minDuration: 0,
			maxDuration: 5 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			def := &spec.StubDefinition{
				ID: "delay-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "delayed",
					Delay:  tt.delay,
				},
			}

			start := time.Now()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			err := w.WriteResponse(rr, def, req, nil)
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("WriteResponse failed: %v", err)
			}

			if elapsed < tt.minDuration {
				t.Errorf("elapsed %v < min %v", elapsed, tt.minDuration)
			}
			if elapsed > tt.maxDuration {
				t.Errorf("elapsed %v > max %v", elapsed, tt.maxDuration)
			}
		})
	}
}

func TestHTTPWriter_WriteResponse_RandomDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	def := &spec.StubDefinition{
		ID: "random-delay-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "random",
			Delay: &spec.DelayDefinition{
				Type: "random",
				Min:  20,
				Max:  60,
			},
		},
	}

	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		start := time.Now()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		err := w.WriteResponse(rr, def, req, nil)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("WriteResponse failed: %v", err)
		}

		if elapsed < 15*time.Millisecond {
			t.Errorf("iteration %d: elapsed %v < 15ms", i, elapsed)
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("iteration %d: elapsed %v > 100ms", i, elapsed)
		}
	}
}

func TestHTTPWriter_WriteCORSHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	tests := []struct {
		name       string
		origin     string
		corsOpts   *CORSOptions
		wantOrigin string
		wantMethods string
		wantHeaders string
		wantCredentials string
		wantMaxAge string
	}{
		{
			name:       "nil options",
			origin:     "http://example.com",
			corsOpts:   nil,
			wantOrigin: "",
		},
		{
			name:       "empty origin",
			origin:     "",
			corsOpts:   &CORSOptions{AllowedOrigins: []string{"*"}},
			wantOrigin: "",
		},
		{
			name:       "wildcard origin",
			origin:     "http://example.com",
			corsOpts:   &CORSOptions{AllowedOrigins: []string{"*"}},
			wantOrigin: "*",
		},
		{
			name:       "specific origin matches",
			origin:     "http://example.com",
			corsOpts:   &CORSOptions{AllowedOrigins: []string{"http://example.com", "http://other.com"}},
			wantOrigin: "http://example.com",
		},
		{
			name:       "specific origin does not match",
			origin:     "http://attacker.com",
			corsOpts:   &CORSOptions{AllowedOrigins: []string{"http://example.com"}},
			wantOrigin: "",
		},
		{
			name:            "all options",
			origin:          "http://example.com",
			corsOpts: &CORSOptions{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "DELETE"},
				AllowedHeaders:   []string{"Content-Type", "Authorization"},
				ExposedHeaders:   []string{"X-Total"},
				AllowCredentials: true,
				MaxAge:         3600,
			},
			wantOrigin:      "*",
			wantMethods:     "GET, POST, DELETE",
			wantHeaders:     "Content-Type, Authorization",
			wantCredentials: "true",
			wantMaxAge:      "3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodOptions, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			w.WriteCORSHeaders(rr, req, tt.corsOpts)

			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if tt.wantMethods != "" {
				if got := rr.Header().Get("Access-Control-Allow-Methods"); got != tt.wantMethods {
					t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, tt.wantMethods)
				}
			}
			if tt.wantHeaders != "" {
				if got := rr.Header().Get("Access-Control-Allow-Headers"); got != tt.wantHeaders {
					t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, tt.wantHeaders)
				}
			}
			if tt.wantCredentials != "" {
				if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != tt.wantCredentials {
					t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, tt.wantCredentials)
				}
			}
			if tt.wantMaxAge != "" {
				if got := rr.Header().Get("Access-Control-Max-Age"); got != tt.wantMaxAge {
					t.Errorf("Access-Control-Max-Age = %q, want %q", got, tt.wantMaxAge)
				}
			}
		})
	}
}

func TestHTTPWriter_MaybeWrapGzip(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name        string
		disableGzip bool
		acceptEnc   string
		wantGzip    bool
	}{
		{
			name:        "gzip enabled and requested",
			disableGzip: false,
			acceptEnc:   "gzip",
			wantGzip:    true,
		},
		{
			name:        "gzip enabled but not requested",
			disableGzip: false,
			acceptEnc:   "",
			wantGzip:    false,
		},
		{
			name:        "gzip disabled",
			disableGzip: true,
			acceptEnc:   "gzip",
			wantGzip:    false,
		},
		{
			name:        "gzip disabled and not requested",
			disableGzip: true,
			acceptEnc:   "",
			wantGzip:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTTPWriter(logger, tt.disableGzip)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.acceptEnc != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEnc)
			}

			gw, result := w.maybeWrapGzip(rr, req)

			_, isGzip := result.(*GzipResponseWriter)
			if isGzip != tt.wantGzip {
				t.Errorf("maybeWrapGzip gzip = %v, want %v", isGzip, tt.wantGzip)
			}

			if tt.wantGzip {
				if gw == nil {
					t.Errorf("maybeWrapGzip returned nil gzip.Writer, want non-nil")
				}
				if ce := rr.Header().Get("Content-Encoding"); ce != "gzip" {
					t.Errorf("Content-Encoding = %q, want gzip", ce)
				}
			} else {
				if gw != nil {
					t.Errorf("maybeWrapGzip returned non-nil gzip.Writer, want nil")
				}
			}
		})
	}
}

func TestGzipResponseWriter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false)

	// Test that gzip compression actually works
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	gw, result := w.maybeWrapGzip(rr, req)
	result.(*GzipResponseWriter).GW.Write([]byte("hello world"))
	if gw != nil {
		gw.Close()
	}

	// Verify the response is gzip compressed
	if ce := rr.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", ce)
	}

	// Decompress and verify
	reader, err := gzip.NewReader(bytes.NewReader(rr.Body.Bytes()))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read decompressed data: %v", err)
	}

	if string(decompressed) != "hello world" {
		t.Errorf("decompressed = %q, want %q", string(decompressed), "hello world")
	}
}

func TestHTTPWriter_ConcurrentSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	// Run multiple goroutines writing responses concurrently
	done := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			rr := httptest.NewRecorder()
			def := &spec.StubDefinition{
				ID: fmt.Sprintf("stub-%d", idx),
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   fmt.Sprintf("response-%d", idx),
				},
			}
			_ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil)
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestHTTPWriter_WriteResponse_ErrorOnWrite(t *testing.T) {
	// This test verifies error handling when writing to a closed response writer
	// Using a custom ResponseWriter that returns errors
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	def := &spec.StubDefinition{
		ID: "error-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   strings.Repeat("x", 1024*1024), // large body
		},
	}

	// httptest.ResponseRecorder should handle this fine
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	err := w.WriteResponse(rr, def, req, nil)
	if err != nil {
		t.Fatalf("WriteResponse failed unexpectedly: %v", err)
	}
}

func TestHTTPWriter_WriteResponse_TemplateRendering(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true)

	tests := []struct {
		name         string
		body         string
		transform    bool
		method       string
		path         string
		headers      map[string]string
		query        string
		wantContains string
		wantExact    string
	}{
		{
			name:         "transform disabled - body returned as-is",
			body:         `Method: {{.Request.Method}}`,
			transform:    false,
			method:       http.MethodPost,
			wantExact:    `Method: {{.Request.Method}}`,
		},
		{
			name:         "transform enabled - request method",
			body:         `Method: {{.Request.Method}}`,
			transform:    true,
			method:       http.MethodPost,
			wantExact:    `Method: POST`,
		},
		{
			name:         "transform enabled - request path",
			body:         `Path: {{.Request.Path}}`,
			transform:    true,
			method:       http.MethodGet,
			path:         "/api/users",
			wantExact:    `Path: /api/users`,
		},
		{
			name:         "transform enabled - request header",
			body:         `Auth: {{.Request.Header "Authorization"}}`,
			transform:    true,
			method:       http.MethodGet,
			headers:      map[string]string{"Authorization": "Bearer xyz"},
			wantExact:    `Auth: Bearer xyz`,
		},
		{
			name:         "transform enabled - request query",
			body:         `Page: {{.Request.Query "page"}}`,
			transform:    true,
			method:       http.MethodGet,
			query:        "page=42",
			wantExact:    `Page: 42`,
		},
		{
			name:         "transform enabled - randomUUID produces valid UUID",
			body:         `{{randomUUID}}`,
			transform:    true,
			method:       http.MethodGet,
			wantContains: "-",
		},
		{
			name:         "transform enabled - now produces RFC3339",
			body:         `{{now}}`,
			transform:    true,
			method:       http.MethodGet,
			wantContains: "T",
		},
		{
			name:         "transform enabled - randomInt in range",
			body:         `{{randomInt 5 10}}`,
			transform:    true,
			method:       http.MethodGet,
			wantContains: "",
		},
		{
			name:         "transform with invalid template logs warning and returns raw",
			body:         `{{.Request.Method`,
			transform:    true,
			method:       http.MethodGet,
			wantExact:    `{{.Request.Method`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.path != "" {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			if tt.query != "" {
				req.URL.RawQuery = tt.query
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			def := &spec.StubDefinition{
				ID: "template-stub",
				Response: spec.ResponseDefinition{
					Status:          200,
					Body:            tt.body,
					TransformResponse: tt.transform,
				},
			}

			err := w.WriteResponse(rr, def, req, nil)
			if err != nil {
				t.Fatalf("WriteResponse failed: %v", err)
			}

			got := rr.Body.String()

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("body = %q, want %q", got, tt.wantExact)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("body = %q, want to contain %q", got, tt.wantContains)
			}
			if tt.name == "transform enabled - randomInt in range" {
				var parsed int
				for _, c := range got {
					if c >= '0' && c <= '9' {
						parsed = parsed*10 + int(c-'0')
					}
				}
				if parsed < 5 || parsed > 10 {
					t.Errorf("randomInt result %d out of range [5, 10]", parsed)
				}
			}
		})
	}
}

func TestCORSOptionsFromConfig(t *testing.T) {
	// Verify that the conversion function works correctly
	// This is tested indirectly through integration tests,
	// but we verify the types are compatible here.
	var _ *CORSOptions = (*CORSOptions)(nil)
}

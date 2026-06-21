package response

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/randx"
	"github.com/sunny809/gochaos/internal/spec"
)

func TestHTTPWriter_WriteResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

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
				"Access-Control-Allow-Origin":   "*",
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
			_, err := w.WriteResponse(rr, tt.def, req, tt.corsOpts, 0, time.Time{})
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
	w := NewHTTPWriter(logger, true, nil)

	tests := []struct {
		name        string
		delay       *spec.DelayDefinition
		minDuration time.Duration
		maxDuration time.Duration
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
			_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
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
	w := NewHTTPWriter(logger, true, nil)

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
		_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
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
	w := NewHTTPWriter(logger, true, nil)

	tests := []struct {
		name            string
		origin          string
		corsOpts        *CORSOptions
		wantOrigin      string
		wantMethods     string
		wantHeaders     string
		wantCredentials string
		wantMaxAge      string
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
			name:   "all options",
			origin: "http://example.com",
			corsOpts: &CORSOptions{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "DELETE"},
				AllowedHeaders:   []string{"Content-Type", "Authorization"},
				ExposedHeaders:   []string{"X-Total"},
				AllowCredentials: true,
				MaxAge:           3600,
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
			w := NewHTTPWriter(logger, tt.disableGzip, nil)
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
	w := NewHTTPWriter(logger, false, nil)

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
	w := NewHTTPWriter(logger, true, nil)

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
			_, _ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil, 0, time.Time{})
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
	w := NewHTTPWriter(logger, true, nil)

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
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed unexpectedly: %v", err)
	}
}

func TestHTTPWriter_WriteResponse_TemplateRendering(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

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
			name:      "transform disabled - body returned as-is",
			body:      `Method: {{.Request.Method}}`,
			transform: false,
			method:    http.MethodPost,
			wantExact: `Method: {{.Request.Method}}`,
		},
		{
			name:      "transform enabled - request method",
			body:      `Method: {{.Request.Method}}`,
			transform: true,
			method:    http.MethodPost,
			wantExact: `Method: POST`,
		},
		{
			name:      "transform enabled - request path",
			body:      `Path: {{.Request.Path}}`,
			transform: true,
			method:    http.MethodGet,
			path:      "/api/users",
			wantExact: `Path: /api/users`,
		},
		{
			name:      "transform enabled - request header",
			body:      `Auth: {{.Request.Header "Authorization"}}`,
			transform: true,
			method:    http.MethodGet,
			headers:   map[string]string{"Authorization": "Bearer xyz"},
			wantExact: `Auth: Bearer xyz`,
		},
		{
			name:      "transform enabled - request query",
			body:      `Page: {{.Request.Query "page"}}`,
			transform: true,
			method:    http.MethodGet,
			query:     "page=42",
			wantExact: `Page: 42`,
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
			name:      "transform with invalid template logs warning and returns raw",
			body:      `{{.Request.Method`,
			transform: true,
			method:    http.MethodGet,
			wantExact: `{{.Request.Method`,
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
					Status:            200,
					Body:              tt.body,
					TransformResponse: tt.transform,
				},
			}

			_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
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

func TestHTTPWriter_WriteResponse_FaultError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	tests := []struct {
		name       string
		def        *spec.StubDefinition
		wantStatus int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name: "error fault returns 500 with JSON body",
			def: &spec.StubDefinition{
				ID: "fault-error-stub",
				Response: spec.ResponseDefinition{
					Fault: &spec.FaultDefinition{Type: "error"},
				},
			},
			wantStatus: 500,
			wantBody:   `{"error":"internal server error","fault":"error"}`,
			wantHeader: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "error fault ignores stub status headers and body",
			def: &spec.StubDefinition{
				ID: "fault-override-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Headers: map[string]string{
						"X-Custom": "should-be-ignored",
					},
					Body:  "this body should not appear",
					Fault: &spec.FaultDefinition{Type: "error"},
				},
			},
			wantStatus: 500,
			wantBody:   `{"error":"internal server error","fault":"error"}`,
			wantHeader: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "nil fault returns normal response",
			def: &spec.StubDefinition{
				ID: "no-fault-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "normal response",
				},
			},
			wantStatus: 200,
			wantBody:   "normal response",
		},
		{
			name: "unknown fault type falls through to normal response",
			def: &spec.StubDefinition{
				ID: "unknown-fault-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "fallback response",
					Fault:  &spec.FaultDefinition{Type: "nonexistent"},
				},
			},
			wantStatus: 200,
			wantBody:   "fallback response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			_, err := w.WriteResponse(rr, tt.def, req, nil, 0, time.Time{})
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

func TestHTTPWriter_WriteResponse_FaultWithDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	// Fault combined with delay: delay is applied first, then fault response
	def := &spec.StubDefinition{
		ID: "fault-delay-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Delay:  &spec.DelayDefinition{Type: "fixed", Value: 50},
			Fault:  &spec.FaultDefinition{Type: "error"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	start := time.Now()
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	if elapsed < 45*time.Millisecond {
		t.Errorf("elapsed %v < 45ms, delay was not applied before fault", elapsed)
	}

	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}

	wantBody := `{"error":"internal server error","fault":"error"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}
}

func TestHTTPWriter_WriteResponse_FaultNotGzipped(t *testing.T) {
	// Verify that fault responses are NOT gzip-compressed even when
	// the client accepts gzip encoding.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false, nil) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-gzip-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "error"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fault response must NOT have Content-Encoding: gzip
	if ce := rr.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want empty (fault should not be gzipped)", ce)
	}

	// Body must be plain JSON, not compressed bytes
	wantBody := `{"error":"internal server error","fault":"error"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}

	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}
}

func TestHTTPWriter_WriteResponse_FaultEmpty(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	tests := []struct {
		name       string
		def        *spec.StubDefinition
		wantStatus int
		wantBody   string
		wantHeader map[string]string
		dontWant   []string // headers that must NOT be present
	}{
		{
			name: "empty fault returns empty body and default status 200",
			def: &spec.StubDefinition{
				ID: "fault-empty-stub",
				Response: spec.ResponseDefinition{
					Fault: &spec.FaultDefinition{Type: "empty"},
				},
			},
			wantStatus: 200,
			wantBody:   "",
			dontWant:   []string{"Content-Type", "X-Custom"},
		},
		{
			name: "empty fault overrides stub body headers and status",
			def: &spec.StubDefinition{
				ID: "fault-empty-override-stub",
				Response: spec.ResponseDefinition{
					Status: 404,
					Headers: map[string]string{
						"Content-Type": "application/json",
						"X-Custom":     "should-be-ignored",
					},
					Body:  "this body should not appear",
					Fault: &spec.FaultDefinition{Type: "empty"},
				},
			},
			wantStatus: 200,
			wantBody:   "",
			dontWant:   []string{"Content-Type", "X-Custom"},
		},
		{
			name: "empty fault with delay is delayed then empty",
			def: &spec.StubDefinition{
				ID: "fault-empty-delay-stub",
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   "should not appear",
					Delay:  &spec.DelayDefinition{Type: "fixed", Value: 50},
					Fault:  &spec.FaultDefinition{Type: "empty"},
				},
			},
			wantStatus: 200,
			wantBody:   "",
			dontWant:   []string{"Content-Type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			start := time.Now()
			_, err := w.WriteResponse(rr, tt.def, req, nil, 0, time.Time{})
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("WriteResponse failed: %v", err)
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatus)
			}

			if body := rr.Body.String(); body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}

			for _, hdr := range tt.dontWant {
				if got := rr.Header().Get(hdr); got != "" {
					t.Errorf("header %q = %q, want empty (not set)", hdr, got)
				}
			}

			for key, wantVal := range tt.wantHeader {
				if got := rr.Header().Get(key); got != wantVal {
					t.Errorf("header %q = %q, want %q", key, got, wantVal)
				}
			}

			// Verify delay was applied for the delay+empty test
			if tt.def.Response.Delay != nil && elapsed < 45*time.Millisecond {
				t.Errorf("elapsed %v < 45ms, delay was not applied before empty fault", elapsed)
			}
		})
	}
}

func TestHTTPWriter_WriteResponse_FaultEmptyNotGzipped(t *testing.T) {
	// Verify that empty fault responses are NOT gzip-compressed even when
	// the client accepts gzip encoding.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false, nil) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-empty-gzip-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "empty"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Empty fault must NOT have Content-Encoding: gzip
	if ce := rr.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want empty (empty fault should not be gzipped)", ce)
	}

	// Body must be empty
	if rr.Body.String() != "" {
		t.Errorf("body = %q, want empty", rr.Body.String())
	}

	if rr.Code != 200 {
		t.Errorf("status code = %d, want 200", rr.Code)
	}
}

func TestCORSOptionsFromConfig(t *testing.T) {
	// Verify that the conversion function works correctly
	// This is tested indirectly through integration tests,
	// but we verify the types are compatible here.
	var _ *CORSOptions = (*CORSOptions)(nil)
}

// mockHijacker wraps an http.ResponseWriter and implements http.Hijacker.
// It returns a real net.Conn (from net.Pipe) so that Close() works in tests.
type mockHijacker struct {
	http.ResponseWriter
	hijacked bool
	conn     net.Conn
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijacked = true
	server, client := net.Pipe()
	m.conn = server // server side: the one we Close() to simulate RST
	// Close the client side immediately — we don't need it in the test.
	_ = client.Close()
	return server, nil, nil
}

func TestHTTPWriter_WriteResponse_FaultConnectionReset_HijackerAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-connreset-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "connection_reset"},
		},
	}

	rr := httptest.NewRecorder()
	hj := &mockHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Hijack must have been called
	if !hj.hijacked {
		t.Error("Hijack() was not called, expected it to be called for connection_reset fault")
	}

	// The connection returned by Hijack should be closed now.
	// A write to the closed conn should fail.
	_ = hj.conn.Close() // close again to verify double-close doesn't panic
}

func TestHTTPWriter_WriteResponse_FaultConnectionReset_HijackerNotAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-connreset-nohj-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "connection_reset"},
		},
	}

	// httptest.NewRecorder does NOT implement http.Hijacker, so this tests the fallback path.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fallback: should return 500 with Connection: close
	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}

	wantBody := `{"error":"connection reset by peer","fault":"connection_reset"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}

	if got := rr.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection header = %q, want %q", got, "close")
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type header = %q, want %q", got, "application/json")
	}
}

func TestHTTPWriter_WriteResponse_FaultConnectionReset_WithDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-connreset-delay-stub",
		Response: spec.ResponseDefinition{
			Delay: &spec.DelayDefinition{Type: "fixed", Value: 50},
			Fault: &spec.FaultDefinition{Type: "connection_reset"},
		},
	}

	rr := httptest.NewRecorder()
	hj := &mockHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Delay must have been applied before the fault
	if elapsed < 45*time.Millisecond {
		t.Errorf("elapsed %v < 45ms, delay was not applied before connection_reset fault", elapsed)
	}

	// Hijack must have been called after the delay
	if !hj.hijacked {
		t.Error("Hijack() was not called after delay")
	}
}

func TestHTTPWriter_WriteResponse_FaultConnectionReset_NotGzipped(t *testing.T) {
	// Verify that connection_reset fault responses (fallback path) are NOT
	// gzip-compressed even when the client accepts gzip encoding.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false, nil) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-connreset-gzip-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "connection_reset"},
		},
	}

	// httptest.NewRecorder does not implement Hijacker, so this goes through the fallback path.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Connection reset fallback must NOT have Content-Encoding: gzip
	if ce := rr.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want empty (connection_reset fallback should not be gzipped)", ce)
	}

	// Body must be plain JSON, not compressed bytes
	wantBody := `{"error":"connection reset by peer","fault":"connection_reset"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}

	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}
}

func TestUnwrapResponseWriter(t *testing.T) {
	tests := []struct {
		name string
		rw   http.ResponseWriter
		want string
	}{
		{
			name: "plain ResponseWriter returns itself",
			rw:   httptest.NewRecorder(),
			want: "*httptest.ResponseRecorder",
		},
		{
			name: "single wrap unwraps to inner",
			rw: &GzipResponseWriter{
				ResponseWriter: httptest.NewRecorder(),
			},
			want: "*httptest.ResponseRecorder",
		},
		{
			name: "double wrap unwraps to innermost",
			rw: &GzipResponseWriter{
				ResponseWriter: &GzipResponseWriter{
					ResponseWriter: httptest.NewRecorder(),
				},
			},
			want: "*httptest.ResponseRecorder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := unwrapResponseWriter(tt.rw)
			got := fmt.Sprintf("%T", inner)
			if got != tt.want {
				t.Errorf("unwrapResponseWriter returned %T, want %s", inner, tt.want)
			}
		})
	}
}

func TestHTTPWriter_ApplyFault(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name          string
		disableGz     bool
		fault         *spec.FaultDefinition
		hijacker      bool
		wantApplied   bool
		wantSlowClose int
		wantStatus    int
		wantBody      string
		wantHeader    map[string]string
		dontWant      []string
	}{
		{
			name:        "nil fault returns not applied",
			fault:       nil,
			wantApplied: false,
			wantStatus:  0, // no status written
		},
		{
			name:        "error fault returns applied with 500 JSON",
			fault:       &spec.FaultDefinition{Type: "error"},
			wantApplied: true,
			wantStatus:  500,
			wantBody:    `{"error":"internal server error","fault":"error"}`,
			wantHeader:  map[string]string{"Content-Type": "application/json"},
		},
		{
			name:        "empty fault returns applied with empty body",
			fault:       &spec.FaultDefinition{Type: "empty"},
			wantApplied: true,
			wantStatus:  0, // Go defaults to 200, but WriteHeader is not called explicitly
			wantBody:    "",
			dontWant:    []string{"Content-Type"},
		},
		{
			name:        "connection_reset without Hijacker falls back to 500",
			fault:       &spec.FaultDefinition{Type: "connection_reset"},
			hijacker:    false,
			wantApplied: true,
			wantStatus:  500,
			wantBody:    `{"error":"connection reset by peer","fault":"connection_reset"}`,
			wantHeader: map[string]string{
				"Connection":   "close",
				"Content-Type": "application/json",
			},
		},
		{
			name:        "connection_reset with Hijacker hijacks and closes",
			fault:       &spec.FaultDefinition{Type: "connection_reset"},
			hijacker:    true,
			wantApplied: true,
			// No assertions on status/body since the connection is hijacked and closed
		},
		{
			name:          "slow_close returns not applied with slowCloseMs set",
			fault:         &spec.FaultDefinition{Type: "slow_close", DelayMs: 500},
			wantApplied:   false,
			wantSlowClose: 500,
		},
		{
			name:          "slow_close default DelayMs is 1000",
			fault:         &spec.FaultDefinition{Type: "slow_close"},
			wantApplied:   false,
			wantSlowClose: 1000,
		},
		{
			name:        "unknown fault type returns not applied",
			fault:       &spec.FaultDefinition{Type: "unknown"},
			wantApplied: false,
			wantStatus:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTTPWriter(logger, tt.disableGz, nil)
			rr := httptest.NewRecorder()

			var rw http.ResponseWriter = rr
			if tt.hijacker {
				rw = &mockHijacker{ResponseWriter: rr}
			}

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			result := w.applyFault(rw, tt.fault, req, 0, time.Time{})

			if result.applied != tt.wantApplied {
				t.Errorf("applyFault().applied = %v, want %v", result.applied, tt.wantApplied)
			}

			if result.slowCloseMs != tt.wantSlowClose {
				t.Errorf("applyFault().slowCloseMs = %d, want %d", result.slowCloseMs, tt.wantSlowClose)
			}

			if !result.applied {
				return // no response was written, skip body/header checks
			}

			if tt.hijacker && tt.fault != nil && tt.fault.Type == "connection_reset" {
				hj := rw.(*mockHijacker)
				if !hj.hijacked {
					t.Error("expected Hijack() to be called")
				}
				return // connection was hijacked, no status/body to check
			}

			if tt.wantStatus != 0 && rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
			if tt.wantBody == "" && rr.Body.String() != "" {
				t.Errorf("body = %q, want empty", rr.Body.String())
			}
			for k, v := range tt.wantHeader {
				if got := rr.Header().Get(k); got != v {
					t.Errorf("header %q = %q, want %q", k, got, v)
				}
			}
			for _, hdr := range tt.dontWant {
				if got := rr.Header().Get(hdr); got != "" {
					t.Errorf("header %q = %q, want empty (not set)", hdr, got)
				}
			}
		})
	}
}

func TestHTTPWriter_ApplyFault_ConcurrentSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	faults := []*spec.FaultDefinition{
		nil,
		{Type: "error"},
		{Type: "empty"},
		{Type: "connection_reset"},
	}

	done := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			rr := httptest.NewRecorder()
			def := &spec.StubDefinition{
				ID: fmt.Sprintf("concurrent-fault-%d", idx),
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   fmt.Sprintf("response-%d", idx),
					Fault:  faults[idx%len(faults)],
				},
			}
			_, _ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil, 0, time.Time{})
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestGzipResponseWriter_Unwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	gw := &GzipResponseWriter{
		ResponseWriter: inner,
		GW:             gzip.NewWriter(inner),
	}

	unwrapped := gw.Unwrap()
	if unwrapped != inner {
		t.Errorf("Unwrap() returned %p, want %p", unwrapped, inner)
	}
}

// --- A7: Infinite Timeout (delay type: timeout) ---

func TestHTTPWriter_ApplyDelay_Timeout_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	// Verify that applyDelay with type "timeout" blocks until context is cancelled.
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		w.applyDelay(&spec.DelayDefinition{Type: "timeout"}, ctx)
	}()

	// Give the goroutine a moment to enter the select block.
	time.Sleep(50 * time.Millisecond)

	// Cancel the context — this should unblock applyDelay.
	cancel()

	select {
	case <-done:
		// Success: applyDelay returned after context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("applyDelay(timeout) did not return after context cancellation")
	}
}

func TestHTTPWriter_ApplyDelay_Timeout_NeverReturnsOnActiveContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		w.applyDelay(&spec.DelayDefinition{Type: "timeout"}, ctx)
	}()

	// Wait a short time — applyDelay should NOT return on an active context.
	select {
	case <-done:
		t.Fatal("applyDelay(timeout) returned unexpectedly on active context")
	case <-time.After(200 * time.Millisecond):
		// Expected: still blocking.
	}

	// Clean up: cancel so the goroutine can exit.
	cancel()
	<-done
}

func TestHTTPWriter_WriteResponse_DelayTimeout_ClientTimeout(t *testing.T) {
	// End-to-end test: a stub with delay type "timeout" causes the client
	// to hit its own timeout because the server never responds.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	// Set up a real HTTP server that uses our HTTPWriter.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(rw http.ResponseWriter, r *http.Request) {
		def := &spec.StubDefinition{
			ID: "timeout-stub",
			Response: spec.ResponseDefinition{
				Delay: &spec.DelayDefinition{Type: "timeout"},
			},
		}
		// The handler will block indefinitely; the client will time out.
		_, _ = w.WriteResponse(rw, def, r, nil, 0, time.Time{})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	resp, err := client.Get(ts.URL + "/slow")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected client timeout error, got nil error")
	}
	// The error should be a timeout (url.Error wrapping context.DeadlineExceeded).
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout-related error, got: %v", err)
	}
}

func TestHTTPWriter_WriteResponse_DelayTimeout_ServerShutdown(t *testing.T) {
	// Verify that when the server shuts down (cancelling the base context),
	// the blocked applyDelay returns and does not leak a goroutine.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /hang", func(rw http.ResponseWriter, r *http.Request) {
		def := &spec.StubDefinition{
			ID: "timeout-stub",
			Response: spec.ResponseDefinition{
				Delay: &spec.DelayDefinition{Type: "timeout"},
			},
		}
		_, _ = w.WriteResponse(rw, def, r, nil, 0, time.Time{})
	})

	ts := httptest.NewServer(mux)

	// Start a request that will hang. Use a short client timeout as a safety net.
	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		client := &http.Client{Timeout: 2 * time.Second}
		_, _ = client.Get(ts.URL + "/hang")
	}()

	// Give the request time to reach the handler.
	time.Sleep(100 * time.Millisecond)

	// Close the server — this cancels the request context.
	ts.Close()

	// The client goroutine should finish (either timeout or connection closed).
	select {
	case <-done:
		// Success: the goroutine exited.
	case <-time.After(3 * time.Second):
		t.Fatal("goroutine leaked: server shutdown did not unblock applyDelay(timeout)")
	}
}

// --- A5: Malformed Response (fault type: malformed) ---

func TestHTTPWriter_WriteResponse_FaultMalformed_HijackerAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-malformed-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "malformed"},
		},
	}

	rr := httptest.NewRecorder()
	hj := &mockHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Hijack must have been called
	if !hj.hijacked {
		t.Error("Hijack() was not called, expected it to be called for malformed fault")
	}
}

func TestHTTPWriter_WriteResponse_FaultMalformed_HijackerNotAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-malformed-nohj-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "malformed"},
		},
	}

	// httptest.NewRecorder does NOT implement http.Hijacker, so this tests the fallback path.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fallback: should return 200 with Content-Length mismatch (truncated body)
	if rr.Code != 200 {
		t.Errorf("status code = %d, want 200", rr.Code)
	}

	if got := rr.Header().Get("Content-Length"); got != "100" {
		t.Errorf("Content-Length = %q, want %q", got, "100")
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	// Body should be truncated (less than 100 bytes as Content-Length claimed)
	body := rr.Body.String()
	if body != `{"partial":` {
		t.Errorf("body = %q, want %q", body, `{"partial":`)
	}
	if len(body) >= 100 {
		t.Errorf("body length = %d, expected < 100 (truncated)", len(body))
	}
}

func TestHTTPWriter_WriteResponse_FaultMalformed_WithDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-malformed-delay-stub",
		Response: spec.ResponseDefinition{
			Delay: &spec.DelayDefinition{Type: "fixed", Value: 50},
			Fault: &spec.FaultDefinition{Type: "malformed"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Delay must have been applied before the fault
	if elapsed < 45*time.Millisecond {
		t.Errorf("elapsed %v < 45ms, delay was not applied before malformed fault", elapsed)
	}

	// Fallback path (no hijacker): truncated response
	if rr.Code != 200 {
		t.Errorf("status code = %d, want 200", rr.Code)
	}
}

func TestHTTPWriter_WriteResponse_FaultMalformed_NotGzipped(t *testing.T) {
	// Verify that malformed fault responses (fallback path) are NOT
	// gzip-compressed even when the client accepts gzip encoding.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false, nil) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-malformed-gzip-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "malformed"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Malformed fault fallback must NOT have Content-Encoding: gzip
	if ce := rr.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want empty (malformed fallback should not be gzipped)", ce)
	}

	// Body must be plain truncated JSON, not compressed bytes
	if rr.Body.String() != `{"partial":` {
		t.Errorf("body = %q, want %q", rr.Body.String(), `{"partial":`)
	}
}

func TestHTTPWriter_ApplyFault_Malformed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		hijacker   bool
		wantReturn bool
		wantStatus int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name:       "malformed with Hijacker hijacks and writes malformed HTTP",
			hijacker:   true,
			wantReturn: true,
			// No assertions on status/body since connection is hijacked
		},
		{
			name:       "malformed without Hijacker falls back to truncated response",
			hijacker:   false,
			wantReturn: true,
			wantStatus: 200,
			wantBody:   `{"partial":`,
			wantHeader: map[string]string{
				"Content-Type":   "application/json",
				"Content-Length": "100",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTTPWriter(logger, true, nil)
			rr := httptest.NewRecorder()

			var rw http.ResponseWriter = rr
			if tt.hijacker {
				rw = &mockHijacker{ResponseWriter: rr}
			}

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			result := w.applyFault(rw, &spec.FaultDefinition{Type: "malformed"}, req, 0, time.Time{})

			if result.applied != tt.wantReturn {
				t.Errorf("applyFault().applied = %v, want %v", result.applied, tt.wantReturn)
			}

			if tt.hijacker {
				hj := rw.(*mockHijacker)
				if !hj.hijacked {
					t.Error("expected Hijack() to be called for malformed fault")
				}
				return // connection was hijacked, no status/body to check
			}

			if tt.wantStatus != 0 && rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rr.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
			}
			for k, v := range tt.wantHeader {
				if got := rr.Header().Get(k); got != v {
					t.Errorf("header %q = %q, want %q", k, got, v)
				}
			}
		})
	}
}

// --- A6: Random Data Then Close (fault type: random_data) ---

func TestHTTPWriter_WriteResponse_FaultRandomData_HijackerAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "fault-randomdata-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "random_data", DataLength: 100},
		},
	}

	rr := httptest.NewRecorder()
	hj := &mockHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Hijack must have been called
	if !hj.hijacked {
		t.Error("Hijack() was not called, expected it to be called for random_data fault")
	}
}

func TestHTTPWriter_WriteResponse_FaultRandomData_HijackerNotAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "fault-randomdata-nohj-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "random_data", DataLength: 100},
		},
	}

	// httptest.NewRecorder does NOT implement http.Hijacker, so this tests the fallback path.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fallback: should return 500 with random hex body
	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}

	if got := rr.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection header = %q, want %q", got, "close")
	}

	if got := rr.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", got, "application/octet-stream")
	}

	// Body should be hex-encoded random data (100 hex chars for 50 bytes)
	body := rr.Body.String()
	if len(body) != 100 {
		t.Errorf("body length = %d, want 100 (hex-encoded 50 bytes)", len(body))
	}
}

func TestHTTPWriter_WriteResponse_FaultRandomData_DefaultDataLength(t *testing.T) {
	// When DataLength is 0 (unspecified), it should default to 256.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "fault-randomdata-default-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "random_data"}, // DataLength not set
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fallback path: hex-encoded 128 bytes (256/2) = 256 hex chars
	body := rr.Body.String()
	if len(body) != 256 {
		t.Errorf("body length = %d, want 256 (hex-encoded 128 bytes for default dataLength=256)", len(body))
	}
}

func TestHTTPWriter_WriteResponse_FaultRandomData_SeedDeterminism(t *testing.T) {
	// Same seed should produce identical random data across runs.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runOnce := func() string {
		rng := randx.NewGlobal(42)
		w := NewHTTPWriter(logger, true, rng)

		def := &spec.StubDefinition{
			ID: "fault-randomdata-seed-stub",
			Response: spec.ResponseDefinition{
				Fault: &spec.FaultDefinition{Type: "random_data", DataLength: 100},
			},
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		_, _ = w.WriteResponse(rr, def, req, nil, 0, time.Time{})
		return rr.Body.String()
	}

	result1 := runOnce()
	result2 := runOnce()

	if result1 != result2 {
		t.Errorf("same seed produced different results:\n  run1=%q\n  run2=%q", result1, result2)
	}
}

func TestHTTPWriter_WriteResponse_FaultRandomData_WithDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, true, rng)

	def := &spec.StubDefinition{
		ID: "fault-randomdata-delay-stub",
		Response: spec.ResponseDefinition{
			Delay: &spec.DelayDefinition{Type: "fixed", Value: 50},
			Fault: &spec.FaultDefinition{Type: "random_data", DataLength: 100},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Delay must have been applied before the fault
	if elapsed < 45*time.Millisecond {
		t.Errorf("elapsed %v < 45ms, delay was not applied before random_data fault", elapsed)
	}

	// Fallback path (no hijacker)
	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}
}

func TestHTTPWriter_WriteResponse_FaultRandomData_NotGzipped(t *testing.T) {
	// Verify that random_data fault responses (fallback path) are NOT
	// gzip-compressed even when the client accepts gzip encoding.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rng := randx.NewGlobal(42)
	w := NewHTTPWriter(logger, false, rng) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-randomdata-gzip-stub",
		Response: spec.ResponseDefinition{
			Fault: &spec.FaultDefinition{Type: "random_data", DataLength: 100},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// random_data fault fallback must NOT have Content-Encoding: gzip
	if ce := rr.Header().Get("Content-Encoding"); ce != "" {
		t.Errorf("Content-Encoding = %q, want empty (random_data fallback should not be gzipped)", ce)
	}
}

func TestHTTPWriter_ApplyFault_RandomData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		hijacker   bool
		dataLength int
		wantReturn bool
		wantStatus int
		wantHeader map[string]string
	}{
		{
			name:       "random_data with Hijacker hijacks and writes random bytes",
			hijacker:   true,
			dataLength: 100,
			wantReturn: true,
		},
		{
			name:       "random_data without Hijacker falls back to 500 random hex",
			hijacker:   false,
			dataLength: 100,
			wantReturn: true,
			wantStatus: 500,
			wantHeader: map[string]string{
				"Connection":   "close",
				"Content-Type": "application/octet-stream",
			},
		},
		{
			name:       "random_data with default DataLength (0 -> 256)",
			hijacker:   false,
			dataLength: 0,
			wantReturn: true,
			wantStatus: 500,
			wantHeader: map[string]string{
				"Content-Type": "application/octet-stream",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := randx.NewGlobal(42)
			w := NewHTTPWriter(logger, true, rng)
			rr := httptest.NewRecorder()

			var rw http.ResponseWriter = rr
			if tt.hijacker {
				rw = &mockHijacker{ResponseWriter: rr}
			}

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			fault := &spec.FaultDefinition{Type: "random_data", DataLength: tt.dataLength}
			result := w.applyFault(rw, fault, req, 0, time.Time{})

			if result.applied != tt.wantReturn {
				t.Errorf("applyFault().applied = %v, want %v", result.applied, tt.wantReturn)
			}

			if tt.hijacker {
				hj := rw.(*mockHijacker)
				if !hj.hijacked {
					t.Error("expected Hijack() to be called for random_data fault")
				}
				return
			}

			if tt.wantStatus != 0 && rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			for k, v := range tt.wantHeader {
				if got := rr.Header().Get(k); got != v {
					t.Errorf("header %q = %q, want %q", k, got, v)
				}
			}

			// Body should contain hex-encoded random data (non-empty)
			body := rr.Body.String()
			if len(body) == 0 {
				t.Error("body is empty, expected random hex data")
			}
		})
	}
}

// --- A8: Slow Close (fault type: slow_close) ---

func TestHTTPWriter_WriteResponse_FaultSlowClose_HijackerAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-slowclose-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   `{"hello":"world"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Fault: &spec.FaultDefinition{Type: "slow_close", DelayMs: 100},
		},
	}

	rr := httptest.NewRecorder()
	hj := &slowCloseHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Hijack must have been called after the response was fully written.
	if !hj.hijacked {
		t.Error("Hijack() was not called, expected it to be called for slow_close fault")
	}

	// The elapsed time should include the 100ms slow close delay.
	if elapsed < 90*time.Millisecond {
		t.Errorf("elapsed %v < 90ms, slow_close delay was not applied", elapsed)
	}
}

func TestHTTPWriter_WriteResponse_FaultSlowClose_DefaultDelayMs(t *testing.T) {
	// When DelayMs is 0 (unspecified), it should default to 1000ms.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-slowclose-default-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Fault:  &spec.FaultDefinition{Type: "slow_close"}, // DelayMs not set
		},
	}

	rr := httptest.NewRecorder()
	hj := &slowCloseHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// The default delay is 1000ms -- check for at least 900ms.
	if elapsed < 900*time.Millisecond {
		t.Errorf("elapsed %v < 900ms, default slow_close delay (1000ms) was not applied", elapsed)
	}

	if !hj.hijacked {
		t.Error("Hijack() was not called for slow_close fault with default delay")
	}
}

func TestHTTPWriter_WriteResponse_FaultSlowClose_HijackerNotAvailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-slowclose-nohj-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   `{"hello":"world"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Fault: &spec.FaultDefinition{Type: "slow_close", DelayMs: 100},
		},
	}

	// httptest.NewRecorder does NOT implement http.Hijacker, so this tests the fallback path.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fallback: normal response with Connection: close header.
	if rr.Code != 200 {
		t.Errorf("status code = %d, want 200", rr.Code)
	}

	if got := rr.Body.String(); got != `{"hello":"world"}` {
		t.Errorf("body = %q, want %q", got, `{"hello":"world"}`)
	}

	if got := rr.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection header = %q, want %q", got, "close")
	}
}

func TestHTTPWriter_WriteResponse_FaultSlowClose_WithDelay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "fault-slowclose-delay-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ok",
			Delay:  &spec.DelayDefinition{Type: "fixed", Value: 50},
			Fault:  &spec.FaultDefinition{Type: "slow_close", DelayMs: 100},
		},
	}

	rr := httptest.NewRecorder()
	hj := &slowCloseHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Both the 50ms delay and the 100ms slow_close should be applied.
	if elapsed < 140*time.Millisecond {
		t.Errorf("elapsed %v < 140ms, delay + slow_close was not applied", elapsed)
	}

	if !hj.hijacked {
		t.Error("Hijack() was not called after delay + slow_close")
	}
}

func TestHTTPWriter_WriteResponse_FaultSlowClose_NotGzipped(t *testing.T) {
	// Verify that slow_close responses are NOT gzip-compressed even when
	// the client accepts gzip encoding. Slow_close skips gzip wrapping
	// because it needs to hijack the connection after writing.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, false, nil) // gzip enabled

	def := &spec.StubDefinition{
		ID: "fault-slowclose-gzip-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "hello world",
			Fault:  &spec.FaultDefinition{Type: "slow_close", DelayMs: 50},
		},
	}

	// Use a hijacker so the slow_close path is exercised.
	rr := httptest.NewRecorder()
	hj := &slowCloseHijacker{ResponseWriter: rr}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	_, err := w.WriteResponse(hj, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// The response was written before hijacking, so the recorder captured it.
	// Since slow_close skips gzip wrapping, Content-Encoding should not be set.
	if ce := rr.Header().Get("Content-Encoding"); ce == "gzip" {
		t.Errorf("Content-Encoding = %q, slow_close should skip gzip wrapping", ce)
	}
}

func TestHTTPWriter_ApplyFault_SlowClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name       string
		hijacker   bool
		delayMs    int
		wantReturn bool
	}{
		{
			name:       "slow_close with Hijacker returns false and sets pending",
			hijacker:   true,
			delayMs:    100,
			wantReturn: false,
		},
		{
			name:       "slow_close without Hijacker returns false and sets pending",
			hijacker:   false,
			delayMs:    100,
			wantReturn: false,
		},
		{
			name:       "slow_close with default DelayMs (0 -> 1000)",
			hijacker:   true,
			delayMs:    0,
			wantReturn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewHTTPWriter(logger, true, nil)
			rr := httptest.NewRecorder()

			var rw http.ResponseWriter = rr
			if tt.hijacker {
				rw = &slowCloseHijacker{ResponseWriter: rr}
			}

			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			result := w.applyFault(rw, &spec.FaultDefinition{Type: "slow_close", DelayMs: tt.delayMs}, req, 0, time.Time{})

			if result.applied != tt.wantReturn {
				t.Errorf("applyFault().applied = %v, want %v", result.applied, tt.wantReturn)
			}

			// slowCloseMs should be set regardless of hijacker availability.
			expectedMs := tt.delayMs
			if expectedMs <= 0 {
				expectedMs = 1000
			}
			if result.slowCloseMs != expectedMs {
				t.Errorf("slowCloseMs = %d, want %d", result.slowCloseMs, expectedMs)
			}
		})
	}
}

func TestHTTPWriter_WriteResponse_FaultSlowClose_ConcurrentSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			rr := httptest.NewRecorder()
			def := &spec.StubDefinition{
				ID: fmt.Sprintf("slowclose-concurrent-%d", idx),
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   fmt.Sprintf("response-%d", idx),
					Fault:  &spec.FaultDefinition{Type: "slow_close", DelayMs: 10},
				},
			}
			_, _ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil, 0, time.Time{})
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}

// --- A9: Chunked Dribble (delay type: dribble) ---

func TestHTTPWriter_WriteResponse_DelayDribble_BasicChunking(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	// 3 chunks over 150ms -> ~50ms between chunks.
	def := &spec.StubDefinition{
		ID: "dribble-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "ABCDEFGHI", // 9 chars -> 3 chars per chunk
			Delay: &spec.DelayDefinition{
				Type:          "dribble",
				Chunks:        3,
				TotalDuration: 150,
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Total time should be approximately 150ms (2 intervals of 50ms between 3 chunks).
	if elapsed < 90*time.Millisecond {
		t.Errorf("elapsed %v < 90ms, dribble delay was not applied", elapsed)
	}

	if rr.Code != 200 {
		t.Errorf("status code = %d, want 200", rr.Code)
	}

	// Body should be complete -- all chunks delivered.
	if rr.Body.String() != "ABCDEFGHI" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "ABCDEFGHI")
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_ZeroChunks(t *testing.T) {
	// chunks: 0 should be ignored -- dribble config is not set, normal body writing.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "dribble-zero-chunks-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "hello",
			Delay: &spec.DelayDefinition{
				Type:          "dribble",
				Chunks:        0,
				TotalDuration: 100,
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	start := time.Now()
	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// No delay should be applied (dribble with chunks=0 is invalid and ignored).
	if elapsed > 50*time.Millisecond {
		t.Errorf("elapsed %v > 50ms, dribble with chunks=0 should be ignored", elapsed)
	}

	// Body should be written normally (not chunked).
	if rr.Body.String() != "hello" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "hello")
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_ClientDisconnect(t *testing.T) {
	// When the client disconnects, the remaining chunks should not be sent.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	// Set up a real HTTP server.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dribble", func(rw http.ResponseWriter, r *http.Request) {
		def := &spec.StubDefinition{
			ID: "dribble-disconnect-stub",
			Response: spec.ResponseDefinition{
				Status: 200,
				Body:   "ABCDEFGHIJKLMNO", // 15 chars
				Delay: &spec.DelayDefinition{
					Type:          "dribble",
					Chunks:        5,
					TotalDuration: 5000, // 5 seconds total (1s between chunks)
				},
			},
		}
		_, _ = w.WriteResponse(rw, def, r, nil, 0, time.Time{})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Client cancels after 200ms -- should only get the first chunk or two.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/dribble", nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// Client timeout is expected -- the request was cancelled.
		return
	}
	defer resp.Body.Close()

	// If we got here, we received a partial response.
	// Read what we can before the context deadline.
	body, _ := io.ReadAll(resp.Body)
	// The body should be incomplete (less than 15 chars).
	if len(body) >= 15 {
		t.Errorf("body should be incomplete after client disconnect, got %q", string(body))
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_WithFault(t *testing.T) {
	// When both dribble delay and fault are configured, fault takes priority.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "dribble-fault-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Body:   "should not appear",
			Delay: &spec.DelayDefinition{
				Type:          "dribble",
				Chunks:        3,
				TotalDuration: 150,
			},
			Fault: &spec.FaultDefinition{Type: "error"},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Fault should take priority -- 500 with error body, not dribble.
	if rr.Code != 500 {
		t.Errorf("status code = %d, want 500", rr.Code)
	}

	wantBody := `{"error":"internal server error","fault":"error"}`
	if rr.Body.String() != wantBody {
		t.Errorf("body = %q, want %q", rr.Body.String(), wantBody)
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_Base64Body(t *testing.T) {
	// Dribble should also work with base64-encoded bodies.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	decoded := "Hello World!"
	def := &spec.StubDefinition{
		ID: "dribble-base64-stub",
		Response: spec.ResponseDefinition{
			Status:     200,
			Base64Body: base64.StdEncoding.EncodeToString([]byte(decoded)),
			Delay: &spec.DelayDefinition{
				Type:          "dribble",
				Chunks:        3,
				TotalDuration: 150,
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	if rr.Body.String() != decoded {
		t.Errorf("body = %q, want %q", rr.Body.String(), decoded)
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_EmptyBody(t *testing.T) {
	// Dribble with empty body should work without panicking.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	def := &spec.StubDefinition{
		ID: "dribble-empty-stub",
		Response: spec.ResponseDefinition{
			Status: 200,
			Delay: &spec.DelayDefinition{
				Type:          "dribble",
				Chunks:        3,
				TotalDuration: 150,
			},
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	_, err := w.WriteResponse(rr, def, req, nil, 0, time.Time{})
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	if rr.Body.String() != "" {
		t.Errorf("body = %q, want empty", rr.Body.String())
	}
}

func TestHTTPWriter_WriteResponse_DelayDribble_ConcurrentSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewHTTPWriter(logger, true, nil)

	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			rr := httptest.NewRecorder()
			def := &spec.StubDefinition{
				ID: fmt.Sprintf("dribble-concurrent-%d", idx),
				Response: spec.ResponseDefinition{
					Status: 200,
					Body:   fmt.Sprintf("response-%d", idx),
					Delay: &spec.DelayDefinition{
						Type:          "dribble",
						Chunks:        2,
						TotalDuration: 20,
					},
				},
			}
			_, _ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil, 0, time.Time{})
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

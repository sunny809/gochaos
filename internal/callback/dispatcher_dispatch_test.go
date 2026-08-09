package callback

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/callbacklog"
	"github.com/sunny809/gochaos/internal/spec"
)

// --- NewDispatcher ---

func TestNewDispatcher(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)

	t.Run("enabled dispatcher", func(t *testing.T) {
		d := NewDispatcher(logger, log, 5*time.Second, true)
		if d == nil {
			t.Fatal("NewDispatcher returned nil")
		}
		if !d.enabled {
			t.Error("expected enabled=true")
		}
		if d.defaultTimeout != 5*time.Second {
			t.Errorf("defaultTimeout = %v, want 5s", d.defaultTimeout)
		}
		if d.client == nil {
			t.Error("client should not be nil")
		}
	})

	t.Run("disabled dispatcher", func(t *testing.T) {
		d := NewDispatcher(logger, log, 1*time.Second, false)
		if d.enabled {
			t.Error("expected enabled=false")
		}
	})

	t.Run("does not follow redirects", func(t *testing.T) {
		d := NewDispatcher(logger, log, time.Second, true)
		// The CheckRedirect function should be set to not follow redirects.
		if d.client.CheckRedirect == nil {
			t.Fatal("CheckRedirect should be set")
		}
		// Verify it returns ErrUseLastResponse.
		err := d.client.CheckRedirect(nil, nil)
		if err != http.ErrUseLastResponse {
			t.Errorf("CheckRedirect returned %v, want ErrUseLastResponse", err)
		}
	})
}

// --- SetSSRFBypass ---

func TestSetSSRFBypass(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	t.Run("default is false", func(t *testing.T) {
		if d.ssrfBypass.Load() {
			t.Error("expected ssrfBypass to default to false")
		}
	})

	t.Run("set to true", func(t *testing.T) {
		d.SetSSRFBypass(true)
		if !d.ssrfBypass.Load() {
			t.Error("expected ssrfBypass=true after SetSSRFBypass(true)")
		}
	})

	t.Run("set back to false", func(t *testing.T) {
		d.SetSSRFBypass(false)
		if d.ssrfBypass.Load() {
			t.Error("expected ssrfBypass=false after SetSSRFBypass(false)")
		}
	})
}

// --- Dispatch (async fire-and-forget) ---

func TestDispatch_NilCallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	// Should not panic or block.
	d.Dispatch(nil, "stub-1", RequestContext{})
	// No log entry should be recorded for nil callback.
	if log.Len() != 0 {
		t.Errorf("expected 0 log entries for nil callback, got %d", log.Len())
	}
}

func TestDispatch_Disabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, false) // disabled

	callback := &spec.CallbackDefinition{
		URL:  "http://example.com/callback",
		Body: `{"hello":"world"}`,
	}
	d.Dispatch(callback, "stub-disabled", RequestContext{Method: "GET", Path: "/test"})

	// Wait for async dispatch to complete.
	d.WaitInFlight(context.Background())

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDisabled {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackDisabled)
	}
	if entries[0].CallbackURL != callback.URL {
		t.Errorf("callbackURL = %q, want %q", entries[0].CallbackURL, callback.URL)
	}
}

func TestDispatch_ConnectionRefused_RecordsError(t *testing.T) {
	// Verify that a connection-refused error is recorded as CallbackError
	// and the WaitGroup is properly managed (goroutine exits cleanly).
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true) // bypass SSRF so the request actually goes out

	callback := &spec.CallbackDefinition{
		URL: "http://127.0.0.1:1/invalid", // port 1 — connection refused
	}
	d.Dispatch(callback, "stub-refused", RequestContext{})

	// Should not hang — the goroutine completes (with an error entry).
	d.WaitInFlight(context.Background())

	if log.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", log.Len())
	}
	// Connection refused -> CallbackError.
	if entries := log.List(); entries[0].Status != spec.CallbackError {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackError)
	}
}

func TestDispatch_PanicRecovery(t *testing.T) {
	// Verify that a panic inside the goroutine is caught by the recovery
	// handler and does NOT crash the process. The WaitGroup must still be
	// properly managed (Done() called) so WaitInFlight can return.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	panicRecorder := &panicRecorder{}
	d := NewDispatcher(logger, panicRecorder, time.Second, true)
	d.SetSSRFBypass(true)

	// A connection-refused URL triggers a Record() call which will panic.
	// The recovery handler should catch it; the test should not crash.
	callback := &spec.CallbackDefinition{
		URL: "http://127.0.0.1:1/invalid",
	}
	d.Dispatch(callback, "stub-panic", RequestContext{})

	// WaitInFlight must return — proves Done() was called despite the panic.
	done := make(chan struct{})
	go func() {
		d.WaitInFlight(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Success — panic recovered, WaitGroup managed.
	case <-time.After(2 * time.Second):
		t.Fatal("WaitInFlight did not return — WaitGroup may not be managed correctly after panic")
	}
}

// panicRecorder is a CallbackRecorder whose Record method panics — used to
// test the panic recovery path in Dispatch.
type panicRecorder struct{}

func (p *panicRecorder) Record(entry spec.CallbackEntry) {
	panic("intentional panic for testing recovery")
}

// --- dispatchSync: all callback states ---

func TestDispatchSync_InvalidURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	tests := []struct {
		name        string
		url         string
		wantErrSub string
	}{
		{
			name:        "empty URL",
			url:         "",
			wantErrSub: "unsupported scheme",
		},
		{
			name:        "invalid scheme",
			url:         "ftp://example.com/callback",
			wantErrSub: "unsupported scheme",
		},
		{
			name:        "missing host",
			url:         "http://",
			wantErrSub: "missing host",
		},
		{
			name:        "URL with control characters",
			url:         "http://example.com/callback\x00bad",
			wantErrSub: "invalid callback URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Clear()
			callback := &spec.CallbackDefinition{URL: tt.url}
			d.dispatchSync(callback, "stub-invalid-url", RequestContext{})

			entries := log.List()
			if len(entries) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(entries))
			}
			if entries[0].Status != spec.CallbackError {
				t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackError)
			}
			if !strings.Contains(entries[0].Error, tt.wantErrSub) {
				t.Errorf("error = %q, want to contain %q", entries[0].Error, tt.wantErrSub)
			}
		})
	}
}

func TestDispatchSync_SSRFBlocked(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	// Do NOT set SSRF bypass — we want SSRF protection active.

	tests := []struct {
		name string
		url  string
	}{
		{"loopback 127.0.0.1", "http://127.0.0.1:8080/callback"},
		{"loopback localhost", "http://localhost:8080/callback"},
		{"RFC1918 10.x", "http://10.0.0.1:8080/callback"},
		{"RFC1918 192.168.x", "http://192.168.1.1:8080/callback"},
		{"CGNAT 100.64.x", "http://100.64.0.1:8080/callback"},
		{"link-local 169.254.x", "http://169.254.1.1:8080/callback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Clear()
			callback := &spec.CallbackDefinition{URL: tt.url}
			d.dispatchSync(callback, "stub-ssrf", RequestContext{})

			entries := log.List()
			if len(entries) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(entries))
			}
			if entries[0].Status != spec.CallbackSSRFBlocked {
				t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackSSRFBlocked)
			}
			if entries[0].StatusCode != 0 {
				t.Errorf("statusCode = %d, want 0 (request should not have been sent)", entries[0].StatusCode)
			}
		})
	}
}

func TestDispatchSync_SSRFBypass_AllowsPrivateIP(t *testing.T) {
	// With SSRF bypass enabled, callbacks to private IPs should proceed
	// to the HTTP request stage (and fail with connection refused, not SSRF blocked).
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 500*time.Millisecond, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL: "http://127.0.0.1:1/callback", // port 1 — connection refused
	}
	d.dispatchSync(callback, "stub-bypass", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	// With bypass, the SSRF check is skipped. The request goes out and fails
	// with a connection error (not SSRF blocked).
	if entries[0].Status == spec.CallbackSSRFBlocked {
		t.Error("expected SSRF check to be bypassed, but got CallbackSSRFBlocked")
	}
}

func TestDispatchSync_CallbackDelivered(t *testing.T) {
	// Set up a test HTTP server that the callback will hit.
	var receivedRequests []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedRequests = append(receivedRequests, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 5*time.Second, true)
	d.SetSSRFBypass(true) // bypass SSRF for test server (localhost)

	callback := &spec.CallbackDefinition{
		URL:    server.URL + "/webhook",
		Method: http.MethodPost,
		Headers: map[string]string{
			"X-Custom": "test-value",
		},
		Body: `{"method":"{{.Method}}","path":"{{.Path}}"}`,
	}

	reqCtx := RequestContext{
		Method: http.MethodPost,
		Path:   "/api/users",
	}
	d.dispatchSync(callback, "stub-delivered", reqCtx)

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDelivered {
		t.Errorf("status = %q, want %q (error=%s)", entries[0].Status, spec.CallbackDelivered, entries[0].Error)
	}
	if entries[0].StatusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want %d", entries[0].StatusCode, http.StatusOK)
	}
	if entries[0].RequestMethod != http.MethodPost {
		t.Errorf("requestMethod = %q, want %q", entries[0].RequestMethod, http.MethodPost)
	}
	if entries[0].RequestPath != "/api/users" {
		t.Errorf("requestPath = %q, want %q", entries[0].RequestPath, "/api/users")
	}

	// Verify the server received the request.
	mu.Lock()
	defer mu.Unlock()
	if len(receivedRequests) != 1 || receivedRequests[0] != "/webhook" {
		t.Errorf("server received requests: %v, want [/webhook]", receivedRequests)
	}
}

func TestDispatchSync_CallbackDelivered_DefaultMethod(t *testing.T) {
	// When Method is empty, it should default to POST.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("server received method %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL: server.URL + "/webhook",
		// Method intentionally empty — should default to POST.
	}
	d.dispatchSync(callback, "stub-default-method", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDelivered {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackDelivered)
	}
}

func TestDispatchSync_CallbackDelivered_WithBodySetsContentType(t *testing.T) {
	// When Body is set and no Content-Type header is provided, the dispatcher
	// should set Content-Type: application/json automatically.
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL:  server.URL + "/webhook",
		Body: `{"data":"test"}`,
	}
	d.dispatchSync(callback, "stub-content-type", RequestContext{})

	if receivedContentType != "application/json" {
		t.Errorf("Content-Type received by server = %q, want %q", receivedContentType, "application/json")
	}
}

func TestDispatchSync_CallbackDelivered_PreservesExplicitContentType(t *testing.T) {
	// When a Content-Type header is explicitly set, it should not be overridden.
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL:    server.URL + "/webhook",
		Body:   `<xml>data</xml>`,
		Headers: map[string]string{"Content-Type": "application/xml"},
	}
	d.dispatchSync(callback, "stub-explicit-ct", RequestContext{})

	if receivedContentType != "application/xml" {
		t.Errorf("Content-Type received by server = %q, want %q", receivedContentType, "application/xml")
	}
}

func TestDispatchSync_CallbackTimeout(t *testing.T) {
	// Server that delays responding beyond the callback timeout.
	// The handler respects context cancellation so the server can shut down
	// promptly when the client disconnects (avoids blocking server.Close()).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Client disconnected — return early.
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 200*time.Millisecond, true) // 200ms timeout
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL: server.URL + "/slow",
	}
	d.dispatchSync(callback, "stub-timeout", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackTimeout {
		t.Errorf("status = %q, want %q (error=%s)", entries[0].Status, spec.CallbackTimeout, entries[0].Error)
	}
}

func TestDispatchSync_CallbackTimeout_PerCallbackOverride(t *testing.T) {
	// When TimeoutMs is set, it overrides the default timeout.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 10*time.Second, true) // default 10s, but per-callback overrides
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL:       server.URL + "/slow",
		TimeoutMs: 200, // 200ms per-callback timeout
	}
	d.dispatchSync(callback, "stub-timeout-override", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackTimeout {
		t.Errorf("status = %q, want %q (error=%s)", entries[0].Status, spec.CallbackTimeout, entries[0].Error)
	}
}

func TestDispatchSync_CallbackError_ConnectionRefused(t *testing.T) {
	// Use a port that's almost certainly closed.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 1*time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL: "http://127.0.0.1:1/callback", // port 1 — connection refused
	}
	d.dispatchSync(callback, "stub-refused", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackError {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackError)
	}
	if !strings.Contains(entries[0].Error, "callback request failed") {
		t.Errorf("error = %q, want to contain %q", entries[0].Error, "callback request failed")
	}
}

func TestDispatchSync_TemplateRenderError(t *testing.T) {
	// A template that references a non-existent field should produce a render error.
	// However, text/template with an invalid syntax fails at parse time, which is
	// the path we test here.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL:  "http://127.0.0.1:1/callback",
		Body: `{{.Request.Method`, // unclosed template — parse error
	}
	d.dispatchSync(callback, "stub-tmpl-err", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackError {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackError)
	}
	if !strings.Contains(entries[0].Error, "template render error") {
		t.Errorf("error = %q, want to contain %q", entries[0].Error, "template render error")
	}
}

func TestDispatchSync_NoBodySendsNilBody(t *testing.T) {
	// When Body is empty, the request should have no body.
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)
	d.SetSSRFBypass(true)

	callback := &spec.CallbackDefinition{
		URL: server.URL + "/webhook",
		// Body intentionally empty.
	}
	d.dispatchSync(callback, "stub-nobody", RequestContext{})

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDelivered {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackDelivered)
	}
	// The body should be empty (no Content-Type header set either).
	if len(receivedBody) != 0 {
		t.Errorf("server received body = %q, want empty", string(receivedBody))
	}
}

func TestDispatchSync_EntryMetadata(t *testing.T) {
	// Verify that the log entry contains correct metadata fields.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, false) // disabled for predictable status

	callback := &spec.CallbackDefinition{
		URL: "http://example.com/callback",
	}
	reqCtx := RequestContext{
		Method:      http.MethodDelete,
		Path:        "/api/items/42",
		QueryString: "force=true",
	}
	d.dispatchSync(callback, "stub-meta", reqCtx)

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	e := entries[0]
	if e.StubID != "stub-meta" {
		t.Errorf("stubID = %q, want %q", e.StubID, "stub-meta")
	}
	if e.CallbackURL != callback.URL {
		t.Errorf("callbackURL = %q, want %q", e.CallbackURL, callback.URL)
	}
	if e.RequestMethod != http.MethodDelete {
		t.Errorf("requestMethod = %q, want %q", e.RequestMethod, http.MethodDelete)
	}
	if e.RequestPath != "/api/items/42" {
		t.Errorf("requestPath = %q, want %q", e.RequestPath, "/api/items/42")
	}
	if e.DispatchedAt.IsZero() {
		t.Error("dispatchedAt should not be zero")
	}
}

// --- renderTemplate ---

func TestRenderTemplate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	ctx := RequestContext{
		Method:      http.MethodPost,
		Path:        "/api/users",
		QueryString: "page=1",
		Headers:     map[string]string{"Authorization": "Bearer token123"},
		Body:        `{"name":"alice"}`,
	}

	tests := []struct {
		name      string
		tmpl      string
		wantErr   bool
		wantExact string
	}{
		{
			name:      "method interpolation",
			tmpl:      `Method: {{.Method}}`,
			wantExact: "Method: POST",
		},
		{
			name:      "path interpolation",
			tmpl:      `Path: {{.Path}}`,
			wantExact: "Path: /api/users",
		},
		{
			name:      "query string interpolation",
			tmpl:      `Query: {{.QueryString}}`,
			wantExact: "Query: page=1",
		},
		{
			name:      "header interpolation",
			tmpl:      `Auth: {{.Headers.Authorization}}`,
			wantExact: "Auth: Bearer token123",
		},
		{
			name:      "body interpolation",
			tmpl:      `Body: {{.Body}}`,
			wantExact: `Body: {"name":"alice"}`,
		},
		{
			name:      "empty template",
			tmpl:      "",
			wantExact: "",
		},
		{
			name:      "no interpolation",
			tmpl:      "static text",
			wantExact: "static text",
		},
		{
			name:    "invalid template syntax",
			tmpl:    `{{.Method`,
			wantErr: true,
		},
		{
			name:    "non-existent field",
			tmpl:    `{{.NonExistent}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.renderTemplate(tt.tmpl, ctx)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantExact {
				t.Errorf("renderTemplate = %q, want %q", got, tt.wantExact)
			}
		})
	}
}

func TestRenderTemplate_Caching(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	ctx := RequestContext{Method: "GET", Path: "/test"}
	tmpl := `Method: {{.Method}}`

	// First call: parses and caches.
	got1, err := d.renderTemplate(tmpl, ctx)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	if d.templateCount.Load() != 1 {
		t.Errorf("templateCount after first render = %d, want 1", d.templateCount.Load())
	}

	// Second call: uses cache.
	got2, err := d.renderTemplate(tmpl, ctx)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if got1 != got2 {
		t.Errorf("cached result = %q, want %q", got2, got1)
	}
	// Count should still be 1 (not incremented for cache hit).
	if d.templateCount.Load() != 1 {
		t.Errorf("templateCount after second render = %d, want 1", d.templateCount.Load())
	}
}

func TestRenderTemplate_CacheSizeLimit(t *testing.T) {
	// Verify the 100-entry cache limit.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	ctx := RequestContext{Method: "GET", Path: "/test"}

	// Render 105 unique templates (exceeds the 100 limit).
	for i := 0; i < 105; i++ {
		tmpl := fmt.Sprintf(`template-%d {{.Method}}`, i)
		got, err := d.renderTemplate(tmpl, ctx)
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		want := fmt.Sprintf("template-%d GET", i)
		if got != want {
			t.Errorf("render %d = %q, want %q", i, got, want)
		}
	}

	// Count should be capped at 100.
	count := d.templateCount.Load()
	if count != 100 {
		t.Errorf("templateCount = %d, want 100 (capped)", count)
	}
}

// --- WaitInFlight ---

func TestWaitInFlight_NoInFlight(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	// With no in-flight callbacks, WaitInFlight should return immediately.
	done := make(chan struct{})
	go func() {
		d.WaitInFlight(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(1 * time.Second):
		t.Fatal("WaitInFlight should return immediately when no callbacks in flight")
	}
}

func TestWaitInFlight_WaitsForCompletion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 5*time.Second, true)
	d.SetSSRFBypass(true)

	// Start a callback that takes some time.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d.Dispatch(&spec.CallbackDefinition{URL: server.URL + "/slow"}, "stub-wait", RequestContext{})

	// WaitInFlight should block until the callback completes.
	done := make(chan struct{})
	go func() {
		d.WaitInFlight(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Success — waited for the callback.
	case <-time.After(2 * time.Second):
		t.Fatal("WaitInFlight did not return after callbacks completed")
	}
}

func TestWaitInFlight_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 10*time.Second, true)
	d.SetSSRFBypass(true)

	// Start a callback that takes a long time. The handler respects context
	// cancellation so the server can shut down promptly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(30 * time.Second): // longer than the test timeout
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	d.Dispatch(&spec.CallbackDefinition{URL: server.URL + "/hang"}, "stub-hang", RequestContext{})

	// Give the dispatch a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Cancel the context after 200ms — WaitInFlight should return.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		d.WaitInFlight(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success — returned after context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("WaitInFlight did not return after context cancellation")
	}
}

func TestWaitInFlight_MultipleCallsSafe(t *testing.T) {
	// Calling WaitInFlight multiple times should be safe.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, time.Second, true)

	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			d.WaitInFlight(context.Background())
			done <- struct{}{}
		}()
	}

	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatalf("WaitInFlight call %d did not return", i)
		}
	}
}

// --- Dispatch is async (fire-and-forget) ---

func TestDispatch_IsAsync(t *testing.T) {
	// Verify that Dispatch returns immediately, not waiting for the callback.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	log := callbacklog.New(100)
	d := NewDispatcher(logger, log, 5*time.Second, true)
	d.SetSSRFBypass(true)

	// Server that delays responding.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	start := time.Now()
	d.Dispatch(&spec.CallbackDefinition{URL: server.URL + "/slow"}, "stub-async", RequestContext{})
	elapsed := time.Since(start)

	// Dispatch should return in well under 500ms (the server delay).
	if elapsed > 200*time.Millisecond {
		t.Errorf("Dispatch took %v, expected to return immediately (fire-and-forget)", elapsed)
	}

	// Wait for the callback to finish.
	d.WaitInFlight(context.Background())

	entries := log.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDelivered {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackDelivered)
	}
}

// --- CallbackRecorder interface compliance ---

type fakeRecorder struct {
	mu      sync.Mutex
	entries []spec.CallbackEntry
}

func (f *fakeRecorder) Record(entry spec.CallbackEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
}

func (f *fakeRecorder) List() []spec.CallbackEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]spec.CallbackEntry, len(f.entries))
	copy(result, f.entries)
	return result
}

func TestDispatchSync_WithCustomRecorder(t *testing.T) {
	// Verify the dispatcher works with any CallbackRecorder implementation.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	recorder := &fakeRecorder{}
	d := NewDispatcher(logger, recorder, time.Second, false) // disabled

	d.dispatchSync(&spec.CallbackDefinition{URL: "http://example.com/cb"}, "stub-custom", RequestContext{})

	entries := recorder.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Status != spec.CallbackDisabled {
		t.Errorf("status = %q, want %q", entries[0].Status, spec.CallbackDisabled)
	}
}

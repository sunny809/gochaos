// Package callback implements post-response callback dispatch with SSRF protection.
//
// The dispatcher fires callbacks asynchronously after a mock response has been
// written to the client. Each callback is dispatched in a fire-and-forget
// goroutine tracked by a WaitGroup so that graceful shutdown can wait for
// in-flight callbacks to complete.
//
// SSRF protection is enforced at dispatch time — DNS resolution is performed
// when the callback fires (not at registration), and any resolved IP in a
// private/reserved range causes the callback to be blocked and logged.
package callback

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// blockedCIDRs lists RFC 1918, RFC 6598 (CGNAT), loopback, link-local, and
// other reserved IP ranges that callbacks must never reach. DNS resolution at
// dispatch time prevents TOCTOU attacks where a domain resolves to a public IP
// at registration time but to a private IP at dispatch time.
var (
	blockedCIDRs   []*net.IPNet
	parseCIDRsOnce sync.Once
)

// initBlockedCIDRs populates the blockedCIDRs slice. Called lazily via sync.Once
// on the first call to IsBlockedIP, avoiding init() per project convention.
func initBlockedCIDRs() {
	networks := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"100.64.0.0/10",  // RFC 6598 (CGNAT)
		"169.254.0.0/16", // link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 ULA
	}
	for _, network := range networks {
		_, cidr, err := net.ParseCIDR(network)
		if err != nil {
			panic(fmt.Sprintf("callback: invalid blocked CIDR %q: %v", network, err))
		}
		blockedCIDRs = append(blockedCIDRs, cidr)
	}
}

// IsBlockedIP returns true if the IP falls within any blocked CIDR range.
// Exported for testing.
func IsBlockedIP(ip net.IP) bool {
	parseCIDRsOnce.Do(initBlockedCIDRs)
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// IsBlockedHost resolves the host at dispatch time and returns true if any
// resolved IP is in a blocked range. This prevents SSRF via DNS rebinding.
// Exported for testing.
func IsBlockedHost(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS resolution fails, block the request — we can't verify safety.
		return true
	}
	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return true
		}
	}
	return false
}

// RequestContext provides template context for callback body rendering.
type RequestContext struct {
	Method      string
	Path        string
	QueryString string
	Headers     map[string]string
	Body        string
}

// CallbackRecorder is the interface for recording callback dispatch events.
// Implemented by callbacklog.Log.
type CallbackRecorder interface {
	Record(entry spec.CallbackEntry)
}

// Dispatcher handles async post-response callback dispatch with SSRF protection.
type Dispatcher struct {
	client         *http.Client
	logger         *slog.Logger
	callbackLog    CallbackRecorder
	wg             sync.WaitGroup
	templateCache  sync.Map
	templateCount  atomic.Int32 // tracks cache size for bounded growth
	defaultTimeout time.Duration
	enabled        bool
	ssrfBypass     atomic.Bool // when true, skip SSRF checks (for testing only)
}

// NewDispatcher creates a callback dispatcher with the given configuration.
func NewDispatcher(logger *slog.Logger, callbackLog CallbackRecorder, defaultTimeout time.Duration, enabled bool) *Dispatcher {
	return &Dispatcher{
		client: &http.Client{
			// No client-level timeout; per-callback timeout via context.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // don't follow redirects
			},
		},
		logger:         logger,
		callbackLog:    callbackLog,
		defaultTimeout: defaultTimeout,
		enabled:        enabled,
	}
}

// SetSSRFBypass enables or disables SSRF protection. When bypass is true, the
// dispatcher skips IP-based SSRF checks. This should ONLY be used in tests.
func (d *Dispatcher) SetSSRFBypass(bypass bool) {
	d.ssrfBypass.Store(bypass)
}

// Dispatch fires a callback asynchronously after the mock response has been
// written. The callback runs in a goroutine tracked by the dispatcher's
// WaitGroup, so Shutdown can wait for in-flight callbacks.
func (d *Dispatcher) Dispatch(callback *spec.CallbackDefinition, stubID string, reqCtx RequestContext) {
	if callback == nil {
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				d.logger.Error("callback goroutine panicked", "stubId", stubID, "panic", r)
			}
		}()

		d.dispatchSync(callback, stubID, reqCtx)
	}()
}

// WaitInFlight waits for all in-flight callback goroutines to complete or
// until the context is cancelled. Calling WaitInFlight multiple times is safe
// (subsequent calls with a live context return almost immediately when the wg
// counter reaches zero).
func (d *Dispatcher) WaitInFlight(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		// The goroutine above will remain blocked until all in-flight
		// callbacks complete (bounded by per-callback timeout), then exit.
		// Since the caller is shutting down, no new callbacks are dispatched.
		d.logger.Warn("callback shutdown: some in-flight callbacks did not complete", "error", ctx.Err())
	}
}

// dispatchSync executes a single callback synchronously (called from a goroutine).
func (d *Dispatcher) dispatchSync(callback *spec.CallbackDefinition, stubID string, reqCtx RequestContext) {
	now := time.Now()
	entry := spec.CallbackEntry{
		StubID:        stubID,
		CallbackURL:   callback.URL,
		DispatchedAt:  now,
		RequestMethod: reqCtx.Method,
		RequestPath:   reqCtx.Path,
	}

	// Check if callbacks are globally disabled
	if !d.enabled {
		entry.Status = spec.CallbackDisabled
		d.callbackLog.Record(entry)
		return
	}

	// Parse URL and check SSRF
	parsedURL, err := url.Parse(callback.URL)
	if err != nil {
		entry.Status = spec.CallbackError
		entry.Error = fmt.Sprintf("invalid callback URL: %v", err)
		d.callbackLog.Record(entry)
		return
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		entry.Status = spec.CallbackError
		entry.Error = fmt.Sprintf("unsupported scheme %q (must be http or https)", parsedURL.Scheme)
		d.callbackLog.Record(entry)
		return
	}

	if parsedURL.Host == "" {
		entry.Status = spec.CallbackError
		entry.Error = "missing host in callback URL"
		d.callbackLog.Record(entry)
		return
	}

	host := parsedURL.Hostname()
	if !d.ssrfBypass.Load() && IsBlockedHost(host) {
		entry.Status = spec.CallbackSSRFBlocked
		entry.Error = fmt.Sprintf("callback target %q resolves to blocked IP range", host)
		d.callbackLog.Record(entry)
		d.logger.Warn("callback blocked: SSRF protection", "url", callback.URL, "stubId", stubID)
		return
	}

	// Determine timeout
	timeout := d.defaultTimeout
	if callback.TimeoutMs > 0 {
		timeout = time.Duration(callback.TimeoutMs) * time.Millisecond
	}

	// Render callback body template
	var bodyReader *strings.Reader
	if callback.Body != "" {
		rendered, err := d.renderTemplate(callback.Body, reqCtx)
		if err != nil {
			entry.Status = spec.CallbackError
			entry.Error = fmt.Sprintf("template render error: %v", err)
			d.callbackLog.Record(entry)
			return
		}
		bodyReader = strings.NewReader(rendered)
	}

	// Build the HTTP request
	method := callback.Method
	if method == "" {
		method = http.MethodPost
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var req *http.Request
	if bodyReader != nil {
		req, err = http.NewRequestWithContext(ctx, method, callback.URL, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, callback.URL, nil)
	}
	if err != nil {
		entry.Status = spec.CallbackError
		entry.Error = fmt.Sprintf("failed to create callback request: %v", err)
		d.callbackLog.Record(entry)
		return
	}

	// Set headers
	for k, v := range callback.Headers {
		req.Header.Set(k, v)
	}
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute the callback
	resp, err := d.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			entry.Status = spec.CallbackTimeout
			entry.Error = "callback timed out"
		} else {
			entry.Status = spec.CallbackError
			entry.Error = fmt.Sprintf("callback request failed: %v", err)
		}
		d.callbackLog.Record(entry)
		return
	}
	// Drain the response body before closing to allow HTTP connection reuse.
	// Limit drain to 1KB to prevent unbounded memory usage from large responses.
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	resp.Body.Close()

	entry.Status = spec.CallbackDelivered
	entry.StatusCode = resp.StatusCode
	d.callbackLog.Record(entry)

	d.logger.Debug("callback delivered",
		"url", callback.URL,
		"stubId", stubID,
		"status", resp.StatusCode,
	)
}

// renderTemplate renders a text/template with the request context, caching
// parsed templates by their source text for efficiency.
func (d *Dispatcher) renderTemplate(tmplText string, ctx RequestContext) (string, error) {
	// Check cache first
	if cached, ok := d.templateCache.Load(tmplText); ok {
		tmpl := cached.(*template.Template)
		var buf strings.Builder
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return "", fmt.Errorf("template execute error: %w", err)
		}
		return buf.String(), nil
	}

	tmpl, err := template.New("callback").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	// Limit cache size to prevent unbounded growth (max 100 entries).
	if d.templateCount.Load() < 100 {
		d.templateCache.Store(tmplText, tmpl)
		d.templateCount.Add(1)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template execute error: %w", err)
	}
	return buf.String(), nil
}

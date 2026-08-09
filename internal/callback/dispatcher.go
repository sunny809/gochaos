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
	"errors"
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

type (
	// RequestContext provides template context for callback body rendering.
	RequestContext struct {
		Method      string
		Path        string
		QueryString string
		Headers     map[string]string
		Body        string
	}

	// CallbackRecorder is the interface for recording callback dispatch events.
	// Implemented by callbacklog.Log.
	//
	//nolint:revive // "CallbackRecorder" stutters with package name, but "Recorder"
	// is too generic and this is the established name across the codebase.
	CallbackRecorder interface {
		Record(entry spec.CallbackEntry)
	}

	// Dispatcher handles async post-response callback dispatch with SSRF protection.
	Dispatcher struct {
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
)

// blockedCIDRs lists RFC 1918, RFC 6598 (CGNAT), loopback, link-local, and
// other reserved IP ranges that callbacks must never reach. DNS resolution at
// dispatch time prevents TOCTOU attacks where a domain resolves to a public IP
// at registration time but to a private IP at dispatch time.
var (
	blockedCIDRs   []*net.IPNet
	parseCIDRsOnce sync.Once

	// errNoCallbackBody signals that the callback has no body to send.
	// Used so prepareCallbackBody can return a nil reader without a nil error.
	errNoCallbackBody = errors.New("no callback body")
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
	// Use a context with timeout to prevent indefinite blocking on slow DNS.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ipAddrs, err := (&net.Resolver{}).LookupIPAddr(ctx, host)
	if err != nil {
		// If DNS resolution fails, block the request — we can't verify safety.
		return true
	}
	for _, ipAddr := range ipAddrs {
		if IsBlockedIP(ipAddr.IP) {
			return true
		}
	}
	return false
}

// NewDispatcher creates a callback dispatcher with the given configuration.
func NewDispatcher(logger *slog.Logger, callbackLog CallbackRecorder, defaultTimeout time.Duration, enabled bool) *Dispatcher {
	return &Dispatcher{
		client: &http.Client{
			// No client-level timeout; per-callback timeout via context.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
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

// validateCallbackURL parses the callback URL, verifies it uses http/https,
// has a host, and does not resolve to a blocked IP range (SSRF protection).
// Returns the parsed URL and host, or an error string suitable for CallbackEntry.Error.
func (d *Dispatcher) validateCallbackURL(callbackURL string) (parsed *url.URL, host string, errMsg string) {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return nil, "", fmt.Sprintf("invalid callback URL: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, "", fmt.Sprintf("unsupported scheme %q (must be http or https)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, "", "missing host in callback URL"
	}
	host = parsed.Hostname()
	if !d.ssrfBypass.Load() && IsBlockedHost(host) {
		return nil, host, fmt.Sprintf("callback target %q resolves to blocked IP range", host)
	}
	return parsed, host, ""
}

// effectiveTimeout returns the per-callback timeout if set, otherwise the default.
func (d *Dispatcher) effectiveTimeout(timeoutMs int) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return d.defaultTimeout
}

// prepareCallbackBody renders the callback body template (if any) and returns
// a reader for the HTTP request body. Returns a nil reader with no error when
// body is empty (no body to send).
func (d *Dispatcher) prepareCallbackBody(body string, reqCtx RequestContext) (*strings.Reader, error) {
	if body == "" {
		return nil, errNoCallbackBody
	}
	rendered, err := d.renderTemplate(body, reqCtx)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(rendered), nil
}

// checkCallbackURL validates the callback URL and populates the entry with
// the appropriate status. Returns ("", "") if the URL is valid, otherwise
// the status and error message to record.
func (d *Dispatcher) checkCallbackURL(callbackURL, stubID string, entry *spec.CallbackEntry) (status spec.CallbackStatus, errMsg string) {
	_, host, validationErr := d.validateCallbackURL(callbackURL)
	if validationErr == "" {
		return "", ""
	}
	entry.Error = validationErr
	// A non-empty host means the URL parsed fine but the host resolved to
	// a blocked IP range (SSRF). An empty host indicates a malformed URL
	// (parse error, bad scheme, missing host).
	if host == "" {
		return spec.CallbackError, validationErr
	}
	d.logger.Warn("callback blocked: SSRF protection", "url", callbackURL, "stubId", stubID)
	return spec.CallbackSSRFBlocked, validationErr
}

// buildCallbackRequest constructs the HTTP request for a callback, setting
// method, headers, and optional body. The returned cancel function must be
// called by the caller (via defer) once the request lifecycle ends.
func (d *Dispatcher) buildCallbackRequest(callback *spec.CallbackDefinition, timeout time.Duration, bodyReader *strings.Reader) (*http.Request, context.CancelFunc, error) {
	method := callback.Method
	if method == "" {
		method = http.MethodPost
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequestWithContext(ctx, method, callback.URL, bodyReader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, callback.URL, nil)
	}
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create callback request: %v", err)
	}

	for k, v := range callback.Headers {
		req.Header.Set(k, v)
	}
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, cancel, nil
}

// executeCallback sends the request, drains/closes the response, and records
// the entry with the final status.
func (d *Dispatcher) executeCallback(req *http.Request, entry *spec.CallbackEntry) {
	resp, err := d.client.Do(req)
	if err != nil {
		if req.Context().Err() == context.DeadlineExceeded {
			entry.Status = spec.CallbackTimeout
			entry.Error = "callback timed out"
		} else {
			entry.Status = spec.CallbackError
			entry.Error = fmt.Sprintf("callback request failed: %v", err)
		}
		d.callbackLog.Record(*entry)
		return
	}
	// Drain the response body before closing to allow HTTP connection reuse.
	// Limit drain to 1KB to prevent unbounded memory usage from large responses.
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	if err := resp.Body.Close(); err != nil {
		d.logger.Warn("failed to close callback response body", "error", err)
	}

	entry.Status = spec.CallbackDelivered
	entry.StatusCode = resp.StatusCode
	d.callbackLog.Record(*entry)

	d.logger.Debug("callback delivered",
		"stubId", entry.StubID,
		"status", resp.StatusCode,
	)
}

// dispatchSync executes a single callback synchronously (called from a goroutine).
func (d *Dispatcher) dispatchSync(callback *spec.CallbackDefinition, stubID string, reqCtx RequestContext) {
	entry := spec.CallbackEntry{
		StubID:        stubID,
		CallbackURL:   callback.URL,
		DispatchedAt:  time.Now(),
		RequestMethod: reqCtx.Method,
		RequestPath:   reqCtx.Path,
	}

	if !d.enabled {
		entry.Status = spec.CallbackDisabled
		d.callbackLog.Record(entry)
		return
	}

	if status, _ := d.checkCallbackURL(callback.URL, stubID, &entry); status != "" {
		entry.Status = status
		d.callbackLog.Record(entry)
		return
	}

	timeout := d.effectiveTimeout(callback.TimeoutMs)
	bodyReader, err := d.prepareCallbackBody(callback.Body, reqCtx)
	if err != nil && !errors.Is(err, errNoCallbackBody) {
		entry.Status = spec.CallbackError
		entry.Error = fmt.Sprintf("template render error: %v", err)
		d.callbackLog.Record(entry)
		return
	}

	req, cancel, err := d.buildCallbackRequest(callback, timeout, bodyReader)
	if err != nil {
		entry.Status = spec.CallbackError
		entry.Error = err.Error()
		d.callbackLog.Record(entry)
		return
	}
	defer cancel()

	d.executeCallback(req, &entry)
}

// renderTemplate renders a text/template with the request context, caching
// parsed templates by their source text for efficiency.
func (d *Dispatcher) renderTemplate(tmplText string, ctx RequestContext) (string, error) {
	// Check cache first
	if cached, ok := d.templateCache.Load(tmplText); ok {
		tmpl, ok := cached.(*template.Template)
		if !ok {
			return "", fmt.Errorf("template cache corruption: expected *template.Template, got %T", cached)
		}
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

package gmock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/PaesslerAG/jsonpath"
	"github.com/sunny809/gochaos/config"
	"github.com/sunny809/gochaos/internal/admin"
	internallog "github.com/sunny809/gochaos/internal/log"
	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/response"
	"github.com/sunny809/gochaos/internal/stub"
)

// Server is the public interface for a gmock server instance.
type Server interface {
	// Start launches the server. Returns an error if the server fails to start.
	Start() error

	// Stop gracefully shuts down the server. Blocks until shutdown is complete.
	Stop() error

	// URL returns the base URL of the running server (e.g., "http://127.0.0.1:8080").
	URL() string

	// AdminURL returns the base URL of the admin API.
	AdminURL() string

	// Stub registers a stub definition and returns its ID.
	Stub(stub StubDefinition) string

	// StubJSON registers a stub from a JSON byte slice.
	StubJSON(data []byte) (string, error)

	// DeleteStub removes a stub by ID.
	DeleteStub(id string) bool

	// ClearStubs removes all registered stubs.
	ClearStubs()

	// Reset clears all stubs, request log, and scenario state.
	Reset()

	// Verify checks that a request matching the given pattern was received.
	Verify(pattern RequestPattern, count int) VerificationResult

	// VerifyNotCalled checks that no request matching the given pattern was received.
	VerifyNotCalled(pattern RequestPattern) VerificationResult

	// RequestLog returns all logged requests.
	RequestLog() []LoggedRequest

	// UnmatchedRequests returns all requests that matched no stub.
	UnmatchedRequests() []LoggedRequest

	// RecordedStubs returns stubs recorded from proxy mode.
	RecordedStubs() []StubDefinition

	// NearMiss analyzes why a request didn't match any stub.
	NearMiss(method, path string, headers map[string]string, body string) []NearMissResult
}

// mockServer is the concrete implementation of the Server interface.
type mockServer struct {
	mu     sync.RWMutex
	config ServerConfig
	logger *slog.Logger

	// HTTP servers
	httpServer    *http.Server
	adminServer   *http.Server
	listener      net.Listener
	adminListener net.Listener

	// Internal components
	registry       *stub.Registry
	matchEngine    *stub.Engine
	nearMissEngine *nearmiss.Engine
	requestLog     *internallog.RequestLog
	adminHandler   *admin.Handler
	responseWriter response.Writer
}

// NewServer creates a new gmock server with the given options.
func NewServer(opts ...Option) Server {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	logLevel := slog.LevelWarn
	if cfg.Verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	registry := stub.NewRegistry()
	requestLog := internallog.New(cfg.MaxRequests)

	return &mockServer{
		config:         cfg,
		logger:         logger,
		registry:       registry,
		matchEngine:    stub.NewEngine(registry),
		nearMissEngine: nearmiss.NewEngine(),
		requestLog:     requestLog,
		adminHandler:   admin.New(registry, requestLog),
		responseWriter: response.NewHTTPWriter(logger, cfg.DisableGzip),
	}
}

// Start launches the HTTP server.
func (s *mockServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		return fmt.Errorf("gmock: server already running")
	}

	addr := fmt.Sprintf(":%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gmock: failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Determine if admin runs on a separate port
	useSeparateAdminPort := s.config.AdminPort > 0

	mainHandler := s.buildMainHandler(!useSeparateAdminPort)
	s.httpServer = &http.Server{Handler: mainHandler}

	if useSeparateAdminPort {
		adminAddr := fmt.Sprintf(":%d", s.config.AdminPort)
		adminListener, err := net.Listen("tcp", adminAddr)
		if err != nil {
			listener.Close()
			return fmt.Errorf("gmock: failed to listen on admin port %s: %w", adminAddr, err)
		}
		s.adminListener = adminListener
		s.adminServer = &http.Server{Handler: s.adminHandler}
		go s.adminServer.Serve(adminListener) //nolint:errcheck
	}

	go s.httpServer.Serve(listener) //nolint:errcheck

	// Load stub files if configured
	if err := s.loadStubFiles(); err != nil {
		s.logger.Warn("failed to load stub files", "error", err)
	}

	s.logger.Info("gmock server started",
		"url", fmt.Sprintf("http://%s", listener.Addr().String()),
		"separateAdminPort", useSeparateAdminPort,
	)

	return nil
}

// loadStubFiles loads stubs from the configured StubFiles paths.
// Errors from individual files are returned but do not abort loading subsequent files.
func (s *mockServer) loadStubFiles() error {
	if len(s.config.StubFiles) == 0 {
		return nil
	}
	stubs, err := config.LoadStubsFromFiles(s.config.StubFiles)
	if err != nil {
		return err
	}
	for _, def := range stubs {
		if _, err := s.registry.Add(def); err != nil {
			s.logger.Warn("failed to add stub from file", "error", err, "name", def.Name)
		}
	}
	s.logger.Info("loaded stubs from files", "count", len(stubs), "files", s.config.StubFiles)
	return nil
}

// buildMainHandler returns the HTTP handler for the main server port.
// If includeAdmin is true, admin routes are served on the same port.
func (s *mockServer) buildMainHandler(includeAdmin bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dispatch admin requests when admin is on the same port
		if includeAdmin && admin.IsAdminPath(r.URL.Path) {
			s.adminHandler.ServeHTTP(w, r)
			return
		}
		s.serveMock(w, r)
	})
}

// Stop gracefully shuts down the server.
func (s *mockServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("server shutdown: %w", err))
		}
		s.httpServer = nil
	}
	if s.adminServer != nil {
		if err := s.adminServer.Shutdown(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("admin shutdown: %w", err))
		}
		s.adminServer = nil
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	if s.adminListener != nil {
		s.adminListener.Close()
		s.adminListener = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("gmock: shutdown errors: %v", errs)
	}
	return nil
}

// URL returns the base URL of the running server.
func (s *mockServer) URL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", s.listener.Addr().String())
}

// AdminURL returns the base URL of the admin API.
func (s *mockServer) AdminURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.adminListener != nil {
		return fmt.Sprintf("http://%s", s.adminListener.Addr().String())
	}
	if s.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", s.listener.Addr().String())
}

// Stub registers a stub definition and returns the generated ID.
func (s *mockServer) Stub(def StubDefinition) string {
	id, err := s.registry.Add(def)
	if err != nil {
		s.logger.Error("failed to add stub", "error", err)
		return ""
	}
	return id
}

// StubJSON registers a stub from JSON bytes.
func (s *mockServer) StubJSON(data []byte) (string, error) {
	var def StubDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return "", fmt.Errorf("gmock: invalid stub JSON: %w", err)
	}
	return s.registry.Add(def)
}

// DeleteStub removes a stub by ID.
func (s *mockServer) DeleteStub(id string) bool {
	return s.registry.Delete(id)
}

// ClearStubs removes all registered stubs.
func (s *mockServer) ClearStubs() {
	s.registry.DeleteAll()
}

// Reset clears all stubs and request log.
func (s *mockServer) Reset() {
	s.registry.DeleteAll()
	s.requestLog.Clear()
}

// RecordedStubs returns stubs recorded from proxy mode.
// (Stub implementation — full proxy recording in Slice 8)
func (s *mockServer) RecordedStubs() []StubDefinition {
	return nil
}

// NearMiss analyzes why a request did not match any registered stub. The
// returned slice is ordered best-score-first and capped to the engine's
// configured top-N (default 5). When the synthesized request fully matches a
// stub, the slice is empty (non-nil) per the near-miss design contract.
//
// The path argument must be the request-target portion of the URL only —
// that is, an absolute path (beginning with "/") optionally followed by a
// query string ("?k=v"). It must not include a scheme or host (no
// "http://..."), and must not contain raw control characters or other runes
// that net/url rejects. An empty path is treated as "/". Method defaults to
// GET when empty.
//
// If path is malformed such that the synthesized request cannot be built,
// NearMiss logs a warning and returns an empty (non-nil) slice rather than
// panicking — preserving the "never panic" contract for diagnostic helpers.
func (s *mockServer) NearMiss(method, path string, headers map[string]string, body string) []NearMissResult {
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequest(method, "http://internal"+path, strings.NewReader(body))
	if err != nil {
		s.logger.Warn("near-miss: failed to build request from malformed path",
			"error", err, "method", method, "path", path)
		return []NearMissResult{}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	stubs := s.registry.List()
	results := s.nearMissEngine.Compute(req, stubs)
	if results == nil {
		return []NearMissResult{}
	}
	return results
}

// serveMock is the main HTTP handler for mock requests.
// Pipeline:
//   1. Handle CORS preflight if enabled
//   2. Log the request
//   3. Try stub matching
//   4. On match: write the response
//   5. On miss: return 404 with near-miss diagnostic data
func (s *mockServer) serveMock(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight requests
	if s.config.CORSOptions != nil && r.Method == http.MethodOptions && r.Header.Get("Origin") != "" {
		s.writeCORSHeaders(w, r)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Match against registered stubs
	result := s.matchEngine.Match(r)

	matched := result != nil && result.Matched
	stubID := ""
	if matched {
		stubID = result.Stub.ID
	}

	// Log the request (this also captures the body for verification)
	s.requestLog.Record(r, matched, stubID)

	if !matched {
		s.writeNoMatch(w, r)
		return
	}

	// Write the matched response (with optional gzip compression)
	if err := s.responseWriter.WriteResponse(w, result.Stub, r, corsOptsFromConfig(s.config.CORSOptions)); err != nil {
		s.logger.Warn("failed to write response", "stub", result.Stub.ID, "error", err)
	}
}

// writeNoMatch writes a 404 with diagnostic information.
func (s *mockServer) writeNoMatch(w http.ResponseWriter, r *http.Request) {
	// Apply CORS headers if enabled (for unmatched requests too)
	s.responseWriter.WriteCORSHeaders(w, r, corsOptsFromConfig(s.config.CORSOptions))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)

	body := map[string]interface{}{
		"error":  "no stub matched",
		"method": r.Method,
		"path":   r.URL.Path,
	}

	if r.URL.RawQuery != "" {
		body["query"] = r.URL.RawQuery
	}

	buf := &bytes.Buffer{}
	_ = json.NewEncoder(buf).Encode(body)
	_, _ = io.Copy(w, buf)
}

// writeCORSHeaders writes CORS headers for a preflight OPTIONS response.
func (s *mockServer) writeCORSHeaders(w http.ResponseWriter, r *http.Request) {
	s.responseWriter.WriteCORSHeaders(w, r, corsOptsFromConfig(s.config.CORSOptions))
}

// corsOptsFromConfig converts pkg/gmock CORSOptions to internal/response CORSOptions.
func corsOptsFromConfig(opts *CORSOptions) *response.CORSOptions {
	if opts == nil {
		return nil
	}
	return &response.CORSOptions{
		AllowedOrigins:   opts.AllowedOrigins,
		AllowedMethods:   opts.AllowedMethods,
		AllowedHeaders:   opts.AllowedHeaders,
		ExposedHeaders:   opts.ExposedHeaders,
		AllowCredentials: opts.AllowCredentials,
		MaxAge:           opts.MaxAge,
	}
}

// verify checks that a request matching the given pattern was received.
func (s *mockServer) verify(pattern RequestPattern, count int) VerificationResult {
	entries := s.requestLog.Entries()
	actualCount := 0

	for _, entry := range entries {
		if matchPattern(pattern, entry.Request) {
			actualCount++
		}
	}

	result := VerificationResult{
		ExpectedCount:      count,
		ActualCount:        actualCount,
		Matched:            actualCount >= count,
		BodyPattern:        pattern.Body,
		HeaderPattern:      copyMap(pattern.Headers),
		QueryParamPattern:  copyMap(pattern.QueryParams),
	}

	if !result.Matched {
		result.Errors = append(result.Errors,
			fmt.Sprintf("expected at least %d matching requests, got %d", count, actualCount))
	}

	// Special case: VerifyNotCalled (count == 0) requires exact match
	if count == 0 {
		result.Matched = actualCount == 0
		if !result.Matched {
			result.Errors = []string{fmt.Sprintf("expected no matching requests, got %d", actualCount)}
		}
	}

	return result
}

// matchPattern checks if a logged request matches a verification pattern.
func matchPattern(pattern RequestPattern, req LoggedRequest) bool {
	if pattern.Method != "" && !strings.EqualFold(pattern.Method, req.Method) {
		return false
	}
	if pattern.URLPath != "" && pattern.URLPath != req.Path {
		return false
	}
	if pattern.URLPathRegex != "" {
		matched, err := regexp.MatchString(pattern.URLPathRegex, req.Path)
		if err != nil || !matched {
			return false
		}
	}
	if pattern.Body != nil && !matchBody(pattern.Body, req.Body) {
		return false
	}
	for name, valuePattern := range pattern.Headers {
		if !matchHeader(name, valuePattern, req.Headers) {
			return false
		}
	}
	for key, valuePattern := range pattern.QueryParams {
		if !matchQueryParam(key, valuePattern, req.QueryString) {
			return false
		}
	}
	for name, valuePattern := range pattern.Cookies {
		if !matchCookie(name, valuePattern, req.Headers) {
			return false
		}
	}
	return true
}

// -- verification matching helpers --

// matchBody checks if a body string matches the given BodyPattern.
// A nil pattern means no body constraint and always returns true.
func matchBody(pattern *BodyPattern, body string) bool {
	if pattern == nil {
		return true
	}

	if pattern.ExactMatch != "" {
		return body == pattern.ExactMatch
	}

	if pattern.RegexMatch != "" {
		matched, err := regexp.MatchString(pattern.RegexMatch, body)
		if err != nil {
			return false
		}
		return matched
	}

	if pattern.JSONPath != "" {
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			return false
		}
		result, err := jsonpath.Get(pattern.JSONPath, data)
		if err != nil {
			return false
		}
		// Non-nil, non-empty value is a match
		switch v := result.(type) {
		case nil:
			return false
		case string:
			return v != ""
		case []interface{}:
			return len(v) > 0
		case map[string]interface{}:
			return len(v) > 0
		}
		return true
	}

	// Non-nil but empty pattern — no constraint
	return true
}

// matchHeader checks if a header value matches the given pattern.
// name is canonicalized via http.CanonicalHeaderKey.
// Pattern conventions: "!" = absent, "*" = any non-empty, "~regex" = regex, else exact.
func matchHeader(name, valuePattern string, headers HeadersMap) bool {
	canonical := http.CanonicalHeaderKey(name)
	values, exists := headers[canonical]

	switch {
	case valuePattern == "!":
		return !exists
	case valuePattern == "*":
		return exists && len(values) > 0 && values[0] != ""
	case strings.HasPrefix(valuePattern, "~"):
		if !exists || len(values) == 0 {
			return false
		}
		matched, err := regexp.MatchString(valuePattern[1:], values[0])
		if err != nil {
			return false
		}
		return matched
	default:
		return exists && len(values) > 0 && values[0] == valuePattern
	}
}

// matchQueryParam checks if a query parameter value matches the given pattern.
// queryString is the raw query string (e.g., "page=1&limit=10").
// Pattern conventions: "!" = absent, "*" = any non-empty, "~regex" = regex, else exact.
func matchQueryParam(key, valuePattern, queryString string) bool {
	values, err := url.ParseQuery(queryString)
	if err != nil {
		return valuePattern == "!"
	}
	vals := values[key]

	switch {
	case valuePattern == "!":
		return len(vals) == 0
	case valuePattern == "*":
		return len(vals) > 0 && vals[0] != ""
	case strings.HasPrefix(valuePattern, "~"):
		if len(vals) == 0 {
			return false
		}
		matched, err := regexp.MatchString(valuePattern[1:], vals[0])
		if err != nil {
			return false
		}
		return matched
	default:
		return len(vals) > 0 && vals[0] == valuePattern
	}
}

// matchCookie checks if a cookie value matches the given pattern.
// Cookies are read from the "Cookie" header (fallback to "Set-Cookie").
// Pattern conventions: "!" = absent, "*" = any non-empty, "~regex" = regex, else exact.
func matchCookie(name, valuePattern string, headers HeadersMap) bool {
	raw := ""
	if vals, ok := headers["Cookie"]; ok && len(vals) > 0 {
		raw = vals[0]
	} else if vals, ok := headers["Set-Cookie"]; ok && len(vals) > 0 {
		raw = vals[0]
	}

	cookies := parseCookies(raw)
	val, exists := cookies[name]

	switch {
	case valuePattern == "!":
		return !exists
	case valuePattern == "*":
		return exists && val != ""
	case strings.HasPrefix(valuePattern, "~"):
		if !exists {
			return false
		}
		matched, err := regexp.MatchString(valuePattern[1:], val)
		if err != nil {
			return false
		}
		return matched
	default:
		return exists && val == valuePattern
	}
}

// parseCookies parses a raw Cookie header value into a map of cookie name to value.
// Handles standard cookie formatting: "name1=value1; name2=value2"
func parseCookies(raw string) map[string]string {
	result := make(map[string]string)
	if raw == "" {
		return result
	}
	pairs := strings.Split(raw, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key == "" {
			continue
		}
		result[key] = value
	}
	return result
}

// copyMap returns a shallow copy of m. Returns nil when m is nil.
func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

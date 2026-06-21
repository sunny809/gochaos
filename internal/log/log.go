// Package log provides request logging for spec.
//
// Logged requests are stored in a fixed-size ring buffer to bound memory usage.
// The log is concurrent-safe and supports listing, filtering by matched/unmatched
// status, and clearing.
package log

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// Entry wraps a LoggedRequest with internal metadata.
type Entry struct {
	Request spec.LoggedRequest
	Matched bool
	StubID  string // empty if no stub matched
}

// RequestLog is a concurrent-safe ring buffer for incoming requests.
type RequestLog struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	head    int // next index to write
	count   int // number of valid entries
}

// New creates a RequestLog with the given maximum size.
// If max is 0 or negative, defaults to 1000.
func New(max int) *RequestLog {
	if max <= 0 {
		max = 1000
	}
	return &RequestLog{
		entries: make([]Entry, max),
		max:     max,
	}
}

// Record captures an HTTP request into the log.
// The request body is read and stored (and the body is restored for downstream handlers).
func (l *RequestLog) Record(req *http.Request, matched bool, stubID string) {
	entry := Entry{
		Request: spec.LoggedRequest{
			Method:      req.Method,
			Path:        req.URL.Path,
			QueryString: req.URL.RawQuery,
			Headers:     copyHeaders(req.Header),
			ReceivedAt:  time.Now().UTC(),
		},
		Matched: matched,
		StubID:  stubID,
	}

	// Best-effort body capture
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err == nil {
			entry.Request.Body = string(body)
			_ = req.Body.Close()
			// Restore body for downstream handlers
			req.Body = io.NopCloser(strings.NewReader(string(body)))
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.max
	if l.count < l.max {
		l.count++
	}
}

// List returns all logged requests in chronological order (oldest first).
func (l *RequestLog) List() []spec.LoggedRequest {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.snapshot(func(e Entry) bool { return true })
}

// Unmatched returns logged requests that didn't match any stub.
func (l *RequestLog) Unmatched() []spec.LoggedRequest {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.snapshot(func(e Entry) bool { return !e.Matched })
}

// Matched returns logged requests that matched a stub.
func (l *RequestLog) Matched() []spec.LoggedRequest {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.snapshot(func(e Entry) bool { return e.Matched })
}

// MatchingStub returns all logged requests that matched the given stub ID.
func (l *RequestLog) MatchingStub(stubID string) []spec.LoggedRequest {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.snapshot(func(e Entry) bool { return e.StubID == stubID })
}

// Entries returns the raw entries with match metadata (for verification logic).
func (l *RequestLog) Entries() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]Entry, 0, l.count)
	for i := 0; i < l.count; i++ {
		idx := (l.head - l.count + i + l.max) % l.max
		result = append(result, l.entries[idx])
	}
	return result
}

// Clear removes all logged requests.
func (l *RequestLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = make([]Entry, l.max)
	l.head = 0
	l.count = 0
}

// Len returns the number of logged requests.
func (l *RequestLog) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

// snapshot returns a filtered, ordered copy of entries.
// Must be called with at least a read lock held.
func (l *RequestLog) snapshot(filter func(Entry) bool) []spec.LoggedRequest {
	result := make([]spec.LoggedRequest, 0, l.count)
	for i := 0; i < l.count; i++ {
		idx := (l.head - l.count + i + l.max) % l.max
		if filter(l.entries[idx]) {
			result = append(result, l.entries[idx].Request)
		}
	}
	return result
}

// copyHeaders converts http.Header into the JSON-friendly HeadersMap.
func copyHeaders(h http.Header) spec.HeadersMap {
	result := make(spec.HeadersMap, len(h))
	for k, v := range h {
		vc := make([]string, len(v))
		copy(vc, v)
		result[k] = vc
	}
	return result
}

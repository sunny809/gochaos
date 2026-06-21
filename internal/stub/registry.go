// Package stub provides the stub registry — the central data store for
// gmock stub definitions. It supports concurrent-safe CRUD operations
// and priority-ordered stub retrieval for the matching engine.
package stub

import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunny809/gochaos/internal/response"
	"github.com/sunny809/gochaos/internal/spec"
)

// ValidationError indicates a stub definition failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Record wraps a StubDefinition with internal metadata.
type Record struct {
	Definition spec.StubDefinition
	Priority   int
	// sortKey is computed as (priority << 32) | insertionOrder for stable sorting
	sortKey uint64
	// hitCount tracks how many times this stub has been matched by the
	// matching engine. Used by the everyNthRequest activation mode (A2).
	// Access via sync/atomic for lock-free concurrent increments.
	hitCount uint64
}

// rateLimitState holds the token-bucket state for a single stub's rate limiter.
// Each stub that uses the "rate_limit" fault type gets its own rateLimitState
// entry, lazily created on the first ShouldRateLimit call.
type rateLimitState struct {
	tokens     float64
	lastRefill time.Time
}

// Registry is a concurrent-safe store for stub definitions.
// Stubs are keyed by ID and maintained in priority+insertion order.
type Registry struct {
	mu      sync.RWMutex
	stubs   map[string]*Record
	ordered []*Record // sorted by priority then insertion
	nextSeq uint64    // monotonic counter for insertion ordering

	// rateLimitMu protects rateLimitStates. This is a separate lock from the
	// main stub mu to avoid holding the read lock during token-bucket
	// computation, which would block stub additions/deletions.
	rateLimitMu     sync.Mutex
	rateLimitStates map[string]*rateLimitState
}

// NewRegistry creates an empty stub registry.
func NewRegistry() *Registry {
	return &Registry{
		stubs:           make(map[string]*Record),
		rateLimitStates: make(map[string]*rateLimitState),
	}
}

// Add inserts a stub and returns its ID.
// If the stub already has an empty ID, a UUID v4 is generated.
func (r *Registry) Add(def spec.StubDefinition) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generated UUID if empty
	if def.ID == "" {
		id, err := generateID()
		if err != nil {
			return "", fmt.Errorf("stub: failed to generate ID: %w", err)
		}
		def.ID = id
	}

	// Validate fault definition if specified (type, activation, and type-specific fields)
	if def.Response.Fault != nil {
		if err := response.ValidateFault(def.Response.Fault); err != nil {
			return "", &ValidationError{Field: "fault", Message: err.Error()}
		}
	}

	// Merge top-level priority into request priority
	priority := def.Priority
	if priority == 0 {
		priority = def.Request.Priority
	}

	rec := &Record{
		Definition: def,
		Priority:   priority,
		sortKey:    (uint64(priority) << 32) | r.nextSeq,
	}
	r.nextSeq++

	r.stubs[def.ID] = rec
	r.rebuildOrdered()

	return def.ID, nil
}

// Get retrieves a stub by ID. Returns nil if not found.
func (r *Registry) Get(id string) *spec.StubDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.stubs[id]
	if !ok {
		return nil
	}
	// Return a copy to avoid race conditions
	def := rec.Definition
	return &def
}

// Delete removes a stub by ID. Returns true if the stub existed.
func (r *Registry) Delete(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.stubs[id]; !ok {
		return false
	}
	delete(r.stubs, id)
	r.rebuildOrdered()

	r.rateLimitMu.Lock()
	delete(r.rateLimitStates, id)
	r.rateLimitMu.Unlock()

	return true
}

// DeleteAll removes all stubs and resets all rate-limit state.
func (r *Registry) DeleteAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stubs = make(map[string]*Record)
	r.ordered = nil
	r.nextSeq = 0

	r.rateLimitMu.Lock()
	r.rateLimitStates = make(map[string]*rateLimitState)
	r.rateLimitMu.Unlock()
}

// List returns all registered stubs, ordered by priority then insertion.
func (r *Registry) List() []spec.StubDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]spec.StubDefinition, len(r.ordered))
	for i, rec := range r.ordered {
		result[i] = rec.Definition
	}
	return result
}

// Len returns the number of registered stubs.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.stubs)
}

// All returns all records in priority order (for the matching engine).
func (r *Registry) All() []*Record {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Record, len(r.ordered))
	copy(result, r.ordered)
	return result
}

// rebuildOrdered re-sorts the ordered slice by (ascending priority, ascending insertion order).
// Must be called with the write lock held.
func (r *Registry) rebuildOrdered() {
	r.ordered = make([]*Record, 0, len(r.stubs))
	for _, rec := range r.stubs {
		r.ordered = append(r.ordered, rec)
	}

	// Sort by sortKey (priority << 32 | insertionSeq)
	// Lower priority value = higher precedence. Lower seq = older insertion (earlier wins ties).
	insertionSort(r.ordered)
}

// insertionSort sorts records by sortKey ascending. Uses simple insertion sort
// which is O(n) for nearly-sorted data (typical case: one stub added or removed).
func insertionSort(records []*Record) {
	for i := 1; i < len(records); i++ {
		key := records[i]
		j := i - 1
		for j >= 0 && records[j].sortKey > key.sortKey {
			records[j+1] = records[j]
			j--
		}
		records[j+1] = key
	}
}

// generateID creates a UUID v4 without external dependencies.
func generateID() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	// Set version 4
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant bits
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// IncrementHitCount atomically increments the hit count for the stub with the
// given ID and returns the new value after increment. If the stub does not
// exist, it returns 0 without incrementing.
//
// The caller (server.serveMock) should call this after a successful match
// and pass the returned value to WriteResponse so that the everyNthRequest
// activation mode can decide whether the fault should fire.
func (r *Registry) IncrementHitCount(id string) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.stubs[id]
	if !ok {
		return 0
	}
	return atomic.AddUint64(&rec.hitCount, 1)
}

// GetHitCount returns the current hit count for the stub with the given ID.
// Returns 0 if the stub does not exist. This is a read-only operation
// suitable for diagnostics and admin endpoints.
func (r *Registry) GetHitCount(id string) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.stubs[id]
	if !ok {
		return 0
	}
	return atomic.LoadUint64(&rec.hitCount)
}

// ShouldRateLimit determines whether the request to the given stub should be
// rate-limited using a token-bucket algorithm. Returns true if the request
// should be rejected (rate-limited), false if it should proceed normally.
//
// Algorithm:
//  1. If hitCount < afterRequests, the request is not rate-limited (warm-up phase).
//  2. Once hitCount >= afterRequests, the token bucket is consulted:
//     - Tokens refill at the rate of perSecond tokens per second.
//     - The bucket capacity is perSecond (max burst = perSecond).
//     - If at least 1 token is available, it is consumed and the request is allowed.
//     - If no tokens are available, the request is rate-limited.
//
// The initial bucket is full (tokens = perSecond), so the first burst of
// perSecond requests after the warm-up phase are allowed immediately.
//
// If the stub ID is unknown, returns false (no rate limiting).
// If afterRequests <= 0, the warm-up phase is skipped (rate limiting begins immediately).
// If perSecond <= 0, returns false (no rate limiting; this should be caught by validation).
func (r *Registry) ShouldRateLimit(id string, afterRequests int, perSecond int) bool {
	if perSecond <= 0 {
		return false
	}

	// Check if we're still in the warm-up phase.
	// The first afterRequests requests are always allowed, regardless of
	// the token bucket. Only after that does rate limiting begin.
	hitCount := r.GetHitCount(id)
	if afterRequests > 0 && hitCount <= uint64(afterRequests) {
		return false
	}

	// Unknown stub — don't rate limit.
	if r.Get(id) == nil {
		return false
	}

	r.rateLimitMu.Lock()
	defer r.rateLimitMu.Unlock()

	state, ok := r.rateLimitStates[id]
	if !ok {
		// First rate-limited request: initialize with a full bucket.
		state = &rateLimitState{
			tokens:     float64(perSecond),
			lastRefill: time.Now(),
		}
		r.rateLimitStates[id] = state
	}

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(state.lastRefill).Seconds()
	state.tokens = min(float64(perSecond), state.tokens+elapsed*float64(perSecond))
	state.lastRefill = now

	// Try to consume a token.
	if state.tokens >= 1.0 {
		state.tokens -= 1.0
		return false
	}
	return true
}

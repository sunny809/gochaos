// Package stub provides the stub registry — the central data store for
// gmock stub definitions. It supports concurrent-safe CRUD operations
// and priority-ordered stub retrieval for the matching engine.
package stub

import (
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/sunny809/gochaos/internal/spec"
)

// Record wraps a StubDefinition with internal metadata.
type Record struct {
	Definition spec.StubDefinition
	Priority   int
	// sortKey is computed as (priority << 32) | insertionOrder for stable sorting
	sortKey uint64
}

// Registry is a concurrent-safe store for stub definitions.
// Stubs are keyed by ID and maintained in priority+insertion order.
type Registry struct {
	mu      sync.RWMutex
	stubs   map[string]*Record
	ordered []*Record          // sorted by priority then insertion
	nextSeq uint64             // monotonic counter for insertion ordering
}

// NewRegistry creates an empty stub registry.
func NewRegistry() *Registry {
	return &Registry{
		stubs: make(map[string]*Record),
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
	return true
}

// DeleteAll removes all stubs.
func (r *Registry) DeleteAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stubs = make(map[string]*Record)
	r.ordered = nil
	r.nextSeq = 0
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
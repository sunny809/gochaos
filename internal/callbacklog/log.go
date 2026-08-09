// Package callbacklog provides callback dispatch logging for observability.
//
// Logged callback events are stored in a fixed-size ring buffer to bound memory
// usage. The log is concurrent-safe and supports listing and clearing.
package callbacklog

import (
	"sync"

	"github.com/sunny809/gochaos/internal/spec"
)

// Log is a concurrent-safe ring buffer for callback dispatch events.
type Log struct {
	mu      sync.RWMutex
	entries []spec.CallbackEntry
	max     int
	head    int // next index to write
	count   int // number of valid entries
}

// New creates a Log with the given maximum size.
// If size is 0 or negative, defaults to 1000.
func New(size int) *Log {
	if size <= 0 {
		size = 1000
	}
	return &Log{
		entries: make([]spec.CallbackEntry, size),
		max:     size,
	}
}

// Record logs a callback dispatch event into the ring buffer.
func (l *Log) Record(entry spec.CallbackEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.max
	if l.count < l.max {
		l.count++
	}
}

// List returns all logged callback events in chronological order (oldest first).
func (l *Log) List() []spec.CallbackEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]spec.CallbackEntry, l.count)
	for i := 0; i < l.count; i++ {
		idx := (l.head - l.count + i + l.max) % l.max
		result[i] = l.entries[idx]
	}
	return result
}

// Clear removes all logged callback events.
func (l *Log) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = make([]spec.CallbackEntry, l.max)
	l.head = 0
	l.count = 0
}

// Len returns the number of logged callback events.
func (l *Log) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

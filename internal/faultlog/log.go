// Package faultlog provides fault injection logging for observability.
//
// Logged fault injections are stored in a fixed-size ring buffer to bound memory usage.
// The log is concurrent-safe and supports listing and clearing.
package faultlog

import (
	"sync"

	"github.com/sunny809/gochaos/internal/spec"
)

// FaultInjectionLog is a concurrent-safe ring buffer for fault injection events.
type FaultInjectionLog struct {
	mu      sync.RWMutex
	entries []spec.FaultInjectionEntry
	max     int
	head    int // next index to write
	count   int // number of valid entries
}

// NewFaultInjectionLog creates a FaultInjectionLog with the given maximum size.
// If max is 0 or negative, defaults to 1000.
func NewFaultInjectionLog(max int) *FaultInjectionLog {
	if max <= 0 {
		max = 1000
	}
	return &FaultInjectionLog{
		entries: make([]spec.FaultInjectionEntry, max),
		max:     max,
	}
}

// Record logs a fault injection event into the ring buffer.
func (l *FaultInjectionLog) Record(entry spec.FaultInjectionEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.max
	if l.count < l.max {
		l.count++
	}
}

// List returns all logged fault injections in chronological order (oldest first).
func (l *FaultInjectionLog) List() []spec.FaultInjectionEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]spec.FaultInjectionEntry, l.count)
	for i := 0; i < l.count; i++ {
		idx := (l.head - l.count + i + l.max) % l.max
		result[i] = l.entries[idx]
	}
	return result
}

// Clear removes all logged fault injections.
func (l *FaultInjectionLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = make([]spec.FaultInjectionEntry, l.max)
	l.head = 0
	l.count = 0
}

// Len returns the number of logged fault injections.
func (l *FaultInjectionLog) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}

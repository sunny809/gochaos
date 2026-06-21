package faultlog

import (
	"sync"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

func TestNewFaultInjectionLog(t *testing.T) {
	t.Run("default_capacity", func(t *testing.T) {
		log := NewFaultInjectionLog(0)
		if log.max != 1000 {
			t.Errorf("expected default max 1000, got %d", log.max)
		}
	})

	t.Run("negative_capacity_uses_default", func(t *testing.T) {
		log := NewFaultInjectionLog(-100)
		if log.max != 1000 {
			t.Errorf("expected default max 1000, got %d", log.max)
		}
	})

	t.Run("custom_capacity", func(t *testing.T) {
		log := NewFaultInjectionLog(500)
		if log.max != 500 {
			t.Errorf("expected max 500, got %d", log.max)
		}
	})
}

func TestFaultInjectionLog_Record(t *testing.T) {
	t.Run("single_entry", func(t *testing.T) {
		log := NewFaultInjectionLog(10)
		entry := spec.FaultInjectionEntry{
			StubID:         "stub-1",
			FaultType:      "connection_reset",
			ActivatedAt:    time.Now().UTC(),
			RequestMethod:  "GET",
			RequestPath:    "/api/test",
			ActivationMode: spec.ModeAlways,
		}

		log.Record(entry)

		if log.Len() != 1 {
			t.Errorf("expected length 1, got %d", log.Len())
		}

		entries := log.List()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		if entries[0].StubID != "stub-1" {
			t.Errorf("expected stub ID 'stub-1', got %s", entries[0].StubID)
		}
	})

	t.Run("chronological_order", func(t *testing.T) {
		log := NewFaultInjectionLog(10)

		for i := 0; i < 5; i++ {
			log.Record(spec.FaultInjectionEntry{
				StubID:      string(rune('a' + i)),
				ActivatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
			})
		}

		entries := log.List()
		for i := 0; i < 4; i++ {
			if entries[i].StubID >= entries[i+1].StubID {
				t.Errorf("entries not in chronological order: %s before %s", entries[i].StubID, entries[i+1].StubID)
			}
		}
	})

	t.Run("ring_buffer_wrapping", func(t *testing.T) {
		log := NewFaultInjectionLog(3)

		// Add 5 entries to a buffer of size 3
		for i := 0; i < 5; i++ {
			log.Record(spec.FaultInjectionEntry{
				StubID:      string(rune('0' + i)),
				ActivatedAt: time.Now().UTC(),
			})
		}

		if log.Len() != 3 {
			t.Errorf("expected length 3, got %d", log.Len())
		}

		entries := log.List()
		// Should have entries 2, 3, 4 (oldest 0, 1 were overwritten)
		expected := []string{"2", "3", "4"}
		for i, e := range entries {
			if e.StubID != expected[i] {
				t.Errorf("expected entry %d to have stub ID %s, got %s", i, expected[i], e.StubID)
			}
		}
	})

	t.Run("overwrites_oldest", func(t *testing.T) {
		log := NewFaultInjectionLog(2)

		log.Record(spec.FaultInjectionEntry{StubID: "first"})
		log.Record(spec.FaultInjectionEntry{StubID: "second"})
		log.Record(spec.FaultInjectionEntry{StubID: "third"})

		entries := log.List()
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		// "first" should be overwritten
		if entries[0].StubID != "second" {
			t.Errorf("expected first entry to be 'second', got %s", entries[0].StubID)
		}
		if entries[1].StubID != "third" {
			t.Errorf("expected second entry to be 'third', got %s", entries[1].StubID)
		}
	})
}

func TestFaultInjectionLog_Clear(t *testing.T) {
	log := NewFaultInjectionLog(10)

	for i := 0; i < 5; i++ {
		log.Record(spec.FaultInjectionEntry{StubID: string(rune('a' + i))})
	}

	if log.Len() != 5 {
		t.Fatalf("expected length 5 before clear, got %d", log.Len())
	}

	log.Clear()

	if log.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", log.Len())
	}

	entries := log.List()
	if len(entries) != 0 {
		t.Errorf("expected empty list after clear, got %d entries", len(entries))
	}
}

func TestFaultInjectionLog_List(t *testing.T) {
	t.Run("empty_log", func(t *testing.T) {
		log := NewFaultInjectionLog(10)
		entries := log.List()
		if len(entries) != 0 {
			t.Errorf("expected empty list, got %d entries", len(entries))
		}
	})

	t.Run("returns_copy", func(t *testing.T) {
		log := NewFaultInjectionLog(10)
		log.Record(spec.FaultInjectionEntry{StubID: "test"})

		entries1 := log.List()
		entries2 := log.List()

		// Modifying one should not affect the other
		entries1[0].StubID = "modified"
		if entries2[0].StubID != "test" {
			t.Errorf("List() does not return a copy: expected 'test', got %s", entries2[0].StubID)
		}
	})
}

func TestFaultInjectionLog_Concurrent(t *testing.T) {
	log := NewFaultInjectionLog(100)

	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 10
	numOps := 100

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				log.Record(spec.FaultInjectionEntry{
					StubID:      string(rune('0' + id)),
					ActivatedAt: time.Now().UTC(),
				})
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				_ = log.List()
				_ = log.Len()
			}
		}()
	}

	// Concurrent clear
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < numOps; j++ {
			log.Clear()
		}
	}()

	wg.Wait()
	// If we get here without race detection, the test passes
}

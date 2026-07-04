package callbacklog

import (
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

func TestNew(t *testing.T) {
	t.Run("default_size", func(t *testing.T) {
		l := New(0)
		if l.max != 1000 {
			t.Errorf("expected max=1000, got %d", l.max)
		}
	})

	t.Run("negative_size", func(t *testing.T) {
		l := New(-1)
		if l.max != 1000 {
			t.Errorf("expected max=1000, got %d", l.max)
		}
	})

	t.Run("custom_size", func(t *testing.T) {
		l := New(50)
		if l.max != 50 {
			t.Errorf("expected max=50, got %d", l.max)
		}
	})
}

func TestLog_RecordAndList(t *testing.T) {
	l := New(5)

	entry1 := spec.CallbackEntry{
		StubID:       "stub-1",
		CallbackURL:  "http://example.com/hook",
		Status:       spec.CallbackDelivered,
		StatusCode:   200,
		DispatchedAt: time.Now(),
	}

	entry2 := spec.CallbackEntry{
		StubID:       "stub-2",
		CallbackURL:  "http://example.com/hook2",
		Status:       spec.CallbackError,
		Error:        "connection refused",
		DispatchedAt: time.Now(),
	}

	l.Record(entry1)
	l.Record(entry2)

	if l.Len() != 2 {
		t.Errorf("expected Len=2, got %d", l.Len())
	}

	entries := l.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].StubID != "stub-1" {
		t.Errorf("expected first entry StubID=stub-1, got %s", entries[0].StubID)
	}
	if entries[1].StubID != "stub-2" {
		t.Errorf("expected second entry StubID=stub-2, got %s", entries[1].StubID)
	}
}

func TestLog_RingBufferOverflow(t *testing.T) {
	l := New(3)

	for i := 0; i < 5; i++ {
		l.Record(spec.CallbackEntry{
			StubID:       string(rune('a' + i)),
			CallbackURL:  "http://example.com/hook",
			Status:       spec.CallbackDelivered,
			DispatchedAt: time.Now(),
		})
	}

	if l.Len() != 3 {
		t.Errorf("expected Len=3 (max), got %d", l.Len())
	}

	entries := l.List()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should contain the last 3 entries (c, d, e)
	if entries[0].StubID != "c" {
		t.Errorf("expected first entry StubID=c, got %s", entries[0].StubID)
	}
	if entries[2].StubID != "e" {
		t.Errorf("expected last entry StubID=e, got %s", entries[2].StubID)
	}
}

func TestLog_Clear(t *testing.T) {
	l := New(10)
	l.Record(spec.CallbackEntry{StubID: "stub-1", Status: spec.CallbackDelivered, DispatchedAt: time.Now()})
	l.Record(spec.CallbackEntry{StubID: "stub-2", Status: spec.CallbackDelivered, DispatchedAt: time.Now()})

	if l.Len() != 2 {
		t.Errorf("expected Len=2 before clear, got %d", l.Len())
	}

	l.Clear()

	if l.Len() != 0 {
		t.Errorf("expected Len=0 after clear, got %d", l.Len())
	}

	entries := l.List()
	if len(entries) != 0 {
		t.Errorf("expected empty list after clear, got %d entries", len(entries))
	}
}

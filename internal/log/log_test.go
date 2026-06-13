package log

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRequestLogBasic(t *testing.T) {
	l := New(10)

	req := httptest.NewRequest("GET", "/api/users", nil)
	l.Record(req, true, "stub-1")

	if l.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", l.Len())
	}

	list := l.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 list entry, got %d", len(list))
	}
	if list[0].Method != "GET" {
		t.Errorf("expected GET, got %s", list[0].Method)
	}
	if list[0].Path != "/api/users" {
		t.Errorf("expected /api/users, got %s", list[0].Path)
	}
}

func TestRequestLogMatchedFilter(t *testing.T) {
	l := New(10)

	l.Record(httptest.NewRequest("GET", "/matched", nil), true, "stub-1")
	l.Record(httptest.NewRequest("GET", "/unmatched", nil), false, "")

	matched := l.Matched()
	if len(matched) != 1 {
		t.Errorf("expected 1 matched, got %d", len(matched))
	}

	unmatched := l.Unmatched()
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched, got %d", len(unmatched))
	}
}

func TestRequestLogClear(t *testing.T) {
	l := New(10)
	l.Record(httptest.NewRequest("GET", "/a", nil), true, "s1")
	l.Record(httptest.NewRequest("GET", "/b", nil), false, "")
	l.Clear()

	if l.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", l.Len())
	}
}

func TestRequestLogRingBuffer(t *testing.T) {
	l := New(3)

	for i := 0; i < 5; i++ {
		path := "/req"
		l.Record(httptest.NewRequest("GET", path, nil), true, "s1")
	}

	if l.Len() != 3 {
		t.Errorf("expected 3 (ring buffer max), got %d", l.Len())
	}
}

func TestRequestLogBodyCapture(t *testing.T) {
	l := New(10)

	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/api/data", strings.NewReader(body))
	l.Record(req, true, "s1")

	list := l.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	if list[0].Body != body {
		t.Errorf("expected body %q, got %q", body, list[0].Body)
	}
}

func TestRequestLogMatchingStub(t *testing.T) {
	l := New(10)

	l.Record(httptest.NewRequest("GET", "/a", nil), true, "stub-a")
	l.Record(httptest.NewRequest("GET", "/b", nil), true, "stub-b")
	l.Record(httptest.NewRequest("GET", "/c", nil), true, "stub-a")

	entries := l.MatchingStub("stub-a")
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for stub-a, got %d", len(entries))
	}
}

func TestRequestLogConcurrent(t *testing.T) {
	l := New(100)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Record(httptest.NewRequest("GET", "/conc", nil), true, "s1")
		}()
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.List()
			_ = l.Len()
		}()
	}
	wg.Wait()

	if l.Len() != 50 {
		t.Errorf("expected 50 concurrent entries, got %d", l.Len())
	}
}

func TestRequestLogDefaultMax(t *testing.T) {
	l := New(0)
	if l.max != 1000 {
		t.Errorf("expected default max 1000, got %d", l.max)
	}

	l2 := New(-5)
	if l2.max != 1000 {
		t.Errorf("expected default max 1000 for negative, got %d", l2.max)
	}
}

func TestRequestLogQueryString(t *testing.T) {
	l := New(10)
	req := httptest.NewRequest("GET", "/api/users?page=1&limit=10", nil)
	l.Record(req, true, "s1")

	list := l.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	if list[0].QueryString != "page=1&limit=10" {
		t.Errorf("expected 'page=1&limit=10', got %q", list[0].QueryString)
	}
}

func TestRequestLogEntries(t *testing.T) {
	l := New(10)
	l.Record(httptest.NewRequest("GET", "/a", nil), true, "s1")
	l.Record(httptest.NewRequest("GET", "/b", nil), false, "")

	entries := l.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[0].Matched {
		t.Errorf("expected first entry to be matched")
	}
	if entries[1].Matched {
		t.Errorf("expected second entry to be unmatched")
	}
}

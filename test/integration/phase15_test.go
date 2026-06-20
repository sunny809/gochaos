package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sunny809/gochaos/internal/admin"
	"github.com/sunny809/gochaos/internal/faultlog"
	"github.com/sunny809/gochaos/internal/log"
	"github.com/sunny809/gochaos/internal/nearmiss"
	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
)

func TestPhase15_FaultLogAdminAPI(t *testing.T) {
	registry := stub.NewRegistry()
	requestLog := log.New(100)
	faultLog := faultlog.NewFaultInjectionLog(100)
	engine := nearmiss.NewEngine()
	h := admin.New(registry, requestLog, faultLog, engine)

	// Test 1: Empty fault log
	t.Run("empty_fault_log", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/__admin/fault-log", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if result["count"].(float64) != 0 {
			t.Errorf("expected count 0, got %v", result["count"])
		}
		entries := result["entries"].([]interface{})
		if len(entries) != 0 {
			t.Errorf("expected empty entries, got %d", len(entries))
		}
	})

	// Test 2: Record faults and list
	t.Run("list_fault_entries", func(t *testing.T) {
		faultLog.Record(spec.FaultInjectionEntry{
			StubID:         "stub-1",
			FaultType:      "connection_reset",
			ActivatedAt:    time.Now().UTC(),
			RequestMethod:  "GET",
			RequestPath:    "/api/test",
			ActivationMode: spec.ModeAlways,
		})
		faultLog.Record(spec.FaultInjectionEntry{
			StubID:         "stub-2",
			FaultType:      "timeout",
			ActivatedAt:    time.Now().UTC(),
			RequestMethod:  "POST",
			RequestPath:    "/api/create",
			ActivationMode: spec.ModeProbability,
		})

		req := httptest.NewRequest("GET", "/__admin/fault-log", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if result["count"].(float64) != 2 {
			t.Errorf("expected count 2, got %v", result["count"])
		}
		entries := result["entries"].([]interface{})
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})

	// Test 3: Clear fault log
	t.Run("clear_fault_log", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/__admin/fault-log", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if result["cleared"].(bool) != true {
			t.Errorf("expected cleared=true")
		}
		if result["count"].(float64) != 2 {
			t.Errorf("expected cleared count 2, got %v", result["count"])
		}

		if faultLog.Len() != 0 {
			t.Errorf("expected fault log to be empty, got %d", faultLog.Len())
		}
	})
}

func TestPhase15_VerifyFaultsInjected(t *testing.T) {
	faultLog := faultlog.NewFaultInjectionLog(100)

	faultLog.Record(spec.FaultInjectionEntry{
		StubID:         "stub-1",
		FaultType:      "connection_reset",
		ActivatedAt:    time.Now().UTC(),
		RequestMethod:  "GET",
		RequestPath:    "/api/test",
		ActivationMode: spec.ModeAlways,
	})
	faultLog.Record(spec.FaultInjectionEntry{
		StubID:         "stub-1",
		FaultType:      "timeout",
		ActivatedAt:    time.Now().UTC(),
		RequestMethod:  "POST",
		RequestPath:    "/api/create",
		ActivationMode: spec.ModeProbability,
	})
	faultLog.Record(spec.FaultInjectionEntry{
		StubID:         "stub-2",
		FaultType:      "connection_reset",
		ActivatedAt:    time.Now().UTC(),
		RequestMethod:  "GET",
		RequestPath:    "/api/other",
		ActivationMode: spec.ModeAlways,
	})

	entries := faultLog.List()

	count1 := 0
	for _, e := range entries {
		if e.StubID == "stub-1" {
			count1++
		}
	}
	if count1 != 2 {
		t.Errorf("expected 2 entries for stub-1, got %d", count1)
	}

	crCount := 0
	for _, e := range entries {
		if e.FaultType == "connection_reset" {
			crCount++
		}
	}
	if crCount != 2 {
		t.Errorf("expected 2 connection_reset entries, got %d", crCount)
	}
}

func TestPhase15_RateLimitRecording(t *testing.T) {
	faultLog := faultlog.NewFaultInjectionLog(100)

	faultLog.Record(spec.FaultInjectionEntry{
		StubID:         "rate-limit-stub",
		FaultType:      "rate_limit",
		ActivatedAt:    time.Now().UTC(),
		RequestMethod:  "GET",
		RequestPath:    "/api/limited",
		ActivationMode: "rate_limit",
	})

	entries := faultLog.List()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].FaultType != "rate_limit" {
		t.Errorf("expected fault type 'rate_limit', got %s", entries[0].FaultType)
	}
}

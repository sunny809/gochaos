package gmock

import (
	"testing"
)

func TestNewMetrics(t *testing.T) {
	m := newMetrics()
	if m == nil {
		t.Fatal("newMetrics() returned nil")
	}

	snap := m.Snapshot()
	if snap == nil {
		t.Fatal("Snapshot() returned nil")
	}

	expectedKeys := []string{
		"requests_total",
		"requests_matched",
		"requests_unmatched",
		"faults_injected",
		"faults_delayed",
		"nearmiss_queries",
		"stubs_registered",
		"admin_operations",
	}

	for _, key := range expectedKeys {
		if _, ok := snap[key]; !ok {
			t.Errorf("Snapshot missing key: %s", key)
		}
	}

	for key, val := range snap {
		if val != 0 {
			t.Errorf("expected %s to be 0, got %d", key, val)
		}
	}
}

func TestMetricsAdd(t *testing.T) {
	m := newMetrics()

	m.Add("requests_total", 5)
	m.Add("requests_matched", 3)
	m.Add("stubs_registered", 1)

	snap := m.Snapshot()
	if snap["requests_total"] != 5 {
		t.Errorf("requests_total: expected 5, got %d", snap["requests_total"])
	}
	if snap["requests_matched"] != 3 {
		t.Errorf("requests_matched: expected 3, got %d", snap["requests_matched"])
	}
	if snap["stubs_registered"] != 1 {
		t.Errorf("stubs_registered: expected 1, got %d", snap["stubs_registered"])
	}
}

func TestMetricsAddUnknownName(t *testing.T) {
	m := newMetrics()
	// Should not panic and should not affect any known counter
	m.Add("unknown_counter", 1)

	snap := m.Snapshot()
	for _, val := range snap {
		if val != 0 {
			t.Errorf("expected all counters to be 0, got %d", val)
		}
	}
}

func TestMetricsResetAll(t *testing.T) {
	m := newMetrics()
	m.Add("requests_total", 10)
	m.Add("faults_injected", 5)

	m.resetAll()

	snap := m.Snapshot()
	for key, val := range snap {
		if val != 0 {
			t.Errorf("after resetAll, %s: expected 0, got %d", key, val)
		}
	}
}

func TestMetricsConcurrentAddAndSnapshot(t *testing.T) {
	m := newMetrics()
	const goroutines = 10
	const iterations = 100

	done := make(chan struct{}, goroutines*2)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iterations; j++ {
				m.Add("requests_total", 1)
			}
		}()
	}

	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iterations; j++ {
				_ = m.Snapshot()
			}
		}()
	}

	for i := 0; i < goroutines*2; i++ {
		<-done
	}

	snap := m.Snapshot()
	expected := int64(goroutines * iterations)
	if snap["requests_total"] != expected {
		t.Errorf("requests_total: expected %d, got %d", expected, snap["requests_total"])
	}
}

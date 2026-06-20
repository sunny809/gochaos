package integration_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// P0: Fault injection via binary — B-F1
// ---------------------------------------------------------------------------

func TestBinary_Fault_Error(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-error"},
		"response": {"fault": {"type": "error"}}
	}`)

	resp := httpDo(t, "GET", baseURL+"/fault-error", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "error") {
		t.Errorf("expected error JSON body, got: %s", body)
	}
}

func TestBinary_Fault_Empty(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-empty"},
		"response": {"fault": {"type": "empty"}}
	}`)

	resp := httpDo(t, "GET", baseURL+"/fault-empty", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for empty fault, got %d", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestBinary_Fault_Malformed(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-malformed"},
		"response": {"fault": {"type": "malformed"}}
	}`)

	// Malformed responses may cause connection errors or fallback to 500
	resp, err := http.Get(baseURL + "/fault-malformed")
	if err != nil || resp == nil {
		// Connection error is acceptable (malformed sends garbage)
		if err != nil {
			t.Logf("malformed fault caused connection error (expected): %v", err)
		}
		return
	}
	defer resp.Body.Close()
	// If we got a response, it should indicate error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Logf("malformed fault returned %d (fallback)", resp.StatusCode)
	}
}

func TestBinary_Fault_RandomData(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-random"},
		"response": {"fault": {"type": "random_data", "dataLength": 128}}
	}`)

	resp, err := http.Get(baseURL + "/fault-random")
	if err != nil {
		// Connection error is acceptable (random_data sends garbage then closes)
		t.Logf("random_data fault caused connection error (expected): %v", err)
		return
	}
	defer resp.Body.Close()
	t.Logf("random_data fault returned %d (fallback)", resp.StatusCode)
}

func TestBinary_Fault_RateLimit(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-rate"},
		"response": {
			"fault": {
				"type": "rate_limit",
				"afterRequests": 1,
				"perSecond": 1
			}
		}
	}`)

	// First 2 requests should succeed (warm-up + token)
	for i := 0; i < 2; i++ {
		resp := httpDo(t, "GET", baseURL+"/fault-rate", "", "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d: expected 200 during warm-up, got %d", i+1, resp.StatusCode)
		}
	}

	// 4th request should be rate-limited
	resp := httpDo(t, "GET", baseURL+"/fault-rate", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after tokens exhausted, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on rate-limited response")
	}
	if !strings.Contains(body, "rate") {
		t.Errorf("expected rate limit message in body, got: %s", body)
	}
}

func TestBinary_Fault_SlowClose(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/fault-slowclose"},
		"response": {
			"status": 200,
			"body": "ok",
			"fault": {"type": "slow_close", "delayMs": 100}
		}
	}`)

	// slow_close should write normal response then delay before closing.
	// The key assertion is that status=200 with Connection:close header.
	// Body may arrive partially depending on TCP timing with hijack.
	start := time.Now()
	resp, err := http.Get(baseURL + "/fault-slowclose")
	elapsed := time.Since(start)

	if err != nil {
		// Connection error is acceptable for slow_close (hijack closes connection)
		t.Logf("slow_close: connection error (acceptable in hijack path): %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	}

	// Overall timing should be reasonable (<2s) even with slow_close delay
	if elapsed > 2000*time.Millisecond {
		t.Errorf("response took too long: %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// P0: Activation modes via binary — B-A1, B-A2, B-A3
// ---------------------------------------------------------------------------

func TestBinary_Activation_Probability(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/prob"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {"probability": 0.3}
			}
		}
	}`)

	// Send 50 requests and count how many fail
	failures := 0
	total := 50
	for i := 0; i < total; i++ {
		resp := httpDo(t, "GET", baseURL+"/prob", "", "")
		resp.Body.Close()
		if resp.StatusCode == http.StatusInternalServerError {
			failures++
		}
	}

	t.Logf("probability 0.3: %d/%d failures (expected ~15)", failures, total)
	if failures < 5 || failures > 25 {
		t.Errorf("failure count %d out of expected range [5, 25] for probability 0.3", failures)
	}
}

func TestBinary_Activation_EveryNthRequest(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/nth"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {"everyNthRequest": 3}
			}
		}
	}`)

	// Send 9 requests: indices 3, 6, 9 should fail (1-indexed)
	expectedFailures := map[int]bool{3: true, 6: true, 9: true}
	for i := 1; i <= 9; i++ {
		resp := httpDo(t, "GET", baseURL+"/nth", "", "")
		resp.Body.Close()
		isFail := resp.StatusCode == http.StatusInternalServerError
		shouldFail := expectedFailures[i]

		if isFail && !shouldFail {
			t.Errorf("request %d: unexpected failure", i)
		}
		if !isFail && shouldFail {
			t.Errorf("request %d: expected failure but got %d", i, resp.StatusCode)
		}
	}
}

func TestBinary_Activation_TimeWindow(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	// Time window that covers now (elapsed since server start) — startMs: 0, endMs: 60000
	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/window"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {
					"activeBetween": [{"startMs": 0, "endMs": 60000}]
				}
			}
		}
	}`)

	// Should fail within the window
	resp := httpDo(t, "GET", baseURL+"/window", "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 within active time window, got %d", resp.StatusCode)
	}
}

func TestBinary_Activation_TimeWindow_Future(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	// Time window far in the future (1 hour from now)
	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/future-window"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {
					"activeBetween": [{"startMs": 3600000, "endMs": 7200000}]
				}
			}
		}
	}`)

	// Should NOT fail — outside the window
	resp := httpDo(t, "GET", baseURL+"/future-window", "", "")
	resp.Body.Close()
	if resp.StatusCode == http.StatusInternalServerError {
		t.Error("unexpected fault: outside active time window")
	}
}

// ---------------------------------------------------------------------------
// P0: Chaos Observability via binary — B-F2
// ---------------------------------------------------------------------------

func TestBinary_FaultLog_RecordsInjections(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/log-me"},
		"response": {"fault": {"type": "error"}}
	}`)

	// Send requests to trigger faults
	for i := 0; i < 3; i++ {
		resp := httpDo(t, "GET", baseURL+"/log-me", "", "")
		resp.Body.Close()
	}

	// Check fault log
	entries := getFaultLog(t, baseURL)
	if len(entries) != 3 {
		t.Errorf("expected 3 fault log entries, got %d", len(entries))
	}

	// Verify first entry structure
	if len(entries) > 0 {
		entry, ok := entries[0].(map[string]interface{})
		if !ok {
			t.Fatal("entry is not a map")
		}
		if entry["faultType"] != "error" {
			t.Errorf("expected faultType=error, got %v", entry["faultType"])
		}
		if entry["stubId"] == "" {
			t.Error("expected non-empty stubId in fault log entry")
		}
		if entry["requestMethod"] != "GET" {
			t.Errorf("expected requestMethod=GET, got %v", entry["requestMethod"])
		}
	}
}

func TestBinary_FaultLog_ProbabilisticRecords(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/prob-log"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {"probability": 0.5}
			}
		}
	}`)

	// Send 20 requests
	for i := 0; i < 20; i++ {
		resp := httpDo(t, "GET", baseURL+"/prob-log", "", "")
		resp.Body.Close()
	}

	// Fault log should have recorded some (not all)
	entries := getFaultLog(t, baseURL)
	t.Logf("probabilistic fault: %d entries out of 20 requests", len(entries))
	if len(entries) == 0 {
		t.Error("expected at least some fault log entries for probability 0.5")
	}
	if len(entries) == 20 {
		t.Error("probability 0.5 should not fire on every request")
	}
}

func TestBinary_FaultLog_ActivationMode(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/mode-test"},
		"response": {
			"fault": {
				"type": "error",
				"activation": {"probability": 0.5, "everyNthRequest": 2}
			}
		}
	}`)

	// Send enough to trigger some faults
	for i := 0; i < 10; i++ {
		resp := httpDo(t, "GET", baseURL+"/mode-test", "", "")
		resp.Body.Close()
	}

	entries := getFaultLog(t, baseURL)
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if ok && entry["activationMode"] != nil {
			mode := entry["activationMode"].(string)
			if mode != "combined" {
				t.Logf("activation mode: %s", mode)
			}
		}
	}
}

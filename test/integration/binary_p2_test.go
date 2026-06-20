package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// P2: Admin request log filters via binary — L1-L3
// ---------------------------------------------------------------------------

func TestBinary_RequestLog_Filters(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	// Register a stub
	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/api/logged"},
		"response": {"status": 200, "body": "logged"}
	}`)

	// Send matched request
	resp := httpDo(t, "GET", baseURL+"/api/logged", "", "")
	resp.Body.Close()

	// Send unmatched request
	resp = httpDo(t, "GET", baseURL+"/api/unknown", "", "")
	resp.Body.Close()

	// Check total requests
	resp = httpDo(t, "GET", baseURL+"/__admin/requests", "", "")
	defer resp.Body.Close()
	var result struct {
		Requests []map[string]interface{} `json:"requests"`
		Meta     map[string]int          `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Meta["total"] != 2 {
		t.Errorf("expected total=2, got %d", result.Meta["total"])
	}

	// Filter by matched
	resp = httpDo(t, "GET", baseURL+"/__admin/requests?filter=matched", "", "")
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Requests) != 1 {
		t.Errorf("expected 1 matched request, got %d", len(result.Requests))
	}

	// Filter by unmatched
	resp = httpDo(t, "GET", baseURL+"/__admin/requests?filter=unmatched", "", "")
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Requests) != 1 {
		t.Errorf("expected 1 unmatched request, got %d", len(result.Requests))
	}
}

// ---------------------------------------------------------------------------
// P2: Fault-log clear via binary — F1
// ---------------------------------------------------------------------------

func TestBinary_FaultLog_Clear(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/log-clear"},
		"response": {"fault": {"type": "error"}}
	}`)

	// Trigger a fault
	resp := httpDo(t, "GET", baseURL+"/log-clear", "", "")
	resp.Body.Close()

	// Verify fault logged
	entries := getFaultLog(t, baseURL)
	if len(entries) != 1 {
		t.Fatalf("expected 1 fault log entry, got %d", len(entries))
	}

	// Clear fault log
	resp = httpDo(t, "DELETE", baseURL+"/__admin/fault-log", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear: expected 200, got %d", resp.StatusCode)
	}
	var clearResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&clearResult); err != nil {
		t.Fatalf("decode clear: %v", err)
	}
	if clearResult["cleared"] != true {
		t.Error("expected cleared=true")
	}

	// Verify empty after clear
	entries = getFaultLog(t, baseURL)
	if len(entries) != 0 {
		t.Errorf("expected empty fault log after clear, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// P2: Separate admin port via binary — P1
// ---------------------------------------------------------------------------

func TestBinary_SeparateAdminPort(t *testing.T) {
	binaryPath := buildBinary(t)

	mainPort := pickPort()
	adminPort := pickPort()

	// Start binary manually — can't use startBinary helper because
	// health endpoint is on admin port, not main port
	args := []string{"start", "--port", strconv.Itoa(mainPort), "--admin-port", strconv.Itoa(adminPort)}
	cmd := exec.Command(binaryPath, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", mainPort)
	adminURL := fmt.Sprintf("http://127.0.0.1:%d", adminPort)

	// Wait for both ports to be ready (TCP dial)
	waitForPort(t, mainPort, 5*time.Second)
	waitForPort(t, adminPort, 5*time.Second)

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// Health check on admin port should work (health is admin-only)
	resp := httpDo(t, "GET", adminURL+"/__admin/health", "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin health: expected 200, got %d", resp.StatusCode)
	}

	// Create stub via admin port — then use it via main port
	createStub(t, adminURL, `{
		"request": {"method": "GET", "urlPath": "/via-admin-port"},
		"response": {"status": 200, "body": "admin-created"}
	}`)

	resp = httpDo(t, "GET", baseURL+"/via-admin-port", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 via main port, got %d", resp.StatusCode)
	}
	if string(body) != "admin-created" {
		t.Errorf("expected 'admin-created', got %q", string(body))
	}
}

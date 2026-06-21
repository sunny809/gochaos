package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// P1: Delay distributions via binary — D1-D5
// ---------------------------------------------------------------------------

func TestBinary_Delay_Fixed(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/delay-fixed"},
		"response": {"status": 200, "body": "slow", "delay": {"type": "fixed", "value": 200}}
	}`)

	start := time.Now()
	resp := httpDo(t, "GET", baseURL+"/delay-fixed", "", "")
	elapsed := time.Since(start)
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "slow" {
		t.Errorf("expected body 'slow', got %q", string(body))
	}
	if elapsed < 180*time.Millisecond {
		t.Errorf("expected delay >= 200ms, got %v", elapsed)
	}
	t.Logf("fixed delay: %v", elapsed)
}

func TestBinary_Delay_Random(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/delay-random"},
		"response": {"status": 200, "body": "varies", "delay": {"type": "random", "min": 30, "max": 80}}
	}`)

	for i := 0; i < 3; i++ {
		start := time.Now()
		resp := httpDo(t, "GET", baseURL+"/delay-random", "", "")
		elapsed := time.Since(start)
		resp.Body.Close()
		if elapsed < 15*time.Millisecond || elapsed > 200*time.Millisecond {
			t.Errorf("request %d: delay %v out of range [15ms, 200ms]", i+1, elapsed)
		}
		t.Logf("random delay %d: %v", i+1, elapsed)
	}
}

func TestBinary_Delay_Lognormal(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/delay-lognormal"},
		"response": {"status": 200, "body": "tail", "delay": {"type": "lognormal", "p50": 10, "p95": 50, "p99": 200}}
	}`)

	samples := make([]time.Duration, 20)
	for i := 0; i < 20; i++ {
		start := time.Now()
		resp := httpDo(t, "GET", baseURL+"/delay-lognormal", "", "")
		samples[i] = time.Since(start)
		resp.Body.Close()
	}

	fast := 0
	for _, s := range samples {
		if s < 100*time.Millisecond {
			fast++
		}
	}
	if fast < 15 {
		t.Errorf("expected most samples < 100ms, got %d/20 fast", fast)
	}
	t.Logf("lognormal samples: %v", samples)
}

func TestBinary_Delay_Timeout(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/delay-timeout"},
		"response": {"delay": {"type": "timeout"}}
	}`)

	client := &http.Client{Timeout: 500 * time.Millisecond}
	start := time.Now()
	_, err := client.Get(baseURL + "/delay-timeout")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error")
	} else {
		t.Logf("timeout: err=%v, elapsed=%v", err, elapsed)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("expected timeout ~500ms, got %v", elapsed)
	}
}

func TestBinary_Delay_Dribble(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/delay-dribble"},
		"response": {"status": 200, "body": "hello world", "delay": {"type": "dribble", "chunks": 3, "totalDuration": 300}}
	}`)

	start := time.Now()
	resp := httpDo(t, "GET", baseURL+"/delay-dribble", "", "")
	elapsed := time.Since(start)
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "hello world" {
		t.Errorf("expected body 'hello world', got %q", string(body))
	}
	// Dribble with 3 chunks / 300ms = ~200ms net delay (2 intervals)
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected dribble delay >= 150ms, got %v", elapsed)
	}
	t.Logf("dribble delay: %v", elapsed)
}

// ---------------------------------------------------------------------------
// P1: Admin API CRUD via binary — A1-A3
// ---------------------------------------------------------------------------

func TestBinary_Admin_CRUD(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	// Create stub
	resp := httpDo(t, "POST", baseURL+"/__admin/mappings", "application/json", `{
		"request": {"method": "PUT", "urlPath": "/api/item"},
		"response": {"status": 201, "body": "created"}
	}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	stubID, _ := created["id"].(string)
	if stubID == "" {
		t.Fatal("expected non-empty stub ID")
	}

	// List stubs
	resp = httpDo(t, "GET", baseURL+"/__admin/mappings", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var list struct {
		Mappings []map[string]interface{} `json:"mappings"`
		Meta     map[string]int           `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Meta["total"] != 1 {
		t.Errorf("expected total=1, got %d", list.Meta["total"])
	}
	if len(list.Mappings) != 1 {
		t.Errorf("expected 1 mapping, got %d", len(list.Mappings))
	}

	// Get stub by ID
	resp = httpDo(t, "GET", baseURL+"/__admin/mappings/"+stubID, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got["id"] != stubID {
		t.Errorf("expected id %s, got %v", stubID, got["id"])
	}

	// Delete stub (returns 204 No Content)
	resp = httpDo(t, "DELETE", baseURL+"/__admin/mappings/"+stubID, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Verify deletion
	resp = httpDo(t, "GET", baseURL+"/__admin/mappings", "", "")
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Meta["total"] != 0 {
		t.Errorf("expected total=0 after delete, got %d", list.Meta["total"])
	}
}

// ---------------------------------------------------------------------------
// P1: Admin Reset via binary — R1
// ---------------------------------------------------------------------------

func TestBinary_Admin_Reset(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/will-reset"},
		"response": {"status": 200, "body": "before-reset"}
	}`)

	resp := httpDo(t, "GET", baseURL+"/will-reset", "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stub should work before reset, got %d", resp.StatusCode)
	}

	resp = httpDo(t, "POST", baseURL+"/__admin/reset", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset: expected 200, got %d", resp.StatusCode)
	}

	resp = httpDo(t, "GET", baseURL+"/will-reset", "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after reset, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// P1: YAML stubs loading — Y1
// ---------------------------------------------------------------------------

func TestBinary_YAML_Stubs(t *testing.T) {
	binaryPath := buildBinary(t)

	yamlContent := `mappings:
  - name: get-items
    request:
      method: GET
      urlPath: /api/items
    response:
      status: 200
      body: '[{"id":1,"name":"item1"}]'
  - name: health-check
    request:
      method: GET
      urlPath: /healthz
    response:
      status: 200
      body: 'ok'
`
	yamlPath := dumpStubsTo(t, yamlContent)

	baseURL, cleanup := startBinary(t, binaryPath, "--stubs", yamlPath)
	defer cleanup()

	resp := httpDo(t, "GET", baseURL+"/api/items", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("items: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "item1") {
		t.Errorf("expected body to contain item1, got %s", body)
	}

	resp = httpDo(t, "GET", baseURL+"/healthz", "", "")
	defer resp.Body.Close()
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz: expected 200, got %d", resp.StatusCode)
	}
	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(body))
	}
}

// ---------------------------------------------------------------------------
// P1: Near-Miss diagnostics — N1
// ---------------------------------------------------------------------------

func TestBinary_NearMiss_404Body(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/api/users"},
		"response": {"status": 200, "body": "users"}
	}`)

	resp := httpDo(t, "GET", baseURL+"/api/orders", "", "")
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	if !strings.Contains(body, "nearMisses") {
		t.Errorf("expected nearMisses in 404 body, got: %s", body)
	}
}

func TestBinary_NearMiss_AdminAPI(t *testing.T) {
	binaryPath := buildBinary(t)
	baseURL, cleanup := startBinary(t, binaryPath)
	defer cleanup()

	createStub(t, baseURL, `{
		"request": {"method": "GET", "urlPath": "/api/users"},
		"response": {"status": 200, "body": "users"}
	}`)

	resp := httpDo(t, "POST", baseURL+"/__admin/nearmiss", "application/json", `{
		"method": "GET",
		"path": "/api/orders"
	}`)
	defer resp.Body.Close()
	body := readAll(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	nearMisses, _ := result["nearMisses"].([]interface{})
	if len(nearMisses) == 0 {
		t.Error("expected at least 1 near-miss result")
	}
}

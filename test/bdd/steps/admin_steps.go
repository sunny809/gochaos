package steps

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cucumber/godog"
	"github.com/sunny809/gochaos/pkg/gmock"
)

// registerAdminSteps registers all P3 admin API step definitions.
func registerAdminSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Admin CRUD
	ctx.Step(`^I register a stub via admin API for (GET|POST|PUT|DELETE|PATCH) ([^\s]+) returning (\d+)$`, tc.iRegisterStubViaAdminAPI)
	ctx.Step(`^I register a stub via admin API for (GET|POST|PUT|DELETE|PATCH) ([^\s]+) returning (\d+) with body "([^"]*)"$`, tc.iRegisterStubViaAdminAPIWithBody)

	ctx.Step(`^I register a stub via admin API with fault "([^"]*)" for (GET|POST) ([^\s]+) returning (\d+)$`, tc.iRegisterStubViaAdminAPIWithFault)

	ctx.Step(`^I list all stubs$`, tc.iListAllStubs)
	ctx.Step(`^I get stub "([^"]*)"$`, tc.iGetStub)
	ctx.Step(`^I delete stub "([^"]*)"$`, tc.iDeleteStubViaAdmin)

	// Admin Reset
	ctx.Step(`^I reset via admin API$`, tc.iResetViaAdminAPI)

	// Metrics
	ctx.Step(`^I fetch Prometheus metrics$`, tc.iFetchPrometheusMetrics)
	ctx.Step(`^I fetch JSON metrics$`, tc.iFetchJSONMetrics)
	ctx.Step(`^the Prometheus response contains "([^"]*)"$`, tc.thePrometheusResponseContains)
	ctx.Step(`^the JSON metrics contain "([^"]*)"$`, tc.theJSONMetricsContainKey)

	// Near-miss
	ctx.Step(`^I query near-miss for (GET|POST|PUT|DELETE|PATCH) "([^"]*)"$`, tc.iQueryNearmiss)

	// Request log
	ctx.Step(`^I list request log$`, tc.iListRequestLog)
	ctx.Step(`^I list request log with filter "([^"]*)"$`, tc.iListRequestLogWithFilter)
	ctx.Step(`^I clear request log$`, tc.iClearRequestLog)

	// Health
	ctx.Step(`^I check liveness endpoint$`, tc.iCheckLivenessEndpoint)
	ctx.Step(`^I check readiness endpoint$`, tc.iCheckReadinessEndpoint)
}

// ============================================================================
// Admin CRUD steps
// ============================================================================

// iRegisterStubViaAdminAPI creates a stub via POST /__admin/mappings.
func (tc *TestContext) iRegisterStubViaAdminAPI(method, path, statusStr string) error {
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	payload := buildStubPayload(method, path, status, "", nil, nil)
	resp, body, err := tc.SendAdminRequest("POST", "/__admin/mappings", payload)
	if err != nil {
		return fmt.Errorf("admin create failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("expected 201 Created, got %d: %s", resp.StatusCode, string(body))
	}

	// Extract ID from response
	var created gmock.StubDefinition
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("failed to decode created stub: %w", err)
	}
	if created.ID == "" {
		return fmt.Errorf("created stub has empty ID")
	}
	tc.AddStubID(created.ID)

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iRegisterStubViaAdminAPIWithBody creates a stub with a response body via admin API.
func (tc *TestContext) iRegisterStubViaAdminAPIWithBody(method, path, statusStr, bodyStr string) error {
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	payload := buildStubPayload(method, path, status, bodyStr, nil, nil)
	resp, body, err := tc.SendAdminRequest("POST", "/__admin/mappings", payload)
	if err != nil {
		return fmt.Errorf("admin create failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("expected 201 Created, got %d: %s", resp.StatusCode, string(body))
	}

	var created gmock.StubDefinition
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("failed to decode created stub: %w", err)
	}
	if created.ID == "" {
		return fmt.Errorf("created stub has empty ID")
	}
	tc.AddStubID(created.ID)

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iRegisterStubViaAdminAPIWithFault creates a stub with a fault via admin API.
func (tc *TestContext) iRegisterStubViaAdminAPIWithFault(faultType, method, path, statusStr string) error {
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	fault := map[string]interface{}{
		"type": faultType,
	}
	payload := buildStubPayload(method, path, status, "", fault, nil)
	resp, body, err := tc.SendAdminRequest("POST", "/__admin/mappings", payload)
	if err != nil {
		return fmt.Errorf("admin create failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("expected 201 Created, got %d: %s", resp.StatusCode, string(body))
	}

	var created gmock.StubDefinition
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("failed to decode created stub: %w", err)
	}
	if created.ID == "" {
		return fmt.Errorf("created stub has empty ID")
	}
	tc.AddStubID(created.ID)

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iListAllStubs fetches all stubs via GET /__admin/mappings.
func (tc *TestContext) iListAllStubs() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/mappings", nil)
	if err != nil {
		return fmt.Errorf("admin list failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iGetStub fetches a specific stub via GET /__admin/mappings/{id}.
func (tc *TestContext) iGetStub(id string) error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/mappings/"+id, nil)
	if err != nil {
		return fmt.Errorf("admin get failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iDeleteStubViaAdmin deletes a stub via DELETE /__admin/mappings/{id}.
func (tc *TestContext) iDeleteStubViaAdmin(id string) error {
	resp, body, err := tc.SendAdminRequest("DELETE", "/__admin/mappings/"+id, nil)
	if err != nil {
		return fmt.Errorf("admin delete failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Admin Reset
// ============================================================================

// iResetViaAdminAPI sends POST /__admin/reset.
func (tc *TestContext) iResetViaAdminAPI() error {
	resp, body, err := tc.SendAdminRequest("POST", "/__admin/reset", nil)
	if err != nil {
		return fmt.Errorf("admin reset failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Metrics steps
// ============================================================================

// iFetchPrometheusMetrics fetches GET /__admin/metrics/prometheus.
func (tc *TestContext) iFetchPrometheusMetrics() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/metrics/prometheus", nil)
	if err != nil {
		return fmt.Errorf("failed to fetch Prometheus metrics: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iFetchJSONMetrics fetches GET /__admin/metrics.
func (tc *TestContext) iFetchJSONMetrics() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/metrics", nil)
	if err != nil {
		return fmt.Errorf("failed to fetch JSON metrics: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// thePrometheusResponseContains asserts that the Prometheus response body contains a string.
func (tc *TestContext) thePrometheusResponseContains(expected string) error {
	if tc.body == nil {
		return fmt.Errorf("no response body to check")
	}
	if !strings.Contains(string(tc.body), expected) {
		return fmt.Errorf("expected Prometheus response to contain %q, got:\n%s", expected, string(tc.body))
	}
	return nil
}

// theJSONMetricsContainKey asserts that the JSON metrics response contains the given key.
func (tc *TestContext) theJSONMetricsContainKey(key string) error {
	if tc.body == nil {
		return fmt.Errorf("no response body to check")
	}
	var metrics map[string]interface{}
	if err := json.Unmarshal(tc.body, &metrics); err != nil {
		return fmt.Errorf("failed to parse JSON metrics: %w", err)
	}
	if _, ok := metrics[key]; !ok {
		return fmt.Errorf("JSON metrics missing key %q; available keys: %v", key, getMapKeys(metrics))
	}
	return nil
}

// ============================================================================
// Near-miss steps
// ============================================================================

// iQueryNearmiss sends POST /__admin/nearmiss with the given method and path.
func (tc *TestContext) iQueryNearmiss(method, path string) error {
	payload := map[string]string{
		"method": method,
		"path":   path,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal near-miss payload: %w", err)
	}

	resp, body, err := tc.SendAdminRequest("POST", "/__admin/nearmiss", jsonPayload)
	if err != nil {
		return fmt.Errorf("near-miss request failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Request log steps
// ============================================================================

// iListRequestLog fetches GET /__admin/requests.
func (tc *TestContext) iListRequestLog() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/requests", nil)
	if err != nil {
		return fmt.Errorf("failed to list request log: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iListRequestLogWithFilter fetches GET /__admin/requests?filter={filter}.
func (tc *TestContext) iListRequestLogWithFilter(filter string) error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/requests?filter="+filter, nil)
	if err != nil {
		return fmt.Errorf("failed to list request log: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iClearRequestLog sends DELETE /__admin/requests.
func (tc *TestContext) iClearRequestLog() error {
	resp, body, err := tc.SendAdminRequest("DELETE", "/__admin/requests", nil)
	if err != nil {
		return fmt.Errorf("failed to clear request log: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Health endpoints
// ============================================================================

// iCheckLivenessEndpoint fetches GET /__admin/health/live.
func (tc *TestContext) iCheckLivenessEndpoint() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/health/live", nil)
	if err != nil {
		return fmt.Errorf("liveness check failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// iCheckReadinessEndpoint fetches GET /__admin/health/ready.
func (tc *TestContext) iCheckReadinessEndpoint() error {
	resp, body, err := tc.SendAdminRequest("GET", "/__admin/health/ready", nil)
	if err != nil {
		return fmt.Errorf("readiness check failed: %w", err)
	}

	tc.mu.Lock()
	tc.response = resp
	tc.body = body
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Response helpers
// ============================================================================

// ResponseHasStatus checks if the last admin response has the given status code.
func (tc *TestContext) ResponseHasStatus(expected int) error {
	if tc.response == nil {
		return fmt.Errorf("no admin response to check")
	}
	if tc.response.StatusCode != expected {
		return fmt.Errorf("expected admin response status %d, got %d", expected, tc.response.StatusCode)
	}
	return nil
}

// ResponseBodyContains checks if the last admin response body contains a string.
func (tc *TestContext) ResponseBodyContains(substr string) error {
	if tc.body == nil {
		return fmt.Errorf("no response body captured")
	}
	if !strings.Contains(string(tc.body), substr) {
		return fmt.Errorf("expected response body to contain %q, got: %s", substr, string(tc.body))
	}
	return nil
}

// ParseAdminResponseBody parses the last admin response body as JSON into the given value.
func (tc *TestContext) ParseAdminResponseBody(v interface{}) error {
	if tc.body == nil {
		return fmt.Errorf("no response body captured")
	}
	return json.Unmarshal(tc.body, v)
}

// ============================================================================
// Helpers
// ============================================================================

// buildStubPayload constructs a JSON payload for POST /__admin/mappings.
func buildStubPayload(method, path string, status int, body string, fault map[string]interface{}, delay map[string]interface{}) []byte {
	response := map[string]interface{}{
		"status": status,
	}
	if body != "" {
		response["body"] = body
	}
	if fault != nil {
		response["fault"] = fault
	}
	if delay != nil {
		response["delay"] = delay
	}

	payload := map[string]interface{}{
		"request": map[string]interface{}{
			"method":  method,
			"urlPath": path,
		},
		"response": response,
	}

	jsonPayload, _ := json.Marshal(payload)
	return jsonPayload
}

// getMapKeys returns the keys of a map for error messages.
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Ensure io is used (for unused import prevention)
var _ = io.Discard

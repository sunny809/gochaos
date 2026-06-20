// Package integration_test provides black-box integration tests against the
// compiled gmock binary. These tests start the binary as a subprocess and
// interact with it via HTTP — exactly like a real user would with curl.
package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// buildBinary builds the gmock CLI binary and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()

	cachePath := "/tmp/gmock-integration-test"
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath
	}

	// Need to find the go.mod root. Use git rev-parse.
	root := discoverModuleRoot(t)
	cmd := exec.Command("go", "build", "-o", cachePath, ".")
	cmd.Dir = filepath.Join(root, "cmd", "gmock")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build gmock binary: %v\n%s", err, string(out))
	}

	if err := os.Chmod(cachePath, 0755); err != nil {
		t.Fatalf("failed to chmod binary: %v", err)
	}

	return cachePath
}

// discoverModuleRoot finds the go module root by looking for go.mod.
func discoverModuleRoot(t *testing.T) string {
	t.Helper()

	// Try relative to test file location, then walk up
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to getwd: %v", err)
	}
	// The test runs from the module root via `go test`
	// but also might run from within test/integration/
	// Check if we're in the module root (has go.mod)
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return dir
	}
	// Try test/integration/
	if _, err := os.Stat(filepath.Join(dir, "../../go.mod")); err == nil {
		return filepath.Join(dir, "../..")
	}
	t.Fatalf("could not find go.mod from %s", dir)
	return ""
}

// startBinary starts the gmock binary as a subprocess on a random port.
// Returns the base URL and cleanup function.
func startBinary(t *testing.T, binaryPath string, extraArgs ...string) (string, func()) {
	t.Helper()

	port := pickPort()
	args := []string{"start", "--port", strconv.Itoa(port)}
	args = append(args, extraArgs...)

	cmd := exec.Command(binaryPath, args...)
	// Capture output for debugging but don't spam test logs
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start gmock binary: %v", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server to be ready (poll health endpoint up to 5s)
	waitForReady(t, baseURL, 5*time.Second)

	return baseURL, func() {
		killCmd := exec.Command(binaryPath, "reset", "--admin-url", baseURL+"/__admin")
		_ = killCmd.Run()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
}

// pickPort finds a free TCP port.
func pickPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitForReady polls the health endpoint until the server responds 200.
func waitForReady(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/__admin/health")
		if err == nil && resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %v", baseURL, timeout)
}

// httpDo is a convenience wrapper for HTTP requests.
func httpDo(t *testing.T, method, url, contentType, body string) *http.Response {
	t.Helper()

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodGet || method == http.MethodDelete {
		// Ensure no body for GET/DELETE
		req.Body = nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to %s failed: %v", url, err)
	}
	return resp
}

// createStub registers a stub via the admin API and returns the stub ID.
func createStub(t *testing.T, baseURL, jsonBody string) string {
	t.Helper()

	resp := httpDo(t, "POST", baseURL+"/__admin/mappings", "application/json", jsonBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("createStub: expected 201/200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	id, _ := result["id"].(string)
	return id
}

// getFaultLog retrieves the fault injection log entries.
func getFaultLog(t *testing.T, baseURL string) []interface{} {
	t.Helper()

	resp := httpDo(t, "GET", baseURL+"/__admin/fault-log", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("getFaultLog: expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode fault log: %v", err)
	}

	entries, _ := result["entries"].([]interface{})
	return entries
}

// dumpStubsTo creates a temporary YAML stub file and returns its path.
func dumpStubsTo(t *testing.T, yamlContent string) string {
	t.Helper()

	f, err := os.CreateTemp("", "gmock-stubs-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	return f.Name()
}

// readAll reads the full response body.
func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	return string(body)
}

// waitForPort polls a TCP port until it accepts a connection.
func waitForPort(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d did not become ready within %v", port, timeout)
}
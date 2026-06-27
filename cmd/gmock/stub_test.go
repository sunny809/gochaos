package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

var testMu sync.Mutex

// newTestServer creates a mock gmock admin server for CLI tests.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /__admin/mappings":
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"mappings": []map[string]interface{}{
					{"id": "stub-1", "request": map[string]interface{}{"method": "GET", "urlPath": "/test"}},
				},
				"meta": map[string]interface{}{"total": 1},
			})
		case "POST /__admin/mappings":
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			writeJSON(w, http.StatusCreated, map[string]interface{}{"id": "new-stub-id"})
		case "GET /__admin/mappings/stub-1":
			writeJSON(w, http.StatusOK, map[string]interface{}{"id": "stub-1"})
		case "GET /__admin/mappings/nonexistent":
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
		case "DELETE /__admin/mappings/stub-1":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /__admin/mappings":
			w.WriteHeader(http.StatusNoContent)
		case "POST /__admin/reset":
			w.WriteHeader(http.StatusOK)
			writeJSON(w, http.StatusOK, map[string]interface{}{"status": "reset"})
		case "GET /__admin/requests":
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requests": []map[string]interface{}{},
				"meta":     map[string]interface{}{"total": 0},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "not found"})
		}
	}))
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func TestStubListCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newStubListCmd()
	commonAdminURL = server.URL
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("stub-1")) {
		t.Errorf("expected output to contain 'stub-1', got: %s", output)
	}
}

func TestStubGetCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newStubGetCmd()
	commonAdminURL = server.URL
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"stub-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("stub-1")) {
		t.Errorf("expected output to contain 'stub-1', got: %s", output)
	}
}

func TestStubGetCmdNotFound(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newStubGetCmd()
	commonAdminURL = server.URL
	commonClient = server.Client()

	cmd.SetArgs([]string{"nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for nonexistent stub, got nil")
	}
}

func TestStubDeleteCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newStubDeleteCmd()
	commonAdminURL = server.URL
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"stub-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("deleted")) {
		t.Errorf("expected output to contain 'deleted', got: %s", output)
	}
}

func TestStubDeleteAllCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newStubDeleteCmd()
	commonAdminURL = server.URL
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})
	cmd.Flags().Set("all", "true")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("deleted")) {
		t.Errorf("expected output to contain 'deleted', got: %s", output)
	}
}

func TestResetCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newResetCmd()
	cmd.Flags().Set("admin-url", server.URL)
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("reset")) {
		t.Errorf("expected output to contain 'reset', got: %s", output)
	}
}

func TestRequestsCmd(t *testing.T) {
	testMu.Lock()
	defer testMu.Unlock()
	server := newTestServer(t)
	defer server.Close()

	cmd := newRequestsCmd()
	cmd.Flags().Set("admin-url", server.URL)
	commonClient = server.Client()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Errorf("expected output to contain JSON, got: %s", output)
	}
}

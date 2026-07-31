package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReportCommandJUnit fetches and prints the JUnit report.
// Reuses the package-level testMu, commonAdminURL and commonClient
// declared in stub_test.go / stub.go. The admin URL is passed via the
// command's own --admin-url flag (same convention as the reset/requests
// command tests); commonClient is pointed at the fake server.
func TestReportCommandJUnit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__admin/report" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "junit" {
			t.Errorf("expected format=junit, got %q", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<testsuite name="gmock-chaos" tests="1" failures="1"/>`))
	}))
	defer ts.Close()

	testMu.Lock()
	defer testMu.Unlock()
	commonAdminURL = ts.URL
	commonClient = ts.Client()

	cmd := newReportCmd()
	cmd.SetArgs([]string{"--format", "junit"})
	cmd.Flags().Set("admin-url", ts.URL)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "<testsuite name=\"gmock-chaos\"") {
		t.Fatalf("expected junit output, got: %s", buf.String())
	}
}

// TestReportCommandRejectsBadFormat surfaces the server's 400 response.
func TestReportCommandRejectsBadFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid format: must be junit or json"}`))
	}))
	defer ts.Close()

	testMu.Lock()
	defer testMu.Unlock()
	commonAdminURL = ts.URL
	commonClient = ts.Client()

	cmd := newReportCmd()
	cmd.SetArgs([]string{"--format", "bogus"})
	cmd.Flags().Set("admin-url", ts.URL)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for bad format")
	}
}

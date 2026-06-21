package gmock_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// noMatchResponse mirrors the on-the-wire 404 body emitted by writeNoMatch in
// pkg/gmock/server.go. It is duplicated here (rather than imported) because the
// type is unexported — tests assert against the public JSON contract only.
type noMatchResponse struct {
	Error      string `json:"error"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	NearMisses []struct {
		StubID        string `json:"stubId"`
		StubName      string `json:"stubName,omitempty"`
		Score         int    `json:"score"`
		MaxScore      int    `json:"maxScore"`
		TopMissReason string `json:"topMissReason"`
	} `json:"nearMisses"`
}

// startMockServer starts a gmock server on a random port and registers the
// supplied cleanup function with t.Cleanup.
func startMockServer(t *testing.T) gmock.Server {
	t.Helper()
	srv := gmock.NewServer(gmock.WithPort(0))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

// readNoMatchBody decodes the 404 body and asserts the basic contract:
// status code, content-type, and that nearMisses is present (possibly empty).
func readNoMatchBody(t *testing.T, resp *http.Response) noMatchResponse {
	t.Helper()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body noMatchResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body %q: %v", string(raw), err)
	}
	return body
}

// TestWriteNoMatch_IncludesNearMiss verifies that the 404 response body
// embeds near-miss diagnostics for each registered stub when a request fails
// to match. This is the headline P0.3 behavior: the on-miss response actually
// tells the user *which* stubs were close and *why*.
func TestWriteNoMatch_IncludesNearMiss(t *testing.T) {
	srv := startMockServer(t)

	// Register a single stub that the test request will not match.
	srv.Stub(gmock.StubDefinition{
		Name: "create-user",
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusCreated},
	})

	// Send an unmatched request: wrong method AND wrong path.
	resp, err := http.Get(srv.URL() + "/api/orders")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	defer resp.Body.Close()

	body := readNoMatchBody(t, resp)

	if body.Error != "no matching stub" {
		t.Errorf("expected error 'no matching stub', got %q", body.Error)
	}
	if body.Method != http.MethodGet {
		t.Errorf("expected method GET, got %q", body.Method)
	}
	if body.Path != "/api/orders" {
		t.Errorf("expected path /api/orders, got %q", body.Path)
	}
	if len(body.NearMisses) == 0 {
		t.Fatalf("expected at least one nearMiss entry, got 0; body=%+v", body)
	}

	first := body.NearMisses[0]
	if first.StubID == "" {
		t.Errorf("expected non-empty stubId in first nearMiss, got %+v", first)
	}
	if first.MaxScore <= 0 {
		t.Errorf("expected positive maxScore in first nearMiss, got %d", first.MaxScore)
	}
	if first.TopMissReason == "" {
		t.Errorf("expected non-empty topMissReason for unmatched stub, got %+v", first)
	}
}

// TestWriteNoMatch_EmptyRegistry verifies the empty-registry contract: the
// nearMisses field is an empty (non-nil) array, NOT null and NOT missing.
// Clients should be able to decode and iterate over it unconditionally.
func TestWriteNoMatch_EmptyRegistry(t *testing.T) {
	srv := startMockServer(t)

	resp, err := http.Get(srv.URL() + "/no/such/path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	defer resp.Body.Close()

	body := readNoMatchBody(t, resp)

	if body.Error != "no matching stub" {
		t.Errorf("expected error 'no matching stub', got %q", body.Error)
	}
	if body.NearMisses == nil {
		t.Fatalf("expected non-nil empty nearMisses array, got nil")
	}
	if len(body.NearMisses) != 0 {
		t.Errorf("expected empty nearMisses for empty registry, got %d entries", len(body.NearMisses))
	}

	// Re-decode raw bytes to confirm the JSON contains "[]" rather than
	// "null" — the json.Unmarshal above happily decodes both into a nil
	// slice, so a string-level check is the only way to nail this down.
	resp2, err := http.Get(srv.URL() + "/no/such/path")
	if err != nil {
		t.Fatalf("GET (2): %v", err)
	}
	if resp2 == nil {
		t.Fatal("nil response")
	}
	defer resp2.Body.Close()
	raw, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("decode generic: %v", err)
	}
	nm, ok := generic["nearMisses"]
	if !ok {
		t.Fatalf("nearMisses field missing from body: %s", string(raw))
	}
	arr, ok := nm.([]any)
	if !ok {
		t.Fatalf("expected nearMisses to be a JSON array, got %T (%v)", nm, nm)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty JSON array, got %d entries: %v", len(arr), arr)
	}
}

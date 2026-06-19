package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

// decodeNearMissResponse parses a 200-OK response body from POST /__admin/nearmiss.
func decodeNearMissResponse(t *testing.T, body []byte) (nearMisses []spec.NearMissResult, meta map[string]interface{}) {
	t.Helper()
	var raw struct {
		NearMisses []spec.NearMissResult  `json:"nearMisses"`
		Meta       map[string]interface{} `json:"meta"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, string(body))
	}
	return raw.NearMisses, raw.Meta
}

// postNearMiss is a small helper that fires a POST /__admin/nearmiss with the
// given JSON body and returns the recorder so assertions can inspect status +
// body. body may be any JSON-shaped value (struct, map, raw string).
func postNearMiss(t *testing.T, h *Handler, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case string:
		buf.WriteString(b)
	case []byte:
		buf.Write(b)
	default:
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/__admin/nearmiss", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestNearMissEndpoint_Basic(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Name:     "users-by-id",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/api/users/123"},
		Response: spec.ResponseDefinition{Status: 200},
	})
	registry.Add(spec.StubDefinition{
		Name:     "orders-by-id",
		Request:  spec.RequestPattern{Method: "POST", URLPath: "/api/orders/42"},
		Response: spec.ResponseDefinition{Status: 201},
	})

	w := postNearMiss(t, h, NearMissRequest{
		Method: "GET",
		Path:   "/api/users/124", // close to /api/users/123
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	nearMisses, meta := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) < 1 {
		t.Fatalf("expected at least 1 near-miss, got 0")
	}
	// breakdown must be populated for each result
	for i, nm := range nearMisses {
		if len(nm.Breakdown) == 0 {
			t.Errorf("nearMisses[%d] has empty breakdown", i)
		}
		if nm.MaxScore <= 0 {
			t.Errorf("nearMisses[%d] has non-positive maxScore=%d", i, nm.MaxScore)
		}
	}
	if got, ok := meta["total"].(float64); !ok || int(got) != len(nearMisses) {
		t.Errorf("meta.total mismatch: %v vs len=%d", meta["total"], len(nearMisses))
	}
	if _, ok := meta["topN"].(float64); !ok {
		t.Errorf("meta.topN missing or wrong type: %v", meta["topN"])
	}
}

func TestNearMissEndpoint_ExactMatch(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Name:     "exact",
		Request:  spec.RequestPattern{Method: "GET", URLPath: "/exact"},
		Response: spec.ResponseDefinition{Status: 200},
	})

	w := postNearMiss(t, h, NearMissRequest{Method: "GET", Path: "/exact"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	nearMisses, meta := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) != 0 {
		t.Errorf("expected empty nearMisses on exact match, got %d", len(nearMisses))
	}
	if got, ok := meta["total"].(float64); !ok || got != 0 {
		t.Errorf("expected meta.total=0, got %v", meta["total"])
	}
	// Sanity: the JSON payload should encode an empty array, not null.
	if !bytes.Contains(w.Body.Bytes(), []byte(`"nearMisses":[]`)) {
		t.Errorf("expected nearMisses:[] in body, got %s", w.Body.String())
	}
}

func TestNearMissEndpoint_EmptyRegistry(t *testing.T) {
	h, _, _ := setupTest()

	w := postNearMiss(t, h, NearMissRequest{Path: "/anything"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	nearMisses, meta := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) != 0 {
		t.Errorf("expected empty nearMisses, got %d", len(nearMisses))
	}
	if got, ok := meta["total"].(float64); !ok || got != 0 {
		t.Errorf("expected meta.total=0, got %v", meta["total"])
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"nearMisses":[]`)) {
		t.Errorf("expected nearMisses:[] for empty registry, got %s", w.Body.String())
	}
}

func TestNearMissEndpoint_MissingPath(t *testing.T) {
	h, _, _ := setupTest()

	w := postNearMiss(t, h, map[string]string{"method": "GET"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "path is required" {
		t.Errorf("unexpected error: %q", body["error"])
	}
}

func TestNearMissEndpoint_InvalidJSON(t *testing.T) {
	h, _, _ := setupTest()

	w := postNearMiss(t, h, "this is not json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if !strings.HasPrefix(body["error"], "invalid JSON:") {
		t.Errorf("expected error to start with 'invalid JSON:', got %q", body["error"])
	}
}

func TestNearMissEndpoint_DefaultMethod(t *testing.T) {
	h, registry, _ := setupTest()
	// Register a POST stub. The near-miss request omits "method" so the
	// handler defaults it to GET; the method dimension should NOT match.
	registry.Add(spec.StubDefinition{
		Name:     "create-thing",
		Request:  spec.RequestPattern{Method: "POST", URLPath: "/things"},
		Response: spec.ResponseDefinition{Status: 201},
	})

	w := postNearMiss(t, h, NearMissRequest{Path: "/things"}) // method omitted
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	nearMisses, _ := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(nearMisses))
	}
	var foundMethod bool
	for _, dim := range nearMisses[0].Breakdown {
		if dim.Dimension == "method" {
			foundMethod = true
			if dim.Matched {
				t.Errorf("expected method dim NOT to match (default GET vs stub POST), got Matched=true")
			}
			if dim.Actual != http.MethodGet {
				t.Errorf("expected method.Actual=GET, got %q", dim.Actual)
			}
		}
	}
	if !foundMethod {
		t.Errorf("expected breakdown to contain a 'method' dimension, dims=%+v", nearMisses[0].Breakdown)
	}
}

func TestNearMissEndpoint_WithHeaders(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Name: "tenant-required",
		Request: spec.RequestPattern{
			Method:  "GET",
			URLPath: "/api/things",
			Headers: map[string]string{
				"X-Tenant-Id": "acme",
			},
		},
		Response: spec.ResponseDefinition{Status: 200},
	})

	// Send a wrong tenant header — header matcher should report the actual value.
	w := postNearMiss(t, h, NearMissRequest{
		Method:  "GET",
		Path:    "/api/things",
		Headers: map[string]string{"X-Tenant-Id": "evil-corp"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	nearMisses, _ := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(nearMisses))
	}
	var foundHeader bool
	for _, dim := range nearMisses[0].Breakdown {
		if dim.Dimension == "header:X-Tenant-Id" {
			foundHeader = true
			if dim.Matched {
				t.Errorf("expected header dim NOT to match")
			}
			if dim.Actual != "evil-corp" {
				t.Errorf("expected header dim.Actual=evil-corp, got %q", dim.Actual)
			}
		}
	}
	if !foundHeader {
		t.Errorf("expected breakdown to contain a 'header:X-Tenant-Id' dimension, dims=%+v", nearMisses[0].Breakdown)
	}
}

func TestNearMissEndpoint_WithBody(t *testing.T) {
	h, registry, _ := setupTest()
	registry.Add(spec.StubDefinition{
		Name: "body-required",
		Request: spec.RequestPattern{
			Method:  "POST",
			URLPath: "/submit",
			Body:    &spec.BodyPattern{ExactMatch: `{"x":1}`},
		},
		Response: spec.ResponseDefinition{Status: 200},
	})

	// Send a different body — the body matcher should report it didn't match.
	w := postNearMiss(t, h, NearMissRequest{
		Method: "POST",
		Path:   "/submit",
		Body:   `{"x":2}`,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	nearMisses, _ := decodeNearMissResponse(t, w.Body.Bytes())
	if len(nearMisses) != 1 {
		t.Fatalf("expected 1 near-miss, got %d", len(nearMisses))
	}
	var foundBody bool
	for _, dim := range nearMisses[0].Breakdown {
		if dim.Dimension == "body" {
			foundBody = true
			if dim.Matched {
				t.Errorf("expected body dim NOT to match")
			}
		}
	}
	if !foundBody {
		t.Errorf("expected breakdown to contain a 'body' dimension, dims=%+v", nearMisses[0].Breakdown)
	}
}

func TestNearMissEndpoint_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTest()

	req := httptest.NewRequest(http.MethodGet, "/__admin/nearmiss", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

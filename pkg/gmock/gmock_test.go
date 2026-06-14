package gmock_test

import (
	"encoding/json"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

func TestStubDefinitionRoundTrip(t *testing.T) {
	stub := gmock.StubDefinition{
		Name: "test-stub",
		Request: gmock.RequestPattern{
			Method:  "GET",
			URLPath: "/api/users",
			Headers: map[string]string{"Authorization": "Bearer *"},
			QueryParams: map[string]string{"page": "1"},
		},
		Response: gmock.ResponseDefinition{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    `{"users":[]}`,
		},
		Priority: 1,
	}

	// Test JSON serialization
	data, err := json.Marshal(stub)
	if err != nil {
		t.Fatalf("failed to marshal stub: %v", err)
	}

	var decoded gmock.StubDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal stub: %v", err)
	}

	if decoded.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", decoded.Request.Method)
	}
	if decoded.Request.URLPath != "/api/users" {
		t.Errorf("expected /api/users, got %s", decoded.Request.URLPath)
	}
	if decoded.Response.Status != 200 {
		t.Errorf("expected 200, got %d", decoded.Response.Status)
	}
	if decoded.Priority != 1 {
		t.Errorf("expected priority 1, got %d", decoded.Priority)
	}
}

func TestStubDefinitionDefaults(t *testing.T) {
	stub := gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method: "POST",
		},
		Response: gmock.ResponseDefinition{
			Status: 201,
		},
	}

	data, _ := json.Marshal(stub)

	var decoded gmock.StubDefinition
	json.Unmarshal(data, &decoded)

	if decoded.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", decoded.Request.Method)
	}
	if decoded.Response.Status != 201 {
		t.Errorf("expected 201, got %d", decoded.Response.Status)
	}
}

func TestFaultDefinition(t *testing.T) {
	fault := gmock.FaultDefinition{Type: "connection_reset"}
	data, _ := json.Marshal(fault)

	var decoded gmock.FaultDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal fault: %v", err)
	}
	if decoded.Type != "connection_reset" {
		t.Errorf("expected connection_reset, got %s", decoded.Type)
	}
}

func TestDelayDefinition(t *testing.T) {
	delay := gmock.DelayDefinition{Type: "fixed", Value: 2000}
	data, _ := json.Marshal(delay)

	var decoded gmock.DelayDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal delay: %v", err)
	}
	if decoded.Type != "fixed" {
		t.Errorf("expected fixed, got %s", decoded.Type)
	}
	if decoded.Value != 2000 {
		t.Errorf("expected 2000, got %d", decoded.Value)
	}
}

func TestScenarioState(t *testing.T) {
	sc := gmock.ScenarioState{
		Name:          "order-flow",
		RequiredState: "CREATED",
		NextState:     "PAID",
	}
	data, _ := json.Marshal(sc)

	var decoded gmock.ScenarioState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal scenario: %v", err)
	}
	if decoded.Name != "order-flow" {
		t.Errorf("expected order-flow, got %s", decoded.Name)
	}
	if decoded.RequiredState != "CREATED" {
		t.Errorf("expected CREATED, got %s", decoded.RequiredState)
	}
	if decoded.NextState != "PAID" {
		t.Errorf("expected PAID, got %s", decoded.NextState)
	}
}

func TestBodyPattern(t *testing.T) {
	bp := gmock.BodyPattern{JSONPath: "$.name"}
	data, _ := json.Marshal(bp)

	var decoded gmock.BodyPattern
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal body pattern: %v", err)
	}
	if decoded.JSONPath != "$.name" {
		t.Errorf("expected $.name, got %s", decoded.JSONPath)
	}
}

func TestServerOptions(t *testing.T) {
	cfg := gmock.DefaultConfig()
	if cfg.Port != 0 {
		t.Errorf("expected port 0, got %d", cfg.Port)
	}
	if cfg.MaxRequests != 1000 {
		t.Errorf("expected MaxRequests 1000, got %d", cfg.MaxRequests)
	}

	withPort := gmock.WithPort(8080)
	withPort(&cfg)
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}

	withAdmin := gmock.WithAdminPort(8081)
	withAdmin(&cfg)
	if cfg.AdminPort != 8081 {
		t.Errorf("expected admin port 8081, got %d", cfg.AdminPort)
	}

	withFiles := gmock.WithStubFiles("a.yaml", "b.yaml")
	withFiles(&cfg)
	if len(cfg.StubFiles) != 2 {
		t.Errorf("expected 2 stub files, got %d", len(cfg.StubFiles))
	}

	withVerbose := gmock.WithVerbose()
	withVerbose(&cfg)
	if !cfg.Verbose {
		t.Errorf("expected verbose true")
	}
}

func TestVerificationResult(t *testing.T) {
	vr := gmock.VerificationResult{
		ExpectedCount: 3,
		ActualCount:   3,
		Matched:       true,
	}
	if !vr.Matched {
		t.Errorf("expected Matched true")
	}
	if vr.ExpectedCount != vr.ActualCount {
		t.Errorf("expected counts to match")
	}

	vr2 := gmock.VerificationResult{
		ExpectedCount: 2,
		ActualCount:   1,
		Matched:       false,
		Errors:        []string{"expected 2, got 1"},
	}
	if vr2.Matched {
		t.Errorf("expected Matched false")
	}
}

func TestNewServer(t *testing.T) {
	server := gmock.NewServer(gmock.WithPort(0))
	if server == nil {
		t.Fatal("expected non-nil server")
	}

	// Verify it implements Server interface
	var _ gmock.Server = server
}

func TestNewServerWithOptions(t *testing.T) {
	server := gmock.NewServer(
		gmock.WithPort(0),
		gmock.WithVerbose(),
		gmock.WithMaxRequests(500),
	)
	if server == nil {
		t.Fatal("expected non-nil server")
	}
}
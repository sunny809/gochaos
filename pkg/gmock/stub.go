// Package gmock provides a Go-native HTTP mock server inspired by WireMock.
//
// gmock supports dual operation modes:
//   - Embedded library: Start a mock server inside Go tests using gmock.NewServer()
//   - Standalone CLI: Run as a standalone HTTP server using the `gmock start` command
//
// # Quick Start (Library Mode)
//
//	server := gmock.NewServer(gmock.WithPort(0)) // random port
//	defer server.Stop()
//	server.Start()
//
//	server.Stub(gmock.StubDefinition{
//	    Request: gmock.RequestPattern{
//	        Method:  http.MethodGet,
//	        URLPath: "/api/users",
//	    },
//	    Response: gmock.ResponseDefinition{
//	        Status:  http.StatusOK,
//	        Headers: map[string]string{"Content-Type": "application/json"},
//	        Body:    `{"users":[]}`,
//	    },
//	})
//
//	resp, _ := http.Get(server.URL() + "/api/users")
package gmock

import "github.com/sunny809/gochaos/internal/spec"

// Type aliases re-export the core types from internal/spec.
// This keeps the public API stable while allowing internal packages
// to reference the canonical type definitions without an import cycle.

// RequestPattern defines the criteria for matching an incoming HTTP request.
type RequestPattern = spec.RequestPattern

// BodyPattern specifies the strategy for matching a request body.
type BodyPattern = spec.BodyPattern

// ResponseDefinition defines what the mock server should respond with.
type ResponseDefinition = spec.ResponseDefinition

// FaultDefinition describes a network-level fault to simulate.
type FaultDefinition = spec.FaultDefinition

// Activation controls when a fault is triggered.
type Activation = spec.Activation

// TimeWindow defines a time range during which a fault is active.
type TimeWindow = spec.TimeWindow

// DelayDefinition describes a simulated response delay.
type DelayDefinition = spec.DelayDefinition

// StubDefinition is a complete stub mapping a request pattern to a response.
type StubDefinition = spec.StubDefinition

// ScenarioState defines a stateful scenario constraint.
type ScenarioState = spec.ScenarioState

// ProxyConfig configures the proxy fallback behavior.
type ProxyConfig = spec.ProxyConfig

// LoggedRequest represents a recorded incoming request.
type LoggedRequest = spec.LoggedRequest

// HeadersMap is a JSON-friendly representation of http.Header.
type HeadersMap = spec.HeadersMap

// VerificationResult contains the outcome of a verify assertion.
type VerificationResult = spec.VerificationResult

// MatchResult is returned by the matching engine.
type MatchResult = spec.MatchResult

// NearMissResult describes why a request nearly matched a stub.
type NearMissResult = spec.NearMissResult

// DimensionScore breaks down a near-miss score by matching dimension.
type DimensionScore = spec.DimensionScore
// Package spec defines the core data structures used throughout gmock.
//
// These types are placed in an internal package to avoid an import cycle
// between pkg/gmock (the public API) and internal/* packages that need
// to work with these types.
//
// The public types in pkg/gmock are aliases of these types.
package spec

import "time"

// --- Request Matching ---

// RequestPattern defines the criteria for matching an incoming HTTP request.
type RequestPattern struct {
	// Method matches the HTTP method (GET, POST, PUT, etc.).
	// Empty string matches any method.
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// URLPath performs an exact match against the request URL path.
	URLPath string `json:"urlPath,omitempty" yaml:"urlPath,omitempty"`

	// URLPathRegex matches the request URL path against a regular expression.
	URLPathRegex string `json:"urlPathRegex,omitempty" yaml:"urlPathRegex,omitempty"`

	// Accept matches the Accept request header using proper media type negotiation.
	// Supports wildcards (*/*, type/*), quality values, and media ranges.
	Accept string `json:"accept,omitempty" yaml:"accept,omitempty"`

	// QueryParams matches query parameters by key and value patterns.
	QueryParams map[string]string `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`

	// Headers matches request headers by name and value patterns.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// Cookies matches request cookies by name and value patterns.
	Cookies map[string]string `json:"cookies,omitempty" yaml:"cookies,omitempty"`

	// Body specifies how to match the request body.
	Body *BodyPattern `json:"body,omitempty" yaml:"body,omitempty"`

	// Priority controls matching precedence. Lower values have higher priority.
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// BodyPattern specifies the strategy for matching a request body.
type BodyPattern struct {
	ExactMatch string `json:"exactMatch,omitempty" yaml:"exactMatch,omitempty"`
	RegexMatch string `json:"regexMatch,omitempty" yaml:"regexMatch,omitempty"`
	JSONPath   string `json:"jsonPath,omitempty" yaml:"jsonPath,omitempty"`
}

// --- Response Definition ---

// ResponseDefinition defines what the mock server should respond with.
type ResponseDefinition struct {
	Status   int               `json:"status,omitempty" yaml:"status,omitempty"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body     string            `json:"body,omitempty" yaml:"body,omitempty"`

	// Base64Body is a base64-encoded binary response body.
	// Takes precedence over Body when both are set and decodes successfully.
	Base64Body string `json:"base64Body,omitempty" yaml:"base64Body,omitempty"`

	TransformResponse bool              `json:"transformResponse,omitempty" yaml:"transformResponse,omitempty"`
	Fault             *FaultDefinition  `json:"fault,omitempty" yaml:"fault,omitempty"`
	Delay             *DelayDefinition  `json:"delay,omitempty" yaml:"delay,omitempty"`
}

// FaultDefinition describes a network-level fault to simulate.
type FaultDefinition struct {
	Type string `json:"type" yaml:"type"`
}

// DelayDefinition describes a simulated response delay.
type DelayDefinition struct {
	Type  string `json:"type" yaml:"type"`
	Value int    `json:"value,omitempty" yaml:"value,omitempty"`
	Min   int    `json:"min,omitempty" yaml:"min,omitempty"`
	Max   int    `json:"max,omitempty" yaml:"max,omitempty"`
}

// --- Stub Definition ---

// StubDefinition is a complete stub mapping a request pattern to a response.
type StubDefinition struct {
	ID       string             `json:"id,omitempty" yaml:"id,omitempty"`
	Name     string             `json:"name,omitempty" yaml:"name,omitempty"`
	Request  RequestPattern     `json:"request" yaml:"request"`
	Response ResponseDefinition `json:"response" yaml:"response"`
	Scenario *ScenarioState     `json:"scenario,omitempty" yaml:"scenario,omitempty"`
	Priority int                `json:"priority,omitempty" yaml:"priority,omitempty"`
}

// --- Scenario ---

// ScenarioState defines a stateful scenario constraint.
type ScenarioState struct {
	Name          string `json:"name" yaml:"name"`
	RequiredState string `json:"requiredState,omitempty" yaml:"requiredState,omitempty"`
	NextState     string `json:"nextState,omitempty" yaml:"nextState,omitempty"`
}

// --- Proxy ---

// ProxyConfig configures the proxy fallback behavior.
type ProxyConfig struct {
	TargetURL      string `json:"targetUrl" yaml:"targetUrl"`
	RecordMode     bool   `json:"recordMode,omitempty" yaml:"recordMode,omitempty"`
	MaxBodyCapture int    `json:"maxBodyCapture,omitempty" yaml:"maxBodyCapture,omitempty"`
}

// --- Request Log ---

// LoggedRequest represents a recorded incoming request.
type LoggedRequest struct {
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	QueryString string     `json:"queryString,omitempty"`
	Headers     HeadersMap `json:"headers,omitempty"`
	Body        string     `json:"body,omitempty"`
	ReceivedAt  time.Time  `json:"receivedAt"`
}

// HeadersMap is a JSON-friendly representation of http.Header.
type HeadersMap map[string][]string

// --- Verification ---

// VerificationResult contains the outcome of a verify assertion.
type VerificationResult struct {
	ExpectedCount int      `json:"expectedCount"`
	ActualCount   int      `json:"actualCount"`
	Matched       bool     `json:"matched"`
	Errors        []string `json:"errors,omitempty"`

	// BodyPattern echoes back the BodyPattern from the request pattern,
	// or nil when body was not asserted in the verify call.
	BodyPattern *BodyPattern `json:"bodyPattern,omitempty"`

	// HeaderPattern echoes back the Headers map from the request pattern,
	// or nil when headers were not asserted. This is a shallow copy of the
	// caller-supplied pattern to prevent aliasing after Verify returns.
	HeaderPattern map[string]string `json:"headerPattern,omitempty"`

	// QueryParamPattern echoes back the QueryParams map from the request pattern,
	// or nil when query params were not asserted. This is a shallow copy of the
	// caller-supplied pattern to prevent aliasing after Verify returns.
	QueryParamPattern map[string]string `json:"queryParamPattern,omitempty"`
}

// --- Match Result ---

// MatchResult is returned by the matching engine.
type MatchResult struct {
	Stub    *StubDefinition
	Score   int
	Matched bool
}

// --- Near-Miss ---

// NearMissResult describes why a request nearly matched a stub.
type NearMissResult struct {
	StubID    string           `json:"stubId"`
	StubName  string           `json:"stubName,omitempty"`
	Score     int              `json:"score"`
	MaxScore  int              `json:"maxScore"`
	Breakdown []DimensionScore `json:"breakdown"`
	Reason    string           `json:"reason"`
}

// DimensionScore breaks down a near-miss score by matching dimension.
type DimensionScore struct {
	Dimension string `json:"dimension"`
	Matched   bool   `json:"matched"`
	Score     int    `json:"score"`
	MaxScore  int    `json:"maxScore"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
	// Reason is a short, human-readable explanation of why this dimension
	// did not match. It is empty when Matched is true.
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

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

	// DataLength specifies the number of random bytes to send for the
	// "random_data" fault type before closing the connection.
	// When zero or unset, defaults to 256 bytes.
	DataLength int `json:"dataLength,omitempty" yaml:"dataLength,omitempty"`

	// DelayMs specifies the delay in milliseconds before closing the connection
	// for the "slow_close" fault type. The full response is sent first, then
	// the server waits DelayMs before sending FIN. When zero or unset,
	// defaults to 1000ms.
	DelayMs int `json:"delayMs,omitempty" yaml:"delayMs,omitempty"`

	// Activation controls when a fault is triggered. When nil, the fault is
	// always-on (fires on every matched request). When set, the fault only
	// fires when the activation criteria are met.
	Activation *Activation `json:"activation,omitempty" yaml:"activation,omitempty"`

	// AfterRequests is the number of requests that must be served normally
	// before rate limiting kicks in. Used only for the "rate_limit" fault type.
	// When zero or unset, rate limiting begins immediately.
	AfterRequests int `json:"afterRequests,omitempty" yaml:"afterRequests,omitempty"`

	// PerSecond is the maximum number of requests per second allowed after
	// the AfterRequests threshold is reached. Used only for the "rate_limit"
	// fault type. Must be > 0 when fault type is "rate_limit".
	PerSecond int `json:"perSecond,omitempty" yaml:"perSecond,omitempty"`

	// RateLimitStatus is the HTTP status code returned when a request is
	// rate-limited. Used only for the "rate_limit" fault type.
	// When zero or unset, defaults to 429 (Too Many Requests).
	RateLimitStatus int `json:"rateLimitStatus,omitempty" yaml:"rateLimitStatus,omitempty"`
}

// Activation controls when a fault is triggered. When nil, the fault is always-on.
// The three activation modes (Probability, EveryNthRequest, ActiveBetween) are
// evaluated independently; if any mode is configured and its check passes, the
// fault fires. This allows combining modes (e.g., "50% probability during peak
// hours").
type Activation struct {
	// Probability is the chance [0.0, 1.0] that the fault fires on any given
	// request. 0.0 means never, 1.0 means always. When combined with other
	// activation modes, this probability is applied independently.
	Probability float64 `json:"probability,omitempty" yaml:"probability,omitempty"`

	// EveryNthRequest fires the fault every Nth request that matches the stub.
	// A value of 3 means the fault fires on the 3rd, 6th, 9th, ... request.
	// Must be > 0 when set.
	EveryNthRequest int `json:"everyNthRequest,omitempty" yaml:"everyNthRequest,omitempty"`

	// ActiveBetween is a list of time windows during which the fault is active.
	// Each window can optionally override the top-level Probability.
	// When empty, time-based activation is not applied.
	ActiveBetween []TimeWindow `json:"activeBetween,omitempty" yaml:"activeBetween,omitempty"`
}

// TimeWindow defines a time range during which a fault is active.
// StartMs and EndMs are milliseconds elapsed since the server started
// (not Unix timestamps). The current elapsed time is computed as
// time.Since(serverStart).Milliseconds().
type TimeWindow struct {
	// StartMs is the inclusive start of the time window (ms since server start).
	StartMs int64 `json:"startMs" yaml:"startMs"`

	// EndMs is the exclusive end of the time window (ms since server start).
	EndMs int64 `json:"endMs" yaml:"endMs"`

	// Probability overrides the top-level Activation.Probability within this
	// window. When zero (the default), the fault is always-on within this
	// window (no probabilistic gating). Must be in (0, 1] when set.
	Probability float64 `json:"probability,omitempty" yaml:"probability,omitempty"`
}

// DelayDefinition describes a simulated response delay.
type DelayDefinition struct {
	Type  string `json:"type" yaml:"type"`
	Value int    `json:"value,omitempty" yaml:"value,omitempty"`
	Min   int    `json:"min,omitempty" yaml:"min,omitempty"`
	Max   int    `json:"max,omitempty" yaml:"max,omitempty"`

	// P50 is the median latency in milliseconds for lognormal delay.
	// Used when Type is "lognormal". At least P50 and one higher
	// percentile (P95 or P99) must be non-zero.
	P50 int `json:"p50,omitempty" yaml:"p50,omitempty"`

	// P95 is the 95th percentile latency in milliseconds for lognormal delay.
	P95 int `json:"p95,omitempty" yaml:"p95,omitempty"`

	// P99 is the 99th percentile latency in milliseconds for lognormal delay.
	P99 int `json:"p99,omitempty" yaml:"p99,omitempty"`

	// Chunks specifies the number of equal-sized chunks to split the response
	// body into for the "dribble" delay type. Each chunk is sent with an
	// interval of TotalDuration/Chunks milliseconds between writes.
	// Must be > 0 when Type is "dribble".
	Chunks int `json:"chunks,omitempty" yaml:"chunks,omitempty"`

	// TotalDuration is the total time in milliseconds over which the dribble
	// response body is sent. The interval between chunks is
	// TotalDuration/Chunks ms. Must be > 0 when Type is "dribble".
	TotalDuration int `json:"totalDuration,omitempty" yaml:"totalDuration,omitempty"`
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

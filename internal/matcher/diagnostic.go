package matcher

import "net/http"

// Diagnosis describes the result of evaluating a single matcher against a
// request, with enough detail to render a human-readable miss explanation.
//
// Matchers that implement DiagnosticMatcher emit a Diagnosis instead of (or
// in addition to) the bare (bool, int) returned by ScoreMatch. The near-miss
// engine consumes Diagnosis values to build per-dimension breakdowns.
type Diagnosis struct {
	// Dimension identifies the matched dimension. Conventional values:
	// "method", "path", "accept", "body", "header:<Name>", "query:<key>",
	// "cookie:<name>".
	Dimension string

	// Matched is true when the matcher accepted the request.
	Matched bool

	// Score is the score this matcher contributed (0 when not matched).
	Score int

	// MaxScore is the score the matcher would have contributed on a full match.
	MaxScore int

	// Expected is a short string description of what the matcher wanted
	// (e.g. the configured value or pattern).
	Expected string

	// Actual is a short string description of what the request actually
	// supplied for this dimension. Empty when the dimension is absent on the
	// request (e.g. missing header).
	Actual string

	// Reason is a short, human-readable explanation of why the dimension did
	// not match. Reason is empty when Matched is true.
	Reason string
}

// DiagnosticMatcher is an optional interface that matchers may implement to
// produce structured diagnostics for the near-miss engine.
//
// Matchers that do not implement this interface are still usable for primary
// matching; the near-miss engine falls back to a generic diff (matcher.String()
// for Expected, empty Actual) for those.
//
// Consumers should always type-assert: `if dm, ok := m.(DiagnosticMatcher); ok`.
type DiagnosticMatcher interface {
	Matcher
	Diagnose(req *http.Request) Diagnosis
}

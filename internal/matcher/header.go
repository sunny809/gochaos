package matcher

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// HeaderMatcher matches a specific request header by name and value pattern.
//
// Value pattern conventions:
//   - "exact string"    — exact match
//   - "~regex"          — regular expression (prefix with ~)
//   - "*"               — match any non-empty value
//   - "!"               — require the header to be absent
type HeaderMatcher struct {
	name    string
	pattern string
	regex   *regexp.Regexp // compiled only for "~" patterns
}

// NewHeaderMatcher creates a HeaderMatcher.
// If pattern starts with "~", it's treated as a regular expression.
// An empty pattern matches any non-empty value.
func NewHeaderMatcher(name, pattern string) (*HeaderMatcher, error) {
	m := &HeaderMatcher{
		name:    http.CanonicalHeaderKey(name),
		pattern: pattern,
	}

	if strings.HasPrefix(pattern, "~") {
		re, err := regexp.Compile(pattern[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid header regex for %s: %w", name, err)
		}
		m.regex = re
	}

	return m, nil
}

// Match returns true if the request header matches.
func (m *HeaderMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 5 (or 0 if not matched).
func (m *HeaderMatcher) ScoreMatch(req *http.Request) (bool, int) {
	values, exists := req.Header[m.name]

	switch {
	case m.pattern == "!":
		// Header must be absent
		if exists {
			return false, 0
		}
		return true, 5

	case m.pattern == "*":
		// Any non-empty value
		if !exists || len(values) == 0 || values[0] == "" {
			return false, 0
		}
		return true, 5

	case strings.HasPrefix(m.pattern, "~"):
		// Regex match
		if !exists || len(values) == 0 {
			return false, 0
		}
		if m.regex != nil && m.regex.MatchString(values[0]) {
			return true, 5
		}
		return false, 0

	default:
		// Exact match
		if !exists || len(values) == 0 {
			return false, 0
		}
		if values[0] == m.pattern {
			return true, 5
		}
		return false, 0
	}
}

// String returns a description of this matcher.
func (m *HeaderMatcher) String() string {
	return fmt.Sprintf("header %s=%s", m.name, m.pattern)
}

// Name returns the header name (canonical).
func (m *HeaderMatcher) Name() string {
	return m.name
}

// Pattern returns the value pattern.
func (m *HeaderMatcher) Pattern() string {
	return m.pattern
}

// Diagnose returns a structured diagnosis for near-miss reporting.
//
// For absent headers, Actual is left empty (matching the project convention
// of using empty string to denote "not supplied"). Expected is the configured
// pattern verbatim (regex prefixed with "~", "*" for any value, "!" for must
// be absent).
func (m *HeaderMatcher) Diagnose(req *http.Request) Diagnosis {
	values, exists := req.Header[m.name]
	var actual string
	if exists && len(values) > 0 {
		actual = values[0]
	}

	d := Diagnosis{
		Dimension: "header:" + m.name,
		MaxScore:  5,
		Expected:  m.pattern,
		Actual:    actual,
	}

	matched, score := m.ScoreMatch(req)
	if matched {
		d.Matched = true
		d.Score = score
		return d
	}

	switch {
	case m.pattern == "!":
		d.Reason = fmt.Sprintf("header %s should be absent but is %q", m.name, actual)
	case !exists:
		d.Reason = fmt.Sprintf("header %s missing", m.name)
	case m.pattern == "*":
		d.Reason = fmt.Sprintf("header %s value is empty", m.name)
	case strings.HasPrefix(m.pattern, "~"):
		d.Reason = fmt.Sprintf("header %s=%q does not match regex %s", m.name, actual, m.pattern[1:])
	default:
		d.Reason = fmt.Sprintf("header %s=%q does not equal %q", m.name, actual, m.pattern)
	}
	return d
}

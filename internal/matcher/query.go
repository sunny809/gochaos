package matcher

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// QueryParamMatcher matches a query parameter by key and value pattern.
//
// Value pattern conventions:
//   - "exact string"    — exact match
//   - "~regex"          — regular expression (prefix with ~)
//   - "*"               — match any non-empty value for the key
//   - "!"               — require the key to be absent
type QueryParamMatcher struct {
	key     string
	pattern string
	regex   *regexp.Regexp // compiled only for "~" patterns
}

// NewQueryParamMatcher creates a QueryParamMatcher.
// If pattern starts with "~", it's treated as a regular expression.
func NewQueryParamMatcher(key, pattern string) (*QueryParamMatcher, error) {
	m := &QueryParamMatcher{
		key:     key,
		pattern: pattern,
	}

	if strings.HasPrefix(pattern, "~") {
		re, err := regexp.Compile(pattern[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid query param regex for %s: %w", key, err)
		}
		m.regex = re
	}

	return m, nil
}

// Match returns true if the query parameter matches.
func (m *QueryParamMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 3 (or 0 if not matched).
func (m *QueryParamMatcher) ScoreMatch(req *http.Request) (bool, int) {
	values := req.URL.Query()[m.key]

	switch {
	case m.pattern == "!":
		// Key must be absent
		if len(values) > 0 {
			return false, 0
		}
		return true, 3

	case m.pattern == "*":
		// Any non-empty value
		if len(values) == 0 || values[0] == "" {
			return false, 0
		}
		return true, 3

	case strings.HasPrefix(m.pattern, "~"):
		// Regex match
		if len(values) == 0 {
			return false, 0
		}
		if m.regex != nil && m.regex.MatchString(values[0]) {
			return true, 3
		}
		return false, 0

	default:
		// Exact match
		if len(values) > 0 && values[0] == m.pattern {
			return true, 3
		}
		return false, 0
	}
}

// String returns a description of this matcher.
func (m *QueryParamMatcher) String() string {
	return fmt.Sprintf("query %s=%s", m.key, m.pattern)
}

// Key returns the query parameter key.
func (m *QueryParamMatcher) Key() string {
	return m.key
}

// Pattern returns the value pattern.
func (m *QueryParamMatcher) Pattern() string {
	return m.pattern
}

// Diagnose returns a structured diagnosis for near-miss reporting.
func (m *QueryParamMatcher) Diagnose(req *http.Request) Diagnosis {
	values := req.URL.Query()[m.key]
	var actual string
	if len(values) > 0 {
		actual = values[0]
	}

	d := Diagnosis{
		Dimension: "query:" + m.key,
		MaxScore:  3,
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
		d.Reason = fmt.Sprintf("query %s should be absent", m.key)
	case len(values) == 0:
		d.Reason = fmt.Sprintf("query %s missing", m.key)
	case m.pattern == "*":
		d.Reason = fmt.Sprintf("query %s value is empty", m.key)
	case strings.HasPrefix(m.pattern, "~"):
		d.Reason = fmt.Sprintf("query %s=%q does not match regex %s", m.key, actual, m.pattern[1:])
	default:
		d.Reason = fmt.Sprintf("query %s=%q does not equal %q", m.key, actual, m.pattern)
	}
	return d
}

package matcher

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// CookieMatcher matches a specific cookie by name and value pattern.
//
// Value pattern conventions:
//   - "exact string"    — exact match
//   - "~regex"          — regular expression (prefix with ~)
//   - "*"               — match any non-empty value
//   - "!"               — require the cookie to be absent
type CookieMatcher struct {
	name    string
	pattern string
	regex   *regexp.Regexp // compiled only for "~" patterns
}

// NewCookieMatcher creates a CookieMatcher.
// If pattern starts with "~", it's treated as a regular expression.
func NewCookieMatcher(name, pattern string) (*CookieMatcher, error) {
	m := &CookieMatcher{
		name:    name,
		pattern: pattern,
	}

	if strings.HasPrefix(pattern, "~") {
		re, err := regexp.Compile(pattern[1:])
		if err != nil {
			return nil, fmt.Errorf("invalid cookie regex for %s: %w", name, err)
		}
		m.regex = re
	}

	return m, nil
}

// Match returns true if the request cookie matches.
func (m *CookieMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 4 (or 0 if not matched).
func (m *CookieMatcher) ScoreMatch(req *http.Request) (bool, int) {
	cookies := req.Cookies()

	// Find the cookie by name
	var cookieValue string
	found := false
	for _, c := range cookies {
		if c.Name == m.name {
			cookieValue = c.Value
			found = true
			break
		}
	}

	switch {
	case m.pattern == "!":
		// Cookie must be absent
		if found {
			return false, 0
		}
		return true, 4

	case m.pattern == "*":
		// Any non-empty value
		if !found || cookieValue == "" {
			return false, 0
		}
		return true, 4

	case strings.HasPrefix(m.pattern, "~"):
		// Regex match
		if !found {
			return false, 0
		}
		if m.regex != nil && m.regex.MatchString(cookieValue) {
			return true, 4
		}
		return false, 0

	default:
		// Exact match
		if !found {
			return false, 0
		}
		if cookieValue == m.pattern {
			return true, 4
		}
		return false, 0
	}
}

// String returns a description of this matcher.
func (m *CookieMatcher) String() string {
	return fmt.Sprintf("cookie %s=%s", m.name, m.pattern)
}

// Name returns the cookie name.
func (m *CookieMatcher) Name() string {
	return m.name
}

// Pattern returns the value pattern.
func (m *CookieMatcher) Pattern() string {
	return m.pattern
}

// Diagnose returns a structured diagnosis for near-miss reporting.
func (m *CookieMatcher) Diagnose(req *http.Request) Diagnosis {
	cookies := req.Cookies()
	var actual string
	found := false
	for _, c := range cookies {
		if c.Name == m.name {
			actual = c.Value
			found = true
			break
		}
	}

	d := Diagnosis{
		Dimension: "cookie:" + m.name,
		MaxScore:  4,
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
		d.Reason = fmt.Sprintf("cookie %s should be absent", m.name)
	case !found:
		d.Reason = fmt.Sprintf("cookie %s missing", m.name)
	case m.pattern == "*":
		d.Reason = fmt.Sprintf("cookie %s value is empty", m.name)
	case strings.HasPrefix(m.pattern, "~"):
		d.Reason = fmt.Sprintf("cookie %s=%q does not match regex %s", m.name, actual, m.pattern[1:])
	default:
		d.Reason = fmt.Sprintf("cookie %s=%q does not equal %q", m.name, actual, m.pattern)
	}
	return d
}

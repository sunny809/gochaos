package matcher

import (
	"fmt"
	"net/http"
	"regexp"
)

// PathExactMatcher matches the request URL path exactly.
type PathExactMatcher struct {
	path string
}

// NewPathExactMatcher creates a matcher for an exact path match.
func NewPathExactMatcher(path string) *PathExactMatcher {
	return &PathExactMatcher{path: path}
}

// Match returns true if the request URL path matches exactly.
func (m *PathExactMatcher) Match(req *http.Request) bool {
	return req.URL.Path == m.path
}

// ScoreMatch returns the match result with score 30 for an exact match.
func (m *PathExactMatcher) ScoreMatch(req *http.Request) (bool, int) {
	if req.URL.Path == m.path {
		return true, 30
	}
	return false, 0
}

// String returns a description of this matcher.
func (m *PathExactMatcher) String() string {
	return fmt.Sprintf("path exact=%s", m.path)
}

// Path returns the target path.
func (m *PathExactMatcher) Path() string {
	return m.path
}

// ---

// PathRegexMatcher matches the request URL path against a regular expression.
type PathRegexMatcher struct {
	pattern *regexp.Regexp
	raw     string
}

// NewPathRegexMatcher creates a matcher for a regex path match.
// Returns an error if the pattern is not a valid regular expression.
func NewPathRegexMatcher(pattern string) (*PathRegexMatcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid path regex %q: %w", pattern, err)
	}
	return &PathRegexMatcher{pattern: re, raw: pattern}, nil
}

// MustNewPathRegexMatcher is like NewPathRegexMatcher but panics on error.
func MustNewPathRegexMatcher(pattern string) *PathRegexMatcher {
	m, err := NewPathRegexMatcher(pattern)
	if err != nil {
		panic(err)
	}
	return m
}

// Match returns true if the request URL path matches the regex.
func (m *PathRegexMatcher) Match(req *http.Request) bool {
	return m.pattern.MatchString(req.URL.Path)
}

// ScoreMatch returns the match result with score 15 for a regex match.
func (m *PathRegexMatcher) ScoreMatch(req *http.Request) (bool, int) {
	if m.pattern.MatchString(req.URL.Path) {
		return true, 15
	}
	return false, 0
}

// String returns a description of this matcher.
func (m *PathRegexMatcher) String() string {
	return fmt.Sprintf("path regex=%s", m.raw)
}

// Raw returns the raw regex pattern string.
func (m *PathRegexMatcher) Raw() string {
	return m.raw
}
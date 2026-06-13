package matcher

import (
	"fmt"
	"net/http"
	"strings"
)

// MethodMatcher matches the HTTP method (GET, POST, PUT, DELETE, etc.).
type MethodMatcher struct {
	method string // normalized (uppercase)
}

// NewMethodMatcher creates a MethodMatcher for the given method.
// The method is normalized to uppercase.
func NewMethodMatcher(method string) *MethodMatcher {
	return &MethodMatcher{method: strings.ToUpper(method)}
}

// Match returns true if the request method matches.
func (m *MethodMatcher) Match(req *http.Request) bool {
	return strings.EqualFold(req.Method, m.method)
}

// ScoreMatch returns the match result with a score of 10.
func (m *MethodMatcher) ScoreMatch(req *http.Request) (bool, int) {
	matched := strings.EqualFold(req.Method, m.method)
	score := 0
	if matched {
		score = 10
	}
	return matched, score
}

// String returns a description of this matcher.
func (m *MethodMatcher) String() string {
	return fmt.Sprintf("method=%s", m.method)
}

// Method returns the target method (uppercase).
func (m *MethodMatcher) Method() string {
	return m.method
}
// Package matcher provides request matching components for gmock.
//
// Each matcher implements the Matcher interface, which returns both a boolean
// match result and a numeric score. The score is used by the matching engine
// to select the best stub when multiple stubs match, and by the near-miss
// analyzer to explain why a request didn't match.
package matcher

import (
	"net/http"
	"strings"
)

// Matcher defines the interface for request matching.
// Each implementation matches a single dimension of an HTTP request.
type Matcher interface {
	// Match returns true if the request matches this matcher's criteria.
	Match(req *http.Request) bool

	// ScoreMatch returns both the match result and a numerical score.
	// Higher scores indicate a more specific match. Used for prioritization
	// and near-miss analysis.
	ScoreMatch(req *http.Request) (matched bool, score int)

	// String returns a human-readable description of this matcher.
	String() string
}

// MatcherFunc is an adapter that allows a function to serve as a Matcher.
type MatcherFunc func(req *http.Request) (matched bool, score int)

// Match implements the Matcher interface.
func (f MatcherFunc) Match(req *http.Request) bool {
	matched, _ := f(req)
	return matched
}

// ScoreMatch implements the Matcher interface.
func (f MatcherFunc) ScoreMatch(req *http.Request) (bool, int) {
	return f(req)
}

// String implements the Matcher interface.
func (f MatcherFunc) String() string {
	return "custom matcher function"
}

// CompositeMatcher combines multiple matchers with AND semantics.
// A request must match ALL matchers to be considered a match.
// The total score is the sum of all matching matcher scores.
type CompositeMatcher struct {
	matchers []Matcher
}

// NewCompositeMatcher creates a composite matcher from the given matchers.
func NewCompositeMatcher(matchers ...Matcher) *CompositeMatcher {
	return &CompositeMatcher{matchers: matchers}
}

// Match returns true only when all matchers match.
func (c *CompositeMatcher) Match(req *http.Request) bool {
	for _, m := range c.matchers {
		if !m.Match(req) {
			return false
		}
	}
	return true
}

// ScoreMatch runs all matchers and returns the aggregate score.
// If any matcher fails, the match is false but the score reflects
// how many dimensions matched (useful for near-miss analysis).
func (c *CompositeMatcher) ScoreMatch(req *http.Request) (bool, int) {
	totalScore := 0
	allMatched := true

	for _, m := range c.matchers {
		matched, score := m.ScoreMatch(req)
		if !matched {
			allMatched = false
		}
		totalScore += score
	}

	return allMatched, totalScore
}

// String returns a description of the composite matcher.
func (c *CompositeMatcher) String() string {
	parts := make([]string, len(c.matchers))
	for i, m := range c.matchers {
		parts[i] = m.String()
	}
	return "Composite(" + strings.Join(parts, " && ") + ")"
}

// Matchers returns the constituent matchers (for inspection/debugging).
func (c *CompositeMatcher) Matchers() []Matcher {
	return c.matchers
}

// AlwaysMatch is a matcher that always matches with a score of 0.
// Used as a default when no matcher is specified for a dimension.
var AlwaysMatch Matcher = MatcherFunc(func(req *http.Request) (bool, int) {
	return true, 0
})

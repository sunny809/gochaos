package stub

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/sunny809/gochaos/internal/matcher"
	"github.com/sunny809/gochaos/internal/spec"
)

// Engine performs stub matching against incoming HTTP requests.
// It uses a Registry as its data source and creates Matcher instances
// from StubDefinition RequestPatterns.
type Engine struct {
	registry *Registry
}

// NewEngine creates a matching engine backed by the given registry.
func NewEngine(registry *Registry) *Engine {
	return &Engine{registry: registry}
}

// Match searches for the best matching stub for the given request.
// Returns the best match or nil if no stub matches.
func (e *Engine) Match(req *http.Request) *spec.MatchResult {
	records := e.registry.All()
	if len(records) == 0 {
		return nil
	}

	var bestMatch *spec.MatchResult

	for _, rec := range records {
		composite := buildMatcher(rec.Definition.Request)
		matched, score := composite.ScoreMatch(req)

		if matched {
			def := rec.Definition
			result := &spec.MatchResult{
				Stub:    &def,
				Score:   score,
				Matched: true,
			}

			if bestMatch == nil || score > bestMatch.Score {
				bestMatch = result
			}
		}
	}

	return bestMatch
}

// MatchWithScore runs the full scoring for near-miss analysis.
// Returns ALL results (both matched and unmatched) sorted by score descending.
func (e *Engine) MatchWithScore(req *http.Request) []spec.MatchResult {
	records := e.registry.All()
	results := make([]spec.MatchResult, 0, len(records))

	for _, rec := range records {
		composite := buildMatcher(rec.Definition.Request)
		matched, score := composite.ScoreMatch(req)

		def := rec.Definition
		results = append(results, spec.MatchResult{
			Stub:    &def,
			Score:   score,
			Matched: matched,
		})
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// buildMatcher creates a CompositeMatcher from a RequestPattern.
func buildMatcher(pattern spec.RequestPattern) *matcher.CompositeMatcher {
	var matchers []matcher.Matcher

	// Method
	if pattern.Method != "" {
		matchers = append(matchers, matcher.NewMethodMatcher(pattern.Method))
	}

	// URL path: exact takes precedence over regex if both are set
	if pattern.URLPath != "" {
		matchers = append(matchers, matcher.NewPathExactMatcher(pattern.URLPath))
	} else if pattern.URLPathRegex != "" {
		matchers = append(matchers, matcher.MustNewPathRegexMatcher(pattern.URLPathRegex))
	}

	// Accept header (content negotiation)
	if pattern.Accept != "" {
		matchers = append(matchers, matcher.NewAcceptMatcher(pattern.Accept))
	}

	// Headers
	for name, valuePattern := range pattern.Headers {
		hm, err := matcher.NewHeaderMatcher(name, valuePattern)
		if err == nil {
			matchers = append(matchers, hm)
		}
	}

	// Cookies
	for name, valuePattern := range pattern.Cookies {
		cm, err := matcher.NewCookieMatcher(name, valuePattern)
		if err == nil {
			matchers = append(matchers, cm)
		}
	}

	// Query parameters
	for key, valuePattern := range pattern.QueryParams {
		qm, err := matcher.NewQueryParamMatcher(key, valuePattern)
		if err == nil {
			matchers = append(matchers, qm)
		}
	}

	// Body
	if pattern.Body != nil {
		switch {
		case pattern.Body.ExactMatch != "":
			matchers = append(matchers, matcher.NewBodyExactMatcher(pattern.Body.ExactMatch))
		case pattern.Body.RegexMatch != "":
			bm, err := matcher.NewBodyRegexMatcher(pattern.Body.RegexMatch)
			if err == nil {
				matchers = append(matchers, bm)
			}
		case pattern.Body.JSONPath != "":
			matchers = append(matchers, matcher.NewBodyJSONPathMatcher(pattern.Body.JSONPath))
		}
	}

	if len(matchers) == 0 {
		// Match-anything stub
		return matcher.NewCompositeMatcher()
	}

	return matcher.NewCompositeMatcher(matchers...)
}

// maxScore computes the maximum possible match score for a pattern.
// Used by near-miss analysis for normalization.
func maxScore(pattern spec.RequestPattern) int {
	score := 0

	if pattern.Method != "" {
		score += 10
	}
	if pattern.URLPath != "" {
		score += 30
	} else if pattern.URLPathRegex != "" {
		score += 15
	}
	if pattern.Accept != "" {
		score += 7
	}
	score += len(pattern.Headers) * 5
	score += len(pattern.Cookies) * 4
	score += len(pattern.QueryParams) * 3

	if pattern.Body != nil {
		switch {
		case pattern.Body.ExactMatch != "":
			score += 20
		case pattern.Body.RegexMatch != "":
			score += 10
		case pattern.Body.JSONPath != "":
			score += 12
		}
	}

	return score
}

// WeightedScore returns the match score as a percentage of the maximum possible.
// If max is 0, returns 0 (no stubs defined).
func WeightedScore(actual, max int) float64 {
	if max == 0 {
		return 0
	}
	return math.Round(float64(actual)/float64(max)*100) / 100
}

// RequestKey builds a human-readable key for a request (for logging/diagnostics).
func RequestKey(req *http.Request) string {
	if req == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s %s", req.Method, req.URL.Path)
}

// ExtractRequestPattern creates a simple RequestPattern from the request.
// Used for near-miss analysis when comparing an incoming request against stubs.
func ExtractRequestPattern(req *http.Request) spec.RequestPattern {
	p := spec.RequestPattern{
		Method:  req.Method,
		URLPath: req.URL.Path,
	}

	// Extract headers (simplified — just first value)
	p.Headers = make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			p.Headers[k] = v[0]
		}
	}

	// Extract query params (simplified — just first value)
	p.QueryParams = make(map[string]string)
	for k, v := range req.URL.Query() {
		if len(v) > 0 {
			p.QueryParams[k] = v[0]
		}
	}

	return p
}

// PatternsMatch checks if a RequestPattern matches another RequestPattern.
// This is used for verification — checking if a logged request matches a
// verification pattern.
func PatternsMatch(pattern, candidate spec.RequestPattern) (bool, int) {
	score := 0
	max := 0

	// Method
	if pattern.Method != "" {
		max += 10
		if strings.EqualFold(pattern.Method, candidate.Method) {
			score += 10
		}
	}

	// Path
	if pattern.URLPath != "" {
		max += 30
		if pattern.URLPath == candidate.URLPath {
			score += 30
		}
	} else if pattern.URLPathRegex != "" {
		max += 15
		if matched, _ := regexp.MatchString(pattern.URLPathRegex, candidate.URLPath); matched {
			score += 15
		}
	}

	return score > 0 && score == max, score
}
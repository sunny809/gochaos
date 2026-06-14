package nearmiss

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/sunny809/gochaos/internal/matcher"
	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/stub"
)

// defaultTopN is the cap applied to result lists when no Option overrides it.
const defaultTopN = 5

// Engine produces per-stub, per-dimension diagnostic explanations of why a
// request did not match. See package doc for the resolved design decisions.
//
// Engine is stateless after construction and safe for concurrent use.
type Engine struct {
	topN int
}

// Option configures an Engine via NewEngine.
type Option func(*Engine)

// WithTopN limits the number of NearMissResult entries returned by Compute.
// Values <= 0 disable truncation (return all candidates).
func WithTopN(n int) Option {
	return func(e *Engine) {
		e.topN = n
	}
}

// NewEngine constructs a near-miss engine. The default topN is 5.
func NewEngine(opts ...Option) *Engine {
	e := &Engine{topN: defaultTopN}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Compute returns near-miss results for stubs that did not fully match.
// Stubs that fully match are omitted (per package decision #1). Results are
// ordered best-score-first using a stable sort, so equal-score candidates
// preserve the caller-provided input order (typically registry ordering:
// priority asc, registration sequence asc).
//
// Compute holds no locks. The stubs slice is read-only from the engine's
// perspective and may be safely shared across goroutines once snapshotted.
//
// A nil or empty stubs argument returns a non-nil empty result.
func (e *Engine) Compute(req *http.Request, stubs []spec.StubDefinition) []spec.NearMissResult {
	resultCap := len(stubs)
	if e.topN > 0 && e.topN < resultCap {
		resultCap = e.topN
	}
	results := make([]spec.NearMissResult, 0, resultCap)

	for i := range stubs {
		def := stubs[i]
		composite := stub.BuildMatcher(def.Request)
		breakdown, totalScore, totalMax, fullMatch := diagnoseComposite(composite, req)

		if fullMatch {
			// Skip stubs that fully match — empty list contract for exact match.
			continue
		}

		results = append(results, spec.NearMissResult{
			StubID:    def.ID,
			StubName:  def.Name,
			Score:     totalScore,
			MaxScore:  totalMax,
			Breakdown: breakdown,
		})
	}

	// Stable sort by Score descending. Stable preserves the caller's input
	// order for ties — typically (priority asc, registration order asc).
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if e.topN > 0 && len(results) > e.topN {
		results = results[:e.topN]
	}

	return results
}

// diagnoseComposite walks the constituent matchers of a CompositeMatcher and
// emits one DimensionScore per matcher. Matchers implementing
// matcher.DiagnosticMatcher contribute structured diagnostics; matchers that
// don't (e.g. matcher.AlwaysMatch or custom user matchers) contribute a
// generic dimension based on ScoreMatch + String().
//
// We chose option (a) from the task breakdown: walk the composite's children
// via the existing CompositeMatcher.Matchers() accessor. Reasoning:
//
//   - Reuses the exact composition built by stub.BuildMatcher (the same one
//     primary matching uses), guaranteeing the diagnostic surface mirrors
//     what was actually evaluated.
//   - Keeps stub.BuildMatcher's return type stable (*CompositeMatcher) — no
//     callers need to switch over a slice-vs-composite return.
//   - The accessor already exists on CompositeMatcher, so this is purely a
//     consumption choice with zero new public API on the matcher package.
func diagnoseComposite(c *matcher.CompositeMatcher, req *http.Request) (breakdown []spec.DimensionScore, totalScore, totalMax int, fullMatch bool) {
	children := c.Matchers()
	if len(children) == 0 {
		// Match-anything stub: behaves as a full match with zero score.
		return nil, 0, 0, true
	}

	breakdown = make([]spec.DimensionScore, 0, len(children))
	fullMatch = true

	for idx, m := range children {
		var ds spec.DimensionScore

		if dm, ok := m.(matcher.DiagnosticMatcher); ok {
			d := dm.Diagnose(req)
			ds = spec.DimensionScore{
				Dimension: d.Dimension,
				Matched:   d.Matched,
				Score:     d.Score,
				MaxScore:  d.MaxScore,
				Expected:  d.Expected,
				Actual:    d.Actual,
				Reason:    d.Reason,
			}
		} else {
			// Generic fallback for matchers without Diagnose support.
			//
			// This branch is defensive: stub.BuildMatcher only ever produces
			// matchers that already implement DiagnosticMatcher, so in
			// production paths the type assertion above always succeeds. The
			// fallback exists so that user-supplied custom matchers (added via
			// future extension points) still produce a usable diagnostic. It
			// is exercised in tests via a white-box test file that calls
			// diagnoseComposite directly with a non-DiagnosticMatcher child.
			matched, score := m.ScoreMatch(req)
			ds = spec.DimensionScore{
				Dimension: fallbackDimension(idx),
				Matched:   matched,
				Score:     score,
				MaxScore:  score, // best we can infer without DiagnosticMatcher
				Expected:  m.String(),
			}
			if !matched {
				ds.Reason = fmt.Sprintf("matcher %q did not match", m.String())
			}
		}

		if !ds.Matched {
			fullMatch = false
		}
		totalScore += ds.Score
		totalMax += ds.MaxScore
		breakdown = append(breakdown, ds)
	}

	return breakdown, totalScore, totalMax, fullMatch
}

// fallbackDimension synthesizes a stable Dimension label for matchers that do
// not implement DiagnosticMatcher. The index suffix avoids collisions when
// multiple custom matchers share an identical String(). The "custom:" prefix
// signals to consumers that the underlying matcher carried no structured
// diagnostic; the matcher's String() is exposed via Expected on the same
// DimensionScore for context.
func fallbackDimension(idx int) string {
	return fmt.Sprintf("custom:%d", idx)
}

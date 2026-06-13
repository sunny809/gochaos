# ADR-003: Matcher Scoring with (bool, int)

## Status

Accepted

## Context

The request matching engine must determine which stub best matches an incoming request. Multiple stubs may partially match (e.g., same path but different headers). We need a way to:
1. Decide if a stub matches at all (boolean)
2. Rank matching stubs to find the "best" match (score)
3. Support future near-miss diagnostics that explain *why* a stub didn't match

A simple boolean "matched / not matched" is insufficient: when multiple stubs match, we need to pick the most specific one. Conversely, when no stub matches, we need to report which stubs came closest.

## Decision

Each matcher returns `(bool, int)` where:
- `bool` — whether this dimension matched
- `int` — the specificity score for this dimension (higher = more specific)

The matching engine aggregates scores across all dimensions. A stub is considered matched only if all required dimensions match (boolean AND). Among matched stubs, the one with the highest total score wins.

Scoring rules:
- Exact match (e.g., `urlPath: /api/users`) scores higher than regex match
- Regex match scores higher than wildcard (`*`)
- Header match scores higher than absent-header check (`!`)
- Each matching dimension contributes its score to the total
- Priority field on the stub overrides the score-based ordering (lower priority value = higher precedence)

Interface:

```go
type Matcher interface {
    Match(req *http.Request) (matched bool, score int)
    String() string
}
```

## Consequences

**Positive:**
- Enables precise "best match" selection when multiple stubs match
- Provides foundation for near-miss diagnostics (Slice 10)
- Scores are opaque integers — easy to extend with new matchers
- Priority override allows users to force ordering when needed

**Negative:**
- Score design is somewhat arbitrary; changing weights is a breaking change
- Two-stage match-then-rank is slightly more expensive than a single boolean check
- Must document scoring rules so users can predict match behavior

## Alternatives Considered

- **Single boolean per matcher, priority only**: Rejected because it cannot distinguish between "matched with 1 dimension" and "matched with 5 dimensions" when both have the same priority
- **Float scores (0.0 to 1.0)**: Rejected because integer scores are simpler, avoid precision issues, and are sufficient for ranking
- **Separate Match() and Score() methods**: Rejected because it requires two passes over matchers; the combined `(bool, int)` is more efficient

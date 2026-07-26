# AI Code Review Found 7 Bugs: My Methodology

*How to do code review with AI at "max effort" — and why it matters.*

## The Problem

Code review is the most important quality gate in software engineering. It's also
the most skipped, rushed, or敷衍 (half-hearted) step — especially when you're
working alone on an open-source project.

When I was building gochaos, I needed a review methodology that could catch real
bugs without a team of reviewers. The answer: a structured, multi-angle AI code
review.

## The Methodology

### Phase 1: The 10 Angles

Instead of asking "find bugs," I ask 10 independent questions, each from a
different perspective:

| # | Angle | What it hunts for |
|---|-------|-------------------|
| 1 | Line-by-line diff scan | Every line, every condition, every edge case |
| 2 | Removed-behavior audit | What was deleted and what replaced it |
| 3 | Cross-file tracer | Callers that break, callees that change |
| 4 | Language-pitfall specialist | Go-specific footguns (nil map, range var, etc.) |
| 5 | Wrapper/proxy correctness | Does the wrapping type delegate correctly? |
| 6 | Reuse | Does this reimplement something we already have? |
| 7 | Simplification | Is there unnecessary complexity? |
| 8 | Efficiency | Redundant work, wasted allocations |
| 9 | Altitude | Is this fix at the right depth? |
| 10 | Conventions | Does the code follow project CLAUDE.md rules? |

### Phase 2: Verify or Refute

Every candidate finding gets a verifier. The verifier is told to try to
**refute** the finding — not confirm it. This adversarial process eliminates
false positives.

The verdict is one of three:
- **CONFIRMED**: can name the inputs/state that trigger it
- **PLAUSIBLE**: mechanism is real, trigger is uncertain
- **REFUTED**: factually wrong or guarded elsewhere

### Phase 3: Sweep for Gaps

The sweep is a fresh reviewer who sees only the findings list and the diff.
Their job: find what the first pass missed. This catches things like:
- Setup/teardown asymmetry in tests
- Config defaults flipped
- Lock scope that shrank during refactoring

## The Result

In one review of a 305-line commit (async callback feature), this methodology
found 7 bugs:

1. Data race on `ssrfBypass` field (bool → atomic.Bool)
2. `init()` used for business logic (violates project convention)
3. Response body not drained before Close (prevents connection reuse)
4. WaitInFlight goroutine leak (helper goroutine not bounded by context)
5. Template cache unbounded growth (no eviction mechanism)
6. Missing callback log clear in admin reset handler
7. Wrong indentation (misleading control flow)

## Why This Matters

Most solo developers skip code review because "there's no one to review it."
AI-assisted review fills this gap — but only if you do it methodically.

A single "review my code" prompt is like asking "what's wrong with this?" —
you'll get surface-level feedback. A structured 10-angle review with adversarial
verification is like having a team of specialists.

## The Pattern

This methodology is reusable. You can apply it to any codebase, any language,
any diff size:

1. **Fan out** — ask 10 independent questions
2. **Verify** — try to refute each finding
3. **Sweep** — check for gaps
4. **Fix** — address confirmed findings
5. **Re-review** — verify fixes didn't introduce new bugs
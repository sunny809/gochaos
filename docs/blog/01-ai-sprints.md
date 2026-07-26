# Building gochaos in 8 AI-Assisted Sprints

*How to build a production-quality open-source tool in 2 months with AI assistance.*

## The Challenge

Could you build a production-quality open-source tool — with 7 fault types, 5 delay
distributions, 3 activation modes, seedable RNG, and CI-gateable assertions — in
about 2 months, with AI as your co-pilot?

That was the question I set out to answer with gochaos (gmock), a Go-native HTTP
mock server for chaos testing.

## Our Process

### Sprint Structure

Each sprint followed a 6-role workflow:

```
Kickoff (PO) → Design (Tech Lead) → Develop (Developer) → 
Test (QA) → Review (Tech Lead) → Release (PO+SM)
```

These are not titles — they're **roles with fixed templates**. Every sprint
produces the same artifacts: requirements, design doc, test report, code review
summary, definition of done.

### What AI Did

- **Product Owner**: AI helped translate vague requirements into structured user
  stories with acceptance criteria
- **Tech Lead**: AI reviewed architecture decisions against project constraints
  and conventions
- **Developer**: AI wrote the implementation based on the design doc
- **QA**: AI designed test scenarios and verified coverage
- **Code Reviewer**: AI reviewed every PR for correctness bugs, efficiency, and
  conventions

### What the Human Did

- Made all final decisions
- Set the vision and product direction
- Rejected bad suggestions
- Approved good ones
- Maintained the big picture

## Results

- 8 sprints, ~40 features, 0 critical bugs in production
- 47 BDD scenarios, 220 steps, all passing with `-race`
- Code review caught 7 bugs before they shipped
- 100% of tests pass with the race detector

## Lessons

1. **AI is a multiplier, not a replacement** — the human still owns decisions
2. **Process matters more than tools** — structured sprints beat ad-hoc coding
3. **Review is non-negotiable** — even AI-written code needs review
4. **Start with the positioning** — knowing what you're NOT building is as
   important as knowing what you are

## The Takeaway

The question isn't "will AI replace developers?" It's "what can a developer
achieve *with* AI that they couldn't alone?"

For me, the answer was: build a production-grade open-source tool in 2 months
that fills a real gap in the ecosystem.
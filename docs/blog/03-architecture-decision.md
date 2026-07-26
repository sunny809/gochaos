# From SLA Simulator to Fault-Burst Generator: An Architecture Decision

*Why the most important decision I made was what NOT to build.*

## The Original Vision

When I started gochaos, the vision was an "SLA-driven chaos simulation server."
You'd declare `availability: 99.9%`, `p99 < 200ms`, and the server would
automatically drive responses to match that SLA over time. It was ambitious,
complicated — and wrong.

## The Pivot

Three months in, I realized the SLA engine was never going to ship. It was
specified, designed, but never built. The reason? It was addressing the wrong
question.

### Producer View vs Consumer View

```
PRODUCER VIEW — "I run InventoryService"
- I publish an SLA: 99.9% available, p99 < 200ms
- I monitor SLIs in prod: error rate, latency histograms
- Question: am I keeping my promise?

CONSUMER VIEW — "I run OrderService, which calls Inventory"
- I assume Inventory will sometimes break
- I write retries, timeouts, circuit breakers, fallbacks
- I want to test those resilience strategies
- Question: do I survive when Inventory misbehaves?
```

A mock server is a tool used by **consumers**. It has no business pretending to
*meet* an SLA — only to *break* one on demand.

## The Insight

SLA is the producer's view. Resilience testing is the consumer's view. They're
two different questions, answered by two different tools:

- "Does this service meet its availability target?" → SLO/SLI tooling in
  production (Prometheus, Honeycomb, Datadog)
- "Does this service survive when its dependencies fail?" → chaos testing in
  pre-production

A mock server that tries to do both does neither well.

## The Result

The SLA engine was retired. In its place: a "fault-burst generator" — a tool
that rapidly reproduces high-failure, high-latency, and high-unreliability
conditions in seconds-to-minutes windows.

This shift unlocked three properties that define gochaos today:

1. **Burst-shaped** — faults activate by probability, request count, or time
   window, not just always-on
2. **Reproducible** — seedable RNG makes every fault sequence deterministic
3. **Observable** — every fault is logged and CI-gateable

## The Lesson

The best architecture decisions are often the ones you make about what NOT to
build. Every feature that ships is a bet on what matters. Every feature that's
cut is a bet on what doesn't.

The question isn't "can we build this?" — it's "should we build this, given who
we're building for?"
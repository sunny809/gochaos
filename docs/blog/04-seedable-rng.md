# Why My Mock Server Needs Seedable RNG

*Probabilistic chaos is useless if it isn't debuggable.*

## The Problem

When you run a chaos experiment with a 30% error rate, you have a problem:
every run produces a different sequence of failures. In CI, the test might pass
10 times then fail once — and you can't reproduce the failure because the RNG
seed was different.

This is the #1 reason teams don't run probabilistic chaos tests in CI. They
can't afford the flakiness.

## The Solution: Seedable RNG

gochaos's `WithRandSeed(42)` makes every probabilistic decision derive from a
configurable seed. Same seed → same fault sequence. Every time.

```go
server := gmock.NewServer(
    gmock.WithPort(0),
    gmock.WithRandSeed(42),  // ← deterministic faults
)
```

## How It Works

Internally, gochaos maintains a deterministic random number generator (RNG) that
is seeded once at server creation. Every probabilistic decision — "should this
request fail?" — is a deterministic function of the seed and the request count.

This means:
- **Same seed + same request sequence = same fault pattern**
- **Different seed = different fault pattern** (for wider coverage)
- **No seed = unpredictable** (for production-like chaos)

## Why This Matters for CI

The classic pattern is a test matrix:

```go
func TestPaymentServiceMatrix(t *testing.T) {
    for _, seed := range []int64{42, 43, 44, 45} {
        t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
            server := gmock.NewServer(
                gmock.WithPort(0),
                gmock.WithRandSeed(seed),
            )
            // ... same experiment, different fault sequence ...
        })
    }
}
```

Each seed exercises a different fault sequence. If seed 42 passes and seed 43
fails, you can reproduce seed 43's failure locally — same seed, same result.

## The Debugging Story

Without seedable RNG, a CI failure looks like:
> "Test failed — sometimes. Might be flaky. Let's re-run."

With seedable RNG, it looks like:
> "Test failed with seed 43. I can reproduce it locally. The fault sequence
> was: requests 3, 7, 12, 14 got errors. Request 8 got a 2-second delay.
> Request 9 got a connection reset. The SUT's circuit breaker tripped at
> request 12 and didn't recover."

One is a guessing game. The other is a debugging session.

## The Takeaway

Seedable RNG is not a nice-to-have for chaos testing — it's a prerequisite for
CI-gating. Without it, you're not running chaos experiments. You're rolling
dice.
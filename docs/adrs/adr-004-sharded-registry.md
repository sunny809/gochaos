# ADR-004: Sharded Stub Registry

- **Status**: Superseded
- **Date**: 2026-01-15 (original), 2026-06-13 (updated to reflect actual implementation)
- **Deciders**: Sunny

## Technical Story

The stub registry needs to store and retrieve stub mappings concurrently. Multiple goroutines may register stubs while requests are being matched and served.

## Context

The original design proposed a sharded concurrent map using xxhash to distribute keys across 64 shards, each with its own RWMutex, to maximize throughput under high concurrency. However, during implementation it was decided that a simpler approach was sufficient for the project's expected load profile.

## Decision

Adopt a flat `map[string]*StubDefinition` protected by a single `sync.RWMutex`.

The actual implementation in `internal/stub/registry.go`:

```go
type Registry struct {
    mu    sync.RWMutex
    stubs map[string]*spec.StubDefinition
    priorities []string // ordered slice for priority-based matching
}
```

Key design choices:

1. **Flat map keyed by UUID**: Each stub gets a unique ID generated at registration time. Lookups by ID (for admin API operations) are O(1).
2. **Single RWMutex**: Sufficient for the expected concurrency level. Read locks are held during matching; write locks during registration/deregistration.
3. **Priority ordering**: A separate `[]string` slice tracks stub insertion order. When matching, stubs are iterated in insertion order so the first match wins (higher priority = registered earlier).
4. **Priority insertion**: The `AddStub` method accepts a priority index, allowing the admin API or configuration loader to insert stubs at specific positions rather than always appending.

The matching flow:
1. Acquire read lock
2. Iterate stubs in priority order
3. Each stub delegates to its matchers (path, method, headers, query, body)
4. Return first match with its score
5. Release read lock

## Consequences

### Positive

- Simple, easy to reason about implementation
- No external dependency on xxhash
- Sufficient performance for mock server workloads (typically <1000 stubs, <100 req/s)
- Easy to debug and inspect

### Negative

- Single lock could become a contention point under very high concurrency
- Linear scan of all stubs for each request (not O(1) matching)

### Neutral

- The sharded design is documented here for future reference if performance requirements change
- Migration path: replace the flat map with a sharded map underneath the same interface if needed

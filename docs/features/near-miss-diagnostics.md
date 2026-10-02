# Near-Miss Diagnostics

> When a request doesn't match any stub, gochaos doesn't just return 404 — it tells you
> **why** no stub matched and **which stub came closest**. This transforms 404 debugging
> from "guess and check" into a data-driven process.

## Problem: Blind 404s

Without near-miss diagnostics, an unmatched request returns a dead-end:

```json
{
  "error": "no matching stub",
  "method": "GET",
  "path": "/api/users/42"
}
```

You know **that** it didn't match, but not **why**. Is the path wrong? Method mismatched?
Headers? Body? Near-miss answers those questions.

---

## How Near-Miss Works

When a request arrives and no stub has a perfect match, the near-miss engine:

1. Compares the request against **every registered stub**
2. For each stub, scores each **configured** matching dimension and sums the results
3. Returns the top-N **near misses** (default top-N: 5) — stubs that came closest, best first

Each near miss shows a `topMissReason` (the single most useful fix) plus, via the
admin endpoint, a **per-dimension breakdown** of what matched and what didn't.

### How Scoring Works

Each stub's maximum score is the **sum of the weights of the dimensions it
configures** — it is not a fixed total. A stub that defines only `method` + `urlPath`
scores out of 10 + 30 = 40; a full-pattern stub scores out of more.

| Dimension | Max score | Notes |
|-----------|-----------|-------|
| `urlPath` (exact) | 30 | Exact path match |
| `urlPathRegex` | 30 | Regex path match, as a path matcher |
| `method` | 10 | HTTP method |
| body (JSONPath) | 20 | JSONPath body match |
| body (regex) | 12 | `regexMatch` |
| body (exact) | 10 | `exactMatch` |
| `accept` | 7 | Media-type negotiation |
| `headers` | 5 | Header name → pattern |
| `cookies` | 4 | Cookie name → pattern |
| `queryParams` | 3 | Query param → pattern |
| `priority` | — | Tie-break only, not scored |

> Only dimensions the stub actually configures are scored (and only the ones with
> matchers contribute). `maxScore` in the output is that stub's per-dimension maxes
> summed — so a method+path stub reports `maxScore: 40`, not a blanket 8.

---

## Near-Miss in 404 Responses

The easiest way to use near-miss: it's **automatically included** in every 404 response.

```bash
# Register a stub with a similar path
curl -X POST http://localhost:8080/__admin/mappings \
  -H 'Content-Type: application/json' \
  -d '{"request":{"method":"GET","urlPath":"/api/user"},"response":{"status":200,"body":"\"ok\""}}'

# Make a request to a slightly different path
curl -v http://localhost:8080/api/users
```

**Response** (status 404):

```json
{
  "error": "no matching stub",
  "method": "GET",
  "path": "/api/users",
  "nearMisses": [
    {
      "stubId": "abc-123",
      "stubName": "",
      "score": 10,
      "maxScore": 40,
      "topMissReason": "path /api/users does not equal /api/user"
    }
  ]
}
```

**Reading it**: the stub `/api/user` scored 10/40 — the **path** dimension mismatched
(`/api/users` vs `/api/user`). `topMissReason` states the one fix. The 404 payload is
deliberately slim: for the full per-dimension breakdown, call `POST /__admin/nearmiss`.

---

## Near-Miss Admin API

For programmatic access, use the dedicated endpoint — it returns the **full
per-dimension breakdown** the 404 payload omits:

```
POST /__admin/nearmiss
```

Submit a request; the engine compares it against all registered stubs without
processing the request itself.

### cURL Example

```bash
# Submit a hypothetical request to see what would match
curl -X POST http://localhost:8080/__admin/nearmiss \
  -H 'Content-Type: application/json' \
  -d '{
    "method": "GET",
    "path": "/api/users",
    "headers": {"Accept": "application/json"}
  }'
```

**Response** (200 OK):

```json
{
  "meta": { "topN": 5, "total": 1 },
  "nearMisses": [
    {
      "stubId": "abc-123",
      "stubName": "get-user-v1",
      "score": 18,
      "maxScore": 45,
      "reason": "",
      "breakdown": [
        { "dimension": "method", "matched": true,  "score": 10, "maxScore": 10, "expected": "GET", "actual": "GET" },
        { "dimension": "path",   "matched": false, "score": 0,  "maxScore": 30, "expected": "/api/user", "actual": "/api/users", "reason": "path /api/users does not equal /api/user" },
        { "dimension": "accept", "matched": true,  "score": 7,  "maxScore": 7,  "expected": "application/json", "actual": "application/json" },
        { "dimension": "headers","matched": true,  "score": 1,  "maxScore": 5 }
      ]
    }
  ]
}
```

### Go Library API

```go
results := server.NearMiss("GET", "/api/users", map[string]string{
    "Accept": "application/json",
}, "")

for _, nm := range results {
    fmt.Printf("Stub %s scored %d/%d: %s\n", nm.StubID, nm.Score, nm.MaxScore, nm.Reason)
    for _, d := range nm.Breakdown {
        if !d.Matched {
            fmt.Printf("  %s: expected=%q actual=%q\n", d.Dimension, d.Expected, d.Actual)
        }
    }
}
```

> `NearMissResult` fields: `StubID`, `StubName`, `Score`, `MaxScore`, `Breakdown`
> (`[]DimensionScore`: `Dimension`, `Matched`, `Score`, `MaxScore`, `Expected`,
> `Actual`, `Reason`) and `Reason`. The full result is only returned from the admin
> endpoint and `NearMiss` library call; the 404 payload carries the slim
> `stubId/stubName/score/maxScore/topMissReason` projection.

---

## When to Use Near-Miss

| Scenario | Method | Why |
|----------|--------|-----|
| Quick debugging during development | Check 404 body `nearMisses` field | Zero setup — included automatically |
| CI failure investigation | Check 404 body in test output | No extra API calls needed |
| Automated stub validation | `POST /__admin/nearmiss` | Test that your stubs will match expected requests |
| Pre-deployment stub audit | `POST /__admin/nearmiss` with known request patterns | Catch stub configuration errors before they reach production |

---

## Responsive Reference

### 404 near-miss entry

| Field | Type | Description |
|-------|------|-------------|
| `stubId` | string | ID of the near-miss stub |
| `stubName` | string | Optional human-readable name (omitted when empty) |
| `score` | int | Sum of matched-dimension scores for this stub |
| `maxScore` | int | Max possible for this stub's configured dimensions |
| `topMissReason` | string | The single most useful mismatch, human-readable |

### Admin endpoint near-miss entry

| Field | Type | Description |
|-------|------|-------------|
| `stubId` / `stubName` | string | Stub identification |
| `score` / `maxScore` | int | As above |
| `breakdown` | array | Per-dimension `DimensionScore` objects (dimension, matched, score, maxScore, expected, actual, reason) |
| `reason` | string | Empty unless a dimension-level reason applies |
| `meta.topN` / `meta.total` | int | Configured limit and total candidates scanned |

---

## Pitfalls

1. **No stub registered** → `nearMisses` is empty. There's nothing to compare against.
2. **Many stubs** → Near-miss scans all stubs. With <100 stubs this is sub-millisecond.
   For very large registries, consider narrowing the stub set.
3. **Body matching** → Request bodies are read during near-miss. For large bodies,
   the engine captures up to the first 64KB.
4. **Only dimensions the stub configures are scored** → A stub defining only method +
   path scores out of 40, not 8 — `maxScore` reflects what that stub actually checks.
5. **`topMissReason` ≠ fix guarantee** → It names the closest mismatch, which is
   usually the fix, but a request may be near several stubs at once.

---

## Full Example

See [examples/verification/](../examples/verification/) for a runnable example that
uses the verification and near-miss APIs together, and
[stub-matching.md](stub-matching.md) for the full matcher reference.
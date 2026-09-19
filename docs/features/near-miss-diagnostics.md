# Near-Miss Diagnostics

> When a request doesn't match any stub, gochaos doesn't just return 404 — it tells you
> **why** no stub matched and **which stub came closest**. This transforms 404 debugging
> from "guess and check" into a data-driven process.

## Problem: Blind 404s

Without near-miss diagnostics, an unmatched request returns a dead-end:

```json
{
  "error": "no stub matched",
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
2. For each stub, scores all 8 matching dimensions (method, path, headers, query, body, cookies, accept, priority)
3. Returns the top-N **near misses** — stubs that came closest to matching

Each near miss shows a **per-dimension breakdown** of what matched and what didn't.

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
  "error": "no stub matched",
  "method": "GET",
  "path": "/api/users",
  "nearMisses": [
    {
      "stubId": "abc-123",
      "name": "",
      "priority": 0,
      "scoreBreakdown": {
        "method": {"score": 1, "maxScore": 1, "matched": true},
        "path":   {"score": 0, "maxScore": 1, "matched": false, "expected": "/api/user", "actual": "/api/users"},
        "headers":{"score": 1, "maxScore": 1, "matched": true},
        "query":  {"score": 1, "maxScore": 1, "matched": true},
        "body":   {"score": 1, "maxScore": 1, "matched": true},
        "cookies":{"score": 1, "maxScore": 1, "matched": true},
        "accept": {"score": 1, "maxScore": 1, "matched": true}
      },
      "totalScore": 6,
      "maxPossibleScore": 7
    }
  ]
}
```

**Reading the breakdown**: The stub `/api/user` scored 6/7 — the **path** dimension
mismatched (`/api/users` vs `/api/user`). That's your debugging hint.

---

## Near-Miss Admin API

For programmatic access, use the dedicated endpoint:

```
POST /__admin/nearmiss
```

Submit a request body; the engine compares it against all registered stubs and returns
the near-miss breakdown without actually processing the request.

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
  "nearMisses": [
    {
      "stubId": "abc-123",
      "name": "get-user-v1",
      "scoreBreakdown": {
        "method":  {"score": 1, "maxScore": 1, "matched": true},
        "path":    {"score": 0, "maxScore": 1, "matched": false},
        "headers": {"score": 1, "maxScore": 1, "matched": true}
      },
      "totalScore": 6,
      "maxPossibleScore": 7
    }
  ]
}
```

### Go Library API

```go
result, err := server.ComputeNearMiss(gmock.RequestPattern{
    Method:  "GET",
    URLPath: "/api/users",
})
if err != nil {
    t.Fatal(err)
}
for _, nm := range result.NearMisses {
    fmt.Printf("Stub %s scored %d/%d\n", nm.StubID, nm.TotalScore, nm.MaxPossibleScore)
    for dim, breakdown := range nm.ScoreBreakdown {
        if !breakdown.Matched {
            fmt.Printf("  %s: expected=%q, actual=%q\n", dim, breakdown.Expected, breakdown.Actual)
        }
    }
}
```

---

## When to Use Near-Miss

| Scenario | Method | Why |
|----------|--------|-----|
| Quick debugging during development | Check 404 body `nearMisses` field | Zero setup — included automatically |
| CI failure investigation | Check 404 body in test output | No extra API calls needed |
| Automated stub validation | `POST /__admin/nearmiss` | Test that your stubs will match expected requests |
| Pre-deployment stub audit | `POST /__admin/nearmiss` with known request patterns | Catch stub configuration errors before they reach production |

---

## Near-Miss Response Reference

### ScoreBreakdown

| Field | Type | Description |
|-------|------|-------------|
| `score` | int | Actual score for this dimension (0 or 1) |
| `maxScore` | int | Maximum possible score (always 1 for current matchers) |
| `matched` | bool | Whether this dimension matched |
| `expected` | string | *(included when not matched)* What the stub expected |
| `actual` | string | *(included when not matched)* What the request provided |

### NearMissEntry

| Field | Type | Description |
|-------|------|-------------|
| `stubId` | string | UUID of the near-miss stub |
| `name` | string | Optional human-readable name |
| `priority` | int | Stub priority (lower = higher) |
| `scoreBreakdown` | object | Per-dimension breakdown (see above) |
| `totalScore` | int | Sum of all dimension scores |
| `maxPossibleScore` | int | Maximum possible total (8 dimensions) |
| `matchingRatio` | float | `totalScore / maxPossibleScore` |

---

## Pitfalls

1. **No stub registered** → `nearMisses` is empty. There's nothing to compare against.
2. **Many stubs** → Near-miss scans all stubs. With <100 stubs this is sub-millisecond.
   For very large registries, consider narrowing the stub set.
3. **Body matching** → Request bodies are read during near-miss. For large bodies,
   the engine captures up to the first 64KB.
4. **Only dimensions with matchers are scored** → If a stub only defines method + path,
   only those 2 dimensions affect the score (maxPossibleScore = 2).

---

## Full Example

See [examples/nearmiss/](../examples/nearmiss/main_test.go) for a complete, runnable
example demonstrating near-miss debugging and programmatic access.
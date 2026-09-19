# Stub Matching

> gochaos matches incoming requests against registered stubs across 8 dimensions.
> Each dimension returns a score; the stub with the highest total score wins.

## Matching Dimensions

| Dimension | Field | Type | Description |
|-----------|-------|------|-------------|
| Method | `request.method` | exact | HTTP method (GET, POST, etc.). Empty = wildcard. |
| Path | `request.urlPath` | exact | Exact URL path match. |
| Path (regex) | `request.urlPathRegex` | regex | Regex compiled at registration time. |
| Accept | `request.accept` | media type | Proper media type negotiation with wildcards and quality values. |
| Headers | `request.headers` | regex (value) | Header name → regex pattern for value. |
| Query | `request.queryParams` | regex (value) | Query param name → regex pattern for value. |
| Cookies | `request.cookies` | regex (value) | Cookie name → regex pattern for value. |
| Body | `request.body` | exact/regex/JSONPath | Three body matching strategies (see below). |

## Body Matching Strategies

```go
// Exact match — the body must be byte-for-byte identical
Body: &gmock.BodyPattern{
    ExactMatch: `{"name":"Alice"}`,
}

// Regex match — the body must match the pattern
Body: &gmock.BodyPattern{
    RegexMatch: `"name":"[A-Z][a-z]+"`,
}

// JSONPath match — the body must satisfy a JSONPath expression
Body: &gmock.BodyPattern{
    JSONPath: `$.users[?(@.age > 18)]`,
}
```

## Priority Ordering

Stubs can have explicit priority values. Lower numbers = higher priority.

```go
// This stub matches first for GET /api/users/1
server.Stub(gmock.StubDefinition{
    Priority: 1,
    Request:  gmock.RequestPattern{Method: "GET", URLPath: "/api/users/1"},
    Response: gmock.ResponseDefinition{Body: `{"name":"VIP"}`},
})

// This stub is the fallback for all GET /api/users/*
server.Stub(gmock.StubDefinition{
    Priority: 10,
    Request:  gmock.RequestPattern{Method: "GET", URLPathRegex: "^/api/users/"},
    Response: gmock.ResponseDefinition{Body: `{"name":"default"}`},
})
```

## Matching Algorithm

1. Iterate all stubs in priority order (lower priority value first)
2. For each stub, score all 8 dimensions
3. Sum the dimension scores (each dimension contributes 1 if matched, 0 if not)
4. The stub with the highest total score wins
5. If no stub scores > 0, return 404

## Unmatched Requests

When no stub matches, the server returns:

```json
{
  "error": "no stub matched",
  "method": "GET",
  "path": "/api/unknown",
  "query": "page=1"
}
```

## Stub Priority Reference

| Priority | Behavior |
|----------|----------|
| 0 (default) | Normal priority |
| 1-99 | Higher than default (more specific stubs) |
| 100+ | Lower than default (catch-all / fallback stubs) |
| Negative | Reserved for internal use |

## Complete Example

See [examples/basic/](../examples/basic/main.go) for path matching,
and the [Admin API](../admin-api.md) for JSONPath body matching examples.

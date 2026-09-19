# Response Templating

> gochaos supports Go `text/template`-based response templating. Templates have
> access to the incoming request and built-in helper functions for generating
> dynamic responses.

## Enabling Templating

Set `TransformResponse: true` on the response definition:

```go
server.Stub(gmock.StubDefinition{
    Request: gmock.RequestPattern{
        Method:  http.MethodGet,
        URLPath: "/api/dynamic",
    },
    Response: gmock.ResponseDefinition{
        Status: http.StatusOK,
        Headers: map[string]string{
            "Content-Type": "application/json",
        },
        Body: `{"path":"{{.Request.Path}}","method":"{{.Request.Method}}","uuid":"{{randomUUID}}","timestamp":"{{now}}"}`,
        TransformResponse: true, // <-- enables template rendering
    },
})
```

## Template Variables

Access request properties through `.Request`:

| Expression | Returns | Example |
|-----------|---------|---------|
| `{{.Request.Method}}` | HTTP method string | `GET` |
| `{{.Request.Path}}` | URL path | `/api/users/42` |
| `{{.Request.Header "Content-Type"}}` | Header value | `application/json` |
| `{{.Request.Query "page"}}` | Query parameter | `1` |

## Template Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `randomUUID` | `func() string` | Generates a random UUID v4 |
| `now` | `func() string` | Current UTC timestamp in RFC3339 format |
| `randomInt` | `func(min, max int) int` | Random integer in range [min, max] |

## Examples

### Echo Request Path

```go
Body: `{"requestedPath":"{{.Request.Path}}"}`,
```

Input: `GET /api/users/42`
Output: `{"requestedPath":"/api/users/42"}`

### Generate Unique IDs

```go
Body: `{"orderId":"{{randomUUID}}","createdAt":"{{now}}"}`,
```

Output: `{"orderId":"a1b2c3d4-...","createdAt":"2026-06-13T12:00:00Z"}`

### Random Data Generation

```go
Body: `{"randomAge":{{randomInt 18 99}},"name":"User-{{randomInt 1000 9999}}"}`,
```

Output: `{"randomAge":42,"name":"User-7351"}`

### Combine All

```go
Body: `{
  "timestamp": "{{now}}",
  "method": "{{.Request.Method}}",
  "path": "{{.Request.Path}}",
  "uuid": "{{randomUUID}}",
  "duration": {{randomInt 50 500}}
}`,
```

### Admin API (JSON)

```json
{
  "request": { "method": "GET", "urlPath": "/api/dynamic" },
  "response": {
    "status": 200,
    "headers": { "Content-Type": "application/json" },
    "body": "{\"path\":\"{{.Request.Path}}\",\"uuid\":\"{{randomUUID}}\"}",
    "transformResponse": true
  }
}
```

### Admin API (YAML)

```yaml
request:
  method: GET
  urlPath: /api/dynamic
response:
  status: 200
  headers:
    Content-Type: application/json
  body: '{"path":"{{.Request.Path}}","uuid":"{{randomUUID}}"}'
  transformResponse: true
```

## Important Notes

- Uses Go's **`text/template`** (NOT `html/template`) — no HTML escaping
- Templates are cached after first parse (compiled once, reused)
- If template parsing fails, the raw body is returned with a warning log
- `TransformResponse: false` (default) returns the body as-is

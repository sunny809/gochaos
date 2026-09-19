# CORS Support

> gochaos supports Cross-Origin Resource Sharing (CORS) for browser-based clients.
> Both preflight (OPTIONS) and actual requests are handled.

## Enabling CORS

### With Defaults (Allow All)

```go
server := gmock.NewServer(
    gmocks.WithPort(0),
    gmocks.WithCORSEnabled(),
)
```

This configures:
- Allow all origins (`*`)
- Allow standard HTTP methods
- Allow `Content-Type` and `Authorization` headers
- Max age: 86400 seconds (24 hours)

### With Custom Options

```go
server := gmock.NewServer(
    gmocks.WithPort(0),
    gmocks.WithCORS(gmocks.CORSOptions{
        AllowedOrigins:   []string{"https://myapp.com", "https://admin.example.com"},
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
        AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Request-ID"},
        ExposedHeaders:   []string{"X-RateLimit-Remaining", "X-Request-ID"},
        AllowCredentials: true,
        MaxAge:           3600, // 1 hour
    }),
)
```

### CLI Mode

```bash
# Enable CORS with default settings
gmock start --port 8080 --cors
```

## How CORS Works

### Preflight Request (OPTIONS)

When a browser sends a preflight OPTIONS request with an `Origin` header, gochaos responds
with the appropriate CORS headers:

```
Access-Control-Allow-Origin:  https://myapp.com
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH
Access-Control-Allow-Headers: Content-Type, Authorization, X-Request-ID
Access-Control-Max-Age:       3600
```

### Actual Request

For actual cross-origin requests (GET, POST, etc.), gochaos adds:

```
Access-Control-Allow-Origin:      https://myapp.com
Access-Control-Expose-Headers:    X-RateLimit-Remaining
Access-Control-Allow-Credentials: true
```

### Unmatched Requests

CORS headers are also applied to 404 responses for unmatched requests, so browsers
can properly read the error response.

## CORSOptions Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `AllowedOrigins` | `[]string` | `["*"]` | Allowed origins. `*` = all origins. |
| `AllowedMethods` | `[]string` | All standard methods | HTTP methods allowed. |
| `AllowedHeaders` | `[]string` | `["Content-Type", "Authorization"]` | Allowed request headers. |
| `ExposedHeaders` | `[]string` | `[]` | Response headers exposed to JS. |
| `AllowCredentials` | `bool` | `false` | Allow cookies/auth headers. |
| `MaxAge` | `int` | `86400` | Preflight cache TTL (seconds). |

## Full Example

See [examples/cors/](../examples/cors/main.go) for a complete, runnable example.

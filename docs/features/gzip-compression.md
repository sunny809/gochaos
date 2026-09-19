# Gzip Compression

> gochaos automatically compresses responses with gzip when the client includes
> `Accept-Encoding: gzip` in the request.

## How It Works

1. Client sends request with `Accept-Encoding: gzip`
2. gochaos detects the header and wraps the response writer in a gzip compressor
3. Response is compressed transparently
4. `Content-Encoding: gzip` header is set automatically
5. Client decompresses normally

## Client Example

```go
// When using Go's default http.Client, gzip is handled automatically
req, _ := http.NewRequest("GET", server.URL()+"/api/data", nil)
req.Header.Set("Accept-Encoding", "gzip")

// Go's http.Client automatically decompresses gzip responses
resp, _ := http.DefaultClient.Do(req)
body, _ := io.ReadAll(resp.Body) // Already decompressed
```

## Disabling Gzip

```go
// Library mode
server := gmock.NewServer(
    gmocks.WithPort(0),
    gmocks.WithGzip(false),
)

// CLI mode — not directly available (library only)
```

## Gzip + Faults

Fault injection responses bypass gzip compression. Faults are written directly
to the original (unwrapped) ResponseWriter. This ensures:
- Error bodies are readable without decompression
- Connection reset works at the TCP level
- Empty responses are truly empty

## How Gzip Works Internally

```
Response pipeline:
1. Delay (applied before anything)
2. Fault? → write directly to original ResponseWriter → return
3. Check Accept-Encoding → wrap with GzipResponseWriter
4. CORS headers
5. Stub headers
6. Status code
7. Body (compressed through gzip)
```

The `GzipResponseWriter` implements `Unwrap()` for Go 1.20+ ResponseWriter
unwrapping, allowing the fault system to reach the original writer.

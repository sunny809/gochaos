// Package response provides the response writing port and adapters for the gmock server.
//
// This package implements the hexagonal architecture pattern, separating the
// concern of writing HTTP responses from the server lifecycle management.
package response

import (
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
	"github.com/sunny809/gochaos/internal/templating"
)

// CORSOptions configures Cross-Origin Resource Sharing support.
type CORSOptions struct {
	// AllowedOrigins specifies which origins are allowed. Default: ["*"]
	AllowedOrigins []string
	// AllowedMethods specifies which methods are allowed. Default: standard HTTP methods
	AllowedMethods []string
	// AllowedHeaders specifies which headers are allowed. Default: ["Content-Type", "Authorization"]
	AllowedHeaders []string
	// ExposedHeaders specifies which headers are exposed to the browser.
	ExposedHeaders []string
	// AllowCredentials indicates whether credentials (cookies, auth) are allowed.
	AllowCredentials bool
	// MaxAge specifies how long the preflight result can be cached (seconds).
	MaxAge int
}

// HTTPWriter is the concrete adapter that writes HTTP responses.
type HTTPWriter struct {
	logger      *slog.Logger
	disableGzip bool
	tmplEngine  *templating.Engine
}

// NewHTTPWriter creates a new HTTPWriter with the given options.
func NewHTTPWriter(logger *slog.Logger, disableGzip bool) *HTTPWriter {
	return &HTTPWriter{
		logger:      logger,
		disableGzip: disableGzip,
		tmplEngine:  templating.NewEngine(),
	}
}

// WriteResponse writes the stub response to the client.
// It handles: delay, fault injection, gzip, CORS headers, stub headers, status, body (incl. base64).
func (w *HTTPWriter) WriteResponse(rw http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions) error {
	resp := def.Response

	// 1. Apply delay before writing anything (simulates network latency)
	w.applyDelay(resp.Delay)

	// 2. Apply fault on the original ResponseWriter (faults bypass gzip)
	if w.applyFault(rw, resp.Fault, req) {
		return nil
	}

	// 3. Wrap with gzip only if no fault was applied
	gw, rw := w.maybeWrapGzip(rw, req)
	if gw != nil {
		defer func() {
			if err := gw.Close(); err != nil {
				w.logger.Warn("failed to close gzip writer", "error", err)
			}
		}()
	}

	// 4. Apply CORS headers if enabled (before stub-specific headers so they can override)
	w.applyCORSHeaders(rw, corsOpts)

	// 5. Headers
	for k, v := range resp.Headers {
		rw.Header().Set(k, v)
	}

	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	rw.WriteHeader(status)

	if resp.Body != "" {
		body := resp.Body
		if resp.TransformResponse {
			rendered, err := w.tmplEngine.Render(body, req)
			if err != nil {
				w.logger.Warn("template rendering failed", "stub", def.ID, "error", err)
			} else {
				body = rendered
			}
		}
		_, err := io.Copy(rw, strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to write response body: %w", err)
		}
	} else if resp.Base64Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Base64Body)
		if err != nil {
			w.logger.Warn("failed to decode base64 body", "stub", def.ID, "error", err)
		} else {
			_, err = rw.Write(decoded)
			if err != nil {
				return fmt.Errorf("failed to write base64 response body: %w", err)
			}
		}
	}

	return nil
}

// applyDelay sleeps for the duration specified by the DelayDefinition.
// This simulates network latency before the response is sent.
func (w *HTTPWriter) applyDelay(delay *spec.DelayDefinition) {
	if delay == nil {
		return
	}
	switch delay.Type {
	case "fixed":
		time.Sleep(time.Duration(delay.Value) * time.Millisecond)
	case "random":
		min := delay.Min
		max := delay.Max
		if max == 0 {
			max = delay.Value
		}
		if max > 0 {
			d := min
			if max > min {
				d = min + rand.Intn(max-min+1)
			}
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
	}
}

// applyFault injects a network-level fault into the response.
// Returns true if a fault was applied (short-circuits normal response writing),
// false if no fault is configured and normal response writing should proceed.
// Fault responses are written directly to the original ResponseWriter,
// bypassing gzip compression.
func (w *HTTPWriter) applyFault(rw http.ResponseWriter, fault *spec.FaultDefinition, req *http.Request) bool {
	if fault == nil {
		return false
	}

	switch fault.Type {
	case "error":
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(`{"error":"internal server error","fault":"error"}`))
		return true
	case "empty":
		// Send an empty response: no headers, no explicit status (Go defaults to 200),
		// no body. Optionally flush to ensure the response is sent immediately.
		if f, ok := rw.(http.Flusher); ok {
			f.Flush()
		}
		return true
	case "connection_reset":
		// Attempt to Hijack the underlying TCP connection and close it,
		// causing the client to receive a TCP RST (connection reset by peer).
		inner := unwrapResponseWriter(rw)
		if hj, ok := inner.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err != nil {
				// Hijack failed — fall back to 500 close instead of panicking.
				w.logger.Warn("hijack failed, falling back to 500 close", "error", err)
				rw.Header().Set("Connection", "close")
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusInternalServerError)
				_, _ = rw.Write([]byte(`{"error":"connection reset by peer","fault":"connection_reset"}`))
				return true
			}
			// Successfully hijacked — close the connection to send TCP RST.
			_ = conn.Close()
			return true
		}
		// Hijacker not available (e.g. wrapped ResponseWriter in tests or proxies).
		// Fall back to a 500 response with Connection: close.
		w.logger.Warn("hijacker not available, falling back to 500 close")
		rw.Header().Set("Connection", "close")
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(`{"error":"connection reset by peer","fault":"connection_reset"}`))
		return true
	default:
		// Unknown fault types are ignored; normal response proceeds.
		w.logger.Warn("unknown fault type, skipping fault injection", "faultType", fault.Type)
		return false
	}
}

// applyCORSHeaders writes CORS headers for non-preflight (actual) requests.
func (w *HTTPWriter) applyCORSHeaders(rw http.ResponseWriter, opts *CORSOptions) {
	if opts == nil {
		return
	}

	// For actual requests, we need to add Access-Control-Allow-Origin.
	// Without an Origin request header, we use the configured default.
	if len(opts.AllowedOrigins) > 0 {
		rw.Header().Set("Access-Control-Allow-Origin", opts.AllowedOrigins[0])
	}

	// Set exposed headers for actual requests
	if len(opts.ExposedHeaders) > 0 {
		rw.Header().Set("Access-Control-Expose-Headers", strings.Join(opts.ExposedHeaders, ", "))
	}

	// Set credentials
	if opts.AllowCredentials {
		rw.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

// WriteCORSHeaders writes CORS headers for a preflight OPTIONS response.
func (w *HTTPWriter) WriteCORSHeaders(rw http.ResponseWriter, r *http.Request, opts *CORSOptions) {
	if opts == nil {
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	// Set allowed origin
	if len(opts.AllowedOrigins) == 0 || opts.AllowedOrigins[0] == "*" {
		rw.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		for _, allowed := range opts.AllowedOrigins {
			if allowed == origin {
				rw.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
	}

	// Set allowed methods
	if len(opts.AllowedMethods) > 0 {
		rw.Header().Set("Access-Control-Allow-Methods", strings.Join(opts.AllowedMethods, ", "))
	}

	// Set allowed headers
	if len(opts.AllowedHeaders) > 0 {
		rw.Header().Set("Access-Control-Allow-Headers", strings.Join(opts.AllowedHeaders, ", "))
	}

	// Set exposed headers
	if len(opts.ExposedHeaders) > 0 {
		rw.Header().Set("Access-Control-Expose-Headers", strings.Join(opts.ExposedHeaders, ", "))
	}

	// Set credentials
	if opts.AllowCredentials {
		rw.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Set max age
	if opts.MaxAge > 0 {
		rw.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", opts.MaxAge))
	}
}

// GzipResponseWriter wraps http.ResponseWriter to compress writes through gzip.
type GzipResponseWriter struct {
	http.ResponseWriter
	GW *gzip.Writer
}

// Write compresses data through the gzip writer.
func (w *GzipResponseWriter) Write(b []byte) (int, error) {
	return w.GW.Write(b)
}

// Unwrap returns the underlying http.ResponseWriter.
// This supports the Go 1.20+ ResponseWriter unwrapping pattern,
// allowing callers to access interfaces (e.g. http.Hijacker) on the inner writer.
func (w *GzipResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Close flushes the gzip writer.
func (w *GzipResponseWriter) Close() error {
	return w.GW.Close()
}

// unwrapResponseWriter recursively unwraps ResponseWriters that implement
// the Unwrap() http.ResponseWriter interface (Go 1.20+ pattern).
// This is used to reach the underlying writer for operations like Hijack.
func unwrapResponseWriter(rw http.ResponseWriter) http.ResponseWriter {
	for {
		unwrap, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		inner := unwrap.Unwrap()
		if inner == nil {
			break
		}
		rw = inner
	}
	return rw
}

// maybeWrapGzip wraps the ResponseWriter in a gzip compressor if the client
// accepts gzip encoding. Returns the gzip writer (if created) and the
// potentially-wrapped ResponseWriter.
func (w *HTTPWriter) maybeWrapGzip(rw http.ResponseWriter, r *http.Request) (*gzip.Writer, http.ResponseWriter) {
	if w.disableGzip {
		return nil, rw
	}
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		return nil, rw
	}
	rw.Header().Set("Content-Encoding", "gzip")
	gw := gzip.NewWriter(rw)
	return gw, &GzipResponseWriter{
		ResponseWriter: rw,
		GW:             gw,
	}
}

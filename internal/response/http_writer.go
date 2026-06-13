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
// It handles: delay, CORS headers, stub headers, status, body (incl. base64).
func (w *HTTPWriter) WriteResponse(rw http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions) error {
	// Wrap with gzip if client accepts it
	gw, rw := w.maybeWrapGzip(rw, req)
	if gw != nil {
		defer func() {
			if err := gw.Close(); err != nil {
				w.logger.Warn("failed to close gzip writer", "error", err)
			}
		}()
	}

	resp := def.Response

	// Apply delay before writing anything (simulates network latency)
	w.applyDelay(resp.Delay)

	// Apply CORS headers if enabled (before stub-specific headers so they can override)
	w.applyCORSHeaders(rw, corsOpts)

	// Headers
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

// Close flushes the gzip writer.
func (w *GzipResponseWriter) Close() error {
	return w.GW.Close()
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

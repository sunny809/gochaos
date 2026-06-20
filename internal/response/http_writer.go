// Package response provides the response writing port and adapters for the gmock server.
//
// This package implements the hexagonal architecture pattern, separating the
// concern of writing HTTP responses from the server lifecycle management.
package response

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sunny809/gochaos/internal/delayx"
	"github.com/sunny809/gochaos/internal/randx"
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

// dribbleConfig holds the configuration for chunked dribble body writing.
// When non-nil, the body writing phase splits the response body into Chunks
// equal parts, sending each part with an interval of TotalDuration/Chunks
// milliseconds between them.
type dribbleConfig struct {
	chunks        int
	totalDuration int
}

// faultResult captures the outcome of applyFault for use by WriteResponse.
// This avoids mutating shared state on HTTPWriter, which would cause data
// races when WriteResponse is called concurrently from multiple goroutines.
type faultResult struct {
	// applied is true when a fault short-circuited normal response writing.
	applied bool

	// slowCloseMs, when > 0, indicates that after the normal response is
	// fully written, the connection should be hijacked, held open for this
	// many milliseconds, then closed. This implements the "slow_close" fault.
	slowCloseMs int

	// faultType is the type of fault that was applied (for logging).
	faultType string

	// activationMode is the mode that caused the fault to fire (for logging).
	activationMode spec.ActivationMode
}

// delayResult captures the outcome of applyDelay for use by WriteResponse.
type delayResult struct {
	// dribble, when non-nil, indicates that the response body should be
	// written in chunks with delays between them.
	dribble *dribbleConfig
}

// HTTPWriter is the concrete adapter that writes HTTP responses.
// It is safe for concurrent use — all per-request state is passed through
// local variables, not stored on the struct.
type HTTPWriter struct {
	logger      *slog.Logger
	disableGzip bool
	tmplEngine  *templating.Engine
	rng         randx.RNG
}

// NewHTTPWriter creates a new HTTPWriter with the given options.
// The rng parameter provides the seedable random number generator used for
// all probabilistic behavior (delays, fault injection). If nil, a default
// RNG seeded from the clock is created.
func NewHTTPWriter(logger *slog.Logger, disableGzip bool, rng randx.RNG) *HTTPWriter {
	if rng == nil {
		rng = randx.NewGlobal(0)
	}
	return &HTTPWriter{
		logger:      logger,
		disableGzip: disableGzip,
		tmplEngine:  templating.NewEngine(),
		rng:         rng,
	}
}

// rngFill fills the given byte slice with random data from the HTTPWriter's
// seedable RNG. This ensures deterministic output when a seed is configured.
func (w *HTTPWriter) rngFill(p []byte) {
	_, _ = w.rng.Read(p)
}

// WriteResponse writes the stub response to the client.
// It handles: delay, fault injection, gzip, CORS headers, stub headers, status, body (incl. base64).
//
// The hitCount parameter is the current hit count for the matched stub,
// used by the everyNthRequest activation mode to decide whether a fault
// should fire.
//
// The serverStart parameter is the time the server was started, used by
// the activeBetween time-window activation mode to compute elapsed time
// since server boot.
func (w *HTTPWriter) WriteResponse(rw http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions, hitCount uint64, serverStart time.Time) (FaultInjectionInfo, error) {
	resp := def.Response

	// 1. Apply delay before writing anything (simulates network latency).
	// Some delay types (e.g. "dribble") return a dribble config instead of sleeping.
	dr := w.applyDelay(resp.Delay, req.Context())

	// 2. Apply fault on the original ResponseWriter (faults bypass gzip).
	// Some fault types (e.g. "slow_close") return a slowCloseMs config
	// instead of short-circuiting, so the normal response is written first.
	fr := w.applyFault(rw, resp.Fault, req, hitCount, serverStart)
	if fr.applied {
		// Return fault injection info if a fault was applied (short-circuited)
		return FaultInjectionInfo{
			Injected:       true,
			FaultType:      fr.faultType,
			ActivationMode: fr.activationMode,
		}, nil
	}

	// Check if slow_close is pending - this is still a fault injection
	// but doesn't short-circuit, so we return info but continue writing
	slowClosePending := fr.slowCloseMs > 0

	// 3. Wrap with gzip only if no fault was applied and no slow_close is pending
	// (slow_close needs to flush the full response before hijacking, and gzip
	// buffering would interfere with the hijack timing).
	if slowClosePending {
		rw.Header().Set("Connection", "close")
		if f, ok := rw.(http.Flusher); ok {
			defer f.Flush()
		}
	} else {
		gw, gzw := w.maybeWrapGzip(rw, req)
		if gw != nil {
			defer func() {
				if err := gw.Close(); err != nil {
					w.logger.Warn("failed to close gzip writer", "error", err)
				}
			}()
			rw = gzw
		}
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

	// 6. Body — check for dribble mode first.
	if dr.dribble != nil {
		if err := w.writeDribbleBody(rw, resp, req, def.ID, dr.dribble); err != nil {
			return FaultInjectionInfo{}, err
		}
	} else if resp.Body != "" {
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
			return FaultInjectionInfo{}, fmt.Errorf("failed to write response body: %w", err)
		}
	} else if resp.Base64Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Base64Body)
		if err != nil {
			w.logger.Warn("failed to decode base64 body", "stub", def.ID, "error", err)
		} else {
			_, err = rw.Write(decoded)
			if err != nil {
				return FaultInjectionInfo{}, fmt.Errorf("failed to write base64 response body: %w", err)
			}
		}
	}

	// 7. Post-write: apply slow_close if pending.
	if slowClosePending {
		w.applySlowClose(rw, fr.slowCloseMs)
		// Return fault injection info for slow_close
		return FaultInjectionInfo{
			Injected:       true,
			FaultType:      "slow_close",
			ActivationMode: fr.activationMode,
		}, nil
	}

	// Return empty FaultInjectionInfo if no fault was applied
	return FaultInjectionInfo{}, nil
}

// writeDribbleBody writes the response body in chunks with delays between them.
// The body is split into cfg.chunks equal parts, and each part is written with
// an interval of cfg.totalDuration/cfg.chunks milliseconds between writes.
// Each chunk is flushed immediately via http.Flusher to ensure the client
// receives data incrementally.
//
// If the request context is cancelled (client disconnect), writing stops
// and the function returns nil without error — the remaining chunks are
// simply not sent.
func (w *HTTPWriter) writeDribbleBody(rw http.ResponseWriter, resp spec.ResponseDefinition, req *http.Request, stubID string, cfg *dribbleConfig) error {
	// Resolve the body content.
	var body string
	if resp.Body != "" {
		body = resp.Body
		if resp.TransformResponse {
			rendered, err := w.tmplEngine.Render(body, req)
			if err != nil {
				w.logger.Warn("template rendering failed", "stub", stubID, "error", err)
			} else {
				body = rendered
			}
		}
	} else if resp.Base64Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(resp.Base64Body)
		if err != nil {
			w.logger.Warn("failed to decode base64 body", "stub", stubID, "error", err)
			return nil
		}
		body = string(decoded)
	}

	if body == "" || cfg.chunks <= 0 {
		return nil
	}

	// Compute chunk size and interval.
	bodyLen := len(body)
	chunkSize := bodyLen / cfg.chunks
	// Remainder bytes go into the last chunk.
	interval := time.Duration(cfg.totalDuration) * time.Millisecond / time.Duration(cfg.chunks)

	flusher, hasFlusher := rw.(http.Flusher)
	ctx := req.Context()

	for i := 0; i < cfg.chunks; i++ {
		// Check if client has disconnected.
		if ctx.Err() != nil {
			return nil
		}

		start := i * chunkSize
		end := (i + 1) * chunkSize
		if i == cfg.chunks-1 {
			end = bodyLen // last chunk gets the remainder
		}

		_, err := rw.Write([]byte(body[start:end]))
		if err != nil {
			return fmt.Errorf("dribble: failed to write chunk %d: %w", i, err)
		}

		if hasFlusher {
			flusher.Flush()
		}

		// Sleep between chunks (not after the last one).
		if i < cfg.chunks-1 {
			select {
			case <-ctx.Done():
				// Client disconnected — stop sending chunks.
				return nil
			case <-time.After(interval):
				// Continue to next chunk.
			}
		}
	}

	return nil
}

// applySlowClose hijacks the connection after the full response has been
// written, sleeps for the configured delay, then closes the connection.
// This simulates a server that is slow to send FIN after completing the
// response — the client receives all data but the TCP connection lingers.
//
// When Hijack is not available (e.g. wrapped ResponseWriter), the function
// is a no-op because the Connection: close header was already set during
// the normal response writing phase, which is the best available fallback.
func (w *HTTPWriter) applySlowClose(rw http.ResponseWriter, delayMs int) {
	inner := unwrapResponseWriter(rw)
	hj, ok := inner.(http.Hijacker)
	if !ok {
		// Fallback already applied (Connection: close header set in WriteResponse).
		return
	}

	conn, bufrw, err := hj.Hijack()
	if err != nil {
		w.logger.Warn("hijack failed for slow_close, connection will close normally", "error", err)
		return
	}

	// Flush any remaining buffered data.
	if bufrw != nil {
		_ = bufrw.Flush()
	}

	// Sleep before closing — this is the "slow close" delay.
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	_ = conn.Close()
}

// applyDelay sleeps for the duration specified by the DelayDefinition.
// This simulates network latency before the response is sent.
//
// The ctx parameter enables the "timeout" delay type, which blocks until
// the context is cancelled (typically when the client disconnects or the
// server shuts down). For other delay types, ctx is unused.
//
// The "dribble" delay type does not sleep here; instead it returns a
// dribbleConfig so that the body writing phase can send chunks incrementally.
func (w *HTTPWriter) applyDelay(delay *spec.DelayDefinition, ctx context.Context) delayResult {
	if delay == nil {
		return delayResult{}
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
				d = min + w.rng.Intn(max-min+1)
			}
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
	case "timeout":
		// Block indefinitely until the context is cancelled.
		// When the client disconnects, the request context is cancelled
		// automatically by the HTTP server. When the server shuts down,
		// the server's base context is cancelled.
		select {
		case <-ctx.Done():
			// Context cancelled — client disconnected or server shutting down.
		}
	case "lognormal":
		mu, sigma, err := delayx.LognormalFromPercentiles(delay.P50, delay.P95, delay.P99)
		if err != nil {
			w.logger.Warn("invalid lognormal parameters", "error", err)
			return delayResult{}
		}
		d := delayx.Sample(mu, sigma, w.rng)
		time.Sleep(d)
	case "dribble":
		// Dribble does not sleep here. Instead, it configures the body
		// writing phase to send the response in chunks with delays.
		chunks := delay.Chunks
		if chunks <= 0 {
			w.logger.Warn("dribble delay requires chunks > 0, ignoring", "chunks", chunks)
			return delayResult{}
		}
		totalDuration := delay.TotalDuration
		if totalDuration <= 0 {
			totalDuration = delay.Value // fallback to Value if TotalDuration not set
		}
		if totalDuration <= 0 {
			w.logger.Warn("dribble delay requires totalDuration > 0, ignoring", "totalDuration", totalDuration)
			return delayResult{}
		}
		return delayResult{
			dribble: &dribbleConfig{
				chunks:        chunks,
				totalDuration: totalDuration,
			},
		}
	}
	return delayResult{}
}

// applyFault injects a network-level fault into the response.
// Returns a faultResult indicating whether a fault was applied (short-circuiting
// normal response writing) and any pending post-write actions.
//
// Fault responses are written directly to the original ResponseWriter,
// bypassing gzip compression.
//
// Special case: "slow_close" sets faultResult.slowCloseMs instead of
// short-circuiting, so the normal response is written first, then the
// connection is hijacked, held open, and closed after the delay.
//
// When the fault has an Activation configuration, ShouldActivate is called
// first to determine whether the fault should fire. If ShouldActivate returns
// ShouldFire=false, the fault is skipped and normal response writing proceeds.
//
// The hitCount parameter is the current hit count for the matched stub,
// used by the everyNthRequest activation mode.
//
// The serverStart parameter is the time the server was started, used by
// the activeBetween time-window activation mode.
func (w *HTTPWriter) applyFault(rw http.ResponseWriter, fault *spec.FaultDefinition, req *http.Request, hitCount uint64, serverStart time.Time) faultResult {
	if fault == nil {
		return faultResult{}
	}

	// Check activation criteria — if the fault should not fire, skip it.
	activateResult := ShouldActivate(fault.Activation, w.rng, hitCount, serverStart)
	if !activateResult.ShouldFire {
		return faultResult{}
	}

	// Prepare common result fields for all fault types
	result := faultResult{
		applied:       true,
		faultType:     fault.Type,
		activationMode: activateResult.Mode,
	}

	switch fault.Type {
	case "error":
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(`{"error":"internal server error","fault":"error"}`))
		return result
	case "empty":
		// Send an empty response: no headers, no explicit status (Go defaults to 200),
		// no body. Optionally flush to ensure the response is sent immediately.
		if f, ok := rw.(http.Flusher); ok {
			f.Flush()
		}
		return result
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
				return result
			}
			// Successfully hijacked — close the connection to send TCP RST.
			_ = conn.Close()
			return result
		}
		// Hijacker not available (e.g. wrapped ResponseWriter in tests or proxies).
		// Fall back to a 500 response with Connection: close.
		w.logger.Warn("hijacker not available, falling back to 500 close")
		rw.Header().Set("Connection", "close")
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(`{"error":"connection reset by peer","fault":"connection_reset"}`))
		return result
	case "malformed":
		// Send a malformed HTTP response to simulate protocol-level corruption.
		// When Hijack is available, we write an invalid HTTP response directly
		// to the TCP connection (Content-Length mismatch), then close it.
		// When Hijack is not available, we fall back to a 200 response with
		// a truncated body — only half the promised content is sent.
		inner := unwrapResponseWriter(rw)
		if hj, ok := inner.(http.Hijacker); ok {
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				w.logger.Warn("hijack failed for malformed fault, falling back to truncated response", "error", err)
				rw.Header().Set("Content-Type", "application/json")
				rw.Header().Set("Content-Length", "100")
				rw.WriteHeader(http.StatusOK)
				// Write only half of what Content-Length promised.
				_, _ = rw.Write([]byte(`{"partial":`))
				return result
			}
			// Write a malformed HTTP response directly to the connection.
			// Content-Length says 100 bytes but we only send 15, then close.
			malformed := "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n{\"truncated\":"
			_, _ = conn.Write([]byte(malformed))
			if bufrw != nil {
				_ = bufrw.Flush()
			}
			_ = conn.Close()
			return result
		}
		// Hijacker not available — fall back to truncated response.
		w.logger.Warn("hijacker not available for malformed fault, falling back to truncated response")
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Content-Length", "100")
		rw.WriteHeader(http.StatusOK)
		// Write only a partial JSON body — less than Content-Length promised.
		_, _ = rw.Write([]byte(`{"partial":`))
		return result
	case "random_data":
		// Send N bytes of random garbage data, then close the connection.
		// This simulates a scenario where a proxy or middleware injects
		// random bytes into the response stream before the connection drops.
		dataLen := fault.DataLength
		if dataLen <= 0 {
			dataLen = 256 // default
		}

		inner := unwrapResponseWriter(rw)
		if hj, ok := inner.(http.Hijacker); ok {
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				w.logger.Warn("hijack failed for random_data fault, falling back to 500 random hex", "error", err)
				rw.Header().Set("Connection", "close")
				rw.Header().Set("Content-Type", "application/octet-stream")
				rw.WriteHeader(http.StatusInternalServerError)
				garbage := make([]byte, dataLen)
				w.rngFill(garbage)
				_, _ = fmt.Fprintf(rw, "%x", garbage[:dataLen/2]) // hex-encoded, half length
				return result
			}
			// Generate random bytes and write directly to the TCP connection.
			garbage := make([]byte, dataLen)
			w.rngFill(garbage)
			_, _ = conn.Write(garbage)
			if bufrw != nil {
				_ = bufrw.Flush()
			}
			_ = conn.Close()
			return result
		}
		// Hijacker not available — fall back to 500 + random hex body.
		w.logger.Warn("hijacker not available for random_data fault, falling back to 500 random hex")
		rw.Header().Set("Connection", "close")
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.WriteHeader(http.StatusInternalServerError)
		garbage := make([]byte, dataLen)
		w.rngFill(garbage)
		_, _ = fmt.Fprintf(rw, "%x", garbage[:dataLen/2]) // hex-encoded, half length
		return result
	case "slow_close":
		// Slow close: send the complete response normally, then delay before
		// closing the connection. This simulates a server that is slow to
		// send FIN after completing the response.
		//
		// Unlike other fault types, slow_close does NOT short-circuit normal
		// response writing. Instead, it returns slowCloseMs so that
		// WriteResponse can hijack the connection after the full response
		// is written, sleep for the configured delay, then close.
		delayMs := fault.DelayMs
		if delayMs <= 0 {
			delayMs = 1000 // default 1 second
		}
		result.slowCloseMs = delayMs
		result.applied = false // slow_close doesn't short-circuit
		return result
	case "rate_limit":
		// Rate limiting is handled at the serveMock layer (before WriteResponse
		// is called), so applyFault is a no-op for this type. If execution
		// reaches here, the request was NOT rate-limited and should proceed
		// with the normal response.
		return faultResult{}
	default:
		// Unknown fault types are ignored; normal response proceeds.
		w.logger.Warn("unknown fault type, skipping fault injection", "faultType", fault.Type)
		return faultResult{}
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

// slowCloseHijacker wraps an http.ResponseWriter and implements http.Hijacker.
// It returns a real net.Conn (from net.Pipe) so that Close() works in tests.
// Unlike mockHijacker, this variant does NOT close the client side immediately,
// allowing test code to read data from the client end of the pipe.
type slowCloseHijacker struct {
	http.ResponseWriter
	hijacked bool
	conn     net.Conn
	client   net.Conn
}

func (m *slowCloseHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijacked = true
	server, client := net.Pipe()
	m.conn = server
	m.client = client
	return server, nil, nil
}

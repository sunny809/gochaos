// Package response provides the response writing port and adapters for the gmock server.
//
// The package implements the hexagonal architecture pattern, separating the
// concern of writing HTTP responses from the server lifecycle management.
// It handles response delays (fixed, random, lognormal, timeout, dribble),
// fault injection (error, empty, connection_reset, malformed, random_data,
// slow_close, rate_limit), gzip compression, binary bodies, and CORS headers.
//
// Sub-concerns live in separate files:
//   - http_writer.go — the HTTP transport adapter (Writer interface implementation)
//   - writer.go      — the Writer interface definition and CORSOptions
//   - activation.go  — fault activation logic (probability, Nth-request, time-window)
//   - fault.go       — fault type validation
package response

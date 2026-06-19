// Package response provides the response writing port and adapters for the gmock server.
//
// This package implements the hexagonal architecture pattern, separating the
// concern of writing HTTP responses from the server lifecycle management.
package response

import (
	"net/http"
	"time"

	"github.com/sunny809/gochaos/internal/spec"
)

// Writer is the port for writing HTTP responses.
type Writer interface {
	// WriteResponse writes the stub response to the client.
	// It handles: delay, fault injection, gzip, CORS headers, stub headers,
	// status, body (incl. base64).
	//
	// The hitCount parameter is the current hit count for the matched stub,
	// used by the everyNthRequest activation mode to decide whether a fault
	// should fire. The caller (server.serveMock) should increment the stub's
	// hit count via registry.IncrementHitCount before calling WriteResponse
	// and pass the returned value here.
	//
	// The serverStart parameter is the time the server was started, used by
	// the activeBetween time-window activation mode to compute elapsed time
	// since server boot.
	WriteResponse(w http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions, hitCount uint64, serverStart time.Time) error

	// WriteCORSHeaders writes CORS headers for a preflight OPTIONS response.
	WriteCORSHeaders(w http.ResponseWriter, r *http.Request, opts *CORSOptions)
}

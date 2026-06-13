// Package response provides the response writing port and adapters for the gmock server.
//
// This package implements the hexagonal architecture pattern, separating the
// concern of writing HTTP responses from the server lifecycle management.
package response

import (
	"net/http"

	"github.com/sunny809/gochaos/internal/spec"
)

// Writer is the port for writing HTTP responses.
type Writer interface {
	// WriteResponse writes the stub response to the client.
	// It handles: delay, CORS headers, stub headers, status, body (incl. base64),
	// and optional template transformation.
	WriteResponse(w http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions) error

	// WriteCORSHeaders writes CORS headers for a preflight OPTIONS response.
	WriteCORSHeaders(w http.ResponseWriter, r *http.Request, corsOpts *CORSOptions)
}

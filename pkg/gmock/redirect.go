package gmock

import "net/http"

// WithRedirect creates a ResponseDefinition for an HTTP redirect.
//
// Supported status codes: 301 (Moved Permanently), 302 (Found), 307 (Temporary Redirect),
// 308 (Permanent Redirect). Defaults to 302 if status is 0.
//
// Example:
//
//	server.Stub(gmock.StubDefinition{
//	    Request:  gmock.RequestPattern{Method: "GET", URLPath: "/old-path"},
//	    Response: gmock.WithRedirect(http.StatusMovedPermanently, "/new-path"),
//	})
func WithRedirect(status int, location string) ResponseDefinition {
	if status == 0 {
		status = http.StatusFound
	}
	return ResponseDefinition{
		Status: status,
		Headers: map[string]string{
			"Location": location,
		},
	}
}

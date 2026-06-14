package matcher

import (
	"fmt"
	"mime"
	"net/http"
	"strings"
)

// AcceptMatcher matches the Accept request header against a desired media type.
//
// It handles proper Accept header parsing including quality values (q=),
// wildcards (*/*, type/*), and media ranges. This is more sophisticated than
// a simple header exact match because Accept headers contain comma-separated
// values with optional quality parameters.
//
// Example Accept headers that would match mediaType "application/json":
//
//	Accept: application/json
//	Accept: application/json; q=0.9
//	Accept: text/html, application/json
//	Accept: */*
//	Accept: application/*
type AcceptMatcher struct {
	mediaType string
	params    map[string]string
}

// NewAcceptMatcher creates an AcceptMatcher for the given media type.
// The media type should be a valid MIME type like "application/json".
func NewAcceptMatcher(mediaType string) *AcceptMatcher {
	_, params, _ := mime.ParseMediaType(mediaType)
	if params == nil {
		params = make(map[string]string)
	}
	// Remove q parameter from the desired media type (it's for the client, not for matching)
	delete(params, "q")
	return &AcceptMatcher{
		mediaType: mediaType,
		params:    params,
	}
}

// Match returns true if the request's Accept header is compatible with this matcher's media type.
func (m *AcceptMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 7 for an Accept match.
func (m *AcceptMatcher) ScoreMatch(req *http.Request) (bool, int) {
	accept := req.Header.Get("Accept")
	if accept == "" {
		// No Accept header means the client accepts anything (compatible)
		return true, 7
	}

	// Parse the Accept header into individual media ranges
	mediaRanges := strings.Split(accept, ",")
	for _, mr := range mediaRanges {
		mr = strings.TrimSpace(mr)
		if mr == "" {
			continue
		}
		if m.matchesRange(mr) {
			return true, 7
		}
	}

	return false, 0
}

// matchesRange checks if a single media range (e.g., "application/json;q=0.9")
// is compatible with the desired media type.
func (m *AcceptMatcher) matchesRange(mediaRange string) bool {
	mediaType, params, err := mime.ParseMediaType(mediaRange)
	if err != nil {
		return false
	}

	// Check quality factor — q=0 means "not acceptable"
	if q, ok := params["q"]; ok {
		if q == "0" || q == "0.0" {
			return false
		}
	}

	return typesCompatible(m.mediaType, mediaType)
}

// typesCompatible checks if the desired type matches the offered type.
// Handles wildcards: */* matches anything, type/* matches any subtype.
func typesCompatible(desired, offered string) bool {
	if desired == offered {
		return true
	}

	// Split into type and subtype
	desiredParts := strings.SplitN(desired, "/", 2)
	offeredParts := strings.SplitN(offered, "/", 2)

	if len(desiredParts) != 2 || len(offeredParts) != 2 {
		return false
	}

	dType, dSub := desiredParts[0], desiredParts[1]
	oType, oSub := offeredParts[0], offeredParts[1]

	// */* matches anything
	if oType == "*" && oSub == "*" {
		return true
	}

	// type/* matches any subtype of the same type
	if oType == dType && oSub == "*" {
		return true
	}

	// application/* matches application/json, etc.
	if dType == oType && dSub == "*" {
		return true
	}

	return false
}

// String returns a description of this matcher.
func (m *AcceptMatcher) String() string {
	return fmt.Sprintf("accept=%s", m.mediaType)
}

// MediaType returns the target media type.
func (m *AcceptMatcher) MediaType() string {
	return m.mediaType
}

// Diagnose returns a structured diagnosis for near-miss reporting.
func (m *AcceptMatcher) Diagnose(req *http.Request) Diagnosis {
	actual := req.Header.Get("Accept")
	d := Diagnosis{
		Dimension: "accept",
		MaxScore:  7,
		Expected:  m.mediaType,
		Actual:    actual,
	}
	matched, score := m.ScoreMatch(req)
	if matched {
		d.Matched = true
		d.Score = score
		return d
	}
	d.Reason = fmt.Sprintf("Accept %q is not compatible with %s", actual, m.mediaType)
	return d
}

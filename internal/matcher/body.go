package matcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

// BodyExactMatcher matches the request body exactly (byte-for-byte).
type BodyExactMatcher struct {
	body string
}

// NewBodyExactMatcher creates a BodyExactMatcher.
func NewBodyExactMatcher(body string) *BodyExactMatcher {
	return &BodyExactMatcher{body: body}
}

// Match returns true if the request body matches exactly.
func (m *BodyExactMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 20 for an exact body match.
func (m *BodyExactMatcher) ScoreMatch(req *http.Request) (bool, int) {
	body, err := readBody(req)
	if err != nil {
		return false, 0
	}
	if body == m.body {
		return true, 20
	}
	return false, 0
}

// String returns a description of this matcher.
func (m *BodyExactMatcher) String() string {
	return "body exact match"
}

// Diagnose returns a structured diagnosis for near-miss reporting.
//
// Note on Actual: body content is intentionally truncated to truncateBodyLen
// characters to keep diagnostics small and avoid pulling large request
// payloads into result structs. readBody restores req.Body before returning,
// so subsequent handlers/matchers can re-read it.
func (m *BodyExactMatcher) Diagnose(req *http.Request) Diagnosis {
	body, err := readBody(req)
	d := Diagnosis{
		Dimension: "body",
		MaxScore:  20,
		Expected:  truncateBody(m.body),
	}
	if err != nil {
		d.Reason = fmt.Sprintf("could not read body: %v", err)
		return d
	}
	d.Actual = truncateBody(body)
	if body == m.body {
		d.Matched = true
		d.Score = 20
		return d
	}
	d.Reason = "body does not equal expected exact value"
	return d
}

// ---

// BodyRegexMatcher matches the request body against a regular expression.
type BodyRegexMatcher struct {
	pattern *regexp.Regexp
	raw     string
}

// NewBodyRegexMatcher creates a BodyRegexMatcher.
func NewBodyRegexMatcher(pattern string) (*BodyRegexMatcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid body regex: %w", err)
	}
	return &BodyRegexMatcher{pattern: re, raw: pattern}, nil
}

// Match returns true if the request body matches the regex.
func (m *BodyRegexMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 10 for a regex body match.
func (m *BodyRegexMatcher) ScoreMatch(req *http.Request) (bool, int) {
	body, err := readBody(req)
	if err != nil {
		return false, 0
	}
	if m.pattern.MatchString(body) {
		return true, 10
	}
	return false, 0
}

// String returns a description of this matcher.
func (m *BodyRegexMatcher) String() string {
	return fmt.Sprintf("body regex=%s", m.raw)
}

// Diagnose returns a structured diagnosis for near-miss reporting.
// See BodyExactMatcher.Diagnose for the body-truncation rationale.
func (m *BodyRegexMatcher) Diagnose(req *http.Request) Diagnosis {
	body, err := readBody(req)
	d := Diagnosis{
		Dimension: "body",
		MaxScore:  10,
		Expected:  m.raw,
	}
	if err != nil {
		d.Reason = fmt.Sprintf("could not read body: %v", err)
		return d
	}
	d.Actual = truncateBody(body)
	if m.pattern.MatchString(body) {
		d.Matched = true
		d.Score = 10
		return d
	}
	d.Reason = fmt.Sprintf("body does not match regex %s", m.raw)
	return d
}

// ---

// BodyJSONPathMatcher matches the request body by evaluating a JSONPath expression.
// The match succeeds if the expression evaluates to a non-nil, non-empty value.
type BodyJSONPathMatcher struct {
	path string
}

// NewBodyJSONPathMatcher creates a BodyJSONPathMatcher.
func NewBodyJSONPathMatcher(path string) *BodyJSONPathMatcher {
	return &BodyJSONPathMatcher{path: path}
}

// Match returns true if the JSONPath expression finds a value.
func (m *BodyJSONPathMatcher) Match(req *http.Request) bool {
	matched, _ := m.ScoreMatch(req)
	return matched
}

// ScoreMatch returns the match result with score 12 for a successful JSONPath match.
func (m *BodyJSONPathMatcher) ScoreMatch(req *http.Request) (bool, int) {
	body, err := readBody(req)
	if err != nil || body == "" {
		return false, 0
	}

	// Parse JSON body and evaluate JSONPath
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return false, 0
	}

	result, err := jsonpath.Get(m.path, data)
	if err != nil {
		return false, 0
	}

	// Check that result is non-nil and non-empty
	switch v := result.(type) {
	case nil:
		return false, 0
	case string:
		if v == "" {
			return false, 0
		}
	case []interface{}:
		if len(v) == 0 {
			return false, 0
		}
	case map[string]interface{}:
		if len(v) == 0 {
			return false, 0
		}
	}

	return true, 12
}

// String returns a description of this matcher.
func (m *BodyJSONPathMatcher) String() string {
	return fmt.Sprintf("body jsonpath=%s", m.path)
}

// Diagnose returns a structured diagnosis for near-miss reporting.
// See BodyExactMatcher.Diagnose for the body-truncation rationale.
func (m *BodyJSONPathMatcher) Diagnose(req *http.Request) Diagnosis {
	body, err := readBody(req)
	d := Diagnosis{
		Dimension: "body",
		MaxScore:  12,
		Expected:  m.path,
	}
	if err != nil {
		d.Reason = fmt.Sprintf("could not read body: %v", err)
		return d
	}
	d.Actual = truncateBody(body)

	if body == "" {
		d.Reason = "body is empty"
		return d
	}

	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		d.Reason = fmt.Sprintf("body is not valid JSON: %v", err)
		return d
	}

	result, err := jsonpath.Get(m.path, data)
	if err != nil {
		d.Reason = fmt.Sprintf("jsonpath %s did not resolve: %v", m.path, err)
		return d
	}

	switch v := result.(type) {
	case nil:
		d.Reason = fmt.Sprintf("jsonpath %s resolved to nil", m.path)
		return d
	case string:
		if v == "" {
			d.Reason = fmt.Sprintf("jsonpath %s resolved to empty string", m.path)
			return d
		}
	case []interface{}:
		if len(v) == 0 {
			d.Reason = fmt.Sprintf("jsonpath %s resolved to empty array", m.path)
			return d
		}
	case map[string]interface{}:
		if len(v) == 0 {
			d.Reason = fmt.Sprintf("jsonpath %s resolved to empty object", m.path)
			return d
		}
	}

	d.Matched = true
	d.Score = 12
	return d
}

// readBody reads the full request body and restores it for subsequent handlers.
func readBody(req *http.Request) (string, error) {
	if req.Body == nil {
		return "", nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return "", err
	}
	req.Body.Close()
	// Restore the body so it can be read again
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	return string(body), nil
}

// truncateBodyLen is the maximum number of body characters retained in a
// Diagnosis.Expected/Actual field. The cap keeps near-miss results bounded
// regardless of upstream payload size — body diffs are diagnostic hints, not
// audit trails.
const truncateBodyLen = 80

// truncateBody returns s as-is if it fits in truncateBodyLen, or the first
// truncateBodyLen characters followed by "…" otherwise. Operates on runes so
// it does not slice mid-UTF-8.
func truncateBody(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= truncateBodyLen {
		return s
	}
	return string(runes[:truncateBodyLen]) + "…"
}
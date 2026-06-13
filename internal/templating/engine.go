// Package templating provides response template rendering with custom helper functions.
//
// The Engine uses Go's text/template (not html/template) to avoid HTML escaping
// issues when templating JSON responses. Built-in functions include random value
// generation and request field extraction.
package templating

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"text/template"
	"time"
)

// Engine renders stub response templates with request context.
type Engine struct {
	funcs template.FuncMap
	mu    sync.RWMutex
	cache map[string]*template.Template
}

// requestData exposes request fields to templates via method calls.
type requestData struct {
	req *http.Request
}

// Method returns the HTTP method of the request.
func (r *requestData) Method() string {
	return r.req.Method
}

// Path returns the request URL path.
func (r *requestData) Path() string {
	return r.req.URL.Path
}

// Header returns the first value of the named header, or an empty string.
func (r *requestData) Header(name string) string {
	return r.req.Header.Get(name)
}

// Query returns the first value of the named query parameter, or an empty string.
func (r *requestData) Query(key string) string {
	return r.req.URL.Query().Get(key)
}

// templateData is the top-level data passed to every template execution.
type templateData struct {
	Request *requestData
}

// NewEngine creates a templating engine with built-in helper functions.
func NewEngine() *Engine {
	return &Engine{
		funcs: template.FuncMap{
			"randomUUID": func() string {
				return generateUUID()
			},
			"now": func() string {
				return time.Now().UTC().Format(time.RFC3339)
			},
			"randomInt": func(min, max int) int {
				if min > max {
					min, max = max, min
				}
				n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
				return min + int(n.Int64())
			},
		},
		cache: make(map[string]*template.Template),
	}
}

// Render evaluates the template with the given request context.
// Returns the rendered body or an error.
func (e *Engine) Render(body string, req *http.Request) (string, error) {
	e.mu.RLock()
	tmpl, ok := e.cache[body]
	e.mu.RUnlock()
	if ok {
		return e.execute(tmpl, req)
	}

	e.mu.Lock()
	// Double-check in case another goroutine compiled while we were waiting
	tmpl, ok = e.cache[body]
	if !ok {
		var err error
		tmpl, err = template.New("response").Funcs(e.funcs).Parse(body)
		if err != nil {
			e.mu.Unlock()
			return "", fmt.Errorf("failed to parse template: %w", err)
		}
		e.cache[body] = tmpl
	}
	e.mu.Unlock()

	return e.execute(tmpl, req)
}

func (e *Engine) execute(tmpl *template.Template, req *http.Request) (string, error) {
	data := templateData{
		Request: &requestData{req: req},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// generateUUID generates a random UUID v4 using crypto/rand.
func generateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// Set version (4) and variant (RFC 4122)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

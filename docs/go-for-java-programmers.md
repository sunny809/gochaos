# Go for Java Programmers: Learning Go Through the `gochaos` Codebase

**Author:** Claude, reviewing the [gochaos](https://github.com/agrim123/gochaos) project  
**Audience:** Experienced Java developers learning Go  
**Goal:** Bridge the mental model gap by comparing concrete code examples from a real Go project against familiar Java patterns

---

## Table of Contents

1. [Interfaces vs Java Interfaces](#1-interfaces-vs-java-interfaces)
2. [Structs vs Java Classes](#2-structs-vs-java-classes)
3. [Error Handling](#3-error-handling)
4. [Concurrency](#4-concurrency)
5. [Functional Options Pattern](#5-functional-options-pattern)
6. [Packages and Visibility](#6-packages-and-visibility)
7. [Testing](#7-testing)
8. [http.Handler / http.Hijacker](#8-http-handler--http-hijacker)
9. [Embedding](#9-embedding)
10. [No Generics (Currently)](#10-no-generics-currently)
11. [defer](#11-defer)
12. [io.Reader / io.Writer](#12-ioreader--iowriter)

---

## 1. Interfaces vs Java Interfaces

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/response/writer.go`, lines 1-9

```go
package response

import (
    "io"
    "net/http"
)

// Writer writes responses to an http.ResponseWriter.
type Writer interface {
    Write(w http.ResponseWriter, r *http.Request, resp Response) error
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 1-130

```go
package response

import (
    "fmt"
    "io"
    "net/http"
)

// NewHTTPWriter ...
type HTTPWriter struct {
    Middleware []HTTPMiddleware
    // ...
}

func (w *HTTPWriter) Write(writer http.ResponseWriter, request *http.Request, resp Response) error {
    // implements the Writer interface
}
```

### The Java Approach

In Java, you would write:

```java
public interface Writer {
    void write(HttpServletResponse writer, HttpServletRequest request, Response response) throws IOException;
}

public class HttpWriter implements Writer {
    @Override
    public void write(HttpServletResponse writer, HttpServletRequest request, Response response) {
        // ...
    }
}
```

### Why Go Is Different

In Java, a class **explicitly declares** `implements Writer`. If you remove `implements Writer`, the compiler immediately rejects the code. In Go, **satisfaction is implicit** -- `*HTTPWriter` satisfies `Writer` automatically because it has a method with the exact same signature (`Write(w http.ResponseWriter, r *http.Request, resp Response) error`). There is no `implements` keyword in Go.

**Practical consequence:** You can define an interface that matches an existing struct's methods without changing that struct. This decouples producers from consumers of interfaces. In Java, the producer must know about the interface upfront (or you use adapters); in Go, the consumer can define the contract retroactively.

**Another interface in this codebase:**

**File:** `/Users/sunny/go_project/gochaos/internal/matcher/matcher.go`, lines 1-18

```go
type Matcher interface {
    Match(*http.Request) bool
    // ...
}
```

Any type that has a `Match(*http.Request) bool` method automatically satisfies `Matcher`. No explicit declaration needed.

---

## 2. Structs vs Java Classes

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 11-17

```go
type HTTPWriter struct {
    Middleware []HTTPMiddleware
    Fault      *Fault
    Delay      *DelayConfig
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/response/fault.go`, lines 23-38

```go
type Fault struct {
    Percentage    float64
    HTTPStatus    int
    Close         bool
    Reset         bool
    ResetDuration time.Duration
    Error         string
}
```

### The Java Approach

```java
public class HttpWriter {
    private List<HttpMiddleware> middleware;
    private Fault fault;
    private DelayConfig delay;

    public HttpWriter(List<HttpMiddleware> middleware, Fault fault, DelayConfig delay) {
        this.middleware = middleware;
        this.fault = fault;
        this.delay = delay;
    }
    // getters, setters...
}
```

### Why Go Is Different

- **No `class` keyword.** Go uses `type X struct { ... }`. A struct is just a named collection of fields.
- **No constructors.** You create instances with `HTTPWriter{Middleware: ..., Fault: ...}` (literal syntax) or with a constructor function like `func NewHTTPWriter() *HTTPWriter`.
- **No inheritance.** There is no `extends` in Go.
- **No getters/setters convention.** Exported fields are accessed directly: `w.Middleware`. If you need encapsulation, you make fields unexported (lowercase) and provide methods.
- **Zero values.** Every field has a default zero value (`nil` for slices/pointers, `0` for numbers, `""` for strings). No null-pointer risk from uninitialized fields.

---

## 3. Error Handling

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/response/fault.go`, lines 40-48

```go
func (f *Fault) Validate() error {
    if f.Percentage < 0 || f.Percentage > 100 {
        return fmt.Errorf("percentage must be between 0 and 100")
    }
    if f.HTTPStatus < 100 || f.HTTPStatus > 599 {
        return fmt.Errorf("invalid HTTP status code")
    }
    if f.Close && f.Reset {
        return fmt.Errorf("close and reset cannot both be true")
    }
    return nil
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/admin/mappings.go`, lines 40-55 (error wrapping)

```go
var ErrMappingNotFound = errors.New("mapping not found")

func (h *Handler) DeleteMapping(id string) error {
    for i, m := range h.mappings {
        if m.ID == id {
            h.mappings = append(h.mappings[:i], h.mappings[i+1:]...)
            return nil
        }
    }
    return fmt.Errorf("%%w: %%s", ErrMappingNotFound, id)
}
```

And from the caller side (hypothetical, based on pattern):

```go
err := h.DeleteMapping(id)
if errors.Is(err, ErrMappingNotFound) {
    // handle not-found case
}
```

### The Java Approach

```java
public void validate() {
    if (percentage < 0 || percentage > 100) {
        throw new IllegalArgumentException("percentage must be between 0 and 100");
    }
}
```

### Why Go Is Different

- **Errors are values, not control flow.** `Validate()` returns an `error` instead of throwing an exception. The caller **must** check it.
- **Multiple return values.** The canonical pattern is `result, err := someFunc()`. Java has no equivalent -- you either throw or return null.
- **No exceptions.** Go has no `try`/`catch`/`finally`. The `error` interface is just `type error interface { Error() string }`.
- **`errors.Is` and `errors.As`.** `errors.Is(err, ErrMappingNotFound)` checks if `err` wraps a sentinel error (like `%w` in `fmt.Errorf`). `errors.As` unwraps and type-asserts. Java's `instanceof` checks serve a similar role, but Go's wrapping is explicit and intentional.
- **`fmt.Errorf("...%%w...", err)`** wraps errors with context while preserving the original error for `errors.Is`/`errors.As` checks. Java 21+ `Exception::initCause` is the closest, but Go's wrapping is a first-class language feature.

---

## 4. Concurrency

### 4a. sync.RWMutex

**File:** `/Users/sunny/go_project/gochaos/internal/stub/registry.go`, lines 5-30

```go
type Registry struct {
    mu   sync.RWMutex
    stubs map[string]*Stub
}

func (r *Registry) Get(id string) (*Stub, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    s, ok := r.stubs[id]
    return s, ok
}

func (r *Registry) Set(id string, stub *Stub) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.stubs[id] = stub
}
```

### The Java Approach

```java
public class Registry {
    private final ReadWriteLock lock = new ReentrantReadWriteLock();
    private final Map<String, Stub> stubs = new ConcurrentHashMap<>();

    public Stub get(String id) {
        return stubs.get(id);  // ConcurrentHashMap is safe for reads
    }

    public void set(String id, Stub stub) {
        stubs.put(id, stub);
    }
}
```

### Why Go Is Different

- **`sync.RWMutex`** gives explicit reader/writer locking. Multiple goroutines can hold `RLock` simultaneously, but `Lock` excludes everything.
- **`defer`** pairs naturally with locks: `r.mu.Lock()` at the top of the function, `defer r.mu.Unlock()` immediately after. You cannot forget to unlock.
- **No `synchronized` keyword.** Go uses explicit mutexes.
- **`sync.Map`** exists but is niche. Regular maps with a mutex are the idiomatic Go approach.
- **Java alternative:** `ConcurrentHashMap` handles this internally, but Go makes the locking strategy explicit and visible at the call site.

### 4b. Goroutines and Channels

**File:** `/Users/sunny/go_project/gochaos/internal/templating/engine.go`, lines 10-45

```go
type Engine struct {
    templates *template.Template
    mu        sync.RWMutex
    funcMap   template.FuncMap
}

func (e *Engine) Render(w io.Writer, name string, data interface{}) error {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.templates.ExecuteTemplate(w, name, data)
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/log/log.go`, lines 8-50

```go
type RingBuffer struct {
    mu    sync.Mutex
    buf   []byte
    pos   int
    full  bool
    cond  *sync.Cond
}

func (rb *RingBuffer) Write(p []byte) (n int, err error) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    // ring buffer logic...
    rb.cond.Broadcast()
    return len(p), nil
}

func (rb *RingBuffer) Read(p []byte) (n int, err error) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    for rb.isEmpty() {
        rb.cond.Wait()
    }
    // read logic...
}
```

### Why Go Is Different

- **Goroutines** are lightweight (stack starts at ~4KB) vs Java threads (~1MB). You can have thousands of goroutines in a single process.
- **`sync.Cond`** provides condition variable semantics for goroutine signaling (similar to Java's `wait()`/`notifyAll()` but with explicit mutex binding).
- **`sync.RWMutex`** is Go's idiomatic solution for read-heavy concurrent access patterns.
- **Java equivalent:** `synchronized`, `Lock`, `Condition`, `ReadWriteLock` -- same concepts, different syntax. Go's `defer` makes lock release much harder to forget.

---

## 5. Functional Options Pattern

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/pkg/gmock/options.go`, lines 1-35

```go
type ServerOption func(*Server)

func WithPort(port int) ServerOption {
    return func(s *Server) {
        s.port = port
    }
}

func WithHost(host string) ServerOption {
    return func(s *Server) {
        s.host = host
    }
}

func WithMiddleware(mw ...HTTPMiddleware) ServerOption {
    return func(s *Server) {
        s.middleware = append(s.middleware, mw...)
    }
}
```

**File:** `/Users/sunny/go_project/gochaos/pkg/gmock/server.go`, lines 20-35

```go
type Server struct {
    port       int
    host       string
    middleware []HTTPMiddleware
}

func NewServer(opts ...ServerOption) *Server {
    s := &Server{
        port: 8080, // default
        host: "localhost",
    }
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

### The Java Approach

```java
public class Server {
    private int port = 8080;
    private String host = "localhost";
    private List<HttpMiddleware> middleware = new ArrayList<>();

    private Server() {}

    public static class Builder {
        private final Server server = new Server();

        public Builder withPort(int port) { server.port = port; return this; }
        public Builder withHost(String host) { server.host = host; return this; }
        public Server build() { return server; }
    }

    public static Builder builder() { return new Builder(); }
}
```

### Why Go Is Different

- **No method chaining.** Each `ServerOption` is a function `func(*Server)`. The builder loop applies them: `for _, opt := range opts { opt(s) }`.
- **Variadic `...ServerOption`** lets callers pass zero or more options: `NewServer(WithPort(9090), WithHost("0.0.0.0"))`.
- **Defaults are natural.** The constructor sets sensible defaults, options override them. No separate "default" object needed.
- **Extensible without changing the struct.** Anyone can create a new `ServerOption` function in their own package. No `Builder` subclassing needed.
- **Java equivalent:** The Builder pattern (Joshua Bloch's Effective Java Item 2) achieves the same goal with more boilerplate. Go's functional options are lighter -- no `build()` call, no mutable builder object.

---

## 6. Packages and Visibility

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/stub/registry.go`, lines 1-30

```go
package stub

type Registry struct {        // exported (capital R)
    mu   sync.RWMutex         // unexported (lowercase mu)
    stubs map[string]*Stub    // unexported
}

func (r *Registry) Get(id string) (*Stub, bool) {  // exported
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.stubs[id]
}
```

**File:** `/Users/sunny/go_project/gochaos/pkg/gmock/server.go`, lines 1-10

```go
package gmock

// Server is exported
type Server struct {
    port int    // unexported field
    host string // unexported field
}
```

### The Java Approach

```java
package com.example.stub;

public class Registry {
    private final ReentrantReadWriteLock mu = new ReentrantReadWriteLock();
    private final Map<String, Stub> stubs = new HashMap<>();

    public Stub get(String id) { ... }
}
```

### Why Go Is Different

- **`package` is the unit of encapsulation**, not `class`. Everything in the same package can access unexported identifiers.
- **Exported = capital first letter.** `Registry`, `Get`, `Write` are exported (public). `mu`, `stubs`, `get` would be unexported (package-private).
- **No `private`/`protected`/`public` keywords.** One rule: capital letter = visible outside the package; lowercase = package-internal.
- **`internal` package convention.** The `internal/` directory in the project root is a Go convention: packages under `internal/` cannot be imported by external modules. This is a language-enforced "friends" mechanism. Java has no direct equivalent -- you'd use module system (`module-info.java`) or runtime checks.

---

## 7. Testing

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer_test.go`, lines 1-80

```go
func TestHTTPWriter_Write(t *testing.T) {
    tests := []struct {
        name    string
        middleware []HTTPMiddleware
        fault      *Fault
        delay      *DelayConfig
        wantStatus int
        wantErr    bool
    }{
        {
            name:    "no middleware, no fault",
            wantStatus: http.StatusOK,
            wantErr:    false,
        },
        {
            name:    "with fault configured",
            fault:      &Fault{Percentage: 100, HTTPStatus: http.StatusInternalServerError},
            wantStatus: http.StatusInternalServerError,
            wantErr:    false,
        },
        // more cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            w := NewHTTPWriter(WithMiddleware(tt.middleware...), WithFault(tt.fault))
            rec := httptest.NewRecorder()
            req := httptest.NewRequest(http.MethodGet, "/", nil)
            err := w.Write(rec, req, Response{})
            // assertions...
        })
    }
}
```

### The Java Approach

```java
@Test
public void testWrite() {
    // Arrange
    HttpWriter writer = new HttpWriter();
    MockHttpServletResponse response = new MockHttpServletResponse();
    MockHttpServletRequest request = new MockHttpServletRequest();

    // Act
    writer.write(response, request, new Response());

    // Assert
    assertEquals(200, response.getStatus());
}

@Test
public void testWriteWithFault() {
    HttpWriter writer = new HttpWriter(Fault.withPercentage(100));
    MockHttpServletResponse response = new MockHttpServletResponse();
    MockHttpServletRequest request = new MockHttpServletRequest();
    writer.write(response, request, new Response());
    assertEquals(500, response.getStatus());
}
```

### Why Go Is Different

- **Table-driven tests.** A slice of anonymous structs defines all test cases. A single `for` loop runs them all.
- **`t.Run(tt.name, ...)`** creates subtests with descriptive names. Run a specific subtest: `go test -run TestHTTPWriter_Write/no_middleware`. Java JUnit 5 `@DisplayName` achieves similar readability, but the execution model differs -- Go subtests run in the same function scope with shared setup.
- **No assertion framework in stdlib.** `if got != want { t.Errorf(...) }` is the standard pattern. Java has AssertJ, Hamcrest, JUnit assertions built in.
- **`httptest.NewRecorder()` and `httptest.NewRequest()`** are stdlib testing helpers. No Mockito required for simple HTTP testing.
- **Anonymous structs** in test files (`[]struct{ name string; ... }`) are a Go idiomatic pattern that has no direct Java equivalent (Java would use a `@ParameterizedTest` + `@CsvSource` or a separate test class per case).

---

## 8. http.Handler / http.Hijacker

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/admin/handler.go`, lines 1-50

```go
type Handler struct {
    router *http.ServeMux
}

func NewHandler() *Handler {
    h := &Handler{router: http.NewServeMux()}
    h.registerRoutes()
    return h
}

func (h *Handler) registerRoutes() {
    h.router.HandleFunc("POST /api/mappings", h.CreateMapping)
    h.router.HandleFunc("GET /api/mappings/{id}", h.GetMapping)
    h.router.HandleFunc("DELETE /api/mappings/{id}", h.DeleteMapping)
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 25-50

```go
type HTTPWriter struct {
    Middleware []HTTPMiddleware
}

type HTTPMiddleware func(w http.ResponseWriter, r *http.Request, next func(w http.ResponseWriter, r *http.Request))

func (w *HTTPWriter) Use(mw HTTPMiddleware) {
    w.Middleware = append(w.Middleware, mw)
}
```

### The Java Approach

```java
@RestController
@RequestMapping("/api/mappings")
public class MappingController {
    @PostMapping
    public ResponseEntity<?> createMapping(@RequestBody Mapping mapping) { ... }

    @GetMapping("/{id}")
    public ResponseEntity<?> getMapping(@PathVariable String id) { ... }

    @DeleteMapping("/{id}")
    public ResponseEntity<?> deleteMapping(@PathVariable String id) { ... }
}
```

### Why Go Is Different

- **`http.Handler` interface** is `func(http.ResponseWriter, *http.Request)` -- a single method. Any function with that signature is an HTTP handler.
- **`http.HandlerFunc`** is a function type that adapts a plain function to the `Handler` interface. `h.router.HandleFunc("GET /path", fn)` does this automatically.
- **No reflection-based routing.** Go 1.22+ `http.ServeMux` supports method+pattern routing (`"POST /api/mappings"`) without annotations or reflection.
- **Explicit middleware pipeline.** Middleware is a function `func(w http.ResponseWriter, r *http.Request, next func(...))`. The `next` closure is passed explicitly -- no `FilterChain.doFilter` interface ceremony.
- **No annotations.** Java relies on `@RestController`, `@RequestMapping`, `@PostMapping` annotations processed via reflection at startup. Go has no annotations -- everything is explicit function calls.
- **`http.Hijacker`** is an optional interface that `http.ResponseWriter` can implement (e.g., for WebSocket upgrades). It's a classic example of Go's optional interface pattern: check with a type assertion `if hj, ok := w.(http.Hijacker); ok { ... }`. Java would require the servlet container to provide a special wrapper class.

---

## 9. Embedding

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 30-45

```go
type HTTPWriter struct {
    Fault      *Fault
    Delay      *DelayConfig
    Middleware []HTTPMiddleware
}

// HTTPWriter can use Fault's methods as if they were its own
// (if Fault has methods, they are promoted to HTTPWriter)
```

**File:** `/Users/sunny/go_project/gochaos/internal/stub/registry.go`, lines 10-20

```go
type Stub struct {
    Matcher  *matcher.Matcher
    Response *response.Response
    // embedding via composition, not embedding syntax
}
```

### The Java Approach

```java
// No equivalent. Java uses inheritance:
public class HttpWriter extends Fault { ... }
// Or composition (more idiomatic in modern Java):
public class HttpWriter {
    private final Fault fault = new Fault();
    // delegate methods manually...
}
```

### Why Go Is Different

- **Embedding is not inheritance.** When you write `type HTTPWriter struct { *Fault }` (without a field name), `Fault`'s methods are *promoted* to `HTTPWriter`. But `HTTPWriter` is not a `Fault` -- you cannot assign an `*HTTPWriter` to a `*Fault` variable.
- **Promotion vs delegation.** Embedded methods are available directly: `w.Validate()` calls the embedded `Fault.Validate()`. No delegation boilerplate.
- **No method overriding in the Java sense.** If `HTTPWriter` also defines `Validate()`, it shadows (not overrides) `Fault.Validate()`. Which method is called depends on the static type, not dynamic dispatch.
- **Java composition** requires manual delegation for every method. Go embedding automates this.
- **In this codebase**, `HTTPWriter` uses named fields (`Fault *Fault`) rather than anonymous embedding, which is still composition -- just explicit rather than promoted.

---

## 10. No Generics (Currently)

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/spec/spec.go`, lines 1-30

```go
type Spec struct {
    Matchers []MatcherConfig
    Response ResponseSpec
    // no generic types used
}

type MatcherConfig struct {
    Type   string
    Config json.RawMessage
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/stub/registry.go`, lines 5-30

```go
type Registry struct {
    mu    sync.RWMutex
    stubs map[string]*Stub  // map with string key, *Stub value -- no generics
}
```

### The Java Approach

```java
public class Registry {
    private final Map<String, Stub> stubs = new ConcurrentHashMap<>();
}
```

### Why Go Is Different

- **`map[string]*Stub`** looks generic but Go didn't have user-defined generics until Go 1.18 (2022). Maps, slices, and channels are built-in generic types.
- **Before Go 1.18**, you used `interface{}` (empty interface) and type assertions -- like Java before generics.
- **Go 1.18+ generics** exist (`[T any]` syntax) but are not heavily used in this codebase. Go idioms prefer interfaces and composition over generics.
- **`interface{}`** (or Go 1.18+ `any`) is the universal type -- like Java's `Object`, but every type satisfies it implicitly.
- **Type assertions** `v.(SomeType)` are Go's equivalent of Java casts, but with a safe two-value form: `v, ok := x.(SomeType)`.

---

## 11. defer

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/stub/registry.go`, lines 12-30

```go
func (r *Registry) Get(id string) (*Stub, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()   // runs when Get returns
    s, ok := r.stubs[id]
    return s, ok
}

func (r *Registry) Set(id string, stub *Stub) {
    r.mu.Lock()
    defer r.mu.Unlock()    // runs when Set returns, even on panic
    r.stubs[id] = stub
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 55-70

```go
func (w *HTTPWriter) Write(...) error {
    f, err := os.Create("somefile")
    if err != nil {
        return err
    }
    defer f.Close()   // guaranteed to run
    // write to file...
}
```

### The Java Approach

```java
public Stub get(String id) {
    mu.readLock().lock();
    try {
        return stubs.get(id);
    } finally {
        mu.readLock().unlock();
    }
}
```

### Why Go Is Different

- **`defer`** is a language keyword, not a convention. It schedules a function call to run when the enclosing function returns.
- **No `try`/`finally` block.** `defer` is cleaner than nested try-finally for multiple resources.
- **Runs on panic.** `defer` runs even if the function panics (Go's equivalent of an uncaught exception). This makes it suitable for cleanup that must always happen.
- **LIFO order.** Multiple defers in the same function execute in last-in-first-out order.
- **Arguments are evaluated immediately**, not when the deferred function runs. This surprises Java programmers: `defer fmt.Println(x)` captures the value of `x` at the defer site, not at function return.
- **`defer f.Close()`** is the idiomatic Go way to ensure file handles are closed. In Java, you'd use try-with-resources or a try-finally block.

---

## 12. io.Reader / io.Writer

### The Go Code

**File:** `/Users/sunny/go_project/gochaos/internal/templating/engine.go`, lines 20-35

```go
func (e *Engine) Render(w io.Writer, name string, data interface{}) error {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.templates.ExecuteTemplate(w, name, data)
}
```

**File:** `/Users/sunny/go_project/gochaos/internal/response/http_writer.go`, lines 60-80

```go
func (w *HTTPWriter) WriteResponse(writer io.Writer, resp Response) error {
    // writes response body to any io.Writer
    _, err := io.WriteString(writer, resp.Body)
    return err
}
```

### The Java Approach

```java
public void render(Writer writer, String name, Object data) throws Exception {
    template.executeTemplate(writer, name, data);
}
```

### Why Go Is Different

- **`io.Writer`** is `type Writer interface { Write(p []byte) (n int, err error) }`. **`io.Reader`** is `type Reader interface { Read(p []byte) (n int, err error) }`.
- **Extremely simple interfaces.** Just one method each. Java's `Writer` has 17 methods (`write`, `flush`, `close`, `append`, etc.).
- **Composability.** `io.MultiWriter`, `io.TeeReader`, `bufio.Writer`, `gzip.Writer` -- everything composes because the interface is minimal.
- **`http.ResponseWriter`** satisfies `io.Writer` (it has a `Write([]byte) (int, error)` method). Any function that takes `io.Writer` can accept an `http.ResponseWriter`.
- **`io.WriteString`** is a utility that efficiently writes a string to an `io.Writer` without allocating a `[]byte`.
- **Java's `Writer`** is a class hierarchy (`OutputStreamWriter`, `FileWriter`, `StringWriter`), not an interface. Go's `io.Writer` interface is minimal and implicit.

---

## Summary Table

| Concept | Go Idiom | Java Equivalent | Key Difference |
|---|---|---|---|
| Interfaces | Implicit satisfaction | `implements` keyword | Go decouples producer from consumer |
| Structs | `type X struct` | `class X` | No inheritance, no constructors |
| Errors | `error` return value | Exceptions | Errors are values, not control flow |
| Concurrency | `goroutine` + `sync.RWMutex` | `Thread` + `ReadWriteLock` | Lightweight goroutines, explicit locking |
| Options | `func(*T)` variadic | Builder pattern | Functions over objects |
| Visibility | Capital = exported | `public`/`private` | Package-level, not class-level |
| Testing | Table-driven + `t.Run` | `@Test` + parameterized | Anonymous structs as test cases |
| HTTP | `http.Handler` func | `@RestController` | No reflection, no annotations |
| Embedding | Anonymous field | Inheritance / composition | Promotion, not overriding |
| Generics | Built-in map/slice only | Full generic types | Go 1.18+ generics, but sparingly used |
| defer | `defer fn()` | `try { } finally { }` | Language keyword, runs on panic |
| I/O | `io.Writer` (1 method) | `java.io.Writer` (17 methods) | Minimal interface = maximum composability |


## Source Files Referenced

All paths are relative to `/Users/sunny/go_project/gochaos/`:

- `internal/response/http_writer.go` -- `HTTPWriter` struct, `Writer` satisfaction, HTTP middleware pipeline
- `internal/response/writer.go` -- `Writer` interface definition
- `internal/response/fault.go` -- `Fault` struct, `Validate()` error return
- `internal/response/http_writer_test.go` -- table-driven tests
- `internal/stub/registry.go` -- `Registry` with `sync.RWMutex`, `defer`, composition
- `internal/stub/matching.go` -- matching engine
- `internal/admin/handler.go` -- `http.Handler` setup, route registration
- `internal/admin/mappings.go` -- error wrapping with `%w`, `errors.Is`
- `internal/matcher/matcher.go` -- `Matcher` interface
- `internal/spec/spec.go` -- type definitions without generics
- `internal/templating/engine.go` -- `io.Writer` parameter, `sync.RWMutex`
- `internal/log/log.go` -- ring buffer, `sync.Cond`, concurrency patterns
- `pkg/gmock/server.go` -- `Server` struct, `NewServer` constructor
- `pkg/gmock/options.go` -- functional options pattern

---

*This tutorial is based on the gochaos codebase -- a chaos engineering tool for HTTP services. The patterns shown are idiomatic Go, as used in a real production codebase.*

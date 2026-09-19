# Go for Java Programmers

> Learning Go through the `gochaos` codebase — a Go-native HTTP mock server with built-in chaos engineering.

**Target Audience**: Java developer with 3+ years experience. You know Spring Boot, Maven/Gradle, JUnit, and REST APIs. You want to learn Go by reading a real project.

**How to use this guide**: Each section shows Go code from this repo, compares it to the Java equivalent, and explains the mental model shift. The code is real — every example references actual file paths and line numbers from `gochaos`.

---

## Table of Contents

1. [Quick Start](#1-quick-start)
2. [No Classes, Just Structs](#2-no-classes-just-structs)
3. [Interfaces Are Satisfied Implicitly](#3-interfaces-are-satisfied-implicitly)
4. [Errors Are Values](#4-errors-are-values)
5. [Composition over Inheritance (Embedding)](#5-composition-over-inheritance-embedding)
6. [Visibility by Capitalization](#6-visibility-by-capitalization)
7. [Concurrency with sync.RWMutex](#7-concurrency-with-syncrwmutex)
    - [7.5. Lock-Free Counters with sync/atomic](#75-lock-free-counters-with-syncatomic)
    - [7.6. Multiple Mutexes for Granular Locking](#76-multiple-mutexes-for-granular-locking)
8. [Goroutines and Channels](#8-goroutines-and-channels)
    - [8.5. Cancellation with context.Context and select](#85-cancellation-with-contextcontext-and-select)
    - [8.6. time.Duration — Type-Safe Time Arithmetic](#86-timeduration--type-safe-time-arithmetic)
9. [defer for Cleanup](#9-defer-for-cleanup)
10. [Functional Options Pattern](#10-functional-options-pattern)
11. [Table-Driven Tests](#11-table-driven-tests)
12. [io.Reader / io.Writer](#12-ioreader--iowriter)
13. [HTTP Without a Framework](#13-http-without-a-framework)
14. [Packages and the Module System](#14-packages-and-the-module-system)
15. [Struct Tags and Serialization](#15-struct-tags-and-serialization)
16. [Type Assertions and Optional Interfaces](#16-type-assertions-and-optional-interfaces)
17. [Type Switches](#17-type-switches)
18. [Custom Error Types and Error Wrapping](#18-custom-error-types-and-error-wrapping)
19. [sort.Slice / sort.SliceStable](#19-sortslice--sortslicestable)
20. [Benchmarks and Race Tests](#20-benchmarks-and-race-tests)
21. [`net/http/httptest` Package](#21-nethttphttptest-package)
22. [Summary Reference Table](#22-summary-reference-table)

---

## 1. Quick Start

Before we dive into the language, let's see how Go projects work in practice.

### Building and Testing

```bash
# Clone the repo
git clone https://github.com/sunny809/gochaos
cd gochaos

# Build everything (no tests)
go build ./...

# Run all tests with the race detector
go test -race ./...

# Build the CLI binary
go build -o gochaos ./cmd/gmock

# Run it
./gochaos --help
```

### Project Layout

Compare to a typical Maven/Gradle project:

```
gochaos/                        # Java equivalent
├── cmd/gmock/                  #   gmock-app/src/main/java/com/example/
│   └── main.go                 #     Application.java (entry point)
├── pkg/gmock/                  #   gmock-lib/src/main/java/
│   └── server.go               #     Server.java (public API)
├── internal/                   #   No direct equivalent (Java modules?)
│   ├── spec/spec.go            #     model/StubDefinition.java
│   ├── stub/registry.go        #     service/StubRegistry.java
│   ├── matcher/matcher.go      #     matcher/RequestMatcher.java
│   ├── response/http_writer.go #     response/HttpResponseWriter.java
│   ├── admin/handler.go        #     controller/AdminController.java
│   └── log/log.go              #     util/RequestLogger.java
├── config/config.go            #   config/StubFileLoader.java
├── test/integration/           #   src/test/integration/
└── testdata/                   #   src/test/resources/
```

**Key differences from Java**:

| Aspect | Go | Java |
|--------|----|------|
| Build tool | `go build` / `go test` | Maven (`mvn compile`) or Gradle |
| Dependencies | `go.mod` (one file) | `pom.xml` or `build.gradle` (XML/Groovy) |
| Module path | `github.com/sunny809/gochaos` | `com.example.gmock` |
| Package names | Short, flat (`stub`, `spec`, `admin`) | Deep hierarchy (`com.example.gmock.internal.stub`) |
| Test files | `xxx_test.go` next to `xxx.go` | `src/test/java/` mirror tree |

**First impression**: Run `go test -race ./...` and see all tests pass. This is the Go equivalent of `mvn clean test` — but faster and with built-in race detection.

### What is gochaos?

gochaos is a **mock HTTP server** you can embed in Go tests or run as a CLI. Think of it as WireMock but native to Go — no JVM required.

```go
// Embed in a Go test
server := gmock.NewServer(gmock.WithPort(0))
server.Start()
defer server.Stop()

server.Stub(gmock.StubDefinition{
    Request:  gmock.RequestPattern{Method: "GET", URLPath: "/api/users"},
    Response: gmock.ResponseDefinition{Status: 200, Body: `{"users":[]}`},
})

// Your test code sends real HTTP requests to server.URL()
```

Java developers will recognize this as similar to WireMock's Java API — but the Go version uses structs and functional options instead of builders.

---

## 2. No Classes, Just Structs

### The Go Code

In Go, there is no `class` keyword. You define data with **structs** and behavior with **methods** (functions attached to a type).

**File**: `internal/stub/registry.go` (lines 22-29)

```go
// Record wraps a StubDefinition with internal metadata.
type Record struct {
    Definition spec.StubDefinition
    Priority   int
    sortKey    uint64
}
```

**File**: `internal/response/http_writer.go` (lines 38-43)

```go
// HTTPWriter is the concrete adapter that writes HTTP responses.
type HTTPWriter struct {
    logger      *slog.Logger
    disableGzip bool
    tmplEngine  *templating.Engine
}
```

**File**: `internal/spec/spec.go` (lines 15-44)

```go
type RequestPattern struct {
    Method       string            `json:"method,omitempty" yaml:"method,omitempty"`
    URLPath      string            `json:"urlPath,omitempty" yaml:"urlPath,omitempty"`
    URLPathRegex string            `json:"urlPathRegex,omitempty" yaml:"urlPathRegex,omitempty"`
    Headers      map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
    Body         *BodyPattern      `json:"body,omitempty" yaml:"body,omitempty"`
    Priority     int               `json:"priority,omitempty" yaml:"priority,omitempty"`
}
```

### What Java Would Look Like

```java
public class RequestPattern {
    private String method;
    private String urlPath;
    private String urlPathRegex;
    private Map<String, String> headers;
    private BodyPattern body;
    private int priority;

    // Constructor
    public RequestPattern(String method, String urlPath, ...) { ... }

    // Getters and setters for every field
    public String getMethod() { return method; }
    public void setMethod(String method) { this.method = method; }
    // ... 10 more getter/setter pairs
}
```

### Why Go Is Different

1. **No constructors.** In Java, `new RequestPattern()` calls a constructor you write. In Go, you create structs with literal syntax:

```go
pattern := RequestPattern{
    Method:  "GET",
    URLPath: "/api/users",
    Headers: map[string]string{"Accept": "application/json"},
}
```

If you need construction logic, you write a factory function — not a constructor:

**File**: `internal/response/http_writer.go` (lines 46-52)

```go
func NewHTTPWriter(logger *slog.Logger, disableGzip bool) *HTTPWriter {
    return &HTTPWriter{
        logger:      logger,
        disableGzip: disableGzip,
        tmplEngine:  templating.NewEngine(),
    }
}
```

2. **Zero values.** Every field in a Go struct has a default value:
   - `string` → `""` (empty string)
   - `int` → `0`
   - `*Pointer` → `nil`
   - `map` → `nil`
   - `slice` → `nil`

   In Java, primitives have defaults (`0`, `false`) but reference types default to `null`. Go's zero values mean you can often use a struct without explicitly initializing every field.

3. **No getters/setters convention.** In Java, fields are `private` and accessed through `getX()` / `setX()`. In Go, if a field should be readable, it's exported (capital letter) and accessed directly:

```go
rec := registry.Get("stub-1")
fmt.Println(rec.Priority)   // direct field access, no getPriority()
```

4. **Methods are separate from the struct.** Methods are defined outside the struct body, using a **receiver** parameter:

```go
type Record struct { ... }

// Method with pointer receiver
func (r *Record) String() string {
    return fmt.Sprintf("Record{ID: %s, Priority: %d}", r.Definition.ID, r.Priority)
}

// Method with value receiver (operates on a copy)
func (r Record) IsHighPriority() bool {
    return r.Priority < 5
}
```

Compare to Java, where methods are always inside the class body.

---

## 3. Interfaces Are Satisfied Implicitly

This is probably the biggest mental model shift for Java developers.

### The Go Code

**File**: `internal/response/writer.go` (lines 14-22)

```go
// Writer is the port for writing HTTP responses.
type Writer interface {
    WriteResponse(w http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions) error
    WriteCORSHeaders(w http.ResponseWriter, r *http.Request, corsOpts *CORSOptions)
}
```

**File**: `internal/response/http_writer.go` (lines 38-43, 56-118)

```go
type HTTPWriter struct { ... }

// HTTPWriter implements WriteResponse — but nowhere does it say "implements Writer"
func (w *HTTPWriter) WriteResponse(rw http.ResponseWriter, def *spec.StubDefinition, req *http.Request, corsOpts *CORSOptions) error {
    // ... implementation ...
}

func (w *HTTPWriter) WriteCORSHeaders(rw http.ResponseWriter, r *http.Request, corsOpts *CORSOptions) {
    // ... implementation ...
}
```

**File**: `internal/matcher/matcher.go` (lines 14-46)

```go
// Matcher is the interface for request matching.
type Matcher interface {
    Match(req *http.Request) bool
    ScoreMatch(req *http.Request) (matched bool, score int)
    String() string
}

// MatcherFunc is a function adapter — makes any function a Matcher.
type MatcherFunc func(req *http.Request) bool

func (f MatcherFunc) Match(req *http.Request) bool {
    return f(req)
}

func (f MatcherFunc) ScoreMatch(req *http.Request) (bool, int) {
    return f(req), 1
}

func (f MatcherFunc) String() string {
    return "func matcher"
}
```

### What Java Would Look Like

```java
// Java requires explicit "implements" keyword
public class HttpWriter implements Writer {
    @Override
    public void writeResponse(HttpServletResponse w, StubDefinition def, HttpRequest req, CorsOptions opts) { ... }

    @Override
    public void writeCorsHeaders(HttpServletResponse w, HttpRequest req, CorsOptions opts) { ... }
}
```

### Why Go Is Different

1. **No `implements` keyword.** A type satisfies an interface automatically if it has the required methods. The compiler checks this at the point of assignment, not at the point of definition.

2. **Structural typing.** The interface is defined by its method set, not by its name. If `HTTPWriter` has the same methods as `Writer`, it **is** a `Writer`.

3. **Small interfaces are idiomatic.** Go favors tiny interfaces (1-3 methods):

```go
type error interface { Error() string }           // 1 method
type Stringer interface { String() string }        // 1 method
type Reader interface { Read([]byte) (int, error) } // 1 method
type Writer interface { Write([]byte) (int, error) } // 1 method
```

Compare to Java's `java.util.List` with 25+ methods.

4. **Consumer defines the interface.** In the `Matcher` example, the `Matcher` interface is defined in `internal/matcher/matcher.go` — the consumer of matching behavior. The concrete matchers (method, path, header, cookie, query, body, accept) each implement `Matcher` without knowing it exists. They just happen to have the right methods.

5. **`MatcherFunc` adapter pattern.** `MatcherFunc` is a named function type that implements `Matcher` by delegating to the function itself. This is Go's equivalent of Java's functional interfaces:

```java
// Java equivalent: Matcher is a @FunctionalInterface
Matcher m = req -> req.getMethod().equals("GET");
```

6. **Interface satisfaction is checked at compile time**, but only where the interface is used:

```go
// This line would fail to compile if HTTPWriter didn't satisfy Writer
var _ Writer = (*HTTPWriter)(nil)  // compile-time check
```

This is Go's version of Java's `@Override` annotation — it forces the compiler to verify interface satisfaction at a specific point.

---

## 4. Errors Are Values

Java uses exceptions (try/catch/throw). Go uses **error values** returned from functions.

### The Go Code

**File**: `internal/response/fault.go` (lines 33-49)

```go
// ValidateFaultType checks whether the given fault type is valid.
func ValidateFaultType(faultType string) error {
    if faultType == "" {
        return nil    // nil means "no error"
    }
    if validFaultTypes[faultType] {
        return nil
    }
    return fmt.Errorf("invalid fault type %q; valid types: %s",
        faultType, strings.Join(valid, ", "))
}
```

**File**: `internal/admin/mappings.go` (lines 21-29)

```go
id, err := h.registry.Add(def)
if err != nil {
    var valErr *stub.ValidationError
    if errors.As(err, &valErr) {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    writeError(w, http.StatusInternalServerError, err.Error())
    return
}
```

**File**: `internal/stub/registry.go` (lines 15-21)

```go
// ValidationError is a custom error type.
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
```

**File**: `internal/response/http_writer.go` (lines 101-103)

```go
_, err := io.Copy(rw, strings.NewReader(body))
if err != nil {
    return fmt.Errorf("failed to write response body: %w", err)
}
```

### What Java Would Look Like

```java
// Java throws exceptions
public class ValidationException extends RuntimeException {
    private final String field;
    public ValidationException(String field, String message) { ... }
}

// Caller uses try/catch
try {
    registry.add(def);
} catch (ValidationException e) {
    response.sendError(400, e.getMessage());
} catch (Exception e) {
    response.sendError(500, e.getMessage());
}
```

### Why Go Is Different

1. **Errors are return values, not control flow.** Functions that can fail return `(result, error)`. The caller checks the error explicitly:

```go
id, err := registry.Add(def)
if err != nil {
    // handle error
}
```

There is no `throws` declaration, no `try` block, no `catch` clause.

2. **The `error` interface is minimal.** Any type with an `Error() string` method is an error:

```go
type error interface {
    Error() string
}
```

This is Go's equivalent of Java's `Throwable` — but infinitely simpler.

3. **`errors.As` for type checking.** In Java, you use `instanceof` to check exception types:

```java
try { ... }
catch (ValidationException e) { ... }  // instanceof check built into catch
```

In Go, you use `errors.As`:

```go
var valErr *stub.ValidationError
if errors.As(err, &valErr) {
    // valErr is now a *stub.ValidationError
}
```

4. **Error wrapping with `%w`.** Go lets you wrap errors with additional context:

```go
return fmt.Errorf("failed to write response body: %w", err)
```

The `%w` verb creates a wrapped error. Callers can still access the original error through `errors.As` or `errors.Is` — similar to Java's exception chaining (cause).

5. **Sentinel errors.** Go uses package-level error variables as sentinels (like Java's constants):

```go
var ErrNotFound = errors.New("stub not found")
// Usage: errors.Is(err, ErrNotFound)
```

6. **No stack traces by default.** `fmt.Errorf` doesn't capture a stack trace. If you need traces, you add them explicitly or use a third-party package. This is intentional — errors in Go are lightweight values, not heavyweight exception objects.

---

## 5. Composition over Inheritance (Embedding)

Go has no inheritance. No `extends` keyword. Instead, Go uses **struct embedding** — a form of composition where one struct includes another without a field name.

### The Go Code

**File**: `internal/response/http_writer.go` (lines 274-293)

```go
type GzipResponseWriter struct {
    http.ResponseWriter    // embedded — no field name!
    GW *gzip.Writer
}

// Write overrides the embedded ResponseWriter's Write method.
func (w *GzipResponseWriter) Write(b []byte) (int, error) {
    return w.GW.Write(b)  // compress through gzip
}

// Unwrap returns the underlying ResponseWriter.
func (w *GzipResponseWriter) Unwrap() http.ResponseWriter {
    return w.ResponseWriter
}
```

**File**: `internal/response/http_writer_test.go` (lines 993-1008)

```go
type mockHijacker struct {
    http.ResponseWriter    // embedded
    hijacked bool
    conn     net.Conn
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
    m.hijacked = true
    server, client := net.Pipe()
    m.conn = server
    _ = client.Close()
    return server, nil, nil
}
```

### What Java Would Look Like

```java
// Java has no direct equivalent. The closest is HttpServletResponseWrapper:
public class GzipResponseWriter extends HttpServletResponseWrapper {
    private final GzipWriter gw;

    public GzipResponseWriter(HttpServletResponse response, GzipWriter gw) {
        super(response);  // delegation through wrapper
        this.gw = gw;
    }

    @Override
    public void write(byte[] b) throws IOException {
        gw.write(b);  // manual delegation
    }
}
```

### Why Go Is Different

1. **Embedding promotes methods.** When `GzipResponseWriter` embeds `http.ResponseWriter`, all of `http.ResponseWriter`'s methods are "promoted" to `GzipResponseWriter`. You can call `gw.Header()`, `gw.WriteHeader()`, and `gw.Write()` directly — they are automatically delegated to the embedded writer.

2. **Override by shadowing.** If the outer struct defines a method with the same name, it shadows the embedded method. `GzipResponseWriter.Write()` shadows `http.ResponseWriter.Write()`. There's no `@Override` annotation and no way to call "super" — though you can reach the embedded field explicitly:

```go
// To call the original Write:
gw.ResponseWriter.Write(b)  // qualify the embedded field
```

3. **Embedding is NOT inheritance.** `*GzipResponseWriter` is NOT a subtype of `*http.ResponseWriter`. You can assign `*GzipResponseWriter` to an `http.ResponseWriter` interface variable (because it satisfies the interface), but not to a `*http.ResponseWriter` struct variable.

4. **No `super` keyword.** In Java, `super.method()` calls the parent class's implementation. In Go, you call through the embedded field by name: `gw.ResponseWriter.Write(b)`.

5. **Multiple embedding.** A struct can embed multiple types:

```go
type Logger struct {
    *slog.Logger          // embedded
    io.Closer             // embedded
}
```

This is Go's version of multiple inheritance — without the diamond problem, because it's delegation, not inheritance.

---

## 6. Visibility by Capitalization

Java has `public`, `protected`, `private`, and package-private. Go has one rule: **capital letter = exported, lowercase = unexported**.

### The Go Code

**File**: `internal/stub/registry.go` (lines 1-38)

```go
package stub

// Registry is exported (capital R)
type Registry struct {
    mu      sync.RWMutex       // unexported (lowercase mu)
    stubs   map[string]*Record // unexported
    ordered []*Record          // unexported
    nextSeq uint64             // unexported
}

// Get is exported (capital G)
func (r *Registry) Get(id string) *spec.StubDefinition {
    r.mu.RLock()  // accessing unexported field within the same package — OK
    defer r.mu.RUnlock()
    // ...
}
```

**File**: `internal/response/fault.go` (lines 17-31)

```go
// validFaultTypes is unexported — package-internal only.
var validFaultTypes = map[string]bool{
    "error":            true,
    "empty":            true,
    "connection_reset": true,
}

// ValidFaultTypes is exported — returns a copy so callers can't mutate the internal map.
func ValidFaultTypes() map[string]bool {
    result := make(map[string]bool, len(validFaultTypes))
    for k, v := range validFaultTypes {
        result[k] = v
    }
    return result
}

// ValidateFaultType is exported — package's public API.
func ValidateFaultType(faultType string) error {
    // can access validFaultTypes because we're in the same package
    if validFaultTypes[faultType] {
        return nil
    }
    // ...
}
```

### What Java Would Look Like

```java
package com.example.stub;

public class Registry {
    private final ReadWriteLock mu = new ReentrantReadWriteLock();  // private
    private final Map<String, Record> stubs = new HashMap<>();      // private

    public Record get(String id) { return stubs.get(id); }         // public
}
```

### Why Go Is Different

1. **One rule replaces 4 keywords.** `public` → capital letter. `private` → lowercase. No `protected` keyword (Go has no inheritance anyway). No `package-private` distinction (same package = everything accessible).

2. **`internal/` package convention.** This is Go's mechanism for restricting imports to a specific module tree. Packages under `internal/` can only be imported by code rooted at the parent:

```
gochaos/
├── internal/          ← only importable by gochaos itself
│   ├── stub/          ← not importable by external modules
│   ├── response/      ← not importable by external modules
│   └── admin/         ← not importable by external modules
├── pkg/gmock/         ← importable by anyone
```

Java's closest equivalent is Java modules (`module-info.java`) with `exports` directives.

3. **Defensive copy pattern.** Since there are no `private` fields at the package level, returning a copy is the idiomatic way to protect internal state:

```go
func ValidFaultTypes() map[string]bool {
    result := make(map[string]bool, len(validFaultTypes))
    for k, v := range validFaultTypes {
        result[k] = v
    }
    return result  // caller gets a copy, can't modify the original
}
```

---

## 7. Concurrency with sync.RWMutex

Go's approach to thread safety is explicit: use `sync.RWMutex` for reader/writer locks, and pair `Lock()` with `defer Unlock()`.

### The Go Code

**File**: `internal/stub/registry.go` (lines 31-86)

```go
type Registry struct {
    mu      sync.RWMutex
    stubs   map[string]*Record
    ordered []*Record
    nextSeq uint64
}

// Read method: acquire read lock
func (r *Registry) Get(id string) *spec.StubDefinition {
    r.mu.RLock()
    defer r.mu.RUnlock()

    rec, ok := r.stubs[id]
    if !ok {
        return nil
    }
    def := rec.Definition
    return &def  // return a copy to avoid race conditions
}

// Write method: acquire write lock
func (r *Registry) Add(def spec.StubDefinition) (string, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    // ... modify map and ordered slice ...
    r.stubs[def.ID] = rec
    r.rebuildOrdered()
    return def.ID, nil
}
```

**File**: `internal/log/log.go` (lines 26-32, 48-80)

```go
type RequestLog struct {
    mu      sync.RWMutex
    entries []Entry
    max     int
    head    int
    count   int
}

func (l *RequestLog) Record(req *http.Request, matched bool, stubID string) {
    // ... build entry ...
    l.mu.Lock()
    defer l.mu.Unlock()
    l.entries[l.head] = entry
    l.head = (l.head + 1) % l.max
    if l.count < l.max {
        l.count++
    }
}
```

### What Java Would Look Like

```java
public class Registry {
    private final ReadWriteLock lock = new ReentrantReadWriteLock();
    private final Map<String, Record> stubs = new ConcurrentHashMap<>();

    public Record get(String id) {
        lock.readLock().lock();
        try {
            return stubs.get(id);
        } finally {
            lock.readLock().unlock();
        }
    }

    public String add(StubDefinition def) {
        lock.writeLock().lock();
        try {
            // modify map and list
        } finally {
            lock.writeLock().unlock();
        }
    }
}
```

### Why Go Is Different

1. **`defer` pairs naturally with locks.** The lock acquire and release are adjacent in the code:

```go
r.mu.Lock()
defer r.mu.Unlock()
```

In Java, the `unlock()` call in a `finally` block is visually far from `lock()`. With `defer`, they're on consecutive lines — you can't forget to unlock.

2. **`RLock()` / `RUnlock()` for read concurrency.** Multiple goroutines can hold the read lock simultaneously. Only write locks (`Lock()`) are exclusive. This is critical for performance — reads are the common case in request matching.

3. **No `ConcurrentHashMap` equivalent.** Go doesn't have a concurrent map in the standard library (though `sync.Map` exists for specific use cases). Instead, you protect a regular `map` with a mutex. This is more explicit but also more flexible.

4. **Return copies to avoid races.** In `Get()`, the method returns a copy of the struct, not a pointer to the internal entry:

```go
def := rec.Definition  // copy
return &def
```

This prevents the caller from modifying the registry's internal state through a shared pointer. In Java, you'd either clone the object or design it to be immutable.

---

### 7.5. Lock-Free Counters with sync/atomic

Go's `sync/atomic` package provides low-level atomic operations on primitive types. This is the Go equivalent of Java's `AtomicLong`, `AtomicInteger`, etc.

#### The Go Code

**File**: `internal/stub/registry.go` (lines 233-241)

```go
// IncrementHitCount atomically increments the hit count for the stub.
func (r *Registry) IncrementHitCount(id string) uint64 {
    r.mu.RLock()
    defer r.mu.RUnlock()

    rec, ok := r.stubs[id]
    if !ok {
        return 0
    }
    // Atomic increment - no lock needed for this specific field.
    return atomic.AddUint64(&rec.hitCount, 1)
}

// GetHitCount atomically reads the hit count.
func (r *Registry) GetHitCount(id string) uint64 {
    r.mu.RLock()
    defer r.mu.RUnlock()

    rec, ok := r.stubs[id]
    if !ok {
        return 0
    }
    return atomic.LoadUint64(&rec.hitCount)
}
```

**File**: `internal/stub/registry.go` (lines 38-41)

```go
type Record struct {
    // ... other fields ...
    hitCount uint64  // Must be accessed atomically!
}
```

#### The Java Equivalent

```java
public class Record {
    private final AtomicLong hitCount = new AtomicLong(0);
    
    public long incrementHitCount() {
        return hitCount.incrementAndGet();
    }
    
    public long getHitCount() {
        return hitCount.get();
    }
}
```

#### Why Go is Different

| Aspect | Go | Java |
|--------|-----|------|
| **Abstraction level** | Raw memory operations on `*uint64` | Wrapper class (`AtomicLong`) |
| **Allocation** | Zero — counter is a struct field | Heap allocation for `AtomicLong` object |
| **Type safety** | You must remember to use `atomic` | Compiler enforces use of `AtomicLong` |
| **Operations** | `atomic.AddUint64(&x, delta)` | `x.incrementAndGet()` |
| **Granularity** | Per-field atomicity | Per-object atomicity |

**Key mental model shift**: In Go, `atomic` operates on *memory addresses* (`&field`), not objects. The counter is just a `uint64` field in your struct — no wrapper needed. This is zero-allocation and more efficient, but you must consciously choose to use it. The compiler won't warn you if you forget.

#### Common atomic Operations

| Go Function | Purpose | Java Equivalent |
|-------------|---------|-----------------|
| `atomic.LoadUint64(&x)` | Read atomically | `x.get()` |
| `atomic.StoreUint64(&x, val)` | Write atomically | `x.set(val)` |
| `atomic.AddUint64(&x, delta)` | Add atomically | `x.addAndGet(delta)` |
| `atomic.SwapUint64(&x, new)` | Swap atomically | `x.getAndSet(new)` |
| `atomic.CompareAndSwapUint64(&x, old, new)` | CAS | `x.compareAndSet(old, new)` |

#### When to Use atomic vs Mutex

- **Use `atomic`**: For simple counters, flags, single-field updates
- **Use `sync.Mutex`**: When you need to update multiple fields atomically, or perform complex logic

The `hitCount` in `gochaos` uses `atomic` because:
1. It's a single counter field
2. The read lock on `r.mu` already protects access to the `Record` struct itself
3. `atomic.AddUint64` is faster than acquiring a second lock just for the counter

---

### 7.6. Multiple Mutexes for Granular Locking

When a struct has independent state that doesn't need to be locked together, use multiple mutexes to reduce contention.

#### The Go Code

**File**: `internal/stub/registry.go` (lines 47-58)

```go
type Registry struct {
    // Main mutex: protects stubs, ordered, nextSeq
    mu      sync.RWMutex
    stubs   map[string]*Record
    ordered []*Record
    nextSeq uint64

    // Separate mutex: protects rateLimitStates.
    // This avoids holding the read lock during token-bucket computation,
    // which would block stub additions/deletions.
    rateLimitMu     sync.Mutex
    rateLimitStates map[string]*rateLimitState
}
```

**File**: `internal/stub/registry.go` (lines 293-310)

```go
func (r *Registry) ShouldRateLimit(id string, afterRequests, perSecond int) bool {
    // Only acquire rateLimitMu, not the main mu.
    // This allows other goroutines to add/remove stubs concurrently.
    r.rateLimitMu.Lock()
    defer r.rateLimitMu.Unlock()

    state, ok := r.rateLimitStates[id]
    if !ok {
        state = &rateLimitState{tokens: float64(afterRequests)}
        r.rateLimitStates[id] = state
    }
    
    // ... token bucket computation ...
    state.tokens = min(float64(perSecond), state.tokens + elapsed*float64(perSecond))
    
    if state.tokens >= 1 {
        state.tokens -= 1
        return false  // Not rate limited
    }
    return true  // Rate limited
}
```

#### The Java Equivalent

```java
public class Registry {
    private final ReadWriteLock mainLock = new ReentrantReadWriteLock();
    private final Map<String, Record> stubs = new HashMap<>();
    
    // Separate lock for rate limiting
    private final ReentrantLock rateLimitLock = new ReentrantLock();
    private final Map<String, RateLimitState> rateLimitStates = new HashMap<>();
    
    public boolean shouldRateLimit(String id, int afterRequests, int perSecond) {
        rateLimitLock.lock();
        try {
            // ... computation ...
        } finally {
            rateLimitLock.unlock();
        }
    }
}
```

#### Why Go is Different

| Aspect | Go | Java |
|--------|-----|------|
| **Comment convention** | Go idiomatic: document what each mutex protects | Java: `@GuardedBy` annotation (optional) |
| **Lock naming** | Typically `mu`, `xxxMu` | Typically `lock`, `xxxLock` |
| **Granularity reasoning** | "Avoid blocking X while doing Y" — explicit in comments | Same reasoning, but less common to document |

**Key insight**: The comment `// This avoids holding the read lock during token-bucket computation` is critical. In Go, documenting *why* a lock exists is idiomatic. This helps future readers understand lock granularity decisions.

**Anti-pattern to avoid**: A single giant mutex that protects everything. This creates contention and reduces concurrency. In `gochaos`, rate-limit checking happens frequently (every request), while stub additions/deletions are rare. Separating the locks prevents frequent rate-limit checks from blocking rare admin operations.

---

## 8. Goroutines and Channels

Goroutines are lightweight threads. Channels are typed conduits for communication between goroutines.

### The Go Code

**File**: `pkg/gmock/server.go` (lines 149-152)

```go
// Start launches HTTP servers in separate goroutines.
func (s *mockServer) Start() error {
    // ...

    // Launch the main HTTP server in a goroutine
    go s.httpServer.Serve(listener)  // non-blocking

    // Launch the admin server in a separate goroutine
    if useSeparateAdminPort {
        go s.adminServer.Serve(adminListener)  // non-blocking
    }
    return nil
}
```

**File**: `internal/response/http_writer_test.go` (lines 1309-1340)

```go
func TestHTTPWriter_ApplyFault_ConcurrentSafety(t *testing.T) {
    w := NewHTTPWriter(logger, true)

    faults := []*spec.FaultDefinition{
        nil,
        {Type: "error"},
        {Type: "empty"},
        {Type: "connection_reset"},
    }

    done := make(chan struct{}, 100)  // buffered channel as counting semaphore
    for i := 0; i < 100; i++ {
        go func(idx int) {
            defer func() { done <- struct{}{} }()  // signal completion
            rr := httptest.NewRecorder()
            def := &spec.StubDefinition{
                ID: fmt.Sprintf("concurrent-fault-%d", idx),
                Response: spec.ResponseDefinition{
                    Status: 200,
                    Body:   fmt.Sprintf("response-%d", idx),
                    Fault:  faults[idx%len(faults)],
                },
            }
            _ = w.WriteResponse(rr, def, httptest.NewRequest(http.MethodGet, "/test", nil), nil)
        }(i)
    }

    // Wait for all 100 goroutines to complete
    for i := 0; i < 100; i++ {
        <-done  // blocks until a value is received
    }
}
```

### What Java Would Look Like

```java
// Java equivalent — using ExecutorService
ExecutorService executor = Executors.newFixedThreadPool(10);
CountDownLatch latch = new CountDownLatch(100);

for (int i = 0; i < 100; i++) {
    int idx = i;
    executor.submit(() -> {
        try {
            // ... write response ...
        } finally {
            latch.countDown();
        }
    });
}

latch.await(10, TimeUnit.SECONDS);
executor.shutdown();
```

### Why Go Is Different

1. **`go` keyword launches a goroutine.** It's that simple. No `ExecutorService`, no `Thread` subclass, no `Runnable`. Just `go fn()`.

2. **Goroutines are cheap.** A goroutine starts with ~4KB of stack (vs ~1MB for a Java thread). You can have thousands of goroutines in a single process. The test above spawns 100 without breaking a sweat.

3. **Channels are typed.** `chan struct{}` is a channel that carries empty structs (zero bytes of data). `chan int` carries integers. The type system ensures you can't send the wrong type.

4. **Channel as semaphore.** The `done` channel is buffered (capacity 100). Each goroutine sends a value when done. The main goroutine receives 100 times, blocking each time until a value is available. This is Go's built-in `CountDownLatch`.

5. **`struct{}{}` as a signal.** Go uses empty structs in channels when only the event matters, not the data. `struct{}{}` takes zero memory — it's purely a synchronization signal.

6. **No thread pool configuration.** Java requires you to choose between `newCachedThreadPool()`, `newFixedThreadPool(10)`, etc. Go's goroutines are multiplexed onto OS threads by the Go runtime — you don't manage the pool.

---

### 8.5. Cancellation with context.Context and select

Go's `context.Context` provides a standard way to cancel operations. Combined with `select`, you can wait on multiple channels simultaneously — the first one ready wins.

#### The Go Code

**File**: `internal/response/http_writer.go` (lines 268-275)

```go
func (w *HTTPWriter) writeDribbleBody(rw http.ResponseWriter, resp spec.ResponseDefinition, req *http.Request, stubID string, cfg *dribbleConfig) error {
    // Get context from the HTTP request — it's cancelled when client disconnects.
    ctx := req.Context()
    
    interval := time.Duration(cfg.totalDuration) / time.Duration(cfg.chunks)
    body := w.renderBody(resp, req, stubID)
    
    for i := 0; i < cfg.chunks; i++ {
        // Check if client has disconnected.
        if ctx.Err() != nil {
            return nil  // Stop writing, client is gone.
        }
        
        // Write one chunk.
        start := i * chunkSize
        end := min(start+chunkSize, len(body))
        if _, err := rw.Write([]byte(body[start:end])); err != nil {
            return err
        }
        if f, ok := rw.(http.Flusher); ok {
            f.Flush()  // Send chunk immediately.
        }
        
        // Sleep between chunks (not after the last one).
        if i < cfg.chunks-1 {
            select {
            case <-ctx.Done():
                // Client disconnected — stop sending chunks.
                return nil
            case <-time.After(interval):
                // Interval elapsed — continue to next chunk.
            }
        }
    }
    return nil
}
```

**File**: `internal/response/http_writer.go` (lines 246-260) — Timeout delay

```go
func (w *HTTPWriter) applyDelay(delay *spec.DelayDefinition, ctx context.Context) delayResult {
    if delay == nil {
        return delayResult{}
    }
    
    switch delay.Type {
    case "timeout":
        // Block forever until context is cancelled.
        select {
        case <-ctx.Done():
            // Client disconnected or server shutting down.
        }
        return delayResult{applied: true}
    
    case "fixed":
        select {
        case <-time.After(time.Duration(delay.Value) * time.Millisecond):
            // Delay elapsed.
        case <-ctx.Done():
            // Cancelled before delay finished.
        }
        return delayResult{applied: true}
    // ...
    }
}
```

#### The Java Equivalent

Java has no direct equivalent. You'd use `Future.cancel()` or thread interruption:

```java
// Java approach 1: ExecutorService and Future
ExecutorService executor = Executors.newSingleThreadExecutor();
Future<?> future = executor.submit(() -> {
    for (int i = 0; i < chunks; i++) {
        if (Thread.currentThread().isInterrupted()) {
            return;  // Stop writing.
        }
        // Write chunk...
        Thread.sleep(interval.toMillis());
    }
});

// To cancel:
future.cancel(true);  // 'true' means interrupt the thread.

// Java approach 2: CompletableFuture with timeout
CompletableFuture<Void> future = CompletableFuture.runAsync(() -> {
    // Write chunks...
}, executor)
.orTimeout(totalDuration.toMillis(), TimeUnit.MILLISECONDS);

// To cancel:
future.cancel(true);
```

#### Why Go is Different

| Aspect | Go | Java |
|--------|-----|------|
| **Cancellation primitive** | `context.Context` (interface) | `Future.cancel()` + `Thread.interrupt()` |
| **Cancellation signal** | Channel close (`ctx.Done()` returns a closed channel) | Thread interruption (throws `InterruptedException`) |
| **Waiting on multiple conditions** | `select` keyword (built-in) | No built-in equivalent — use `CompletableFuture.anyOf()` |
| **Timeout support** | `context.WithTimeout(parent, duration)` | `CompletableFuture.orTimeout()` (Java 9+) |
| **Exception on cancel** | No — check `ctx.Err()` or `ctx.Done()` | Yes — `InterruptedException`, `CancellationException` |

**Key mental model shift**:
1. **Go cancellation is cooperative**: Code must check `ctx.Done()` or `ctx.Err()`. No forced interruption.
2. **`select` is a language keyword**: `select { case <-ch1: ... case <-ch2: ... }` waits on multiple channels. First ready wins.
3. **`time.After(d)` returns a channel**: Not a sleep call. It sends the current time on a channel after duration `d`.
4. **`ctx.Done()` is a channel**: When context is cancelled, this channel is closed. Reading from a closed channel always succeeds immediately (returns zero value).

#### Common Patterns

| Pattern | Go | Java |
|---------|-----|------|
| **Cancel on timeout** | `ctx, cancel := context.WithTimeout(parent, 5*time.Second)` | `future.orTimeout(5, TimeUnit.SECONDS)` |
| **Cancel on parent cancel** | `ctx, cancel := context.WithCancel(parent)` — inherits cancellation | Child futures don't auto-cancel when parent cancels |
| **Cancel manually** | `cancel()` function returned by `WithCancel/WithTimeout` | `future.cancel(true)` |
| **Check if cancelled** | `if ctx.Err() != nil { ... }` or `select { case <-ctx.Done(): ... }` | `Thread.currentThread().isInterrupted()` |
| **Propagate to children** | Pass `ctx` to child goroutines — they inherit cancellation | Manually cascade `cancel()` to child futures |

**Why `req.Context()` is important**: In Go's `net/http`, every request has a context that's cancelled when:
- The client disconnects (connection closed)
- The server shuts down (`Shutdown()` called)
- A timeout configured on the server fires (`ReadTimeout`, `WriteTimeout`)

Using `req.Context()` means your code automatically respects all these cancellation conditions. No manual cleanup needed.

---

### 8.6. time.Duration — Type-Safe Time Arithmetic

Go's `time.Duration` is a named type (`int64` nanoseconds) that makes time arithmetic type-safe and readable.

#### The Go Code

**File**: `internal/delayx/lognormal.go` (lines 92-99)

```go
func Sample(mu, sigma float64, rng randx.RNG) time.Duration {
    z := rng.NormFloat64()           // Standard normal sample.
    x := math.Exp(mu + sigma*z)       // Lognormal sample in milliseconds.
    if x < 0 {
        x = 0
    }
    // Convert float milliseconds to time.Duration.
    return time.Duration(x) * time.Millisecond
}
```

**File**: `pkg/gmock/server.go` (lines 157-158)

```go
// Record server start time for time-window calculations.
s.startTime = time.Now()

// Later: check elapsed time.
elapsed := time.Since(s.startTime).Milliseconds()
```

#### The Java Equivalent

```java
import java.time.Duration;

public Duration sample(double mu, double sigma, Random rng) {
    double z = rng.nextGaussian();
    double x = Math.exp(mu + sigma * z);
    if (x < 0) x = 0;
    return Duration.ofMillis((long) x);
}

// Record start time.
Instant startTime = Instant.now();

// Later: check elapsed time.
long elapsedMs = Duration.between(startTime, Instant.now()).toMillis();
```

#### Why Go is Different

| Aspect | Go | Java |
|--------|-----|------|
| **Type definition** | `type Duration = int64` (nanoseconds) | `class Duration` (object) |
| **Creation** | `time.Duration(ms) * time.Millisecond` | `Duration.ofMillis(ms)` |
| **Arithmetic** | `d1 + d2`, `d * 2`, `d / 3` (native operators) | `d1.plus(d2)`, `d.multipliedBy(2)`, `d.dividedBy(3)` (methods) |
| **Units** | Constants: `time.Millisecond`, `time.Second`, etc. | Enum: `ChronoUnit.MILLIS`, `ChronoUnit.SECONDS` |
| **Printing** | `fmt.Println(d)` → "1.5s" (human-readable) | `d.toString()` → "PT1.5S" (ISO-8601) |
| **Parse from string** | `time.ParseDuration("1.5s")` | `Duration.parse("PT1.5S")` |

**Key mental model shift**:
1. **`time.Duration` is an integer**: It's `int64` nanoseconds. You can do arithmetic with `+`, `-`, `*`, `/` directly.
2. **Constants are typed values**: `time.Millisecond` is a `time.Duration` constant (1,000,000 nanoseconds). Multiplying by it converts units.
3. **Conversion pattern**: `time.Duration(floatMs) * time.Millisecond` converts a float milliseconds to Duration.
4. **`time.Since(start)`**: Returns elapsed Duration since a `time.Time`. More readable than Java's `Duration.between(start, Instant.now())`.
5. **`.Milliseconds()`, `.Seconds()`**: Extract integer units. Java uses `.toMillis()`, `.toSeconds()`.

#### Common Patterns

| Operation | Go | Java |
|-----------|-----|------|
| **5 seconds** | `5 * time.Second` | `Duration.ofSeconds(5)` |
| **1.5 seconds** | `1500 * time.Millisecond` or `time.Duration(1500) * time.Millisecond` | `Duration.ofMillis(1500)` or `Duration.ofSeconds(1, 500_000_000)` |
| **Add durations** | `d1 + d2` | `d1.plus(d2)` |
| **Multiply by scalar** | `d * 2` | `d.multipliedBy(2)` |
| **Compare** | `d1 < d2` (native comparison) | `d1.compareTo(d2) < 0` |
| **Parse** | `time.ParseDuration("2h30m")` (accepts "h", "m", "s", "ms", "us", "ns") | `Duration.parse("PT2H30M")` (ISO-8601 only) |

**Go's advantage**: The `ParseDuration` format is more human-friendly. `"2h30m"` is easier to read than `"PT2H30M"`. The arithmetic operators are more intuitive than method chaining.

---

## 9. defer for Cleanup

`defer` schedules a function call to run when the enclosing function returns. It's Go's `finally` block — but better.

### The Go Code

**File**: `internal/stub/registry.go` (lines 49-51)

```go
func (r *Registry) Add(def spec.StubDefinition) (string, error) {
    r.mu.Lock()
    defer r.mu.Unlock()  // guaranteed to run when Add returns
    // ... critical section ...
}
```

**File**: `internal/response/http_writer.go` (lines 68-75)

```go
gw, rw := w.maybeWrapGzip(rw, req)
if gw != nil {
    defer func() {
        if err := gw.Close(); err != nil {
            w.logger.Warn("failed to close gzip writer", "error", err)
        }
    }()
}
```

**File**: `pkg/gmock/server.go` (lines 120-122)

```go
func (s *mockServer) Start() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // ... start servers ...
}
```

### What Java Would Look Like

```java
public Record get(String id) {
    lock.readLock().lock();
    try {
        return stubs.get(id);
    } finally {
        lock.readLock().unlock();  // guaranteed to run
    }
}
```

### Why Go Is Different

1. **`defer` is a language keyword**, not a library pattern. It's built into the language semantics.

2. **Runs on panic.** `defer` executes even if the function panics (Go's equivalent of an uncaught exception). This makes it perfect for cleanup that must always happen — lock release, file close, connection close.

3. **No `try`/`finally` nesting.** With `defer`, multiple resources are handled linearly:

```go
f1, _ := os.Open("file1")
defer f1.Close()
f2, _ := os.Open("file2")
defer f2.Close()
```

In Java, nested try-finally blocks quickly become unreadable:

```java
try (FileReader f1 = new FileReader("file1")) {
    try (FileReader f2 = new FileReader("file2")) {
        // ...
    }
}
```

4. **LIFO order.** Multiple defers execute in last-in-first-out order — like a stack. Resources are released in reverse order of acquisition.

5. **Arguments are evaluated immediately.** This surprises Java developers:

```go
x := 1
defer fmt.Println(x)  // prints 1 (x is captured at defer time, not at function return)
x = 2                 // changing x doesn't affect the deferred call
```

To capture the current value at function return, use a closure:

```go
x := 1
defer func() { fmt.Println(x) }()  // prints 2 (closure captures variable reference)
x = 2
```

---

## 10. Functional Options Pattern

This is Go's idiomatic alternative to the Java Builder pattern.

### The Go Code

**File**: `pkg/gmock/options.go` (lines 4-137)

```go
// Option is a function that modifies a ServerConfig.
type Option func(*ServerConfig)

func WithPort(port int) Option {
    return func(c *ServerConfig) {
        c.Port = port
    }
}

func WithVerbose() Option {
    return func(c *ServerConfig) {
        c.Verbose = true
    }
}

func WithStubFiles(files ...string) Option {
    return func(c *ServerConfig) {
        c.StubFiles = append(c.StubFiles, files...)
    }
}

func WithCORSEnabled() Option {
    return func(c *ServerConfig) {
        c.CORSOptions = &CORSOptions{
            AllowedOrigins: []string{"*"},
            AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"},
            AllowedHeaders: []string{"Content-Type", "Authorization"},
            MaxAge:         86400,
        }
    }
}

func DefaultConfig() ServerConfig {
    return ServerConfig{
        Port:        0,
        MaxRequests: 1000,
    }
}
```

**File**: `pkg/gmock/server.go` (lines 92-116)

```go
func NewServer(opts ...Option) Server {
    cfg := DefaultConfig()
    for _, opt := range opts {
        opt(&cfg)  // apply each option to the config
    }
    // ... build server from cfg ...
}
```

Usage:

```go
server := gmock.NewServer(
    gmock.WithPort(9090),
    gmock.WithVerbose(),
    gmock.WithCORSEnabled(),
)
```

### What Java Would Look Like

```java
// Java Builder pattern
Server server = new Server.Builder()
    .withPort(9090)
    .withVerbose()
    .withCorsEnabled()
    .build();
```

### Why Go Is Different

1. **`Option` is a function type.** `type Option func(*ServerConfig)` is a function that modifies config. Each `With*` function returns a closure that captures the desired value and applies it to the config.

2. **No builder object.** There's no separate `Builder` class with mutable state. The config struct IS the state, and options are functions that mutate it.

3. **No `.build()` call.** Options are applied in a loop. The result is a fully configured config struct.

4. **Defaults are natural.** `DefaultConfig()` returns a config with sensible defaults. Options override specific fields. If no options are given, defaults apply.

5. **Extensible without modification.** Anyone in any package can create a new `Option` function. No builder subclassing or method overloading needed:

```go
// In any package:
func WithCustomTimeout(d time.Duration) gmock.Option {
    return func(c *gmock.ServerConfig) {
        c.Timeout = d
    }
}
```

6. **Variadic `...Option`** lets callers pass zero or more options:

```go
server := gmock.NewServer()           // all defaults
server := gmock.NewServer(opts...)    // slice of options
```

---

## 11. Table-Driven Tests

Go's testing style is distinctive: use anonymous structs to define test cases, iterate over them with a `for` loop, and use `t.Run` for subtest names.

### The Go Code

**File**: `internal/response/http_writer_test.go` (lines 21-191)

```go
func TestHTTPWriter_WriteResponse(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    w := NewHTTPWriter(logger, true)

    // Define test cases as an anonymous struct slice
    tests := []struct {
        name       string
        def        *spec.StubDefinition
        corsOpts   *CORSOptions
        wantStatus int
        wantBody   string
        wantHeader map[string]string
    }{
        {
            name: "basic response with body",
            def: &spec.StubDefinition{
                Response: spec.ResponseDefinition{
                    Status: 200,
                    Body:   `{"hello":"world"}`,
                },
            },
            wantStatus: 200,
            wantBody:   `{"hello":"world"}`,
        },
        {
            name: "empty body",
            def: &spec.StubDefinition{
                Response: spec.ResponseDefinition{Status: 204},
            },
            wantStatus: 204,
            wantBody:   "",
        },
        // 9 test cases total
    }

    // Run all test cases
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            rr := httptest.NewRecorder()
            req := httptest.NewRequest(http.MethodGet, "/test", nil)
            err := w.WriteResponse(rr, tt.def, req, tt.corsOpts)
            if err != nil {
                t.Fatalf("WriteResponse failed: %v", err)
            }
            if rr.Code != tt.wantStatus {
                t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatus)
            }
            if rr.Body.String() != tt.wantBody {
                t.Errorf("body = %q, want %q", rr.Body.String(), tt.wantBody)
            }
        })
    }
}
```

**File**: `internal/response/fault_test.go` (lines 8-38)

```go
func TestValidateFaultType(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {name: "empty string is valid", input: "", wantErr: false},
        {name: "error is valid", input: "error", wantErr: false},
        {name: "INVALID is rejected", input: "INVALID", wantErr: true},
        {name: "whitespace is rejected", input: " ", wantErr: true},
        {name: "null byte is rejected", input: "error\x00", wantErr: true},
        // 15 test cases total
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateFaultType(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateFaultType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
            }
        })
    }
}
```

### What Java Would Look Like

```java
// Java JUnit 5 with @ParameterizedTest
@ParameterizedTest
@CsvSource({
    "GET, /api/users, 200",
    "POST, /api/users, 201",
    "DELETE, /api/users/1, 204"
})
void testWriteResponse(String method, String path, int expectedStatus) {
    // ...
}
```

### Why Go Is Different

1. **No assertions library.** Go's standard `testing` package provides only `t.Error()`, `t.Fatalf()`, `t.Log()`. There is no `assertEquals`, `assertTrue`, or `assertThrows`. You write explicit conditionals:

```go
if got != want {
    t.Errorf("got %q, want %q", got, want)
}
```

2. **Table-driven tests are idiomatic.** An anonymous struct defines all test cases. A single `for` loop runs them all. This pattern appears in virtually every Go project.

3. **`t.Run` creates named subtests.** You can run a specific subtest by name:

```bash
go test -run TestValidateFaultType/INVALID_is_rejected
```

This is Go's equivalent of JUnit's `@DisplayName` and selective test execution.

4. **`httptest` is in the standard library.** Go provides `httptest.NewRecorder()` (captures HTTP responses) and `httptest.NewRequest()` (creates HTTP requests). No Mockito or PowerMock needed:

```go
rr := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
handler.ServeHTTP(rr, req)
// Assert on rr.Code, rr.Header(), rr.Body
```

5. **Test files are next to the source.** `http_writer_test.go` is in the same directory as `http_writer.go`. Both are in the same package (`response`). This means tests can access unexported identifiers directly — no reflection needed.

6. **Fatal vs Error.** `t.Fatalf()` stops the current test immediately (like JUnit's `fail()`). `t.Errorf()` reports a failure but continues the test (like JUnit's soft assertions). This allows one test case to report multiple failures.

---

## 12. io.Reader / io.Writer

Go's I/O model is built on two minimal interfaces: `Reader` (one method) and `Writer` (one method). This simplicity enables immense composability.

### The Go Code

**File**: `internal/response/http_writer.go` (lines 96-101)

```go
// io.Copy reads from a Reader (strings.NewReader) and writes to a Writer (http.ResponseWriter)
_, err := io.Copy(rw, strings.NewReader(body))
```

**File**: `internal/response/http_writer.go` (lines 274-293)

```go
type GzipResponseWriter struct {
    http.ResponseWriter    // satisfies io.Writer (has Write method)
    GW *gzip.Writer        // also satisfies io.Writer
}

func (w *GzipResponseWriter) Write(b []byte) (int, error) {
    return w.GW.Write(b)  // compress through gzip
}
```

**File**: `internal/log/log.go` (lines 62-63)

```go
body, err := io.ReadAll(req.Body)  // req.Body is io.ReadCloser (Reader + Closer)
```

### What Java Would Look Like

```java
try (InputStream in = new ByteArrayInputStream(body.getBytes());
     OutputStream out = response.getOutputStream()) {
    in.transferTo(out);  // Java 9+
}
```

### Why Go Is Different

1. **One-method interfaces.** `io.Writer` is `Write(p []byte) (n int, err error)`. `io.Reader` is `Read(p []byte) (n int, err error)`. That's it. Java's `java.io.Writer` has 17 methods; Go's has 1.

2. **Maximum composability.** Because the interfaces are so small, anything that can read or write bytes composes with everything else:
   - `strings.NewReader(s)` → `io.Reader` from a string
   - `bytes.NewReader(b)` → `io.Reader` from a byte slice
   - `gzip.NewWriter(w)` → wraps an `io.Writer` with compression
   - `io.MultiWriter(w1, w2)` → writes to multiple writers simultaneously
   - `io.TeeReader(r, w)` → reads from r, writes everything to w (Unix `tee`)

3. **`http.ResponseWriter` satisfies `io.Writer`.** Because it has a `Write([]byte) (int, error)` method, it can be used anywhere an `io.Writer` is expected. This is how `io.Copy(rw, reader)` writes to an HTTP response — no adapter needed.

4. **`io.Copy` replaces Java's loop.** Instead of `while ((n = in.read(buf)) != -1) { out.write(buf, 0, n); }`, Go has `io.Copy(w, r)`.

5. **`io.ReadAll` replaces `ByteArrayOutputStream` pattern.** Instead of `ByteArrayOutputStream baos = new ByteArrayOutputStream(); in.transferTo(baos); baos.toByteArray()`, Go has `io.ReadAll(r)`.

---

## 13. HTTP Without a Framework

Go's standard library has everything you need for HTTP servers. No Spring Boot, no annotations, no reflection.

### The Go Code

**File**: `internal/admin/handler.go` (lines 32-111)

```go
type Handler struct {
    registry   *stub.Registry
    requestLog *log.RequestLog
    resetFns   []func()
}

// ServeHTTP implements http.Handler — the only method needed.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Path

    switch {
    case path == Prefix+"mappings" || path == Prefix+"mappings/":
        switch r.Method {
        case http.MethodGet:
            h.listMappings(w, r)
        case http.MethodPost:
            h.createMapping(w, r)
        case http.MethodDelete:
            h.deleteAllMappings(w, r)
        default:
            methodNotAllowed(w)
        }

    case strings.HasPrefix(path, Prefix+"mappings/"):
        id := strings.TrimPrefix(path, Prefix+"mappings/")
        switch r.Method {
        case http.MethodGet:
            h.getMapping(w, r, id)
        case http.MethodDelete:
            h.deleteMapping(w, r, id)
        }

    case path == Prefix+"reset":
        h.reset(w, r)

    case path == Prefix+"health":
        h.health(w, r)

    default:
        writeJSON(w, http.StatusNotFound, map[string]string{
            "error": "admin endpoint not found: " + path,
        })
    }
}
```

**File**: `internal/response/http_writer.go` (lines 150-200)

```go
case "connection_reset":
    inner := unwrapResponseWriter(rw)
    if hj, ok := inner.(http.Hijacker); ok {
        conn, _, err := hj.Hijack()
        if err != nil {
            // fallback to 500
        }
        _ = conn.Close()
        return true
    }
```

### What Java Would Look Like

```java
@RestController
@RequestMapping("/__admin")
public class AdminController {

    @GetMapping("/mappings")
    public List<Mapping> listMappings() { ... }

    @PostMapping("/mappings")
    public ResponseEntity<Mapping> createMapping(@RequestBody Mapping body) { ... }

    @DeleteMapping("/mappings/{id}")
    public ResponseEntity<Void> deleteMapping(@PathVariable String id) { ... }
}
```

### Why Go Is Different

1. **`http.Handler` is a single-method interface.** Any type with `ServeHTTP(http.ResponseWriter, *http.Request)` is an HTTP handler. No annotations, no DI, no reflection.

```go
type Handler interface {
    ServeHTTP(http.ResponseWriter, *http.Request)
}
```

2. **Manual routing is explicit.** The admin handler uses a `switch` statement on path and method. No framework hides the routing logic. Go 1.22+ added method-pattern routing to `http.ServeMux` (`GET /path/{id}`), but explicit routing is still common.

3. **`http.Hijacker` is an optional interface.** The type assertion `hj, ok := inner.(http.Hijacker)` checks at runtime whether the `ResponseWriter` also supports TCP-level hijacking. This is Go's idiom for optional capabilities — no separate interface hierarchy or marker interface.

4. **Middleware via wrapping.** Go composes HTTP middleware by wrapping `http.Handler`:

```go
func LoggerMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println(r.Method, r.URL.Path)
        next.ServeHTTP(w, r)
    })
}
```

This is Go's equivalent of Spring's `@Component` + `WebFilter` — but without annotations or DI.

5. **No framework dependency.** The entire gochaos server uses only the standard `net/http` package. No Spring Boot, no Tomcat, no Undertow.

---

## 14. Packages and the Module System

### The Go Code

**File**: `go.mod` (root directory)

```
module github.com/sunny809/gochaos

go 1.25.0

require (
    github.com/PaesslerAG/jsonpath v0.1.1
    github.com/spf13/cobra v1.10.2
    gopkg.in/yaml.v3 v3.0.1
)
```

**File**: `internal/stub/registry.go` (lines 1-12)

```go
package stub  // package declaration — short, one word

import (
    "crypto/rand"
    "fmt"
    "sync"

    "github.com/sunny809/gochaos/internal/response"  // intra-module import
    "github.com/sunny809/gochaos/internal/spec"
)
```

### What Java Would Look Like

```xml
<!-- Maven pom.xml -->
<groupId>com.github.sunny809</groupId>
<artifactId>gochaos</artifactId>
<version>0.1.0</version>

<dependencies>
    <dependency>
        <groupId>com.github.PaesslerAG</groupId>
        <artifactId>jsonpath</artifactId>
        <version>0.1.1</version>
    </dependency>
</dependencies>
```

### Why Go Is Different

1. **Flat package tree.** Go packages are directories with short names: `stub`, `spec`, `response`, `admin`, `log`. Java's convention is deep nesting: `com.example.gmock.internal.stub`. Go avoids this — package names are part of the import path but short in usage.

2. **`go.mod` is one file.** All dependencies are in a single `go.mod` file. No XML, no Groovy DSL. `go get` adds entries automatically.

3. **`go.sum` for verification.** `go.sum` contains cryptographic hashes of all dependencies. The `go mod verify` command checks that downloaded modules match — Go's version of Maven's checksum verification.

4. **`internal/` is enforced by the compiler.** Packages under `internal/` can only be imported by code rooted at the parent. This is Go's mechanism for public-vs-private at the package level — no `module-info.java` needed.

5. **Import paths are URLs.** `"github.com/sunny809/gochaos/internal/spec"` is both the import path and the VCS location. Go resolves imports directly from version control — no artifact repository like Maven Central required.

6. **No version ranges.** `go.mod` records exact versions. No `[1.0, 2.0)` ranges. This avoids the "works on my machine" problem of dependency resolution.

---

## 15. Struct Tags and Serialization

### The Go Code

**File**: `internal/spec/spec.go` (lines 15-44)

```go
type RequestPattern struct {
    Method       string            `json:"method,omitempty" yaml:"method,omitempty"`
    URLPath      string            `json:"urlPath,omitempty" yaml:"urlPath,omitempty"`
    URLPathRegex string            `json:"urlPathRegex,omitempty" yaml:"urlPathRegex,omitempty"`
    Accept       string            `json:"accept,omitempty" yaml:"accept,omitempty"`
    QueryParams  map[string]string `json:"queryParams,omitempty" yaml:"queryParams,omitempty"`
    Headers      map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
    Cookies      map[string]string `json:"cookies,omitempty" yaml:"cookies,omitempty"`
    Body         *BodyPattern      `json:"body,omitempty" yaml:"body,omitempty"`
    Priority     int               `json:"priority,omitempty" yaml:"priority,omitempty"`
}
```

**File**: `pkg/gmock/server.go` (lines 269-276)

```go
// StubJSON registers a stub from JSON bytes.
func (s *mockServer) StubJSON(data []byte) (string, error) {
    var def StubDefinition
    if err := json.Unmarshal(data, &def); err != nil {
        return "", fmt.Errorf("gmock: invalid stub JSON: %w", err)
    }
    return s.registry.Add(def)
}
```

### What Java Would Look Like

```java
// Jackson annotations
public class RequestPattern {
    @JsonProperty("method")
    private String method;

    @JsonProperty("urlPath")
    private String urlPath;

    // getters and setters...
}
```

### Why Go Is Different

1. **Struct tags are raw strings.** `json:"method,omitempty"` is a string literal. The `encoding/json` and `gopkg.in/yaml.v3` libraries parse these tags at runtime using reflection. Java annotations are compiled metadata — a fundamentally different mechanism.

2. **Dual tags for JSON + YAML.** The same struct has both `json:` and `yaml:` tags. Go's struct tags are just strings — any library can read them. Java would require separate annotations for Jackson (`@JsonProperty`) and SnakeYAML.

3. **`omitempty`** means "omit this field if it's zero-valued". `method: ""` is omitted from JSON output. This is Go's equivalent of `@JsonInclude(Include.NON_NULL)`.

4. **JSON and YAML are built-in or minimal-dependency.** `encoding/json` is in the standard library. `gopkg.in/yaml.v3` is a single third-party package. No Jackson, no Gson, no SnakeYAML.

5. **Unmarshal into a struct directly:**

```go
var def StubDefinition
json.Unmarshal(data, &def)
```

No `ObjectMapper`, no `TypeReference`, no configuration object. Just a bytes-to-struct operation.

---

## 16. Type Assertions and Optional Interfaces

> Go doesn't have Java's `instanceof` keyword. Instead, you use the **comma-ok form** to probe whether a value satisfies an interface at runtime — a pattern called "capability probing" that is central to Go's philosophy of additive interfaces.

### The Go Code

gochaos uses type assertions to check whether a `ResponseWriter` supports additional capabilities. The `http.Flusher` interface has one method: `Flush()`. Not all writers implement it:

```go
// internal/response/http_writer.go:164
if flusher, ok := rw.(http.Flusher); ok {
    flusher.Flush()
}
```

The connection reset fault goes further — it checks for `http.Hijacker` to close the raw TCP connection:

```go
// internal/response/http_writer.go:172
if hijacker, ok := inner.(http.Hijacker); ok {
    conn, _, _ := hijacker.Hijack()
    conn.Close() // sends TCP RST
}
```

The near-miss engine uses the same pattern to detect whether a matcher supports structured diagnostics:

```go
// internal/nearmiss/engine.go:122
if dm, ok := m.(matcher.DiagnosticMatcher); ok {
    diag := dm.Diagnose(req)
    // ... use diag.Dimension, diag.Matched, diag.Reason ...
}
```

The `DiagnosticMatcher` interface itself is defined as an **interface embedding** — it combines the existing `Matcher` with a new `Diagnose` method:

```go
// internal/matcher/diagnostic.go:48
type DiagnosticMatcher interface {
    Matcher
    Diagnose(req *http.Request) Diagnosis
}
```

And the compile-time check that a concrete type satisfies an interface:

```go
// internal/nearmiss/engine_fallback_test.go:35
var _ matcher.Matcher = stubMatcher{}
```

### What Java Would Look Like

```java
// Java uses instanceof + cast
if (responseWriter instanceof Flushable) {
    ((Flushable) responseWriter).flush();
}

// For the near-miss engine — a long instanceof chain
if (matcher instanceof DiagnosticMatcher) {
    Diagnosis diag = ((DiagnosticMatcher) matcher).diagnose(request);
    // ...
}

// In Java, DiagnosticMatcher would need Matcher in its extends clause
// or be a separate interface that matchers explicitly implement
```

### Why Go Is Different

1. **No `instanceof` — use comma-ok.** Go's `x, ok := i.(T)` returns `(zero, false)` when the assertion fails instead of throwing `ClassCastException`. This makes probing safe without try/catch.

2. **Optional interfaces are additive.** `http.ResponseWriter` is a minimal 3-method interface. Capabilities like `http.Flusher`, `http.Hijacker`, `http.Pusher` are separate interfaces you probe for. This keeps the base interface small and lets you add behavior without breaking existing implementors.

3. **Interface embedding is not interface inheritance.** `DiagnosticMatcher` embedding `Matcher` means "a DiagnosticMatcher is also a Matcher". But no class "implements" `DiagnosticMatcher` — it's satisfied implicitly if the methods exist (see Section 3).

4. **The `var _ Interface = (*T)(nil)` idiom** asserts at compile time (zero runtime cost) that a type satisfies an interface. Java's `implements` does this at the type declaration; Go's implicit satisfaction means you need this idiom when you want the compiler to check your work.

5. **This pattern appears everywhere in Go stdlib.** Go uses optional interfaces for compression (`io.ByteReader`, `io.ByteWriter`), network capabilities (`net.Error.Temporary()`), and HTTP middleware unwrapping (`ResponseWriter.Unwrap()` in Go 1.20+). Once you learn it, you see it everywhere.

---

## 17. Type Switches

> A **type switch** is Go's compact replacement for Java's `instanceof` chain. It dispatches on the concrete type of an interface value using `switch v := x.(type) { ... }`.

### The Go Code

gochaos's JSONPath body matcher unmarshals the request body into an `interface{}` and then switches on the actual type:

```go
// internal/matcher/body.go:175
switch v := result.(type) {
case map[string]interface{}:
    // JSON object — walk keys
    // ...
case []interface{}:
    // JSON array — walk elements
    // ...
default:
    return false, 0
}
```

The same pattern appears again for a different JSONPath result:

```go
// internal/matcher/body.go:232
switch v := result.(type) {
case []interface{}:
    // array of matched nodes
    return len(v) > 0, len(v) * scorePerMatch
case bool:
    if v {
        return true, maxScore
    }
}
```

### What Java Would Look Like

```java
// Java uses a chain of instanceof checks
Object result = jsonPath.read(body);
if (result instanceof Map) {
    Map<String, Object> map = (Map<String, Object>) result;
    // walk keys...
} else if (result instanceof List) {
    List<?> list = (List<?>) result;
    // walk elements...
} else {
    return false;
}
```

### Why Go Is Different

1. **`switch v := x.(type)` is syntactic sugar over type assertions.** The `v` variable is automatically typed to the matched case's type within each branch. No explicit cast needed.

2. **The `default` case is optional but recommended.** If you don't handle a type and there's no `default`, the switch does nothing — no error, no exception. This is both a convenience and a footgun.

3. **Type switches only work on `interface{}` values.** You can't type-switch on a concrete type. This matches Go's philosophy: type switches exist to handle dynamic dispatch, not to replace method dispatch.

4. **The order of cases doesn't matter for correctness.** Unlike `instanceof` chains where order changes behavior (if a subclass check comes after superclass), Go's type switch matches exactly one case — the first matching one — but there are no subtype relationships in Go's type system.

5. **JSON parsing is the most common use case.** `json.Unmarshal` into `interface{}` produces `map[string]interface{}` for objects, `[]interface{}` for arrays, `string`/`float64`/`bool`/`nil` for primitives. Type switches are the idiomatic way to handle this.

---

## 18. Custom Error Types and Error Wrapping

> Section 4 taught you "errors are values." This section shows how to define **custom error types** and use `%w` wrapping + `errors.As` to build error chains — Go's equivalent of Java's exception-cause chains.

### The Go Code

gochaos defines a `ValidationError` for stub validation failures. It carries structured data alongside the error message:

```go
// internal/stub/registry.go:16
type ValidationError struct {
    Field   string
    Message string
    Err     error // optional wrapped error
}

func (e *ValidationError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("validation: %s: %s (%s)", e.Field, e.Message, e.Err)
    }
    return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}
```

The admin API handler unwraps it with `errors.As`:

```go
// internal/admin/mappings.go:24
var valErr *stub.ValidationError
if errors.As(err, &valErr) {
    http.Error(w, valErr.Message, http.StatusBadRequest)
    return
}
```

Error wrapping with `%w` is used throughout:

```go
// config/config.go:35
return nil, fmt.Errorf("config: read %s: %w", path, err)
```

### What Java Would Look Like

```java
// Custom exception with structured data
public class ValidationError extends RuntimeException {
    private final String field;
    private final String message;

    public ValidationError(String field, String message, Throwable cause) {
        super(field + ": " + message, cause);
        this.field = field;
        this.message = message;
    }
}

// Catching and inspecting
try {
    registry.add(stub);
} catch (ValidationError e) {
    if (e.getField().equals("fault")) {
        response.sendError(400, e.getMessage());
    }
}
```

### Why Go Is Different

1. **`Error()` is just an interface method.** Go's `error` interface has one method: `Error() string`. Any type that implements it is an error. No `extends RuntimeException`, no checked vs unchecked distinction.

2. **`%w` wrapping creates a chain, not a cause tree.** `fmt.Errorf("context: %w", err)` wraps `err` so that `errors.As` and `errors.Is` can traverse the chain. Unlike Java's `new Exception("msg", cause)` where the cause is a single node, Go's chain is linear — each error wraps exactly one other error.

3. **`errors.As` is like Java's `catch (Type e)`.** It walks the error chain looking for a matching type. The second argument must be a pointer to the target type (`&valErr`), and `errors.As` sets it to the matched error.

4. **`errors.Is` is for sentinel comparison** (though gochaos doesn't use sentinel errors). It checks whether any error in the chain matches a target value: `errors.Is(err, io.EOF)`.

5. **Custom error types are structs, not classes.** They carry data fields just like Java exceptions. They don't need serialVersionUID, stack traces are generated by `runtime.Callers` when needed, and the `%+v` format with `github.com/pkg/errors` (not used here) can include them.

6. **Zero-cost when not taken.** Creating an `error` value doesn't capture a stack trace unless you explicitly ask for it. Java's `new Exception()` captures the stack trace eagerly. This makes Go error creation orders of magnitude cheaper in the success path.

---

## 19. sort.Slice / sort.SliceStable

> Go's sorting is closure-based: pass a slice and a less-function to `sort.Slice`. No `Comparator` interface, no `Collections.sort` utility class. And `sort.SliceStable` preserves the original order of equal elements — critical for deterministic tie-breaking.

### The Go Code

The near-miss engine sorts results by score descending, using `sort.SliceStable` so that equal-scoring stubs keep their registration order:

```go
// internal/nearmiss/engine.go:82
sort.SliceStable(results, func(i, j int) bool {
    return results[i].Score > results[j].Score
})
```

A simpler example sorts valid fault types alphabetically:

```go
// internal/response/fault.go:47
sort.Strings(valid)
```

The race test also uses `sort.SliceStable` to normalize dimension order before deep-equal comparison (because `Headers` map iteration is non-deterministic):

```go
// internal/nearmiss/engine_race_test.go:26
sort.SliceStable(bd, func(a, b int) bool {
    return bd[a].Dimension < bd[b].Dimension
})
```

### What Java Would Look Like

```java
// Java 8+
results.sort((a, b) -> Integer.compare(b.getScore(), a.getScore()));

// Java's List.sort is stable by default — no separate method needed
// For the near-miss engine, Java's sort is always stable

// For the fault types:
Collections.sort(validFaultTypes);
```

### Why Go Is Different

1. **`sort.Slice` is a function, not a method.** You pass the slice as the first argument. The less-function takes indices, not values: `func(i, j int) bool { return slice[i] < slice[j] }`.

2. **Stability is opt-in.** `sort.Slice` is NOT stable — equal elements may be reordered. `sort.SliceStable` preserves original order. In Java, `List.sort` is stable by default. This is a common gotcha for Java developers moving to Go.

3. **The less-function must be a strict weak ordering.** `a < b` means "a should sort before b." Return `true` for ascending, `false` for descending. This is the same contract as Java's `Comparator.compare(a, b) < 0`.

4. **`sort.Strings` and `sort.Ints` exist for simple cases.** When sorting a `[]string` or `[]int` in natural order, you don't need a less-function at all.

5. **For complex types, you can implement `sort.Interface`** — `Len()`, `Less()`, `Swap()`. This is the Go equivalent of implementing `Comparable`. But `sort.Slice` is preferred for most cases because it's shorter.

6. **The closure captures variables from the outer scope.** This is how you sort by a computed value without storing it: `sort.Slice(items, func(i, j int) bool { return expensiveComputation(items[i]) < expensiveComputation(items[j]) })`. (But beware: this calls the function O(n log n) times.)

---

## 20. Benchmarks and Race Tests

> Go's `testing` package includes built-in benchmarking (`testing.B`) and the race detector is a first-class tool (`go test -race`). Together, they let you measure performance and detect data races without third-party tools.

### The Go Code: Benchmarks

The near-miss engine has a benchmark that measures `Compute` on 100 stubs:

```go
// internal/nearmiss/engine_bench_test.go
func BenchmarkCompute_100Stubs(b *testing.B) {
    stubs := buildRealisticStubs(100) // setup, outside b.N loop
    req := httptest.NewRequest("GET", "/api/resource", nil)
    engine := nearmiss.NewEngine()

    b.ReportAllocs()   // report allocation count
    b.ResetTimer()     // exclude setup from timing

    for i := 0; i < b.N; i++ {
        engine.Compute(req, stubs)
    }
}
```

Run with:
```bash
go test -bench=. -benchmem ./internal/nearmiss/...
```

Output:
```
BenchmarkCompute_100Stubs-4    1000    1200000 ns/op    570000 B/op    6753 allocs/op
```

### The Go Code: Race Tests

The same package has a dedicated race test — a concurrent test designed to be run with the race detector:

```go
// internal/nearmiss/engine_race_test.go
func TestEngine_ConcurrentCompute(t *testing.T) {
    engine := nearmiss.NewEngine()
    stubs := buildRealisticStubs(50)
    goroutines := 32
    iterations := 50

    var wg sync.WaitGroup
    for g := 0; g < goroutines; g++ {
        g := g // rebind for closure (see sidebar)
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := 0; i < iterations; i++ {
                req := buildVaryingRequest(g, i)
                results := engine.Compute(req, stubs)
                // ... assert results ...
            }
        }()
    }
    wg.Wait()
}
```

### What Java Would Look Like

```java
// JMH benchmark
@Benchmark
@BenchmarkMode(Mode.AverageTime)
@OutputTimeUnit(TimeUnit.NANOSECONDS)
public void compute() {
    engine.compute(request, stubs);
}

// No built-in race detector equivalent
// IntelliJ has a thread-sanitizer plugin, but it's not in the toolchain
```

### Why Go Is Different

1. **Benchmarks are built into `testing.B`.** No JMH, no Caliper, no separate dependency. The `b.N` loop is determined by the framework: it starts small and increases until the benchmark runs for about 1 second.

2. **`b.ReportAllocs()` is a one-liner.** You get allocation count and bytes allocated per operation for free. In Java, measuring allocations requires JFR or a profiler.

3. **`b.ResetTimer()` separates setup from measurement.** Call it after expensive setup (building stubs, reading files) to exclude it from `b.N` timing.

4. **The `-bench` flag is powerful.** `-bench=.` runs all benchmarks. `-bench=BenchmarkCompute` matches by name. `-benchmem` adds allocation reporting. `-count=N` runs N times for statistical significance.

5. **The race detector is a flag, not a tool.** `go test -race` recompiles with instrumentation. No JVM flags, no agent, no plugin. It detects all defined races in your test run.

6. **Race tests are just normal tests that exercise concurrency.** You write goroutines doing concurrent operations, then assert deterministic results. The race detector proves no unsynchronized access occurred. It is NOT a proof of correctness — a test with `-race` clean can still have logic bugs.

7. **The `g := g` rebind is critical.** Inside the goroutine closure, the loop variable `g` is captured by reference. By the time the goroutine runs, `g` may have advanced. Rebinding creates a per-iteration copy. Go 1.22+ fixes this for `for` loops, but closures in `go func()` still need the rebind pattern.

---

## 21. `net/http/httptest` Package

> Go's `httptest` package provides three utilities for testing HTTP code without spinning up a full server framework: `NewRecorder` (captures responses), `NewRequest` (builds requests), and `NewServer` (runs a real HTTP server on a random port).

### The Go Code: NewRecorder + NewRequest

Most unit tests use `httptest.ResponseRecorder` to capture handler output:

```go
// internal/response/http_writer_test.go:168-169
w := httptest.NewRecorder()
req := httptest.NewRequest(http.MethodGet, "/test", nil)

// Now pass w and req to any http.Handler
handler.ServeHTTP(w, req)

// Assert the captured response
assert.Equal(t, 200, w.Code)
assert.Contains(t, w.Body.String(), "expected content")
```

The near-miss race test uses `httptest.NewRequest` to build realistic requests:

```go
// internal/nearmiss/engine_race_test.go:93
req := httptest.NewRequest("POST", "/api/reservations", bodyReader)
req.Header.Set("X-Tenant-Id", fmt.Sprintf("tenant-%d", g%5))
```

### What Java Would Look Like

```java
// Spring MockMvc
mockMvc.perform(get("/test"))
    .andExpect(status().isOk())
    .andExpect(content().string(containsString("expected")));

// Or with MockHttpServletResponse directly
MockHttpServletResponse response = new MockHttpServletResponse();
MockHttpServletRequest request = new MockHttpServletRequest("GET", "/test");
handler.handleRequest(request, response);
assertEquals(200, response.getStatus());
```

### Why Go Is Different

1. **`httptest.ResponseRecorder` implements `http.ResponseWriter`.** You can pass it directly to any `http.Handler` — no adapter needed. It records the status code (`w.Code`), headers (`w.Header()`), and body (`w.Body`).

2. **`httptest.NewRequest` creates a real `*http.Request`.** Not a mock, not a stub — a fully functional request with a context, ready body, and URL. This is Go's equivalent of Spring's `MockHttpServletRequest` but simpler.

3. **`httptest.NewServer` starts a real HTTP server on a random port.** This is used in integration tests (gochaos's `test/integration/`). The server returns its URL so you can point real HTTP clients at it:
    ```go
    ts := httptest.NewServer(handler)
    defer ts.Close()
    resp, _ := http.Get(ts.URL + "/api/health")
    ```

4. **No assertion framework needed.** `httptest.ResponseRecorder` exposes raw `Code`, `Header()`, and `Body`. You assert with plain `if` or `require.Equal` — no `andExpect().status().isOk()` chain.

5. **`httptest.NewRequest` is NOT a mock.** It doesn't need Mockito setup, `when()` calls, or `verify()` assertions. It builds a real request with real headers, real body, and a real context. This is consistent with Go's philosophy: test with real objects, not mocks.

---

## 22. Summary Reference Table

| Concept | Go | Java | Key Difference |
|---------|----|------|----------------|
| **Type definition** | `type X struct { ... }` | `class X { ... }` | No `class` keyword. Methods defined outside struct body. |
| **Constructor** | `func NewX() *X` | `new X()` or `X x = new X()` | Factory functions, not constructors. Zero values by default. |
| **Interface** | Implicit satisfaction | `implements` keyword | Structural typing. Consumer defines the contract. |
| **Error handling** | `result, err := fn()` | `try { ... } catch (E e) { ... }` | Errors are return values, not control flow. |
| **Error wrapping** | `fmt.Errorf("ctx: %w", err)` | `new RuntimeException("ctx", cause)` | `%w` creates wrappable errors. `errors.As`/`errors.Is` for unwrapping. |
| **Custom errors** | `type E struct { Msg string; Err error }` | `class E extends RuntimeException` | Any type with `Error() string` is an error. `errors.As` for type-directed unwrapping. |
| **Inheritance** | Embedding (`type X struct { Y }`) | `extends` | Method promotion, not inheritance. No `super`. |
| **Visibility** | Capital = exported | `public`, `private`, `protected` | One rule replaces four keywords. |
| **Module privacy** | `internal/` directory | `module-info.java` | Compiler-enforced. No import outside module tree. |
| **Concurrency** | `go fn()`, `sync.Mutex`, channels | `Thread`, `ExecutorService`, `synchronized` | Goroutines are lightweight (4KB stack). `defer` for unlock. |
| **Lock pattern** | `mu.Lock()` / `defer mu.Unlock()` | `lock.lock()` / `try { ... } finally { lock.unlock() }` | Lock and unlock on adjacent lines. |
| **Atomic counters** | `atomic.AddUint64(&x, 1)` | `AtomicLong.incrementAndGet()` | Operates on memory address, not wrapper object. Zero allocation. |
| **Multiple mutexes** | `mu` + `rateLimitMu` in same struct | `ReadWriteLock` + `ReentrantLock` | Document lock ownership in comments. |
| **Cancellation** | `context.Context` + `select` | `Future.cancel()` + `Thread.interrupt()` | Cooperative via channels, not forced interruption. |
| **Wait on multiple** | `select { case <-ch1: case <-ch2: }` | `CompletableFuture.anyOf()` | Language keyword, not library. |
| **Time duration** | `time.Duration` (`int64` nanoseconds) | `java.time.Duration` (object) | Arithmetic with `+`, `-`, `*`, `/` operators. |
| **Time arithmetic** | `d1 + d2`, `5 * time.Second` | `d1.plus(d2)`, `Duration.ofSeconds(5)` | Native operators vs method chaining. |
| **Resource cleanup** | `defer resource.Close()` | `try (Resource r = ...) { ... }` | LIFO order. Runs on panic. |
| **Configuration** | `func(*Config)` variadic options | Builder pattern | Functions over objects. No `.build()` call. |
| **Testing** | Table-driven + `t.Run` | JUnit `@Test` + `@ParameterizedTest` | Anonymous structs as test cases. No assertion library. |
| **Benchmarks** | `testing.B` + `b.N` loop + `b.ReportAllocs()` | JMH | Built-in. `-benchmem` for allocation reporting. |
| **Race detection** | `go test -race` | No built-in equivalent | Instrumented runtime detects data races. |
| **Sorting** | `sort.Slice(s, func(i,j int) bool)` | `list.sort((a,b) -> ...)` | Stable sort is opt-in (`sort.SliceStable`). |
| **HTTP server** | `http.Handler` interface | Spring `@RestController` | Single-method interface. No annotations. |
| **HTTP testing** | `httptest.NewRecorder()` + `NewRequest()` | MockMvc / `MockHttpServletResponse` | Real `http.ResponseWriter` + `*http.Request`. No mocks. |
| **I/O** | `io.Reader` / `io.Writer` (1 method) | `java.io.InputStream` / `OutputStream` (many methods) | Minimal interfaces = maximum composability. |
| **Serialization** | Struct tags: `json:"name"` | Jackson `@JsonProperty` | Tags are raw strings. Libraries parse them with reflection. |
| **Type assertion** | `x, ok := i.(Type)` | `instanceof` + cast | Comma-ok form prevents panics. No ClassCastException. |
| **Type switch** | `switch v := x.(type) { ... }` | `instanceof` chain | Variable is auto-typed per case. No explicit cast. |
| **Build tool** | `go build`, `go test` | Maven, Gradle | Built-in. No XML. No lifecycle phases. |
| **Dependencies** | `go.mod` | `pom.xml` / `build.gradle` | Exact versions only. No ranges. VCS-based resolution. |
| **Generics** | Built-in (map, slice, chan) | Full generic types | Go 1.18+ has generics, but interfaces are preferred. |
| **`nil`** | Valid for pointers, slices, maps, interfaces | `null` for objects | `nil` slice is valid (`len()` returns 0). `nil` map panics on write. |
| **`min`/`max` built-ins** | `min(a, b)`, `max(a, b)` (Go 1.21+) | `Math.min(a, b)`, `Math.max(a, b)` | Generic, work with any ordered type. No import needed. |

---

## Appendix: Key Go Commands for Java Developers

| Task | Go Command | Java Equivalent |
|------|-----------|-----------------|
| Build | `go build ./...` | `mvn compile` |
| Run tests | `go test ./...` | `mvn test` |
| Run tests with race detection | `go test -race ./...` | `mvn test` (no built-in equivalent) |
| Run one test | `go test -run TestName` | `mvn test -Dtest=TestName` |
| Run one subtest | `go test -run TestName/sub_test` | `mvn test -Dtest=TestName#method` |
| Get a dependency | `go get example.com/pkg@v1.2.3` | Add to `pom.xml` |
| Tidy dependencies | `go mod tidy` | `mvn dependency:resolve` |
| Static analysis | `go vet ./...` | `mvn checkstyle:check` |
| Format code | `gofmt -w .` | IDE auto-format |
| Install binary | `go install ./cmd/gmock` | `mvn package` + manual copy |
| View documentation | `go doc package.Function` | Javadoc |
| Update all dependencies | `go get -u ./...` | Maven Versions plugin |

---

*This guide was written using the [gochaos](https://github.com/sunny809/gochaos) codebase as a teaching vehicle. All code examples reference real file paths from the project.*

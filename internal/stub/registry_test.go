package stub_test

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sunny809/gochaos/internal/stub"
	"github.com/sunny809/gochaos/pkg/gmock"
)

func TestRegistryAddAndGet(t *testing.T) {
	r := stub.NewRegistry()

	def := gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "ok"},
	}

	id, err := r.Add(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}

	retrieved := r.Get(id)
	if retrieved == nil {
		t.Fatal("expected stub to be retrievable")
	}
	if retrieved.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", retrieved.Request.Method)
	}
}

func TestRegistryAddPreservesID(t *testing.T) {
	r := stub.NewRegistry()

	def := gmock.StubDefinition{
		ID:       "custom-id",
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		Response: gmock.ResponseDefinition{Status: 200},
	}

	id, _ := r.Add(def)
	if id != "custom-id" {
		t.Errorf("expected custom-id, got %s", id)
	}
}

func TestRegistryDelete(t *testing.T) {
	r := stub.NewRegistry()

	id, _ := r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
	})

	if !r.Delete(id) {
		t.Error("expected Delete to return true")
	}
	if r.Get(id) != nil {
		t.Error("expected stub to be deleted")
	}
	if r.Delete(id) {
		t.Error("expected second Delete to return false")
	}
}

func TestRegistryDeleteAll(t *testing.T) {
	r := stub.NewRegistry()

	for i := 0; i < 5; i++ {
		r.Add(gmock.StubDefinition{
			Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
		})
	}

	if r.Len() != 5 {
		t.Errorf("expected 5 stubs, got %d", r.Len())
	}

	r.DeleteAll()
	if r.Len() != 0 {
		t.Errorf("expected 0 stubs after DeleteAll, got %d", r.Len())
	}
}

func TestRegistryList(t *testing.T) {
	r := stub.NewRegistry()

	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/a"},
	})
	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{Method: "GET", URLPath: "/b"},
	})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 stubs, got %d", len(list))
	}
}

func TestRegistryPriorityOrdering(t *testing.T) {
	r := stub.NewRegistry()

	// Add stubs in unsorted priority order
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/low"},
		Priority: 10,
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/high"},
		Priority: 1,
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{URLPath: "/mid"},
		Priority: 5,
	})

	list := r.List()
	if list[0].Request.URLPath != "/high" {
		t.Errorf("expected /high first, got %s", list[0].Request.URLPath)
	}
	if list[1].Request.URLPath != "/mid" {
		t.Errorf("expected /mid second, got %s", list[1].Request.URLPath)
	}
	if list[2].Request.URLPath != "/low" {
		t.Errorf("expected /low third, got %s", list[2].Request.URLPath)
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := stub.NewRegistry()
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Add(gmock.StubDefinition{
				Request: gmock.RequestPattern{Method: "GET", URLPath: "/test"},
			})
		}()
	}

	// Concurrent readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
			_ = r.Len()
		}()
	}

	wg.Wait()

	if r.Len() != 100 {
		t.Errorf("expected 100 stubs, got %d", r.Len())
	}
}

func TestEngineMatch(t *testing.T) {
	r := stub.NewRegistry()
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "users"},
	})
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "POST", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 201, Body: "created"},
	})

	engine := stub.NewEngine(r)

	// Match GET /users
	req := httptest.NewRequest("GET", "/users", nil)
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match for GET /users")
	}
	if result.Stub.Response.Body != "users" {
		t.Errorf("expected 'users' body, got %s", result.Stub.Response.Body)
	}

	// Match POST /users
	req = httptest.NewRequest("POST", "/users", nil)
	result = engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match for POST /users")
	}
	if result.Stub.Response.Status != 201 {
		t.Errorf("expected 201, got %d", result.Stub.Response.Status)
	}

	// No match for DELETE
	req = httptest.NewRequest("DELETE", "/users", nil)
	result = engine.Match(req)
	if result != nil {
		t.Errorf("expected no match for DELETE, got %+v", result)
	}
}

func TestEngineMatchSpecificity(t *testing.T) {
	r := stub.NewRegistry()

	// Less specific stub (matches GET to anything)
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "any"},
	})

	// More specific stub (matches GET /users)
	r.Add(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: "GET", URLPath: "/users"},
		Response: gmock.ResponseDefinition{Status: 200, Body: "specific"},
	})

	engine := stub.NewEngine(r)

	req := httptest.NewRequest("GET", "/users", nil)
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match")
	}
	if result.Stub.Response.Body != "specific" {
		t.Errorf("expected most specific stub to win, got %s", result.Stub.Response.Body)
	}
}

func TestEngineWithHeaders(t *testing.T) {
	r := stub.NewRegistry()

	r.Add(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  "GET",
			URLPath: "/api",
			Headers: map[string]string{"Authorization": "Bearer token"},
		},
		Response: gmock.ResponseDefinition{Status: 200, Body: "authorized"},
	})

	engine := stub.NewEngine(r)

	// With correct header
	req := httptest.NewRequest("GET", "/api", nil)
	req.Header.Set("Authorization", "Bearer token")
	result := engine.Match(req)
	if result == nil || !result.Matched {
		t.Fatal("expected match with correct header")
	}

	// Without header
	req = httptest.NewRequest("GET", "/api", nil)
	result = engine.Match(req)
	if result != nil {
		t.Error("expected no match without header")
	}
}

func TestEngineEmptyRegistry(t *testing.T) {
	r := stub.NewRegistry()
	engine := stub.NewEngine(r)

	req := httptest.NewRequest("GET", "/anything", nil)
	result := engine.Match(req)
	if result != nil {
		t.Errorf("expected no match on empty registry, got %+v", result)
	}
}
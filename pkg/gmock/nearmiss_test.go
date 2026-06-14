package gmock_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// newServerForNearMiss returns a started server (random port) and a cleanup func.
// We start the server because it's the documented public path for using gmock,
// even though NearMiss itself does not require Start. This mirrors verification
// tests in this package.
func newServerForNearMiss(t *testing.T) gmock.Server {
	t.Helper()
	srv := gmock.NewServer(gmock.WithPort(0))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

func TestNearMiss_NoStubsRegistered_ReturnsEmptyNonNil(t *testing.T) {
	srv := newServerForNearMiss(t)

	got := srv.NearMiss(http.MethodGet, "/anything", nil, "")
	if got == nil {
		t.Fatal("expected non-nil result, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(got))
	}
}

func TestNearMiss_ExactMatch_ReturnsEmptyNonNil(t *testing.T) {
	srv := newServerForNearMiss(t)
	srv.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	got := srv.NearMiss(http.MethodGet, "/api/users", nil, "")
	if got == nil {
		t.Fatal("expected non-nil empty result for exact match, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice on exact match, got %d entries: %+v", len(got), got)
	}
}

func TestNearMiss_OrderingAndDiagnostics(t *testing.T) {
	srv := newServerForNearMiss(t)

	// Stub A: matches method+path but expects header "X-Token: secret".
	// Will produce 1 unmatched dimension (header) -> high partial score.
	idA := srv.Stub(gmock.StubDefinition{
		Name: "A-header-mismatch",
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/api/users",
			Headers: map[string]string{"X-Token": "secret"},
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	// Stub B: completely different method and path.
	// Expected to score lower than A.
	idB := srv.Stub(gmock.StubDefinition{
		Name: "B-everything-mismatch",
		Request: gmock.RequestPattern{
			Method:  http.MethodPost,
			URLPath: "/api/orders",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	got := srv.NearMiss(http.MethodGet, "/api/users", map[string]string{"X-Token": "wrong"}, "")
	if len(got) < 2 {
		t.Fatalf("expected at least 2 near-miss results, got %d: %+v", len(got), got)
	}

	// Best score (A) must come first.
	if got[0].StubID != idA {
		t.Errorf("expected stub A first, got %q (ids: A=%s B=%s)", got[0].StubID, idA, idB)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("expected got[0].Score (%d) > got[1].Score (%d)", got[0].Score, got[1].Score)
	}

	// Verify A's breakdown contains a Reason on the unmatched header dimension.
	foundUnmatchedWithReason := false
	for _, dim := range got[0].Breakdown {
		if !dim.Matched && dim.Reason != "" {
			foundUnmatchedWithReason = true
			break
		}
	}
	if !foundUnmatchedWithReason {
		t.Errorf("expected at least one unmatched dimension with a Reason on top result, breakdown=%+v", got[0].Breakdown)
	}
}

func TestNearMiss_TopNDefaultTruncation(t *testing.T) {
	srv := newServerForNearMiss(t)

	// Register 7 distinctly-failing stubs; default topN is 5.
	for i := 0; i < 7; i++ {
		srv.Stub(gmock.StubDefinition{
			Name: "stub-" + string(rune('a'+i)),
			Request: gmock.RequestPattern{
				Method:  http.MethodGet,
				URLPath: "/api/" + string(rune('a'+i)),
			},
			Response: gmock.ResponseDefinition{Status: http.StatusOK},
		})
	}

	got := srv.NearMiss(http.MethodGet, "/does-not-exist", nil, "")
	if len(got) != 5 {
		t.Fatalf("expected top-N default truncation to 5, got %d", len(got))
	}
}

func TestNearMiss_DefaultMethodGET(t *testing.T) {
	srv := newServerForNearMiss(t)
	srv.Stub(gmock.StubDefinition{
		Request: gmock.RequestPattern{
			Method:  http.MethodGet,
			URLPath: "/x",
		},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	// Empty method should default to GET; with matching path this becomes an
	// exact match -> empty result.
	got := srv.NearMiss("", "/x", nil, "")
	if len(got) != 0 {
		t.Fatalf("expected empty (exact match w/ defaulted GET), got %+v", got)
	}
}

func TestNearMiss_MalformedPath_ReturnsEmptyNonNil(t *testing.T) {
	srv := newServerForNearMiss(t)
	srv.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/x"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	cases := []struct {
		name string
		path string
	}{
		{"control_byte_DEL", "\x7f"},
		{"control_byte_NUL", "\x00"},
		{"newline_in_path", "\n"},
		{"space_in_host_position", " "},
		{"invalid_percent_escape", "%ZZ"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("NearMiss panicked on malformed path %q: %v", tc.path, r)
				}
			}()
			got := srv.NearMiss(http.MethodGet, tc.path, nil, "")
			if got == nil {
				t.Fatalf("expected non-nil empty slice for malformed path %q, got nil", tc.path)
			}
			if len(got) != 0 {
				t.Fatalf("expected empty slice for malformed path %q, got %d entries", tc.path, len(got))
			}
		})
	}
}

func TestNearMiss_EmptyPath_DoesNotPanic(t *testing.T) {
	srv := newServerForNearMiss(t)
	srv.Stub(gmock.StubDefinition{
		Request:  gmock.RequestPattern{Method: http.MethodGet, URLPath: "/x"},
		Response: gmock.ResponseDefinition{Status: http.StatusOK},
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NearMiss panicked on empty path: %v", r)
		}
	}()
	got := srv.NearMiss(http.MethodGet, "", nil, "")
	if got == nil {
		t.Fatal("expected non-nil result for empty path, got nil")
	}
}

func TestNearMiss_ConcurrentSmoke(t *testing.T) {
	srv := newServerForNearMiss(t)

	// Seed a few stubs.
	for i := 0; i < 3; i++ {
		srv.Stub(gmock.StubDefinition{
			Request: gmock.RequestPattern{
				Method:  http.MethodGet,
				URLPath: "/api/" + string(rune('a'+i)),
			},
			Response: gmock.ResponseDefinition{Status: http.StatusOK},
		})
	}

	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup

	// Readers: call NearMiss repeatedly.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = srv.NearMiss(http.MethodGet, "/api/missing", map[string]string{"X-K": "v"}, "")
			}
		}()
	}

	// Writer: churn the registry while readers run.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			id := srv.Stub(gmock.StubDefinition{
				Request: gmock.RequestPattern{
					Method:  http.MethodPost,
					URLPath: "/churn/" + strings.Repeat("z", i%5+1),
				},
				Response: gmock.ResponseDefinition{Status: http.StatusOK},
			})
			if id != "" {
				srv.DeleteStub(id)
			}
		}
	}()

	wg.Wait()
}

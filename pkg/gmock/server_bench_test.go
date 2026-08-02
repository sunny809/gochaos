package gmock_test

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/sunny809/gochaos/pkg/gmock"
	"github.com/sunny809/gochaos/test/testutil"
)

// BenchmarkFullPipeline measures the end-to-end request matching + response
// pipeline throughput. It registers 100 stubs, then sends concurrent requests
// and measures how many complete per second.
//
// Target: ≥10,000 req/sec on an 8-core machine.
func BenchmarkFullPipeline(b *testing.B) {
	// Register 100 stubs with varying paths
	srv := testutil.StartServer(b)
	baseURL := srv.URL()

	for i := range 100 {
		path := fmt.Sprintf("/api/resource/%d", i)
		srv.Stub(gmock.StubDefinition{
			Request: gmock.RequestPattern{
				Method:  http.MethodGet,
				URLPath: path,
			},
			Response: gmock.ResponseDefinition{
				Status: http.StatusOK,
				Body:   fmt.Sprintf(`{"id":%d,"name":"resource-%d"}`, i, i),
			},
		})
	}

	// Pre-construct HTTP client with keep-alive
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
		},
	}

	// Warm-up: send a few requests to establish connections
	for i := range 10 {
		resp, err := client.Get(baseURL + fmt.Sprintf("/api/resource/%d", i))
		if err != nil {
			b.Fatalf("warm-up request failed: %v", err)
		}
		resp.Body.Close()
	}

	// Benchmark with 100 concurrent goroutines handled by RunParallel.
	// Use an atomic counter to distribute requests across stub paths.
	b.ResetTimer()
	b.SetParallelism(100)

	var counter atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := counter.Add(1) % 100
			path := fmt.Sprintf("/api/resource/%d", idx)

			resp, err := client.Get(baseURL + path)
			if err != nil {
				b.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				b.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		}
	})
}

// BenchmarkNoMatch measures throughput for unmatched requests (404 path).
// This tests the near-miss computation overhead on every unmatched request.
func BenchmarkNoMatch(b *testing.B) {
	srv := testutil.StartServer(b)
	baseURL := srv.URL()

	// Register 100 stubs (so near-miss has work to do)
	for i := range 100 {
		path := fmt.Sprintf("/api/resource/%d", i)
		srv.Stub(gmock.StubDefinition{
			Request: gmock.RequestPattern{
				Method:  http.MethodGet,
				URLPath: path,
			},
			Response: gmock.ResponseDefinition{
				Status: http.StatusOK,
				Body:   `{"ok":true}`,
			},
		})
	}

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Send to a path that doesn't match any stub
			resp, err := client.Get(baseURL + "/api/no-match")
			if err != nil {
				b.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				b.Fatalf("expected 404, got %d", resp.StatusCode)
			}
		}
	})
}
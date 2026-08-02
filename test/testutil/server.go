// Package testutil provides shared test helpers for the gochaos project.
//
// Server lifecycle and async polling are the two most common patterns across
// unit, integration, and BDD tests. Consolidating them here eliminates the
// five startServer variants and eight time.Sleep calls that were previously
// duplicated across test packages.
package testutil

import (
	"testing"
	"time"

	"github.com/sunny809/gochaos/pkg/gmock"
)

// StartServer creates a gmock server on a random port, starts it, and
// registers cleanup via t.Cleanup. The variadic opts are appended after
// WithPort(0), so callers can override defaults or add options like
// WithCallbackSSRFBypass() and WithRandSeed().
//
// It accepts testing.TB so both *testing.T (unit/integration) and
// *testing.B (benchmarks) can use it.
func StartServer(t testing.TB, opts ...gmock.Option) gmock.Server {
	t.Helper()
	opts = append([]gmock.Option{gmock.WithPort(0)}, opts...)
	srv := gmock.NewServer(opts...)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

// Wait polls fn every 10ms until it returns true or the timeout elapses.
// On timeout it calls t.Fatalf with the description. Replaces time.Sleep
// in async callback/dispatch tests with a deterministic wait.
func Wait(t testing.TB, desc string, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s (%v)", desc, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

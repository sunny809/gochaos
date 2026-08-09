package gmock

import (
	"testing"
)

// TestNilServer_Verify verifies that Verify on a nil server returns a
// graceful result instead of panicking on a nil pointer dereference.
func TestNilServer_Verify(t *testing.T) {
	var s *mockServer

	result := s.Verify(RequestPattern{Method: "GET", URLPath: "/test"}, 1)
	if result.Matched {
		t.Error("nil server: expected Matched=false")
	}
	if result.ExpectedCount != 1 {
		t.Errorf("nil server: ExpectedCount = %d, want 1", result.ExpectedCount)
	}
	if result.ActualCount != 0 {
		t.Errorf("nil server: ActualCount = %d, want 0", result.ActualCount)
	}
	if len(result.Errors) == 0 {
		t.Error("nil server: expected error message, got none")
	}
}

// TestNilServer_VerifyNotCalled verifies the nil guard for VerifyNotCalled.
func TestNilServer_VerifyNotCalled(t *testing.T) {
	var s *mockServer

	// VerifyNotCalled is Verify(pattern, 0), which should be Matched=true
	// when count <= 0 even on a nil server.
	result := s.VerifyNotCalled(RequestPattern{Method: "GET"})
	if !result.Matched {
		t.Error("nil server: VerifyNotCalled with count=0 should match")
	}
}

// TestNilServer_VerifyFaultsInjected verifies the nil guard for VerifyFaultsInjected.
func TestNilServer_VerifyFaultsInjected(t *testing.T) {
	var s *mockServer

	result := s.VerifyFaultsInjected(FaultPattern{FaultType: "error"}, 1)
	if result.Matched {
		t.Error("nil server: expected Matched=false")
	}
	if result.ExpectedCount != 1 {
		t.Errorf("nil server: ExpectedCount = %d, want 1", result.ExpectedCount)
	}
	if len(result.Errors) == 0 {
		t.Error("nil server: expected error message, got none")
	}
}

// TestNilServer_VerifyCallbacks verifies the nil guard for VerifyCallbacks.
func TestNilServer_VerifyCallbacks(t *testing.T) {
	var s *mockServer

	result := s.VerifyCallbacks(CallbackPattern{Status: "delivered"}, 1)
	if result.Matched {
		t.Error("nil server: expected Matched=false")
	}
	if result.ExpectedCount != 1 {
		t.Errorf("nil server: ExpectedCount = %d, want 1", result.ExpectedCount)
	}
	if len(result.Errors) == 0 {
		t.Error("nil server: expected error message, got none")
	}
}

// TestNilServer_RequestLog verifies the nil guard for RequestLog.
func TestNilServer_RequestLog(t *testing.T) {
	var s *mockServer

	result := s.RequestLog()
	if result != nil {
		t.Errorf("nil server: expected nil RequestLog, got %v", result)
	}
}

// TestNilServer_UnmatchedRequests verifies the nil guard for UnmatchedRequests.
func TestNilServer_UnmatchedRequests(t *testing.T) {
	var s *mockServer

	result := s.UnmatchedRequests()
	if result != nil {
		t.Errorf("nil server: expected nil UnmatchedRequests, got %v", result)
	}
}

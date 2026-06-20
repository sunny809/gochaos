package gmock

// Verify checks that a request matching the given pattern was received
// at least the specified number of times.
func (s *mockServer) Verify(pattern RequestPattern, count int) VerificationResult {
	if s == nil {
		return VerificationResult{
			ExpectedCount: count,
			ActualCount:   0,
			Matched:       count <= 0,
			Errors:        []string{"server not initialized"},
		}
	}
	return s.verify(pattern, count)
}

// VerifyNotCalled checks that no request matching the given pattern was received.
// Equivalent to Verify(pattern, 0).
func (s *mockServer) VerifyNotCalled(pattern RequestPattern) VerificationResult {
	return s.Verify(pattern, 0)
}

// VerifyFaultsInjected checks that faults matching the given pattern were injected
// at least the specified number of times. Pattern fields are optional; empty fields
// match all entries.
func (s *mockServer) VerifyFaultsInjected(pattern FaultPattern, count int) FaultVerificationResult {
	if s == nil {
		return FaultVerificationResult{
			ExpectedCount: count,
			ActualCount:   0,
			Matched:       count <= 0,
			Errors:        []string{"server not initialized"},
		}
	}
	return s.verifyFaultsInjected(pattern, count)
}

// RequestLog returns all logged requests for inspection.
// The returned slice is a snapshot; it is safe to iterate but should not be modified.
func (s *mockServer) RequestLog() []LoggedRequest {
	if s == nil || s.requestLog == nil {
		return nil
	}
	return s.requestLog.List()
}

// UnmatchedRequests returns all requests that matched no stub.
func (s *mockServer) UnmatchedRequests() []LoggedRequest {
	if s == nil || s.requestLog == nil {
		return nil
	}
	return s.requestLog.Unmatched()
}
package steps

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/sunny809/gochaos/pkg/gmock"
)

// registerChaosSteps registers all P2 chaos injection step definitions.
func registerChaosSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Fault injection steps (uses default path "/test")
	ctx.Step(`^a stub with (\S+) fault$`, tc.aStubWithFault)
	ctx.Step(`^a stub with rate_limit fault with (\d+) per second$`, tc.aStubWithRateLimitFault)
	ctx.Step(`^a stub with (\S+) fault and (\d+) random data bytes$`, tc.aStubWithFaultAndDataLength)

	// Delay steps
	ctx.Step(`^a fixed delay of (\d+)ms$`, tc.aFixedDelay)
	ctx.Step(`^a random delay between (\d+)ms and (\d+)ms$`, tc.aRandomDelay)
	ctx.Step(`^a lognormal delay with P50=(\d+)ms and P95=(\d+)ms$`, tc.aLognormalDelayP95)
	ctx.Step(`^a lognormal delay with P50=(\d+)ms and P99=(\d+)ms$`, tc.aLognormalDelayP99)
	ctx.Step(`^a timeout delay of (\d+)ms$`, tc.aTimeoutDelay)
	ctx.Step(`^a dribble delay with (\d+) chunks over (\d+)ms$`, tc.aDribbleDelay)

	// Activation mode steps
	ctx.Step(`^a stub with (\d+(?:\.\d+)?)% probability of (\S+) fault$`, tc.aStubWithProbabilityFault)
	ctx.Step(`^a stub with every (\d+)(?:st|nd|rd|th)? request (\S+) fault$`, tc.aStubWithEveryNthFault)
	ctx.Step(`^a stub with (\S+) fault active between (\d+)ms and (\d+)ms$`, tc.aStubWithActiveBetweenFault)

	// Seed step
	ctx.Step(`^a seed (\d+)$`, tc.aSeed)

	// Multi-request step
	ctx.Step(`^I send (\d+) requests to "([^"]*)"$`, tc.iSendNRequestsTo)

	// Statistical assertion steps
	ctx.Step(`^at least (\d+) responses are (\d+)$`, tc.atLeastResponsesAreStatus)
	ctx.Step(`^at most (\d+) responses are (\d+)$`, tc.atMostResponsesAreStatus)
	ctx.Step(`^exactly (\d+) responses are (\d+)$`, tc.exactlyResponsesAreStatus)

	// Fault verification steps
	ctx.Step(`^(\d+) (\S+) faults were injected$`, tc.nFaultsWereInjected)
	ctx.Step(`^at least (\d+) (\S+) faults were injected$`, tc.atLeastNFaultsWereInjected)
	ctx.Step(`^at most (\d+) (\S+) faults were injected$`, tc.atMostNFaultsWereInjected)
	ctx.Step(`^(\d+) (\S+) faults were injected via (\S+)$`, tc.nFaultsWereInjectedVia)
	ctx.Step(`^no (\S+) faults were injected$`, tc.noFaultsWereInjected)
}

// ============================================================================
// Fault injection steps
// ============================================================================

// aStubWithFault creates a stub at /test with the given fault type.
// Supported types: error, empty, connection_reset, malformed, random_data, slow_close, rate_limit.
func (tc *TestContext) aStubWithFault(faultType string) error {
	validFaults := map[string]bool{
		"error": true, "empty": true, "connection_reset": true,
		"malformed": true, "random_data": true, "slow_close": true,
		"rate_limit": true,
	}
	if !validFaults[faultType] {
		return fmt.Errorf("unknown fault type: %q", faultType)
	}

	def := tc.buildStubWithFault(faultType)
	id := tc.server.Stub(def)
	if id == "" {
		return fmt.Errorf("failed to register fault stub for type %q", faultType)
	}
	tc.AddStubID(id)
	return nil
}

// aStubWithRateLimitFault creates a stub with rate_limit fault and custom PerSecond.
func (tc *TestContext) aStubWithRateLimitFault(perSecond string) error {
	ps, err := parseInt(perSecond)
	if err != nil {
		return fmt.Errorf("invalid perSecond value %q: %w", perSecond, err)
	}

	def := tc.buildStubWithFault("rate_limit")
	def.Response.Status = 200
	def.Response.Fault.PerSecond = ps
	if def.Response.Fault.AfterRequests == 0 {
		def.Response.Fault.AfterRequests = 5 // burst before rate limiting
	}
	id := tc.server.Stub(def)
	if id == "" {
		return fmt.Errorf("failed to register rate_limit fault stub")
	}
	tc.AddStubID(id)
	return nil
}

// aStubWithFaultAndDataLength creates a stub with random_data fault and custom DataLength.
func (tc *TestContext) aStubWithFaultAndDataLength(faultType, dataLen string) error {
	if faultType != "random_data" {
		return fmt.Errorf("dataLength is only valid for random_data fault, got %q", faultType)
	}
	dl, err := parseInt(dataLen)
	if err != nil {
		return fmt.Errorf("invalid dataLength %q: %w", dataLen, err)
	}

	def := tc.buildStubWithFault("random_data")
	def.Response.Fault.DataLength = dl
	id := tc.server.Stub(def)
	if id == "" {
		return fmt.Errorf("failed to register random_data fault stub with DataLength=%d", dl)
	}
	tc.AddStubID(id)
	return nil
}

// ============================================================================
// Delay steps
// ============================================================================

// aFixedDelay stores a fixed delay config for the next fault stub.
func (tc *TestContext) aFixedDelay(ms string) error {
	v, err := parseInt(ms)
	if err != nil {
		return fmt.Errorf("invalid delay value %q: %w", ms, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type:  "fixed",
		Value: v,
	}
	tc.mu.Unlock()
	return nil
}

// aRandomDelay stores a random delay config for the next fault stub.
func (tc *TestContext) aRandomDelay(minStr, maxStr string) error {
	min, err := parseInt(minStr)
	if err != nil {
		return fmt.Errorf("invalid min delay %q: %w", minStr, err)
	}
	max, err := parseInt(maxStr)
	if err != nil {
		return fmt.Errorf("invalid max delay %q: %w", maxStr, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type: "random",
		Min:  min,
		Max:  max,
	}
	tc.mu.Unlock()
	return nil
}

// aLognormalDelayP95 stores a lognormal delay with P95 for the next fault stub.
func (tc *TestContext) aLognormalDelayP95(p50Str, p95Str string) error {
	p50, err := parseInt(p50Str)
	if err != nil {
		return fmt.Errorf("invalid P50 %q: %w", p50Str, err)
	}
	p95, err := parseInt(p95Str)
	if err != nil {
		return fmt.Errorf("invalid P95 %q: %w", p95Str, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type: "lognormal",
		P50:  p50,
		P95:  p95,
	}
	tc.mu.Unlock()
	return nil
}

// aLognormalDelayP99 stores a lognormal delay with P99 for the next fault stub.
func (tc *TestContext) aLognormalDelayP99(p50Str, p99Str string) error {
	p50, err := parseInt(p50Str)
	if err != nil {
		return fmt.Errorf("invalid P50 %q: %w", p50Str, err)
	}
	p99, err := parseInt(p99Str)
	if err != nil {
		return fmt.Errorf("invalid P99 %q: %w", p99Str, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type: "lognormal",
		P50:  p50,
		P99:  p99,
	}
	tc.mu.Unlock()
	return nil
}

// aTimeoutDelay stores a timeout delay for the next fault stub.
func (tc *TestContext) aTimeoutDelay(ms string) error {
	v, err := parseInt(ms)
	if err != nil {
		return fmt.Errorf("invalid timeout value %q: %w", ms, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type:  "timeout",
		Value: v,
	}
	tc.mu.Unlock()
	return nil
}

// aDribbleDelay stores a dribble delay for the next fault stub.
func (tc *TestContext) aDribbleDelay(chunksStr, totalStr string) error {
	chunks, err := parseInt(chunksStr)
	if err != nil {
		return fmt.Errorf("invalid chunks value %q: %w", chunksStr, err)
	}
	total, err := parseInt(totalStr)
	if err != nil {
		return fmt.Errorf("invalid total duration %q: %w", totalStr, err)
	}
	tc.mu.Lock()
	tc.pendingDelay = &gmock.DelayDefinition{
		Type:          "dribble",
		Chunks:        chunks,
		TotalDuration: total,
	}
	tc.mu.Unlock()
	return nil
}

// ============================================================================
// Activation mode steps
// ============================================================================

// aStubWithProbabilityFault creates a stub with a probabilistic activation.
func (tc *TestContext) aStubWithProbabilityFault(pctStr, faultType string) error {
	pct, err := parseInt(pctStr)
	if err != nil {
		return fmt.Errorf("invalid percentage %q: %w", pctStr, err)
	}
	if pct < 0 || pct > 100 {
		return fmt.Errorf("percentage must be 0-100, got %d", pct)
	}

	tc.mu.Lock()
	tc.pendingActivation = &gmock.Activation{
		Probability: float64(pct) / 100.0,
	}
	tc.mu.Unlock()

	return tc.aStubWithFault(faultType)
}

// aStubWithEveryNthFault creates a stub with every-Nth-request activation.
func (tc *TestContext) aStubWithEveryNthFault(nStr, faultType string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid N %q: %w", nStr, err)
	}
	if n <= 0 {
		return fmt.Errorf("N must be positive, got %d", n)
	}

	tc.mu.Lock()
	tc.pendingActivation = &gmock.Activation{
		EveryNthRequest: n,
	}
	tc.mu.Unlock()

	return tc.aStubWithFault(faultType)
}

// aStubWithActiveBetweenFault creates a stub with time-window activation.
func (tc *TestContext) aStubWithActiveBetweenFault(faultType, startStr, endStr string) error {
	start, err := parseInt(startStr)
	if err != nil {
		return fmt.Errorf("invalid start ms %q: %w", startStr, err)
	}
	end, err := parseInt(endStr)
	if err != nil {
		return fmt.Errorf("invalid end ms %q: %w", endStr, err)
	}

	tc.mu.Lock()
	tc.pendingActivation = &gmock.Activation{
		ActiveBetween: []gmock.TimeWindow{
			{StartMs: int64(start), EndMs: int64(end)},
		},
	}
	tc.mu.Unlock()

	return tc.aStubWithFault(faultType)
}

// ============================================================================
// Seed step
// ============================================================================

// aSeed sets the random seed and restarts the server with WithRandSeed.
func (tc *TestContext) aSeed(seedStr string) error {
	seed, err := parseInt(seedStr)
	if err != nil {
		return fmt.Errorf("invalid seed %q: %w", seedStr, err)
	}
	tc.mu.Lock()
	tc.seed = int64(seed)
	tc.mu.Unlock()

	// Restart the server with the seed applied
	tc.AfterScenario()
	return tc.BeforeScenario()
}

// ============================================================================
// Multi-request step
// ============================================================================

// iSendNRequestsTo sends n GET requests to the given path.
func (tc *TestContext) iSendNRequestsTo(nStr, path string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid request count %q: %w", nStr, err)
	}
	return tc.SendNRequests(n, "GET", path)
}

// ============================================================================
// Statistical assertion steps
// ============================================================================

// atLeastResponsesAreStatus asserts that at least n responses have the given status.
func (tc *TestContext) atLeastResponsesAreStatus(nStr, statusStr string) error {
	expected, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	actual := tc.countResponsesWithStatus(status)
	if actual < expected {
		return fmt.Errorf("expected at least %d responses with status %d, got %d (total responses: %d)",
			expected, status, actual, len(tc.responses))
	}
	return nil
}

// atMostResponsesAreStatus asserts that at most n responses have the given status.
func (tc *TestContext) atMostResponsesAreStatus(nStr, statusStr string) error {
	expected, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	actual := tc.countResponsesWithStatus(status)
	if actual > expected {
		return fmt.Errorf("expected at most %d responses with status %d, got %d (total responses: %d)",
			expected, status, actual, len(tc.responses))
	}
	return nil
}

// exactlyResponsesAreStatus asserts that exactly n responses have the given status.
func (tc *TestContext) exactlyResponsesAreStatus(nStr, statusStr string) error {
	expected, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}
	status, err := parseInt(statusStr)
	if err != nil {
		return fmt.Errorf("invalid status %q: %w", statusStr, err)
	}

	actual := tc.countResponsesWithStatus(status)
	if actual != expected {
		return fmt.Errorf("expected exactly %d responses with status %d, got %d (total responses: %d)",
			expected, status, actual, len(tc.responses))
	}
	return nil
}

// countResponsesWithStatus counts how many collected responses have the given status code.
func (tc *TestContext) countResponsesWithStatus(status int) int {
	count := 0
	for _, resp := range tc.responses {
		if resp != nil && resp.StatusCode == status {
			count++
		}
	}
	return count
}

// ============================================================================
// Fault verification steps — uses VerifyFaultsInjected API
// ============================================================================

// nFaultsWereInjected asserts that exactly n faults of the given type were injected.
func (tc *TestContext) nFaultsWereInjected(nStr, faultType string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}

	result := tc.server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType: faultType,
	}, n)

	if !result.Matched {
		return fmt.Errorf("fault verification failed: expected %d %q faults, got %d (errors: %s)",
			n, faultType, result.ActualCount, strings.Join(result.Errors, "; "))
	}
	return nil
}

// atLeastNFaultsWereInjected asserts that at least n faults of the given type were injected.
func (tc *TestContext) atLeastNFaultsWereInjected(nStr, faultType string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}

	result := tc.server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType: faultType,
	}, n)

	if result.ActualCount < n {
		return fmt.Errorf("fault verification failed: expected at least %d %q faults, got %d (errors: %s)",
			n, faultType, result.ActualCount, strings.Join(result.Errors, "; "))
	}
	return nil
}

// atMostNFaultsWereInjected asserts that at most n faults of the given type were injected.
func (tc *TestContext) atMostNFaultsWereInjected(nStr, faultType string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}

	result := tc.server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType: faultType,
	}, n)

	if result.ActualCount > n {
		return fmt.Errorf("fault verification failed: expected at most %d %q faults, got %d (errors: %s)",
			n, faultType, result.ActualCount, strings.Join(result.Errors, "; "))
	}
	return nil
}

// nFaultsWereInjectedVia asserts that exactly n faults of the given type were injected
// via the specified activation mode.
func (tc *TestContext) nFaultsWereInjectedVia(nStr, faultType, mode string) error {
	n, err := parseInt(nStr)
	if err != nil {
		return fmt.Errorf("invalid count %q: %w", nStr, err)
	}

	activationMode := tc.mapActivationMode(mode)

	result := tc.server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType:      faultType,
		ActivationMode: activationMode,
	}, n)

	if !result.Matched {
		return fmt.Errorf("fault verification failed: expected %d %q faults via %q, got %d (errors: %s)",
			n, faultType, mode, result.ActualCount, strings.Join(result.Errors, "; "))
	}
	return nil
}

// noFaultsWereInjected asserts that no faults of the given type were injected.
func (tc *TestContext) noFaultsWereInjected(faultType string) error {
	result := tc.server.VerifyFaultsInjected(gmock.FaultPattern{
		FaultType: faultType,
	}, 0)

	if !result.Matched {
		return fmt.Errorf("fault verification failed: expected 0 %q faults, got %d (errors: %s)",
			faultType, result.ActualCount, strings.Join(result.Errors, "; "))
	}
	return nil
}

// mapActivationMode converts a Gherkin step string to the internal ActivationMode constant.
func (tc *TestContext) mapActivationMode(mode string) string {
	switch strings.ToLower(mode) {
	case "always":
		return string(gmock.ModeAlways)
	case "probability":
		return string(gmock.ModeProbability)
	case "every_nth_request", "nth_request", "nth":
		return string(gmock.ModeNthRequest)
	case "time_window", "active_between", "timewindow":
		return string(gmock.ModeTimeWindow)
	case "combined":
		return string(gmock.ModeCombined)
	default:
		return mode
	}
}

// ============================================================================
// Helper methods for TCP-level fault observation
// ============================================================================

// sendAndExpectError sends a request expecting a connection-level error.
func (tc *TestContext) sendAndExpectError(path string) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	_, err := client.Get(tc.baseURL + path)
	if err != nil {
		return nil // expected — request failed due to fault
	}
	return fmt.Errorf("expected connection error for path %q but request succeeded", path)
}

// sendAndIgnoreError sends a request and ignores any connection errors.
func (tc *TestContext) sendAndIgnoreError(path string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(tc.baseURL + path)
	if err != nil {
		return nil // connection error is expected for some fault types
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	tc.mu.Lock()
	tc.response = resp
	tc.mu.Unlock()
	return nil
}

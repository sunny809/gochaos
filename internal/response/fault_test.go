package response

import (
	"strings"
	"testing"

	"github.com/sunny809/gochaos/internal/spec"
)

func TestValidateFaultType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty string is valid", input: "", wantErr: false},
		{name: "error is valid", input: "error", wantErr: false},
		{name: "empty is valid", input: "empty", wantErr: false},
		{name: "connection_reset is valid", input: "connection_reset", wantErr: false},
		{name: "malformed is valid", input: "malformed", wantErr: false},
		{name: "random_data is valid", input: "random_data", wantErr: false},
		{name: "slow_close is valid", input: "slow_close", wantErr: false},
		{name: "rate_limit is valid", input: "rate_limit", wantErr: false},
		{name: "INVALID is rejected", input: "INVALID", wantErr: true},
		{name: "Connection_Reset is rejected (case sensitive)", input: "Connection_Reset", wantErr: true},
		{name: "connection-reset is rejected (hyphen not underscore)", input: "connection-reset", wantErr: true},
		{name: "random string is rejected", input: "timeout", wantErr: true},
		{name: "whitespace is rejected", input: " ", wantErr: true},
		{name: "very long string is rejected", input: "error" + strings.Repeat("x", 1000), wantErr: true},
		{name: "string with leading space is rejected", input: " error", wantErr: true},
		{name: "string with trailing space is rejected", input: "error ", wantErr: true},
		{name: "string with tab is rejected", input: "error\t", wantErr: true},
		{name: "unicode is rejected", input: "错误", wantErr: true},
		{name: "null byte is rejected", input: "error\x00", wantErr: true},
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

func TestValidFaultTypes(t *testing.T) {
	expected := map[string]bool{
		"error":            true,
		"empty":            true,
		"connection_reset": true,
		"malformed":        true,
		"random_data":      true,
		"slow_close":       true,
		"rate_limit":       true,
	}
	got := ValidFaultTypes()
	for k := range expected {
		if !got[k] {
			t.Errorf("ValidFaultTypes() missing key %q", k)
		}
	}
	if len(got) != len(expected) {
		t.Errorf("ValidFaultTypes() has %d entries, expected %d", len(got), len(expected))
	}
}

func TestValidateDelay(t *testing.T) {
	tests := []struct {
		name    string
		delay   *spec.DelayDefinition
		wantErr bool
	}{
		{
			name:    "nil delay is valid",
			delay:   nil,
			wantErr: false,
		},
		{
			name:    "fixed delay is valid",
			delay:   &spec.DelayDefinition{Type: "fixed", Value: 100},
			wantErr: false,
		},
		{
			name:    "random delay is valid",
			delay:   &spec.DelayDefinition{Type: "random", Min: 50, Max: 200},
			wantErr: false,
		},
		{
			name:    "timeout delay is valid",
			delay:   &spec.DelayDefinition{Type: "timeout"},
			wantErr: false,
		},
		{
			name:    "lognormal delay with p50 and p95 is valid",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 100, P95: 500},
			wantErr: false,
		},
		{
			name:    "lognormal delay with p50 and p99 is valid",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 100, P99: 1000},
			wantErr: false,
		},
		{
			name:    "lognormal delay with all percentiles is valid",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 100, P95: 500, P99: 1000},
			wantErr: false,
		},
		{
			name:    "dribble delay with chunks and totalDuration is valid",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: 5, TotalDuration: 500},
			wantErr: false,
		},
		{
			name:    "dribble delay with chunks and value is valid",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: 3, Value: 300},
			wantErr: false,
		},
		{
			name:    "unknown delay type is rejected",
			delay:   &spec.DelayDefinition{Type: "fibonacci"},
			wantErr: true,
		},
		{
			name:    "empty delay type is rejected",
			delay:   &spec.DelayDefinition{Type: ""},
			wantErr: true,
		},
		{
			name:    "dribble with chunks=0 is rejected",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: 0, TotalDuration: 500},
			wantErr: true,
		},
		{
			name:    "dribble with negative chunks is rejected",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: -1, TotalDuration: 500},
			wantErr: true,
		},
		{
			name:    "dribble with no duration is rejected",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: 5},
			wantErr: true,
		},
		{
			name:    "dribble with zero totalDuration and zero value is rejected",
			delay:   &spec.DelayDefinition{Type: "dribble", Chunks: 5, TotalDuration: 0, Value: 0},
			wantErr: true,
		},
		{
			name:    "lognormal with p50=0 is rejected",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 0, P95: 500},
			wantErr: true,
		},
		{
			name:    "lognormal with no p95/p99 is rejected",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 100},
			wantErr: true,
		},
		{
			name:    "lognormal with p50=0 and no p95/p99 is rejected",
			delay:   &spec.DelayDefinition{Type: "lognormal", P50: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDelay(tt.delay)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDelay() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFault(t *testing.T) {
	t.Helper()

	tests := []struct {
		name    string
		fault   *spec.FaultDefinition
		wantErr bool
	}{
		{
			name:    "nil fault is valid",
			fault:   nil,
			wantErr: false,
		},
		{
			name:    "error type is valid",
			fault:   &spec.FaultDefinition{Type: "error"},
			wantErr: false,
		},
		{
			name:    "rate_limit with perSecond > 0 is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 2},
			wantErr: false,
		},
		{
			name:    "rate_limit with perSecond = 0 is invalid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 0},
			wantErr: true,
		},
		{
			name:    "rate_limit with perSecond < 0 is invalid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: -1},
			wantErr: true,
		},
		{
			name:    "rate_limit with afterRequests and perSecond is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", AfterRequests: 5, PerSecond: 2},
			wantErr: false,
		},
		{
			name:    "rate_limit with custom status is valid",
			fault:   &spec.FaultDefinition{Type: "rate_limit", PerSecond: 10, RateLimitStatus: 503},
			wantErr: false,
		},
		{
			name:    "invalid fault type is rejected",
			fault:   &spec.FaultDefinition{Type: "bogus"},
			wantErr: true,
		},
		{
			name:    "empty type is valid",
			fault:   &spec.FaultDefinition{Type: ""},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			err := ValidateFault(tt.fault)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFault() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

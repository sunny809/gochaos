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

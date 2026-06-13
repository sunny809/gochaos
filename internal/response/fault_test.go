package response

import (
	"strings"
	"testing"
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

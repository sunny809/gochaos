// Package response provides the response writing port and adapters for the gmock server.
//
// This file implements fault injection validation for the response pipeline.
// Fault types simulate network-level failures such as internal server errors,
// empty responses, and connection resets.
package response

import (
	"fmt"
	"sort"
	"strings"
)

// validFaultTypes defines the set of supported fault types.
// Each key is a fault type string; the value is always true.
// This is unexported to prevent external mutation; use ValidateFaultType() for validation.
var validFaultTypes = map[string]bool{
	"error":            true,
	"empty":            true,
	"connection_reset": true,
}

// ValidFaultTypes returns the set of recognized fault type strings.
// The returned map is a copy and cannot be used to modify the internal set.
func ValidFaultTypes() map[string]bool {
	result := make(map[string]bool, len(validFaultTypes))
	for k, v := range validFaultTypes {
		result[k] = v
	}
	return result
}

// ValidateFaultType checks whether the given fault type is valid.
// An empty string means no fault is configured, which is considered valid.
// Invalid types return an error listing all supported types.
func ValidateFaultType(faultType string) error {
	if faultType == "" {
		return nil
	}
	if validFaultTypes[faultType] {
		return nil
	}
	valid := make([]string, 0, len(validFaultTypes))
	for k := range validFaultTypes {
		valid = append(valid, k)
	}
	sort.Strings(valid)
	return fmt.Errorf("invalid fault type %q; valid types: %s", faultType, strings.Join(valid, ", "))
}

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

	"github.com/sunny809/gochaos/internal/spec"
)

// validFaultTypes defines the set of supported fault types.
// Each key is a fault type string; the value is always true.
// This is unexported to prevent external mutation; use ValidateFaultType() for validation.
var validFaultTypes = map[string]bool{
	"error":            true,
	"empty":            true,
	"connection_reset": true,
	"malformed":        true,
	"random_data":      true,
	"slow_close":       true,
	"rate_limit":       true,
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

// ValidateFault performs full validation of a FaultDefinition, including
// type-specific field constraints. Returns nil if the definition is valid.
//
// Validation rules:
//   - Type must be a recognized fault type (or empty)
//   - For "rate_limit": PerSecond must be > 0
//   - Activation must be valid (delegated to ValidateActivation)
func ValidateFault(fault *spec.FaultDefinition) error {
	if fault == nil {
		return nil
	}
	if err := ValidateFaultType(fault.Type); err != nil {
		return err
	}
	if fault.Type == "rate_limit" && fault.PerSecond <= 0 {
		return fmt.Errorf("fault.perSecond must be > 0 for rate_limit type, got %d", fault.PerSecond)
	}
	if err := ValidateActivation(fault.Activation); err != nil {
		return err
	}
	return nil
}

// ValidateActivation checks that the activation configuration is valid.
// A nil activation is valid (always-on behavior). When non-nil, each
// configured field is validated:
//   - Probability must be in [0.0, 1.0]
//   - EveryNthRequest must be > 0 when set (negative values are invalid)
//   - Each TimeWindow must have EndMs >= StartMs and Probability in [0, 1]
//
// When multiple activation modes are configured simultaneously (e.g.,
// Probability + EveryNthRequest), they use AND semantics — all configured
// modes must pass for the fault to fire. This is validated as a valid
// combination.
func ValidateActivation(activation *spec.Activation) error {
	if activation == nil {
		return nil
	}

	if activation.Probability < 0.0 || activation.Probability > 1.0 {
		return fmt.Errorf("activation.probability %f is out of range [0.0, 1.0]", activation.Probability)
	}

	if activation.EveryNthRequest < 0 {
		return fmt.Errorf("activation.everyNthRequest %d must be >= 0", activation.EveryNthRequest)
	}
	if activation.EveryNthRequest == 0 && len(activation.ActiveBetween) == 0 && activation.Probability == 0.0 {
		// All fields at zero-value means no activation mode is configured.
		// This is technically valid but semantically useless — treat as valid
		// for forward compatibility (future fields may be added).
	}

	for i, tw := range activation.ActiveBetween {
		if tw.EndMs < tw.StartMs {
			return fmt.Errorf("activation.activeBetween[%d]: endMs %d < startMs %d", i, tw.EndMs, tw.StartMs)
		}
		if tw.Probability < 0.0 || tw.Probability > 1.0 {
			return fmt.Errorf("activation.activeBetween[%d]: probability %f is out of range [0.0, 1.0]", i, tw.Probability)
		}
	}

	// Check for overlapping windows. Overlaps are not rejected (the first
	// matching window wins at runtime), but they are flagged as an error
	// because they usually indicate a configuration mistake.
	for i := 0; i < len(activation.ActiveBetween); i++ {
		for j := i + 1; j < len(activation.ActiveBetween); j++ {
			a, b := activation.ActiveBetween[i], activation.ActiveBetween[j]
			// Two windows [a.start, a.end) and [b.start, b.end) overlap if
			// a.start < b.end && b.start < a.end.
			if a.StartMs < b.EndMs && b.StartMs < a.EndMs {
				return fmt.Errorf("activation.activeBetween: windows [%d] and [%d] overlap ([%d,%d) and [%d,%d))",
					i, j, a.StartMs, a.EndMs, b.StartMs, b.EndMs)
			}
		}
	}

	return nil
}

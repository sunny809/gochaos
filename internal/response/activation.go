// Package response provides the response writing port and adapters for the gmock server.
//
// This file implements the activation logic for fault injection. When a fault
// has an Activation configuration, ShouldActivate determines whether the fault
// should fire for the current request based on probability, request count, and
// time window criteria.
package response

import (
	"time"

	"github.com/sunny809/gochaos/internal/randx"
	"github.com/sunny809/gochaos/internal/spec"
)

// ShouldActivateResult captures the outcome of activation evaluation.
type ShouldActivateResult struct {
	ShouldFire bool
	Mode       spec.ActivationMode
}

// ShouldActivate checks whether a fault should be triggered based on its
// activation criteria. Returns ShouldActivateResult with ShouldFire=true if
// the fault should fire, along with the ActivationMode that caused it to fire.
//
// When activation is nil, returns {true, ModeAlways} (always-on behavior),
// preserving backward compatibility with stubs that do not configure activation.
//
// Activation modes are evaluated independently. When multiple modes are
// configured simultaneously, they use AND semantics: all configured modes
// must pass for the fault to fire. A mode is considered "configured" when
// its field is set to a non-zero value:
//
//   - Probability > 0: RNG draw must be < Probability
//   - EveryNthRequest > 0: hitCount must be a multiple of EveryNthRequest
//   - ActiveBetween non-empty: current time must fall within a window (A3)
//
// When an Activation struct is present but no mode is configured (all
// zero-values), the fault is always-on for backward compatibility.
//
// Parameters:
//   - activation: the Activation configuration from FaultDefinition (may be nil)
//   - rng: the seedable random number generator for probabilistic decisions
//   - hitCount: the number of times this stub has been matched (for everyNthRequest)
//   - serverStart: the time the server started (for time-window calculations)
func ShouldActivate(activation *spec.Activation, rng randx.RNG, hitCount uint64, serverStart time.Time) ShouldActivateResult {
	if activation == nil {
		return ShouldActivateResult{ShouldFire: true, Mode: spec.ModeAlways}
	}

	// Determine which modes are configured (non-zero value).
	hasProb := activation.Probability > 0.0
	hasNth := activation.EveryNthRequest > 0
	hasTimeWindow := len(activation.ActiveBetween) > 0

	// If no mode is configured, treat as always-on.
	if !hasProb && !hasNth && !hasTimeWindow {
		return ShouldActivateResult{ShouldFire: true, Mode: spec.ModeAlways}
	}

	// Evaluate each configured mode. Unconfigured modes default to true
	// (no constraint), so they don't block activation in the AND chain.

	// A1: Probability-based activation.
	probPass := true
	if hasProb {
		probPass = rng.Float64() < activation.Probability
	}

	// A2: EveryNthRequest activation — fire when hitCount is a multiple of
	// EveryNthRequest. hitCount is the count AFTER increment (starts at 1).
	nthPass := true
	if hasNth {
		nthPass = hitCount%uint64(activation.EveryNthRequest) == 0
	}

	// A3: ActiveBetween time-window activation.
	// Current time is measured relative to serverStart (elapsed since server boot).
	// If the current elapsed ms falls within a window, the time constraint passes.
	// A window-level Probability overrides the top-level Probability when set (> 0).
	// A window with no Probability (zero-value) is always-on within that window.
	// If the current time is outside all windows, the time constraint fails.
	timeWindowPass := true
	if hasTimeWindow {
		timeWindowPass = false
		elapsedMs := time.Since(serverStart).Milliseconds()
		for _, tw := range activation.ActiveBetween {
			if elapsedMs >= tw.StartMs && elapsedMs < tw.EndMs {
				// Hit a window. Check window-level probability.
				if tw.Probability > 0.0 {
					// Window-level probability overrides top-level.
					timeWindowPass = rng.Float64() < tw.Probability
				} else {
					// No window-level probability — always-on within this window.
					timeWindowPass = true
				}
				break // first matching window wins (array order)
			}
		}
	}

	// AND semantics: all configured modes must pass.
	result := probPass && nthPass && timeWindowPass

	if !result {
		return ShouldActivateResult{ShouldFire: false, Mode: ""}
	}

	// Determine the activation mode for logging.
	// When multiple modes are configured and all pass, use ModeCombined.
	mode := determineActivationMode(hasProb, hasNth, hasTimeWindow)
	return ShouldActivateResult{ShouldFire: true, Mode: mode}
}

// determineActivationMode returns the ActivationMode based on which modes were configured.
func determineActivationMode(hasProb, hasNth, hasTimeWindow bool) spec.ActivationMode {
	count := 0
	var mode spec.ActivationMode

	if hasProb {
		count++
		mode = spec.ModeProbability
	}
	if hasNth {
		count++
		mode = spec.ModeNthRequest
	}
	if hasTimeWindow {
		count++
		mode = spec.ModeTimeWindow
	}

	if count > 1 {
		return spec.ModeCombined
	}
	return mode
}

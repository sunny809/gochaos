package report

import (
	"encoding/json"

	"github.com/sunny809/gochaos/internal/spec"
)

// JSONReport is the structured report envelope for the JSON format.
type JSONReport struct {
	Suite   string                     `json:"suite"`
	Entries []spec.FaultInjectionEntry `json:"entries"`
}

// JSON renders the fault injection log as a JSON report envelope.
func JSON(entries []spec.FaultInjectionEntry, suiteName string) ([]byte, error) {
	// Normalize nil to an empty slice so the envelope renders "entries": []
	// instead of "entries": null — script consumers do `.entries[]` and
	// must always see an array.
	if entries == nil {
		entries = []spec.FaultInjectionEntry{}
	}
	return json.MarshalIndent(JSONReport{Suite: suiteName, Entries: entries}, "", "  ")
}

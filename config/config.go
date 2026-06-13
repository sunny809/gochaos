// Package config provides configuration loading for gmock.
//
// Supports loading stub definitions from JSON or YAML files. Single-stub and
// multi-stub formats are supported.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sunny809/gochaos/internal/spec"
)

// StubFile represents the on-disk format for stub files.
// A file may contain a single stub or a list of stubs under "mappings".
type StubFile struct {
	Mappings []spec.StubDefinition `json:"mappings,omitempty" yaml:"mappings,omitempty"`
}

// LoadStubsFromFile reads a JSON or YAML file and returns the contained stubs.
// The format is auto-detected from the file extension.
//
// Supports:
//   - .json / .yaml / .yml files
//   - Single stub at the root
//   - Multiple stubs under a "mappings" array
func LoadStubsFromFile(path string) ([]spec.StubDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return parseYAML(data)
	case ".json":
		return parseJSON(data)
	default:
		// Try JSON first, fall back to YAML
		if stubs, err := parseJSON(data); err == nil {
			return stubs, nil
		}
		return parseYAML(data)
	}
}

// LoadStubsFromFiles loads stubs from multiple files and returns the combined result.
func LoadStubsFromFiles(paths []string) ([]spec.StubDefinition, error) {
	var all []spec.StubDefinition
	for _, path := range paths {
		stubs, err := LoadStubsFromFile(path)
		if err != nil {
			return nil, err
		}
		all = append(all, stubs...)
	}
	return all, nil
}

func parseJSON(data []byte) ([]spec.StubDefinition, error) {
	// Try multi-stub format first
	var file StubFile
	if err := json.Unmarshal(data, &file); err == nil && file.Mappings != nil {
		return file.Mappings, nil
	}

	// Try array of stubs
	var array []spec.StubDefinition
	if err := json.Unmarshal(data, &array); err == nil {
		return array, nil
	}

	// Single stub
	var single spec.StubDefinition
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("config: parse JSON: %w", err)
	}
	return []spec.StubDefinition{single}, nil
}

func parseYAML(data []byte) ([]spec.StubDefinition, error) {
	// Try multi-stub format first
	var file StubFile
	if err := yaml.Unmarshal(data, &file); err == nil && file.Mappings != nil {
		return file.Mappings, nil
	}

	// Try array of stubs
	var array []spec.StubDefinition
	if err := yaml.Unmarshal(data, &array); err == nil && len(array) > 0 {
		return array, nil
	}

	// Single stub
	var single spec.StubDefinition
	if err := yaml.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("config: parse YAML: %w", err)
	}
	return []spec.StubDefinition{single}, nil
}
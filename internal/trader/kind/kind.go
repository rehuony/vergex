// Package kind says what can run as a trader. Each kind has a file of its own with
// its config, starting points and limits; a trader carries one kind and its config.
package kind

import (
	"encoding/json"
	"fmt"
)

// The kinds of trader.
const (
	Agent = "agent" // Asks a model for decisions on a set of coins.
	Grid  = "grid"  // Has a model manage grid levels on one symbol.
)

// Config is what a kind decides.
type Config interface {
	// Validate refuses a value outside its choices and brings numbers within bounds.
	Validate() error
	// RiskFields are the fields of this config that change position size or leverage,
	// named as they are in its JSON.
	RiskFields() []string
}

// Preset is a starting config, named by ID. What it is called is up to the interface.
type Preset struct {
	ID     string `json:"id"`
	Config Config `json:"config"`
}

func empty(name string) (Config, error) {
	switch name {
	case Agent:
		return &AgentConfig{}, nil
	case Grid:
		return &GridConfig{}, nil
	}
	return nil, fmt.Errorf("unknown trader kind %q", name)
}

// Decode reads a stored config of a kind and checks it.
func Decode(name string, raw []byte) (Config, error) {
	config, err := empty(name)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, config); err != nil {
		return nil, fmt.Errorf("failed to parse %s config: %w", name, err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

// Presets lists the starting configs of a kind. The first is the default.
func Presets(name string) ([]Preset, error) {
	switch name {
	case Agent:
		return agentPresets(), nil
	case Grid:
		return []Preset{{ID: "default", Config: DefaultGrid()}}, nil
	}
	return nil, fmt.Errorf("unknown trader kind %q", name)
}

// FromPreset returns a kind's preset; an empty id is its default.
func FromPreset(name, id string) (Config, error) {
	presets, err := Presets(name)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return presets[0].Config, nil
	}
	for _, preset := range presets {
		if preset.ID == id {
			return preset.Config, nil
		}
	}
	return nil, fmt.Errorf("unknown %s preset %q", name, id)
}

// Merge applies a partial config onto a whole one and checks the result. Nested
// objects merge, so fields a patch leaves out keep their values. It also returns what
// it had to bring within bounds, so whoever asked can be told.
func Merge(name string, base Config, patch map[string]any) (Config, []string, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, nil, err
	}
	var merged map[string]any
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, nil, err
	}
	mergeMaps(merged, patch)
	if raw, err = json.Marshal(merged); err != nil {
		return nil, nil, err
	}

	asked, err := empty(name)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, asked); err != nil {
		return nil, nil, fmt.Errorf("failed to parse %s config: %w", name, err)
	}
	config, err := Decode(name, raw)
	if err != nil {
		return nil, nil, err
	}
	return config, adjustments(asked, config), nil
}

func mergeMaps(dst, src map[string]any) {
	for key, value := range src {
		from, fromMap := value.(map[string]any)
		into, intoMap := dst[key].(map[string]any)
		if fromMap && intoMap {
			mergeMaps(into, from)
			continue
		}
		dst[key] = value
	}
}

// TouchesRisk reports whether a patch changes any of a kind's risk fields, at any depth.
func TouchesRisk(name string, patch map[string]any) bool {
	config, err := empty(name)
	if err != nil {
		return true
	}
	fields := map[string]bool{}
	for _, field := range config.RiskFields() {
		fields[field] = true
	}
	return touches(patch, fields)
}

func touches(patch map[string]any, fields map[string]bool) bool {
	for key, value := range patch {
		if fields[key] {
			return true
		}
		if child, ok := value.(map[string]any); ok && touches(child, fields) {
			return true
		}
	}
	return false
}

// adjustments describes the risk limits Validate changed.
func adjustments(before, after Config) []string {
	from, ok := before.(*AgentConfig)
	to, same := after.(*AgentConfig)
	if !ok || !same {
		return nil
	}
	return from.RiskControl.adjustments(to.RiskControl)
}

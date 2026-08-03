// Package googlegenerativeai adapts Google Generative AI endpoints to
// Denova's provider-neutral agent model contract.
package googlegenerativeai

import (
	"fmt"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

type ThinkingMode string

const (
	ThinkingModeNone   ThinkingMode = "none"
	ThinkingModeLevel  ThinkingMode = "level"
	ThinkingModeBudget ThinkingMode = "budget"
)

type Compatibility struct {
	APIVersion      string            `json:"api_version,omitempty"`
	ThinkingMode    ThinkingMode      `json:"thinking_mode,omitempty"`
	ThinkingLevels  map[string]string `json:"thinking_levels,omitempty"`
	ThinkingBudgets map[string]int32  `json:"thinking_budgets,omitempty"`
	ExtraBody       map[string]any    `json:"extra_body,omitempty"`
}

func resolveCompatibility(config providers.ModelConfig) (Compatibility, error) {
	compatibility := Compatibility{}
	if err := providers.DecodeProtocolOptions(config.ProtocolOptions, &compatibility); err != nil {
		return Compatibility{}, fmt.Errorf("google generative AI compatibility: %w", err)
	}
	compatibility.APIVersion = strings.Trim(strings.TrimSpace(compatibility.APIVersion), "/")
	if compatibility.ThinkingMode == "" {
		compatibility.ThinkingMode = ThinkingModeNone
	}
	switch compatibility.ThinkingMode {
	case ThinkingModeNone, ThinkingModeLevel, ThinkingModeBudget:
	default:
		return Compatibility{}, fmt.Errorf("google generative AI compatibility: unsupported thinking mode %q", compatibility.ThinkingMode)
	}
	for source, target := range compatibility.ThinkingLevels {
		if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
			return Compatibility{}, fmt.Errorf("google generative AI compatibility: invalid thinking level mapping for %q", source)
		}
		compatibility.ThinkingLevels[source] = strings.ToUpper(strings.TrimSpace(target))
	}
	return compatibility, nil
}

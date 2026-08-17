// Package anthropicmessages adapts Anthropic Messages-compatible endpoints to
// Denova's provider-neutral agent model contract.
package anthropicmessages

import (
	"fmt"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

const defaultMaxOutputTokens int64 = 65536

type ThinkingMode string

const (
	ThinkingModeNone     ThinkingMode = "none"
	ThinkingModeAdaptive ThinkingMode = "adaptive"
	ThinkingModeBudget   ThinkingMode = "budget"
)

// Compatibility describes endpoint traits that are not part of Denova's
// provider-neutral model contract.
type Compatibility struct {
	ThinkingMode           ThinkingMode      `json:"thinking_mode,omitempty"`
	SupportsEffort         *bool             `json:"supports_effort,omitempty"`
	EffortMap              map[string]string `json:"effort_map,omitempty"`
	ThinkingBudgets        map[string]int64  `json:"thinking_budgets,omitempty"`
	DefaultThinkingBudget  int64             `json:"default_thinking_budget,omitempty"`
	DefaultMaxOutputTokens int64             `json:"default_max_output_tokens,omitempty"`
	SupportsToolChoice     *bool             `json:"supports_tool_choice,omitempty"`
	ExtraBody              map[string]any    `json:"extra_body,omitempty"`
}

func resolveCompatibility(config providers.ModelConfig) (Compatibility, error) {
	compatibility := Compatibility{}
	if err := providers.DecodeProtocolOptions(config.ProtocolOptions, &compatibility); err != nil {
		return Compatibility{}, fmt.Errorf("anthropic messages compatibility: %w", err)
	}
	if compatibility.ThinkingMode == "" {
		compatibility.ThinkingMode = ThinkingModeNone
	}
	switch compatibility.ThinkingMode {
	case ThinkingModeNone, ThinkingModeAdaptive, ThinkingModeBudget:
	default:
		return Compatibility{}, fmt.Errorf("anthropic messages compatibility: unsupported thinking mode %q", compatibility.ThinkingMode)
	}
	if compatibility.SupportsEffort == nil {
		compatibility.SupportsEffort = boolPointer(true)
	}
	if compatibility.SupportsToolChoice == nil {
		compatibility.SupportsToolChoice = boolPointer(true)
	}
	if compatibility.DefaultMaxOutputTokens == 0 {
		compatibility.DefaultMaxOutputTokens = defaultMaxOutputTokens
	}
	if compatibility.DefaultMaxOutputTokens < 0 {
		return Compatibility{}, fmt.Errorf("anthropic messages compatibility: default max output tokens must be positive")
	}
	if compatibility.ThinkingMode == ThinkingModeBudget && compatibility.DefaultThinkingBudget < 1024 {
		return Compatibility{}, fmt.Errorf("anthropic messages compatibility: default thinking budget must be at least 1024")
	}
	for level, effort := range compatibility.EffortMap {
		if strings.TrimSpace(level) == "" {
			return Compatibility{}, fmt.Errorf("anthropic messages compatibility: effort map contains an empty source")
		}
		compatibility.EffortMap[level] = strings.TrimSpace(effort)
	}
	for level, budget := range compatibility.ThinkingBudgets {
		if strings.TrimSpace(level) == "" || budget < 1024 {
			return Compatibility{}, fmt.Errorf("anthropic messages compatibility: invalid thinking budget for %q", level)
		}
	}
	return compatibility, nil
}

func boolPointer(value bool) *bool { return &value }

func (compatibility Compatibility) mappedEffort(level providers.ThinkingLevel) (string, bool) {
	if level == "" || level == providers.ThinkingLevelDefault || level == providers.ThinkingLevelOff || !*compatibility.SupportsEffort {
		return "", false
	}
	if mapped, ok := compatibility.EffortMap[string(level)]; ok {
		return mapped, mapped != ""
	}
	return string(level), true
}

func (compatibility Compatibility) thinkingBudget(level providers.ThinkingLevel) int64 {
	if budget := compatibility.ThinkingBudgets[string(level)]; budget > 0 {
		return budget
	}
	return compatibility.DefaultThinkingBudget
}

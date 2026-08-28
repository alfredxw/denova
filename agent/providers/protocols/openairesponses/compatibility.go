package openairesponses

import (
	"fmt"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

// StoreMode controls whether the Responses request omits store or sends an
// explicit boolean. Some compatible endpoints reject the field entirely.
type StoreMode string

const (
	StoreModeOmit  StoreMode = "omit"
	StoreModeFalse StoreMode = "false"
	StoreModeTrue  StoreMode = "true"
)

// ReasoningSummary controls the optional reasoning summary request.
type ReasoningSummary string

const (
	ReasoningSummaryOmit ReasoningSummary = "omit"
	ReasoningSummaryAuto ReasoningSummary = "auto"
)

// ReasoningContext controls how much prior reasoning the provider may reuse.
// Omit remains the compatible-endpoint default because not every Responses
// dialect implements this OpenAI reasoning option.
type ReasoningContext string

const (
	ReasoningContextOmit        ReasoningContext = "omit"
	ReasoningContextAuto        ReasoningContext = "auto"
	ReasoningContextCurrentTurn ReasoningContext = "current_turn"
	ReasoningContextAllTurns    ReasoningContext = "all_turns"
)

// Compatibility describes only Responses-dialect traits. Provider identity is
// deliberately absent so custom endpoints can reuse the same behavior.
type Compatibility struct {
	Store                     StoreMode         `json:"store,omitempty"`
	IncludeEncryptedReasoning bool              `json:"include_encrypted_reasoning,omitempty"`
	SupportsReasoningEffort   *bool             `json:"supports_reasoning_effort,omitempty"`
	EffortMap                 map[string]string `json:"effort_map,omitempty"`
	ReasoningContext          ReasoningContext  `json:"reasoning_context,omitempty"`
	ReasoningSummary          ReasoningSummary  `json:"reasoning_summary,omitempty"`
	SupportsToolChoice        *bool             `json:"supports_tool_choice,omitempty"`
	ExtraBody                 map[string]any    `json:"extra_body,omitempty"`
}

func resolveCompatibility(config providers.ModelConfig) (Compatibility, error) {
	compatibility := Compatibility{}
	if err := providers.DecodeProtocolOptions(config.ProtocolOptions, &compatibility); err != nil {
		return Compatibility{}, fmt.Errorf("openai responses compatibility: %w", err)
	}
	if compatibility.Store == "" {
		compatibility.Store = StoreModeOmit
	}
	switch compatibility.Store {
	case StoreModeOmit, StoreModeFalse, StoreModeTrue:
	default:
		return Compatibility{}, fmt.Errorf("openai responses compatibility: unsupported store mode %q", compatibility.Store)
	}
	if compatibility.ReasoningSummary == "" {
		compatibility.ReasoningSummary = ReasoningSummaryOmit
	}
	switch compatibility.ReasoningSummary {
	case ReasoningSummaryOmit, ReasoningSummaryAuto:
	default:
		return Compatibility{}, fmt.Errorf("openai responses compatibility: unsupported reasoning summary %q", compatibility.ReasoningSummary)
	}
	if compatibility.ReasoningContext == "" {
		compatibility.ReasoningContext = ReasoningContextOmit
	}
	switch compatibility.ReasoningContext {
	case ReasoningContextOmit, ReasoningContextAuto, ReasoningContextCurrentTurn, ReasoningContextAllTurns:
	default:
		return Compatibility{}, fmt.Errorf("openai responses compatibility: unsupported reasoning context %q", compatibility.ReasoningContext)
	}
	if compatibility.SupportsReasoningEffort == nil {
		compatibility.SupportsReasoningEffort = boolPointer(true)
	}
	if compatibility.SupportsToolChoice == nil {
		compatibility.SupportsToolChoice = boolPointer(true)
	}
	for source, target := range compatibility.EffortMap {
		if strings.TrimSpace(source) == "" {
			return Compatibility{}, fmt.Errorf("openai responses compatibility: effort map contains an empty source")
		}
		compatibility.EffortMap[source] = strings.TrimSpace(target)
	}
	return compatibility, nil
}

func boolPointer(value bool) *bool { return &value }

func (compatibility Compatibility) mappedEffort(level providers.ThinkingLevel) (string, bool) {
	if level == "" || level == providers.ThinkingLevelDefault || !*compatibility.SupportsReasoningEffort {
		return "", false
	}
	if mapped, exists := compatibility.EffortMap[string(level)]; exists {
		return mapped, mapped != ""
	}
	if level == providers.ThinkingLevelOff {
		return "none", true
	}
	return string(level), true
}

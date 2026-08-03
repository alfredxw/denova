package openaichatcompletions

import (
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// ThinkingToggle identifies only the extra request field used to enable or
// disable thinking. Reasoning effort remains an independent capability because
// endpoints such as DeepSeek require both controls.
type ThinkingToggle string

const (
	ThinkingToggleNone           ThinkingToggle = "none"
	ThinkingToggleNested         ThinkingToggle = "nested"
	ThinkingToggleEnableThinking ThinkingToggle = "enable_thinking"
)

// ReasoningReplay controls which assistant reasoning blocks must round-trip.
type ReasoningReplay string

const (
	ReasoningReplayNever     ReasoningReplay = "never"
	ReasoningReplayToolCalls ReasoningReplay = "tool_calls"
	ReasoningReplayAlways    ReasoningReplay = "always"
)

// MaxTokensField selects the compatible Chat Completions output-limit field.
type MaxTokensField string

const (
	MaxTokensFieldMaxTokens           MaxTokensField = "max_tokens"
	MaxTokensFieldMaxCompletionTokens MaxTokensField = "max_completion_tokens"
)

// Compatibility contains endpoint-dialect traits interpreted only by the Chat
// Completions adapter. Pointer booleans distinguish an omitted safe default
// from an explicit endpoint override.
type Compatibility struct {
	ThinkingToggle               ThinkingToggle    `json:"thinking_toggle,omitempty"`
	SupportsReasoningEffort      *bool             `json:"supports_reasoning_effort,omitempty"`
	EffortMap                    map[string]string `json:"effort_map,omitempty"`
	ReasoningContentField        string            `json:"reasoning_content_field,omitempty"`
	ReasoningReplay              ReasoningReplay   `json:"reasoning_replay,omitempty"`
	SupportsStreamUsage          *bool             `json:"supports_stream_usage,omitempty"`
	SupportsToolChoice           *bool             `json:"supports_tool_choice,omitempty"`
	MaxTokensField               MaxTokensField    `json:"max_tokens_field,omitempty"`
	ExtraBody                    map[string]any    `json:"extra_body,omitempty"`
	RepairTextToolCalls          bool              `json:"repair_text_tool_calls,omitempty"`
	RepairInlineThinking         bool              `json:"repair_inline_thinking,omitempty"`
	RequestReasoningSplit        bool              `json:"request_reasoning_split,omitempty"`
	RequiresAssistantToolContent bool              `json:"requires_assistant_tool_content,omitempty"`
}

func resolveCompatibility(config providers.ModelConfig) (Compatibility, error) {
	compatibility := Compatibility{}
	if err := providers.DecodeProtocolOptions(config.ProtocolOptions, &compatibility); err != nil {
		return Compatibility{}, fmt.Errorf("openai chat completions compatibility: %w", err)
	}
	if compatibility.ThinkingToggle == "" {
		compatibility.ThinkingToggle = ThinkingToggleNone
	}
	switch compatibility.ThinkingToggle {
	case ThinkingToggleNone, ThinkingToggleNested, ThinkingToggleEnableThinking:
	default:
		return Compatibility{}, fmt.Errorf("openai chat completions compatibility: unsupported thinking toggle %q", compatibility.ThinkingToggle)
	}
	if compatibility.ReasoningReplay == "" {
		compatibility.ReasoningReplay = ReasoningReplayNever
	}
	switch compatibility.ReasoningReplay {
	case ReasoningReplayNever, ReasoningReplayToolCalls, ReasoningReplayAlways:
	default:
		return Compatibility{}, fmt.Errorf("openai chat completions compatibility: unsupported reasoning replay %q", compatibility.ReasoningReplay)
	}
	if compatibility.MaxTokensField == "" {
		compatibility.MaxTokensField = MaxTokensFieldMaxTokens
	}
	switch compatibility.MaxTokensField {
	case MaxTokensFieldMaxTokens, MaxTokensFieldMaxCompletionTokens:
	default:
		return Compatibility{}, fmt.Errorf("openai chat completions compatibility: unsupported max token field %q", compatibility.MaxTokensField)
	}
	if compatibility.ReasoningContentField == "" {
		compatibility.ReasoningContentField = "reasoning_content"
	}
	compatibility.ReasoningContentField = strings.TrimSpace(compatibility.ReasoningContentField)
	if compatibility.SupportsReasoningEffort == nil {
		compatibility.SupportsReasoningEffort = boolPointer(true)
	}
	if compatibility.SupportsStreamUsage == nil {
		compatibility.SupportsStreamUsage = boolPointer(true)
	}
	if compatibility.SupportsToolChoice == nil {
		compatibility.SupportsToolChoice = boolPointer(true)
	}
	return compatibility, nil
}

func boolPointer(value bool) *bool { return &value }

func (compatibility Compatibility) mappedEffort(level providers.ThinkingLevel) (string, bool) {
	if level == "" || level == providers.ThinkingLevelDefault || !*compatibility.SupportsReasoningEffort {
		return "", false
	}
	key := string(level)
	if mapped, exists := compatibility.EffortMap[key]; exists {
		return mapped, mapped != ""
	}
	if level == providers.ThinkingLevelOff {
		return "none", true
	}
	return key, true
}

func (compatibility Compatibility) thinkingFields(level providers.ThinkingLevel) map[string]any {
	if level == "" || level == providers.ThinkingLevelDefault || compatibility.ThinkingToggle == ThinkingToggleNone {
		return nil
	}
	enabled := level != providers.ThinkingLevelOff
	switch compatibility.ThinkingToggle {
	case ThinkingToggleNested:
		mode := "enabled"
		if !enabled {
			mode = "disabled"
		}
		return map[string]any{"thinking": map[string]any{"type": mode}}
	case ThinkingToggleEnableThinking:
		return map[string]any{"enable_thinking": enabled}
	case ThinkingToggleNone:
		return nil
	}
	return nil
}

func (compatibility Compatibility) shouldReplayReasoning(message *agent.Message, level providers.ThinkingLevel) bool {
	if message == nil || level == providers.ThinkingLevelOff {
		return false
	}
	switch compatibility.ReasoningReplay {
	case ReasoningReplayNever:
		return false
	case ReasoningReplayToolCalls:
		return len(message.ToolCalls) > 0
	case ReasoningReplayAlways:
		return true
	}
	return false
}

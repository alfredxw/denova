package providers

import (
	"fmt"
	"strings"
)

// ThinkingLevel is Denova's provider-neutral reasoning control. Default asks
// the adapter to omit provider thinking fields; all other values are explicit.
// Adapters own the wire-level mapping for their protocol and provider.
type ThinkingLevel string

const (
	ThinkingLevelDefault ThinkingLevel = "default"
	ThinkingLevelOff     ThinkingLevel = "off"
	ThinkingLevelLow     ThinkingLevel = "low"
	ThinkingLevelMedium  ThinkingLevel = "medium"
	ThinkingLevelHigh    ThinkingLevel = "high"
	ThinkingLevelXHigh   ThinkingLevel = "xhigh"
	ThinkingLevelMax     ThinkingLevel = "max"
)

var thinkingLevels = [...]ThinkingLevel{
	ThinkingLevelDefault,
	ThinkingLevelOff,
	ThinkingLevelLow,
	ThinkingLevelMedium,
	ThinkingLevelHigh,
	ThinkingLevelXHigh,
	ThinkingLevelMax,
}

// ThinkingLevels returns the complete canonical vocabulary in UI order.
func ThinkingLevels() []ThinkingLevel {
	return append([]ThinkingLevel(nil), thinkingLevels[:]...)
}

// NormalizeThinkingLevel accepts common human/provider spellings while keeping
// Denova's persisted and adapter-facing vocabulary stable.
func NormalizeThinkingLevel(value string) ThinkingLevel {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", "-", " ", "-").Replace(normalized)
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	switch normalized {
	case "", "auto", "model-default", "provider-default":
		return ThinkingLevelDefault
	case "none", "disabled":
		return ThinkingLevelOff
	case "minimal", "light":
		return ThinkingLevelLow
	case "extra-high", "extra-high-effort":
		return ThinkingLevelXHigh
	case "maximum":
		return ThinkingLevelMax
	default:
		return ThinkingLevel(normalized)
	}
}

// ParseThinkingLevel validates a value before it crosses the provider registry
// seam. Unknown provider-specific tokens belong in an adapter, not callers.
func ParseThinkingLevel(value string) (ThinkingLevel, error) {
	level := NormalizeThinkingLevel(value)
	for _, candidate := range thinkingLevels {
		if level == candidate {
			return level, nil
		}
	}
	return "", fmt.Errorf("unsupported thinking level %q", value)
}

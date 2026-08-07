package providers

import "strings"

// ModelLimits contains documented limits for model identities whose provider
// defaults are too small or ambiguous for Denova's tool-heavy workloads.
// Unknown and custom models intentionally stay unbounded here so their
// endpoint or explicit profile configuration remains authoritative.
type ModelLimits struct {
	MaxOutputTokens int
}

const deepSeekV4MaxOutputTokens = 384 * 1024

// LookupModelLimits returns capabilities only for model IDs whose limits are
// stable and documented by their first-party provider.
func LookupModelLimits(provider ProviderID, model string) (ModelLimits, bool) {
	provider = ProviderID(strings.ToLower(strings.TrimSpace(string(provider))))
	model = strings.ToLower(strings.TrimSpace(model))
	switch provider {
	case ProviderDeepSeek:
		switch model {
		case "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-chat", "deepseek-reasoner":
			// DeepSeek V4 has a 384K maximum output. deepseek-chat and
			// deepseek-reasoner are current compatibility aliases for V4 Flash.
			return ModelLimits{MaxOutputTokens: deepSeekV4MaxOutputTokens}, true
		}
	case ProviderAnthropic:
		if hasModelPrefix(model,
			"claude-fable-5",
			"claude-mythos-5",
			"claude-mythos-preview",
			"claude-opus-5",
			"claude-sonnet-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
		) {
			// Current Claude 5 models and the listed Claude 4.6+ models
			// support 128K synchronous Messages output. Older Claude 4.5
			// variants remain on the protocol's documented 64K fallback.
			return ModelLimits{MaxOutputTokens: 128 * 1024}, true
		}
	case ProviderMiniMax, ProviderMiniMaxCN:
		if strings.HasPrefix(model, "minimax-m2") {
			// MiniMax documents a 128K maximum output, including CoT, for
			// the M2 family. This supersedes the conservative 64K fallback
			// required by the generic Anthropic Messages adapter.
			return ModelLimits{MaxOutputTokens: 128 * 1024}, true
		}
	}
	return ModelLimits{}, false
}

func hasModelPrefix(model string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if model == prefix || strings.HasPrefix(model, prefix+"-") {
			return true
		}
	}
	return false
}

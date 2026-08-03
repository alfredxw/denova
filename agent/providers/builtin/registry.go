// Package builtin assembles Denova's built-in protocol adapters and optional
// provider presets. Presets improve setup ergonomics but never form an
// allowlist: any provider ID can use any registered protocol explicitly.
package builtin

import (
	"encoding/json"
	"fmt"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/anthropicmessages"
	"github.com/alfredxw/denova/agent/providers/protocols/googlegenerativeai"
	"github.com/alfredxw/denova/agent/providers/protocols/openaichatcompletions"
	"github.com/alfredxw/denova/agent/providers/protocols/openairesponses"
)

// NewRegistry returns a fully validated, independent built-in registry.
func NewRegistry() (*providers.Registry, error) {
	registry := providers.NewRegistry()
	for _, adapter := range []providers.ProtocolAdapter{
		openaichatcompletions.NewAdapter(),
		openairesponses.NewAdapter(),
		anthropicmessages.NewAdapter(),
		googlegenerativeai.NewAdapter(),
	} {
		if err := registry.RegisterProtocol(adapter); err != nil {
			return nil, fmt.Errorf("build provider registry: %w", err)
		}
	}

	presets, err := providerPresets()
	if err != nil {
		return nil, fmt.Errorf("build provider presets: %w", err)
	}
	for _, preset := range presets {
		if err := registry.RegisterProviderPreset(preset); err != nil {
			return nil, fmt.Errorf("build provider registry: %w", err)
		}
	}
	return registry, nil
}

func providerPresets() ([]providers.ProviderPreset, error) {
	openAIChat, err := protocolOptions(openaichatcompletions.Compatibility{})
	if err != nil {
		return nil, err
	}
	openAIResponses, err := protocolOptions(openairesponses.Compatibility{
		Store:                     openairesponses.StoreModeFalse,
		IncludeEncryptedReasoning: true,
		ReasoningSummary:          openairesponses.ReasoningSummaryAuto,
	})
	if err != nil {
		return nil, err
	}
	deepSeekChat, err := protocolOptions(openaichatcompletions.Compatibility{
		ThinkingToggle:        openaichatcompletions.ThinkingToggleNested,
		ReasoningReplay:       openaichatcompletions.ReasoningReplayToolCalls,
		ReasoningContentField: "reasoning_content",
		EffortMap: map[string]string{
			"off":     "",
			"minimal": "low",
			"medium":  "high",
		},
	})
	if err != nil {
		return nil, err
	}
	deepSeekResponses, err := protocolOptions(openairesponses.Compatibility{
		Store:            openairesponses.StoreModeOmit,
		ReasoningSummary: openairesponses.ReasoningSummaryOmit,
		EffortMap: map[string]string{
			"minimal": "low",
			"medium":  "high",
		},
	})
	if err != nil {
		return nil, err
	}
	supportsEffort := false
	minimaxChat, err := protocolOptions(openaichatcompletions.Compatibility{
		ThinkingToggle:          openaichatcompletions.ThinkingToggleAdaptive,
		SupportsReasoningEffort: &supportsEffort,
		ReasoningContentField:   "reasoning_content",
		ReasoningReplay:         openaichatcompletions.ReasoningReplayToolCalls,
		MaxTokensField:          openaichatcompletions.MaxTokensFieldMaxCompletionTokens,
		RepairInlineThinking:    true,
		RequestReasoningSplit:   true,
	})
	if err != nil {
		return nil, err
	}
	minimaxAnthropic, err := protocolOptions(anthropicmessages.Compatibility{
		ThinkingMode:           anthropicmessages.ThinkingModeAdaptive,
		SupportsEffort:         &supportsEffort,
		DefaultMaxOutputTokens: 65536,
	})
	if err != nil {
		return nil, err
	}
	anthropicNative, err := protocolOptions(anthropicmessages.Compatibility{
		ThinkingMode:           anthropicmessages.ThinkingModeAdaptive,
		DefaultMaxOutputTokens: 65536,
		EffortMap: map[string]string{
			"minimal": "low",
		},
	})
	if err != nil {
		return nil, err
	}
	deepSeekAnthropic, err := protocolOptions(anthropicmessages.Compatibility{
		ThinkingMode:           anthropicmessages.ThinkingModeBudget,
		DefaultThinkingBudget:  8192,
		DefaultMaxOutputTokens: 65536,
		EffortMap: map[string]string{
			"minimal": "low",
			"medium":  "high",
		},
	})
	if err != nil {
		return nil, err
	}
	anthropicCompatible, err := protocolOptions(anthropicmessages.Compatibility{
		ThinkingMode:           anthropicmessages.ThinkingModeNone,
		DefaultMaxOutputTokens: 65536,
	})
	if err != nil {
		return nil, err
	}
	googleNative, err := protocolOptions(googlegenerativeai.Compatibility{
		APIVersion:   "v1beta",
		ThinkingMode: googlegenerativeai.ThinkingModeLevel,
		ThinkingLevels: map[string]string{
			"minimal": "MINIMAL",
			"low":     "LOW",
			"medium":  "MEDIUM",
			"high":    "HIGH",
			"xhigh":   "HIGH",
			"max":     "HIGH",
		},
	})
	if err != nil {
		return nil, err
	}
	supportsStreamUsage := false
	veniceChat, err := protocolOptions(openaichatcompletions.Compatibility{
		SupportsStreamUsage: &supportsStreamUsage,
	})
	if err != nil {
		return nil, err
	}

	chatEndpoint := func(baseURL string) map[providers.ProtocolID]providers.EndpointPreset {
		return map[providers.ProtocolID]providers.EndpointPreset{
			providers.ProtocolOpenAIChatCompletions: {BaseURL: baseURL, ProtocolOptions: openAIChat},
		}
	}
	return []providers.ProviderPreset{
		{
			ID:              providers.ProviderOpenAI,
			Name:            "OpenAI",
			DefaultProtocol: providers.ProtocolOpenAIResponses,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIResponses:       {BaseURL: "https://api.openai.com/v1", ProtocolOptions: openAIResponses},
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://api.openai.com/v1", ProtocolOptions: openAIChat},
			},
		},
		{
			ID:              providers.ProviderDeepSeek,
			Name:            "DeepSeek",
			DefaultProtocol: providers.ProtocolOpenAIChatCompletions,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://api.deepseek.com", ProtocolOptions: deepSeekChat},
				providers.ProtocolOpenAIResponses:       {BaseURL: "https://api.deepseek.com", ProtocolOptions: deepSeekResponses},
				providers.ProtocolAnthropicMessages:     {BaseURL: "https://api.deepseek.com/anthropic", ProtocolOptions: deepSeekAnthropic},
			},
		},
		{
			ID:              "anthropic",
			Name:            "Anthropic",
			DefaultProtocol: providers.ProtocolAnthropicMessages,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolAnthropicMessages: {BaseURL: "https://api.anthropic.com", ProtocolOptions: anthropicNative},
			},
		},
		{
			ID:              "google",
			Name:            "Google Gemini",
			DefaultProtocol: providers.ProtocolGoogleGenerativeAI,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolGoogleGenerativeAI:    {BaseURL: "https://generativelanguage.googleapis.com", ProtocolOptions: googleNative},
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", ProtocolOptions: openAIChat},
			},
		},
		{
			ID:              providers.ProviderOpenAICompatible,
			Name:            "OpenAI Compatible",
			DefaultProtocol: providers.ProtocolOpenAIChatCompletions,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIChatCompletions: {ProtocolOptions: openAIChat},
				providers.ProtocolOpenAIResponses:       {},
			},
		},
		{ID: "groq", Name: "Groq", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.groq.com/openai/v1")},
		{ID: "cerebras", Name: "Cerebras", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.cerebras.ai/v1")},
		{ID: "huggingface", Name: "Hugging Face", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://router.huggingface.co/v1")},
		{ID: "together", Name: "Together AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.together.xyz/v1")},
		{ID: "nvidia", Name: "NVIDIA NIM", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://integrate.api.nvidia.com/v1")},
		{ID: "novita", Name: "Novita AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.novita.ai/openai/v1")},
		{ID: "xai", Name: "xAI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.x.ai/v1")},
		{ID: "mistral", Name: "Mistral AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.mistral.ai/v1")},
		{ID: "fireworks", Name: "Fireworks AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.fireworks.ai/inference/v1")},
		{ID: "openrouter", Name: "OpenRouter", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://openrouter.ai/api/v1")},
		{ID: "moonshot", Name: "Moonshot AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.moonshot.ai/v1")},
		{ID: "alibaba", Name: "Alibaba Cloud Model Studio", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://coding-intl.dashscope.aliyuncs.com/v1")},
		{ID: "zhipu", Name: "Zhipu AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://open.bigmodel.cn/api/coding/paas/v4")},
		{ID: "siliconflow", Name: "SiliconFlow", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.siliconflow.cn/v1")},
		{ID: "volcengine", Name: "Volcengine Ark", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://ark.cn-beijing.volces.com/api/v3")},
		{ID: "synthetic", Name: "Synthetic", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://api.synthetic.new/openai/v1")},
		{ID: "baseten", Name: "Baseten", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://inference.baseten.co/v1")},
		{ID: "nanogpt", Name: "NanoGPT", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("https://nano-gpt.com/api/v1")},
		{ID: "ollama", Name: "Ollama", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("http://127.0.0.1:11434/v1")},
		{ID: "lm-studio", Name: "LM Studio", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("http://127.0.0.1:1234/v1")},
		{ID: "litellm", Name: "LiteLLM", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("http://127.0.0.1:4000/v1")},
		{ID: "vllm", Name: "vLLM", DefaultProtocol: providers.ProtocolOpenAIChatCompletions, Endpoints: chatEndpoint("http://127.0.0.1:8000/v1")},
		{
			ID: "venice", Name: "Venice AI", DefaultProtocol: providers.ProtocolOpenAIChatCompletions,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://api.venice.ai/api/v1", ProtocolOptions: veniceChat},
			},
		},
		{
			ID:              "minimax",
			Name:            "MiniMax",
			DefaultProtocol: providers.ProtocolAnthropicMessages,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://api.minimax.io/v1", ProtocolOptions: minimaxChat},
				providers.ProtocolAnthropicMessages:     {BaseURL: "https://api.minimax.io/anthropic", ProtocolOptions: minimaxAnthropic},
			},
		},
		{
			ID:              "minimax-cn",
			Name:            "MiniMax China",
			DefaultProtocol: providers.ProtocolAnthropicMessages,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolOpenAIChatCompletions: {BaseURL: "https://api.minimaxi.com/v1", ProtocolOptions: minimaxChat},
				providers.ProtocolAnthropicMessages:     {BaseURL: "https://api.minimaxi.com/anthropic", ProtocolOptions: minimaxAnthropic},
			},
		},
		{
			ID:              "zai",
			Name:            "Z.AI",
			DefaultProtocol: providers.ProtocolAnthropicMessages,
			Endpoints: map[providers.ProtocolID]providers.EndpointPreset{
				providers.ProtocolAnthropicMessages: {BaseURL: "https://api.z.ai/api/anthropic", ProtocolOptions: anthropicCompatible},
			},
		},
	}, nil
}

func protocolOptions(value any) (json.RawMessage, error) {
	options, err := providers.EncodeProtocolOptions(value)
	if err != nil {
		return nil, err
	}
	return options, nil
}

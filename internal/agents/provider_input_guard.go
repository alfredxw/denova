package agents

import (
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// ProviderInputLimitError is returned before a provider sees an input that
// exceeds the non-disableable complete-context safety boundary.
type ProviderInputLimitError struct {
	AgentKind string
	Bytes     int
	MaxBytes  int
	Tokens    int
	MaxTokens int
}

func (e *ProviderInputLimitError) Error() string {
	return fmt.Sprintf("provider input exceeds hard context limit: agent=%s bytes=%d/%d estimated_tokens=%d/%d", e.AgentKind, e.Bytes, e.MaxBytes, e.Tokens, e.MaxTokens)
}

func validateProviderInput(agentKind string, messages []*agent.Message, tools []*agent.ToolInfo, maxBytes, maxTokens int) error {
	if maxBytes <= 0 {
		maxBytes = config.DefaultAgentContextMaxProviderInputBytes
	}
	payload, err := json.Marshal(struct {
		Messages []*agent.Message  `json:"messages"`
		Tools    []*agent.ToolInfo `json:"tools,omitempty"`
	}{Messages: messages, Tools: tools})
	if err != nil {
		return fmt.Errorf("serialize provider input for hard-limit validation: %w", err)
	}
	tokens := EstimateContextTokens(messages, tools)
	if len(payload) <= maxBytes && (maxTokens <= 0 || tokens <= maxTokens) {
		return nil
	}
	return &ProviderInputLimitError{
		AgentKind: agentKind, Bytes: len(payload), MaxBytes: maxBytes,
		Tokens: tokens, MaxTokens: maxTokens,
	}
}

// validateConfiguredProviderInput applies the same final provider boundary to
// standalone model-only agents that do not pass through Agent middleware. Every
// provider call must validate the complete serialized request at its last host
// boundary; upstream prompt builders being bounded is useful but insufficient.
func validateConfiguredProviderInput(cfg *config.Config, agentKind string, messages []*agent.Message, tools []*agent.ToolInfo) error {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return validateProviderInput(agentKind, messages, tools, contextSettings.MaxProviderInputBytes, modelSettings.ContextWindowTokens)
}

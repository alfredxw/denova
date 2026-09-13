package modelio

import (
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

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

func ValidateInput(agentKind string, model providers.ModelConfig, messages []*agent.Message, tools []*agent.ToolInfo, maxBytes, maxTokens int) error {
	if maxBytes <= 0 {
		maxBytes = config.DefaultAgentContextMaxProviderInputBytes
	}
	size, err := model.InputEstimator().Estimate(messages, tools)
	if err != nil {
		return err
	}
	if size.Bytes <= maxBytes && (maxTokens <= 0 || size.Tokens <= maxTokens) {
		return nil
	}
	return &ProviderInputLimitError{
		AgentKind: agentKind, Bytes: size.Bytes, MaxBytes: maxBytes,
		Tokens: size.Tokens, MaxTokens: maxTokens,
	}
}

// ValidateConfiguredInput applies the same final provider boundary to
// standalone model-only agents that do not pass through Agent middleware. Every
// provider call must validate the complete serialized request at its last host
// boundary; upstream prompt builders being bounded is useful but insufficient.
func ValidateConfiguredInput(cfg *config.Config, agentKind string, messages []*agent.Message, tools []*agent.ToolInfo) error {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	model, err := ConfigFromResolved(modelSettings)
	if err != nil {
		return err
	}
	return ValidateInput(agentKind, model, messages, tools, contextSettings.MaxProviderInputBytes, modelSettings.ContextWindowTokens)
}

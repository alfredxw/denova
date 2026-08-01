package modelio

import (
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestProviderHardLimitRejectsLongHistoryWhenSemanticCompactionIsDisabled(t *testing.T) {
	disabled := false
	maxBytes := 32 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
		CompactionEnabled: &disabled, MaxProviderInputBytes: &maxBytes,
	}}}
	resolved := config.ResolveAgentContext(cfg, config.AgentKindIDE)
	if resolved.CompactionEnabled {
		t.Fatal("test requires user-controlled semantic compaction to be disabled")
	}
	messages := []*agent.Message{agent.UserMessage(strings.Repeat("历史正文。", maxBytes))}
	err := ValidateInput(config.AgentKindIDE, messages, nil, resolved.MaxProviderInputBytes, config.ResolveAgentModel(cfg, config.AgentKindIDE).ContextWindowTokens)
	var limitErr *ProviderInputLimitError
	if !errors.As(err, &limitErr) || limitErr.Bytes <= limitErr.MaxBytes {
		t.Fatalf("complete provider input was not rejected by the non-disableable hard limit: %v", err)
	}
}

func TestStandaloneProviderBoundaryUsesResolvedAgentLimit(t *testing.T) {
	maxBytes := 32 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{Automation: config.AgentContextOverride{
		MaxProviderInputBytes: &maxBytes,
	}}}
	messages := []*agent.Message{
		agent.SystemMessage("bounded standalone agent"),
		agent.UserMessage(strings.Repeat("语义触发证据。", maxBytes)),
	}
	err := ValidateConfiguredInput(cfg, config.AgentKindAutomation, messages, nil)
	var limitErr *ProviderInputLimitError
	if !errors.As(err, &limitErr) || limitErr.AgentKind != config.AgentKindAutomation || limitErr.MaxBytes != maxBytes {
		t.Fatalf("standalone provider boundary = %#v, err=%v", limitErr, err)
	}
}

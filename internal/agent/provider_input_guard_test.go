package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	adk "github.com/alfredxw/denova/adk"

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
	messages := []*adk.Message{adk.UserMessage(strings.Repeat("历史正文。", maxBytes))}
	err := validateProviderInput(config.AgentKindIDE, messages, nil, resolved.MaxProviderInputBytes, config.ResolveAgentModel(cfg, config.AgentKindIDE).ContextWindowTokens)
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
	messages := []*adk.Message{
		adk.SystemMessage("bounded standalone agent"),
		adk.UserMessage(strings.Repeat("语义触发证据。", maxBytes)),
	}
	err := validateConfiguredProviderInput(cfg, config.AgentKindAutomation, messages, nil)
	var limitErr *ProviderInputLimitError
	if !errors.As(err, &limitErr) || limitErr.AgentKind != config.AgentKindAutomation || limitErr.MaxBytes != maxBytes {
		t.Fatalf("standalone provider boundary = %#v, err=%v", limitErr, err)
	}
}

func TestCompactionSummarizerLayersOversizedSourceWithoutDroppingBytes(t *testing.T) {
	maxBytes := 48 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{ContextCompaction: config.AgentContextOverride{
		MaxProviderInputBytes: &maxBytes,
	}}}
	original := summarizeContextForCompaction
	t.Cleanup(func() { summarizeContextForCompaction = original })
	calls := 0
	observedBytes := 0
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, checkpoint string, source []*adk.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		calls++
		batchBytes := 0
		for _, message := range source {
			if message != nil {
				batchBytes += len(message.Content)
			}
		}
		if batchBytes > maxBytes/2 {
			t.Fatalf("layer payload bytes=%d exceeds bounded batch allowance", batchBytes)
		}
		observedBytes += batchBytes
		return checkpoint + strings.Repeat("摘要", 32), batchBytes, nil
	}
	payload := strings.Repeat("不可丢失的历史事实。", 2500)
	messages := []*adk.Message{
		adk.UserMessage(payload), adk.AssistantMessage(payload, nil), adk.UserMessage(payload),
	}
	_, result, err := PrepareContextCompaction(context.Background(), cfg, config.AgentKindIDE, ContextCompactionInput{
		Messages: messages, Force: true, KeepLatestUser: true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 || !result.Triggered {
		t.Fatalf("oversized source was not summarized hierarchically: calls=%d result=%#v", calls, result)
	}
	if observedBytes < len(payload)*3 {
		t.Fatalf("layered summarization silently dropped source bytes: observed=%d source=%d", observedBytes, len(payload)*3)
	}
}

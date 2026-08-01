package chat

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
)

func TestCompactionSummarizerLayersOversizedSourceWithoutDroppingBytes(t *testing.T) {
	maxBytes := 48 * 1024
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{ContextCompaction: config.AgentContextOverride{
		MaxProviderInputBytes: &maxBytes,
	}}}
	calls := 0
	observedBytes := 0
	summarize := func(_ context.Context, _ *config.Config, request agentcompaction.SummaryRequest, _ func(int, string)) (string, error) {
		calls++
		batchBytes := 0
		for _, message := range request.Messages {
			if message != nil {
				batchBytes += len(message.Content)
			}
		}
		observedBytes += batchBytes
		return strings.Repeat("摘要", 32), nil
	}
	payload := strings.Repeat("不可丢失的历史事实。", 2500)
	messages := []*agent.Message{
		agent.UserMessage(payload), agent.AssistantMessage(payload, nil), agent.UserMessage(payload),
	}
	_, result, err := agentcompaction.Prepare(context.Background(), cfg, config.AgentKindIDE, coldCompactionTestInput(agentcompaction.Input{
		Messages: messages, Force: true, KeepLatestUser: true,
	}, summarize), 1)
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

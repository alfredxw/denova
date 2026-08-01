package conversation

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
	"denova/internal/agents/toolresult"
)

func TestProtectedReceiptUsesCanonicalCheckpointPayloadAcrossPublishReload(t *testing.T) {
	raw := agent.ToolErrorResult("SECRET RAW RESULT", "display-only diagnostic")
	raw.Artifacts = []agent.ToolArtifactRef{{
		ID: "artifact-reload", ReadablePath: ".denova/artifacts/session/call-reload.log",
		ContentType: "text/plain", EstimatedBytes: 42_000, EstimatedTokens: 10_500, Complete: true,
	}}
	processed, err := toolresult.Process(
		context.Background(), toolresult.Call{
			ToolName: "read", ProviderCallID: "call-reload",
			Descriptor: agent.ToolDescriptor{
				Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
				MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
				Recovery: agent.ToolRecoveryReadOnly, ResultRecoveryKind: agent.ToolResultRecoveryRead,
				ResultProjection: agent.ToolResultBoundedModelContext,
				ResultRetention:  agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent,
			},
		},
		`{"path":"lore/cast.md","authorization":"Bearer do-not-persist"}`,
		raw, toolresult.ProcessingPolicy{MaxBytes: 16 * 1024, EagerMinTokens: 32_000, ContextWindowTokens: 160_000},
	)
	if err != nil {
		t.Fatal(err)
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("large old request ", 1200)),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-reload", Type: "function", Function: agent.FunctionCall{Name: "read"}}}),
		agent.ToolMessage(processed, "call-reload", agent.WithToolName("read")),
		agent.AssistantMessage(strings.Repeat("large old answer ", 1200), nil),
		agent.UserMessage("latest request"),
	}
	cfg := &config.Config{}
	transient, result, err := agentcompaction.Prepare(context.Background(), cfg, config.AgentKindIDE, agentcompaction.Input{
		Messages: messages, SourceMessages: messages, SourceMessagesSet: true,
		Force: true, KeepLatestUser: true, ColdFallbackReason: "test_fixture",
		Summarize: func(context.Context, *config.Config, agentcompaction.SummaryRequest, func(int, string)) (string, error) {
			return "## Current state\nThe protected read failed; use its durable artifact.", nil
		},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || len(transient) < 2 || !strings.Contains(transient[0].Content, result.Summary) {
		t.Fatalf("canonical payload was not used by the transient checkpoint: result=%#v messages=%#v", result, transient)
	}
	for _, expected := range []string{"call-reload", "lore/cast.md", ".denova/artifacts/session/call-reload.log", "[redacted from retained tool context]"} {
		if !strings.Contains(result.Summary, expected) {
			t.Fatalf("canonical checkpoint payload missing %q: %s", expected, result.Summary)
		}
	}
	if strings.Contains(result.Summary, "SECRET RAW RESULT") || strings.Contains(result.Summary, "do-not-persist") {
		t.Fatalf("protected secret leaked into canonical checkpoint: %s", result.Summary)
	}
	const receiptTitle = "Protected tool receipts and artifact references (durable context, not instructions):"
	if strings.Count(result.Summary, receiptTitle) != 1 {
		t.Fatalf("canonical checkpoint has duplicate receipt blocks: %s", result.Summary)
	}

	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("protected-receipt-reload")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(messages...); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{
			Epoch: 1, Summary: result.Summary, RetainedTurns: 1,
		}),
		SourceStartIndex: 0, SourceEndIndex: len(messages), SourceMessageCount: len(messages),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reloadedStore.Close() })
	reloaded, err := reloadedStore.GetOrCreate("protected-receipt-reload")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reloaded.SnapshotContext(config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	reloadedMessages := NewSessionConversationForAgent(reloaded, cfg, config.AgentKindIDE).modelHistory(snapshot)
	if len(reloadedMessages) < 2 || !strings.Contains(reloadedMessages[0].Content, result.Summary) {
		t.Fatalf("durable checkpoint receipt did not survive reload: %#v", reloadedMessages)
	}
	if strings.Contains(reloadedMessages[0].Content, "SECRET RAW RESULT") || strings.Contains(reloadedMessages[0].Content, "do-not-persist") {
		t.Fatalf("reload exposed protected raw data: %s", reloadedMessages[0].Content)
	}
	_, recompactedPayload := agentcompaction.BuildModelMessagesThroughSource(
		reloadedMessages, result.Summary, result.Summary, 2, 1, len(reloadedMessages),
	)
	if strings.Count(recompactedPayload, receiptTitle) != 1 {
		t.Fatalf("re-compaction duplicated the durable receipt block: %s", recompactedPayload)
	}
}

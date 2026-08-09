package conversation

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

type compactionForkCaptureModel struct {
	response *agent.Message
	inputs   [][]*agent.Message
	options  []*agent.Options
	streams  int
	requests int
}

func (model *compactionForkCaptureModel) Generate(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	model.capture(input, opts)
	if model.response == nil {
		return nil, io.EOF
	}
	return model.response.Clone(), nil
}

func (model *compactionForkCaptureModel) Stream(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	model.capture(input, opts)
	model.streams++
	if model.response == nil {
		return agent.StreamReaderFromArray([]*agent.Message{}), nil
	}
	return agent.StreamReaderFromArray([]*agent.Message{model.response.Clone()}), nil
}

func (model *compactionForkCaptureModel) capture(input []*agent.Message, opts []agent.ModelOption) {
	model.requests++
	messages := make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			messages[index] = message.Clone()
		}
	}
	model.inputs = append(model.inputs, messages)
	model.options = append(model.options, agent.GetCommonOptions(nil, opts...))
}

func TestSessionCompactionMapsDynamicFinalUserAndPostToolBatchToPrimaryFork(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("dynamic-compaction-fork")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(
		agent.UserMessage(strings.Repeat("large prior request ", 900)),
		agent.AssistantMessage(strings.Repeat("large prior answer ", 900), nil),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgentWithRuntimeContext(
		sess, &config.Config{}, config.AgentKindIDE,
		"dynamic workspace state", "chapter cursor is after the city gate",
	)
	assembled, err := assembleAndCommitModelContextForTest(conversation, "continue", "continue")
	if err != nil {
		t.Fatal(err)
	}
	callMessage := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-post-assembly", Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`},
	}})
	toolMessage := agent.ToolMessage(agent.TextToolResult("chapter evidence"), "call-post-assembly", agent.WithToolName("read"))
	if err := conversation.AppendContextMessages(callMessage, toolMessage); err != nil {
		t.Fatal(err)
	}
	primary := append(agentcontext.CloneMessages(assembled), callMessage.Clone(), toolMessage.Clone())
	source, _, _, _, err := conversation.compactionIncrementalSource(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !containsCompactionMessageContent(source, "continue") || containsCompactionMessageContent(source, "chapter cursor is after the city gate") {
		t.Fatalf("canonical source absorbed the dynamic wrapper: %#v", messageContents(source))
	}

	model := &compactionForkCaptureModel{response: agent.AssistantMessage("## Goal\nContinue the chapter.\n\n## Current state\nThe city gate was reached.", nil)}
	request := &agent.ModelCall{Model: model, Messages: primary}
	compacted, result, err := conversation.CompactContextIfNeeded(context.Background(), agentcompaction.Input{
		Messages: primary, Force: true, KeepLatestUser: true, PrimaryRequestSnapshot: request.Snapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != agentcompaction.ExecutionCacheSafeFork || model.requests != 1 {
		t.Fatalf("compaction did not use one cache-safe fork: result=%#v requests=%d", result, model.requests)
	}
	if got := model.inputs[0][:len(primary)]; !reflect.DeepEqual(got, primary) {
		t.Fatalf("cache-safe fork changed primary prefix:\ngot=%#v\nwant=%#v", got, primary)
	}
	if len(compacted) == 0 || !agentcontext.IsCompactionSummaryMessage(compacted[0]) {
		t.Fatalf("missing transient checkpoint: %#v", compacted)
	}
}

func TestSessionManualCompactionMapsNormalizedLegacyProtocolToPrimaryFork(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("normalized-legacy-compaction-fork")
	if err != nil {
		t.Fatal(err)
	}
	legacyUser := agent.UserMessage(strings.Repeat("old request ", 600))
	legacyUser.ToolCallID = "legacy-malformed-user-result"
	legacyUser.ToolName = "read"
	if err := sess.AppendContextMessages(
		legacyUser,
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("current request"),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	snapshot, err := sess.SnapshotContext(config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	canonical := conversation.modelHistory(snapshot)
	normalized, err := agentcontext.NormalizeModelContextMessages(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != len(canonical) || canonical[0].ToolCallID == "" || normalized[0].ToolCallID != "" || normalized[0].ToolName != "" {
		t.Fatalf("legacy fixture was not repaired as expected: %#v", normalized)
	}
	primary := append([]*agent.Message{agent.SystemMessage("stable system")}, normalized...)
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("## Goal\nContinue the current request.", nil)}
	request := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}
	compacted, result, err := conversation.CompactContextIfNeeded(context.Background(), agentcompaction.Input{
		Messages: canonical, Force: true, KeepLatestUser: true, PrimaryRequestSnapshot: request.Snapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != agentcompaction.ExecutionCacheSafeFork || model.requests != 1 {
		t.Fatalf("normalized manual compaction = result:%#v requests:%d", result, model.requests)
	}
	if got := model.inputs[0][:len(primary)]; !reflect.DeepEqual(got, primary) {
		t.Fatalf("normalized provider prefix changed:\ngot=%#v\nwant=%#v", got, primary)
	}
	if _, err := agentcontext.NormalizeModelContextMessages(compacted); err != nil {
		t.Fatalf("compacted legacy context is invalid: %v\n%#v", err, compacted)
	}
	if result.TokensAfter != agentcontext.EstimateTokens(compacted, nil) {
		t.Fatalf("post-normalizer token accounting = %d, want %d", result.TokensAfter, agentcontext.EstimateTokens(compacted, nil))
	}
}

func containsCompactionMessageContent(messages []*agent.Message, content string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

package interactiveapp

import (
	"context"
	"denova/internal/agents/toolresult"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

func TestInteractiveProviderContinuationSurvivesTurnCommitAndReload(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "provider continuation", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	modelConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5.6", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(modelConfig, []json.RawMessage{
		json.RawMessage(`{"id":"reasoning_1","type":"reasoning","encrypted_content":"encrypted-state","summary":[]}`),
		json.RawMessage(`{"id":"message_1","type":"message","status":"completed","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"门开了。","annotations":[]}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "开门", Narrative: "门开了。",
		ProviderContinuation: map[string]any{providers.ExtraKeyContinuation: continuation},
	})
	if err != nil {
		t.Fatal(err)
	}

	reloaded := interactive.NewStore(workspace)
	history, err := reloaded.ReadModelHistory(story.ID, interactive.StoryModelHistoryQuery{
		BranchID: "main", StartTurn: 0, EndTurn: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := BuildModelContextProjection(history, nil, interactive.Snapshot{}, toolresult.ContextPolicy{}, agentrun.CycleIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || projection.Messages[1].Role != agents.RoleAssistant {
		t.Fatalf("projected messages = %#v, want user and assistant", projection.Messages)
	}
	var items []json.RawMessage
	matched, err := providers.DecodeContinuation(projection.Messages[1].Extra, modelConfig, &items)
	if err != nil || !matched || len(items) != 2 {
		t.Fatalf("reloaded continuation items = %#v matched=%t err=%v", items, matched, err)
	}
	if !strings.Contains(string(items[0]), "encrypted-state") || !strings.Contains(string(items[1]), `"phase":"final_answer"`) {
		t.Fatalf("reloaded continuation changed Responses output items: %s", items)
	}
	snapshot, err := reloaded.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	publicJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "encrypted-state") || strings.Contains(string(publicJSON), providers.ExtraKeyContinuation) {
		t.Fatalf("opaque provider continuation leaked into public Game snapshot: %s", publicJSON)
	}
}

func TestInteractiveToolResultSummaryRoundTripsThroughStorySchema(t *testing.T) {
	message := &agents.Message{
		Role:       agents.RoleTool,
		Content:    "bounded rich result",
		Name:       "read result",
		ToolCallID: "call-read-1",
		ToolName:   "read",
		ToolResult: &agent.ToolResultSummary{
			Status:           agent.ToolResultError,
			SyntheticReason:  agent.ToolSyntheticEffectUnknown,
			ModelTruncated:   true,
			DisplayTruncated: true,
			ResultRetention:  agent.ToolResultProtected,
			ContextHints: &agent.ToolResultContextHints{
				Recovery: agent.ToolResultRecoveryHint{
					Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "chapters/one.md", "start_line": float64(10)},
					ArtifactPath: ".denova/artifacts/story/call-read-1.log", EstimatedBytes: 64_000, EstimatedTokens: 16_000,
				},
				ContextValue: agent.ToolResultContextDiscardable, SupersessionKey: "read:chapters/one.md",
			},
			ArtifactPersistence: &agent.ToolArtifactPersistence{Attempted: true, Complete: false, FailureReason: agent.ToolArtifactFailureWrite},
			Artifacts: []agent.ToolArtifactRef{{
				ID: "artifact-1", ReadablePath: ".denova/artifacts/story/call-read-1.log", ContentType: "text/plain",
				EstimatedBytes: 64_000, EstimatedTokens: 16_000, Complete: true,
			}},
		},
	}

	stored, ok := interactiveContextMessageFromSchema(message)
	if !ok {
		t.Fatal("tool result was rejected from story model context")
	}
	rehydrated := schemaMessagesFromInteractiveContext([]interactive.ModelContextMessage{stored})
	if len(rehydrated) != 1 {
		t.Fatalf("rehydrated messages = %d, want 1", len(rehydrated))
	}
	if !reflect.DeepEqual(rehydrated[0], message) {
		t.Fatalf("tool result metadata did not round trip:\nwant=%#v\ngot=%#v", message, rehydrated[0])
	}

	stored.ToolResult.ContextHints.Recovery.Reference["path"] = "mutated"
	if got := message.ToolResult.ContextHints.Recovery.Reference["path"]; got != "chapters/one.md" {
		t.Fatalf("story conversion aliased the source recovery hint: %v", got)
	}
}

func TestInteractiveMultiToolBatchesRoundTripWithProviderLocalIDReuse(t *testing.T) {
	first := agents.AssistantMessage("", []agents.ToolCall{
		{ID: "provider-local", Type: "function", Function: agents.FunctionCall{Name: "read_lore_items", Arguments: `{"ids":["one"]}`}},
		{ID: "parallel", Type: "function", Function: agents.FunctionCall{Name: "search_story_history", Arguments: `{"query":"gate"}`}},
	})
	second := agents.AssistantMessage("", []agents.ToolCall{{
		ID: "provider-local", Type: "function", Function: agents.FunctionCall{Name: "read_lore_items", Arguments: `{"ids":["two"]}`},
	}})
	source := []*agents.Message{
		first,
		agents.ToolMessage(agent.TextToolResult("lore one"), "provider-local"),
		agents.ToolMessage(agent.TextToolResult("history"), "parallel"),
		second,
		agents.ToolMessage(agent.TextToolResult("lore two"), "provider-local"),
	}

	stored := make([]interactive.ModelContextMessage, 0, len(source))
	for _, message := range source {
		converted, ok := interactiveContextMessageFromSchema(message)
		if !ok {
			t.Fatalf("game context rejected valid tool batch message: %#v", message)
		}
		stored = append(stored, converted)
	}
	rehydrated := schemaMessagesFromInteractiveContext(stored)
	rehydrated = toolresult.ApplyContextPolicy(rehydrated, toolresult.ContextPolicy{Enabled: true})
	if len(rehydrated) != 5 || len(rehydrated[0].ToolCalls) != 2 ||
		rehydrated[0].ToolCalls[0].ID != "provider-local" || rehydrated[0].ToolCalls[1].ID != "parallel" ||
		rehydrated[1].ToolCallID != "provider-local" || rehydrated[1].Content != "lore one" ||
		rehydrated[2].ToolCallID != "parallel" || rehydrated[2].Content != "history" ||
		len(rehydrated[3].ToolCalls) != 1 || rehydrated[3].ToolCalls[0].ID != "provider-local" ||
		rehydrated[4].ToolCallID != "provider-local" || rehydrated[4].Content != "lore two" {
		t.Fatalf("game rich tool batches did not round trip atomically: %#v", rehydrated)
	}
}

func TestInteractiveToolBatchSurvivesFailedCycleAndReload(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "tool failure recovery", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	failed := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "检查门", 800, nil)
	identity := agentrun.CycleIdentity{CommandID: "command-fetch-failed", OperationID: "operation-fetch-failed", Cycle: 1}
	failed.BindAgentCycleIdentity(identity)
	materializeInteractiveInputForTest(t, failed, identity)
	batch := durableInteractiveToolBatchFixture()
	if err := failed.AppendContextMessages(batch...); err != nil {
		t.Fatal(err)
	}
	// No assistant narrative is committed: this is the provider-failure/cancel
	// boundary that previously lost the completed tool evidence.

	reloadedStore := interactive.NewStore(workspace)
	next := NewConversation(reloadedStore, t.TempDir(), workspace, story.ID, "main", "继续", 800, nil)
	messages, err := assembleAndCommitInteractiveContextForTest(next, "继续", "继续")
	if err != nil {
		t.Fatal(err)
	}
	var interrupted, assistant, result bool
	for _, message := range messages {
		if message == nil {
			continue
		}
		interrupted = interrupted || strings.Contains(message.Content, "检查门") && strings.Contains(message.Content, "interrupted turn")
		assistant = assistant || message.Role == agents.RoleAssistant && message.Content == "I will fetch the gate record." && len(message.ToolCalls) == 1
		result = result || message.Role == agents.RoleTool && message.ToolCallID == "call-fetch" && message.Content == "The gate was opened recently."
	}
	if !interrupted || !assistant || !result {
		t.Fatalf("reloaded context lost failed-cycle evidence: interrupted=%t assistant=%t result=%t messages=%#v", interrupted, assistant, result, messages)
	}
}

func TestInteractiveSuccessfulTurnConsumesDurableToolBatchWithoutDuplicate(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "tool success", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "检查门", 800, nil)
	identity := agentrun.CycleIdentity{CommandID: "command-fetch-success", OperationID: "operation-fetch-success", Cycle: 1}
	conversation.BindAgentCycleIdentity(identity)
	materializeInteractiveInputForTest(t, conversation, identity)
	batch := durableInteractiveToolBatchFixture()
	if err := conversation.AppendContextMessages(batch...); err != nil {
		t.Fatal(err)
	}
	submitTestTurnResult(t, conversation, "检查门", "确认脚印")
	if err := conversation.AppendAssistant("门后留有新鲜脚印。"); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || len(snapshot.Turns[0].ModelContextMessages) != 2 {
		t.Fatalf("durable batch was lost or duplicated in Turn: %#v", snapshot.Turns)
	}
	if snapshot.Turns[0].ModelContextMessages[0].Content != "I will fetch the gate record." {
		t.Fatalf("assistant tool-call content was not retained: %#v", snapshot.Turns[0].ModelContextMessages)
	}
	if len(snapshot.PendingModelContextBatches) != 0 {
		t.Fatalf("successful Turn left a pending side batch: %#v", snapshot.PendingModelContextBatches)
	}
}

func durableInteractiveToolBatchFixture() []*agents.Message {
	assistant := agents.AssistantMessage("I will fetch the gate record.", []agents.ToolCall{{
		ID: "call-fetch", Type: "function",
		Function: agents.FunctionCall{Name: "web_fetch", Arguments: `{"url":"https://example.test/gate"}`},
	}})
	result := agents.ToolMessage(agent.TextToolResult("The gate was opened recently."), "call-fetch", agents.WithToolName("web_fetch"))
	result.ToolResult = &agent.ToolResultSummary{
		Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultEagerCandidate,
		Artifacts: []agent.ToolArtifactRef{{ReadablePath: ".denova/artifacts/game/fetch.txt", EstimatedBytes: 4096, Complete: true}},
	}
	return []*agents.Message{assistant, result}
}

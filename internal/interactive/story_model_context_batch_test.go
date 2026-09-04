package interactive

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestModelContextBatchPersistsWithoutAdvancingBranch(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "durable tool context", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-fetch", OperationID: "operation-fetch", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "查看门后的线索")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	before, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	messages := durableModelContextBatchFixture()
	modelConfig := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5.6", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(modelConfig, []json.RawMessage{
		json.RawMessage(`{"id":"reasoning_1","type":"reasoning","encrypted_content":"private-batch-state","summary":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	messages[0].ProviderContinuation, err = normalizeProviderContinuation(map[string]any{providers.ExtraKeyContinuation: continuation})
	if err != nil {
		t.Fatal(err)
	}
	intents, err := NewModelContextBatchIntents(identity, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	skipped := intents[0]
	skipped.Sequence = 1
	if _, err := store.AppendModelContextBatch(story.ID, skipped); !errors.Is(err, ErrModelContextBatchIdentityConflict) {
		t.Fatalf("non-contiguous sequence error = %v", err)
	}
	first, err := store.AppendModelContextBatch(story.ID, intents[0])
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.AppendModelContextBatch(story.ID, intents[0])
	if err != nil {
		t.Fatal(err)
	}
	if retry.Revision != first.Revision {
		t.Fatalf("exact retry revision = %q, want %q", retry.Revision, first.Revision)
	}
	conflicting := intents[0]
	conflicting.Messages = CloneModelContextMessages(conflicting.Messages)
	conflicting.Messages[1].Content = "different evidence"
	if _, err := store.AppendModelContextBatch(story.ID, conflicting); !errors.Is(err, ErrModelContextBatchIdentityConflict) {
		t.Fatalf("same sequence with different messages error = %v", err)
	}
	conflicting = intents[0]
	conflicting.Messages = CloneModelContextMessages(conflicting.Messages)
	conflicting.Messages[0].ProviderContinuation = map[string]any{}
	if _, err := store.AppendModelContextBatch(story.ID, conflicting); !errors.Is(err, ErrModelContextBatchIdentityConflict) {
		t.Fatalf("same sequence with different provider continuation error = %v", err)
	}
	after, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Meta.Branches["main"].Head != before.Meta.Branches["main"].Head {
		t.Fatalf("side event advanced branch head: before=%q after=%q", before.Meta.Branches["main"].Head, after.Meta.Branches["main"].Head)
	}
	if after.Snapshot.ContextRevision != before.Snapshot.ContextRevision+1 {
		t.Fatalf("context revision = %d, want %d", after.Snapshot.ContextRevision, before.Snapshot.ContextRevision+1)
	}
	assertPendingModelContextBatch(t, after.Snapshot, messages)

	reloaded := NewStore(workspace)
	reloadedContext, err := reloaded.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertPendingModelContextBatch(t, reloadedContext.Snapshot, messages)
	publicJSON, err := json.Marshal(reloadedContext.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "private-batch-state") || strings.Contains(string(publicJSON), providers.ExtraKeyContinuation) {
		t.Fatalf("private model-context continuation leaked into public Story JSON: %s", publicJSON)
	}
}

func TestModelContextBatchPersistsAcrossRecentSideEventWindow(t *testing.T) {
	for _, acceptBeforeSideEvents := range []bool{false, true} {
		for _, reopen := range []bool{false, true} {
			name := "head_before_window/warm_cache"
			if acceptBeforeSideEvents {
				name = "input_before_window/warm_cache"
			}
			if reopen {
				name = strings.Replace(name, "warm_cache", "cold_cache", 1)
			}
			t.Run(name, func(t *testing.T) {
				workspace := t.TempDir()
				store := NewStore(workspace)
				story, err := store.CreateStory(CreateStoryRequest{Title: "old active head", StoryTellerID: "classic"})
				if err != nil {
					t.Fatal(err)
				}
				turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
					BranchID: "main", User: "enter", Narrative: "The gate opens.",
				})
				if err != nil {
					t.Fatal(err)
				}
				identity := DomainCommitIdentity{CommandID: "command-old-head", OperationID: "operation-old-head", Cycle: 1}
				input, err := NewPlayerInputIntent(identity, "main", "inspect the gate")
				if err != nil {
					t.Fatal(err)
				}
				var receipt PlayerInputReceipt
				if acceptBeforeSideEvents {
					receipt, err = store.CommitPlayerInput(story.ID, input)
					if err != nil {
						t.Fatal(err)
					}
				}
				appendModelInvisibleStoryEvents(t, store, story.ID, "main", storyRecentCacheRecordLimit+1)
				if reopen {
					store = NewStore(workspace)
				}
				if !acceptBeforeSideEvents {
					receipt, err = store.CommitPlayerInput(story.ID, input)
					if err != nil {
						t.Fatal(err)
					}
				}
				if receipt.Event.ParentID != turn.ID {
					t.Fatalf("accepted input parent = %q, want active head %q", receipt.Event.ParentID, turn.ID)
				}
				intents, err := NewModelContextBatchIntents(identity, "main", 0, durableModelContextBatchFixture())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.AppendModelContextBatch(story.ID, intents[0]); err != nil {
					t.Fatalf("model context batch rejected an input attached to the active head: %v", err)
				}
				coldStore := NewStore(workspace)
				_, recent, err := coldStore.readStoryRecentLocked(story.ID, "main")
				if err != nil {
					t.Fatal(err)
				}
				if len(recent) > storyRecentCacheRecordLimit {
					t.Fatalf("cold recent records = %d, want at most %d", len(recent), storyRecentCacheRecordLimit)
				}
			})
		}
	}
}

func TestTurnAtomicallyAbsorbsDurableModelContextBatchOnce(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "consume tool context", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-success", OperationID: "operation-success", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "读取线索")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	messages := durableModelContextBatchFixture()
	intents, err := NewModelContextBatchIntents(identity, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendModelContextBatch(story.ID, intents[0]); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "读取线索", Narrative: "门后留有新鲜脚印。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		// The runtime still carries the just-recorded batch in memory. The store
		// must fold the durable copy in without duplicating it.
		ModelContextMessages: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(turn.ModelContextMessages, messages) {
		t.Fatalf("committed tool context was not absorbed exactly once:\nwant=%#v\ngot=%#v", messages, turn.ModelContextMessages)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 0 || len(snapshot.PendingModelContextBatches) != 0 {
		t.Fatalf("committed input still exposes pending side events: inputs=%#v batches=%#v", snapshot.PendingPlayerInputs, snapshot.PendingModelContextBatches)
	}
}

func TestLaterSuccessfulTurnResolvesInterruptedInputsAndTheirBatches(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "resolve interrupted context", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	interrupted := DomainCommitIdentity{CommandID: "command-interrupted", OperationID: "operation-interrupted", Cycle: 1}
	interruptedInput, err := NewPlayerInputIntent(interrupted, "main", "先检查门后的线索")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, interruptedInput); err != nil {
		t.Fatal(err)
	}
	messages := durableModelContextBatchFixture()
	intents, err := NewModelContextBatchIntents(interrupted, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendModelContextBatch(story.ID, intents[0]); err != nil {
		t.Fatal(err)
	}

	successful := DomainCommitIdentity{CommandID: "command-successor", OperationID: "operation-successor", Cycle: 1}
	successfulInput, err := NewPlayerInputIntent(successful, "main", "继续前进")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, successfulInput); err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: successfulInput.Text, Narrative: "脚印通向长廊深处。",
		AgentCommandID: successful.CommandID, AgentOperationID: successful.OperationID, AgentCycle: successful.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(turn.ConsumedPlayerInputIDs, []string{deterministicPlayerInputID(interrupted), deterministicPlayerInputID(successful)}) {
		t.Fatalf("resolved input identities = %#v", turn.ConsumedPlayerInputIDs)
	}
	if len(turn.ModelContextMessages) != 0 {
		t.Fatalf("interrupted batch was reordered into successor Turn: %#v", turn.ModelContextMessages)
	}
	if len(turn.ResolvedPlayerInputContexts) != 1 {
		t.Fatalf("resolved input contexts = %d, want 1", len(turn.ResolvedPlayerInputContexts))
	}
	resolved := turn.ResolvedPlayerInputContexts[0]
	if resolved.Input.ID != deterministicPlayerInputID(interrupted) || resolved.Input.Text != interruptedInput.Text {
		t.Fatalf("resolved input changed: %#v", resolved.Input)
	}
	if resolved.Input.AcceptedTurnCount != 0 {
		t.Fatalf("resolved accepted boundary = %#v, want 0", resolved.Input.AcceptedTurnCount)
	}
	if len(resolved.ModelContextBatches) != 1 || !reflect.DeepEqual(resolved.ModelContextBatches[0].Messages, messages) {
		t.Fatalf("resolved interrupted batches changed: %#v", resolved.ModelContextBatches)
	}
	for _, candidate := range []*Store{store, NewStore(workspace)} {
		snapshot, err := candidate.Snapshot(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.PendingPlayerInputs) != 0 || len(snapshot.PendingModelContextBatches) != 0 {
			t.Fatalf("resolved interrupted context remained pending after reload: inputs=%#v batches=%#v", snapshot.PendingPlayerInputs, snapshot.PendingModelContextBatches)
		}
		if len(snapshot.Turns) != 1 || !reflect.DeepEqual(snapshot.Turns[0].ResolvedPlayerInputContexts, turn.ResolvedPlayerInputContexts) {
			t.Fatalf("resolved context changed across reload: %#v", snapshot.Turns)
		}
		history, err := candidate.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", StartTurn: 0, EndTurn: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(history.Turns) != 1 || !reflect.DeepEqual(history.Turns[0].ResolvedPlayerInputContexts, turn.ResolvedPlayerInputContexts) {
			t.Fatalf("model history lost resolved context: %#v", history.Turns)
		}
	}
}

func TestResolvedPlayerInputContextsPreserveAcceptanceOrderAndCurrentTurnSuffix(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "ordered resolved context", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}

	first := DomainCommitIdentity{CommandID: "command-first", OperationID: "operation-first", Cycle: 1}
	firstInput := commitPlayerInputAndModelBatchForTest(t, store, story.ID, first, "first interrupted input", "call-first", "first evidence")
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "intervening completed input", Narrative: "intervening completed answer",
	}); err != nil {
		t.Fatal(err)
	}
	second := DomainCommitIdentity{CommandID: "command-second", OperationID: "operation-second", Cycle: 1}
	secondInput := commitPlayerInputAndModelBatchForTest(t, store, story.ID, second, "second interrupted input", "call-second", "second evidence")
	current := DomainCommitIdentity{CommandID: "command-current", OperationID: "operation-current", Cycle: 1}
	currentMessages := durableModelContextBatchFixtureForCall("call-current", "current evidence")
	commitPlayerInputAndModelBatchForTest(t, store, story.ID, current, "current input", "call-current", "current evidence")

	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "current input", Narrative: "current answer",
		AgentCommandID: current.CommandID, AgentOperationID: current.OperationID, AgentCycle: current.Cycle,
		ModelContextMessages: currentMessages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(turn.ModelContextMessages, currentMessages) {
		t.Fatalf("current Turn suffix changed:\nwant=%#v\ngot=%#v", currentMessages, turn.ModelContextMessages)
	}
	if len(turn.ResolvedPlayerInputContexts) != 2 {
		t.Fatalf("resolved contexts = %d, want 2", len(turn.ResolvedPlayerInputContexts))
	}
	if turn.ResolvedPlayerInputContexts[0].Input.ID != firstInput.ID ||
		turn.ResolvedPlayerInputContexts[1].Input.ID != secondInput.ID {
		t.Fatalf("resolved acceptance order changed: %#v", turn.ResolvedPlayerInputContexts)
	}
	for index, wantBoundary := range []int{0, 1} {
		input := turn.ResolvedPlayerInputContexts[index].Input
		if input.AcceptedTurnCount != wantBoundary {
			t.Fatalf("resolved input %d boundary = %#v, want %d", index, input.AcceptedTurnCount, wantBoundary)
		}
	}
	if got := turn.ResolvedPlayerInputContexts[0].ModelContextBatches[0].Messages[1].Content; got != "first evidence" {
		t.Fatalf("first resolved evidence = %q", got)
	}
	if got := turn.ResolvedPlayerInputContexts[1].ModelContextBatches[0].Messages[1].Content; got != "second evidence" {
		t.Fatalf("second resolved evidence = %q", got)
	}

	reloaded := NewStore(workspace)
	history, err := reloaded.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", StartTurn: 0, EndTurn: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Turns) != 2 || !reflect.DeepEqual(history.Turns[1].ResolvedPlayerInputContexts, turn.ResolvedPlayerInputContexts) {
		t.Fatalf("cold model history changed ordered resolved context: %#v", history.Turns)
	}
	if !reflect.DeepEqual(history.Turns[1].ModelContextMessages, currentMessages) {
		t.Fatalf("cold model history changed current suffix: %#v", history.Turns[1].ModelContextMessages)
	}
	cloned := CloneResolvedPlayerInputContexts(turn.ResolvedPlayerInputContexts)
	cloned[0].Input.Text = "mutated input"
	cloned[0].ModelContextBatches[0].Messages[1].Content = "mutated evidence"
	*cloned[0].ModelContextBatches[0].Messages[0].ToolCalls[0].Index = 9
	cloned[0].ModelContextBatches[0].Messages[0].ToolCalls[0].Extra["nested"].(map[string]any)["value"] = "mutated extra"
	if turn.ResolvedPlayerInputContexts[0].Input.Text != "first interrupted input" ||
		turn.ResolvedPlayerInputContexts[0].ModelContextBatches[0].Messages[1].Content != "first evidence" ||
		*turn.ResolvedPlayerInputContexts[0].ModelContextBatches[0].Messages[0].ToolCalls[0].Index != 0 ||
		turn.ResolvedPlayerInputContexts[0].ModelContextBatches[0].Messages[0].ToolCalls[0].Extra["nested"].(map[string]any)["value"] != "original extra" {
		t.Fatalf("resolved context clone aliases canonical data: %#v", turn.ResolvedPlayerInputContexts[0])
	}
	malformed := turn
	malformed.ResolvedPlayerInputContexts = CloneResolvedPlayerInputContexts(turn.ResolvedPlayerInputContexts)
	malformed.ResolvedPlayerInputContexts[0].Input.Text = "hash-changing mutation"
	if _, err := storyEventRecordForWrite(malformed); err == nil {
		t.Fatal("story schema accepted a resolved input whose canonical hash changed")
	}
}

func commitPlayerInputAndModelBatchForTest(
	t *testing.T,
	store *Store,
	storyID string,
	identity DomainCommitIdentity,
	text, callID, evidence string,
) PlayerInputAcceptedEvent {
	t.Helper()
	input, err := NewPlayerInputIntent(identity, "main", text)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.CommitPlayerInput(storyID, input)
	if err != nil {
		t.Fatal(err)
	}
	messages := durableModelContextBatchFixtureForCall(callID, evidence)
	intents, err := NewModelContextBatchIntents(identity, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 {
		t.Fatalf("model context batch intents = %d, want 1", len(intents))
	}
	if _, err := store.AppendModelContextBatch(storyID, intents[0]); err != nil {
		t.Fatal(err)
	}
	return receipt.Event
}

func durableModelContextBatchFixture() []ModelContextMessage {
	return durableModelContextBatchFixtureForCall("call-fetch", "The gate was opened recently.")
}

func TestTurnAbsorbsMixedAgentContextKindsFromCanonicalBatches(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "mixed context", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "command-mixed", OperationID: "operation-mixed", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "继续")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	state := agent.UserMessage("current state")
	state.Extra = map[string]any{
		"agent.context_state":             "v1",
		"agent.context_state.operation":   "upsert",
		"agent.context_state.id":          "workspace",
		"agent.context_state.source":      "test.workspace",
		"agent.context_state.purpose":     "provide current state",
		"agent.context_state.resource":    "workspace",
		"agent.context_state.revision":    "revision-1",
		"agent.context_state.fingerprint": strings.Repeat("a", 64),
	}
	completion := agent.UserMessage("child result")
	completion.TaskCompletion = &agent.TaskCompletionMessageMeta{
		CompletionID: "completion-mixed", Author: "researcher", Recipient: "parent",
	}
	batches := []struct {
		messages []ModelContextMessage
	}{
		{messages: []ModelContextMessage{ModelContextMessageFromAgent(state, nil)}},
		{messages: durableModelContextBatchFixture()},
		{messages: []ModelContextMessage{ModelContextMessageFromAgent(completion, nil)}},
	}
	var expected []ModelContextMessage
	for sequence, batch := range batches {
		intent, err := NewAgentContextBatchIntent(identity, "main", sequence, batch.messages)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := store.AppendModelContextBatch(story.ID, intent)
		if err != nil {
			t.Fatal(err)
		}
		expected = append(expected, receipt.Event.Messages...)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "继续前进。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
		ModelContextMessages: expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(turn.ModelContextMessages, expected) {
		t.Fatalf("mixed canonical context changed:\nwant=%#v\ngot=%#v", expected, turn.ModelContextMessages)
	}
	history, err := NewStore(store.root).ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", StartTurn: 0, EndTurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Turns) != 1 || !reflect.DeepEqual(history.Turns[0].ModelContextMessages, expected) {
		t.Fatalf("cold mixed context=%#v", history.Turns)
	}
}

func durableModelContextBatchFixtureForCall(callID, evidence string) []ModelContextMessage {
	callIndex := 0
	return []ModelContextMessage{
		{
			Role: "assistant", Content: "I will fetch the referenced lore.",
			ToolCalls: []ModelContextToolCall{{
				Index: &callIndex, ID: callID, Type: "function",
				Function: ModelContextFunctionCall{Name: "web_fetch", Arguments: `{"url":"https://example.test/lore"}`},
				Extra:    map[string]any{"nested": map[string]any{"value": "original extra"}},
			}},
		},
		{
			Role: "tool", ToolCallID: callID, ToolName: "web_fetch", Content: evidence,
			ToolResult: &agent.ToolResultSummary{
				Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultEagerCandidate,
				Artifacts: []agent.ToolArtifactRef{{ReadablePath: ".denova/artifacts/game/fetch.txt", EstimatedBytes: 4096, Complete: true}},
			},
		},
	}
}

func assertPendingModelContextBatch(t *testing.T, snapshot Snapshot, messages []ModelContextMessage) {
	t.Helper()
	if len(snapshot.PendingModelContextBatches) != 1 {
		t.Fatalf("pending model context batches = %d, want 1", len(snapshot.PendingModelContextBatches))
	}
	if !reflect.DeepEqual(snapshot.PendingModelContextBatches[0].Messages, messages) {
		t.Fatalf("pending rich batch changed across projection:\nwant=%#v\ngot=%#v", messages, snapshot.PendingModelContextBatches[0].Messages)
	}
}

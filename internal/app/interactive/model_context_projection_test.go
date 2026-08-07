package interactiveapp

import (
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"reflect"
	"strconv"
	"strings"
	"testing"

	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/interactive"
)

func TestInteractiveModelProjectionKeepsInterruptedBatchesStableAcrossSettlementAndReload(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "stable interrupted projection", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "completed-before-pending", Narrative: "first completed answer",
	}); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 4; index++ {
		appendInterruptedGameBatchForProjectionTest(t, store, story.ID, index)
	}
	// A different cycle can settle after those inputs were accepted. Pending
	// evidence must stay before this later Turn rather than moving behind it on
	// the next cold projection.
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "completed-after-pending", Narrative: "later completed answer",
	}); err != nil {
		t.Fatal(err)
	}

	current := interactive.DomainCommitIdentity{CommandID: "current-command", OperationID: "current-operation", Cycle: 1}
	currentIntent, err := interactive.NewPlayerInputIntent(current, "main", "current settling action")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, currentIntent); err != nil {
		t.Fatal(err)
	}
	currentCycle := agentrun.CycleIdentity{CommandID: agentrun.CommandID(current.CommandID), OperationID: agentrun.OperationID(current.OperationID), Cycle: current.Cycle}
	before := interactiveProjectionFromStoreForTest(t, store, story.ID, currentCycle)
	if before.SourceTurnCount != 1 {
		t.Fatalf("compactable completed boundary = %d, want earliest pending boundary 1", before.SourceTurnCount)
	}
	if len(before.PendingInputMessages) != 4 {
		t.Fatalf("projected interrupted inputs = %d, want 4", len(before.PendingInputMessages))
	}
	sourceText := joinedInteractiveProjectionContent(before.SourceMessages)
	if !strings.Contains(sourceText, "completed-before-pending") || strings.Contains(sourceText, "completed-after-pending") || strings.Contains(sourceText, "interrupted-input") {
		t.Fatalf("checkpoint source crossed a pending boundary: %q", sourceText)
	}
	beforeText := joinedInteractiveProjectionContent(before.Messages)
	laterTurn := strings.Index(beforeText, "completed-after-pending")
	if laterTurn < 0 {
		t.Fatalf("later completed Turn missing from projection: %q", beforeText)
	}
	for index := 0; index < 4; index++ {
		marker := strings.Index(beforeText, "interrupted-input-"+strconv.Itoa(index))
		if marker < 0 || marker > laterTurn {
			t.Fatalf("interrupted input %d moved behind later Turn: %q", index, beforeText)
		}
	}

	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "current settling action", Narrative: "current settled answer",
		AgentKind: "interactive_story", AgentCommandID: current.CommandID,
		AgentOperationID: current.OperationID, AgentCycle: current.Cycle,
	}); err != nil {
		t.Fatal(err)
	}
	after := interactiveProjectionFromStoreForTest(t, store, story.ID, agentrun.CycleIdentity{})
	if len(after.Messages) < len(before.Messages) || !reflect.DeepEqual(after.Messages[:len(before.Messages)], before.Messages) {
		t.Fatalf("settlement reordered the canonical pending tail:\nbefore=%#v\nafter=%#v", before.Messages, after.Messages)
	}
	afterText := joinedInteractiveProjectionContent(after.Messages)
	if strings.Index(afterText, "current settling action") < strings.Index(afterText, "completed-after-pending") {
		t.Fatalf("settled current Turn was not appended after the stable tail: %q", afterText)
	}

	reloaded := interactive.NewStore(workspace)
	cold := interactiveProjectionFromStoreForTest(t, reloaded, story.ID, agentrun.CycleIdentity{})
	if !reflect.DeepEqual(cold, after) {
		t.Fatalf("cold Game projection differs from settled projection:\nsettled=%#v\ncold=%#v", after, cold)
	}

	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedParent := storyContext.Meta.Branches["main"].Head
	checkpoint, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("interactive_story", agentcompaction.Result{
			Epoch: 1, Summary: "completed prefix checkpoint", RetainedTurns: 1,
		}),
		SourceTurnCount: 1, ExpectedParentID: &expectedParent,
	})
	if err != nil {
		t.Fatal(err)
	}
	durable := interactiveProjectionFromStoreForTest(t, store, story.ID, agentrun.CycleIdentity{})
	expectedDurable := append(
		[]*agents.Message{agentcontext.NewCompactionSummaryMessage(checkpoint.Epoch, checkpoint.Summary)},
		after.Messages...,
	)
	if !reflect.DeepEqual(durable.Messages, expectedDurable) {
		t.Fatalf("durable checkpoint changed the pending tail:\nwant=%#v\ngot=%#v", expectedDurable, durable.Messages)
	}
	if durable.SourceTurnCount != 3 {
		t.Fatalf("resolved atomic source boundary = %d, want all 3 completed Turns", durable.SourceTurnCount)
	}
	if _, err := agentcontext.NormalizeModelContextMessages(durable.SourceMessages); err != nil {
		t.Fatalf("resolved compaction source is not a valid model transcript: %v", err)
	}
	durableSource := joinedInteractiveProjectionContent(durable.SourceMessages)
	if strings.Contains(durableSource, "completed-before-pending") ||
		!strings.Contains(durableSource, "interrupted-input-0") ||
		!strings.Contains(durableSource, "completed-after-pending") ||
		!strings.Contains(durableSource, "current settling action") {
		t.Fatalf("checkpoint-at-acceptance did not expose the complete resolved interval: %q", durableSource)
	}
	resolvedInput := strings.Index(durableSource, "interrupted-input-0")
	middleTurn := strings.Index(durableSource, "completed-after-pending")
	ownerTurn := strings.Index(durableSource, "current settling action")
	if resolvedInput < 0 || resolvedInput > middleTurn || middleTurn > ownerTurn {
		t.Fatalf("resolved source interval order changed: %q", durableSource)
	}

	storyContext, err = store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedParent = storyContext.Meta.Branches["main"].Head
	if _, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("interactive_story", agentcompaction.Result{
			Epoch: 2, Summary: "fully resolved checkpoint", RetainedTurns: 1,
		}),
		SourceTurnCount: 3, ExpectedParentID: &expectedParent,
	}); err != nil {
		t.Fatal(err)
	}
	fullyCompacted := interactiveProjectionFromStoreForTest(t, store, story.ID, agentrun.CycleIdentity{})
	fullyCompactedText := joinedInteractiveProjectionContent(fullyCompacted.Messages)
	if fullyCompacted.SourceTurnCount != 3 || len(fullyCompacted.SourceMessages) != 0 ||
		strings.Contains(fullyCompactedText, "interrupted-input-") ||
		strings.Contains(fullyCompactedText, "completed-after-pending") ||
		!strings.Contains(fullyCompactedText, "fully resolved checkpoint") ||
		!strings.Contains(fullyCompactedText, "current settling action") {
		t.Fatalf("resolved interval was not absorbed by the advanced checkpoint: %#v", fullyCompacted)
	}
	reloadedAfterCheckpoint := interactive.NewStore(workspace)
	coldCheckpoint := interactiveProjectionFromStoreForTest(t, reloadedAfterCheckpoint, story.ID, agentrun.CycleIdentity{})
	if !reflect.DeepEqual(coldCheckpoint, fullyCompacted) {
		t.Fatalf("cold checkpoint projection differs from canonical view:\ncanonical=%#v\ncold=%#v", fullyCompacted, coldCheckpoint)
	}
}

func TestInteractiveAtomicSourceTurnCountDoesNotBisectResolvedIntervals(t *testing.T) {
	resolved := []interactiveResolvedContext{
		{sourceBoundary: 2, ownerTurn: 5},
		{sourceBoundary: 1, ownerTurn: 3},
	}
	if got := interactiveAtomicSourceTurnCount(0, 6, resolved); got != 6 {
		t.Fatalf("complete source retreated to %d, want 6", got)
	}
	if got := interactiveAtomicSourceTurnCount(0, 5, resolved); got != 1 {
		t.Fatalf("bisected nested source retreated to %d, want oldest acceptance 1", got)
	}
	if got := interactiveAtomicSourceTurnCount(0, 1, resolved); got != 1 {
		t.Fatalf("source before both intervals moved to %d, want 1", got)
	}
	if got := interactiveAtomicSourceTurnCount(2, 5, resolved); got != 2 {
		t.Fatalf("checkpoint-clamped interval retreated to %d, want checkpoint 2", got)
	}
}

func appendInterruptedGameBatchForProjectionTest(t *testing.T, store *interactive.Store, storyID string, index int) {
	t.Helper()
	identity := interactive.DomainCommitIdentity{
		CommandID:   "interrupted-command-" + strconv.Itoa(index),
		OperationID: "interrupted-operation-" + strconv.Itoa(index),
		Cycle:       1,
	}
	input, err := interactive.NewPlayerInputIntent(identity, "main", "interrupted-input-"+strconv.Itoa(index))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.CommitPlayerInput(storyID, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Event.AcceptedTurnCount == nil || *receipt.Event.AcceptedTurnCount != 1 {
		t.Fatalf("accepted logical boundary = %#v, want 1", receipt.Event.AcceptedTurnCount)
	}
	callID := "interrupted-call-" + strconv.Itoa(index)
	messages := []interactive.ModelContextMessage{
		{
			Role: "assistant", Content: "inspect interrupted evidence",
			ToolCalls: []interactive.ModelContextToolCall{{
				ID: callID, Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"evidence.log"}`},
			}},
		},
		{Role: "tool", ToolCallID: callID, ToolName: "read", Content: strings.Repeat("large interrupted evidence ", 1200)},
	}
	intents, err := interactive.NewModelContextBatchIntents(identity, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 {
		t.Fatalf("model-context intents = %d, want 1", len(intents))
	}
	if _, err := store.AppendModelContextBatch(storyID, intents[0]); err != nil {
		t.Fatal(err)
	}
}

func interactiveProjectionFromStoreForTest(
	t *testing.T,
	store *interactive.Store,
	storyID string,
	current agentrun.CycleIdentity,
) ModelContextProjection {
	t.Helper()
	storyContext, err := store.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	start, end, compaction := ModelHistoryRange(storyContext.Snapshot)
	history, err := store.ReadModelHistory(storyID, interactive.StoryModelHistoryQuery{BranchID: "main", StartTurn: start, EndTurn: end})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := BuildModelContextProjection(
		history, compaction, storyContext.Snapshot, toolresult.ContextPolicy{Enabled: true, MaxResultBytes: 256 * 1024}, current,
	)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func joinedInteractiveProjectionContent(messages []*agents.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		if message == nil {
			continue
		}
		builder.WriteString(message.Content)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func TestInteractiveModelProjectionDoesNotReplaySettledNarrativeInsideToolProtocol(t *testing.T) {
	const narrative = "顾清欢接过账册，开始核对第一笔药材账。"
	history := interactive.StoryModelHistory{
		StartTurn:  0,
		EndTurn:    1,
		TotalTurns: 1,
		Turns: []interactive.StoryModelTurn{{
			User:      "开始管账",
			Narrative: narrative,
			ModelContextMessages: []interactive.ModelContextMessage{
				{
					Role: "assistant", Content: narrative,
					ToolCalls: []interactive.ModelContextToolCall{{
						ID: "submit-1", Type: "function",
						Function: interactive.ModelContextFunctionCall{Name: "submit_interactive_turn", Arguments: `{"state_changes":[],"choices":[]}`},
					}},
				},
				{Role: "tool", ToolCallID: "submit-1", ToolName: "submit_interactive_turn", Content: `{"ready":true}`},
			},
		}},
	}
	projection, err := BuildModelContextProjection(
		history, nil, interactive.Snapshot{}, toolresult.ContextPolicy{Enabled: true, MaxResultBytes: 256 * 1024}, agentrun.CycleIdentity{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(joinedInteractiveProjectionContent(projection.Messages), narrative); count != 1 {
		t.Fatalf("settled narrative occurrence count = %d, want 1: %#v", count, projection.Messages)
	}
	if len(projection.Messages) != 4 || len(projection.Messages[1].ToolCalls) != 1 || projection.Messages[1].Content != "" || projection.Messages[2].Role != agents.RoleTool {
		t.Fatalf("tool protocol was not preserved without duplicate prose: %#v", projection.Messages)
	}
	if count := strings.Count(joinedInteractiveProjectionContent(projection.SourceMessages), narrative); count != 1 {
		t.Fatalf("compaction source narrative occurrence count = %d, want 1: %#v", count, projection.SourceMessages)
	}
}

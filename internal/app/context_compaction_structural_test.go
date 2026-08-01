package app

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentstructural "denova/internal/agents/context/structural"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestInteractiveCompactionRejectsNoProgressAfterStableContextReinjection(t *testing.T) {
	original := []*agents.Message{agents.UserMessage("original provider-visible history")}
	candidate := []*agents.Message{agents.UserMessage("bounded checkpoint")}
	stableLeading := strings.Repeat("complete resident lore remains provider-visible. ", 200)
	candidate = preserveInteractiveStableLeadingMessage(candidate, stableLeading)
	candidateTokens := agentcontext.EstimateTokens([]*agents.Message{agents.UserMessage("bounded checkpoint")}, nil)
	truePostTokens := agentcontext.EstimateTokens(candidate, nil)
	if candidateTokens >= truePostTokens {
		t.Fatalf("test fixture has no stable-context pressure: candidate=%d true_post=%d", candidateTokens, truePostTokens)
	}
	tokensBefore := candidateTokens + (truePostTokens-candidateTokens)/2
	result := agentcompaction.Result{
		Triggered:            true,
		TokensBefore:         tokensBefore,
		TokensAfter:          candidateTokens,
		ProjectedTokensAfter: candidateTokens,
		ContextWindowTokens:  truePostTokens * 4,
		RecoveryTargetTokens: truePostTokens,
	}

	messages, result, err := validateInteractiveCompactionProjection(original, candidate, result, nil)
	if err == nil {
		t.Fatal("expected true post-context validation to reject no-progress compaction")
	}
	if result.Triggered || result.SkippedReason != "no_progress" || result.TokensAfter != truePostTokens {
		t.Fatalf("invalid candidate result = %#v", result)
	}
	if len(messages) != 1 || messages[0] != original[0] {
		t.Fatalf("invalid candidate replaced live model input: %#v", messages)
	}
	conversation := &interactiveConversation{}
	conversation.stagePreparedInteractiveCompaction(preparedInteractiveContextCompaction{Result: result})
	if conversation.pendingCompaction != nil {
		t.Fatal("invalid candidate was staged for post-settlement publication")
	}
}

func TestInteractiveCompactionCalibratesTruePostContextAfterStableReinjection(t *testing.T) {
	original := []*agents.Message{agents.UserMessage(strings.Repeat("original history ", 200))}
	candidate := []*agents.Message{agents.UserMessage("bounded checkpoint")}
	candidate = preserveInteractiveStableLeadingMessage(candidate, strings.Repeat("resident lore remains exact. ", 80))
	localTruePost := agentcontext.EstimateTokens(candidate, nil)
	observedTruePost := localTruePost * 2
	contextWindow := (observedTruePost*100 + 83) / 84
	const completionReserve = 31
	const toolReserve = 47
	result := agentcompaction.Result{
		Triggered:                true,
		ObservedPromptTokens:     observedTruePost,
		ObservedEstimateTokens:   localTruePost,
		TokensBefore:             observedTruePost + 100,
		TokensAfter:              agentcontext.EstimateTokens([]*agents.Message{agents.UserMessage("bounded checkpoint")}, nil),
		ProjectedTokensAfter:     localTruePost,
		ReservedCompletionTokens: completionReserve,
		ReservedToolResultTokens: toolReserve,
		ContextWindowTokens:      contextWindow,
		Threshold:                0.85,
		RecoveryBand:             0.80,
	}

	messages, result, err := validateInteractiveCompactionProjection(original, candidate, result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != len(candidate) || result.TokensAfter != observedTruePost {
		t.Fatalf("true post calibration lost: local=%d observed=%d result=%#v", localTruePost, observedTruePost, result)
	}
	if result.ProjectedTokensAfter != observedTruePost+completionReserve+toolReserve {
		t.Fatalf("true post reserves = %d, want %d", result.ProjectedTokensAfter, observedTruePost+completionReserve+toolReserve)
	}
	if result.RecoveryBandMet || !result.Degraded || result.TokensAfter*100 < result.ContextWindowTokens*83 {
		t.Fatalf("provider-calibrated ~84%% context was misclassified: %#v", result)
	}
}

func TestInteractiveCompactionProjectionMatchesDurableReloadAndPreservesPendingSideInput(t *testing.T) {
	hugeNthOldTurn := strings.Repeat("the Nth retained story fact remains visible ", 700)
	turns := []interactive.TurnEvent{
		{ID: "turn-1", BranchID: "main", User: "enter the hall", Narrative: "the hall is empty"},
		{ID: "turn-2", BranchID: "main", User: hugeNthOldTurn, Narrative: "the observatory door opens"},
	}
	sourceMessages := interactiveEffectiveModelMessages(buildInteractiveTurnHistory(turns), nil)
	pendingSideInput := []*agents.Message{
		agents.UserMessage("pending interrupted player input"),
		agents.AssistantMessage("", []agents.ToolCall{{ID: "pending-call", Type: "function", Function: agents.FunctionCall{Name: "read"}}}),
		agents.ToolMessage(agents.TextToolResult("pending durable model-context batch"), "pending-call", agents.WithToolName("read")),
		agents.UserMessage("current player instruction"),
	}
	providerMessages := append(append([]*agents.Message(nil), sourceMessages...), pendingSideInput...)
	transient, payload := agentcompaction.BuildModelMessagesThroughSource(
		providerMessages, "durable story checkpoint", "", 1, 1, len(sourceMessages),
	)
	event := &interactive.ContextCompactionEvent{
		CompactionCheckpoint: agentcompaction.NewCheckpoint("", agentcompaction.Result{
			Epoch: 1, Summary: payload, RetainedTurns: 1,
		}),
		SourceTurnCount: len(turns),
	}
	durableReload := interactiveEffectiveModelMessages(buildInteractiveTurnHistoryWithCompaction(turns, event, 1), event)
	durableReload = append(durableReload, pendingSideInput...)
	if !reflect.DeepEqual(transient, durableReload) {
		t.Fatalf("immediate/durable game projections differ:\nimmediate=%#v\ndurable=%#v", transient, durableReload)
	}
	if agentcontext.EstimateTokens(transient, nil) != agentcontext.EstimateTokens(durableReload, nil) {
		t.Fatalf("immediate/durable token estimates differ: %d vs %d", agentcontext.EstimateTokens(transient, nil), agentcontext.EstimateTokens(durableReload, nil))
	}
	for _, expected := range []string{hugeNthOldTurn, "pending interrupted player input", "pending durable model-context batch", "current player instruction"} {
		found := false
		for _, message := range transient {
			if message != nil && strings.Contains(message.Content, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("compaction dropped provider-visible suffix or Nth old turn %q: %#v", expected, transient)
		}
	}
}

func TestWritingCompactionRemovalUsesDurableStructuralCommand(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("durable compaction removal")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agents.UserMessage("raw history remains canonical")); err != nil {
		t.Fatal(err)
	}
	compaction, err := sess.AppendContextCompaction(session.ContextCompaction{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindIDE, agentcompaction.Result{Summary: "checkpoint"}),
		SourceEndIndex:       1,
		SourceMessageCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agentharness.NewEphemeralService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{
		workspace: "/book", workspaceGeneration: 1, sessionStore: store,
		session: sess, chatService: chat,
	}
	service := &ChatAppService{app: application}
	removed, err := service.executeWritingContextCompactionRemoval(context.Background(), "client-remove-writing-1")
	if err != nil || !removed {
		t.Fatalf("durable removal: removed=%t err=%v", removed, err)
	}
	replayed, err := service.executeWritingContextCompactionRemoval(context.Background(), "client-remove-writing-1")
	if err != nil || !replayed {
		t.Fatalf("idempotent removal replay: removed=%t err=%v", replayed, err)
	}
	if _, ok := sess.LatestContextCompaction(config.AgentKindIDE); ok {
		t.Fatal("removed compaction remains active")
	}
	marker, ok := sess.LatestContextCompactionRemoval(config.AgentKindIDE)
	if !ok || marker.CompactionID != compaction.ID {
		t.Fatalf("missing canonical removal marker: %#v", marker)
	}
	status, err := chat.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: sess.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision == "" {
		t.Fatalf("durable structural settlement missing: %#v", status)
	}
}

func TestInteractiveCompactionRemovalUsesDurableStructuralCommand(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "durable game compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "钟楼响起。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := turn.ID
	compaction, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindInteractiveStory, agentcompaction.Result{Summary: "钟楼响起"}),
		SourceTurnCount:      1,
		ExpectedParentID:     &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	chat := agentharness.NewEphemeralService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{workspace: workspace, workspaceGeneration: 1, interactive: store, chatService: chat}
	service := &InteractiveAppService{app: application}
	removed, err := service.executeInteractiveContextCompactionRemoval(context.Background(), story.ID, "main", "client-remove-game-1")
	if err != nil || !removed {
		t.Fatalf("durable interactive removal: removed=%t err=%v", removed, err)
	}
	replayed, err := service.executeInteractiveContextCompactionRemoval(context.Background(), story.ID, "main", "client-remove-game-1")
	if err != nil || !replayed {
		t.Fatalf("idempotent interactive removal replay: removed=%t err=%v", replayed, err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil || snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.CompactionID != compaction.ID {
		t.Fatalf("canonical interactive removal missing: %#v", snapshot)
	}
	status, err := chat.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationSucceeded ||
		status.LastDomainCommit == nil || status.LastDomainCommit.Revision == "" {
		t.Fatalf("durable game structural settlement missing: %#v", status)
	}
}

func TestInteractivePostSettlementCompactionPublishesAtSettledTurnHead(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "post settlement game compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "观察大厅", Narrative: "大厅中央悬着一盏旧灯。",
	}); err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", 0, &config.Config{})
	conversation.stagePreparedInteractiveCompaction(preparedInteractiveContextCompaction{
		Result: agentcompaction.Result{
			Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "大厅中央有一盏旧灯。",
			SourceMessageCount: 2, RetainedTurns: 2,
		},
		SourceTurnCount: 1,
	})
	settledTurn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "触碰旧灯", Narrative: "灯芯亮起微弱的蓝光。",
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := conversation.PostSettlementContextStructuralSpec(context.Background(), "settled-game-operation", agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("expected staged post-settlement structural spec")
	}
	if spec.RestorePlan == nil || spec.RestorePlan.Domain != agentstructural.DomainStory ||
		spec.RestorePlan.RecordID == "" || spec.RestorePlan.IntentHash == "" || len(spec.RestorePlan.Mutation) == 0 {
		t.Fatalf("post-settlement Story compaction has no exact restore plan: %#v", spec.RestorePlan)
	}
	chat := agentharness.NewEphemeralService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	result, err := chat.ExecuteStructuralOperation(context.Background(), *spec)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compaction.Triggered {
		t.Fatalf("unexpected compaction result: %#v", result)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction == nil {
		t.Fatal("post-settlement checkpoint was not persisted")
	}
	if snapshot.ContextCompaction.ParentID != settledTurn.ID || snapshot.ContextCompaction.SourceTurnCount != 1 {
		t.Fatalf("post-settlement checkpoint = %#v, settled turn = %#v", snapshot.ContextCompaction, settledTurn)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("structural checkpoint changed raw story history: %#v", snapshot.Turns)
	}
}

package interactiveapp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
)

type publicGameCommitModel struct{ narrative string }

func (model publicGameCommitModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return agent.AssistantMessage(model.narrative, nil), nil
}

func (model publicGameCommitModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage(model.narrative, nil)}), nil
}

type publicGameSequenceModel struct {
	mu        sync.Mutex
	responses []*agent.Message
	next      int
}

func (model *publicGameSequenceModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return model.response()
}

func (model *publicGameSequenceModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.response()
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *publicGameSequenceModel) response() (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.next >= len(model.responses) {
		return nil, fmt.Errorf("test Game model exhausted responses")
	}
	response := agent.CloneMessage(model.responses[model.next])
	model.next++
	return response, nil
}

type publicGameHistoryModel struct {
	mu        sync.Mutex
	narrative string
	inputs    [][]*agent.Message
}

func (model *publicGameHistoryModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.response(messages), nil
}

func (model *publicGameHistoryModel) Stream(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.response(messages)}), nil
}

func (model *publicGameHistoryModel) response(messages []*agent.Message) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	model.inputs = append(model.inputs, cloned)
	return agent.AssistantMessage(model.narrative, nil)
}

func (model *publicGameHistoryModel) lastInput(t *testing.T) []*agent.Message {
	t.Helper()
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.inputs) == 0 {
		t.Fatal("Game model received no request")
	}
	result := make([]*agent.Message, len(model.inputs[len(model.inputs)-1]))
	for index, message := range model.inputs[len(model.inputs)-1] {
		result[index] = agent.CloneMessage(message)
	}
	return result
}

type publicGameTestProfile struct {
	prepare   func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error)
	canonical func(context.Context, agentexecution.CanonicalInputRequest) (agent.CanonicalAdapter, error)
}

func (publicGameTestProfile) ID() agentexecution.ProfileID { return agentexecution.ProfileGame }

func (profile publicGameTestProfile) PrepareCycle(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
	if profile.prepare == nil {
		return agentexecution.Cycle{}, fmt.Errorf("test Game profile cannot prepare a cycle")
	}
	return profile.prepare(ctx, request)
}

func (profile publicGameTestProfile) CanonicalInput(ctx context.Context, request agentexecution.CanonicalInputRequest) (agent.CanonicalAdapter, error) {
	if profile.canonical == nil {
		return nil, fmt.Errorf("test Game profile has no canonical input adapter")
	}
	return profile.canonical(ctx, request)
}

type publicGameCleanupManager struct {
	marker      string
	placeholder string
}

func (manager publicGameCleanupManager) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "cleanup.test.game-public-history", Version: 1}
}

func (manager publicGameCleanupManager) Plan(_ context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	for index, message := range request.ModelRequest {
		if message == nil || message.Role != agent.ToolRole || !strings.Contains(message.Content, manager.marker) {
			continue
		}
		return agent.CleanupPlan{
			Action: agent.CleanupProject, Reason: "test rich Game tool history", Renderer: "test.game.cleanup.v1",
			Replacements: []agent.CleanupReplacement{{
				MessageIndex: index, ToolCallID: message.ToolCallID, Placeholder: manager.placeholder,
			}},
		}, nil
	}
	return agent.CleanupPlan{Action: agent.CleanupNone, Reason: "no matching Game tool history", Renderer: "test.game.cleanup.v1"}, nil
}

type publicGameCompactionManager struct{}

func (publicGameCompactionManager) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "compaction.test.game-public-history", Version: 1}
}

func (publicGameCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (publicGameCompactionManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	return agent.CompactionPlan{
		Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: len(request.Messages) - 2,
		Validation: agent.CompactionValidationPolicy{HardLimitBytes: 8 << 20},
	}, nil
}

func (publicGameCompactionManager) Compact(_ context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	if len(request.SourceMessages) == 0 {
		return agent.CompactionCheckpoint{}, fmt.Errorf("Game compaction received no canonical source")
	}
	return agent.CompactionCheckpoint{Summary: "public Game checkpoint", TokenEstimate: 5}, nil
}

func TestPublicAgentRuntimeCommitsCompleteGameTurnAndDisplay(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "public Agent Game commit", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(
		store, t.TempDir(), workspace, story.ID, "main", "推开石门", 800,
		&config.Config{Workspace: workspace},
	)
	submitTestTurnResult(t, conversation, "推开石门", "石门已经开启")

	runtime := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	request := agentchat.ChatRequest{CommandID: "public-game-start", Message: "推开石门", Locale: "zh-CN"}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, StoryID: story.ID, BranchID: "main",
		Workspace: workspace, TaskID: "public-game-task", RootAgentName: "game",
	}
	var eventsMu sync.Mutex
	var events []agentrun.Event
	operation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Definition: agent.Definition{
				Key: "denova.test.public-game", Name: "game", Model: publicGameCommitModel{narrative: "石门缓缓开启。"},
				ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.public-game", Version: 1},
			},
			Conversation: conversation, Request: request, Options: options,
		},
		Emit: func(event agentrun.Event) {
			eventsMu.Lock()
			events = append(events, event)
			eventsMu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "石门缓缓开启。" {
		t.Fatalf("public Game outcome = %#v", outcome)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.User != "推开石门" || snapshot.CurrentTurn.Narrative != "石门缓缓开启。" {
		t.Fatalf("canonical Game turn = %#v", snapshot.CurrentTurn)
	}
	eventsMu.Lock()
	projected := append([]agentrun.Event(nil), events...)
	eventsMu.Unlock()
	chunks, done := 0, 0
	for _, event := range projected {
		switch event.Type {
		case "chunk":
			chunks++
		case "done":
			done++
		}
	}
	if chunks != 1 || done != 1 {
		t.Fatalf("public Game display events = %#v", projected)
	}
}

func TestPublicAgentRuntimeCommitsAccumulatedGameNarrativeWhenFinalModelMessageIsEmpty(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "public Agent projected Game commit", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(
		store, t.TempDir(), workspace, story.ID, "main", "推开石门", 800,
		&config.Config{Workspace: workspace},
	)
	submitTestTurnResult(t, conversation, "推开石门", "石门已经开启")

	tool, err := agent.InferTool("submit_interactive_turn", "Submit the completed test turn", func(context.Context, struct{}) (string, error) {
		return `{"submitted":true}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 4 << 10,
	}
	toolset, err := agent.StaticToolsIdentified(
		agent.CapabilityIdentity{Kind: "tools.test.public-game-projected-commit", Version: 1},
		agent.ToolDefinition{Tool: tool, Descriptor: descriptor},
	)
	if err != nil {
		t.Fatal(err)
	}
	model := &publicGameSequenceModel{responses: []*agent.Message{
		agent.AssistantMessage("石门缓缓开启。", []agent.ToolCall{{
			ID: "submit-turn", Type: "function", Function: agent.FunctionCall{Name: "submit_interactive_turn", Arguments: `{}`},
		}}),
		agent.AssistantMessage("", nil),
	}}
	runtime := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	request := agentchat.ChatRequest{CommandID: "public-game-projected-start", Message: "推开石门", Locale: "zh-CN"}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, StoryID: story.ID, BranchID: "main",
		Workspace: workspace, TaskID: "public-game-projected-task", RootAgentName: "game",
	}
	var events []agentrun.Event
	operation, err := runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Definition: agent.Definition{
				Key: "denova.test.public-game-projected", Name: "game", Model: model,
				ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.public-game-projected", Version: 1},
				Permission:    agentpermission.FullAccess(), Tools: toolset,
			},
			Conversation: conversation, Request: request, Options: options,
		},
		Emit: func(event agentrun.Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "石门缓缓开启。" {
		t.Fatalf("public Game projected outcome = %#v", outcome)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.Narrative != "石门缓缓开启。" {
		t.Fatalf("canonical projected Game turn = %#v", snapshot.CurrentTurn)
	}
	for _, event := range events {
		if event.Type == "error" {
			t.Fatalf("public Game projected commit emitted error = %#v", event)
		}
		if event.Type == "agent_cycle_started" && event.DataString("delivery") != "start_turn" {
			t.Fatalf("public Game cycle delivery = %#v", event.Data)
		}
	}
}

func TestGameCanonicalTranscriptRetainsRichToolHistoryOutsideModelVisibilityPolicy(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "canonical raw Game history", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	const rich = "RICH_GAME_TOOL_HISTORY_MUST_REMAIN_DURABLE"
	appendPublicGameToolTurn(t, store, story.ID, rich)

	dataDir := t.TempDir()
	profile := publicGameNoopProfile(workspace, story.ID)
	runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir,
		agentexecution.WithProfiles(profile),
		agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	disabled := false
	disabledConfig := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
		InteractiveStory: config.AgentContextOverride{ToolResultContextEnabled: &disabled},
	}}
	initialCanonical, err := NewConversation(store, "", workspace, story.ID, "main", "", 800, disabledConfig).CanonicalTranscript(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsMessageContent(initialCanonical.Messages, rich) {
		t.Fatal("canonical Game raw projection omitted rich tool history before public Session sync")
	}
	initialHash, err := agent.TranscriptHash(initialCanonical.Messages)
	if err != nil {
		t.Fatal(err)
	}
	hiddenModel := &publicGameHistoryModel{narrative: "第一轮继续。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, disabledConfig, hiddenModel, nil, nil, "game-hidden-tool-history", "继续但不展示旧工具")
	if containsMessageContent(hiddenModel.lastInput(t), rich) {
		t.Fatal("disabled Game tool-context policy leaked rich historical tool output to the provider")
	}
	status, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil {
		t.Fatal(err)
	}
	if status.TranscriptSync == nil || status.TranscriptSync.MessageCount != len(initialCanonical.Messages) ||
		status.TranscriptSync.SourceHash != initialHash {
		t.Fatalf("public Game raw transcript provenance lost rich tool history: status=%#v canonical=%#v", status.TranscriptSync, initialCanonical.Messages)
	}

	enabled := true
	enabledConfig := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
		InteractiveStory: config.AgentContextOverride{ToolResultContextEnabled: &enabled},
	}}
	visibleModel := &publicGameHistoryModel{narrative: "第二轮继续。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, enabledConfig, visibleModel, nil, nil, "game-visible-tool-history", "重新展示旧工具证据")
	if !containsMessageContent(visibleModel.lastInput(t), rich) {
		t.Fatal("re-enabled Game tool-context policy could not recover rich history from public Agent raw transcript")
	}
}

func TestGamePublicCleanupCompactionRemovalAndColdReopenRestoreRichHistory(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "public Game maintenance", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	const rich = "RICH_GAME_RESULT_SURVIVES_PUBLIC_MAINTENANCE"
	const placeholder = "[Older Game tool result removed; recover with read.]"
	appendPublicGameToolTurn(t, store, story.ID, rich)
	enabled := true
	cfg := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
		InteractiveStory: config.AgentContextOverride{ToolResultContextEnabled: &enabled},
	}}
	cleanup := publicGameCleanupManager{marker: rich, placeholder: placeholder}
	compaction := publicGameCompactionManager{}
	dataDir := t.TempDir()
	profile := publicGameNoopProfile(workspace, story.ID)
	newRuntime := func() *agentexecution.Runtime {
		runtime, runtimeErr := agentexecution.NewAgentRuntime(ctx, dataDir,
			agentexecution.WithProfiles(profile),
			agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
		)
		if runtimeErr != nil {
			t.Fatal(runtimeErr)
		}
		return runtime
	}
	runtime := newRuntime()
	cleanupModel := &publicGameHistoryModel{narrative: "清理后继续。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, cfg, cleanupModel, cleanup, compaction, "game-cleanup", "整理旧证据")
	cleanupInput := cleanupModel.lastInput(t)
	if !containsMessageContent(cleanupInput, placeholder) || containsMessageContent(cleanupInput, rich) {
		t.Fatalf("public Game Cleanup projection is not exact: %#v", cleanupInput)
	}
	status, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Cleanup == nil || len(status.Cleanup.Replacements) != 1 {
		t.Fatalf("public Game Cleanup was not durable: %#v", status.Cleanup)
	}
	storySnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !containsInteractiveToolResult(storySnapshot, rich) {
		t.Fatal("Story Store lost canonical rich tool history while Agent Cleanup was active")
	}

	compacted, err := runtime.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		Action: agentstructural.Compact, CommandID: "game-public-compact", Options: publicGameOptions(workspace, story.ID, "main"),
		Ref: agentrun.ContextCompactionRef{Force: true},
	})
	if err != nil || !compacted.Compaction.Triggered {
		t.Fatalf("public Game Compaction = %#v err=%v", compacted, err)
	}
	status, err = runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil || status.Compaction == nil {
		t.Fatalf("public Game Compaction status=%#v err=%v", status.Compaction, err)
	}
	removed, err := runtime.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		Action: agentstructural.Remove, CommandID: "game-public-remove", Options: publicGameOptions(workspace, story.ID, "main"),
		Ref: agentrun.ContextCompactionRef{CompactionID: status.Compaction.ID},
	})
	if err != nil || !removed.Removed {
		t.Fatalf("public Game Compaction removal = %#v err=%v", removed, err)
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}

	runtime = newRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	reopenedModel := &publicGameHistoryModel{narrative: "重开后继续。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, cfg, reopenedModel, nil, compaction, "game-cold-reopen", "冷重开验证证据")
	reopenedInput := reopenedModel.lastInput(t)
	if !containsMessageContent(reopenedInput, rich) || containsMessageContent(reopenedInput, placeholder) || containsMessageContent(reopenedInput, "public Game checkpoint") {
		t.Fatalf("public Game raw history did not restore after remove/cold reopen: %#v", reopenedInput)
	}
	status, err = runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Cleanup != nil || status.Compaction != nil || status.TranscriptSync == nil {
		t.Fatalf("maintenance cutoff/cold projection is wrong: cleanup=%#v compaction=%#v sync=%#v", status.Cleanup, status.Compaction, status.TranscriptSync)
	}
}

func TestGameTranscriptSyncRebuildsEditedAndRegeneratedBranchWithoutPollutingFork(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Game transcript rebuild", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	const rich = "FORKED_GAME_RICH_TOOL_HISTORY"
	first := appendPublicGameToolTurn(t, store, story.ID, rich)
	second, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "沿主路前进", Narrative: "主路抵达旧钟楼。",
	})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: first.ID, Title: "返回岔路"})
	if err != nil {
		t.Fatal(err)
	}

	enabled := true
	cfg := &config.Config{Workspace: workspace, AgentContexts: config.AgentContextSettings{
		InteractiveStory: config.AgentContextOverride{ToolResultContextEnabled: &enabled},
	}}
	cleanup := publicGameCleanupManager{marker: rich, placeholder: "[fork test cleanup]"}
	compaction := publicGameCompactionManager{}
	dataDir := t.TempDir()
	profile := publicGameNoopProfile(workspace, story.ID)
	newRuntime := func() *agentexecution.Runtime {
		runtime, runtimeErr := agentexecution.NewAgentRuntime(ctx, dataDir,
			agentexecution.WithProfiles(profile),
			agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
		)
		if runtimeErr != nil {
			t.Fatal(runtimeErr)
		}
		return runtime
	}
	runtime := newRuntime()

	mainModel := &publicGameHistoryModel{narrative: "主线同步后的新回合。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, cfg, mainModel, cleanup, compaction, "game-main-bootstrap", "推进主线")
	if !containsMessageContent(mainModel.lastInput(t), second.Narrative) {
		t.Fatal("main branch bootstrap omitted its canonical second turn")
	}
	mainBeforeEdit, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil || mainBeforeEdit.TranscriptSync == nil || mainBeforeEdit.Cleanup == nil {
		t.Fatalf("main bootstrap status=%#v err=%v", mainBeforeEdit, err)
	}
	compacted, err := runtime.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		Action: agentstructural.Compact, CommandID: "game-edit-compact", Options: publicGameOptions(workspace, story.ID, "main"),
		Ref: agentrun.ContextCompactionRef{Force: true},
	})
	if err != nil || !compacted.Compaction.Triggered {
		t.Fatalf("pre-edit compaction=%#v err=%v", compacted, err)
	}

	forkModel := &publicGameHistoryModel{narrative: "分支独有的营地回合。"}
	runPublicGameTurn(t, runtime, store, story.ID, fork.ID, workspace, cfg, forkModel, nil, nil, "game-fork-bootstrap", "折返回营地")
	forkInput := forkModel.lastInput(t)
	if !containsMessageContent(forkInput, rich) || containsMessageContent(forkInput, second.Narrative) || containsMessageContent(forkInput, mainModel.narrative) {
		t.Fatalf("fork imported a sibling suffix or lost its inherited prefix: %#v", forkInput)
	}
	forkStatus, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, fork.ID))
	if err != nil || forkStatus.TranscriptSync == nil {
		t.Fatalf("fork sync status=%#v err=%v", forkStatus.TranscriptSync, err)
	}
	if forkStatus.TranscriptSync.Source == mainBeforeEdit.TranscriptSync.Source {
		t.Fatal("fork and main share one transcript source identity")
	}

	mainSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil || mainSnapshot.CurrentTurn == nil {
		t.Fatalf("main snapshot=%#v err=%v", mainSnapshot.CurrentTurn, err)
	}
	editedTurn := *mainSnapshot.CurrentTurn
	expectedNarrative := editedTurn.Narrative
	const editedNarrative = "主线编辑后抵达修复后的钟楼。"
	if _, err := store.UpdateTurnNarrative(story.ID, interactive.UpdateTurnNarrativeRequest{
		BranchID: "main", TurnID: editedTurn.ID, Narrative: editedNarrative, ExpectedNarrative: &expectedNarrative,
	}); err != nil {
		t.Fatal(err)
	}
	editedModel := &publicGameHistoryModel{narrative: "编辑后的主线继续。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, cfg, editedModel, nil, nil, "game-main-after-edit", "检查编辑后的历史")
	if !containsMessageContent(editedModel.lastInput(t), editedNarrative) {
		t.Fatal("edited canonical narrative did not rebuild the public raw transcript")
	}
	mainAfterEdit, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil || mainAfterEdit.TranscriptSync == nil {
		t.Fatalf("post-edit status=%#v err=%v", mainAfterEdit, err)
	}
	if mainAfterEdit.TranscriptSync.SourceRevision <= mainBeforeEdit.TranscriptSync.SourceRevision ||
		mainAfterEdit.TranscriptSync.SourceHash == mainBeforeEdit.TranscriptSync.SourceHash ||
		mainAfterEdit.Cleanup != nil || mainAfterEdit.Compaction != nil {
		t.Fatalf("edit did not atomically rebuild maintenance generation: before=%#v after=%#v", mainBeforeEdit, mainAfterEdit)
	}

	mainSnapshot, err = store.Snapshot(story.ID, "main")
	if err != nil || mainSnapshot.CurrentTurn == nil {
		t.Fatalf("pre-regenerate snapshot=%#v err=%v", mainSnapshot.CurrentTurn, err)
	}
	regenerateTarget := *mainSnapshot.CurrentTurn
	regeneratedModel := &publicGameHistoryModel{narrative: "再生成后的主线结局。"}
	runPublicGameRegeneration(t, runtime, store, story.ID, "main", workspace, cfg, regeneratedModel, regenerateTarget.ID, "game-main-regenerate", "重新生成这个回合")
	regenerationInput := regeneratedModel.lastInput(t)
	if containsMessageContent(regenerationInput, regenerateTarget.Narrative) || !containsMessageContent(regenerationInput, editedNarrative) {
		t.Fatalf("regeneration did not import exactly the target parent: %#v", regenerationInput)
	}
	mainAfterRegenerate, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil || mainAfterRegenerate.TranscriptSync == nil || mainAfterRegenerate.TranscriptSync.SourceRevision <= mainAfterEdit.TranscriptSync.SourceRevision {
		t.Fatalf("regenerate historical sync did not advance monotonically: edit=%#v regenerate=%#v err=%v", mainAfterEdit.TranscriptSync, mainAfterRegenerate.TranscriptSync, err)
	}

	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	runtime = newRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	coldMainModel := &publicGameHistoryModel{narrative: "冷重开后的主线。"}
	runPublicGameTurn(t, runtime, store, story.ID, "main", workspace, cfg, coldMainModel, nil, nil, "game-main-cold-after-regenerate", "冷重开主线")
	coldMainInput := coldMainModel.lastInput(t)
	if !containsMessageContent(coldMainInput, regeneratedModel.narrative) || containsMessageContent(coldMainInput, regenerateTarget.Narrative) {
		t.Fatalf("cold main transcript resurrected the replaced version: %#v", coldMainInput)
	}
	coldMainStatus, err := runtime.RuntimeStatusProjection(ctx, publicGameOptions(workspace, story.ID, "main"))
	if err != nil || coldMainStatus.TranscriptSync == nil || coldMainStatus.TranscriptSync.SourceRevision <= mainAfterRegenerate.TranscriptSync.SourceRevision {
		t.Fatalf("cold post-regenerate sync did not return to the live branch generation: %#v err=%v", coldMainStatus.TranscriptSync, err)
	}

	coldForkModel := &publicGameHistoryModel{narrative: "冷重开后的分支。"}
	runPublicGameTurn(t, runtime, store, story.ID, fork.ID, workspace, cfg, coldForkModel, nil, nil, "game-fork-cold", "继续分支")
	coldForkInput := coldForkModel.lastInput(t)
	if !containsMessageContent(coldForkInput, forkModel.narrative) || containsMessageContent(coldForkInput, editedNarrative) || containsMessageContent(coldForkInput, regeneratedModel.narrative) {
		t.Fatalf("cold fork was polluted by main branch rebuild: %#v", coldForkInput)
	}
}

func publicGameNoopProfile(workspace, storyID string) publicGameTestProfile {
	return publicGameTestProfile{
		prepare: func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return agentexecution.Cycle{}, fmt.Errorf("unexpected cold Game cycle preparation for story %s in %s", storyID, workspace)
		},
		canonical: func(context.Context, agentexecution.CanonicalInputRequest) (agent.CanonicalAdapter, error) {
			return nil, fmt.Errorf("unexpected provider-free Game canonical reconstruction for story %s", storyID)
		},
	}
}

func publicGameOptions(workspace, storyID, branchID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, StoryID: storyID, BranchID: branchID,
		Workspace: workspace, TaskID: "public-game-history-task", RootAgentName: "game",
	}.Normalize(workspace)
}

func runPublicGameTurn(
	t *testing.T,
	runtime *agentexecution.Runtime,
	store *interactive.Store,
	storyID, branchID, workspace string,
	cfg *config.Config,
	model *publicGameHistoryModel,
	cleanup agent.CleanupManager,
	compaction agent.CompactionManager,
	commandID, input string,
) {
	t.Helper()
	conversation := NewConversation(store, "", workspace, storyID, branchID, input, 800, cfg)
	submitTestTurnResult(t, conversation, input, input)
	policy := toolresult.ResolveContextPolicy(cfg, config.AgentKindInteractiveStory)
	definition := agent.Definition{
		Key: "denova.test.public-game-history", Name: "game", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.public-game-history", Version: 1},
		Cleanup:       cleanup, Compaction: compaction,
		Middlewares: []agent.Middleware{agentchat.NewModelHistoryProjectionMiddleware(policy)},
	}
	request := agentchat.ChatRequest{CommandID: commandID, Message: input, Locale: "zh-CN"}
	operation, err := runtime.Start(context.Background(), agentexecution.StartRequest{Cycle: agentexecution.Cycle{
		Definition: definition, Conversation: conversation, Request: request,
		Options: publicGameOptions(workspace, storyID, branchID),
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(context.Background())
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != model.narrative {
		t.Fatalf("public Game turn outcome = %#v", outcome)
	}
}

func runPublicGameRegeneration(
	t *testing.T,
	runtime *agentexecution.Runtime,
	store *interactive.Store,
	storyID, branchID, workspace string,
	cfg *config.Config,
	model *publicGameHistoryModel,
	targetTurnID, commandID, input string,
) {
	t.Helper()
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	branch, ok := storyContext.Meta.Branches[storyContext.Snapshot.BranchID]
	if !ok {
		t.Fatalf("regeneration branch %q is unavailable", storyContext.Snapshot.BranchID)
	}
	conversation := NewConversation(store, "", workspace, storyID, branchID, input, 800, cfg).
		WithBaseParentID(branch.Head).
		WithRegenerateTarget(targetTurnID).
		WithExecutionParentPinning()
	submitTestTurnResult(t, conversation, input, input)
	definition := agent.Definition{
		Key: "denova.test.public-game-history", Name: "game", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.public-game-history", Version: 1},
		Middlewares: []agent.Middleware{agentchat.NewModelHistoryProjectionMiddleware(
			toolresult.ResolveContextPolicy(cfg, config.AgentKindInteractiveStory),
		)},
	}
	options := publicGameOptions(workspace, storyID, branchID)
	options.TurnID = targetTurnID
	operation, err := runtime.Start(context.Background(), agentexecution.StartRequest{Cycle: agentexecution.Cycle{
		Definition: definition, Conversation: conversation,
		Request: agentchat.ChatRequest{CommandID: commandID, Message: input, Locale: "zh-CN"}, Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(context.Background())
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != model.narrative {
		t.Fatalf("public Game regeneration outcome = %#v", outcome)
	}
}

func appendPublicGameToolTurn(t *testing.T, store *interactive.Store, storyID, rich string) interactive.TurnEvent {
	t.Helper()
	turn, err := store.AppendTurn(storyID, interactive.AppendTurnRequest{
		BranchID: "main", User: "读取旧档案", Narrative: "旧档案已经用于故事。",
		ModelContextMessages: []interactive.ModelContextMessage{
			{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
				ID: "call-game-history", Type: "function",
				Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/archive.md"}`},
			}}},
			{Role: "tool", ToolCallID: "call-game-history", ToolName: "read", Content: rich,
				ToolResult: &agent.ToolResultSummary{Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func containsMessageContent(messages []*agent.Message, value string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, value) {
			return true
		}
	}
	return false
}

func containsInteractiveToolResult(snapshot interactive.Snapshot, value string) bool {
	for _, turn := range snapshot.Turns {
		for _, message := range turn.ModelContextMessages {
			if message.Role == "tool" && strings.Contains(message.Content, value) {
				return true
			}
		}
	}
	return false
}

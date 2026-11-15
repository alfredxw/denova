package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	appsettings "denova/internal/app/settings"
	apptask "denova/internal/app/task"
	"denova/internal/interactive"
)

func TestGameDoesNotExposeGoalForAnyRuntime(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Game without Goal", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main"}
	for _, kind := range []config.RuntimeID{config.RuntimeNative, config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(kind), func(t *testing.T) {
			current, err := application.ConversationConfig(t.Context(), binding)
			if err != nil {
				t.Fatal(err)
			}
			next := current.Config
			next.Runtime = &config.RuntimeSelection{Kind: kind}
			if kind == config.RuntimeCodex {
				next.Runtime.Codex = &config.CodexRuntimeSettings{Model: "fixture"}
			}
			if kind == config.RuntimeClaude {
				next.Runtime.Claude = &config.ClaudeRuntimeSettings{Model: "fixture"}
			}
			if _, err := application.interactive.SetBranchRuntimeConfig(story.ID, "main", next, current.Revision); err != nil {
				t.Fatal(err)
			}
			if _, _, err := application.ConversationGoal(t.Context(), binding); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
				t.Fatalf("Game Goal read was not rejected: %v", err)
			}
			if _, err := application.MutateConversationGoal(t.Context(), binding, ConversationGoalMutation{Action: "set", Objective: "Continue automatically"}); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
				t.Fatalf("Game Goal mutation was not rejected: %v", err)
			}
		})
	}
}

func TestGameRuntimeSwitchAdmissionAndCrashProjection(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Game switch admission", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main"}
	initial, err := application.ConversationConfig(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	externalConfig := initial.Config
	externalConfig.Runtime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	saved, err := application.interactive.SetBranchRuntimeConfig(story.ID, "main", externalConfig, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	selection := ConversationConfigPatch{Runtime: &config.RuntimeSelection{Kind: config.RuntimeNative}}
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.activeInteractiveRun = &interactiveTaskRun{task: task, info: InteractiveTaskInfo{ProjectID: binding.ProjectID, Workspace: application.workspace, StoryID: story.ID, BranchID: "main"}}
	application.mu.Unlock()
	if _, err := application.PatchConversationConfig(t.Context(), binding, selection, saved.Revision); !errors.Is(err, ErrAgentOperationActive) {
		t.Fatalf("active Game switch: %v", err)
	}
	task.RejectStart(errors.New("test finished"))
	if _, err := application.PatchConversationConfig(t.Context(), binding, selection, initial.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("stale Game switch: %v", err)
	}
	native, err := application.PatchConversationConfig(t.Context(), binding, selection, saved.Revision)
	if err != nil || native.Engine().Kind != config.RuntimeNative {
		t.Fatalf("idle Game switch: %+v %v", native, err)
	}
	saved, err = application.interactive.SetBranchRuntimeConfig(story.ID, "main", externalConfig, native.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate process loss immediately after input acceptance, before an
	// interruption can be written. Recovery derives from the existing input.
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{CommandID: "crashed", OperationID: "external-game-crashed", Cycle: 1}, "main", "Continue")
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.WithContextOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.interactive.CommitPlayerInput(story.ID, intent); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PatchConversationConfig(t.Context(), binding, selection, saved.Revision); !errors.Is(err, agentruntime.ErrOperationActive) {
		t.Fatalf("unfinished Game switch: %v", err)
	}
	view := application.InteractiveAgentActiveView(t.Context(), story.ID, "main")
	if !view.RuntimeProjectionOK || view.PendingInterruptionID == "" {
		t.Fatalf("crash is not recoverable: %+v", view)
	}
	if _, err := application.interactiveService().resolveInteractiveStart(InteractiveAgentStartRequest{CommandID: "resume", Message: "Continue", StoryID: story.ID, BranchID: "main", ResumeInterruptionID: view.PendingInterruptionID}); err != nil {
		t.Fatalf("resume admission: %v", err)
	}
	cycle, err := application.interactiveService().prepareInteractiveAgentCycle(t.Context(), interactiveAgentCycleRequest{CommandID: "resume", Message: "Continue", StoryID: story.ID, BranchID: "main", ResumeInterruptionID: view.PendingInterruptionID})
	if err != nil {
		t.Fatal(err)
	}
	if cycle.externalAssembly == nil || cycle.definition.Model != nil || cycle.request.InputVisibility != "model_only" {
		t.Fatal("opening resume lost its product context or constructed a Native model")
	}
}

func TestConversationConfigUpdateDoesNotMutatePreparedCycle(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		OpenAIModel:       "test-model",
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			IDE: config.AgentModelOverride{ThinkingLevel: "medium"},
		},
	}
	sess, initial, err := agentconversation.GetOrCreateSession(store, "writing", &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}

	preparedCycle := runtimeCfg
	if _, err := agentconversation.ApplySession(sess, &preparedCycle, config.AgentKindIDE); err != nil {
		t.Fatal(err)
	}
	next := initial.Config
	next.ThinkingLevel = "high"
	next.ApprovalMode = config.AgentApprovalFullAccess
	if _, err := sess.SetRuntimeConfig(next, initial.Revision); err != nil {
		t.Fatal(err)
	}

	preparedModel := config.ResolveAgentModel(&preparedCycle, config.AgentKindIDE)
	if preparedModel.ThinkingLevel != "medium" || preparedCycle.AgentApprovalMode != config.AgentApprovalWrite {
		t.Fatalf("prepared cycle changed after conversation update: model=%#v approval=%q", preparedModel, preparedCycle.AgentApprovalMode)
	}
	followingCycle := runtimeCfg
	if _, err := agentconversation.ApplySession(sess, &followingCycle, config.AgentKindIDE); err != nil {
		t.Fatal(err)
	}
	followingModel := config.ResolveAgentModel(&followingCycle, config.AgentKindIDE)
	if followingModel.ThinkingLevel != "high" || followingCycle.AgentApprovalMode != config.AgentApprovalFullAccess {
		t.Fatalf("following cycle did not use updated conversation config: model=%#v approval=%q", followingModel, followingCycle.AgentApprovalMode)
	}
}

func TestAppAllowsConversationConfigUpdatesDuringWritingAndGameRuns(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	application.mu.RLock()
	projectID := application.cfg.ProjectID
	workspace := application.workspace
	sess := application.session
	application.mu.RUnlock()

	writingBinding := ConversationConfigBinding{
		Mode: ConversationModeWriting, ProjectID: projectID, SessionID: sess.ID,
	}
	writingConfig, err := application.ConversationConfig(context.Background(), writingBinding)
	if err != nil {
		t.Fatal(err)
	}
	writingTask, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.activeWritingRun = &writingTaskRun{
		task: writingTask, runtime: ideChatRuntime{workspace: workspace, sess: sess},
	}
	application.mu.Unlock()
	approvalMode := config.AgentApprovalFullAccess
	updatedWriting, err := application.PatchConversationConfig(context.Background(), writingBinding, ConversationConfigPatch{
		ApprovalMode: &approvalMode,
	}, writingConfig.Revision)
	writingTask.RejectStart(errors.New("test complete"))
	if err != nil {
		t.Fatalf("update writing config during active run: %v", err)
	}
	if updatedWriting.Revision != writingConfig.Revision+1 || updatedWriting.ApprovalMode != approvalMode {
		t.Fatalf("updated writing config = %#v", updatedWriting)
	}

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Active config", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	gameBinding := ConversationConfigBinding{
		Mode: ConversationModeInteractive, ProjectID: projectID, StoryID: story.ID, BranchID: "main",
	}
	gameConfig, err := application.ConversationConfig(context.Background(), gameBinding)
	if err != nil {
		t.Fatal(err)
	}
	gameTask, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.activeInteractiveRun = &interactiveTaskRun{
		task: gameTask,
		info: InteractiveTaskInfo{ProjectID: projectID, Workspace: workspace, StoryID: story.ID, BranchID: "main"},
	}
	application.mu.Unlock()
	thinkingLevel := "high"
	updatedGame, err := application.PatchConversationConfig(context.Background(), gameBinding, ConversationConfigPatch{
		ThinkingLevel: &thinkingLevel,
	}, gameConfig.Revision)
	gameTask.RejectStart(errors.New("test complete"))
	if err != nil {
		t.Fatalf("update game config during active run: %v", err)
	}
	if updatedGame.Revision != gameConfig.Revision+1 || updatedGame.ThinkingLevel != thinkingLevel {
		t.Fatalf("updated game config = %#v", updatedGame)
	}
}

func TestNewConversationUsesModelDefaultDespiteOlderSessionActivity(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			IDE:     config.AgentModelOverride{ThinkingLevel: "medium"},
			General: config.AgentModelOverride{ThinkingLevel: "low"},
		},
	}

	first, firstSnapshot, err := agentconversation.GetOrCreateSession(store, "writing-first", &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	firstSelection := firstSnapshot.Config
	firstSelection.ThinkingLevel = "high"
	firstSelection.ApprovalMode = config.AgentApprovalFullAccess
	configuredFirst, err := first.SetRuntimeConfig(firstSelection, firstSnapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}

	_, inherited, err := agentconversation.GetOrCreateSession(store, "writing-second", &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	expected := configuredFirst.Config
	expected.ThinkingLevel = "medium"
	if inherited.Revision != 1 || inherited.Config != expected {
		t.Fatalf("new writing conversation did not use its model default: %#v", inherited)
	}

	firstAfter, ok := first.RuntimeConfig()
	if !ok || firstAfter != configuredFirst {
		t.Fatalf("creating a new conversation changed the older one: %#v", firstAfter)
	}

	_, general, err := agentconversation.GetOrCreateSession(store, "general-first", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if general.AgentKind != config.AgentKindGeneral || general.ThinkingLevel != "low" || general.ApprovalMode != config.AgentApprovalWrite {
		t.Fatalf("different Agent kind should use its own default/history: %#v", general)
	}
}

func TestLegacyConversationInitializesOnceAndThenStaysIndependent(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := store.GetOrCreate("legacy-writing")
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			IDE: config.AgentModelOverride{ThinkingLevel: "medium"},
		},
	}
	initialized, err := agentconversation.EnsureSession(legacy, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}

	runtimeCfg.AgentApprovalMode = config.AgentApprovalAsk
	runtimeCfg.AgentModels.IDE.ThinkingLevel = "max"
	again, err := agentconversation.EnsureSession(legacy, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	if again != initialized {
		t.Fatalf("initialized old conversation followed later Settings: before=%#v after=%#v", initialized, again)
	}
}

func TestConversationConfigPreviewDoesNotPersistDraftSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.Config{
		AgentApprovalMode: config.AgentApprovalWrite,
		AgentModels: config.AgentModelSettings{
			General: config.AgentModelOverride{ThinkingLevel: "medium"},
		},
	}
	snapshot, err := agentconversation.PreviewSession(store, "local-draft", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || snapshot.AgentKind != config.AgentKindGeneral || snapshot.ThinkingLevel != "medium" {
		t.Fatalf("draft preview = %#v", snapshot)
	}
	if store.Exists("local-draft") {
		t.Fatal("reading draft configuration must not create a durable session")
	}
	_, persisted, err := agentconversation.GetOrCreateSession(store, "local-draft", &runtimeCfg, config.AgentKindGeneral)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != 1 || !store.Exists("local-draft") {
		t.Fatalf("persisted draft configuration = %#v", persisted)
	}
}

func TestInteractiveConversationConfigPreviewUsesModelDefaultWithoutCreatingStory(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	application.mu.RLock()
	projectID := application.cfg.ProjectID
	application.mu.RUnlock()

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Configured opening", StoryTellerID: "classic", ProfileID: "default", ThinkingLevel: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	thinking := "medium"
	if _, err := application.PatchConversationConfig(context.Background(), ConversationConfigBinding{
		Mode: ConversationModeInteractive, ProjectID: projectID,
	}, ConversationConfigPatch{ThinkingLevel: &thinking}, 0); err != nil {
		t.Fatal(err)
	}
	before, err := application.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}

	preview, err := application.ConversationConfig(context.Background(), ConversationConfigBinding{
		Mode: ConversationModeInteractive, ProjectID: projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Revision != 0 || preview.ProfileID != "default" || preview.ThinkingLevel != thinking {
		t.Fatalf("new story runtime preview = %#v", preview)
	}
	after, err := application.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Stories) != len(before.Stories) || after.CurrentStoryID != story.ID {
		t.Fatalf("preview changed story index: before=%#v after=%#v", before, after)
	}
}

func TestInteractiveStoryCreationPersistsOpeningRuntimeSelection(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	application.mu.RLock()
	projectID := application.cfg.ProjectID
	application.mu.RUnlock()

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Explicit opening runtime", StoryTellerID: "classic", ProfileID: "default", ThinkingLevel: "max",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := application.ConversationConfig(context.Background(), ConversationConfigBinding{
		Mode: ConversationModeInteractive, ProjectID: projectID, StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 1 || snapshot.ProfileID != "default" || snapshot.ThinkingLevel != "max" {
		t.Fatalf("created story runtime config = %#v", snapshot)
	}
}

func TestGameStaleRuntimeSwitchPreservesSuspendedControllerAndResumePath(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{
		"model_endpoints": [{"id":"ep-1","provider":"anthropic","base_url":"http://127.0.0.1:1","api_key":"test-key"}],
		"model_profiles": [
			{"id":"primary","endpoint_id":"ep-1","model":"claude-primary"},
			{"id":"alternate","endpoint_id":"ep-1","model":"claude-alternate"}
		]
	}`), ""); err != nil {
		t.Fatal(err)
	}
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Suspended stale switch", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main"}
	initial, err := application.ConversationConfig(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	externalConfig := initial.Config
	externalConfig.Runtime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{ProfileID: "primary"}}
	saved, err := application.interactive.SetBranchRuntimeConfig(story.ID, "main", externalConfig, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}

	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: "interrupted-cmd", OperationID: "external-game-interrupted-cmd", Cycle: 1,
	}, "main", "Open the gate")
	if err != nil {
		t.Fatal(err)
	}
	input, err := application.interactive.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := application.interactive.MarkTurnInterrupted(story.ID, "main", input.Event.ID, intent.Text, "The gate creaks.", "connection_lost")
	if err != nil {
		t.Fatal(err)
	}

	_, options, err := application.interactiveSessionRuntime(binding)
	if err != nil {
		t.Fatal(err)
	}
	controller, selected, err := application.externalController(options)
	if err != nil || !selected {
		t.Fatalf("externalController failed: selected=%t err=%v", selected, err)
	}
	startedRun, err := controller.Start(t.Context(), agentruntime.ExternalCycleInput{
		Request: agentchat.ChatRequest{CommandID: "interrupted-cmd", Message: "Open the gate"},
	}, func(_ context.Context, _ agentruntime.ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		return nil, context.Canceled
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Submit(t.Context(), agentruntime.Command{
		Kind: agentexecution.CommandSuspend, CommandID: "suspend-cmd", OperationID: startedRun.Receipt().OperationID,
	}); err != nil {
		t.Fatal(err)
	}
	startedRun.Wait(t.Context())

	beforeView := application.InteractiveAgentActiveView(t.Context(), story.ID, "main")
	if !beforeView.RuntimeProjectionOK || beforeView.Runtime.Phase != agentrun.RunPhaseSuspended || beforeView.PendingInterruptionID != interrupted.ID {
		t.Fatalf("unexpected pre-switch active view: %+v", beforeView)
	}

	// Attempt to switch model settings with a stale revision.
	stalePatch := ConversationConfigPatch{Claude: &config.ClaudeRuntimeSettings{ProfileID: "alternate"}}
	if _, err := application.PatchConversationConfig(t.Context(), binding, stalePatch, initial.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict for stale switch, got %v", err)
	}

	// Simulate an underlying Story journal append failure with the valid
	// revision. Making the canonical JSONL read-only fails the O_RDWR append
	// open on every supported platform for non-root runners, unlike the former
	// domain-lock-directory trick that macOS can still open and lock.
	storyJournalPath := interactiveStoryJournalPath(t, application.interactive.Root(), story.ID)
	beforeSize := mustStoryJournalSize(t, storyJournalPath)
	makeStoryJournalReadOnly(t, storyJournalPath)
	if _, err := application.PatchConversationConfig(t.Context(), binding, stalePatch, saved.Revision); err == nil {
		t.Fatal("expected write failure when the story journal is not writable")
	}
	restoreStoryJournalWritable(t, storyJournalPath)
	if size := mustStoryJournalSize(t, storyJournalPath); size != beforeSize {
		t.Fatalf("failed switch touched the canonical journal: before=%d after=%d", beforeSize, size)
	}

	// Branch config, paused controller state, and resume entrypoint must all remain intact.
	currentCfg, err := application.ConversationConfig(t.Context(), binding)
	if err != nil || currentCfg.Revision != saved.Revision || currentCfg.Engine().ModelProfileID() != "primary" {
		t.Fatalf("failed switches mutated branch config: %+v err=%v", currentCfg, err)
	}
	afterFailView := application.InteractiveAgentActiveView(t.Context(), story.ID, "main")
	if !afterFailView.RuntimeProjectionOK || afterFailView.Runtime.Phase != agentrun.RunPhaseSuspended ||
		afterFailView.Runtime.ActiveCommandID != "interrupted-cmd" || afterFailView.PendingInterruptionID != interrupted.ID {
		t.Fatalf("failed switches broke suspended runtime projection or resume view: %+v", afterFailView)
	}
	recoveryActions := agentexecution.RuntimeRecoveryActions(afterFailView.Runtime)
	if len(recoveryActions) == 0 || recoveryActions[0].Kind != agentexecution.RuntimeRecoveryResume || recoveryActions[0].OperationID != startedRun.Receipt().OperationID {
		t.Fatalf("failed switches broke suspended recovery actions: %+v", recoveryActions)
	}
	recoveryInput, err := controller.RecoveryInput(t.Context())
	if err != nil || recoveryInput.Request.CommandID != "interrupted-cmd" {
		t.Fatalf("failed switches cleared recovery input: %+v err=%v", recoveryInput, err)
	}

	// Same-process retry is still rejected: the in-process journal handle is
	// deliberately poisoned after an unknown append failure until reopened.
	_, err = application.PatchConversationConfig(t.Context(), binding, stalePatch, saved.Revision)
	requireReopenJournalError(t, err)
	// Reopen the Story journal over the untouched canonical file: rebuilding
	// the product runtime drops the poisoned handle and replays the unchanged
	// journal. All following assertions run against the reopened runtime.
	application = reopenConversationTestApp(t, application)

	// Confirm valid revision commits config + controller detach in one transaction while retaining the canonical interruption.
	switched, err := application.PatchConversationConfig(t.Context(), binding, stalePatch, saved.Revision)
	if err != nil {
		t.Fatalf("valid switch after failed switches: %v", err)
	}
	if switched.Revision != saved.Revision+1 || switched.Engine().ModelProfileID() != "alternate" {
		t.Fatalf("unexpected switched config: %+v", switched)
	}
	afterSwitchView := application.InteractiveAgentActiveView(t.Context(), story.ID, "main")
	if !afterSwitchView.RuntimeProjectionOK || afterSwitchView.Runtime.Phase != agentrun.RunPhaseIdle || afterSwitchView.PendingInterruptionID != interrupted.ID {
		t.Fatalf("valid switch did not cleanly detach controller while retaining interruption: %+v", afterSwitchView)
	}
	if afterSwitchView.Runtime.Cursor != beforeView.Runtime.Cursor+1 {
		t.Fatalf("config switch and controller detach did not commit cleanly in one step: before=%d after=%d", beforeView.Runtime.Cursor, afterSwitchView.Runtime.Cursor)
	}
}

func TestGameFailedSwitchAllowsOriginalSuspendedTurnToResumeToCompletion(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	if _, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser, []byte(`{
		"model_endpoints": [{"id":"ep-1","provider":"anthropic","base_url":"http://127.0.0.1:1","api_key":"test-key"}],
		"model_profiles": [
			{"id":"primary","endpoint_id":"ep-1","model":"claude-primary"},
			{"id":"alternate","endpoint_id":"ep-1","model":"claude-alternate"}
		]
	}`), ""); err != nil {
		t.Fatal(err)
	}
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Resume after failed switch", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main"}
	initial, err := application.ConversationConfig(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	externalConfig := initial.Config
	externalConfig.Runtime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{ProfileID: "primary"}}
	saved, err := application.interactive.SetBranchRuntimeConfig(story.ID, "main", externalConfig, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}

	_, options, err := application.interactiveSessionRuntime(binding)
	if err != nil {
		t.Fatal(err)
	}
	controller, selected, err := application.externalController(options)
	if err != nil || !selected {
		t.Fatalf("externalController failed: selected=%t err=%v", selected, err)
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: "interrupted-cmd", OperationID: "external-game-interrupted-cmd", Cycle: 1,
	}, "main", "Open the gate")
	if err != nil {
		t.Fatal(err)
	}
	input, err := application.interactive.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.interactive.MarkTurnInterrupted(story.ID, "main", input.Event.ID, intent.Text, "The gate creaks.", "connection_lost"); err != nil {
		t.Fatal(err)
	}

	startedRun, err := controller.Start(t.Context(), agentruntime.ExternalCycleInput{
		Request: agentchat.ChatRequest{CommandID: "interrupted-cmd", Message: "Open the gate"},
	}, func(_ context.Context, _ agentruntime.ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		return nil, context.Canceled
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Submit(t.Context(), agentruntime.Command{
		Kind: agentexecution.CommandSuspend, CommandID: "suspend-cmd", OperationID: startedRun.Receipt().OperationID,
	}); err != nil {
		t.Fatal(err)
	}
	startedRun.Wait(t.Context())

	patch := ConversationConfigPatch{Claude: &config.ClaudeRuntimeSettings{ProfileID: "alternate"}}
	if _, err := application.PatchConversationConfig(t.Context(), binding, patch, initial.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	storyJournalPath := interactiveStoryJournalPath(t, application.interactive.Root(), story.ID)
	beforeSize := mustStoryJournalSize(t, storyJournalPath)
	makeStoryJournalReadOnly(t, storyJournalPath)
	if _, err := application.PatchConversationConfig(t.Context(), binding, patch, saved.Revision); err == nil {
		t.Fatal("expected write failure when the story journal is not writable")
	}
	restoreStoryJournalWritable(t, storyJournalPath)
	if size := mustStoryJournalSize(t, storyJournalPath); size != beforeSize {
		t.Fatalf("failed switch touched the canonical journal: before=%d after=%d", beforeSize, size)
	}

	// Resume the original suspended turn after both stale conflict and write failure.
	view := application.InteractiveAgentActiveView(t.Context(), story.ID, "main")
	actions := agentexecution.RuntimeRecoveryActions(view.Runtime)
	if len(actions) == 0 || actions[0].Kind != agentexecution.RuntimeRecoveryResume {
		t.Fatalf("missing resume action after failed switch: %+v", actions)
	}
	// Same-process resume is still rejected: the in-process journal handle is
	// deliberately poisoned after an unknown append failure until reopened.
	_, err = controller.Resume(t.Context(), actions[0], func(_ context.Context, cycleInput agentruntime.ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		return nil, context.Canceled
	}, nil)
	requireReopenJournalError(t, err)
	// Reopen the Story journal over the untouched canonical file: rebuilding
	// the product runtime drops the poisoned handle and replays the unchanged
	// journal, then the recovered turn resumes on the reopened runtime.
	application = reopenConversationTestApp(t, application)
	_, options, err = application.interactiveSessionRuntime(binding)
	if err != nil {
		t.Fatal(err)
	}
	controller, selected, err = application.externalController(options)
	if err != nil || !selected {
		t.Fatalf("externalController after reopen failed: selected=%t err=%v", selected, err)
	}
	resumedCmd := ""
	resumedRun, err := controller.Resume(t.Context(), actions[0], func(_ context.Context, cycleInput agentruntime.ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		resumedCmd = cycleInput.Request.CommandID
		return testExternalCycleFunc(func(context.Context) agentrun.Outcome {
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}, nil)
	if err != nil {
		t.Fatalf("failed to resume original turn after failed config switch: %v", err)
	}
	if outcome := resumedRun.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resumed turn did not complete: %+v", outcome)
	}
	if resumedCmd != "interrupted-cmd" {
		t.Fatalf("resumed wrong command: %q", resumedCmd)
	}
}

type testExternalCycleFunc func(context.Context) agentrun.Outcome

func (fn testExternalCycleFunc) Wait(ctx context.Context) agentrun.Outcome { return fn(ctx) }

// reopenConversationTestApp rebuilds the product runtime over the same data
// directory. This is the way a poisoned Story journal handle is reopened: the
// fresh Store replays the unchanged canonical file, dropping the deliberately
// invalid in-process projection.
func reopenConversationTestApp(t *testing.T, application *App) *App {
	t.Helper()
	application.mu.RLock()
	workspace := application.workspace
	application.mu.RUnlock()
	// The reopened runtime must see the same settings layer, so it binds to
	// the same isolated data directory regardless of the caller's env setup.
	t.Setenv("DENOVA_DIR", workspace)
	t.Setenv("NOVA_DIR", workspace)
	reopened, err := New(t.Context(), &config.Config{
		OpenAIModel: "test-model", NovaDir: workspace, Workspace: workspace,
		OpenAIBaseURL: "http://127.0.0.1:1/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	return reopened
}

// interactiveStoryJournalPath locates the canonical Story journal backing a
// game, matching interactive.Store.storyPath without crossing the package
// boundary.
func interactiveStoryJournalPath(t *testing.T, interactiveRoot, storyID string) string {
	t.Helper()
	return filepath.Join(interactiveRoot, "interactive", "story", "story-"+storyID+".jsonl")
}

// mustStoryJournalSize returns the physical size of one canonical Story
// journal, used to prove that a failed append changed nothing on disk.
func mustStoryJournalSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// makeStoryJournalReadOnly marks one canonical Story journal read-only and
// guarantees writability is restored when the test ends. os.Chmod(0444) maps
// to the read-only attribute on Windows and to mode bits on POSIX, so the
// O_RDWR append open fails for non-root test users on every supported
// platform. When the injection cannot take effect (for example a root user on
// POSIX), the test skips instead of failing on a misleading "expected write
// failure" that looks like a product regression.
func makeStoryJournalReadOnly(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	probe, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err == nil {
		_ = probe.Close()
		t.Skip("cannot inject append failure: journal still writable under chmod 0444 (running as root?)")
	}
}

// restoreStoryJournalWritable undoes makeStoryJournalReadOnly so subsequent
// assertions run against a writable file.
func restoreStoryJournalWritable(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
}

// requireReopenJournalError asserts the deliberate poison contract of one
// Story journal handle: after an actual append failure, every same-process
// write is rejected until the journal is reopened.
func requireReopenJournalError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "reopen the journal") {
		t.Fatalf("operation after append failure must require reopening the journal: %v", err)
	}
}

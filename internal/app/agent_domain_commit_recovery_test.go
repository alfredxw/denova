package app

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
	compactionapp "denova/internal/app/compaction"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestAppReconcilesExactSessionDomainReceiptsAcrossProfiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "book")
	state := book.NewState(workspace)
	if err := state.InitWorkspace(); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("reconcile-session")
	if err != nil {
		t.Fatal(err)
	}
	identity := session.DomainCommitIdentity{CommandID: "command", OperationID: "operation", Cycle: 2}
	intent, err := session.NewDomainCommitIntent(identity, agents.AssistantMessage("canonical", nil), session.MessageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := sess.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: &config.Config{NovaDir: root}}

	bindings := []agentrun.RuntimeBinding{
		{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sess.ID},
		{AgentKind: agentrun.AgentKindConfigManager, Workspace: workspace, SessionID: sess.ID},
		{AgentKind: agentrun.AgentKindImage, Workspace: workspace, SessionID: sess.ID},
		{AgentKind: agentrun.AgentKindAutomation, Workspace: workspace, SessionID: sess.ID, TaskID: "task"},
	}
	for _, binding := range bindings {
		binding := binding
		t.Run(binding.AgentKind, func(t *testing.T) {
			result, err := application.reconcileHarnessDomainCommit(context.Background(), domainRecoveryRequest(binding, identity.CommandID, identity.OperationID, identity.Cycle, agentrun.DomainCommitOutput, intent.Hash))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Found || result.Revision != fmt.Sprint(receipt.ContextRevision) {
				t.Fatalf("reconcile result = %#v", result)
			}
		})
	}

	wrongIdentity := domainRecoveryRequest(bindings[0], identity.CommandID, "other-operation", identity.Cycle, agentrun.DomainCommitOutput, intent.Hash)
	if _, err := application.reconcileHarnessDomainCommit(context.Background(), wrongIdentity); !errors.Is(err, session.ErrDomainCommitIdentityConflict) {
		t.Fatalf("identity mismatch error = %v, want Session conflict", err)
	}
	wrongHash := domainRecoveryRequest(bindings[0], identity.CommandID, identity.OperationID, identity.Cycle, agentrun.DomainCommitOutput, "sha256:other")
	if _, err := application.reconcileHarnessDomainCommit(context.Background(), wrongHash); !errors.Is(err, session.ErrDomainCommitIdentityConflict) {
		t.Fatalf("hash mismatch error = %v, want Session conflict", err)
	}
}

func TestAppReconcilesGlobalAutomationSessionReceipt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := session.NewStore(filepath.Join(root, "automations", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("global-run")
	if err != nil {
		t.Fatal(err)
	}
	identity := session.DomainCommitIdentity{CommandID: "global-command", OperationID: "global-operation", Cycle: 1}
	intent, err := session.NewDomainCommitIntent(identity, agents.UserMessage("accepted input"), session.MessageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(intent); err != nil {
		t.Fatal(err)
	}
	binding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindAutomation, SessionID: sess.ID, TaskID: "task"}
	result, err := (&App{cfg: &config.Config{NovaDir: root}}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(binding, identity.CommandID, identity.OperationID, identity.Cycle, agentrun.DomainCommitInput, intent.Hash),
	)
	if err != nil || !result.Found || result.Revision == "" {
		t.Fatalf("global automation reconciliation = %#v err=%v", result, err)
	}
}

func TestAppReconcilesExactGameAndDirectorReceipts(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "恢复查询", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	gameRequest := interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "canonical turn",
		AgentCommandID: "game-command", AgentOperationID: "game-operation", AgentCycle: 3,
	}
	gameIntent, err := interactive.NewDomainCommitIntent(gameRequest)
	if err != nil {
		t.Fatal(err)
	}
	playerInput, err := interactive.NewPlayerInputIntent(gameIntent.Identity, gameRequest.BranchID, gameRequest.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, playerInput); err != nil {
		t.Fatal(err)
	}
	gameReceipt, err := store.CommitDomainTurn(story.ID, gameIntent)
	if err != nil {
		t.Fatal(err)
	}
	gameBinding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	gameResult, err := (&App{}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(gameBinding, gameRequest.AgentCommandID, gameRequest.AgentOperationID, gameRequest.AgentCycle, agentrun.DomainCommitOutput, gameIntent.Hash),
	)
	if err != nil || !gameResult.Found || gameResult.Revision != gameReceipt.Revision {
		t.Fatalf("game reconciliation = %#v err=%v", gameResult, err)
	}
	conflict := domainRecoveryRequest(gameBinding, gameRequest.AgentCommandID, gameRequest.AgentOperationID, gameRequest.AgentCycle, agentrun.DomainCommitOutput, "sha256:other")
	if _, err := (&App{}).reconcileHarnessDomainCommit(context.Background(), conflict); !errors.Is(err, interactive.ErrAgentTurnIdentityConflict) {
		t.Fatalf("game mismatch error = %v", err)
	}

	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, gameReceipt.Turn.ID); err != nil {
		t.Fatal(err)
	}
	plan, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	directorIdentity := interactive.DirectorPlanDomainCommitIdentity{CommandID: "director-command", OperationID: "director-operation", Cycle: 1}
	directorIntent, err := interactive.NewDirectorPlanDomainCommitIntent(directorIdentity, token, gameReceipt.Turn.ID, `{"mode":"keep"}`, plan.Docs)
	if err != nil {
		t.Fatal(err)
	}
	directorReceipt, err := store.CommitDirectorPlanRun(directorIntent)
	if err != nil {
		t.Fatal(err)
	}
	directorBinding := agentrun.RuntimeBinding{AgentKind: config.AgentKindInteractiveDirector, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	directorResult, err := (&App{}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(directorBinding, directorIdentity.CommandID, directorIdentity.OperationID, directorIdentity.Cycle, agentrun.DomainCommitOutput, directorIntent.Hash),
	)
	if err != nil || !directorResult.Found || directorResult.Revision != directorReceipt.Revision {
		t.Fatalf("Director reconciliation = %#v err=%v", directorResult, err)
	}
}

func TestAppReconcilesStableSessionAndStoryCompactions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "book")
	state := book.NewState(workspace)
	if err := state.InitWorkspace(); err != nil {
		t.Fatal(err)
	}
	sessionStore, err := session.NewStore(state.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sessionStore.GetOrCreate("structural")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agents.UserMessage("history")); err != nil {
		t.Fatal(err)
	}
	sessionCommand := agentrun.CommandID("session-compact-command")
	sessionRecord := session.ContextCompaction{
		ID: agentstructural.RecordID("cc", string(sessionCommand)),
		CompactionCheckpoint: agentcompaction.NewCheckpoint("ide", agentcompaction.Result{
			Epoch: 1, Summary: "summary", RetainedTurns: 1,
			TokensBefore: 100, TokensAfter: 20, ContextWindowTokens: 1000, Threshold: .8,
			TriggerReason: "manual", Phase: "manual",
		}),
		SourceEndIndex: 1, SourceMessageCount: 1,
	}
	sessionCursor := sess.ContextCursor()
	sessionBinding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sess.ID}
	sessionRef := agentrun.ContextCompactionRef{
		Resource: sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", sessionCursor.Revision),
	}
	sessionRequest := structuralRecoveryRequest(
		t, sessionBinding, sessionCommand, "session-compact-operation", agentstructural.DomainSession,
		agentstructural.Compact, agentrun.StructuralCompactContext, sessionRef,
		agentstructural.Result{Compaction: compactionapp.ResultFromSession(sessionRecord)}, sessionRecord,
	)
	committedSession, err := sess.AppendContextCompactionAt(sessionCursor, sessionRecord)
	if err != nil {
		t.Fatal(err)
	}
	sessionResult, err := (&App{cfg: &config.Config{NovaDir: root}}).reconcileHarnessDomainCommit(context.Background(), sessionRequest)
	if err != nil || !sessionResult.Found || sessionResult.Revision != fmt.Sprintf("session-context:%d", committedSession.ContextRevision) {
		t.Fatalf("Session structural reconciliation = %#v err=%v", sessionResult, err)
	}
	sessionRemoveCommand := agentrun.CommandID("session-remove-command")
	sessionRemoval := session.ContextCompactionRemoval{
		ID: agentstructural.RecordID("ccr", string(sessionRemoveCommand)), AgentKind: "ide",
		CompactionID: committedSession.ID, SourceStartIndex: committedSession.SourceStartIndex,
		SourceEndIndex: committedSession.SourceEndIndex, Reason: "user_removed",
	}
	sessionRemovalCursor := sess.ContextCursor()
	sessionRemoveRequest := structuralRecoveryRequest(
		t, sessionBinding, sessionRemoveCommand, "session-remove-operation", agentstructural.DomainSession,
		agentstructural.Remove, agentrun.StructuralRemoveCompaction,
		agentrun.ContextCompactionRef{
			Resource: sess.ID, CompactionID: committedSession.ID,
			ExpectedRevision: fmt.Sprintf("session-context:%d", sessionRemovalCursor.Revision),
		},
		agentstructural.Result{Removed: true}, sessionRemoval,
	)
	committedRemoval, removed, err := sess.CommitContextCompactionRemovalAt(sessionRemovalCursor, sessionRemoval)
	if err != nil || !removed {
		t.Fatalf("commit Session removal: removed=%v err=%v", removed, err)
	}
	sessionRemoveResult, err := (&App{cfg: &config.Config{NovaDir: root}}).reconcileHarnessDomainCommit(context.Background(), sessionRemoveRequest)
	if err != nil || !sessionRemoveResult.Found || sessionRemoveResult.Revision != fmt.Sprintf("session-context:%d", committedRemoval.ContextRevision) {
		t.Fatalf("Session removal reconciliation = %#v err=%v", sessionRemoveResult, err)
	}

	storyStore := interactive.NewStore(workspace)
	story, err := storyStore.CreateStory(interactive.CreateStoryRequest{Title: "结构恢复", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	storyContext, err := storyStore.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedParent := storyContext.Meta.Branches["main"].Head
	storyCommand := agentrun.CommandID("story-compact-command")
	storyRecord := interactive.ContextCompactionEvent{
		ID: agentstructural.RecordID("cc", string(storyCommand)),
		CompactionCheckpoint: agentcompaction.NewCheckpoint("interactive_story", agentcompaction.Result{
			Epoch: 1, Summary: "story summary", RetainedTurns: 1,
			TokensBefore: 100, TokensAfter: 30, ContextWindowTokens: 1000, Threshold: .8,
			TriggerReason: "manual", Phase: "manual",
		}),
		SourceTurnCount: 1, ExpectedParentID: &expectedParent,
	}
	storyBinding := agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	storyRequest := structuralRecoveryRequest(
		t, storyBinding, storyCommand, "story-compact-operation", agentstructural.DomainStory,
		agentstructural.Compact, agentrun.StructuralCompactContext,
		agentrun.ContextCompactionRef{Resource: story.ID + "/main", ExpectedRevision: agentstructural.StoryRevision(expectedParent)},
		agentstructural.Result{Compaction: compactionapp.ResultFromStory(storyRecord)}, storyRecord,
	)
	committedStory, err := storyStore.AppendContextCompaction(story.ID, "main", storyRecord)
	if err != nil {
		t.Fatal(err)
	}
	storyResult, err := (&App{}).reconcileHarnessDomainCommit(context.Background(), storyRequest)
	if err != nil || !storyResult.Found || storyResult.Revision != "story-head:"+committedStory.ID {
		t.Fatalf("Story structural reconciliation = %#v err=%v", storyResult, err)
	}
	storyRequest.Commit.Hash = "sha256:other"
	if _, err := (&App{}).reconcileHarnessDomainCommit(context.Background(), storyRequest); err == nil {
		t.Fatal("Story structural hash mismatch was accepted")
	}
	storyRemoveCommand := agentrun.CommandID("story-remove-command")
	storyExpectedParent := committedStory.ID
	storyRemoval := interactive.ContextCompactionRemovalEvent{
		ID: agentstructural.RecordID("ccr", string(storyRemoveCommand)), AgentKind: "interactive_story",
		CompactionID: committedStory.ID, SourceTurnCount: committedStory.SourceTurnCount,
		Reason: "user_removed", ExpectedParentID: &storyExpectedParent,
	}
	storyRemoveRequest := structuralRecoveryRequest(
		t, storyBinding, storyRemoveCommand, "story-remove-operation", agentstructural.DomainStory,
		agentstructural.Remove, agentrun.StructuralRemoveCompaction,
		agentrun.ContextCompactionRef{
			Resource: story.ID + "/main", CompactionID: committedStory.ID,
			ExpectedRevision: agentstructural.StoryRevision(storyExpectedParent),
		},
		agentstructural.Result{Removed: true}, storyRemoval,
	)
	committedStoryRemoval, err := storyStore.AppendContextCompactionRemoval(story.ID, "main", storyRemoval)
	if err != nil {
		t.Fatal(err)
	}
	storyRemoveResult, err := (&App{}).reconcileHarnessDomainCommit(context.Background(), storyRemoveRequest)
	if err != nil || !storyRemoveResult.Found || storyRemoveResult.Revision != "story-head:"+committedStoryRemoval.ID {
		t.Fatalf("Story removal reconciliation = %#v err=%v", storyRemoveResult, err)
	}
}

func domainRecoveryRequest(
	binding agentrun.RuntimeBinding,
	commandID string,
	operationID string,
	cycle int,
	stage agentrun.DomainCommitStage,
	hash string,
) agentrun.DomainCommitReconcileRequest {
	return agentrun.DomainCommitReconcileRequest{
		Binding: binding,
		Commit: agentrun.DomainCommitState{
			Identity: agentrun.DomainCommitIdentity{
				CommandID: agentrun.CommandID(commandID), OperationID: agentrun.OperationID(operationID),
				Cycle: cycle, Stage: stage,
			},
			Hash: hash,
		},
	}
}

func structuralRecoveryRequest(
	t *testing.T,
	binding agentrun.RuntimeBinding,
	commandID agentrun.CommandID,
	operationID agentrun.OperationID,
	domain agentstructural.Domain,
	action agentstructural.Action,
	kind agentrun.StructuralOperationKind,
	ref agentrun.ContextCompactionRef,
	result agentstructural.Result,
	mutation any,
) agentrun.DomainCommitReconcileRequest {
	t.Helper()
	plan, err := agentstructural.NewRestorePlan(domain, action, binding, ref, agentstructural.RecordID(structuralRecordPrefix(action), string(commandID)), result, mutation)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agentstructural.EncodeRestorePlan(plan, binding, ref.ExpectedRevision)
	if err != nil {
		t.Fatal(err)
	}
	ref.RestoreDescriptor = descriptor
	request := domainRecoveryRequest(binding, string(commandID), string(operationID), 1, agentrun.DomainCommitOutput, plan.IntentHash)
	request.Structural = &agentrun.StructuralOperation{
		Binding: binding, CommandID: commandID, OperationID: operationID, Cycle: 1, Kind: kind, Ref: ref,
	}
	return request
}

func structuralRecordPrefix(action agentstructural.Action) string {
	if action == agentstructural.Remove {
		return "ccr"
	}
	return "cc"
}

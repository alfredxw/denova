package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
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
	intent, err := session.NewDomainCommitIntent(identity, schema.AssistantMessage("canonical", nil), session.MessageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := sess.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: &config.Config{NovaDir: root}}

	bindings := []agentruntime.BindingRef{
		{Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting, Workspace: workspace, SessionID: sess.ID},
		{Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileConfigManager, Workspace: workspace, SessionID: sess.ID},
		{Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileImage, Workspace: workspace, SessionID: sess.ID},
		{Kind: agentruntime.BindingAutomation, Profile: agentruntime.ProfileAutomation, Workspace: workspace, SessionID: sess.ID, TaskID: "task"},
	}
	for _, binding := range bindings {
		binding := binding
		t.Run(string(binding.Profile), func(t *testing.T) {
			result, err := application.reconcileHarnessDomainCommit(context.Background(), domainRecoveryRequest(binding, identity.CommandID, identity.OperationID, identity.Cycle, agentruntime.DomainCommitOutput, intent.Hash))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Found || result.Revision != fmt.Sprint(receipt.ContextRevision) {
				t.Fatalf("reconcile result = %#v", result)
			}
		})
	}

	wrongIdentity := domainRecoveryRequest(bindings[0], identity.CommandID, "other-operation", identity.Cycle, agentruntime.DomainCommitOutput, intent.Hash)
	if _, err := application.reconcileHarnessDomainCommit(context.Background(), wrongIdentity); !errors.Is(err, session.ErrDomainCommitIdentityConflict) {
		t.Fatalf("identity mismatch error = %v, want Session conflict", err)
	}
	wrongHash := domainRecoveryRequest(bindings[0], identity.CommandID, identity.OperationID, identity.Cycle, agentruntime.DomainCommitOutput, "sha256:other")
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
	intent, err := session.NewDomainCommitIntent(identity, schema.UserMessage("accepted input"), session.MessageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(intent); err != nil {
		t.Fatal(err)
	}
	binding := agentruntime.BindingRef{Kind: agentruntime.BindingAutomation, Profile: agentruntime.ProfileAutomation, SessionID: sess.ID, TaskID: "task"}
	result, err := (&App{cfg: &config.Config{NovaDir: root}}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(binding, identity.CommandID, identity.OperationID, identity.Cycle, agentruntime.DomainCommitInput, intent.Hash),
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
	gameBinding := agentruntime.BindingRef{Kind: agentruntime.BindingGame, Profile: agentruntime.ProfileGame, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	gameResult, err := (&App{}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(gameBinding, gameRequest.AgentCommandID, gameRequest.AgentOperationID, gameRequest.AgentCycle, agentruntime.DomainCommitOutput, gameIntent.Hash),
	)
	if err != nil || !gameResult.Found || gameResult.Revision != gameReceipt.Revision {
		t.Fatalf("game reconciliation = %#v err=%v", gameResult, err)
	}
	conflict := domainRecoveryRequest(gameBinding, gameRequest.AgentCommandID, gameRequest.AgentOperationID, gameRequest.AgentCycle, agentruntime.DomainCommitOutput, "sha256:other")
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
	directorBinding := agentruntime.BindingRef{Kind: agentruntime.BindingGame, Profile: agentruntime.ProfileDirector, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	directorResult, err := (&App{}).reconcileHarnessDomainCommit(
		context.Background(),
		domainRecoveryRequest(directorBinding, directorIdentity.CommandID, directorIdentity.OperationID, directorIdentity.Cycle, agentruntime.DomainCommitOutput, directorIntent.Hash),
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
	if err := sess.Append(schema.UserMessage("history")); err != nil {
		t.Fatal(err)
	}
	sessionCommand := agentruntime.CommandID("session-compact-command")
	sessionRecord := session.ContextCompaction{
		ID: contextStructuralRecordID("cc", string(sessionCommand)), AgentKind: "ide", Epoch: 1,
		Summary: "summary", SourceEndIndex: 1, SourceMessageCount: 1, RetainedTurns: 1,
		TokensBefore: 100, TokensAfter: 20, ContextWindowTokens: 1000, Threshold: .8,
		Reason: "manual", Phase: "manual",
	}
	sessionCursor := sess.ContextCursor()
	sessionBinding := agentruntime.BindingRef{Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting, Workspace: workspace, SessionID: sess.ID}
	sessionRef := agentruntime.ContextCompactionRef{
		Resource: sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", sessionCursor.Revision),
	}
	sessionRequest := structuralRecoveryRequest(
		t, sessionBinding, sessionCommand, "session-compact-operation", agent.ContextStructuralDomainSession,
		agent.ContextStructuralCompact, agentruntime.StructuralCompactContext, sessionRef,
		agent.ContextStructuralResult{Compaction: contextCompactionResultFromSession(sessionRecord)}, sessionRecord,
	)
	committedSession, err := sess.AppendContextCompactionAt(sessionCursor, sessionRecord)
	if err != nil {
		t.Fatal(err)
	}
	sessionResult, err := (&App{cfg: &config.Config{NovaDir: root}}).reconcileHarnessDomainCommit(context.Background(), sessionRequest)
	if err != nil || !sessionResult.Found || sessionResult.Revision != fmt.Sprintf("session-context:%d", committedSession.ContextRevision) {
		t.Fatalf("Session structural reconciliation = %#v err=%v", sessionResult, err)
	}
	sessionRemoveCommand := agentruntime.CommandID("session-remove-command")
	sessionRemoval := session.ContextCompactionRemoval{
		ID: contextStructuralRecordID("ccr", string(sessionRemoveCommand)), AgentKind: "ide",
		CompactionID: committedSession.ID, SourceStartIndex: committedSession.SourceStartIndex,
		SourceEndIndex: committedSession.SourceEndIndex, Reason: "user_removed",
	}
	sessionRemovalCursor := sess.ContextCursor()
	sessionRemoveRequest := structuralRecoveryRequest(
		t, sessionBinding, sessionRemoveCommand, "session-remove-operation", agent.ContextStructuralDomainSession,
		agent.ContextStructuralRemove, agentruntime.StructuralRemoveCompaction,
		agentruntime.ContextCompactionRef{
			Resource: sess.ID, CompactionID: committedSession.ID,
			ExpectedRevision: fmt.Sprintf("session-context:%d", sessionRemovalCursor.Revision),
		},
		agent.ContextStructuralResult{Removed: true}, sessionRemoval,
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
	storyCommand := agentruntime.CommandID("story-compact-command")
	storyRecord := interactive.ContextCompactionEvent{
		ID: contextStructuralRecordID("cc", string(storyCommand)), AgentKind: "interactive_story",
		Epoch: 1, Summary: "story summary", SourceTurnCount: 1, RetainedTurns: 1,
		TokensBefore: 100, TokensAfter: 30, ContextWindowTokens: 1000, Threshold: .8,
		Reason: "manual", Phase: "manual", ExpectedParentID: &expectedParent,
	}
	storyBinding := agentruntime.BindingRef{Kind: agentruntime.BindingGame, Profile: agentruntime.ProfileGame, Workspace: workspace, StoryID: story.ID, BranchID: "main"}
	storyRequest := structuralRecoveryRequest(
		t, storyBinding, storyCommand, "story-compact-operation", agent.ContextStructuralDomainStory,
		agent.ContextStructuralCompact, agentruntime.StructuralCompactContext,
		agentruntime.ContextCompactionRef{Resource: story.ID + "/main", ExpectedRevision: contextStoryRevision(expectedParent)},
		agent.ContextStructuralResult{Compaction: contextCompactionResultFromInteractive(storyRecord)}, storyRecord,
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
	storyRemoveCommand := agentruntime.CommandID("story-remove-command")
	storyExpectedParent := committedStory.ID
	storyRemoval := interactive.ContextCompactionRemovalEvent{
		ID: contextStructuralRecordID("ccr", string(storyRemoveCommand)), AgentKind: "interactive_story",
		CompactionID: committedStory.ID, SourceTurnCount: committedStory.SourceTurnCount,
		Reason: "user_removed", ExpectedParentID: &storyExpectedParent,
	}
	storyRemoveRequest := structuralRecoveryRequest(
		t, storyBinding, storyRemoveCommand, "story-remove-operation", agent.ContextStructuralDomainStory,
		agent.ContextStructuralRemove, agentruntime.StructuralRemoveCompaction,
		agentruntime.ContextCompactionRef{
			Resource: story.ID + "/main", CompactionID: committedStory.ID,
			ExpectedRevision: contextStoryRevision(storyExpectedParent),
		},
		agent.ContextStructuralResult{Removed: true}, storyRemoval,
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
	binding agentruntime.BindingRef,
	commandID string,
	operationID string,
	cycle int,
	stage agentruntime.DomainCommitStage,
	hash string,
) agentruntime.DomainCommitReconcileRequest {
	return agentruntime.DomainCommitReconcileRequest{
		Binding: binding,
		Commit: agentruntime.DomainCommitState{
			Identity: agentruntime.DomainCommitIdentity{
				CommandID: agentruntime.CommandID(commandID), OperationID: agentruntime.OperationID(operationID),
				Cycle: cycle, Stage: stage,
			},
			Hash: hash,
		},
	}
}

func structuralRecoveryRequest(
	t *testing.T,
	binding agentruntime.BindingRef,
	commandID agentruntime.CommandID,
	operationID agentruntime.OperationID,
	domain agent.ContextStructuralDomain,
	action agent.ContextStructuralAction,
	kind agentruntime.StructuralOperationKind,
	ref agentruntime.ContextCompactionRef,
	result agent.ContextStructuralResult,
	mutation any,
) agentruntime.DomainCommitReconcileRequest {
	t.Helper()
	plan, err := newContextStructuralRestorePlan(domain, action, binding, ref, contextStructuralRecordID(structuralRecordPrefix(action), string(commandID)), result, mutation)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.EncodeContextStructuralRestorePlan(plan, binding, ref.ExpectedRevision)
	if err != nil {
		t.Fatal(err)
	}
	ref.RestoreDescriptor = descriptor
	request := domainRecoveryRequest(binding, string(commandID), string(operationID), 1, agentruntime.DomainCommitOutput, plan.IntentHash)
	request.Structural = &agentruntime.StructuralOperationSnapshot{
		Binding: binding, CommandID: commandID, OperationID: operationID, Cycle: 1, Kind: kind, Ref: ref,
	}
	return request
}

func structuralRecordPrefix(action agent.ContextStructuralAction) string {
	if action == agent.ContextStructuralRemove {
		return "ccr"
	}
	return "cc"
}

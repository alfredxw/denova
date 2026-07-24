package app

import (
	"context"
	"fmt"
	"testing"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestAppRestoresFrozenSessionCompactionForInactiveBinding(t *testing.T) {
	root := t.TempDir()
	workspace := root + "/inactive-book"
	state := book.NewState(workspace)
	if err := state.InitWorkspace(); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("inactive structural recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agents.UserMessage("canonical source that must not enter the descriptor")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	binding := writingContextStructuralBinding(workspace, sess.ID)
	commandID := agents.CommandID("inactive-session-compact")
	recordID := contextStructuralRecordID("cc", string(commandID))
	result := agents.ContextCompactionResult{
		Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "bounded checkpoint",
		SourceMessageCount: 1, RetainedTurns: 2, TokensBefore: 120, TokensAfter: 24,
	}
	record := sessionCompactionRecord(recordID, agents.AgentKindIDE, 0, 1, result)
	ref := agents.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "recover exact checkpoint", Resource: sess.ID,
		ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision),
	}
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainSession, agents.ContextStructuralCompact, binding, ref, recordID,
		agents.ContextStructuralResult{Compaction: result}, record,
	)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{}
	spec, err := application.restoreContextStructuralOperation(context.Background(), agents.HarnessStructuralRestoreRequest{
		Binding: binding,
		Snapshot: agents.StructuralOperation{
			Binding: binding, CommandID: commandID, OperationID: "inactive-session-operation",
			Cycle: 1, Kind: agents.StructuralCompactContext, Ref: ref,
		},
		Options: agents.RunOptions{AgentKind: agents.AgentKindIDE, Workspace: workspace, SessionID: sess.ID, Mode: "ide"},
		Plan:    plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agents.ContextStructuralIdentity{CommandID: commandID, OperationID: "inactive-session-operation", Cycle: 1}
	intent, err := spec.Operation.Prepare(context.Background(), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := spec.Operation.Commit(context.Background(), identity, intent)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Revision == "" {
		t.Fatal("restored Session commit returned no canonical revision")
	}
	got, found, err := session.FindStoredContextCompaction(state.SessionDir(), sess.ID, recordID)
	if err != nil || !found || !sameSessionContextCompactionMutation(got, record) {
		t.Fatalf("restored Session checkpoint = %#v found=%t err=%v", got, found, err)
	}
	_, reconciled, found, err := spec.Operation.Reconcile(context.Background())
	if err != nil || !found || reconciled.Revision != receipt.Revision {
		t.Fatalf("restored Session reconcile = %#v found=%t err=%v", reconciled, found, err)
	}
}

func TestAppRestoresFrozenStoryCompactionForInactiveBinding(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "inactive story", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedParent := storyContext.Meta.Branches["main"].Head
	binding := storyContextStructuralBinding(workspace, story.ID, "main")
	commandID := agents.CommandID("inactive-story-compact")
	recordID := contextStructuralRecordID("cc", string(commandID))
	result := agents.ContextCompactionResult{
		Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "bounded story checkpoint",
		RetainedTurns: 2, TokensBefore: 160, TokensAfter: 30,
	}
	event := interactiveCompactionEvent(recordID, expectedParent, 0, result)
	ref := agents.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "recover exact checkpoint",
		Resource: story.ID + "/main", ExpectedRevision: contextStoryRevision(expectedParent),
	}
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainStory, agents.ContextStructuralCompact, binding, ref, recordID,
		agents.ContextStructuralResult{Compaction: result}, event,
	)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := (&App{}).restoreContextStructuralOperation(context.Background(), agents.HarnessStructuralRestoreRequest{
		Binding: binding,
		Snapshot: agents.StructuralOperation{
			Binding: binding, CommandID: commandID, OperationID: "inactive-story-operation",
			Cycle: 1, Kind: agents.StructuralCompactContext, Ref: ref,
		},
		Options: agents.RunOptions{
			AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: story.ID, BranchID: "main", Mode: "interactive",
		},
		Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agents.ContextStructuralIdentity{CommandID: commandID, OperationID: "inactive-story-operation", Cycle: 1}
	intent, err := spec.Operation.Prepare(context.Background(), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Operation.Commit(context.Background(), identity, intent); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.ContextCompactionByID(story.ID, recordID)
	if err != nil || !found || !sameStoryContextCompactionMutation(got, event) {
		t.Fatalf("restored Story checkpoint = %#v found=%t err=%v", got, found, err)
	}
}

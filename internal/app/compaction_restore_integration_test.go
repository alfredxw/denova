package app

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	compactionapp "denova/internal/app/compaction"
	"denova/internal/book"
	"denova/internal/interactive"
	projectdomain "denova/internal/project"
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
	binding := compactionapp.WritingBinding(workspace, sess.ID)
	commandID := agentrun.CommandID("inactive-session-compact")
	recordID := agentstructural.RecordID("cc", string(commandID))
	result := agentcompaction.Result{
		Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "bounded checkpoint",
		SourceMessageCount: 1, RetainedTurns: 2, TokensBefore: 120, TokensAfter: 24,
	}
	record := compactionapp.SessionRecord(recordID, agentrun.AgentKindIDE, 0, 1, result)
	ref := agentrun.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "recover exact checkpoint", Resource: sess.ID,
		ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision),
	}
	plan, err := agentstructural.NewRestorePlan(
		agentstructural.DomainSession, agentstructural.Compact, binding, ref, recordID,
		agentstructural.Result{Compaction: result}, record,
	)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{}
	spec, err := application.restoreContextStructuralOperation(context.Background(), agentexecution.StructuralRestoreRequest{
		Binding: binding,
		Snapshot: agentrun.StructuralOperation{
			Binding: binding, CommandID: commandID, OperationID: "inactive-session-operation",
			Cycle: 1, Kind: agentrun.StructuralCompactContext, Ref: ref,
		},
		Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sess.ID, Mode: "ide"},
		Plan:    plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agentstructural.Identity{CommandID: commandID, OperationID: "inactive-session-operation", Cycle: 1}
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
	if err != nil || !found || !compactionapp.SameSessionMutation(got, record) {
		t.Fatalf("restored Session checkpoint = %#v found=%t err=%v", got, found, err)
	}
	_, reconciled, found, err := spec.Operation.Reconcile(context.Background())
	if err != nil || !found || reconciled.Revision != receipt.Revision {
		t.Fatalf("restored Session reconcile = %#v found=%t err=%v", reconciled, found, err)
	}
}

func TestAppRestoresFrozenGeneralProjectCompactionInUserState(t *testing.T) {
	dataDir := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "general-project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(dataDir)
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "General")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("General structural recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agents.UserMessage("canonical source")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	binding := agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindGeneral, ProjectID: record.ID,
		Mode: "agent_chat", SessionID: sess.ID,
	}
	commandID := agentrun.CommandID("general-project-compact")
	recordID := agentstructural.RecordID("cc", string(commandID))
	result := agentcompaction.Result{
		Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "bounded General checkpoint",
		SourceMessageCount: 1, RetainedTurns: 2, TokensBefore: 120, TokensAfter: 24,
	}
	compaction := compactionapp.SessionRecord(recordID, agentrun.AgentKindGeneral, 0, 1, result)
	ref := agentrun.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "recover exact General checkpoint", Resource: sess.ID,
		ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision),
	}
	plan, err := agentstructural.NewRestorePlan(
		agentstructural.DomainSession, agentstructural.Compact, binding, ref, recordID,
		agentstructural.Result{Compaction: result}, compaction,
	)
	if err != nil {
		t.Fatal(err)
	}
	application := &App{projectRegistry: registry}
	spec, err := application.restoreContextStructuralOperation(context.Background(), agentexecution.StructuralRestoreRequest{
		Binding: binding,
		Snapshot: agentrun.StructuralOperation{
			Binding: binding, CommandID: commandID, OperationID: "general-project-operation",
			Cycle: 1, Kind: agentrun.StructuralCompactContext, Ref: ref,
		},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindGeneral, ProjectID: record.ID,
			SessionID: sess.ID, Mode: "agent_chat",
		},
		Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agentstructural.Identity{
		CommandID: commandID, OperationID: "general-project-operation", Cycle: 1,
	}
	intent, err := spec.Operation.Prepare(context.Background(), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := spec.Operation.Commit(context.Background(), identity, intent)
	if err != nil || receipt.Revision == "" {
		t.Fatalf("General compaction receipt = %#v err=%v", receipt, err)
	}
	got, found, err := session.FindStoredContextCompaction(layout.SessionsDir(), sess.ID, recordID)
	if err != nil || !found || !compactionapp.SameSessionMutation(got, compaction) {
		t.Fatalf("restored General checkpoint = %#v found=%t err=%v", got, found, err)
	}
	if _, err := os.Stat(book.NewState(workspace).SessionDir()); !os.IsNotExist(err) {
		t.Fatalf("General compaction wrote workspace-private state, err=%v", err)
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
	binding := compactionapp.StoryBinding(workspace, story.ID, "main")
	commandID := agentrun.CommandID("inactive-story-compact")
	recordID := agentstructural.RecordID("cc", string(commandID))
	result := agentcompaction.Result{
		Triggered: true, Phase: "mid_run", Epoch: 1, Summary: "bounded story checkpoint",
		RetainedTurns: 2, TokensBefore: 160, TokensAfter: 30,
	}
	event := compactionapp.StoryEvent(recordID, expectedParent, 0, result)
	ref := agentrun.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "recover exact checkpoint",
		Resource: story.ID + "/main", ExpectedRevision: agentstructural.StoryRevision(expectedParent),
	}
	plan, err := agentstructural.NewRestorePlan(
		agentstructural.DomainStory, agentstructural.Compact, binding, ref, recordID,
		agentstructural.Result{Compaction: result}, event,
	)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := (&App{}).restoreContextStructuralOperation(context.Background(), agentexecution.StructuralRestoreRequest{
		Binding: binding,
		Snapshot: agentrun.StructuralOperation{
			Binding: binding, CommandID: commandID, OperationID: "inactive-story-operation",
			Cycle: 1, Kind: agentrun.StructuralCompactContext, Ref: ref,
		},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: story.ID, BranchID: "main", Mode: "interactive",
		},
		Plan: plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity := agentstructural.Identity{CommandID: commandID, OperationID: "inactive-story-operation", Cycle: 1}
	intent, err := spec.Operation.Prepare(context.Background(), identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Operation.Commit(context.Background(), identity, intent); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.ContextCompactionByID(story.ID, recordID)
	if err != nil || !found || !interactive.SameContextCompactionMutation(got, event) {
		t.Fatalf("restored Story checkpoint = %#v found=%t err=%v", got, found, err)
	}
}

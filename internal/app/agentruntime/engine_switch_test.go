package agentruntime

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/execution"
	"denova/internal/agents/external"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func TestEngineSwitchChecksWritingOwnerAndRejectsPreviouslyPreparedNativeExecution(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	selection := conversationconfig.Config{AgentKind: config.AgentKindIDE, ProfileID: "default", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalWrite}
	sess, err := store.GetOrCreateWithRuntimeConfig("writing", selection)
	if err != nil {
		t.Fatal(err)
	}
	native := execution.NewEphemeralRuntime()
	defer native.Close(context.Background())
	engines := NewEngines()
	defer engines.Close()
	connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
	engines.entries[config.RuntimeCodex].factory = func(context.Context) (external.Connection, error) { return connection, nil }
	options := agentrun.Options{AgentKind: config.AgentKindIDE, ProjectID: "book", Workspace: t.TempDir(), SessionID: sess.ID, Mode: "ide"}
	goal, err := native.UpdateGoal(t.Context(), options, agent.GoalMutation{Kind: agent.GoalSet, Objective: "Finish the current chapter."})
	if err != nil {
		t.Fatal(err)
	}
	selection.Runtime = &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "runtime-model"}}
	fromAgents := options
	fromAgents.Mode = "agent_chat"
	if _, err := engines.ApplyEngineSelection(t.Context(), native, sess, fromAgents, selection, 1, config.Config{}); !errors.Is(err, ErrOperationActive) {
		t.Fatalf("AgentChat bypassed the Writing goal: %v", err)
	}
	if connection.probes != 0 {
		t.Fatal("busy selection started an engine")
	}
	if _, err := native.UpdateGoal(t.Context(), options, agent.GoalMutation{Kind: agent.GoalClear, ExpectedRevision: goal.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := engines.ApplyEngineSelection(t.Context(), native, sess, fromAgents, selection, 1, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if release, err := engines.AdmitExecution(t.Context(), sess, nil); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		if release != nil {
			release()
		}
		t.Fatalf("stale Native preparation executed after switch: %v", err)
	}
	release, err := engines.AdmitExecution(t.Context(), sess, selection.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

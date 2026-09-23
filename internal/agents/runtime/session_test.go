package agentruntime

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
	publicgoal "github.com/alfredxw/denova/agent/goal"
)

func TestSessionGoalContractAcrossRuntimeSelections(t *testing.T) {
	for _, kind := range []config.RuntimeID{config.RuntimeNative, config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(kind), func(t *testing.T) {
			store, err := session.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			selection := config.RuntimeSelection{Kind: kind}
			if kind == config.RuntimeCodex {
				selection.Codex = &config.CodexRuntimeSettings{Model: "fixture"}
			}
			if kind == config.RuntimeClaude {
				selection.Claude = &config.ClaudeRuntimeSettings{Model: "fixture"}
			}
			sess, err := store.GetOrCreateWithRuntimeConfig("session", conversationconfig.Config{AgentKind: config.AgentKindIDE, ProfileID: "fixture", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalWrite, Runtime: &selection})
			if err != nil {
				t.Fatal(err)
			}
			native := execution.NewEphemeralRuntime()
			defer native.Close(context.Background())
			engines := NewEngines()
			defer engines.Close()
			options := agentrun.Options{ProjectID: "project", Workspace: t.TempDir(), AgentKind: config.AgentKindIDE, SessionID: sess.ID, Mode: "ide"}
			bound, err := engines.ConversationSession(native, options, sess, selection)
			if err != nil {
				t.Fatal(err)
			}
			goal, err := bound.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Finish and verify"})
			if err != nil {
				t.Fatal(err)
			}
			paused, err := bound.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalPause, ExpectedRevision: goal.Revision})
			if err != nil || paused.Status != agent.GoalPaused {
				t.Fatalf("pause=%+v err=%v", paused, err)
			}
			if _, err := bound.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalResume, ExpectedRevision: goal.Revision}); !errors.Is(err, publicgoal.ErrRevisionConflict) {
				t.Fatalf("stale update accepted: %v", err)
			}
			got, present, err := bound.Goal(t.Context())
			if err != nil || !present || got.ID != goal.ID || got.Revision != paused.Revision {
				t.Fatalf("goal=%+v err=%v", got, err)
			}
			if _, err := bound.Submit(t.Context(), Command{Kind: execution.CommandKind("unknown")}, nil); !errors.Is(err, agentrun.ErrInvalidCommand) {
				t.Fatalf("invalid command accepted: %v", err)
			}
			status, err := bound.Status(t.Context())
			if err != nil || status.Phase != agentrun.RunPhaseIdle {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
}

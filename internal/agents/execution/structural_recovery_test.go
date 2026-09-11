package execution

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentstructural "denova/internal/agents/context/structural"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
)

type structuralTestCompaction struct{ t *testing.T }

func (structuralTestCompaction) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.structural-compaction", Version: 1}
}

func (structuralTestCompaction) SummaryLimitBytes() int { return 64 << 10 }

func (structuralTestCompaction) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	if !request.Force || len(request.Groups) == 0 {
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	return agent.CompactionPlan{
		Action: agent.CompactionCreate, GroupCount: len(request.Groups),
		Validation: agent.CompactionValidationPolicy{HardLimitBytes: 4 << 20},
	}, nil
}

func (manager structuralTestCompaction) Compact(_ context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	manager.t.Helper()
	found := false
	for _, tool := range request.ModelSnapshot.ResolvedOptions().Tools {
		found = found || tool.Name == "task"
	}
	if !found {
		manager.t.Fatal("structural preparation omitted delegated tool schemas")
	}
	return agent.CompactionCheckpoint{Summary: "Preserved writing history."}, nil
}

func TestWritingStructuralOperationsRebuildCanonicalSession(t *testing.T) {
	for _, scenario := range []string{"inspection", "restart", "existing_history"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			workspace, dataDir := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Writing maintenance")
			if err != nil {
				t.Fatal(err)
			}
			layout, err := registry.EnsureStore(record)
			if err != nil {
				t.Fatal(err)
			}
			productStore, err := session.NewStore(layout.SessionsDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = productStore.Close() })
			sess, err := productStore.GetOrCreate("writing-maintenance")
			if err != nil {
				t.Fatal(err)
			}
			rich := strings.Repeat("Earlier chapter details. ", 200)
			for _, message := range []*agent.Message{
				agent.UserMessage(rich), agent.AssistantMessage("Earlier chapter.", nil),
				agent.UserMessage("Recent request."), agent.AssistantMessage("Recent chapter.", nil),
			} {
				if err := sess.Append(message); err != nil {
					t.Fatal(err)
				}
			}
			journalStore, err := canonicalstore.New(dataDir, registry)
			if err != nil {
				t.Fatal(err)
			}
			newRuntime := func() *Runtime {
				runtime, err := NewAgentRuntime(ctx, dataDir, WithSessionStore(journalStore),
					WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
				if err != nil {
					t.Fatal(err)
				}
				return runtime
			}
			model := &publicBackendTestModel{}
			catalog, err := agentdelegation.NewCatalog(nil, agentdelegation.Config{
				Capability: "test.maintenance-delegation", MaxResultBytes: 64 << 10, Parallelism: 1,
				ValidationIdentity: agent.CapabilityIdentity{Kind: "test.maintenance-tools", Version: 1},
				Validate:           func(context.Context, []agent.ToolDefinition) error { return nil },
			}, agentdelegation.Child{
				Name: "writer", Definition: agent.Definition{Model: model},
				Identity: agent.CapabilityIdentity{Kind: "test.maintenance-writer", Version: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			newCycle := func() Cycle {
				return Cycle{
					Definition: agent.Definition{Key: "writing-maintenance", Name: "writer", Model: model,
						ModelIdentity: agent.CapabilityIdentity{Kind: "test.writing-maintenance-model", Version: 1},
						Compaction:    structuralTestCompaction{t: t}, Tools: catalog},
					Conversation: agentconversation.NewSessionConversationForAgent(sess, &config.Config{Workspace: workspace}, agentrun.AgentKindIDE),
					Options: agentrun.Options{ProjectID: record.ID, AgentKind: agentrun.AgentKindIDE, Workspace: workspace,
						StateRoot: layout.StoreRoot, SessionID: sess.ID, RootAgentName: "writer"},
				}
			}
			runtime := newRuntime()
			t.Cleanup(func() { _ = runtime.Close(ctx) })
			if scenario != "existing_history" {
				cycle := newCycle()
				cycle.Request = agentchatRequest("writing-before-maintenance", "Continue the draft")
				operation, err := runtime.Start(ctx, StartRequest{Cycle: cycle})
				if err != nil {
					t.Fatal(err)
				}
				if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
					t.Fatalf("initial writing turn failed: %+v", outcome)
				}
			}
			if scenario == "inspection" {
				cycle := newCycle()
				cycle.Request = agentchatRequest("writing-inspection", "Inspect context")
				if _, err := runtime.Inspect(ctx, cycle); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "restart" {
				if err := runtime.Close(ctx); err != nil {
					t.Fatal(err)
				}
				runtime = newRuntime()
			}
			before := sess.History()
			result, err := runtime.ExecuteStructuralOperation(ctx, newCycle(), agentstructural.Spec{
				Action: agentstructural.Compact, CommandID: "writing-compact", Ref: agentrun.ContextCompactionRef{Force: true},
			})
			if err != nil || !result.Compaction.Triggered {
				t.Fatalf("writing compaction failed: %v", err)
			}
			if !reflect.DeepEqual(before, sess.History()) {
				t.Fatal("manual compaction changed canonical writing messages")
			}
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			runtime = newRuntime()
			result, err = runtime.ExecuteStructuralOperation(ctx, newCycle(), agentstructural.Spec{
				Action: agentstructural.Remove, CommandID: "writing-remove-after-restart",
			})
			if err != nil || !result.Removed {
				t.Fatalf("writing checkpoint removal after restart failed: removed=%t error=%v", result.Removed, err)
			}
			cycle := newCycle()
			cycle.Request = agentchatRequest("writing-after-maintenance", "Continue after maintenance")
			operation, err := runtime.Start(ctx, StartRequest{Cycle: cycle})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("writing could not continue after maintenance: %+v", outcome)
			}
			model.mu.Lock()
			defer model.mu.Unlock()
			restored := false
			for _, message := range model.inputs[len(model.inputs)-1] {
				restored = restored || strings.Contains(message.Content, rich)
			}
			if !restored {
				t.Fatal("writing checkpoint removal did not restore complete history")
			}
		})
	}
}

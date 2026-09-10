package interactiveapp

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
)

func TestGameManualCompactionAfterInspectionAndRestart(t *testing.T) {
	for _, scenario := range []string{"inspection", "restart"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			workspace, dataDir := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Game compaction recovery")
			if err != nil {
				t.Fatal(err)
			}
			journalStore, err := canonicalstore.New(dataDir, registry)
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(workspace)
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Game compaction recovery", StoryTellerID: "classic"})
			if err != nil {
				t.Fatal(err)
			}
			rich := strings.Repeat("Important historical evidence. ", 1000)
			appendPublicGameToolTurn(t, store, story.ID, rich)
			newRuntime := func() *agentexecution.Runtime {
				runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir,
					agentexecution.WithSessionStore(journalStore),
					agentexecution.WithProfiles(publicGameNoopProfile(workspace, story.ID)),
					agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
				)
				if err != nil {
					t.Fatal(err)
				}
				return runtime
			}
			runtime := newRuntime()
			t.Cleanup(func() { _ = runtime.Close(ctx) })
			cfg := &config.Config{Workspace: workspace, OpenAIContextWindowTokens: 128_000}
			model := &publicGameHistoryModel{narrative: "Continue the story.", checkpoint: "public Game checkpoint"}
			manager, err := agentcompaction.NewAgentManagerForModel(cfg, config.AgentKindInteractiveStory, 128_000, model,
				agent.CapabilityIdentity{Kind: "model.test.public-game-history", Version: 1})
			if err != nil {
				t.Fatal(err)
			}
			newCycle := func(message string) agentexecution.Cycle {
				cycle := publicGameMaintenanceCycle(store, workspace, story.ID, cfg, nil)
				cycle.Conversation = NewConversation(store, "", workspace, story.ID, "main", message, 800, cfg)
				cycle.Options.ProjectID = record.ID
				cycle.Definition.Model = model
				cycle.Definition.Compaction = manager
				return cycle
			}
			initial := newCycle("Continue")
			initial.Request = agentchat.ChatRequest{CommandID: "game-before-maintenance", Message: "Continue"}
			submitTestTurnResult(t, initial.Conversation.(*Conversation), "Continue", "Continue")
			operation, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: initial})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("initial game turn failed: %+v", outcome)
			}
			before, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "restart" {
				if err := runtime.Close(ctx); err != nil {
					t.Fatal(err)
				}
				runtime = newRuntime()
			} else {
				inspection := newCycle("Inspect context")
				inspection.Request = agentchat.ChatRequest{CommandID: "game-inspection", Message: "Inspect context"}
				_, err := runtime.Inspect(ctx, inspection)
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := runtime.ExecuteStructuralOperation(ctx, newCycle(""), agentstructural.Spec{
				Action: agentstructural.Compact, CommandID: "game-manual-compact",
				Ref: agentrun.ContextCompactionRef{Force: true},
			})
			if err != nil || !result.Compaction.Triggered {
				t.Fatalf("manual compaction after %s: result=%+v error=%v", scenario, result, err)
			}
			after, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Turns, after.Turns) {
				t.Fatal("manual compaction changed canonical game turns")
			}
			for range 3 {
				appendPublicGameToolTurn(t, store, story.ID, rich)
			}
			before, err = store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := runtime.ExecuteStructuralOperation(ctx, newCycle(""), agentstructural.Spec{
				Action: agentstructural.Compact, CommandID: "game-manual-compact-again",
				Ref: agentrun.ContextCompactionRef{Force: true},
			})
			if err != nil || !repeated.Compaction.Triggered {
				t.Fatalf("repeated manual compaction: result=%+v error=%v", repeated, err)
			}
			after, err = store.Snapshot(story.ID, "main")
			if err != nil || !reflect.DeepEqual(before.Turns, after.Turns) {
				t.Fatalf("repeated compaction changed canonical game turns: %v", err)
			}
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			runtime = newRuntime()
			status, err := runtime.RuntimeStatusProjection(ctx, newCycle("").Options)
			if err != nil || status.Compaction == nil || status.Compaction.Revision != uint64(repeated.Compaction.Epoch) {
				t.Fatalf("checkpoint was not restored from the Story journal: %+v, %v", status.Compaction, err)
			}
			compactedContinuation := newCycle("Continue with checkpoint")
			tool, err := agent.InferTool("read_checkpoint_evidence", "Read historical evidence", func(context.Context, struct{}) (string, error) {
				return "The gate is open.", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.test.checkpoint-evidence", Version: 1}, agent.ToolDefinition{
				Tool: tool, Descriptor: agent.ToolDescriptor{
					Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
					MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
					Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
					ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 4 << 10,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			compactedContinuation.Definition.Model = &checkpointToolModel{history: model}
			compactedContinuation.Definition.Tools = toolset
			compactedContinuation.Definition.Permission = agentpermission.FullAccess()
			compactedContinuation.Request = agentchat.ChatRequest{CommandID: "game-with-checkpoint", Message: "Continue with checkpoint"}
			submitTestTurnResult(t, compactedContinuation.Conversation.(*Conversation), "Continue with checkpoint", "Continue with checkpoint")
			operation, err = runtime.Start(ctx, agentexecution.StartRequest{Cycle: compactedContinuation})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("game could not continue with a restored checkpoint: %+v", outcome)
			}
			if !containsMessageContent(model.lastInput(t), "public Game checkpoint") {
				t.Fatal("continued game did not use the restored checkpoint")
			}
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			runtime = newRuntime()
			removed, err := runtime.ExecuteStructuralOperation(ctx, newCycle(""), agentstructural.Spec{
				Action: agentstructural.Remove, CommandID: "game-remove-after-restart",
				Ref: agentrun.ContextCompactionRef{CompactionID: status.Compaction.ID},
			})
			if err != nil || !removed.Removed {
				t.Fatalf("remove checkpoint after restart: %+v, %v", removed, err)
			}
			continuation := newCycle("Continue after maintenance")
			continuation.Request = agentchat.ChatRequest{CommandID: "game-after-maintenance", Message: "Continue after maintenance"}
			submitTestTurnResult(t, continuation.Conversation.(*Conversation), "Continue after maintenance", "Continue after maintenance")
			operation, err = runtime.Start(ctx, agentexecution.StartRequest{Cycle: continuation})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("game could not continue after maintenance: %+v", outcome)
			}
			if !containsMessageContent(model.lastInput(t), rich) || containsMessageContent(model.lastInput(t), "public Game checkpoint") {
				t.Fatal("checkpoint removal did not restore raw game history")
			}
		})
	}
}

type checkpointToolModel struct {
	history *publicGameHistoryModel
	called  bool
}

func (model *checkpointToolModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	response := model.history.response(messages)
	response.ResponseMeta = &agent.ResponseMeta{FinishReason: "stop", Usage: &agent.TokenUsage{TotalTokens: 100}}
	response.ReasoningContent = "Inspect the gate before continuing."
	if !model.called {
		model.called = true
		response.Content = "I will inspect the gate."
		response.ToolCalls = []agent.ToolCall{{ID: "checkpoint-evidence", Type: "function", Function: agent.FunctionCall{Name: "read_checkpoint_evidence", Arguments: `{}`}}}
		response.ResponseMeta.FinishReason = "tool_calls"
	}
	return response, nil
}

func (model *checkpointToolModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

func publicGameMaintenanceCycle(store *interactive.Store, workspace, storyID string, cfg *config.Config, cleanup agent.CleanupManager) agentexecution.Cycle {
	options := publicGameOptions(workspace, storyID, "main")
	options.TaskID = ""
	return agentexecution.Cycle{
		Definition: agent.Definition{
			Key: "denova.test.public-game-history", Name: "game", Model: &publicGameHistoryModel{narrative: "Unexpected model call."},
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.public-game-history", Version: 1},
			Cleanup:       cleanup, Compaction: publicGameCompactionManager{},
		},
		Conversation: NewConversation(store, "", workspace, storyID, "main", "", 800, cfg),
		Options:      options,
	}
}

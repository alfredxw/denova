package interactiveapp

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/modelio"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
)

const automaticGameCheckpoint = "The traveler followed the river through ninety rainy nights and promised to return to the village."

// Only model output is simulated: admission, automatic pressure planning,
// checkpoint generation/validation, tool commits, and journal recovery are real.
type automaticGameCheckpointModel struct {
	history      publicGameHistoryModel
	summaryCalls int
	toolPending  bool
}

func (model *automaticGameCheckpointModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	if err := modelio.ValidateInput(config.AgentKindInteractiveStory, messages, nil, 4<<20, 128_000); err != nil {
		return nil, err
	}
	if len(messages) > 0 && strings.HasPrefix(messages[len(messages)-1].Content, "[Denova runtime context compaction request]") {
		model.summaryCalls++
		return agent.AssistantMessage(automaticGameCheckpoint, nil), nil
	}
	response := model.history.response(messages)
	promptTokens := agent.EstimateMessagesTokens(messages)
	response.ResponseMeta = &agent.ResponseMeta{
		FinishReason: "stop", Usage: &agent.TokenUsage{PromptTokens: promptTokens, CompletionTokens: 100, TotalTokens: promptTokens + 100},
	}
	response.ReasoningContent = "Check the river before advancing the story."
	if !model.toolPending {
		model.toolPending = true
		response.Content = "I will inspect the river."
		response.ToolCalls = []agent.ToolCall{{ID: "river-evidence", Type: "function", Function: agent.FunctionCall{Name: "read_river", Arguments: `{}`}}}
		response.ResponseMeta.FinishReason = "tool_calls"
	} else {
		model.toolPending = false
	}
	return response, nil
}

func (model *automaticGameCheckpointModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

func TestGameAutomaticCompactionSurvivesConsecutiveTurnsAndRestart(t *testing.T) {
	ctx := context.Background()
	workspace, dataDir := t.TempDir(), t.TempDir()
	registry := project.NewRegistry(dataDir)
	record, err := registry.Add(workspace, project.TypeGeneral, "Long game compaction")
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := canonicalstore.New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Long game compaction", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	for turn := range 90 {
		if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			BranchID: "main", User: fmt.Sprintf("Follow the river on night %d", turn+1),
			Narrative: fmt.Sprintf("Historical night %d: %s", turn+1, strings.Repeat("雨", 1024)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
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
	model := &automaticGameCheckpointModel{history: publicGameHistoryModel{narrative: "The traveler reached the next bridge."}}
	identity := agent.CapabilityIdentity{Kind: "test.automatic-game-checkpoint", Version: 1}
	manager, err := agentcompaction.NewAgentManagerForModel(cfg, config.AgentKindInteractiveStory, 128_000, model, identity)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := agent.InferTool("read_river", "Read current river conditions", func(context.Context, struct{}) (string, error) {
		return "The river is calm and the bridge is open.", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.test.river-evidence", Version: 1}, agent.ToolDefinition{
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
	options := publicGameOptions(workspace, story.ID, "main")
	options.ProjectID = record.ID
	var checkpointID string
	for turn := 91; turn <= 95; turn++ {
		if turn == 93 {
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			runtime = newRuntime()
		}
		input := fmt.Sprintf("Continue to bridge %d", turn)
		conversation := NewConversation(store, "", workspace, story.ID, "main", input, 800, cfg)
		submission := gameSubmissionForTest(t, conversation, input, input)
		operation, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: agentexecution.Cycle{
			Definition: agent.Definition{
				Key: "automatic-game-checkpoint", Name: "game", Model: model, ModelIdentity: identity,
				Middlewares: []agent.Middleware{submission},
				Compaction:  manager, Tools: toolset, Permission: agentpermission.FullAccess(),
			},
			Conversation: conversation, Options: options,
			Request: agentchat.ChatRequest{CommandID: fmt.Sprintf("automatic-game-turn-%d", turn), Message: input},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("turn %d failed: %+v", turn, outcome)
		}
		if model.summaryCalls != 1 {
			t.Fatalf("turn %d generated %d checkpoints; expected the initial automatic compaction only", turn, model.summaryCalls)
		}
		messages := model.history.lastInput(t)
		if !containsMessageContent(messages, automaticGameCheckpoint) || containsMessageContent(messages, before.Turns[0].Narrative) {
			t.Fatalf("turn %d lost the checkpoint or replayed compacted history", turn)
		}
		status, err := runtime.RuntimeStatusProjection(ctx, options)
		if err != nil || status.Compaction == nil {
			t.Fatalf("turn %d lost durable compaction: %+v, %v", turn, status.Compaction, err)
		}
		if checkpointID == "" {
			checkpointID = status.Compaction.ID
			recoveryTarget := int(float64(cfg.OpenAIContextWindowTokens) * config.DefaultContextCompactionThreshold * config.DefaultContextCompactionRecoveryBand)
			if status.Compaction.TokenEstimate <= 0 || status.Compaction.TokenEstimate > recoveryTarget {
				t.Fatalf("automatic compaction did not restore context headroom: projected=%d target=%d", status.Compaction.TokenEstimate, recoveryTarget)
			}
			t.Logf("Compacted 90-turn history: projected_tokens_after=%d recovery_target=%d", status.Compaction.TokenEstimate, recoveryTarget)
		} else if status.Compaction.ID != checkpointID {
			t.Fatalf("turn %d replaced checkpoint %s with %s", turn, checkpointID, status.Compaction.ID)
		}
	}
	after, err := store.Snapshot(story.ID, "main")
	if err != nil || len(after.Turns) != 95 {
		t.Fatalf("game continuation lost turns: count=%d error=%v", len(after.Turns), err)
	}
	if !reflect.DeepEqual(before.Turns, after.Turns[:90]) {
		t.Fatal("automatic compaction changed canonical story history")
	}
}

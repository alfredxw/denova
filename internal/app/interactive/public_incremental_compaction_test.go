package interactiveapp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/interactive"
	"denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/permission"
)

const incrementalIntent = "Verify 24 sources. Corrected budget is 72519, not 72591; preserve evidence IDs."

// Model responses are deterministic; both products use the actual Denova
// manager, primary snapshot fork, final request validator and canonical store.
type longProductModel struct {
	t               *testing.T
	step, summaries int
	resume          bool
	inputs          [][]*agent.Message
}

func (m *longProductModel) Generate(_ context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	if strings.HasPrefix(messages[len(messages)-1].Content, "[Runtime context compaction request]") {
		m.summaries++
		if m.summaries > 1 && !containsMessageContent(messages, "Incremental evidence checkpoint") {
			m.t.Error("summary lost previous checkpoint")
		}
		return agent.AssistantMessage(fmt.Sprintf("Incremental evidence checkpoint %d. Goal: verify 24 sources. Budget corrected from 72591 to 72519. Evidence IDs: source-1 through source-%d. Completed sources remain verified. Pending: read remaining sources, then report.", m.summaries, m.step-1), nil), nil
	}
	m.inputs = append(m.inputs, messages)
	if !m.resume && !containsMessageContent(messages, incrementalIntent) {
		m.t.Error("current input disappeared")
	}
	calls := map[string]bool{}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			calls[call.ID] = true
		}
		if message.Role == agent.ToolRole && !calls[message.ToolCallID] {
			m.t.Errorf("orphan result %s", message.ToolCallID)
		}
	}
	if m.step > 0 && !m.resume && !containsMessageContent(messages, fmt.Sprintf("source-%d", m.step)) {
		m.t.Errorf("latest source %d disappeared", m.step)
	}
	if m.step == 24 || m.resume {
		return agent.AssistantMessage("All evidence verified. Corrected budget 72519.", nil), nil
	}
	m.step++
	return agent.AssistantMessage("Inspect the next source", []agent.ToolCall{{ID: fmt.Sprintf("evidence-%d", m.step), Type: "function", Function: agent.FunctionCall{Name: "evidence", Arguments: fmt.Sprintf(`{"step":%d}`, m.step)}}}), nil
}
func (m *longProductModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	result, err := m.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{result}), nil
}

func TestProductsCompactRepeatedlyWithinOneRunAndColdReopen(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			ctx := t.Context()
			workspace, dataDir := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Incremental checkpoint")
			if err != nil {
				t.Fatal(err)
			}
			layout, err := registry.EnsureStore(record)
			if err != nil {
				t.Fatal(err)
			}
			journals, err := canonicalstore.New(dataDir, registry)
			if err != nil {
				t.Fatal(err)
			}
			stories := interactive.NewStore(workspace)
			defer stories.Close()
			story, err := stories.CreateStory(interactive.CreateStoryRequest{Title: "Incremental checkpoint", StoryTellerID: "classic"})
			if err != nil {
				t.Fatal(err)
			}
			products, err := productsession.NewStore(layout.SessionsDir())
			if err != nil {
				t.Fatal(err)
			}
			defer products.Close()
			writing, err := products.GetOrCreate("incremental-writing")
			if err != nil {
				t.Fatal(err)
			}
			newRuntime := func() *agentexecution.Runtime {
				runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir, agentexecution.WithSessionStore(journals), agentexecution.WithProfiles(publicGameNoopProfile(workspace, story.ID)), agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
				if err != nil {
					t.Fatal(err)
				}
				return runtime
			}
			runtime := newRuntime()
			defer func() { _ = runtime.Close(context.Background()) }()
			cfg := &config.Config{Workspace: workspace, OpenAIContextWindowTokens: 64_000}
			model := &longProductModel{t: t}
			identity := agent.CapabilityIdentity{Kind: "test.long-product", Version: 1}
			manager, err := agentcompaction.NewAgentManagerForModel(cfg, kind, 64_000)
			if err != nil {
				t.Fatal(err)
			}
			executions := map[int]int{}
			tool, err := agent.InferTool("evidence", "Read one source", func(_ context.Context, input struct {
				Step int `json:"step"`
			}) (string, error) {
				executions[input.Step]++
				return fmt.Sprintf("source-%d: corrected budget 72519.\n%s\nsource-%d: verified.", input.Step, strings.Repeat("证", 6000), input.Step), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "test.long-product-tools", Version: 1}, agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead, MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone, Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 32 << 10}})
			if err != nil {
				t.Fatal(err)
			}
			options := agentrun.Options{ProjectID: record.ID, AgentKind: kind, Workspace: workspace, StateRoot: layout.StoreRoot, SessionID: writing.ID}
			if kind == agentrun.AgentKindInteractiveStory {
				options = publicGameOptions(workspace, story.ID, "main")
				options.ProjectID = record.ID
			}
			cycle := func(input, command string) agentexecution.Cycle {
				definition := agent.Definition{Key: "long-product", Name: "long-product", Model: model, ModelIdentity: identity, Tools: toolset, Permission: permission.FullAccess(), Compaction: manager}
				var conversation agentchat.Conversation = agentconversation.NewSessionConversationForAgent(writing, cfg, kind)
				if kind == agentrun.AgentKindInteractiveStory {
					game := NewConversation(stories, "", workspace, story.ID, "main", input, 800, cfg)
					conversation = game
					definition.Middlewares = []agent.Middleware{gameSubmissionForTest(t, game, input, input)}
				}
				return agentexecution.Cycle{Definition: definition, Conversation: conversation, Options: options, Request: agentchat.ChatRequest{CommandID: command, Message: input}}
			}
			run, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: cycle(incrementalIntent, "long-run")})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := run.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("run failed: %+v", outcome)
			}
			if model.summaries < 3 {
				t.Fatalf("one Run generated only %d checkpoints", model.summaries)
			}
			for step := 1; step <= 24; step++ {
				if executions[step] != 1 {
					t.Errorf("source %d executed %d times", step, executions[step])
				}
			}
			status, err := runtime.RuntimeStatusProjection(ctx, options)
			if err != nil || status.Compaction == nil {
				t.Fatalf("checkpoint missing: %+v %v", status.Compaction, err)
			}
			checkpointID := status.Compaction.ID
			t.Logf("one Run generated %d checkpoints at raw boundary %d", model.summaries, status.Compaction.SourceMessageCount)
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			runtime = newRuntime()
			model.resume = true
			resumed, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: cycle("Report the verified budget and evidence.", "cold-continuation")})
			if err != nil {
				t.Fatal(err)
			}
			if outcome := resumed.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("cold continuation failed: %+v", outcome)
			}
			latest := model.inputs[len(model.inputs)-1]
			if !containsMessageContent(latest, "Incremental evidence checkpoint") || !containsMessageContent(latest, "72519") {
				t.Fatal("cold continuation lost checkpoint facts")
			}
			status, err = runtime.RuntimeStatusProjection(ctx, options)
			if err != nil || status.Compaction == nil || status.Compaction.ID != checkpointID {
				t.Fatalf("cold checkpoint changed: %+v %v", status.Compaction, err)
			}
		})
	}
}

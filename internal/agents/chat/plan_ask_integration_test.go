package chat

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
)

func TestPlanModeUsesDurableAskAndResumesSameRun(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("plan-ask")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(
		sess, &config.Config{Workspace: workspace}, config.AgentKindIDE,
	)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID: "command-plan-ask", OperationID: "operation-plan-ask", Cycle: 1,
	})

	tools := []agent.ToolDefinition{
		planAskToolDefinition(t, "read", producttools.BoundedReadDescriptor(agent.ToolSourceRead, config.AgentToolWorkspaceRead)),
		planAskToolDefinition(t, "write", producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)),
		planAskToolDefinition(t, "submit_domain_state", planAskSessionDescriptor("domain_commit")),
	}
	ask, err := producttools.NewAsk()
	if err != nil {
		t.Fatal(err)
	}
	tools = append(tools, ask)
	todo, err := producttools.NewTodo()
	if err != nil {
		t.Fatal(err)
	}
	tools = append(tools, todo)

	model := &planAskIntegrationModel{}
	middleware := agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
		AgentKind: config.AgentKindIDE,
		ToolSettings: config.ResolvedAgentToolSettings{
			config.AgentToolWorkspaceRead: true, config.AgentToolWorkspaceWrite: true,
			config.AgentToolAsk: true, config.AgentToolTodo: true,
		},
		EnforceToolSettings: true,
		Workspace:           workspace,
	})
	builtAgent, err := agent.NewLoop(context.Background(), agent.LoopConfig{
		Name: "plan-ask-integration", Description: "test", Instruction: "test",
		Model: model, Tools: tools, Middlewares: []agent.Middleware{middleware},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: true})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pending := make(chan session.AskInteraction, 1)
	var events []agentrun.Event
	outcomes := make(chan agentrun.Outcome, 1)
	go func() {
		outcomes <- NewExecutor(agentrun.DefaultLoopPolicy()).Run(
			ctx, runner, conversation, book.NewService(workspace),
			ChatRequest{Message: "Plan the refactor", PlanMode: true},
			agentrun.Options{
				AgentKind: config.AgentKindIDE, RootAgentName: "plan-ask-integration",
				TaskID: "task-plan-ask", SessionID: "plan-ask", Workspace: workspace,
			},
			func(event agentrun.Event) {
				events = append(events, event)
				if event.Type == "ask_pending" {
					interaction, ok := event.Data.(session.AskInteraction)
					if ok {
						pending <- interaction
					}
				}
			},
		)
	}()

	var interaction session.AskInteraction
	select {
	case interaction = <-pending:
	case <-time.After(time.Second):
		t.Fatal("Plan Mode did not publish a durable Ask interaction")
	}
	if interaction.ID == "" || interaction.Status != session.AskPending || sess.PendingAsk(interaction.ID) == nil {
		t.Fatalf("ask_pending was emitted before durable state: %#v", interaction)
	}
	if _, err := sess.ResolveAsk(context.Background(), interaction.ID, session.AskAnswered, []session.AskAnswer{{
		QuestionID: "scope", SelectedOptionIDs: []string{"minimal"},
	}}, ""); err != nil {
		t.Fatal(err)
	}

	var outcome agentrun.Outcome
	select {
	case outcome = <-outcomes:
	case <-time.After(time.Second):
		t.Fatal("Plan Mode did not resume after Ask was answered")
	}
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil {
		t.Fatalf("outcome = %#v, want completed", outcome)
	}
	if countEventType(events, "ask_pending") != 1 || countEventType(events, "ask_resolved") != 1 || countEventType(events, "proposed_plan") != 2 {
		t.Fatalf("unexpected Plan/Ask lifecycle: %#v", events)
	}
	if countEventType(events, "plan_question") != 0 {
		t.Fatalf("retired plan-question protocol was emitted: %#v", events)
	}

	calls, toolSurfaces, modelInputs := model.snapshot()
	if calls != 2 || len(toolSurfaces) != 2 {
		t.Fatalf("model calls = %d, tool surfaces = %#v", calls, toolSurfaces)
	}
	for index, names := range toolSurfaces {
		joined := "," + strings.Join(names, ",") + ","
		for _, allowed := range []string{"read", "ask", "todo"} {
			if !strings.Contains(joined, ","+allowed+",") {
				t.Fatalf("call %d missing planning-safe tool %q: %v", index+1, allowed, names)
			}
		}
		for _, blocked := range []string{"write", "submit_domain_state"} {
			if strings.Contains(joined, ","+blocked+",") {
				t.Fatalf("call %d exposed mutating tool %q: %v", index+1, blocked, names)
			}
		}
	}
	if len(modelInputs) != 2 || !strings.Contains(modelInputs[1], `"id":"minimal"`) || !strings.Contains(modelInputs[1], `"label":"Minimal"`) {
		t.Fatalf("second model call did not receive the Ask answer: %#v", modelInputs)
	}
}

type planAskIntegrationModel struct {
	mu           sync.Mutex
	calls        int
	toolSurfaces [][]string
	modelInputs  []string
}

func (model *planAskIntegrationModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	return model.next(messages, options...), nil
}

func (model *planAskIntegrationModel) Stream(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.next(messages, options...)}), nil
}

func (model *planAskIntegrationModel) next(messages []*agent.Message, options ...agent.ModelOption) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	resolved := agent.GetCommonOptions(&agent.Options{}, options...)
	names := make([]string, 0, len(resolved.Tools))
	for _, info := range resolved.Tools {
		if info != nil {
			names = append(names, info.Name)
		}
	}
	model.toolSurfaces = append(model.toolSurfaces, names)
	var input strings.Builder
	for _, message := range messages {
		if message != nil {
			input.WriteString(message.Content)
			input.WriteByte('\n')
		}
	}
	model.modelInputs = append(model.modelInputs, input.String())
	if model.calls == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-plan-scope",
			Function: agent.FunctionCall{
				Name:      "ask",
				Arguments: `{"questions":[{"id":"scope","question":"Choose implementation scope","options":[{"id":"minimal","label":"Minimal","description":"Change only the shared planning flow."},{"id":"full","label":"Full","description":"Also redesign adjacent chat controls."}],"recommended_option_id":"minimal"}]}`,
			},
		}})
	}
	return agent.AssistantMessage("<proposed_plan># Implementation Plan\n\n1. Apply the minimal shared flow.</proposed_plan>", nil)
}

func (model *planAskIntegrationModel) snapshot() (int, [][]string, []string) {
	model.mu.Lock()
	defer model.mu.Unlock()
	toolSurfaces := make([][]string, len(model.toolSurfaces))
	for index := range model.toolSurfaces {
		toolSurfaces[index] = append([]string(nil), model.toolSurfaces[index]...)
	}
	return model.calls, toolSurfaces, append([]string(nil), model.modelInputs...)
}

type planAskNoopTool struct{ name string }

func (tool planAskNoopTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: tool.name}, nil
}

func (planAskNoopTool) Run(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
	return agent.TextToolResult("ok"), nil
}

func planAskToolDefinition(t *testing.T, name string, descriptor agent.ToolDescriptor) agent.ToolDefinition {
	t.Helper()
	definition, err := producttools.Define(planAskNoopTool{name: name}, descriptor)
	if err != nil {
		t.Fatalf("define %s: %v", name, err)
	}
	return definition
}

func planAskSessionDescriptor(capability string) agent.ToolDescriptor {
	return agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: capability,
		Execution: agent.ToolExecutionSessionExclusive, MutationScope: agent.ToolMutationSession,
		PostCheck: agent.ToolPostCheckSessionState, Recovery: agent.ToolRecoveryReconcilable,
		ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultProtected,
		Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1024,
	}
}

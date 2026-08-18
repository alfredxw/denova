package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

type writingPublicAskModel struct {
	mu     sync.Mutex
	calls  int
	inputs [][]*agent.Message
}

func (model *writingPublicAskModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.next(input), nil
}

func (model *writingPublicAskModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.next(input)}), nil
}

func (model *writingPublicAskModel) next(input []*agent.Message) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	cloned := make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			cloned[index] = message.Clone()
		}
	}
	model.inputs = append(model.inputs, cloned)
	if model.calls == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-writing-ask", Type: "function", Function: agent.FunctionCall{
				Name: "ask", Arguments: `{"questions":[{"id":"direction","prompt":"How should the story continue?","options":[{"value":"quiet","label":"Keep it quiet","description":"Keep the tone restrained.","recommended":true},{"value":"reveal","label":"Reveal a clue","description":"Advance the central mystery."}]}]}`,
			},
		}})
	}
	return agent.AssistantMessage("继续写作。 / Continue writing.", nil)
}

func TestWritingAskUsesPublicDurableInteractionThroughApplicationEndpoint(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
		OpenAIBaseURL: "https://example.invalid", OpenAIModel: "ask-model",
	}
	application, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	application.mu.RLock()
	sess := application.session
	runtime := application.executionRuntime
	workspace := application.workspace
	application.mu.RUnlock()
	if sess == nil || runtime == nil {
		t.Fatal("Writing Agent runtime is unavailable")
	}
	ask := publictools.Ask()
	model := &writingPublicAskModel{}
	pending := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	operation, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: agentexecution.Cycle{
		Definition: agent.Definition{
			Key: "denova.test.writing-public-ask", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.writing-public-ask", Version: 1},
			Tools:         ask,
		},
		Conversation: agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
			sess, cfg, agentrun.AgentKindIDE,
			"Test stable context", "Stable product context.",
			"Test turn context", "Current product state.",
		),
		Request: agentchat.ChatRequest{
			CommandID: "writing-public-ask", Message: "Help choose the next direction.",
		},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, StateRoot: cfg.ProjectStateDir,
			Workspace: workspace, SessionID: sess.ID, Mode: "ide", RootAgentName: "root",
		},
	}, Emit: func(event agentrun.Event) {
		if event.Type == "ask_pending" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan agentrun.Outcome, 1)
	go func() { outcomes <- operation.Wait(ctx) }()
	select {
	case <-pending:
	case outcome := <-outcomes:
		model.mu.Lock()
		inputs := append([][]*agent.Message(nil), model.inputs...)
		model.mu.Unlock()
		var rendered []string
		for _, input := range inputs {
			for _, message := range input {
				if message != nil {
					rendered = append(rendered, string(message.Role)+":"+message.Content)
				}
			}
		}
		t.Fatalf("Writing Agent settled before Ask: %#v inputs=%#v", outcome, rendered)
	case <-ctx.Done():
		t.Fatal("Writing Ask did not publish its durable pending Interaction")
	}

	view := application.WritingAgentActiveView(ctx)
	if !view.RuntimeProjectionOK || view.PendingAsk == nil {
		t.Fatalf("Writing active view has no public pending Ask: %#v", view)
	}
	if view.PendingAsk.AgentKind != agentrun.AgentKindIDE || view.PendingAsk.AgentCommandID != "writing-public-ask" ||
		view.PendingAsk.AgentOperationID == "" || view.PendingAsk.AgentCycle != 1 || !view.PendingAsk.AllowOther || len(view.PendingAsk.Questions) != 1 ||
		view.PendingAsk.Questions[0].RecommendedOptionID != "quiet" {
		t.Fatalf("pending Ask lost host Other/recommended projection: %#v", view.PendingAsk)
	}
	result, err := application.AnswerSessionAsk(ctx, sess.ID, view.PendingAsk.ID, []AgentAskAnswer{{
		QuestionID: "direction", SelectedOptionIDs: []string{"other"}, CustomInput: "让风暴提前到来 / Bring the storm forward",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != session.AskAnswered || len(result.Answers) != 1 ||
		result.Answers[0].CustomInput != "让风暴提前到来 / Bring the storm forward" {
		t.Fatalf("Writing Ask resolution = %#v", result)
	}
	if outcome := <-outcomes; outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("Writing Agent outcome = %#v", outcome)
	}
	if active := application.WritingAgentActiveView(ctx); active.PendingAsk != nil {
		t.Fatalf("resolved public Interaction remains pending: %#v", active.PendingAsk)
	}
	model.mu.Lock()
	inputs := append([][]*agent.Message(nil), model.inputs...)
	model.mu.Unlock()
	if len(inputs) != 2 || !strings.Contains(joinAgentMessageContent(inputs[1]), "Bring the storm forward") {
		t.Fatalf("resolved Ask did not resume the same model run: %#v", inputs)
	}
	displayAsks := 0
	for _, entry := range sess.History() {
		if entry.Type == "ask" {
			t.Fatalf("product Session persisted a canonical Ask authority: %#v", entry)
		}
		if entry.Role != "ask" {
			continue
		}
		displayAsks++
		if entry.Message != nil || entry.Ask == nil || entry.Status != session.AskAnswered ||
			entry.Ask.ID != view.PendingAsk.ID || len(entry.Ask.Questions) != 1 || len(entry.Ask.Answers) != 1 ||
			entry.Ask.Answers[0].CustomInput != "让风暴提前到来 / Bring the storm forward" {
			t.Fatalf("resolved display-only Ask history = %#v", entry)
		}
	}
	if displayAsks != 1 {
		t.Fatalf("resolved display-only Ask history count = %d, want 1", displayAsks)
	}
}

func joinAgentMessageContent(messages []*agent.Message) string {
	var joined strings.Builder
	for _, message := range messages {
		if message != nil {
			joined.WriteString(message.Content)
			joined.WriteByte('\n')
		}
	}
	return joined.String()
}

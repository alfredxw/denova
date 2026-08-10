package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/run"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func TestRunReturnsCompletedOutcome(t *testing.T) {
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: &agent.Message{
		Role:             agent.Assistant,
		Content:          "finished answer",
		ReasoningContent: "final thought",
	}}, true)
	conversation := &runControlConversation{}
	var events []agentrun.Event
	service := NewEphemeralRuntime()
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	outcome := runCycle(service,
		context.Background(),
		runner,
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "run-control-service", Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "run-control-test"},
		func(event agentrun.Event) { events = append(events, event) },
	)

	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil || outcome.Reason != "" {
		t.Fatalf("outcome = %#v, want completed", outcome)
	}
	if outcome.Content != "finished answer" || outcome.Thinking != "final thought" {
		t.Fatalf("final output = content %q thinking %q events=%#v", outcome.Content, outcome.Thinking, events)
	}
	if conversation.assistant != outcome.Content || conversation.thinking != outcome.Thinking {
		t.Fatalf("persisted output = content %q thinking %q", conversation.assistant, conversation.thinking)
	}
}

func TestRuntimePreemptWaitsForSafePointAndPersistsExistingAssistant(t *testing.T) {
	model := newRunControlTwoPhaseModel("draft before preempt", "thinking before preempt")
	runner := newRunControlTwoPhaseRunner(t, model)
	conversation := &runControlConversation{}
	controls := make(chan agentrun.Control, 2)
	chunkSeen := make(chan struct{}, 1)
	done := make(chan agentrun.Outcome, 1)
	var events []agentrun.Event

	runOutcomeTestGoroutine(done, "preempt run", func() agentrun.Outcome {
		return newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			agentchat.ChatRequest{Message: "write"},
			agentrun.Options{
				AgentKind:       config.AgentKindInteractiveDirector,
				RootAgentName:   "run-control-test",
				MaintenanceTask: "director_plan_update",
				Controls:        controls,
			},
			func(event agentrun.Event) {
				events = append(events, event)
				if event.Type == "chunk" {
					select {
					case chunkSeen <- struct{}{}:
					default:
					}
				}
			},
		)
	})

	waitRunControlSignal(t, chunkSeen, "assistant chunk")
	waitRunControlSignal(t, model.blocked, "second model call")
	controls <- agentrun.Control{Kind: agentrun.ControlPreempt, Reason: "new steering input"}
	controls <- agentrun.Control{Kind: agentrun.ControlPreempt, Reason: "duplicate steering input"}
	select {
	case outcome := <-done:
		t.Fatalf("preempt returned before the model safe point: %#v", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	close(model.release)
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != agentrun.OutcomePreempted || outcome.Error != nil || outcome.Reason != "new steering input" {
		t.Fatalf("outcome = %#v, want preempted", outcome)
	}
	if outcome.Content != "draft before preemptmodel output after control" || outcome.Thinking != "thinking before preempt" {
		t.Fatalf("final output = content %q thinking %q", outcome.Content, outcome.Thinking)
	}
	if conversation.assistant != outcome.Content || conversation.thinking != outcome.Thinking {
		t.Fatalf("preempted assistant was not persisted: %#v", conversation)
	}
	if conversation.interruptions != 0 {
		t.Fatalf("preempt must not mark an interruption: %d", conversation.interruptions)
	}
	if countEventType(events, "error") != 0 || countEventType(events, "aborted") != 0 {
		t.Fatalf("preempt emitted an ordinary terminal error: %#v", events)
	}
}

func TestRuntimeAbortIsImmediateAndEmitsAbortedWithoutOrdinaryError(t *testing.T) {
	model := newRunControlTwoPhaseModel("draft before abort", "thinking before abort")
	defer close(model.release)
	runner := newRunControlTwoPhaseRunner(t, model)
	conversation := &runControlConversation{}
	controls := make(chan agentrun.Control, 1)
	chunkSeen := make(chan struct{}, 1)
	done := make(chan agentrun.Outcome, 1)
	var events []agentrun.Event

	runOutcomeTestGoroutine(done, "abort run", func() agentrun.Outcome {
		return newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			agentchat.ChatRequest{Message: "write"},
			agentrun.Options{AgentKind: agentrun.AgentKindUnknown, RootAgentName: "run-control-test", Controls: controls},
			func(event agentrun.Event) {
				events = append(events, event)
				if event.Type == "chunk" {
					select {
					case chunkSeen <- struct{}{}:
					default:
					}
				}
			},
		)
	})

	waitRunControlSignal(t, chunkSeen, "assistant chunk")
	waitRunControlSignal(t, model.blocked, "second model call")
	controls <- agentrun.Control{Kind: agentrun.ControlAbort, Reason: "user stopped the run"}
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != agentrun.OutcomeAborted || outcome.Error != nil || outcome.Reason != "user stopped the run" {
		t.Fatalf("outcome = %#v, want aborted", outcome)
	}
	if outcome.Content != "draft before abort" || outcome.Thinking != "thinking before abort" {
		t.Fatalf("final output = content %q thinking %q", outcome.Content, outcome.Thinking)
	}
	if conversation.assistant != "" || conversation.thinking != "" {
		t.Fatalf("aborted assistant crossed the domain commit barrier: %#v", conversation)
	}
	if conversation.interruptions != 0 {
		t.Fatalf("abort must not mark an interruption: %d", conversation.interruptions)
	}
	if got := countEventType(events, "aborted"); got != 1 {
		t.Fatalf("aborted event count = %d, events = %#v", got, events)
	}
	if got := countEventType(events, "error"); got != 0 {
		t.Fatalf("control abort emitted an ordinary error: events = %#v", events)
	}
}

func TestRuntimeAbortEscalatesPendingPreempt(t *testing.T) {
	model := newRunControlTwoPhaseModel("draft before escalation", "")
	defer close(model.release)
	runner := newRunControlTwoPhaseRunner(t, model)
	conversation := &runControlConversation{}
	controls := make(chan agentrun.Control, 2)
	done := make(chan agentrun.Outcome, 1)

	runOutcomeTestGoroutine(done, "abort escalation run", func() agentrun.Outcome {
		return newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			agentchat.ChatRequest{Message: "write"},
			agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Controls: controls},
			nil,
		)
	})

	waitRunControlSignal(t, model.blocked, "second model call")
	controls <- agentrun.Control{Kind: agentrun.ControlPreempt, Reason: "steer pending"}
	controls <- agentrun.Control{Kind: agentrun.ControlAbort, Reason: "stop instead"}
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != agentrun.OutcomeAborted || outcome.Error != nil || outcome.Reason != "stop instead" {
		t.Fatalf("escalated outcome = %#v, want abort", outcome)
	}
	if conversation.interruptions != 0 {
		t.Fatalf("escalated abort must not mark an interruption: %d", conversation.interruptions)
	}
}

func TestRuntimeClosedControlsChannelDoesNotAbort(t *testing.T) {
	controls := make(chan agentrun.Control)
	close(controls)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("done", nil)}, true)

	outcome := newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
		context.Background(),
		runner,
		&runControlConversation{},
		nil,
		agentchat.ChatRequest{Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindAutomation, RootAgentName: "run-control-test", Controls: controls},
		nil,
	)

	if outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("closed controls channel outcome = %#v, want completed", outcome)
	}
}

func TestRuntimeCanceledContextReturnsAbortedOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var events []agentrun.Event

	outcome := newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
		ctx,
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("unused", nil)}, true),
		&runControlConversation{},
		nil,
		agentchat.ChatRequest{Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test"},
		func(event agentrun.Event) { events = append(events, event) },
	)

	if outcome.Status != agentrun.OutcomeAborted || !errors.Is(outcome.Error, context.Canceled) || outcome.Reason != context.Canceled.Error() {
		t.Fatalf("outcome = %#v, want context-canceled abort", outcome)
	}
	if countEventType(events, "aborted") != 1 || countEventType(events, "error") != 0 {
		t.Fatalf("context cancellation events = %#v", events)
	}
}

func TestRuntimeOrdinaryRunnerErrorReturnsFailedOutcome(t *testing.T) {
	wantErr := errors.New("provider failed")
	conversation := &runControlConversation{}
	var events []agentrun.Event

	outcome := newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{err: wantErr}, true),
		conversation,
		nil,
		agentchat.ChatRequest{Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test"},
		func(event agentrun.Event) { events = append(events, event) },
	)

	if outcome.Status != agentrun.OutcomeFailed || outcome.Error == nil || !strings.Contains(outcome.Error.Error(), wantErr.Error()) || !strings.Contains(outcome.Reason, wantErr.Error()) {
		t.Fatalf("outcome = %#v, want failed", outcome)
	}
	if conversation.interruptions != 1 {
		t.Fatalf("ordinary failure interruption count = %d, want 1", conversation.interruptions)
	}
	if conversation.assistant != "" || conversation.thinking != "" {
		t.Fatalf("failed assistant crossed the domain commit barrier: %#v", conversation)
	}
	if countEventType(events, "error") != 1 || countEventType(events, "aborted") != 0 {
		t.Fatalf("ordinary failure events = %#v", events)
	}
}

func TestRuntimePanicAfterPersistenceKeepsFinalOutputInFailedOutcome(t *testing.T) {
	conversation := &runControlConversation{}

	outcome := newTestExecutor(agentrun.DefaultLoopPolicy()).Run(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: &agent.Message{
			Role:             agent.Assistant,
			Content:          "persisted before panic",
			ReasoningContent: "thinking before panic",
		}}, true),
		conversation,
		nil,
		agentchat.ChatRequest{Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test"},
		func(event agentrun.Event) {
			if event.Type == "done" {
				panic("transport panic")
			}
		},
	)

	if outcome.Status != agentrun.OutcomeFailed || outcome.Error == nil {
		t.Fatalf("outcome = %#v, want failed panic", outcome)
	}
	if outcome.Content != conversation.assistant || outcome.Thinking != conversation.thinking {
		t.Fatalf("panic outcome lost persisted output: outcome=%#v conversation=%#v", outcome, conversation)
	}
}

func newRunControlTestRunner(t *testing.T, chatModel agent.BaseChatModel, streaming bool) *agent.Runner {
	t.Helper()
	builtAgent, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name:        "run-control-test",
		Description: "run control test",
		Instruction: "test",
		Model:       chatModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: streaming})
}

func newRunControlTwoPhaseRunner(t *testing.T, chatModel agent.BaseChatModel) *agent.Runner {
	t.Helper()
	builtAgent, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name:        "run-control-test",
		Description: "run control test",
		Instruction: "test",
		Model:       chatModel,
		Tools: []agent.ToolDefinition{{
			Tool: runControlTestTool{},
			Descriptor: agent.ToolDescriptor{
				Source: agent.ToolSourceOther, Execution: agent.ToolExecutionParallelRead,
				MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
				Recovery: agent.ToolRecoveryIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
				ResultRetention: agent.ToolResultDeferred,
				Steering:        agent.SteeringFinishCurrent, MaxResultBytes: 64 * 1024,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: true})
}

func waitRunControlSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitRunControlOutcome(t *testing.T, done <-chan agentrun.Outcome) agentrun.Outcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for run outcome")
		return agentrun.Outcome{}
	}
}

type runControlConversation struct {
	assistant     string
	thinking      string
	interruptions int
}

func (c *runControlConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input agentcontext.ModelContextInput,
) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

func (c *runControlConversation) AppendAssistant(content string) error {
	c.assistant = content
	return nil
}

func (c *runControlConversation) AppendAssistantWithThinking(content, thinking string) error {
	c.assistant = content
	c.thinking = thinking
	return nil
}

func (c *runControlConversation) MarkInterrupted(_, _, _ string) error {
	c.interruptions++
	return nil
}

func (c *runControlConversation) PendingInterruption() *session.Interruption { return nil }

func (c *runControlConversation) ResolveInterruption(string) error { return nil }

type runControlFixedModel struct {
	message *agent.Message
	err     error
}

func (m *runControlFixedModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return m.message, m.err
}

func (m *runControlFixedModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	if m.err != nil {
		return nil, m.err
	}
	return agent.StreamReaderFromArray([]*agent.Message{m.message}), nil
}

type runControlTwoPhaseModel struct {
	content  string
	thinking string
	blocked  chan struct{}
	release  chan struct{}
	mu       sync.Mutex
	calls    int
}

func newRunControlTwoPhaseModel(content, thinking string) *runControlTwoPhaseModel {
	return &runControlTwoPhaseModel{
		content:  content,
		thinking: thinking,
		blocked:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (m *runControlTwoPhaseModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return m.next(ctx)
}

func (m *runControlTwoPhaseModel) Stream(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.next(ctx)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (m *runControlTwoPhaseModel) next(context.Context) (*agent.Message, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		return &agent.Message{
			Role:             agent.Assistant,
			Content:          m.content,
			ReasoningContent: m.thinking,
			ToolCalls: []agent.ToolCall{{
				ID: "run-control-tool-call",
				Function: agent.FunctionCall{
					Name:      "run_control_test_tool",
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	if call == 2 {
		close(m.blocked)
		<-m.release
		return &agent.Message{
			Role:    agent.Assistant,
			Content: "model output after control",
			ToolCalls: []agent.ToolCall{{
				ID: "run-control-second-tool-call",
				Function: agent.FunctionCall{
					Name:      "run_control_test_tool",
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	return agent.AssistantMessage("model output after second tool", nil), nil
}

type runControlTestTool struct{}

func (runControlTestTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{
		Name:        "run_control_test_tool",
		Desc:        "complete the first test phase",
		ParamsOneOf: agent.NewParamsOneOfByParams(map[string]*agent.ParameterInfo{}),
	}, nil
}

func (runControlTestTool) Run(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
	return agent.TextToolResult("first phase complete"), nil
}

var _ agent.BaseChatModel = (*runControlFixedModel)(nil)
var _ agent.BaseChatModel = (*runControlTwoPhaseModel)(nil)
var _ agent.Tool = runControlTestTool{}

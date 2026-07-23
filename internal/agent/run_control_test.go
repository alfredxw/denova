package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alfredxw/denova/adk"

	"denova/config"
	"denova/internal/agent/session"
)

func TestRunWithOptionsReturnsCompletedOutcome(t *testing.T) {
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: &adk.Message{
		Role:             adk.Assistant,
		Content:          "finished answer",
		ReasoningContent: "final thought",
	}}, true)
	conversation := &runControlConversation{}
	var events []Event
	service := NewEphemeralChatService()
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	outcome := service.RunWithOptions(
		context.Background(),
		runner,
		conversation,
		nil,
		ChatRequest{CommandID: "run-control-service", Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "run-control-test"},
		func(event Event) { events = append(events, event) },
	)

	if outcome.Status != RunOutcomeCompleted || outcome.Error != nil || outcome.Reason != "" {
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
	controls := make(chan RunControl, 2)
	chunkSeen := make(chan struct{}, 1)
	done := make(chan RunOutcome, 1)
	var events []Event

	runOutcomeTestGoroutine(done, "preempt run", func() RunOutcome {
		return newTurnExecutor(DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			ChatRequest{Message: "write"},
			RunOptions{
				AgentKind:       config.AgentKindInteractiveDirector,
				RootAgentName:   "run-control-test",
				MaintenanceTask: "director_plan_update",
				Controls:        controls,
			},
			func(event Event) {
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
	controls <- RunControl{Kind: RunControlPreempt, Reason: "new steering input"}
	controls <- RunControl{Kind: RunControlPreempt, Reason: "duplicate steering input"}
	select {
	case outcome := <-done:
		t.Fatalf("preempt returned before the model safe point: %#v", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	close(model.release)
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != RunOutcomePreempted || outcome.Error != nil || outcome.Reason != "new steering input" {
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
	controls := make(chan RunControl, 1)
	chunkSeen := make(chan struct{}, 1)
	done := make(chan RunOutcome, 1)
	var events []Event

	runOutcomeTestGoroutine(done, "abort run", func() RunOutcome {
		return newTurnExecutor(DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			ChatRequest{Message: "write"},
			RunOptions{AgentKind: AgentKindUnknown, RootAgentName: "run-control-test", Controls: controls},
			func(event Event) {
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
	controls <- RunControl{Kind: RunControlAbort, Reason: "user stopped the run"}
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != RunOutcomeAborted || outcome.Error != nil || outcome.Reason != "user stopped the run" {
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
	controls := make(chan RunControl, 2)
	done := make(chan RunOutcome, 1)

	runOutcomeTestGoroutine(done, "abort escalation run", func() RunOutcome {
		return newTurnExecutor(DefaultLoopPolicy()).Run(
			context.Background(),
			runner,
			conversation,
			nil,
			ChatRequest{Message: "write"},
			RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Controls: controls},
			nil,
		)
	})

	waitRunControlSignal(t, model.blocked, "second model call")
	controls <- RunControl{Kind: RunControlPreempt, Reason: "steer pending"}
	controls <- RunControl{Kind: RunControlAbort, Reason: "stop instead"}
	outcome := waitRunControlOutcome(t, done)

	if outcome.Status != RunOutcomeAborted || outcome.Error != nil || outcome.Reason != "stop instead" {
		t.Fatalf("escalated outcome = %#v, want abort", outcome)
	}
	if conversation.interruptions != 0 {
		t.Fatalf("escalated abort must not mark an interruption: %d", conversation.interruptions)
	}
}

func TestRuntimeClosedControlsChannelDoesNotAbort(t *testing.T) {
	controls := make(chan RunControl)
	close(controls)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("done", nil)}, true)

	outcome := newTurnExecutor(DefaultLoopPolicy()).Run(
		context.Background(),
		runner,
		&runControlConversation{},
		nil,
		ChatRequest{Message: "write"},
		RunOptions{AgentKind: AgentKindAutomation, RootAgentName: "run-control-test", Controls: controls},
		nil,
	)

	if outcome.Status != RunOutcomeCompleted {
		t.Fatalf("closed controls channel outcome = %#v, want completed", outcome)
	}
}

func TestRuntimeCanceledContextReturnsAbortedOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var events []Event

	outcome := newTurnExecutor(DefaultLoopPolicy()).Run(
		ctx,
		newRunControlTestRunner(t, &runControlFixedModel{message: adk.AssistantMessage("unused", nil)}, true),
		&runControlConversation{},
		nil,
		ChatRequest{Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		func(event Event) { events = append(events, event) },
	)

	if outcome.Status != RunOutcomeAborted || !errors.Is(outcome.Error, context.Canceled) || outcome.Reason != context.Canceled.Error() {
		t.Fatalf("outcome = %#v, want context-canceled abort", outcome)
	}
	if countEventType(events, "aborted") != 1 || countEventType(events, "error") != 0 {
		t.Fatalf("context cancellation events = %#v", events)
	}
}

func TestRuntimeOrdinaryRunnerErrorReturnsFailedOutcome(t *testing.T) {
	wantErr := errors.New("provider failed")
	conversation := &runControlConversation{}
	var events []Event

	outcome := newTurnExecutor(DefaultLoopPolicy()).Run(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{err: wantErr}, true),
		conversation,
		nil,
		ChatRequest{Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		func(event Event) { events = append(events, event) },
	)

	if outcome.Status != RunOutcomeFailed || outcome.Error == nil || !strings.Contains(outcome.Error.Error(), wantErr.Error()) || !strings.Contains(outcome.Reason, wantErr.Error()) {
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

	outcome := newTurnExecutor(DefaultLoopPolicy()).Run(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: &adk.Message{
			Role:             adk.Assistant,
			Content:          "persisted before panic",
			ReasoningContent: "thinking before panic",
		}}, true),
		conversation,
		nil,
		ChatRequest{Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test"},
		func(event Event) {
			if event.Type == "done" {
				panic("transport panic")
			}
		},
	)

	if outcome.Status != RunOutcomeFailed || outcome.Error == nil {
		t.Fatalf("outcome = %#v, want failed panic", outcome)
	}
	if outcome.Content != conversation.assistant || outcome.Thinking != conversation.thinking {
		t.Fatalf("panic outcome lost persisted output: outcome=%#v conversation=%#v", outcome, conversation)
	}
}

func TestRunControlWatcherRecoversCancellationPanicAndExits(t *testing.T) {
	controls := make(chan RunControl, 1)
	controls <- RunControl{Kind: RunControlAbort, Reason: "test panic"}
	close(controls)
	done := startRunControlWatcher(
		context.Background(),
		controls,
		func(...adk.AgentCancelOption) (*adk.CancelHandle, bool) {
			panic("cancel panic")
		},
		&runControlState{},
	)

	waitRunControlSignal(t, done, "recovered control watcher exit")
}

func newRunControlTestRunner(t *testing.T, chatModel adk.BaseChatModel, streaming bool) *adk.Runner {
	t.Helper()
	builtAgent, err := adk.NewAgent(context.Background(), adk.AgentConfig{
		Name:        "run-control-test",
		Description: "run control test",
		Instruction: "test",
		Model:       chatModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adk.NewRunner(adk.RunnerConfig{Agent: builtAgent, EnableStreaming: streaming})
}

func newRunControlTwoPhaseRunner(t *testing.T, chatModel adk.BaseChatModel) *adk.Runner {
	t.Helper()
	builtAgent, err := adk.NewAgent(context.Background(), adk.AgentConfig{
		Name:        "run-control-test",
		Description: "run control test",
		Instruction: "test",
		Model:       chatModel,
		Tools:       []adk.BaseTool{runControlTestTool{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adk.NewRunner(adk.RunnerConfig{Agent: builtAgent, EnableStreaming: true})
}

func waitRunControlSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitRunControlOutcome(t *testing.T, done <-chan RunOutcome) RunOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for run outcome")
		return RunOutcome{}
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
	input ModelContextInput,
) (ModelContextResult, error) {
	return AssembleSingleUserModelContext(ctx, input)
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
	message *adk.Message
	err     error
}

func (m *runControlFixedModel) Generate(context.Context, []*adk.Message, ...adk.ModelOption) (*adk.Message, error) {
	return m.message, m.err
}

func (m *runControlFixedModel) Stream(context.Context, []*adk.Message, ...adk.ModelOption) (*adk.StreamReader[*adk.Message], error) {
	if m.err != nil {
		return nil, m.err
	}
	return adk.StreamReaderFromArray([]*adk.Message{m.message}), nil
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

func (m *runControlTwoPhaseModel) Generate(ctx context.Context, _ []*adk.Message, _ ...adk.ModelOption) (*adk.Message, error) {
	return m.next(ctx)
}

func (m *runControlTwoPhaseModel) Stream(ctx context.Context, _ []*adk.Message, _ ...adk.ModelOption) (*adk.StreamReader[*adk.Message], error) {
	message, err := m.next(ctx)
	if err != nil {
		return nil, err
	}
	return adk.StreamReaderFromArray([]*adk.Message{message}), nil
}

func (m *runControlTwoPhaseModel) next(context.Context) (*adk.Message, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		return &adk.Message{
			Role:             adk.Assistant,
			Content:          m.content,
			ReasoningContent: m.thinking,
			ToolCalls: []adk.ToolCall{{
				ID: "run-control-tool-call",
				Function: adk.FunctionCall{
					Name:      "run_control_test_tool",
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	if call == 2 {
		close(m.blocked)
		<-m.release
		return &adk.Message{
			Role:    adk.Assistant,
			Content: "model output after control",
			ToolCalls: []adk.ToolCall{{
				ID: "run-control-second-tool-call",
				Function: adk.FunctionCall{
					Name:      "run_control_test_tool",
					Arguments: `{}`,
				},
			}},
		}, nil
	}
	return adk.AssistantMessage("model output after second tool", nil), nil
}

type runControlTestTool struct{}

func (runControlTestTool) Info(context.Context) (*adk.ToolInfo, error) {
	return &adk.ToolInfo{
		Name:        "run_control_test_tool",
		Desc:        "complete the first test phase",
		ParamsOneOf: adk.NewParamsOneOfByParams(map[string]*adk.ParameterInfo{}),
	}, nil
}

func (runControlTestTool) InvokableRun(context.Context, string, ...adk.ToolOption) (string, error) {
	return "first phase complete", nil
}

var _ adk.BaseChatModel = (*runControlFixedModel)(nil)
var _ adk.BaseChatModel = (*runControlTwoPhaseModel)(nil)
var _ adk.InvokableTool = runControlTestTool{}

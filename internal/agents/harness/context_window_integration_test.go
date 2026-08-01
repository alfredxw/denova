package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	runstate "github.com/alfredxw/denova/agent/runtime"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	producttools "denova/internal/agents/tools"
)

func TestContextRewindKeepsExplorationDisplayButCommitsOnlyFinalAnswer(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-rewind-output")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE)

	definitions, err := producttools.NewCatalog(nil, nil, producttools.RuntimeExecutables{}).ContextWindow(
		config.ResolvedAgentToolSettings{config.AgentToolContextRewind: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	model := &contextRewindSequenceModel{}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "context-rewind-integration", Description: "context rewind integration",
		Instruction: "test context rewind", Model: model, Tools: definitions, MaxIterations: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: built, EnableStreaming: false})
	service, err := newService(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	var displayed strings.Builder
	outcome := service.RunWithOptions(
		context.Background(), runner, conversation, nil,
		agentchat.ChatRequest{CommandID: "context-rewind-output", Message: "research then answer"},
		agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, RootAgentName: "context-rewind-integration",
			Workspace: "context-rewind-workspace", SessionID: sess.ID,
		},
		func(event agentrun.Event) {
			if event.Type != "chunk" {
				return
			}
			if data, ok := event.Data.(map[string]interface{}); ok {
				if content, ok := data["content"].(string); ok {
					displayed.WriteString(content)
				}
			}
		},
	)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil {
		t.Fatalf("rewind run outcome = %+v", outcome)
	}
	if outcome.Content != "final answer after rewind" || strings.Contains(outcome.Content, "discarded exploratory prose") {
		t.Fatalf("effective outcome content = %q", outcome.Content)
	}
	if got := displayed.String(); !strings.Contains(got, "discarded exploratory prose") || !strings.Contains(got, "final answer after rewind") {
		t.Fatalf("append-only display content = %q", got)
	}

	snapshot, err := sess.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	// Mid-run tool protocol remains append-only for crash recovery. The durable
	// rewind projection, rather than physical deletion, keeps discarded
	// exploration out of every later model input.
	joinedRaw := joinedContextWindowMessages(snapshot.EffectiveMessages)
	if !strings.Contains(joinedRaw, "discarded exploratory prose") {
		t.Fatalf("append-only canonical transcript lost exploration evidence: %q", joinedRaw)
	}
	projection, err := conversation.SnapshotContextCompaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	projected := joinedContextWindowMessages(projection.Messages)
	if strings.Contains(projected, "discarded exploratory prose") || !strings.Contains(projected, "final answer after rewind") {
		t.Fatalf("rewound model projection = %q", projected)
	}
	thirdInput := joinedContextWindowMessages(model.input(2))
	if strings.Contains(thirdInput, "discarded exploratory prose") || !strings.Contains(thirdInput, agentcontext.RewindSummaryPrefix) {
		t.Fatalf("post-rewind model input = %q", thirdInput)
	}
}

func TestCorruptLatestRewindStopsBeforeModelExecution(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("context-corrupt-rewind")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("stable prefix")); err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	boundary, err := session.NewContextBoundarySnapshot(
		cursor,
		[]*agent.Message{agent.UserMessage("stable prefix")},
		[]*agent.Message{agent.UserMessage("stable prefix")},
		4*1024*1024,
	)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := sess.StoreContextBoundary("cp-corrupt-rewind", boundary)
	if err != nil {
		t.Fatal(err)
	}
	locator.SHA256 = strings.Repeat("0", 64)
	if err := sess.AppendWithMetadata(agent.AssistantMessage("unsafe rewind", nil), session.MessageMetadata{
		ContextOperations: []session.ContextOperation{{
			Kind: session.ContextOperationRewind, AgentKind: agentrun.AgentKindIDE,
			CheckpointID: "cp-corrupt-rewind", MessageCount: cursor.MessageCount,
			BoundaryID: "cp-corrupt-rewind", BoundaryLocator: locator, Report: "must not be trusted",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	model := &contextRewindSequenceModel{}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "context-corrupt-rewind", Description: "context corruption guard",
		Instruction: "must not execute", Model: model, MaxIterations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: built, EnableStreaming: false})
	service, err := newService(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	outcome := service.RunWithOptions(
		context.Background(), runner, agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE), nil,
		agentchat.ChatRequest{CommandID: "context-corrupt-rewind", Message: "continue"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "context-corrupt-rewind", Workspace: "context-corrupt-rewind", SessionID: sess.ID},
		nil,
	)
	if outcome.Error == nil || !strings.Contains(outcome.Error.Error(), "invalid durable boundary") {
		t.Fatalf("corrupt rewind outcome = %+v", outcome)
	}
	if calls := model.callCount(); calls != 0 {
		t.Fatalf("model executed %d times with a corrupt rewind projection", calls)
	}
}

type contextRewindSequenceModel struct {
	mu     sync.Mutex
	calls  int
	inputs [][]*agent.Message
}

func (m *contextRewindSequenceModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	m.mu.Lock()
	m.inputs = append(m.inputs, agentcontext.CloneMessages(input))
	m.calls++
	call := m.calls
	m.mu.Unlock()
	switch call {
	case 1:
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "checkpoint-call", Type: "function",
			Function: agent.FunctionCall{Name: "checkpoint", Arguments: `{"purpose":"inspect implementation"}`},
		}}), nil
	case 2:
		return agent.AssistantMessage("discarded exploratory prose", []agent.ToolCall{{
			ID: "rewind-call", Type: "function",
			Function: agent.FunctionCall{Name: "rewind", Arguments: `{"report":"bounded retained finding"}`},
		}}), nil
	default:
		return agent.AssistantMessage("final answer after rewind", nil), nil
	}
}

func (m *contextRewindSequenceModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (m *contextRewindSequenceModel) input(index int) []*agent.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.inputs) {
		return nil
	}
	return agentcontext.CloneMessages(m.inputs[index])
}

func (m *contextRewindSequenceModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func joinedContextWindowMessages(messages []*agent.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

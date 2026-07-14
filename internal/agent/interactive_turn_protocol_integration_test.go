package agent

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"denova/internal/interactive"
	"denova/internal/session"
)

func TestInteractiveTurnProtocolRecoversMissingSubmissionInsideAgentLoop(t *testing.T) {
	ctx := context.Background()
	var ready atomic.Bool
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, _ interactive.TurnResult) (interactive.TurnSubmissionReceipt, error) {
			ready.Store(true)
			return interactive.TurnSubmissionReceipt{Accepted: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chatModel := &interactiveTurnProtocolChatModel{}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "interactive-protocol-test",
		Description:   "test",
		Instruction:   "test",
		Model:         chatModel,
		MaxIterations: 4,
		Handlers:      []adk.ChatModelAgentMiddleware{newInteractiveTurnProtocolMiddleware(ready.Load)},
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		}},
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  1,
			ShouldRetry: newInteractiveCompletionGuard(ready.Load),
			BackoffFunc: func(context.Context, int) time.Duration { return time.Nanosecond },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	iterator := runner.Run(ctx, []*schema.Message{schema.UserMessage("推开石门")})
	final := ""
	sawInternalRetry := false
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			if _, retrying := interactiveCompletionRetryFromError(event.Err); retrying {
				continue
			}
			t.Fatalf("agent loop failed: %v", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, streamErr := readInteractiveProtocolMessage(event.Output.MessageOutput)
		if streamErr != nil {
			if _, retrying := interactiveCompletionRetryFromError(streamErr); retrying {
				sawInternalRetry = true
				continue
			}
			t.Fatalf("message stream failed: %v", streamErr)
		}
		if message != nil && message.Role == schema.Assistant && len(message.ToolCalls) == 0 {
			final = message.Content
		}
	}

	calls, toolCounts, inputs := chatModel.snapshot()
	if !sawInternalRetry || !ready.Load() || calls != 3 || final != "石门缓缓开启。" {
		t.Fatalf("protocol did not recover in one streaming run: retry=%t ready=%t calls=%d final=%q", sawInternalRetry, ready.Load(), calls, final)
	}
	if len(toolCounts) != 3 || toolCounts[0] == 0 || toolCounts[1] == 0 || toolCounts[2] == 0 {
		t.Fatalf("tool schemas should remain stable across the narrative-only phase: %#v", toolCounts)
	}
	toolChoices := chatModel.toolChoicesSnapshot()
	if len(toolChoices) != 3 || toolChoices[2] != string(schema.ToolChoiceForbidden) {
		t.Fatalf("accepted TurnResult phase must set tool_choice=none without changing schemas: %#v", toolChoices)
	}
	if len(inputs) < 2 || !messageSliceContains(inputs[1], "backend completion guard") {
		t.Fatalf("retry did not receive bounded protocol feedback: %#v", inputs)
	}
	if len(inputs) < 3 || messageSliceContains(inputs[2], "backend completion guard") || messageSliceContains(inputs[2], "Rejected narrative candidate") {
		t.Fatalf("ephemeral retry feedback leaked into the accepted conversation state: %#v", inputs)
	}
}

func TestInteractiveTurnProtocolAccountsRejectedModelCallUsage(t *testing.T) {
	ctx := context.Background()
	var ready atomic.Bool
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, _ interactive.TurnResult) (interactive.TurnSubmissionReceipt, error) {
			ready.Store(true)
			return interactive.TurnSubmissionReceipt{Accepted: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chatModel := &interactiveTurnProtocolChatModel{}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "interactive-protocol-usage-test",
		Description:   "test",
		Instruction:   "test",
		Model:         chatModel,
		MaxIterations: 4,
		Handlers:      []adk.ChatModelAgentMiddleware{newInteractiveTurnProtocolMiddleware(ready.Load)},
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: tools,
		}},
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  1,
			ShouldRetry: newInteractiveCompletionGuard(ready.Load),
			BackoffFunc: func(context.Context, int) time.Duration { return time.Nanosecond },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	conversation := &interactiveProtocolConversation{ready: &ready}
	var usage map[string]any
	NewRuntime(DefaultLoopPolicy()).Run(ctx, runner, conversation, nil, ChatRequest{Message: "推开石门"}, RunOptions{
		AgentKind:     AgentKindInteractiveStory,
		RootAgentName: "interactive-protocol-usage-test",
	}, func(event Event) {
		if event.Type == "token_usage" {
			usage, _ = event.Data.(map[string]any)
		}
	})

	calls, toolCounts, inputs := chatModel.snapshot()
	if conversation.assistant != "石门缓缓开启。" {
		t.Fatalf("final narrative = %q ready=%t calls=%d tools=%#v inputs=%#v usage=%#v", conversation.assistant, ready.Load(), calls, toolCounts, inputs, usage)
	}
	if usage == nil || usage["model_calls"] != 3 || usage["total_tokens"] != 660 {
		t.Fatalf("usage must include rejected, tool-call, and final model responses: %#v", usage)
	}
}

type interactiveProtocolConversation struct {
	ready     *atomic.Bool
	assistant string
}

func (c *interactiveProtocolConversation) PrepareMessages(_, agentMessage string) ([]*schema.Message, error) {
	return []*schema.Message{schema.UserMessage(agentMessage)}, nil
}
func (c *interactiveProtocolConversation) AppendAssistant(content string) error {
	c.assistant = content
	return nil
}
func (c *interactiveProtocolConversation) MarkInterrupted(_, _, _ string) error { return nil }
func (c *interactiveProtocolConversation) PendingInterruption() *session.Interruption {
	return nil
}
func (c *interactiveProtocolConversation) ResolveInterruption(string) error { return nil }
func (c *interactiveProtocolConversation) InteractiveNarrativeReady() bool {
	return c != nil && c.ready != nil && c.ready.Load()
}

type interactiveTurnProtocolChatModel struct {
	mu          sync.Mutex
	calls       int
	toolCounts  []int
	toolChoices []string
	inputs      [][]string
}

func (m *interactiveTurnProtocolChatModel) Generate(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.nextMessage(messages, opts...)
}

func (m *interactiveTurnProtocolChatModel) Stream(_ context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.nextMessage(messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *interactiveTurnProtocolChatModel) nextMessage(messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	common := model.GetCommonOptions(&model.Options{}, opts...)
	m.toolCounts = append(m.toolCounts, len(common.Tools))
	toolChoice := ""
	if common.ToolChoice != nil {
		toolChoice = string(*common.ToolChoice)
	}
	m.toolChoices = append(m.toolChoices, toolChoice)
	input := make([]string, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			input = append(input, message.Content)
		}
	}
	m.inputs = append(m.inputs, input)
	var message *schema.Message
	switch m.calls {
	case 1:
		message = schema.AssistantMessage("门后传来锁链拖地的声音。", nil)
	case 2:
		message = schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-submit",
			Function: schema.FunctionCall{
				Name:      "submit_interactive_turn_result",
				Arguments: `{"contract":{"player_intent":"推开石门","scene_goal":"进入门后"},"scene_result":{"status":"continued"},"plan_signals":{"deviation_level":"none"},"choices":["进入房间","观察门后"]}`,
			},
		}})
	default:
		message = schema.AssistantMessage("石门缓缓开启。", nil)
	}
	message.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     m.calls * 100,
		CompletionTokens: m.calls * 10,
		TotalTokens:      m.calls * 110,
	}}
	return message, nil
}

func (m *interactiveTurnProtocolChatModel) snapshot() (int, []int, [][]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	toolCounts := append([]int(nil), m.toolCounts...)
	inputs := make([][]string, len(m.inputs))
	for index := range m.inputs {
		inputs[index] = append([]string(nil), m.inputs[index]...)
	}
	return m.calls, toolCounts, inputs
}

func (m *interactiveTurnProtocolChatModel) toolChoicesSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.toolChoices...)
}

func messageSliceContains(messages []string, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func readInteractiveProtocolMessage(variant *adk.MessageVariant) (*schema.Message, error) {
	if variant == nil {
		return nil, nil
	}
	if !variant.IsStreaming || variant.MessageStream == nil {
		return variant.Message, nil
	}
	variant.MessageStream.SetAutomaticClose()
	defer variant.MessageStream.Close()
	chunks := make([]*schema.Message, 0, 1)
	for {
		chunk, err := variant.MessageStream.Recv()
		if err == nil {
			chunks = append(chunks, chunk)
			continue
		}
		if err != io.EOF {
			return nil, err
		}
		break
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return schema.ConcatMessages(chunks)
}

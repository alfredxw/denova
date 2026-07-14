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
		SubmitTurnResult: newProtocolSubmissionCollector(&ready),
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
	if len(inputs) < 3 || messageSliceContains(inputs[2], "backend completion guard") || messageSliceContains(inputs[2], "Retained narrative candidate") {
		t.Fatalf("ephemeral retry feedback leaked into the accepted conversation state: %#v", inputs)
	}
}

func TestInteractiveTurnProtocolAccountsRejectedModelCallUsage(t *testing.T) {
	ctx := context.Background()
	var ready atomic.Bool
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: newProtocolSubmissionCollector(&ready),
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
	var events []Event
	NewRuntime(DefaultLoopPolicy()).Run(ctx, runner, conversation, nil, ChatRequest{Message: "推开石门"}, RunOptions{
		AgentKind:     AgentKindInteractiveStory,
		RootAgentName: "interactive-protocol-usage-test",
	}, func(event Event) {
		events = append(events, event)
		if event.Type == "token_usage" {
			usage, _ = event.Data.(map[string]any)
		}
	})

	calls, toolCounts, inputs := chatModel.snapshot()
	if conversation.assistant != "门后传来锁链拖地的声音。" {
		t.Fatalf("final narrative = %q ready=%t calls=%d tools=%#v inputs=%#v usage=%#v", conversation.assistant, ready.Load(), calls, toolCounts, inputs, usage)
	}
	if calls != 2 || usage == nil || usage["model_calls"] != 2 || usage["total_tokens"] != 330 {
		t.Fatalf("usage must include only the candidate and submit model responses: calls=%d usage=%#v", calls, usage)
	}
	chunkIndex := eventTypeIndex(events, "chunk")
	toolIndex := eventTypeIndex(events, "tool_call")
	if chunkIndex < 0 || toolIndex < 0 || chunkIndex >= toolIndex {
		t.Fatalf("narrative must be emitted before submit: chunk=%d tool=%d events=%#v", chunkIndex, toolIndex, events)
	}
}

func TestInteractiveTurnProtocolRetriesRejectedModulesBeforeReusingCandidate(t *testing.T) {
	ctx := context.Background()
	var ready atomic.Bool
	var submissions atomic.Int32
	var submissionMu sync.Mutex
	patchAttempts := 0
	choicesAccepted := false
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			submissions.Add(1)
			submissionMu.Lock()
			defer submissionMu.Unlock()
			patchStatus := interactive.TurnSubmissionModuleMissing
			if input.StateUpdates != nil {
				patchAttempts++
				patchStatus = interactive.TurnSubmissionModuleRejected
				if patchAttempts > 1 {
					patchStatus = interactive.TurnSubmissionModuleAccepted
				}
			}
			if input.Choices != nil {
				choicesAccepted = true
			}
			choiceStatus := interactive.TurnSubmissionModuleMissing
			if choicesAccepted {
				choiceStatus = interactive.TurnSubmissionModuleAccepted
			}
			settled := patchAttempts > 1 && choicesAccepted
			if settled {
				ready.Store(true)
			}
			retry := []string{}
			if patchStatus != interactive.TurnSubmissionModuleAccepted {
				retry = append(retry, interactive.TurnSubmissionModuleActorStatePatches)
			}
			if choiceStatus != interactive.TurnSubmissionModuleAccepted {
				retry = append(retry, interactive.TurnSubmissionModuleChoices)
			}
			return interactive.TurnSubmissionReceipt{Ready: settled, ModuleStatus: interactive.TurnSubmissionModuleStatus{ActorStatePatches: patchStatus, Choices: choiceStatus}, RetryModules: retry}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chatModel := &interactiveTurnProtocolChatModel{responses: []*schema.Message{
		schema.AssistantMessage("门后传来锁链拖地的声音。", nil),
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "call-patches-1", Function: schema.FunctionCall{Name: interactiveActorStatePatchesToolName, Arguments: `{"patches":[{"op":"replace","path":"/story/当前事件","value":1}]}`}},
			{ID: "call-choices", Function: schema.FunctionCall{Name: interactiveChoicesToolName, Arguments: `{"choices":["进入房间","观察门后","检查锁链","询问同伴","退后戒备"]}`}},
		}),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-submit-2",
			Function: schema.FunctionCall{
				Name:      interactiveActorStatePatchesToolName,
				Arguments: `{"patches":[{"op":"replace","path":"/story/当前事件","value":"石门开启"}]}`,
			},
		}}),
		schema.AssistantMessage("不应再次生成正文。", nil),
	}}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "interactive-protocol-module-retry-test",
		Description:   "test",
		Instruction:   "test",
		Model:         chatModel,
		MaxIterations: 5,
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
	var events []Event
	NewRuntime(DefaultLoopPolicy()).Run(ctx, runner, conversation, nil, ChatRequest{Message: "推开石门"}, RunOptions{
		AgentKind:     AgentKindInteractiveStory,
		RootAgentName: "interactive-protocol-module-retry-test",
	}, func(event Event) { events = append(events, event) })

	calls, _, _ := chatModel.snapshot()
	if calls != 3 || submissions.Load() != 3 || !ready.Load() {
		t.Fatalf("module retry did not settle before completion: calls=%d submissions=%d ready=%t", calls, submissions.Load(), ready.Load())
	}
	if conversation.assistant != "门后传来锁链拖地的声音。" {
		t.Fatalf("module retry must reuse the original candidate: %q", conversation.assistant)
	}
	if countEventType(events, "tool_result") != 3 {
		t.Fatalf("both module receipts must remain visible: %#v", events)
	}
}

func TestInteractiveTurnProtocolLocksFirstCandidateAcrossMalformedModuleAndLaterProse(t *testing.T) {
	ctx := context.Background()
	var ready atomic.Bool
	var mu sync.Mutex
	patchesAccepted := false
	choicesAccepted := false
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			mu.Lock()
			defer mu.Unlock()
			patchRejected := false
			for _, diagnostic := range input.Diagnostics {
				if diagnostic.Module == interactive.TurnSubmissionModuleActorStatePatches {
					patchRejected = true
				}
			}
			if input.StateUpdates != nil {
				patchesAccepted = true
			}
			if input.Choices != nil {
				choicesAccepted = true
			}
			settled := patchesAccepted && choicesAccepted
			if settled {
				ready.Store(true)
			}
			patchStatus := interactive.TurnSubmissionModuleMissing
			switch {
			case patchesAccepted:
				patchStatus = interactive.TurnSubmissionModuleAccepted
			case patchRejected:
				patchStatus = interactive.TurnSubmissionModuleRejected
			}
			choiceStatus := interactive.TurnSubmissionModuleMissing
			if choicesAccepted {
				choiceStatus = interactive.TurnSubmissionModuleAccepted
			}
			return interactive.TurnSubmissionReceipt{Ready: settled, ModuleStatus: interactive.TurnSubmissionModuleStatus{
				ActorStatePatches: patchStatus, Choices: choiceStatus,
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	const candidateA = "乱石坡上，主角藏在巨石后观察二十五丈外的敌人。"
	const laterProseB = "废弃灵木料场里，主角躲在断木桩后，看见十五丈外持破灵镜的瘦高个。"
	chatModel := &interactiveTurnProtocolChatModel{responses: []*schema.Message{
		schema.AssistantMessage(candidateA, nil),
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "bad-patches", Function: schema.FunctionCall{Name: interactiveActorStatePatchesToolName, Arguments: `{"patches":[{"op":"replace","path":"/story/当前事件","value":"以"路过的散修"身份"}]}`}},
			{ID: "good-choices", Function: schema.FunctionCall{Name: interactiveChoicesToolName, Arguments: `{"choices":["继续观察","绕到侧面","悄然后退","制造声响","询问同伴"]}`}},
		}),
		schema.AssistantMessage(laterProseB, nil),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "good-patches", Function: schema.FunctionCall{Name: interactiveActorStatePatchesToolName, Arguments: `{"patches":[{"op":"replace","path":"/story/当前事件","value":"主角在乱石坡观察敌情"}]}`},
		}}),
	}}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name: "interactive-protocol-first-candidate-test", Description: "test", Instruction: "test", Model: chatModel, MaxIterations: 6,
		Handlers:         []adk.ChatModelAgentMiddleware{newInteractiveTurnProtocolMiddleware(ready.Load)},
		ToolsConfig:      adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
		ModelRetryConfig: &adk.ModelRetryConfig{MaxRetries: 1, ShouldRetry: newInteractiveCompletionGuard(ready.Load), BackoffFunc: func(context.Context, int) time.Duration { return time.Nanosecond }},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: true})
	conversation := &interactiveProtocolConversation{ready: &ready}
	NewRuntime(DefaultLoopPolicy()).Run(ctx, runner, conversation, nil, ChatRequest{Message: "观察敌情"}, RunOptions{
		AgentKind: AgentKindInteractiveStory, RootAgentName: "interactive-protocol-first-candidate-test",
	}, func(Event) {})

	calls, _, inputs := chatModel.snapshot()
	if calls != 4 || !ready.Load() || conversation.assistant != candidateA {
		t.Fatalf("first candidate was not preserved: calls=%d ready=%t assistant=%q", calls, ready.Load(), conversation.assistant)
	}
	if len(inputs) < 4 || !messageSliceContains(inputs[3], candidateA) || messageSliceContains(inputs[3], laterProseB) {
		t.Fatalf("final module retry must be grounded in candidate A only: %#v", inputs)
	}
}

func eventTypeIndex(events []Event, eventType string) int {
	for index, event := range events {
		if event.Type == eventType {
			return index
		}
	}
	return -1
}

func countEventType(events []Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
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
	responses   []*schema.Message
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
	if m.calls <= len(m.responses) {
		message = m.responses[m.calls-1]
	} else {
		switch m.calls {
		case 1:
			message = schema.AssistantMessage("门后传来锁链拖地的声音。", nil)
		case 2:
			message = schema.AssistantMessage("", []schema.ToolCall{
				{ID: "call-patches", Function: schema.FunctionCall{Name: interactiveActorStatePatchesToolName, Arguments: `{"patches":[]}`}},
				{ID: "call-choices", Function: schema.FunctionCall{Name: interactiveChoicesToolName, Arguments: `{"choices":["进入房间","观察门后","检查锁链","询问同伴","退后戒备"]}`}},
			})
		default:
			message = schema.AssistantMessage("石门缓缓开启。", nil)
		}
	}
	message.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{
		PromptTokens:     m.calls * 100,
		CompletionTokens: m.calls * 10,
		TotalTokens:      m.calls * 110,
	}}
	return message, nil
}

func newProtocolSubmissionCollector(ready *atomic.Bool) func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
	var mu sync.Mutex
	patchesAccepted := false
	choicesAccepted := false
	return func(_ context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
		mu.Lock()
		defer mu.Unlock()
		if input.StateUpdates != nil && len(input.Diagnostics) == 0 {
			patchesAccepted = true
		}
		if input.Choices != nil && len(input.Diagnostics) == 0 {
			choicesAccepted = true
		}
		settled := patchesAccepted && choicesAccepted
		if settled && ready != nil {
			ready.Store(true)
		}
		status := func(accepted bool) string {
			if accepted {
				return interactive.TurnSubmissionModuleAccepted
			}
			return interactive.TurnSubmissionModuleMissing
		}
		return interactive.TurnSubmissionReceipt{Ready: settled, ModuleStatus: interactive.TurnSubmissionModuleStatus{
			ActorStatePatches: status(patchesAccepted), Choices: status(choicesAccepted),
		}}, nil
	}
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

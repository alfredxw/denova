package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"denova/internal/interactive"
)

func TestInteractiveCompletionGuardRetriesFinalAnswerBeforeTurnSubmission(t *testing.T) {
	guard := newInteractiveCompletionGuard(func() bool { return false })
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{})
	draft := schema.AssistantMessage("门后传来锁链拖地的声音。", nil)
	decision := guard(ctx, &adk.RetryContext{
		RetryAttempt:  1,
		InputMessages: []*schema.Message{schema.UserMessage("推开石门")},
		OutputMessage: draft,
	})

	if decision == nil || !decision.Retry || decision.PersistModifiedInputMessages {
		t.Fatalf("missing submission should retry ephemerally: %#v", decision)
	}
	if len(decision.ModifiedInputMessages) != 3 {
		t.Fatalf("retry context should include input, bounded draft, and feedback: %#v", decision.ModifiedInputMessages)
	}
	feedback := decision.ModifiedInputMessages[len(decision.ModifiedInputMessages)-1]
	if feedback.Role != schema.User || !strings.Contains(feedback.Content, "submit_interactive_turn") || !strings.Contains(feedback.Content, "retry_modules") {
		t.Fatalf("retry feedback does not explain the protocol: %#v", feedback)
	}
	secondDecision := guard(ctx, &adk.RetryContext{
		RetryAttempt:  2,
		InputMessages: decision.ModifiedInputMessages,
		OutputMessage: schema.AssistantMessage("第二版候选。", nil),
	})
	if secondDecision == nil || len(secondDecision.ModifiedInputMessages) != 3 {
		t.Fatalf("ephemeral retry feedback must not accumulate across attempts: %#v", secondDecision)
	}
	if !protocolMessagesContain(secondDecision.ModifiedInputMessages, "门后传来锁链拖地的声音。") || protocolMessagesContain(secondDecision.ModifiedInputMessages, "第二版候选。") {
		t.Fatalf("the first narrative candidate must win across retries: %#v", secondDecision.ModifiedInputMessages)
	}
	wrapped := interactiveRetryErrorForTest{reason: decision.RejectReason}
	if _, ok := interactiveCompletionRetryFromError(wrapped); !ok {
		t.Fatalf("protocol retry reason should survive WillRetryError: %v", wrapped)
	}
}

func protocolMessagesContain(messages []*schema.Message, needle string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func TestInteractiveCompletionGuardAcceptsToolCallsAndSubmittedNarrative(t *testing.T) {
	ready := false
	guard := newInteractiveCompletionGuard(func() bool { return ready })
	toolCall := schema.AssistantMessage("", []schema.ToolCall{{
		ID:       "call-submit",
		Function: schema.FunctionCall{Name: interactiveTurnSubmissionToolName, Arguments: `{}`},
	}})
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: toolCall}); decision != nil && decision.Retry {
		t.Fatalf("tool calls must enter the normal ReAct loop: %#v", decision)
	}
	ready = true
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: schema.AssistantMessage("石门缓缓开启。", nil)}); decision != nil && decision.Retry {
		t.Fatalf("submitted narrative should complete normally: %#v", decision)
	}
}

func TestInteractiveTurnProtocolMiddlewareKeepsStableToolsAndForbidsCallsAfterSubmission(t *testing.T) {
	ready := false
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return ready })
	state := &adk.ChatModelAgentState{ToolInfos: []*schema.ToolInfo{{Name: interactiveTurnSubmissionToolName}}}
	_, state, err := middleware.BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("collecting phase should retain tools: state=%#v err=%v", state, err)
	}
	ready = true
	_, state, err = middleware.BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("submitted phase should keep the stable tool schema: state=%#v err=%v", state, err)
	}

	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Generate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if base.toolChoice == nil || *base.toolChoice != schema.ToolChoiceForbidden {
		t.Fatalf("submitted phase must forbid further tool calls while retaining schemas: %#v", base.toolChoice)
	}
	state.Messages = append(state.Messages, schema.AssistantMessage("", []schema.ToolCall{{
		ID:       "unexpected-call",
		Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`},
	}}))
	if _, _, err := middleware.AfterModelRewriteState(context.Background(), state, &adk.ModelContext{}); err == nil {
		t.Fatal("backend guard must reject a provider that ignores tool_choice=none")
	}
}

func TestInteractiveTurnProtocolAppliesStoryCompletionBudgetOnlyToNarrativeCandidate(t *testing.T) {
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return false }, 1234)
	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	state := &interactiveTurnProtocolRunState{}
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, state)
	if _, err := wrapped.Generate(ctx, nil, model.WithMaxTokens(9999)); err != nil {
		t.Fatal(err)
	}
	if base.maxTokens == nil || *base.maxTokens != 1234 {
		t.Fatalf("first visible narrative should use the story-derived completion budget: %#v", base.maxTokens)
	}
	state.retainNarrativeCandidate("正文候选")
	if _, err := wrapped.Generate(ctx, nil, model.WithMaxTokens(9999)); err != nil {
		t.Fatal(err)
	}
	if base.maxTokens == nil || *base.maxTokens != 9999 {
		t.Fatalf("structured retry must keep the provider/model budget: %#v", base.maxTokens)
	}
}

// TestInteractiveTurnProtocolRetryCapsOpenAICompletionAndDropsReasoning
// guards against WR-04: the retry path must wire B fix's impl-specific
// OpenAI options (max_completion_tokens + low reasoning effort) so a
// Minimax-M3 retry stops re-running a full reasoning pass.
func TestInteractiveTurnProtocolRetryCapsOpenAICompletionAndDropsReasoning(t *testing.T) {
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return false }, 1234)
	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	state := &interactiveTurnProtocolRunState{}
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, state)

	// Narrative phase (candidate not yet ready): the retry caps must NOT be
	// applied; the test passes plain options (no caps) so captureOpenAIOptions
	// records empty impl-specific values.
	if _, err := wrapped.Generate(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if base.maxCompletionTok != nil {
		t.Fatalf("narrative phase must not introduce a completion-token cap: got %v", *base.maxCompletionTok)
	}
	if base.reasoningEffort != "" {
		t.Fatalf("narrative phase must not introduce a reasoning effort: got %q", base.reasoningEffort)
	}

	// Retry phase (candidate ready): even when the caller pre-sets a higher
	// reasoning effort, the protocol must cap completion tokens at the retry
	// budget and force reasoning_effort down to "low" (B fix, Minimax-M3).
	state.retainNarrativeCandidate("正文候选")
	if _, err := wrapped.Generate(ctx, nil, openai.WithReasoningEffort(openai.ReasoningEffortLevelHigh)); err != nil {
		t.Fatal(err)
	}
	if base.maxCompletionTok == nil || *base.maxCompletionTok != interactiveRetryCompletionBudget {
		t.Fatalf("retry must cap completion tokens at %d, got %#v", interactiveRetryCompletionBudget, base.maxCompletionTok)
	}
	if base.reasoningEffort != string(openai.ReasoningEffortLevelLow) {
		t.Fatalf("retry must drop reasoning effort to low, got %q", base.reasoningEffort)
	}
}

type interactiveProtocolOptionModel struct {
	toolChoice       *schema.ToolChoice
	maxTokens        *int
	maxCompletionTok *int
	reasoningEffort  string
}

func (m *interactiveProtocolOptionModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.captureOptions(opts)
	return schema.AssistantMessage("正文", nil), nil
}

func (m *interactiveProtocolOptionModel) Stream(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.captureOptions(opts)
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("正文", nil)}), nil
}

func (m *interactiveProtocolOptionModel) captureOptions(opts []model.Option) {
	common := model.GetCommonOptions(&model.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	m.maxTokens = common.MaxTokens
	maxTok, effort := captureOpenAIOptions(opts)
	m.maxCompletionTok = maxTok
	m.reasoningEffort = effort
}

// openaiCapture mirrors the unexported openaiOptions struct from
// eino-ext/libs/acl/openai so we can read its implSpecificOptFn writes from
// the same memory layout. Field order MUST track the upstream type; if a new
// field is added upstream, append it here to keep the layout in sync.
type openaiCapture struct {
	extraFields                  map[string]any
	reasoningEffort              openai.ReasoningEffortLevel
	extraHeader                  map[string]string
	requestBodyModifier          func([]byte) ([]byte, error)
	requestPayloadModifier       func(ctx context.Context, msg []*schema.Message, rawBody []byte) ([]byte, error)
	responseMessageModifier      func(ctx context.Context, msg *schema.Message, rawBody []byte) (*schema.Message, error)
	responseChunkMessageModifier func(ctx context.Context, msg *schema.Message, rawBody []byte, end bool) (*schema.Message, error)
	maxCompletionTokens          *int
}

// captureOpenAIOptions replays each option's implSpecificOptFn against a
// stand-in openaiOptions layout and returns the resulting budget-relevant
// fields. Reflect+unsafe is unavoidable: the openai package keeps Options
// unexported, so callers cannot construct a typed base for
// model.GetImplSpecificOptions.
func captureOpenAIOptions(opts []model.Option) (*int, string) {
	var cap openaiCapture
	for _, opt := range opts {
		v := reflect.ValueOf(&opt).Elem()
		f := v.FieldByName("implSpecificOptFn")
		if !f.IsValid() || f.IsNil() {
			continue
		}
		f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		fn, ok := f.Interface().(func(unsafe.Pointer))
		if !ok {
			// Fallback: the implSpecificOptFn is a func(*openaiOptions); we
			// know its single argument must be a pointer type, so build a
			// stub that simply records the writes by reflection.
			raw, _ := f.Interface().(any)
			if raw == nil {
				continue
			}
			// Allocate a stand-in of openaiOptions. We'll re-use *openaiCapture.
			capturePtr := unsafe.Pointer(&cap)
			reflectFn := reflect.ValueOf(raw)
			// reflectFn.Call needs a Value matching the parameter type of fn.
			argType := reflectFn.Type().In(0)
			// argType is *openaiOptions (unexported). Use unsafe cast: alloc
			// raw bytes via reflect.NewAt with argType and copy from cap.
			ptr := reflect.NewAt(argType.Elem(), capturePtr)
			reflectFn.Call([]reflect.Value{ptr})
		} else {
			fn(unsafe.Pointer(&cap))
		}
	}
	return cap.maxCompletionTokens, string(cap.reasoningEffort)
}

type interactiveRetryErrorForTest struct {
	reason any
}

func (e interactiveRetryErrorForTest) Error() string {
	return "stream rejected"
}

func (e interactiveRetryErrorForTest) RejectReason() any {
	return e.reason
}

func TestInteractiveRetryFeedbackFromReceiptTargetsExactErrors(t *testing.T) {
	receipt := interactive.TurnSubmissionReceipt{
		Ready:        false,
		RetryModules: []string{"choices"},
		Diagnostics: []interactive.TurnSubmissionDiagnostic{{
			Module:    interactive.TurnSubmissionModuleStateChanges,
			Code:      "state_value_invalid",
			Path:      "/state_changes/2",
			Expected:  "number",
			Actual:    "string",
			Retryable: true,
			MessageZH: "字段类型不匹配",
			MessageEN: "type mismatch",
		}},
	}
	feedback := interactiveRetryFeedbackFromReceipt(receipt)
	for _, want := range []string{"choices", "state_changes", "/state_changes/2", "number", "string", "字段类型不匹配"} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("targeted retry feedback missing %q: %s", want, feedback)
		}
	}
}

func TestInteractiveRetryFeedbackFromReceiptEmptyYieldsNothing(t *testing.T) {
	if feedback := interactiveRetryFeedbackFromReceipt(interactive.TurnSubmissionReceipt{}); feedback != "" {
		t.Fatalf("receipt without diagnostics should yield no targeted feedback: %s", feedback)
	}
}

// submitAssistantTurn returns an assistant message that invoked
// submit_interactive_turn with the given tool call id.
func submitAssistantTurn(id string) *schema.Message {
	return schema.AssistantMessage("", []schema.ToolCall{{
		ID:       id,
		Function: schema.FunctionCall{Name: interactiveTurnSubmissionToolName, Arguments: `{}`},
	}})
}

// toolResultForCall returns a schema.Tool message whose ToolCallID matches
// the assistant's invocation. content is the JSON-encoded receipt.
func toolResultForCall(id, content string) *schema.Message {
	return schema.ToolMessage(content, id)
}

// TestLastInteractiveSubmitReceiptReturnsMostRecentCorrectlyPairedReceipt
// guards against WR-03: a history with multiple submit_interactive_turn
// invocations must return the most-recent receipt whose ToolCallID actually
// matches the corresponding assistant tool call, ignoring successful older
// submissions and any receipt that landed under a wrong ID.
func TestLastInteractiveSubmitReceiptReturnsMostRecentCorrectlyPairedReceipt(t *testing.T) {
	// Most recent assistant turn: submit with ID "call-second".
	second := submitAssistantTurn("call-second")
	// Earlier assistant turn: submit with ID "call-first".
	first := submitAssistantTurn("call-first")
	// Interfering receipt: same content as a correctly-paired one but
	// addressed to a tool call id that does NOT exist in the assistant
	// turn above. The parser must ignore it.
	decoy := toolResultForCall("call-NOPE", mustMarshalReceipt(interactive.TurnSubmissionReceipt{
		Ready:        true,
		RetryModules: []string{"decoy"},
	}))
	// Earlier successful receipt (well-paired to first).
	earlyReady := toolResultForCall("call-first", mustMarshalReceipt(interactive.TurnSubmissionReceipt{
		Ready: true,
	}))
	// Most recent rejected receipt, correctly paired to second.
	rejected := toolResultForCall("call-second", mustMarshalReceipt(interactive.TurnSubmissionReceipt{
		Ready:        false,
		RetryModules: []string{"choices"},
		Diagnostics: []interactive.TurnSubmissionDiagnostic{{
			Module:    interactive.TurnSubmissionModuleStateChanges,
			Code:      "state_value_invalid",
			Path:      "/state_changes/2",
			Expected:  "number",
			Actual:    "string",
			MessageZH: "字段类型不匹配",
		}},
	}))

	messages := []*schema.Message{
		schema.UserMessage("请提供正文候选并提交两个模块"),
		first,
		earlyReady,
		schema.UserMessage("已接受 state_changes，请继续 choices。"),
		decoy, // interfering tool result with a wrong call id
		schema.AssistantMessage("继续 choices。", nil),
		second,
		rejected,
	}

	receipt, ok := lastInteractiveSubmitReceipt(messages)
	if !ok {
		t.Fatalf("expected the most-recent correctly-paired receipt to surface")
	}
	if receipt.Ready {
		t.Fatalf("most recent receipt should be the rejection, got %#v", receipt)
	}
	if len(receipt.RetryModules) != 1 || receipt.RetryModules[0] != "choices" {
		t.Fatalf("most recent receipt should expose the choices module, got %#v", receipt.RetryModules)
	}
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Path != "/state_changes/2" {
		t.Fatalf("most recent receipt diagnostics should describe /state_changes/2, got %#v", receipt.Diagnostics)
	}
}

func TestLastInteractiveSubmitReceiptReturnsFalseWhenNoHistoryHasSubmission(t *testing.T) {
	messages := []*schema.Message{
		schema.UserMessage("你好"),
		schema.AssistantMessage("我也好", nil),
	}
	if _, ok := lastInteractiveSubmitReceipt(messages); ok {
		t.Fatal("no submit_interactive_turn in history should yield no receipt")
	}
}

func TestLastInteractiveSubmitReceiptReturnsFalseOnInvalidReceiptJSON(t *testing.T) {
	// Receipt with invalid JSON: parser must return false so the guard can
	// fall back to generic feedback rather than feeding garbage back to the
	// model.
	messages := []*schema.Message{
		submitAssistantTurn("call-only"),
		toolResultForCall("call-only", "{this is not json"),
	}
	if _, ok := lastInteractiveSubmitReceipt(messages); ok {
		t.Fatal("invalid receipt JSON must surface as no receipt")
	}
}

func mustMarshalReceipt(receipt interactive.TurnSubmissionReceipt) string {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestInteractiveCompletionGuardFailsFastAfterRetryBudget(t *testing.T) {
	guard := newInteractiveCompletionGuard(func() bool { return false })
	state := &interactiveTurnProtocolRunState{}
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, state)
	input := []*schema.Message{schema.UserMessage("继续剧情")}
	output := schema.AssistantMessage("又一版正文候选。", nil)

	for attempt := 1; attempt <= interactiveCompletionGuardMaxRetries; attempt++ {
		decision := guard(ctx, &adk.RetryContext{RetryAttempt: attempt, InputMessages: input, OutputMessage: output})
		if decision == nil || !decision.Retry {
			t.Fatalf("attempt %d within the guard budget should retry: %#v", attempt, decision)
		}
	}

	decision := guard(ctx, &adk.RetryContext{RetryAttempt: interactiveCompletionGuardMaxRetries + 1, InputMessages: input, OutputMessage: output})
	if decision == nil || decision.Retry {
		t.Fatalf("exceeding the guard budget must stop retrying: %#v", decision)
	}
	if !errors.Is(decision.RewriteError, ErrInteractiveCompletionRetriesExceeded) {
		t.Fatalf("exceeding the guard budget must surface the typed error: %#v", decision.RewriteError)
	}
}

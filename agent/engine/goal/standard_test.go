package goal_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alfredxw/denova/agent"
	agentgoal "github.com/alfredxw/denova/agent/engine/goal"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentstream "github.com/alfredxw/denova/agent/model/stream"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type recordedGoalModelCall struct {
	messages  []*agentschema.Message
	options   *agentmodel.Options
	streaming bool
}

type goalModel struct {
	mu        sync.Mutex
	responses []*agentschema.Message
	calls     []recordedGoalModelCall
	onCall    func(int) error
}

func (model *goalModel) Generate(_ context.Context, input []*agentschema.Message, options ...agentmodel.ModelOption) (*agentschema.Message, error) {
	return model.next(input, options, false)
}

func (model *goalModel) Stream(_ context.Context, input []*agentschema.Message, options ...agentmodel.ModelOption) (*agentstream.StreamReader[*agentschema.Message], error) {
	message, err := model.next(input, options, true)
	if err != nil {
		return nil, err
	}
	return agentstream.StreamReaderFromArray([]*agentschema.Message{message}), nil
}

func (model *goalModel) next(input []*agentschema.Message, options []agentmodel.ModelOption, streaming bool) (*agentschema.Message, error) {
	model.mu.Lock()
	index := len(model.calls)
	model.calls = append(model.calls, recordedGoalModelCall{
		messages: cloneGoalTestMessages(input), options: agentmodel.GetCommonOptions(&agentmodel.Options{}, options...), streaming: streaming,
	})
	if len(model.responses) == 0 {
		model.mu.Unlock()
		return nil, errors.New("Goal model exhausted")
	}
	response := model.responses[0].Clone()
	model.responses = model.responses[1:]
	onCall := model.onCall
	model.mu.Unlock()
	if onCall != nil {
		if err := onCall(index); err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (model *goalModel) recordedCalls() []recordedGoalModelCall {
	model.mu.Lock()
	defer model.mu.Unlock()
	result := make([]recordedGoalModelCall, len(model.calls))
	copy(result, model.calls)
	return result
}

func cloneGoalTestMessages(messages []*agentschema.Message) []*agentschema.Message {
	cloned := make([]*agentschema.Message, len(messages))
	for index, message := range messages {
		cloned[index] = message.Clone()
	}
	return cloned
}

type fixedGoalModelOptionMiddleware struct{ agentmiddleware.BaseMiddleware }

func (*fixedGoalModelOptionMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agentmodel.ModelCall,
	_ *agentmiddleware.ModelContext,
) (context.Context, *agentmodel.ModelCall, error) {
	call.Options = append(call.Options,
		agentmodel.WithTools([]*agentschema.ToolInfo{{Name: "lookup", Desc: "Lookup evidence"}}),
		agentmodel.WithMaxTokens(777),
		agentmodel.WithSessionKey("goal-cache-session"),
	)
	return ctx, call, nil
}

func TestStandardGoalEvaluatorForksExactFinalRequestAndCompletes(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	model := &goalModel{responses: []*agentschema.Message{
		agentschema.AssistantMessage("实现和验证都已完成。", nil),
		agentschema.AssistantMessage(`{"verdict":"complete","reason":"目标已经完整实现并通过验证。","next_instruction":""}`, nil),
	}}
	owner, session := newGoalTestSession(t, "goal-complete", model, agentgoal.Standard(agentgoal.WithClock(func() time.Time { return now })),
		agentmiddleware.IdentifyMiddleware(&fixedGoalModelOptionMiddleware{}, agentschema.CapabilityIdentity{Kind: "test.goal.options", Version: 1}),
	)
	defer func() { _ = owner.Close(context.Background()) }()
	created := setGoalForTest(t, session, "完整实现目标并验证结果")

	run, err := session.Run(context.Background(), agent.Text("开始执行"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agentschema.ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil || !present {
		t.Fatalf("Goal=%#v present=%t error=%v", current, present, err)
	}
	if current.Status != agentgoal.GoalCompleted || current.Revision != created.Revision+1 || current.Report != "目标已经完整实现并通过验证。" {
		t.Fatalf("completed Goal=%#v", current)
	}

	calls := model.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("model calls=%d, want primary plus evaluator", len(calls))
	}
	primary, evaluator := calls[0], calls[1]
	if !primary.streaming || evaluator.streaming {
		t.Fatalf("streaming modes primary=%t evaluator=%t", primary.streaming, evaluator.streaming)
	}
	if primary.options.MaxTokens == nil || *primary.options.MaxTokens != 777 || len(primary.options.Tools) != 1 {
		t.Fatalf("middleware options were not captured: %#v", primary.options)
	}
	if evaluator.options.MaxTokens == nil || *evaluator.options.MaxTokens != 512 ||
		!reflect.DeepEqual(evaluator.options.Tools, primary.options.Tools) ||
		evaluator.options.SessionKey != primary.options.SessionKey ||
		evaluator.options.CachePrefixMessages != primary.options.CachePrefixMessages {
		t.Fatalf("evaluator did not preserve primary tools and cache routing with bounded output: %#v", evaluator.options)
	}
	if len(evaluator.messages) != len(primary.messages)+2 ||
		!reflect.DeepEqual(evaluator.messages[:len(primary.messages)], primary.messages) {
		t.Fatalf("evaluator did not preserve the exact primary request prefix")
	}
	final := evaluator.messages[len(evaluator.messages)-2]
	prompt := evaluator.messages[len(evaluator.messages)-1]
	if final.Role != agentschema.Assistant || final.Content != "实现和验证都已完成。" {
		t.Fatalf("evaluator final evidence=%#v", final)
	}
	if prompt.Role != agentschema.User || prompt.Content == "" {
		t.Fatalf("evaluator suffix prompt=%#v", prompt)
	}
	if !strings.Contains(prompt.Content, "same language as the active objective") {
		t.Fatal("evaluator prompt does not preserve the active objective language")
	}
	if strings.Contains(prompt.Content, "English") {
		t.Fatal("evaluator prompt must not force English rationale or continuation text")
	}
}

func TestStandardGoalEvaluatorContinuesUntilComplete(t *testing.T) {
	model := &goalModel{responses: []*agentschema.Message{
		agentschema.AssistantMessage("第一步已经完成。", nil),
		agentschema.AssistantMessage(`{"verdict":"continue","reason":"仍有实现和验证工作。","next_instruction":"完成剩余实现并运行验证。"}`, nil),
		agentschema.AssistantMessage("剩余实现和验证已经完成。", nil),
		agentschema.AssistantMessage(`{"verdict":"complete","reason":"全部要求均已验证。","next_instruction":""}`, nil),
	}}
	owner, session := newGoalTestSession(t, "goal-continue", model, agentgoal.Standard())
	defer func() { _ = owner.Close(context.Background()) }()
	created := setGoalForTest(t, session, "完成全部实现和验证")

	run, err := session.Run(context.Background(), agent.Text("先完成第一步"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agentschema.ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil || !present || current.Status != agentgoal.GoalCompleted || current.Revision != created.Revision+1 {
		t.Fatalf("Goal=%#v present=%t error=%v", current, present, err)
	}
	calls := model.recordedCalls()
	if len(calls) != 4 {
		t.Fatalf("model calls=%d, want two primary/evaluator pairs", len(calls))
	}
	if !goalTestMessagesContain(calls[2].messages, "完成剩余实现并运行验证。") {
		t.Fatalf("autonomous continuation was not projected into the next main call: %#v", calls[2].messages)
	}
	if goalTestMessagesContain(calls[2].messages, calls[1].messages[len(calls[1].messages)-1].Content) {
		t.Fatal("evaluator output leaked into the main transcript")
	}
}

func TestStandardGoalEvaluatorBlocksAndPersistsLocalizedReason(t *testing.T) {
	model := &goalModel{responses: []*agentschema.Message{
		agentschema.AssistantMessage("需要用户提供缺失的密钥。", nil),
		agentschema.AssistantMessage(`{"verdict":"blocked","reason":"缺少完成目标所必需的外部密钥。","next_instruction":""}`, nil),
	}}
	owner, session := newGoalTestSession(t, "goal-blocked", model, agentgoal.Standard())
	defer func() { _ = owner.Close(context.Background()) }()
	created := setGoalForTest(t, session, "使用外部密钥完成部署")

	run, err := session.Run(context.Background(), agent.Text("继续部署"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agentschema.ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil || !present || current.Status != agentgoal.GoalBlocked || current.Revision != created.Revision+1 ||
		current.Report != "缺少完成目标所必需的外部密钥。" {
		t.Fatalf("Goal=%#v present=%t error=%v", current, present, err)
	}
}

func TestStandardGoalEvaluatorFailureStopsWithoutFalseCompletion(t *testing.T) {
	tests := []struct {
		name       string
		evaluation *agentschema.Message
	}{
		{name: "malformed", evaluation: agentschema.AssistantMessage("not-json", nil)},
		{name: "tool_call", evaluation: agentschema.AssistantMessage(`{"verdict":"complete","reason":"invalid","next_instruction":""}`, []agentschema.ToolCall{{
			ID: "denied", Type: "function", Function: agentschema.FunctionCall{Name: "unexpected", Arguments: `{}`},
		}})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &goalModel{responses: []*agentschema.Message{
				agentschema.AssistantMessage("主回合仍应正常完成。", nil), test.evaluation,
			}}
			owner, session := newGoalTestSession(t, "goal-evaluation-failure-"+test.name, model, agentgoal.Standard())
			defer func() { _ = owner.Close(context.Background()) }()
			created := setGoalForTest(t, session, "不能被误判完成")
			observationContext, cancelObservation := context.WithCancel(context.Background())
			observation, err := session.Observe(observationContext, 0)
			if err != nil {
				cancelObservation()
				t.Fatal(err)
			}

			run, err := session.Run(context.Background(), agent.Text("执行一次"))
			if err != nil {
				t.Fatal(err)
			}
			if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agentschema.ResultCompleted {
				t.Fatalf("result=%#v error=%v", result, waitErr)
			}
			current, present, err := session.Goal(context.Background())
			if err != nil || !present || current.Status != agentgoal.GoalActive || current.Revision != created.Revision {
				t.Fatalf("Goal=%#v present=%t error=%v", current, present, err)
			}
			if calls := model.recordedCalls(); len(calls) != 2 {
				t.Fatalf("model calls=%d, evaluator failure must stop autonomous continuation", len(calls))
			}
			cancelObservation()
			var failure *agentevent.GoalEvaluationFailed
			for event := range observation.Events {
				if payload, ok := event.Payload.(agentevent.GoalEvaluationFailed); ok {
					copy := payload
					failure = &copy
				}
			}
			if failure == nil || failure.GoalID != created.ID || failure.GoalRevision != created.Revision || failure.Code != "agent_runtime.goal_evaluation_failed" || strings.TrimSpace(failure.Detail) == "" {
				t.Fatalf("Goal evaluation failure event = %#v", failure)
			}
		})
	}
}

func TestStandardGoalDiscardsStaleEvaluation(t *testing.T) {
	model := &goalModel{responses: []*agentschema.Message{
		agentschema.AssistantMessage("旧目标似乎已经完成。", nil),
		agentschema.AssistantMessage(`{"verdict":"complete","reason":"旧快照认为已完成。","next_instruction":""}`, nil),
	}}
	owner, session := newGoalTestSession(t, "goal-stale", model, agentgoal.Standard())
	defer func() { _ = owner.Close(context.Background()) }()
	created := setGoalForTest(t, session, "在评估期间可能被暂停")
	model.onCall = func(index int) error {
		if index != 1 {
			return nil
		}
		_, err := session.UpdateGoal(context.Background(), agentschema.GoalMutation{
			Kind: agentschema.GoalPause, ExpectedID: created.ID, ExpectedRevision: created.Revision,
		})
		return err
	}

	run, err := session.Run(context.Background(), agent.Text("执行旧快照"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agentschema.ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil || !present || current.Status != agentgoal.GoalPaused || current.Revision != created.Revision+1 {
		t.Fatalf("stale evaluator overwrote Goal=%#v present=%t error=%v", current, present, err)
	}
}

func TestStandardGoalUsesRevisionAndMutationIdempotencyFences(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	manager := agentgoal.Standard(agentgoal.WithClock(func() time.Time { return now }))
	created, err := manager.Apply(context.Background(), agentgoal.GoalApplyRequest{Mutation: agentschema.GoalMutation{
		Kind: agentschema.GoalSet, Objective: "ship", MutationID: "mutation-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := manager.Apply(context.Background(), agentgoal.GoalApplyRequest{
		Present: true, Current: created,
		Mutation: agentschema.GoalMutation{Kind: agentschema.GoalSet, Objective: "ignored replay", MutationID: "mutation-1"},
	})
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("idempotent replay=%#v err=%v", replayed, err)
	}
	_, err = manager.Apply(context.Background(), agentgoal.GoalApplyRequest{
		Present: true, Current: created,
		Mutation: agentschema.GoalMutation{Kind: agentschema.GoalPause, ExpectedRevision: created.Revision + 1, MutationID: "mutation-2"},
	})
	if !errors.Is(err, agentgoal.ErrRevisionConflict) {
		t.Fatalf("stale mutation error=%v", err)
	}
}

func TestStandardGoalHostMayUseCompleteStateMachine(t *testing.T) {
	owner, session := newGoalTestSession(t, "goal-host-state-machine", &goalModel{}, agentgoal.Standard())
	defer func() { _ = owner.Close(context.Background()) }()
	apply := func(mutation agentschema.GoalMutation) agentgoal.GoalState {
		t.Helper()
		state, err := session.UpdateGoal(context.Background(), mutation)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	state := apply(agentschema.GoalMutation{Kind: agentschema.GoalSet, Objective: "first objective"})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalPause, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalResume, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalComplete, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalClear, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalSet, Objective: "second objective"})
	state = apply(agentschema.GoalMutation{Kind: agentschema.GoalBlock, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	if state.Status != agentgoal.GoalBlocked || state.Objective != "second objective" {
		t.Fatalf("final Goal state=%#v", state)
	}
}

func TestStandardGoalPreparationIsActiveOnlyEscapedAndToolFree(t *testing.T) {
	manager := agentgoal.Standard()
	for _, test := range []struct {
		name    string
		present bool
		status  agentgoal.GoalStatus
	}{
		{name: "absent"},
		{name: "paused", present: true, status: agentgoal.GoalPaused},
		{name: "completed", present: true, status: agentgoal.GoalCompleted},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := manager.Prepare(context.Background(), agentgoal.GoalPrepareRequest{
				Present: test.present, State: agentgoal.GoalState{Status: test.status},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(prepared.Tools) != 0 || len(prepared.Context) != 0 || prepared.ReservedTokens != 0 {
				t.Fatalf("inactive Goal preparation=%#v", prepared)
			}
		})
	}
	prepared, err := manager.Prepare(context.Background(), agentgoal.GoalPrepareRequest{
		Present: true,
		State: agentgoal.GoalState{
			ID: `goal-<unsafe>`, Objective: `Ship <complete> & "verified"`,
			Status: agentgoal.GoalActive, Revision: 7,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Tools) != 0 || len(prepared.Context) != 1 || prepared.ReservedTokens < 512 {
		t.Fatalf("active Goal preparation=%#v", prepared)
	}
	content := prepared.Context[0].Content
	for _, required := range []string{
		`goal-&lt;unsafe&gt;`, `Ship &lt;complete&gt; &amp; &#34;verified&#34;`,
		"entire objective", "intermediate milestone", "user input or an external state change",
		"evaluated by the runtime",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("active Goal protocol missing %q: %s", required, content)
		}
	}
	if strings.Contains(content, `Ship <complete>`) {
		t.Fatalf("active Goal objective was not escaped: %s", content)
	}
}

func newGoalTestSession(
	t *testing.T,
	name string,
	model *goalModel,
	manager agentgoal.GoalManager,
	middlewares ...agentmiddleware.Middleware,
) (*agent.Agent, *agent.Session) {
	t.Helper()
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: model, Goal: manager, Middlewares: middlewares,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), agentsession.Named(name))
	if err != nil {
		_ = owner.Close(context.Background())
		t.Fatal(err)
	}
	return owner, session
}

func setGoalForTest(t *testing.T, session *agent.Session, objective string) agentgoal.GoalState {
	t.Helper()
	created, err := session.UpdateGoal(context.Background(), agentschema.GoalMutation{
		Kind: agentschema.GoalSet, Objective: objective,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func goalTestMessagesContain(messages []*agentschema.Message, value string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, value) {
			return true
		}
	}
	return false
}

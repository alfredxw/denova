package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestDefinitionSourceObservesExactTurnReasonAndAcceptedIdentity(t *testing.T) {
	tests := []struct {
		name     string
		delivery runstate.DeliveryKind
		recovery bool
		want     TurnReason
	}{
		{name: "start", delivery: runstate.DeliveryStart, want: TurnReasonStart},
		{name: "steer", delivery: runstate.DeliverySteer, want: TurnReasonSteer},
		{name: "follow-up", delivery: runstate.DeliveryFollowUp, want: TurnReasonFollowUp},
		{name: "next-turn", delivery: runstate.DeliveryNextTurn, want: TurnReasonNextTurn},
		{name: "recovery", delivery: runstate.DeliveryStart, recovery: true, want: TurnReasonRecovery},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
			var observed PrepareRequest
			source := SourceFunc(func(_ context.Context, request PrepareRequest) (Definition, error) {
				observed = request
				return Definition{Key: "reason-test", Model: model}, nil
			})
			_, runtimeInput, err := encodeInput(Input{Text: "accepted"})
			if err != nil {
				t.Fatal(err)
			}
			engine := &definitionEngine{
				source: source, key: NamedSession("reason-" + test.name), cacheKeys: defaultCacheKey,
			}
			result, err := engine.Run(context.Background(), runstate.EngineRequest{
				Snapshot: runstate.TurnSnapshot{
					CommandID: "accepted-command", OperationID: "accepted-run", Cycle: 3,
					Delivery: test.delivery, Input: runtimeInput,
				},
				Recovery: test.recovery,
			}, func(runstate.EngineEvent) error { return nil })
			if err != nil || result.Status != runstate.EngineCompleted {
				t.Fatalf("result=%#v error=%v", result, err)
			}
			if observed.Reason != test.want || observed.Run.ID != "accepted-run" ||
				observed.Run.CommandID != "accepted-command" || observed.Run.Cycle != 3 ||
				observed.Run.Delivery != TurnDelivery(test.delivery) {
				t.Fatalf("PrepareRequest = %#v, want reason=%q", observed, test.want)
			}
		})
	}
}

func TestIndependentRunsMayChangeDefinitionIdentity(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first", nil), AssistantMessage("second", nil),
	}}
	var calls atomic.Int32
	source := SourceFunc(func(context.Context, PrepareRequest) (Definition, error) {
		version := calls.Add(1)
		return Definition{
			Key: "dynamic-definition", Model: model,
			ModelIdentity: CapabilityIdentity{Kind: "model.dynamic", Version: 1, ConfigHash: string(rune('0' + version))},
			Instructions:  string(rune('0' + version)),
		}, nil
	})
	owner, err := New(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("dynamic-definition"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two"} {
		run, err := session.Run(context.Background(), Text(text))
		if err != nil {
			t.Fatal(err)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("run %q result=%#v error=%v", text, result, waitErr)
		}
	}
}

type mutableFinalContext struct {
	mu      sync.RWMutex
	content string
}

func (*mutableFinalContext) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.dynamic-final", Version: 1}
}

func (source *mutableFinalContext) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	source.mu.RLock()
	content := source.content
	source.mu.RUnlock()
	return []ContextFragment{{
		Source: "test.dynamic", Purpose: "same-cycle recovery fence", Resource: "dynamic-final",
		Revision: "stable-identity", Placement: ContextFinalUserMessage, Content: content, HardLimit: 64 << 10,
	}}, nil
}

func (source *mutableFinalContext) set(value string) {
	source.mu.Lock()
	source.content = value
	source.mu.Unlock()
}

func TestInteractionRecoveryRejectsSameIdentityMaterializedContextDrift(t *testing.T) {
	contextSource := &mutableFinalContext{content: "original final-user context"}
	tool, err := InferTool("mutate", "mutate", func(context.Context, struct{}) (string, error) { return "ok", nil })
	if err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{ID: "mutate-call", Type: "function", Function: FunctionCall{Name: "mutate", Arguments: `{}`}}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Model: model, Context: contextSource,
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.dynamic-context", Version: 1}, ToolDefinition{
			Tool: tool, Descriptor: ToolDescriptor{
				Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
				MutationScope: ToolMutationWorkspace, PostCheck: ToolPostCheckWorkspaceChange,
				Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
				ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
			},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("change it"))
	if err != nil {
		t.Fatal(err)
	}
	var interaction InteractionRequested
	for event := range run.Events() {
		if value, ok := event.Payload.(InteractionRequested); ok {
			interaction = value
			break
		}
	}
	contextSource.set("changed final-user context")
	if err := run.Respond(context.Background(), interaction.Request.ID, InteractionResponse{Permission: PermissionAllowOnce}); !errors.Is(err, ErrDefinitionMismatch) {
		t.Fatalf("drift response error = %v, want DefinitionMismatch", err)
	}
	contextSource.set("original final-user context")
	if err := run.Respond(context.Background(), interaction.Request.ID, InteractionResponse{Permission: PermissionAllowOnce}); err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("stable recovery result=%#v error=%v", result, waitErr)
	}
	calls := model.calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want initial and recovered calls", len(calls))
	}
	for _, call := range calls {
		found := false
		for _, message := range call {
			if message != nil && message.Role == User && strings.Contains(message.Content, "original final-user context") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("interaction recovery lost exact model-only user projection: %#v", call)
		}
	}
}

type mutableGoalPreparation struct {
	admissionGoalManager
	mu      sync.RWMutex
	variant string
}

func (*mutableGoalPreparation) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "goal.dynamic-preparation", Version: 1}
}

func (manager *mutableGoalPreparation) set(variant string) {
	manager.mu.Lock()
	manager.variant = variant
	manager.mu.Unlock()
}

func (manager *mutableGoalPreparation) Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error) {
	manager.mu.RLock()
	variant := manager.variant
	manager.mu.RUnlock()
	tool, err := InferTool("goal_action", "goal action "+variant, func(context.Context, struct{}) (string, error) {
		return "ok", nil
	})
	if err != nil {
		return GoalPreparation{}, err
	}
	return GoalPreparation{
		Context: []ContextFragment{{
			Source: "test.goal", Purpose: "dynamic Goal recovery fence", Resource: "goal-context",
			Revision: "stable-identity", Placement: ContextLeadingMessage, Content: "goal context " + variant,
			HardLimit: 64 << 10,
		}},
		Tools: []ToolDefinition{{
			Tool: tool, Descriptor: ToolDescriptor{
				Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
				MutationScope: ToolMutationWorkspace, PostCheck: ToolPostCheckWorkspaceChange,
				Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
				ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
			},
		}},
	}, nil
}

func TestInteractionRecoveryRejectsSameIdentityGoalToolAndContextDrift(t *testing.T) {
	manager := &mutableGoalPreparation{variant: "original"}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{ID: "goal-call", Type: "function", Function: FunctionCall{Name: "goal_action", Arguments: `{}`}}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{Model: model, Goal: manager})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("continue the goal"))
	if err != nil {
		t.Fatal(err)
	}
	var interaction InteractionRequested
	for event := range run.Events() {
		if value, ok := event.Payload.(InteractionRequested); ok {
			interaction = value
			break
		}
	}
	manager.set("changed")
	if err := run.Respond(context.Background(), interaction.Request.ID, InteractionResponse{Permission: PermissionAllowOnce}); !errors.Is(err, ErrDefinitionMismatch) {
		t.Fatalf("drift response error = %v, want DefinitionMismatch", err)
	}
	manager.set("original")
	if err := run.Respond(context.Background(), interaction.Request.ID, InteractionResponse{Permission: PermissionAllowOnce}); err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("stable Goal recovery result=%#v error=%v", result, waitErr)
	}
}

type fixedPermissionPolicy struct{ decision PermissionDecisionKind }

func (policy fixedPermissionPolicy) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "permission.fixed-test", Version: 1, ConfigHash: string(policy.decision)}
}
func (policy fixedPermissionPolicy) Evaluate(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionDecision{Kind: policy.decision}, nil
}
func (fixedPermissionPolicy) Resolve(context.Context, PermissionResolveRequest) (PermissionResolvedDecision, error) {
	return PermissionResolvedDecision{}, errors.New("unexpected permission resolution")
}

type invokedToolMiddleware struct {
	BaseMiddleware
	invoked atomic.Bool
	rewrite string
}

func (middleware *invokedToolMiddleware) WrapToolCall(
	_ context.Context, endpoint ToolCallEndpoint, _ *ToolContext,
) (ToolCallEndpoint, error) {
	return func(ctx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
		middleware.invoked.Store(true)
		if middleware.rewrite != "" {
			arguments = middleware.rewrite
		}
		return endpoint(ctx, arguments, options...)
	}, nil
}

func TestPermissionFencePrecedesCallerMiddlewareAndFreezesArguments(t *testing.T) {
	tests := []struct {
		name       string
		decision   PermissionDecisionKind
		rewrite    string
		wantInvoke bool
	}{
		{name: "deny-before-middleware", decision: PermissionBlock},
		{name: "reject-rewritten-arguments", decision: PermissionAllow, rewrite: `{"value":2}`, wantInvoke: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var executed atomic.Bool
			tool, err := InferTool("mutate", "mutate", func(_ context.Context, input struct {
				Value int `json:"value"`
			}) (string, error) {
				executed.Store(true)
				return "ok", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			middleware := &invokedToolMiddleware{rewrite: test.rewrite}
			model := &lifecycleModel{responses: []*Message{
				AssistantMessage("", []ToolCall{{ID: "mutate-call", Type: "function", Function: FunctionCall{Name: "mutate", Arguments: `{"value":1}`}}}),
				AssistantMessage("done", nil),
			}}
			owner, err := New(context.Background(), Definition{
				Model: model, Permission: fixedPermissionPolicy{decision: test.decision},
				Middlewares: []Middleware{middleware},
				Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.permission-fence", Version: 1}, ToolDefinition{
					Tool: tool, Descriptor: ToolDescriptor{
						Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
						MutationScope: ToolMutationWorkspace, PostCheck: ToolPostCheckWorkspaceChange,
						Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
						ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
					},
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			run, err := owner.Run(context.Background(), Text("change it"))
			if err != nil {
				t.Fatal(err)
			}
			if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
				t.Fatalf("result=%#v error=%v", result, waitErr)
			}
			if middleware.invoked.Load() != test.wantInvoke || executed.Load() {
				t.Fatalf("middleware invoked=%t, concrete executed=%t", middleware.invoked.Load(), executed.Load())
			}
			for event := range run.Events() {
				if _, started := event.Payload.(ToolStarted); started {
					t.Fatal("ToolStarted was published before the fixed permission/argument fence")
				}
			}
		})
	}
}

type admissionGoalManager struct{}

func (admissionGoalManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "goal.admission-test", Version: 1}
}

func (admissionGoalManager) Apply(_ context.Context, request GoalApplyRequest) (GoalState, error) {
	if request.Present && request.Current.LastMutationID == request.Mutation.MutationID {
		return request.Current, nil
	}
	if request.Mutation.Kind != GoalSet {
		if request.Mutation.ExpectedRevision != request.Current.Revision {
			return GoalState{}, errors.New("stale Goal revision")
		}
		return GoalState{}, errors.New("admission test supports only Goal set")
	}
	if request.Mutation.ExpectedRevision != 0 && request.Mutation.ExpectedRevision != request.Current.Revision {
		return GoalState{}, errors.New("stale Goal revision")
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	return GoalState{
		ID: "admission-goal", Objective: request.Mutation.Objective, Status: GoalActive,
		Revision: request.Current.Revision + 1, CreatedAt: now, UpdatedAt: now,
		ActiveSince: &now, LastMutationID: request.Mutation.MutationID,
	}, nil
}

func (admissionGoalManager) Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error) {
	return GoalPreparation{}, nil
}

func (admissionGoalManager) AfterRun(context.Context, GoalAfterRunRequest) (GoalContinuation, error) {
	return GoalContinuation{}, nil
}

func TestGoalMutationIsAppliedAtomicallyAtEveryInputAdmission(t *testing.T) {
	tests := []struct {
		name  string
		admit func(context.Context, *Run, Input) (*Run, error)
	}{
		{
			name: "steer",
			admit: func(ctx context.Context, run *Run, input Input) (*Run, error) {
				_, err := run.Steer(ctx, input)
				return run, err
			},
		},
		{
			name: "follow-up",
			admit: func(ctx context.Context, run *Run, input Input) (*Run, error) {
				_, err := run.Queue(ctx, input)
				return run, err
			},
		},
		{name: "next-turn", admit: func(ctx context.Context, run *Run, input Input) (*Run, error) {
			return run.FollowUp(ctx, input)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newGatedLifecycleModel()
			owner, err := New(context.Background(), Definition{Model: model, Goal: admissionGoalManager{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			session, err := owner.Session(context.Background(), NamedSession("goal-admission-"+test.name))
			if err != nil {
				t.Fatal(err)
			}
			parent, err := session.Run(context.Background(), Input{Text: "parent", IdempotencyKey: "parent"})
			if err != nil {
				t.Fatal(err)
			}
			<-model.calls
			mutation := &GoalMutation{Kind: GoalSet, Objective: test.name, MutationID: "mutation-" + test.name}
			target, err := test.admit(context.Background(), parent, Input{
				Text: test.name, IdempotencyKey: "command-" + test.name, Goal: mutation,
			})
			if err != nil {
				t.Fatal(err)
			}
			state, present, err := session.Goal(context.Background())
			if err != nil || !present || state.Objective != test.name || state.LastMutationID != mutation.MutationID {
				t.Fatalf("Goal at command receipt = %#v present=%t error=%v", state, present, err)
			}
			model.responses <- AssistantMessage("parent answer", nil)
			<-model.calls
			model.responses <- AssistantMessage("continuation answer", nil)
			if result, waitErr := target.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
				t.Fatalf("target result=%#v error=%v", result, waitErr)
			}
		})
	}

	t.Run("start-and-idempotent-replay", func(t *testing.T) {
		model := newGatedLifecycleModel()
		owner, err := New(context.Background(), Definition{Model: model, Goal: admissionGoalManager{}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Close(context.Background()) })
		session, err := owner.Session(context.Background(), NamedSession("goal-admission-start"))
		if err != nil {
			t.Fatal(err)
		}
		input := Input{
			Text: "start", IdempotencyKey: "start-command",
			Goal: &GoalMutation{Kind: GoalSet, Objective: "start", MutationID: "start-mutation"},
		}
		run, err := session.Run(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		state, present, err := session.Goal(context.Background())
		if err != nil || !present || state.Revision != 1 {
			t.Fatalf("start Goal=%#v present=%t error=%v", state, present, err)
		}
		replayed, err := session.Run(context.Background(), input)
		if err != nil || !replayed.Receipt().Replayed {
			t.Fatalf("replayed Run receipt=%#v error=%v", replayed.Receipt(), err)
		}
		replayedState, _, err := session.Goal(context.Background())
		if err != nil || replayedState.Revision != state.Revision {
			t.Fatalf("idempotent Goal replay=%#v error=%v", replayedState, err)
		}
		<-model.calls
		model.responses <- AssistantMessage("done", nil)
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("result=%#v error=%v", result, waitErr)
		}
	})

	t.Run("cas-conflict-rejects-command", func(t *testing.T) {
		model := newGatedLifecycleModel()
		owner, err := New(context.Background(), Definition{Model: model, Goal: admissionGoalManager{}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Close(context.Background()) })
		session, err := owner.Session(context.Background(), NamedSession("goal-admission-conflict"))
		if err != nil {
			t.Fatal(err)
		}
		created, err := session.UpdateGoal(context.Background(), GoalMutation{Kind: GoalSet, Objective: "current"})
		if err != nil {
			t.Fatal(err)
		}
		run, err := session.Run(context.Background(), Text("parent"))
		if err != nil {
			t.Fatal(err)
		}
		<-model.calls
		_, err = run.Queue(context.Background(), Input{
			Text: "stale", IdempotencyKey: "stale-goal-command",
			Goal: &GoalMutation{
				Kind: GoalSet, Objective: "must not apply", ExpectedRevision: created.Revision + 1,
				MutationID: "stale-goal-mutation",
			},
		})
		if err == nil {
			t.Fatal("stale Goal mutation command was accepted")
		}
		current, _, goalErr := session.Goal(context.Background())
		if goalErr != nil || current.Revision != created.Revision || current.Objective != created.Objective {
			t.Fatalf("Goal changed after rejected command: %#v error=%v", current, goalErr)
		}
		model.responses <- AssistantMessage("done", nil)
		_, _ = run.Wait(context.Background())
	})
}

type typedContinuationGoalManager struct {
	admissionGoalManager
	mu     sync.Mutex
	inputs []Input
}

func (manager *typedContinuationGoalManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "goal.typed-continuation-test", Version: 1}
}

func (manager *typedContinuationGoalManager) AfterRun(_ context.Context, request GoalAfterRunRequest) (GoalContinuation, error) {
	manager.mu.Lock()
	manager.inputs = append(manager.inputs, request.Input)
	index := len(manager.inputs)
	manager.mu.Unlock()
	if index != 1 {
		return GoalContinuation{}, nil
	}
	return GoalContinuation{Continue: true, Input: Input{
		Text:     "autonomous next step",
		HostData: &HostData{Type: "test.goal-turn", Version: 1, Data: json.RawMessage(`{"kind":"next"}`)},
	}}, nil
}

func TestDynamicGoalContinuationBuildsFreshTypedHostInput(t *testing.T) {
	model := newGatedLifecycleModel()
	manager := &typedContinuationGoalManager{}
	var mu sync.Mutex
	var prepared []PrepareRequest
	source := SourceFunc(func(_ context.Context, request PrepareRequest) (Definition, error) {
		if request.Reason != TurnReasonGoalMutation {
			if request.Input.HostData == nil || request.Input.HostData.Type != "test.goal-turn" {
				return Definition{}, errors.New("dynamic Source requires typed turn HostData")
			}
			var data struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(request.Input.HostData.Data, &data); err != nil {
				return Definition{}, err
			}
			want := "start"
			if request.Run.Autonomous {
				want = "next"
			}
			if data.Kind != want {
				return Definition{}, errors.New("dynamic Source received stale continuation HostData")
			}
		}
		mu.Lock()
		prepared = append(prepared, request)
		mu.Unlock()
		return Definition{Key: "typed-goal-source", Model: model, Goal: manager}, nil
	})
	owner, err := New(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("typed-goal-continuation"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.UpdateGoal(context.Background(), GoalMutation{Kind: GoalSet, Objective: "continue"}); err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{
		Text: "first step", IdempotencyKey: "typed-start",
		HostData: &HostData{Type: "test.goal-turn", Version: 1, Data: json.RawMessage(`{"kind":"start"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	model.responses <- AssistantMessage("first done", nil)
	second := <-model.calls
	if len(second) == 0 || second[len(second)-1].Content != "autonomous next step" {
		t.Fatalf("autonomous model input = %#v", second)
	}
	model.responses <- AssistantMessage("all done", nil)
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	manager.mu.Lock()
	inputs := append([]Input(nil), manager.inputs...)
	manager.mu.Unlock()
	if len(inputs) != 2 || inputs[0].IdempotencyKey != "typed-start" || inputs[0].HostData == nil ||
		inputs[1].HostData == nil || string(inputs[1].HostData.Data) != `{"kind":"next"}` {
		t.Fatalf("Goal AfterRun inputs = %#v", inputs)
	}
	mu.Lock()
	requests := append([]PrepareRequest(nil), prepared...)
	mu.Unlock()
	if len(requests) != 3 || requests[0].Reason != TurnReasonGoalMutation || requests[1].Reason != TurnReasonStart ||
		requests[2].Reason != TurnReasonFollowUp || !requests[2].Run.Autonomous ||
		requests[2].Run.Delivery != TurnDeliveryFollowUp {
		t.Fatalf("dynamic Source requests = %#v", requests)
	}
}

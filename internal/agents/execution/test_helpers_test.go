package execution

import (
	"context"
	"fmt"

	"denova/internal/agents/chat"
	agentstructural "denova/internal/agents/context/structural"
	"denova/internal/agents/run"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func runCycle(
	runtime *Runtime,
	ctx context.Context,
	runner *agent.Runner,
	conversation chat.Conversation,
	bookService *book.Service,
	request chat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) agentrun.Outcome {
	return runtime.Run(ctx, StartRequest{
		Cycle: Cycle{
			Runner: runner, Conversation: conversation, BookService: bookService,
			Request: request, Options: options,
		},
		Emit: emit,
	})
}

func startCycle(
	runtime *Runtime,
	ctx context.Context,
	runner *agent.Runner,
	conversation chat.Conversation,
	bookService *book.Service,
	request chat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) (*Operation, error) {
	return runtime.Start(ctx, StartRequest{
		Cycle: Cycle{
			Runner: runner, Conversation: conversation, BookService: bookService,
			Request: request, Options: options,
		},
		Emit: emit,
	})
}

type testExecutionProfile struct {
	id          ProfileID
	prepare     func(context.Context, CycleRestoreRequest) (Cycle, error)
	plan        func(context.Context, InputMaterializationRequest) (agentrun.InputMaterializationPlan, error)
	materialize func(context.Context, InputMaterializationRequest, agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error)
	reconcile   func(context.Context, agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error)
	structural  func(context.Context, StructuralRestoreRequest) (agentstructural.Spec, error)
}

func (profile *testExecutionProfile) ID() ProfileID { return profile.id }

func (profile *testExecutionProfile) PrepareCycle(ctx context.Context, request CycleRestoreRequest) (Cycle, error) {
	if profile.prepare == nil {
		return Cycle{}, ErrCyclePreparationUnavailable
	}
	return profile.prepare(ctx, request)
}

func (profile *testExecutionProfile) PlanInput(ctx context.Context, request InputMaterializationRequest) (agentrun.InputMaterializationPlan, error) {
	if profile.plan == nil {
		return agentrun.InputMaterializationPlan{}, nil
	}
	return profile.plan(ctx, request)
}

func (profile *testExecutionProfile) MaterializeInput(ctx context.Context, request InputMaterializationRequest, plan agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error) {
	if profile.materialize == nil {
		return agentrun.InputMaterializationReceipt{}, nil
	}
	return profile.materialize(ctx, request, plan)
}

func (profile *testExecutionProfile) ReconcileDomainCommit(ctx context.Context, request agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
	if profile.reconcile == nil {
		return agentrun.DomainCommitReconcileResult{}, nil
	}
	return profile.reconcile(ctx, request)
}

func (profile *testExecutionProfile) RestoreStructural(ctx context.Context, request StructuralRestoreRequest) (agentstructural.Spec, error) {
	if profile.structural == nil {
		return agentstructural.Spec{}, ErrStructuralRestoreUnavailable
	}
	return profile.structural(ctx, request)
}

func newTestExecutor(policy agentrun.LoopPolicy) *chat.Executor {
	return chat.NewExecutor(policy)
}

func runOutcomeTestGoroutine(destination chan<- agentrun.Outcome, scope string, run func() agentrun.Outcome) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- agentrun.Outcome{
					Status: agentrun.OutcomeFailed,
					Error:  fmt.Errorf("%s panic: %v", scope, recovered),
				}
			}
		}()
		destination <- run()
	}()
}

type engineTestResult struct {
	result runstate.EngineResult
	err    error
}

func runEngineTestGoroutine(
	destination chan<- engineTestResult,
	scope string,
	run func() (runstate.EngineResult, error),
) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- engineTestResult{err: fmt.Errorf("%s panic: %v", scope, recovered)}
			}
		}()
		result, err := run()
		destination <- engineTestResult{result: result, err: err}
	}()
}

func runErrorTestGoroutine(destination chan<- error, scope string, run func() error) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- fmt.Errorf("%s panic: %v", scope, recovered)
			}
		}()
		destination <- run()
	}()
}

func countEventType(events []agentrun.Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func mustRuntimeBinding(binding agentrun.RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}

package app

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
)

func executionProfileForTest(application *App, binding agentrun.RuntimeBinding) (agentexecution.Profile, error) {
	ref, err := binding.Ref()
	if err != nil {
		return nil, err
	}
	for _, profile := range application.executionProfiles() {
		if profile.ID() == agentexecution.ProfileID(ref.Profile) {
			return profile, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", agentexecution.ErrProfileNotFound, ref.Profile)
}

func prepareProfileCycleForTest(application *App, ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
	profile, err := executionProfileForTest(application, request.Binding)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	queued, ok := profile.(agentexecution.QueuedCycleProfile)
	if !ok {
		return agentexecution.Cycle{}, fmt.Errorf("%w: profile %q", agentexecution.ErrCyclePreparationUnavailable, profile.ID())
	}
	return queued.PrepareCycle(ctx, request)
}

func reconcileProfileDomainCommitForTest(application *App, ctx context.Context, request agentrun.DomainCommitReconcileRequest) (agentrun.DomainCommitReconcileResult, error) {
	profile, err := executionProfileForTest(application, request.Binding)
	if err != nil {
		return agentrun.DomainCommitReconcileResult{}, err
	}
	domain, ok := profile.(agentexecution.DomainCommitProfile)
	if !ok {
		return agentrun.DomainCommitReconcileResult{}, fmt.Errorf("%w: profile %q", agentexecution.ErrDomainCommitUnavailable, profile.ID())
	}
	return domain.ReconcileDomainCommit(ctx, request)
}

func planProfileInputForTest(application *App, ctx context.Context, request agentexecution.InputMaterializationRequest) (agentrun.InputMaterializationPlan, error) {
	profile, err := executionProfileForTest(application, request.Binding)
	if err != nil {
		return agentrun.InputMaterializationPlan{}, err
	}
	input, ok := profile.(agentexecution.InputProfile)
	if !ok {
		return agentrun.InputMaterializationPlan{}, fmt.Errorf("%w: profile %q", agentexecution.ErrInputMaterializationUnavailable, profile.ID())
	}
	return input.PlanInput(ctx, request)
}

func materializeProfileInputForTest(application *App, ctx context.Context, request agentexecution.InputMaterializationRequest, plan agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error) {
	profile, err := executionProfileForTest(application, request.Binding)
	if err != nil {
		return agentrun.InputMaterializationReceipt{}, err
	}
	input, ok := profile.(agentexecution.InputProfile)
	if !ok {
		return agentrun.InputMaterializationReceipt{}, fmt.Errorf("%w: profile %q", agentexecution.ErrInputMaterializationUnavailable, profile.ID())
	}
	return input.MaterializeInput(ctx, request, plan)
}

func assertExecutionProfileCapabilitiesForTest(
	profile agentexecution.Profile,
) (queued, input, domain, structural bool) {
	_, queued = profile.(agentexecution.QueuedCycleProfile)
	_, input = profile.(agentexecution.InputProfile)
	_, domain = profile.(agentexecution.DomainCommitProfile)
	_, structural = profile.(agentexecution.StructuralProfile)
	return
}

type profileInputMaterializerForTest struct{ application *App }

func (materializer profileInputMaterializerForTest) PlanInput(ctx context.Context, request agentexecution.InputMaterializationRequest) (agentrun.InputMaterializationPlan, error) {
	return planProfileInputForTest(materializer.application, ctx, request)
}

func (materializer profileInputMaterializerForTest) MaterializeInput(ctx context.Context, request agentexecution.InputMaterializationRequest, plan agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error) {
	return materializeProfileInputForTest(materializer.application, ctx, request, plan)
}

func runExecutionCycle(
	runtime *agentexecution.Runtime,
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	request agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) agentrun.Outcome {
	return runtime.Run(ctx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Runner: runner, Conversation: conversation, BookService: bookService,
			Request: request, Options: options,
		},
		Emit: emit,
	})
}

func startExecutionCycle(
	runtime *agentexecution.Runtime,
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	request agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) (*agentexecution.Operation, error) {
	return runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Runner: runner, Conversation: conversation, BookService: bookService,
			Request: request, Options: options,
		},
		Emit: emit,
	})
}

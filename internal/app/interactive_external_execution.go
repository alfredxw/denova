package app

import (
	"context"
	"errors"

	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	interactiveapp "denova/internal/app/interactive"
)

func (s *InteractiveAppService) externalGameFactory(initial *interactiveAgentCycle, taskID string) agentruntime.ExternalCycleFactory {
	return func(ctx context.Context, input agentruntime.ExternalCycleInput, emit func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (agentruntime.ExternalCycle, error) {
		request := input.Request
		cycle := initial
		var err error
		if input.Resume {
			pending, err := interactiveapp.ExternalTurnInterruption(initial.store, initial.storyID, initial.branchID)
			if err != nil {
				return nil, err
			}
			if pending != nil {
				request.ResumeInterruptionID = pending.ID
			}
		}
		if input.Resume || request.CommandID != initial.request.CommandID {
			cycle, err = s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
				CommandID: request.CommandID, StoryID: initial.storyID, BranchID: initial.branchID, Message: request.Message,
				ResumeInterruptionID: request.ResumeInterruptionID, StyleScenes: request.StyleScenes, Locale: request.Locale,
				InputVisibility: request.InputVisibility, AttachedFiles: request.AttachedFiles, AttachmentIDs: request.AttachmentIDs,
				RegenerateFromTurnID: input.RegenerateFromTurnID,
			})
			if err != nil {
				return nil, err
			}
		}
		if cycle.externalAssembly == nil {
			return nil, errors.New("external Game selection changed during execution")
		}
		cycle.bindCommit(emit)
		state, err := agentruntime.GameState(cycle.options(taskID), cycle.store)
		if err != nil {
			return nil, err
		}
		plan, err := state.Plan(ctx)
		if err != nil {
			return nil, err
		}
		return interactiveapp.StartExternalTurn(ctx, interactiveapp.ExternalTurnConfig{
			Conversation: cycle.conversation, Request: cycle.request, Config: cycle.runtimeCfg, Assembly: *cycle.externalAssembly,
			BookService: cycle.bookService, Runtime: s.app.AgentEngines().ExternalRuntime(cycle.runtimeCfg), Emit: emit,
			Plan: plan, ObservePlan: state.ObservePlan, Guidance: input.Guidance, PrepareHistory: state.PrepareExternalHistory,
		})
	}
}

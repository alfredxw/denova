package app

import (
	"context"
	"errors"
	"strings"

	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive"
)

// resolveGameResumeBinding resolves the pending player input a resume request
// actually continues. It returns empty when the request is not a resume; an
// interruption that no longer matches the journal or whose input is missing
// fails with the same errors StartExternalTurn's own resume check raises. The
// engine-operation prefix check stays with StartExternalTurn, which is the
// authoritative gate before the provider resumes.
func resolveGameResumeBinding(store *interactive.Store, storyID, branchID, resumeInterruptionID string) (agentrun.CommandID, error) {
	resumeInterruptionID = strings.TrimSpace(resumeInterruptionID)
	if resumeInterruptionID == "" {
		return "", nil
	}
	pending, err := interactiveapp.ExternalTurnInterruption(store, storyID, branchID)
	if err != nil {
		return "", err
	}
	if pending == nil || pending.ID != resumeInterruptionID {
		return "", errors.New("Game interruption changed before resume")
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return "", err
	}
	for _, input := range snapshot.PendingPlayerInputs {
		if input.ID == pending.PlayerInputID {
			return agentrun.CommandID(input.AgentCommandID), nil
		}
	}
	// Reachable only when the journal changes between the interruption read and
	// this snapshot read; StartExternalTurn re-checks and owns the final refusal.
	return "", errors.New("interrupted Game input is unavailable")
}

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

package app

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
)

// restoreHarnessTurn reconstructs only process-local execution dependencies.
// The durable descriptor remains authoritative for caller input and binding;
// actual model/tool work starts later, after the runtime accepts the exact
// queued command replay and invokes HarnessTurnSpec.Prepare.
func (a *App) restoreHarnessTurn(_ context.Context, request agent.HarnessTurnRestoreRequest) (agent.HarnessTurnSpec, error) {
	if a == nil {
		return agent.HarnessTurnSpec{}, agent.ErrHarnessTurnRestoreUnavailable
	}
	switch request.Binding.Profile {
	case runstate.ProfileWriting:
		return a.restoreWritingHarnessTurn(request), nil
	case runstate.ProfileGame:
		return a.restoreInteractiveHarnessTurn(request), nil
	default:
		return agent.HarnessTurnSpec{}, fmt.Errorf(
			"%w: profile %q has no queued-turn restorer",
			agent.ErrHarnessTurnRestoreUnavailable,
			request.Binding.Profile,
		)
	}
}

func (a *App) restoreWritingHarnessTurn(request agent.HarnessTurnRestoreRequest) agent.HarnessTurnSpec {
	return agent.HarnessTurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agent.HarnessTurnExecution, error) {
			execution, runtime, err := a.chat().prepareWritingHarnessTurn(ctx, request.Request, request.Options.TaskID)
			if err != nil {
				return agent.HarnessTurnExecution{}, err
			}
			if strings.TrimSpace(runtime.workspace) != request.Binding.Workspace || runtime.sess == nil || runtime.sess.ID != request.Binding.SessionID {
				return agent.HarnessTurnExecution{}, fmt.Errorf(
					"%w: restored writing runtime does not match durable binding",
					agent.ErrHarnessTurnRestoreUnavailable,
				)
			}
			return execution, nil
		},
	}
}

func (a *App) restoreInteractiveHarnessTurn(request agent.HarnessTurnRestoreRequest) agent.HarnessTurnSpec {
	return agent.HarnessTurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agent.HarnessTurnExecution, error) {
			cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
				StoryID: request.Binding.StoryID, BranchID: request.Binding.BranchID,
				Message: request.Request.Message, StyleScenes: request.Request.StyleScenes,
				Locale: request.Request.Locale,
			})
			if err != nil {
				return agent.HarnessTurnExecution{}, err
			}
			if cycle.workspace != request.Binding.Workspace || cycle.storyID != request.Binding.StoryID || cycle.branchID != request.Binding.BranchID {
				return agent.HarnessTurnExecution{}, fmt.Errorf(
					"%w: restored game runtime does not match durable binding",
					agent.ErrHarnessTurnRestoreUnavailable,
				)
			}
			cycle.bindCommit(request.Emit)
			return agent.HarnessTurnExecution{
				Runner: cycle.runner, Conversation: cycle.conversation,
				BookService: cycle.bookService, Request: cycle.request,
				Options: cycle.options(request.Options.TaskID),
			}, nil
		},
	}
}

func (s *ChatAppService) prepareWritingHarnessTurn(
	ctx context.Context,
	request agent.ChatRequest,
	taskID string,
) (agent.HarnessTurnExecution, ideChatRuntime, error) {
	runtime, resolved, err := s.prepareIDEChatRuntime(ctx, request)
	if err != nil {
		return agent.HarnessTurnExecution{}, ideChatRuntime{}, err
	}
	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		return agent.HarnessTurnExecution{}, ideChatRuntime{}, err
	}
	runtimeContexts := agent.IDEWorkspaceRuntimeContextsForRequest(runtime.state, resolved)
	conversation := agent.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	var onUserMessageCommitted func(context.Context) error
	if !resolved.ResolvedReviewFeedback.Empty() {
		onUserMessageCommitted = func(commitCtx context.Context) error {
			return s.consumeResolvedReviewFeedback(commitCtx, runtime, resolved)
		}
	}
	return agent.HarnessTurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService, Request: resolved,
		Options: agent.RunOptions{
			AgentKind:              agent.AgentKindIDE,
			TaskID:                 strings.TrimSpace(taskID),
			SessionID:              runtime.sess.ID,
			ReviewThreadID:         resolved.ResolvedReviewFeedback.PrimaryReviewThreadID(),
			Workspace:              runtime.workspace,
			Mode:                   "ide",
			IdleTimeout:            agentIdleTimeout(runtime.cfg),
			ToolResultMaxBytes:     agentToolResultMaxBytes(runtime.cfg),
			SystemPromptLog:        systemPrompt,
			OnMutationsVerified:    s.app.writingMutationCallback(strings.TrimSpace(taskID), conversation),
			OnUserMessageCommitted: onUserMessageCommitted,
		},
	}, runtime, nil
}

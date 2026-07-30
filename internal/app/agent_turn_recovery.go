package app

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
)

// restoreHarnessTurn reconstructs only process-local execution dependencies.
// The durable descriptor remains authoritative for caller input and binding;
// actual model/tool work starts later, after the runtime accepts the exact
// queued command replay and invokes HarnessTurnSpec.Prepare.
func (a *App) restoreHarnessTurn(_ context.Context, request agents.HarnessTurnRestoreRequest) (agents.HarnessTurnSpec, error) {
	if a == nil {
		return agents.HarnessTurnSpec{}, agents.ErrHarnessTurnRestoreUnavailable
	}
	binding := request.Binding
	switch binding.AgentKind {
	case agents.AgentKindGeneral:
		return a.agentChat().restoreHarnessTurn(request, binding), nil
	case agents.AgentKindIDE:
		if binding.Mode == agentChatRuntimeMode {
			return a.agentChat().restoreHarnessTurn(request, binding), nil
		}
		return a.restoreWritingHarnessTurn(request, binding), nil
	case agents.AgentKindInteractiveStory:
		return a.restoreInteractiveHarnessTurn(request, binding), nil
	default:
		return agents.HarnessTurnSpec{}, fmt.Errorf(
			"%w: profile %q has no queued-turn restorer",
			agents.ErrHarnessTurnRestoreUnavailable,
			binding.AgentKind,
		)
	}
}

func (a *App) restoreWritingHarnessTurn(request agents.HarnessTurnRestoreRequest, binding agents.RuntimeBinding) agents.HarnessTurnSpec {
	return agents.HarnessTurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agents.HarnessTurnExecution, error) {
			execution, runtime, err := a.chat().prepareWritingHarnessTurn(ctx, request.Request, request.Options.TaskID)
			if err != nil {
				return agents.HarnessTurnExecution{}, err
			}
			if strings.TrimSpace(runtime.workspace) != binding.Workspace || runtime.sess == nil || runtime.sess.ID != binding.SessionID {
				return agents.HarnessTurnExecution{}, fmt.Errorf(
					"%w: restored writing runtime does not match durable binding",
					agents.ErrHarnessTurnRestoreUnavailable,
				)
			}
			return execution, nil
		},
	}
}

func (a *App) restoreInteractiveHarnessTurn(request agents.HarnessTurnRestoreRequest, binding agents.RuntimeBinding) agents.HarnessTurnSpec {
	return agents.HarnessTurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agents.HarnessTurnExecution, error) {
			cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
				StoryID: binding.StoryID, BranchID: binding.BranchID,
				Message: request.Request.Message, StyleScenes: request.Request.StyleScenes,
				Locale: request.Request.Locale,
			})
			if err != nil {
				return agents.HarnessTurnExecution{}, err
			}
			if cycle.workspace != binding.Workspace || cycle.storyID != binding.StoryID || cycle.branchID != binding.BranchID {
				return agents.HarnessTurnExecution{}, fmt.Errorf(
					"%w: restored game runtime does not match durable binding",
					agents.ErrHarnessTurnRestoreUnavailable,
				)
			}
			cycle.bindCommit(request.Emit)
			return agents.HarnessTurnExecution{
				Runner: cycle.runner, Conversation: cycle.conversation,
				BookService: cycle.bookService, Request: cycle.request,
				Options: cycle.options(request.Options.TaskID),
			}, nil
		},
	}
}

func (s *ChatAppService) prepareWritingHarnessTurn(
	ctx context.Context,
	request agents.ChatRequest,
	taskID string,
) (agents.HarnessTurnExecution, ideChatRuntime, error) {
	runtime, resolved, err := s.prepareIDEChatRuntime(ctx, request)
	if err != nil {
		return agents.HarnessTurnExecution{}, ideChatRuntime{}, err
	}
	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		return agents.HarnessTurnExecution{}, ideChatRuntime{}, err
	}
	runtimeContexts := agents.IDEWorkspaceRuntimeContextsForRequest(runtime.state, resolved)
	conversation := agents.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	options := s.bindReviewFeedbackInputCommit(agents.RunOptions{
		AgentKind:          agents.AgentKindIDE,
		StateRoot:          runtime.projectState,
		TaskID:             strings.TrimSpace(taskID),
		SessionID:          runtime.sess.ID,
		Workspace:          runtime.workspace,
		Mode:               "ide",
		IdleTimeout:        agentIdleTimeout(runtime.cfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(runtime.cfg),
		SystemPromptLog:    systemPrompt,
		OnMutationsVerified: s.app.writingMutationCallback(
			strings.TrimSpace(taskID), conversation,
		),
	}, runtime, resolved)
	return agents.HarnessTurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService, Request: resolved,
		Options: options,
	}, runtime, nil
}

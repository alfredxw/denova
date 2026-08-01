package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
)

// restoreHarnessTurn reconstructs only process-local execution dependencies.
// The durable descriptor remains authoritative for caller input and binding;
// actual model/tool work starts later, after the runtime accepts the exact
// queued command replay and invokes TurnSpec.Prepare.
func (a *App) restoreHarnessTurn(_ context.Context, request agentharness.TurnRestoreRequest) (agentharness.TurnSpec, error) {
	if a == nil {
		return agentharness.TurnSpec{}, agentharness.ErrTurnRestoreUnavailable
	}
	binding := request.Binding
	switch binding.AgentKind {
	case agentrun.AgentKindGeneral:
		return a.agentChat().restoreHarnessTurn(request, binding), nil
	case agentrun.AgentKindIDE:
		if binding.Mode == agentChatRuntimeMode {
			return a.agentChat().restoreHarnessTurn(request, binding), nil
		}
		return a.restoreWritingHarnessTurn(request, binding), nil
	case agentrun.AgentKindInteractiveStory:
		return a.restoreInteractiveHarnessTurn(request, binding), nil
	default:
		return agentharness.TurnSpec{}, fmt.Errorf(
			"%w: profile %q has no queued-turn restorer",
			agentharness.ErrTurnRestoreUnavailable,
			binding.AgentKind,
		)
	}
}

func (a *App) restoreWritingHarnessTurn(request agentharness.TurnRestoreRequest, binding agentrun.RuntimeBinding) agentharness.TurnSpec {
	return agentharness.TurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agentharness.TurnExecution, error) {
			execution, runtime, err := a.chat().prepareWritingHarnessTurn(ctx, request.Request, request.Options.TaskID)
			if err != nil {
				return agentharness.TurnExecution{}, err
			}
			if strings.TrimSpace(runtime.workspace) != binding.Workspace || runtime.sess == nil || runtime.sess.ID != binding.SessionID {
				return agentharness.TurnExecution{}, fmt.Errorf(
					"%w: restored writing runtime does not match durable binding",
					agentharness.ErrTurnRestoreUnavailable,
				)
			}
			return execution, nil
		},
	}
}

func (a *App) restoreInteractiveHarnessTurn(request agentharness.TurnRestoreRequest, binding agentrun.RuntimeBinding) agentharness.TurnSpec {
	return agentharness.TurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agentharness.TurnExecution, error) {
			cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
				StoryID: binding.StoryID, BranchID: binding.BranchID,
				Message: request.Request.Message, StyleScenes: request.Request.StyleScenes,
				Locale: request.Request.Locale,
			})
			if err != nil {
				return agentharness.TurnExecution{}, err
			}
			if cycle.workspace != binding.Workspace || cycle.storyID != binding.StoryID || cycle.branchID != binding.BranchID {
				return agentharness.TurnExecution{}, fmt.Errorf(
					"%w: restored game runtime does not match durable binding",
					agentharness.ErrTurnRestoreUnavailable,
				)
			}
			cycle.bindCommit(request.Emit)
			return agentharness.TurnExecution{
				Runner: cycle.runner, Conversation: cycle.conversation,
				BookService: cycle.bookService, Request: cycle.request,
				Options: cycle.options(request.Options.TaskID),
			}, nil
		},
	}
}

func (s *ChatAppService) prepareWritingHarnessTurn(
	ctx context.Context,
	request agentchat.ChatRequest,
	taskID string,
) (agentharness.TurnExecution, ideChatRuntime, error) {
	runtime, resolved, err := s.prepareIDEChatRuntime(ctx, request)
	if err != nil {
		return agentharness.TurnExecution{}, ideChatRuntime{}, err
	}
	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		return agentharness.TurnExecution{}, ideChatRuntime{}, err
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, resolved.IDEContext)
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	options := s.bindReviewFeedbackInputCommit(agentrun.Options{
		AgentKind:          agentrun.AgentKindIDE,
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
	return agentharness.TurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService, Request: resolved,
		Options: options,
	}, runtime, nil
}

package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"

	agent "github.com/alfredxw/denova/agent"
)

func (a *App) prepareWritingProfileCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	var activeRuntime ideChatRuntime
	var task *apptask.Task
	if strings.TrimSpace(request.Options.TaskID) != "" {
		var err error
		activeRuntime, task, err = a.chat().activeCommandRuntime()
		if err != nil {
			return agentexecution.Cycle{}, err
		}
		if task.ID() != strings.TrimSpace(request.Options.TaskID) || activeRuntime.workspace != binding.Workspace || activeRuntime.sess == nil || activeRuntime.sess.ID != binding.SessionID {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
	}
	cycle, runtime, err := a.chat().prepareWritingCycle(ctx, request.Request, request.Options.TaskID)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if strings.TrimSpace(runtime.workspace) != binding.Workspace || runtime.sess == nil || runtime.sess.ID != binding.SessionID {
		return agentexecution.Cycle{}, fmt.Errorf(
			"%w: prepared writing runtime does not match durable binding",
			agentexecution.ErrCyclePreparationUnavailable,
		)
	}
	if task != nil {
		if runtime.workspace != activeRuntime.workspace || runtime.sess != activeRuntime.sess || runtime.executionRuntime != activeRuntime.executionRuntime {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
		if err := a.chat().confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentexecution.Cycle{}, err
		}
	}
	return cycle, nil
}

func (a *App) prepareInteractiveProfileCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	var target interactiveAgentCommandTarget
	if strings.TrimSpace(request.Options.TaskID) != "" {
		var err error
		target, err = a.interactiveService().activeAgentCommandTarget(binding.StoryID, binding.BranchID)
		if err != nil {
			return agentexecution.Cycle{}, err
		}
		if target.task.ID() != strings.TrimSpace(request.Options.TaskID) {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
	}
	cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
		CommandID: string(request.CommandID),
		StoryID:   binding.StoryID, BranchID: binding.BranchID,
		Message: request.Request.Message, StyleScenes: request.Request.StyleScenes,
		Locale: request.Request.Locale,
	})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if cycle.workspace != binding.Workspace || cycle.storyID != binding.StoryID || cycle.branchID != binding.BranchID {
		return agentexecution.Cycle{}, fmt.Errorf(
			"%w: prepared game runtime does not match durable binding",
			agentexecution.ErrCyclePreparationUnavailable,
		)
	}
	if target.task != nil {
		if cycle.executionRuntime != target.executionRuntime {
			return agentexecution.Cycle{}, ErrAgentContextChanged
		}
		if err := a.interactiveService().confirmActiveAgentCommandTarget(target); err != nil {
			return agentexecution.Cycle{}, err
		}
	}
	cycle.bindCommit(request.Emit)
	return agentexecution.Cycle{
		Definition: cycle.definition, Conversation: cycle.conversation,
		BookService: cycle.bookService, Request: cycle.request,
		Options: cycle.options(request.Options.TaskID),
	}, nil
}

func (s *ChatAppService) prepareWritingCycle(
	ctx context.Context,
	request agentchat.ChatRequest,
	taskID string,
) (agentexecution.Cycle, ideChatRuntime, error) {
	runtime, resolved, err := s.prepareIDEChatRuntime(ctx, request)
	if err != nil {
		return agentexecution.Cycle{}, ideChatRuntime{}, err
	}
	builtAgent, err := appagentruntime.BuildConversationAgent(
		ctx, &runtime.cfg, runtime.state, runtime.ideTeller, agentrun.AgentKindIDE,
	)
	if err != nil {
		return agentexecution.Cycle{}, ideChatRuntime{}, err
	}
	systemPrompt := builtAgent.Composition
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, resolved.IDEContext)
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	).WithInputVisibility(resolved.InputVisibility)
	options := s.bindReviewFeedbackInputCommit(agentrun.Options{
		AgentKind:          agentrun.AgentKindIDE,
		StateRoot:          runtime.projectState,
		TaskID:             strings.TrimSpace(taskID),
		SessionID:          runtime.sess.ID,
		Workspace:          runtime.workspace,
		Mode:               "ide",
		IdleTimeout:        appagentruntime.IdleTimeout(runtime.cfg),
		ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtime.cfg),
		SystemPromptLog:    systemPrompt,
		OnMutationsVerified: s.app.writingMutationCallback(
			strings.TrimSpace(taskID), conversation,
		),
	}, runtime, resolved)
	return agentexecution.Cycle{
		Definition: builtAgent.Definition, Conversation: conversation, BookService: runtime.bookService, Request: resolved,
		Options: options,
	}, runtime, nil
}

type queuedExecutionProfile struct {
	id        agentexecution.ProfileID
	prepare   func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error)
	canonical func(context.Context, agentexecution.CanonicalInputRequest) (agent.CanonicalAdapter, error)
}

func (profile queuedExecutionProfile) ID() agentexecution.ProfileID {
	return profile.id
}

func (profile queuedExecutionProfile) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
) (agentexecution.Cycle, error) {
	if profile.prepare == nil {
		return agentexecution.Cycle{}, fmt.Errorf("%w: profile %q cannot prepare a cycle", agentexecution.ErrProfileInvalid, profile.ID())
	}
	return profile.prepare(ctx, request)
}

func (profile queuedExecutionProfile) CanonicalInput(
	ctx context.Context,
	request agentexecution.CanonicalInputRequest,
) (agent.CanonicalAdapter, error) {
	if profile.canonical == nil {
		return nil, fmt.Errorf("%w: profile %q has no canonical input boundary", agentexecution.ErrProfileInvalid, profile.ID())
	}
	return profile.canonical(ctx, request)
}

func (app *App) executionProfiles() []agentexecution.Profile {
	profile := func(
		id agentexecution.ProfileID,
		prepare func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error),
		canonical func(context.Context, agentexecution.CanonicalInputRequest) (agent.CanonicalAdapter, error),
	) queuedExecutionProfile {
		return queuedExecutionProfile{id: id, prepare: prepare, canonical: canonical}
	}
	return []agentexecution.Profile{
		profile(agentexecution.ProfileWriting, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.prepareWritingProfileCycle(ctx, request, request.Binding)
		}, app.sessionCanonicalInput),
		profile(agentexecution.ProfileAgentChat, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.AgentChat().PrepareCycle(ctx, request, request.Binding)
		}, app.sessionCanonicalInput),
		profile(agentexecution.ProfileGame, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.prepareInteractiveProfileCycle(ctx, request, request.Binding)
		}, app.gameCanonicalInput),
		profile(agentexecution.ProfileConfigManager, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.ConfigManager().PrepareCycle(ctx, request, request.Binding)
		}, app.sessionCanonicalInput),
		profile(agentexecution.ProfileHarnessOptimizer, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.ContinualLearning().PrepareCycle(ctx, request, request.Binding)
		}, app.sessionCanonicalInput),
		profile(agentexecution.ProfileImage, func(ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
			return app.Images().PrepareCycle(ctx, request, request.Binding)
		}, app.sessionCanonicalInput),
	}
}

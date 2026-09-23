package app

import (
	"context"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	conversationapp "denova/internal/app/conversation"
	interactiveapp "denova/internal/app/interactive"
	apptask "denova/internal/app/task"
)

func (s *ChatAppService) recoverExternal(ctx context.Context, control *agentruntime.ExternalController, action agentexecution.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	input, err := control.RecoveryInput(ctx)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	runtime, request, err := s.prepareIDEChatRuntime(ctx, input.Request)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	host, err := s.app.AgentHostCapabilities(ctx, &runtime.cfg, agentrun.AgentKindIDE)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	executor, err := conversationapp.BuildExecution(ctx, sharedConversationRuntime(runtime), host, s.app.AgentEngines(), "")
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	a := s.app
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition || a.workspace != runtime.workspace || a.session != runtime.sess {
			return ErrAgentContextChanged
		}
		if err := a.registerWorkspaceTaskLocked(task, runtime.workspace, true); err != nil {
			return err
		}
		a.activeTask = task
		a.activeWritingRun = &writingTaskRun{task: task, runtime: runtime, recoveryActions: map[string]agentrun.CommandReceipt{}}
		return nil
	})
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if action.Kind == agentexecution.RuntimeRecoveryAbort {
		if err := a.AgentEngines().Operations.AbortQuestions(ctx, runtime.projectID, runtime.sess); err != nil {
			rollbackWritingAttachTask(a, task, err)
			return AgentRuntimeRecoveryResult{}, err
		}
	}
	run, err := control.Resume(ctx, action, executor.ExternalFactory(request, conversationapp.ProjectConversation(sharedConversationRuntime(runtime), request), runtime.agentOptions(task.ID())), task.Emit)
	if err != nil {
		rollbackWritingAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	a.mu.Lock()
	a.activeWritingRun.recoveryActions[recoveryActionKey(action)] = run.Receipt()
	a.mu.Unlock()
	err = task.Start(func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		run.Wait(ctx)
	})
	if err != nil {
		task.Abort()
		run.Wait(task.Context())
		rollbackWritingAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: action, Receipt: run.Receipt()}, nil
}

func (s *InteractiveAppService) recoverExternal(ctx context.Context, control *agentruntime.ExternalController, options agentrun.Options, action agentexecution.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	input, err := control.RecoveryInput(ctx)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	pending, err := interactiveapp.ExternalTurnInterruption(s.store(), options.StoryID, options.BranchID)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	resumeID := ""
	if pending != nil {
		resumeID = pending.ID
	}
	cycle, err := s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{CommandID: input.Request.CommandID, ResumeInterruptionID: resumeID, StoryID: options.StoryID, BranchID: options.BranchID, Message: input.Request.Message, Locale: input.Request.Locale, StyleScenes: input.Request.StyleScenes, AttachedFiles: input.Request.AttachedFiles, InputVisibility: input.Request.InputVisibility, RegenerateFromTurnID: input.RegenerateFromTurnID})
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	a := s.app
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition || a.workspace != cycle.workspace || a.interactive != cycle.store {
			return ErrAgentContextChanged
		}
		if err := a.registerWorkspaceTaskLocked(task, cycle.workspace, true); err != nil {
			return err
		}
		a.activeInteractiveRun = &interactiveTaskRun{task: task, info: InteractiveTaskInfo{TaskID: task.ID(), ProjectID: options.ProjectID, Workspace: options.Workspace, StoryID: options.StoryID, BranchID: options.BranchID, CommandID: input.Request.CommandID, Message: input.Request.Message, RegenerateFromTurnID: input.RegenerateFromTurnID, Attachments: input.Request.AttachedFiles}, recoveryActions: map[string]agentrun.CommandReceipt{}}
		return nil
	})
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run, err := control.Resume(ctx, action, s.externalGameFactory(cycle, task.ID()), task.Emit)
	if err != nil {
		rollbackInteractiveAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	a.mu.Lock()
	a.activeInteractiveRun.recoveryActions[recoveryActionKey(action)] = run.Receipt()
	a.mu.Unlock()
	err = task.Start(func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		run.Wait(ctx)
	})
	if err != nil {
		task.Abort()
		run.Wait(task.Context())
		rollbackInteractiveAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: action, Receipt: run.Receipt()}, nil
}

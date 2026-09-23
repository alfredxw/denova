package agentchat

import (
	"context"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	appagentruntime "denova/internal/app/agentruntime"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
)

func (service *Service) recoverExternal(ctx context.Context, binding Binding, control *agentruntime.ExternalController, action agentexecution.RuntimeRecoveryAction) (appagentruntime.RecoveryResult, error) {
	input, err := control.RecoveryInput(ctx)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	runtime, request, err := conversationapp.Prepare(ctx, project.conversation(sess), input.Request)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	host, err := service.host.ProjectAgentHostCapabilities(ctx, runtime.ProjectType, &runtime.Config, runtime.AgentKind)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	executor, err := conversationapp.BuildExecution(ctx, runtime, host, service.host.AgentEngines(), "")
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	active := &run{binding: binding, runtime: runtime, commandID: request.CommandID, recoveryActions: map[string]agentrun.CommandReceipt{}}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error { active.task = task; return service.installActiveRun(active) })
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	if action.Kind == agentexecution.RuntimeRecoveryAbort {
		err = service.host.AgentEngines().Operations.AbortQuestions(ctx, binding.ProjectID, sess)
	}
	var resumed *agentruntime.ExternalRun
	if err == nil {
		resumed, err = control.Resume(ctx, action, executor.ExternalFactory(request, conversationapp.ProjectConversation(runtime, request), runtimeOptions(binding, task.ID())), task.Emit)
	}
	if err != nil {
		task.RejectStart(err)
		service.releaseActiveRun(active)
		return appagentruntime.RecoveryResult{}, err
	}
	active.recoveryActions[agentruntime.RecoveryActionKey(action)] = resumed.Receipt()
	err = task.Start(func(ctx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		defer service.releaseActiveRun(active)
		resumed.Wait(ctx)
	})
	if err != nil {
		task.Abort()
		resumed.Wait(task.Context())
		task.RejectStart(err)
		service.releaseActiveRun(active)
		return appagentruntime.RecoveryResult{}, err
	}
	return appagentruntime.RecoveryResult{Task: task, Action: action, Receipt: resumed.Receipt()}, nil
}

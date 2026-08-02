package agentchat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	chatagent "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	appagentruntime "denova/internal/app/agentruntime"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
)

// StartTask starts one project-scoped turn without switching the foreground
// Writing Book or Session.
func (service *Service) StartTask(ctx context.Context, binding Binding, request chatagent.ChatRequest) (*apptask.Task, error) {
	service.admission.Lock()
	defer service.admission.Unlock()

	var err error
	binding, err = service.ResolveBinding(binding)
	if err != nil {
		return nil, err
	}
	request.CommandID = strings.TrimSpace(request.CommandID)
	if request.CommandID == "" {
		return nil, apptask.ErrCommandIDRequired
	}
	if err := agentrun.ValidateCommandID(request.CommandID); err != nil {
		return nil, err
	}
	request = chatagent.CaptureChatRequestCallerInput(request)
	fingerprint := agentharness.RequestSemanticFingerprint(request)
	identity := apptask.StartIdentity{
		CommandID: request.CommandID, Scope: binding.ProjectID,
		SessionID: binding.SessionID, Fingerprint: fingerprint,
	}
	if replay, ok, err := service.starts.Replay(identity); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	if active := service.activeRun(binding); active != nil && active.task != nil && !active.task.Finished() {
		return nil, appagentruntime.ErrOperationActive
	}

	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return nil, err
	}
	sess, err := getOrCreateConversation(project, binding)
	if err != nil {
		return nil, err
	}
	runtime, request, err := conversationapp.Prepare(ctx, project.conversation(sess), request)
	if err != nil {
		return nil, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConversation(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
	)
	if err != nil {
		return nil, err
	}
	conversation := conversationapp.ProjectConversation(runtime, request)

	var mutationMu sync.Mutex
	var verifiedMutations []agenttool.Mutation
	var postRunVerification agenttool.Verification
	active := &run{binding: binding, commandID: request.CommandID, runtime: runtime}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		active.task = task
		return service.installActiveRun(active)
	})
	if err != nil {
		return nil, err
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		task.RejectStart(err)
		service.releaseActiveRun(active)
		return nil, err
	}

	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	options := startOptions(active, request.ResolvedReviewFeedback.PrimaryReviewThreadID(), systemPrompt, func(mutations []agenttool.Mutation, verification agenttool.Verification) {
		mutationMu.Lock()
		verifiedMutations = append([]agenttool.Mutation(nil), mutations...)
		postRunVerification = verification
		mutationMu.Unlock()
	})
	options = conversationapp.BindReviewFeedback(options, runtime, request)
	accepted, err := runtime.ChatService.StartWithOptions(
		acceptCtx, runner, conversation, runtime.BookService, request, options, task.Emit,
	)
	releaseAcceptance()
	if err != nil {
		reservation.Rollback()
		task.RejectStart(err)
		service.releaseActiveRun(active)
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return nil, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, request.CommandID)
		}
		return nil, err
	}

	if err := task.Start(func(runCtx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer service.releaseActiveRun(active)
		slog.InfoContext(runCtx, fmt.Sprintf(
			"[app/agentchat] run begin task_id=%s project_id=%s workspace=%q session_id=%s message_len=%d",
			task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID, len(request.Message),
		))
		accepted.Wait(runCtx)
		_, outputCommitted := conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
		postSettlementCtx := runCtx
		if outputCommitted {
			postSettlementCtx = context.WithoutCancel(runCtx)
		}
		mutationMu.Lock()
		mutations := append([]agenttool.Mutation(nil), verifiedMutations...)
		verification := postRunVerification
		mutationMu.Unlock()
		if outputCommitted && len(mutations) > 0 && service.host != nil {
			service.host.OnVerifiedMutations(postSettlementCtx, "agent_chat_post_run", runtime.VersionService, runtime.Config, mutations, verification)
		}
		slog.InfoContext(runCtx, fmt.Sprintf(
			"[app/agentchat] run end task_id=%s project_id=%s workspace=%q session_id=%s status=%s",
			task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID, task.Status(),
		))
	}); err != nil {
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		task.Finish()
		service.releaseActiveRun(active)
		return nil, err
	}
	reservation.Commit()
	return task, nil
}

func startOptions(
	active *run,
	reviewThreadID string,
	systemPrompt prompts.SystemPromptComposition,
	onVerified func([]agenttool.Mutation, agenttool.Verification),
) agentrun.Options {
	options := runtimeOptions(active.binding, active.task.ID())
	options.ReviewThreadID = strings.TrimSpace(reviewThreadID)
	options.IdleTimeout = appagentruntime.IdleTimeout(active.runtime.Config)
	options.ToolResultMaxBytes = appagentruntime.ToolResultMaxBytes(active.runtime.Config)
	options.SystemPromptLog = systemPrompt
	options.OnMutationsVerified = func(_ context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		onVerified(mutations, verification)
	}
	return options
}

// SubmitCommand targets one exact Project conversation. A command from
// another tab cannot steer, queue into, or abort this binding.
func (service *Service) SubmitCommand(ctx context.Context, binding Binding, command appagentruntime.Command) (agentrun.CommandReceipt, error) {
	var err error
	binding, err = service.ResolveBinding(binding)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	active := service.activeRun(binding)
	if active == nil || active.task == nil || active.task.Finished() {
		return agentrun.CommandReceipt{}, appagentruntime.ErrNoActiveOperation
	}

	options := runtimeOptions(binding, active.task.ID())
	switch command.Kind {
	case agentharness.CommandAbort, agentharness.CommandSteerQueued, agentharness.CommandCancelQueued:
		return active.runtime.ChatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: options,
		})
	case agentharness.CommandSteer, agentharness.CommandFollowUp, agentharness.CommandNextTurn:
		// Prepared below after the durable runtime admits the exact command.
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported AgentChat command %q", agentrun.ErrInvalidCommand, command.Kind)
	}

	prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
		if service.activeRun(binding) != active || active.task.Finished() {
			return agentharness.TurnExecution{}, appagentruntime.ErrContextChanged
		}
		execution, err := service.prepareCommandExecution(prepareCtx, active, command.Input)
		if err != nil {
			return agentharness.TurnExecution{}, err
		}
		if service.activeRun(binding) != active || active.task.Finished() {
			return agentharness.TurnExecution{}, appagentruntime.ErrContextChanged
		}
		return execution, nil
	}
	return active.runtime.ChatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: active.task.Emit, Prepare: prepare, Options: options,
	})
}

func (service *Service) prepareCommandExecution(ctx context.Context, active *run, request chatagent.ChatRequest) (agentharness.TurnExecution, error) {
	runtime, resolved, err := conversationapp.Prepare(ctx, active.runtime, request)
	if err != nil {
		return agentharness.TurnExecution{}, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConversation(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
	)
	if err != nil {
		return agentharness.TurnExecution{}, err
	}
	conversation := conversationapp.ProjectConversation(runtime, resolved)
	options := runtimeOptions(active.binding, active.task.ID())
	options.IdleTimeout = appagentruntime.IdleTimeout(runtime.Config)
	options.ToolResultMaxBytes = appagentruntime.ToolResultMaxBytes(runtime.Config)
	options.SystemPromptLog = systemPrompt
	options.OnMutationsVerified = func(callbackCtx context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		if service.host != nil {
			service.host.OnVerifiedMutations(callbackCtx, "agent_chat_post_run", runtime.VersionService, runtime.Config, mutations, verification)
		}
	}
	options = conversationapp.BindReviewFeedback(options, runtime, resolved)
	return agentharness.TurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.BookService,
		Request: resolved, Options: options,
	}, nil
}

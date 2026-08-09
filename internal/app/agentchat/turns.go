package agentchat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	chatagent "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
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
	turn, err := service.AcceptTurn(ctx, TurnRequest{Binding: binding, ChatRequest: request})
	if err != nil {
		return nil, err
	}
	if turn.Replayed() {
		return turn.Task(), nil
	}
	if err := turn.Start(); err != nil {
		return nil, err
	}
	return turn.Task(), nil
}

// AcceptedTurn is one project-Agent command that crossed durable admission.
// Its caller owns exactly one of Start or Wait. Start is used by interactive
// AgentChat; Automation already owns a Task worker and calls Wait from it.
type AcceptedTurn struct {
	service      *Service
	active       *run
	accepted     *agentharness.AcceptedRun
	task         *apptask.Task
	runtime      conversationapp.Runtime
	conversation *agentconversation.SessionConversation
	replayed     bool

	mutationMu        sync.Mutex
	verifiedMutations []agenttool.Mutation
	verification      agenttool.Verification
}

func (turn *AcceptedTurn) Task() *apptask.Task {
	if turn == nil {
		return nil
	}
	return turn.task
}

func (turn *AcceptedTurn) Receipt() agentrun.CommandReceipt {
	if turn == nil || turn.accepted == nil {
		return agentrun.CommandReceipt{}
	}
	return turn.accepted.Receipt()
}

func (turn *AcceptedTurn) Replayed() bool {
	return turn != nil && turn.replayed
}

func (turn *AcceptedTurn) Start() error {
	if turn == nil || turn.task == nil {
		return fmt.Errorf("accepted AgentChat turn is unavailable")
	}
	if turn.replayed {
		return nil
	}
	if err := turn.task.Start(func(ctx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		turn.Wait(ctx)
	}); err != nil {
		turn.task.Abort()
		turn.Wait(turn.task.Context())
		turn.task.Finish()
		return err
	}
	return nil
}

// Wait settles the accepted project-Agent command and publishes verified
// mutations through the same host callback used by interactive AgentChat.
func (turn *AcceptedTurn) Wait(ctx context.Context) agentrun.Outcome {
	if turn == nil || turn.accepted == nil || turn.active == nil {
		err := fmt.Errorf("accepted AgentChat turn is unavailable")
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	defer turn.service.releaseActiveRun(turn.active)
	binding := turn.active.binding
	slog.InfoContext(ctx, fmt.Sprintf(
		"[app/agentchat] run begin task_id=%s project_id=%s workspace=%q session_id=%s message_len=%d origin=%s",
		turn.task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID,
		len(turn.active.request.Message), turn.active.policy.Origin,
	))
	outcome := turn.accepted.Wait(ctx)
	_, outputCommitted := turn.conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
	postSettlementCtx := ctx
	if outputCommitted {
		postSettlementCtx = context.WithoutCancel(ctx)
	}
	turn.mutationMu.Lock()
	mutations := append([]agenttool.Mutation(nil), turn.verifiedMutations...)
	verification := turn.verification
	turn.mutationMu.Unlock()
	if outputCommitted && len(mutations) > 0 && turn.service.host != nil {
		turn.service.host.OnVerifiedMutations(postSettlementCtx, "agent_chat_post_run", turn.runtime.VersionService, turn.runtime.Config, mutations, verification)
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[app/agentchat] run end task_id=%s project_id=%s workspace=%q session_id=%s status=%s origin=%s",
		turn.task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID,
		turn.task.Status(), turn.active.policy.Origin,
	))
	return outcome
}

// AcceptTurn is the single project-conversation admission seam. It resolves
// identity, applies an optional invocation ceiling, creates the project Agent,
// and crosses durable StartTurn acceptance without starting a second worker.
func (service *Service) AcceptTurn(ctx context.Context, input TurnRequest) (*AcceptedTurn, error) {
	service.admission.Lock()
	defer service.admission.Unlock()

	var err error
	binding, err := service.ResolveBinding(input.Binding)
	if err != nil {
		return nil, err
	}
	request := input.ChatRequest
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
		if input.Task != nil && input.Task != replay {
			return nil, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, request.CommandID)
		}
		return &AcceptedTurn{service: service, task: replay, replayed: true}, nil
	}
	busyPolicy := input.Policy.BusyPolicy
	if busyPolicy == "" {
		busyPolicy = TurnBusyReject
	}
	if busyPolicy != TurnBusyReject && busyPolicy != TurnBusyWait {
		return nil, fmt.Errorf("unsupported AgentChat busy policy %q", input.Policy.BusyPolicy)
	}
	for {
		active := service.activeRun(binding)
		if active == nil || active.task == nil || active.task.Finished() {
			break
		}
		switch busyPolicy {
		case TurnBusyReject:
			return nil, appagentruntime.ErrOperationActive
		case TurnBusyWait:
			slog.InfoContext(ctx, fmt.Sprintf(
				"[app/agentchat] turn waiting for conversation owner project_id=%s session_id=%s task_id=%s",
				binding.ProjectID, binding.SessionID, active.task.ID(),
			))
			select {
			case <-active.task.Done():
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("resolve AgentChat Project runtime: %w", err)
	}
	sess, created, err := getOrCreateConversation(project, binding)
	if err != nil {
		return nil, fmt.Errorf("open AgentChat conversation: %w", err)
	}
	if created {
		if title := strings.TrimSpace(input.Policy.SessionTitle); title != "" {
			if err := sess.Rename(title); err != nil {
				return nil, fmt.Errorf("name AgentChat conversation: %w", err)
			}
		}
	}
	runtime, request, err := conversationapp.Prepare(ctx, project.conversation(sess), request)
	if err != nil {
		return nil, fmt.Errorf("prepare AgentChat turn: %w", err)
	}
	if err := applyTurnPolicy(&runtime, input.Policy); err != nil {
		return nil, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConversation(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
	)
	if err != nil {
		return nil, fmt.Errorf("build AgentChat Project Agent: %w", err)
	}
	conversation := conversationapp.ProjectConversation(runtime, request)

	task := input.Task
	createdTask := false
	if task == nil {
		task, err = apptask.NewDeferredWithContext(ctx, nil)
		if err != nil {
			return nil, err
		}
		createdTask = true
	}
	emit := input.Emit
	if emit == nil {
		emit = task.Emit
	}
	active := &run{binding: binding, commandID: request.CommandID, task: task, runtime: runtime, request: request, policy: input.Policy}
	if err := service.installActiveRun(active); err != nil {
		if createdTask {
			task.RejectStart(err)
		}
		return nil, err
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		if createdTask {
			task.RejectStart(err)
		}
		service.releaseActiveRun(active)
		return nil, err
	}
	turn := &AcceptedTurn{
		service: service, active: active, task: task, runtime: runtime, conversation: conversation,
	}

	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	options := startOptions(active, request.ResolvedReviewFeedback.PrimaryReviewThreadID(), systemPrompt, func(mutations []agenttool.Mutation, verification agenttool.Verification) {
		turn.mutationMu.Lock()
		turn.verifiedMutations = append([]agenttool.Mutation(nil), mutations...)
		turn.verification = verification
		turn.mutationMu.Unlock()
	})
	options = conversationapp.BindReviewFeedback(options, runtime, request)
	accepted, err := runtime.ChatService.StartWithOptions(
		acceptCtx, runner, conversation, runtime.BookService, request, options, emit,
	)
	releaseAcceptance()
	if err != nil {
		reservation.Rollback()
		if createdTask {
			task.RejectStart(err)
		}
		service.releaseActiveRun(active)
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return nil, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, request.CommandID)
		}
		return nil, err
	}
	reservation.Commit()
	turn.accepted = accepted
	return turn, nil
}

func applyTurnPolicy(runtime *conversationapp.Runtime, policy TurnPolicy) error {
	if runtime == nil {
		return fmt.Errorf("project conversation runtime is nil")
	}
	if profileID := strings.TrimSpace(policy.ModelProfileID); profileID != "" {
		current := config.ResolveAgentModel(&runtime.Config, runtime.AgentKind)
		if err := config.ApplyAgentModelSelection(&runtime.Config, runtime.AgentKind, profileID, current.ThinkingLevel); err != nil {
			return fmt.Errorf("apply project Agent model override: %w", err)
		}
	}
	if len(policy.DisabledCapabilities) == 0 {
		return nil
	}
	resolved := config.ResolveAgentTools(&runtime.Config, runtime.AgentKind)
	override := make(config.AgentToolOverride, len(config.AgentToolCapabilities()))
	for _, capability := range config.AgentToolCapabilities() {
		override[capability.Source] = resolved.Allows(capability.Source)
	}
	for _, capability := range policy.DisabledCapabilities {
		override[strings.TrimSpace(capability)] = false
	}
	switch runtime.AgentKind {
	case agentrun.AgentKindIDE:
		runtime.Config.AgentTools.IDE = override
	case agentrun.AgentKindGeneral:
		runtime.Config.AgentTools.General = override
	default:
		return fmt.Errorf("unsupported project Agent kind %q", runtime.AgentKind)
	}
	return nil
}

func startOptions(
	active *run,
	reviewThreadID string,
	systemPrompt prompts.SystemPromptComposition,
	onVerified func([]agenttool.Mutation, agenttool.Verification),
) agentrun.Options {
	traceID := strings.TrimSpace(active.policy.TraceID)
	if traceID == "" {
		traceID = active.task.ID()
	}
	options := runtimeOptions(active.binding, traceID)
	options.AutomationTaskID = strings.TrimSpace(active.policy.OriginID)
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
	if err := applyTurnPolicy(&runtime, active.policy); err != nil {
		return agentharness.TurnExecution{}, err
	}
	runner, systemPrompt, err := appagentruntime.BuildConversation(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
	)
	if err != nil {
		return agentharness.TurnExecution{}, err
	}
	conversation := conversationapp.ProjectConversation(runtime, resolved)
	options := startOptions(active, resolved.ResolvedReviewFeedback.PrimaryReviewThreadID(), systemPrompt, func(mutations []agenttool.Mutation, verification agenttool.Verification) {
		if service.host != nil {
			service.host.OnVerifiedMutations(context.WithoutCancel(ctx), "agent_chat_post_run", runtime.VersionService, runtime.Config, mutations, verification)
		}
	})
	options = conversationapp.BindReviewFeedback(options, runtime, resolved)
	return agentharness.TurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.BookService,
		Request: resolved, Options: options,
	}, nil
}

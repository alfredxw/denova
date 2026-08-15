package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	appagentruntime "denova/internal/app/agentruntime"
)

func (a *App) StartTask(ctx context.Context, req agentchat.ChatRequest) *apptask.Task {
	return a.chat().StartTask(ctx, req)
}

func (s *ChatAppService) StartTask(ctx context.Context, req agentchat.ChatRequest) *apptask.Task {
	task, err := s.StartTaskWithError(ctx, req)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-task] 准备 IDE Agent 运行时失败 err=%v", err))
		return nil
	}
	return task
}

// StartTaskWithError preserves preparation failures so HTTP callers can
// distinguish invalid review references from a missing workspace.
func (a *App) StartTaskWithError(ctx context.Context, req agentchat.ChatRequest) (*apptask.Task, error) {
	return a.chat().StartTaskWithError(ctx, req)
}

func (s *ChatAppService) StartTaskWithError(ctx context.Context, req agentchat.ChatRequest) (*apptask.Task, error) {
	return s.startTaskWithError(ctx, "", req)
}

// StartTaskForSessionWithError binds a new Writing turn to the exact Session
// displayed by the caller. The admission lock makes this check atomic with
// create, switch, and delete operations.
func (a *App) StartTaskForSessionWithError(ctx context.Context, sessionID string, req agentchat.ChatRequest) (*apptask.Task, error) {
	return a.chat().StartTaskForSessionWithError(ctx, sessionID, req)
}

func (s *ChatAppService) StartTaskForSessionWithError(ctx context.Context, sessionID string, req agentchat.ChatRequest) (*apptask.Task, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrInvalidAgentBinding
	}
	return s.startTaskWithError(ctx, sessionID, req)
}

func (s *ChatAppService) startTaskWithError(ctx context.Context, expectedSessionID string, req agentchat.ChatRequest) (*apptask.Task, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	req.CommandID = strings.TrimSpace(req.CommandID)
	if req.CommandID == "" {
		return nil, ErrAgentCommandIDRequired
	}
	if err := agentrun.ValidateCommandID(req.CommandID); err != nil {
		return nil, err
	}
	req = agentchat.CaptureChatRequestCallerInput(req)
	requestFingerprint := agentexecution.RequestSemanticFingerprint(req)
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	sessionID := ""
	if a.session != nil {
		sessionID = a.session.ID
	}
	a.mu.RUnlock()
	if expectedSessionID != "" && sessionID != expectedSessionID {
		return nil, ErrAgentContextChanged
	}
	if workspace == "" || sessionID == "" {
		return nil, ErrNoWorkspace
	}
	if replay, ok, err := s.starts.Replay(agentStartIdentity(req.CommandID, workspace, sessionID, requestFingerprint)); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	a.mu.RLock()
	activeBeforeStart := activeWritingTaskLocked(a)
	a.mu.RUnlock()
	if activeBeforeStart != nil && !activeBeforeStart.Finished() {
		return nil, ErrAgentOperationActive
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	ctx = operation.Context()
	runtime, req, err := s.prepareIDEChatRuntime(ctx, req)
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	transitioning := a.workspaceTransition
	contextChanged := a.workspace != runtime.workspace || a.session != runtime.sess
	operationActive := a.activeTask != nil && !a.activeTask.Finished()
	a.mu.RUnlock()
	if transitioning {
		return nil, ErrWorkspaceTransition
	}
	if contextChanged {
		return nil, ErrAgentContextChanged
	}
	if operationActive {
		return nil, ErrAgentOperationActive
	}

	agentHost, err := a.HarnessAgentHostCapabilities(ctx, &runtime.cfg, agentrun.AgentKindIDE)
	if err != nil {
		return nil, err
	}
	builtAgent, err := appagentruntime.BuildConversationAgent(
		ctx, &runtime.cfg, runtime.state, runtime.ideTeller, agentrun.AgentKindIDE,
		agentHost,
	)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-task] failed to refresh Agent runtime workspace=%s err=%v", runtime.workspace, err))
		return nil, err
	}
	systemPrompt := builtAgent.Composition
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, req.IDEContext)
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess,
		&runtime.cfg,
		config.AgentKindIDE,
		runtimeContexts.StableTitle,
		runtimeContexts.Stable,
		runtimeContexts.DynamicTitle,
		runtimeContexts.Dynamic,
	).WithInputVisibility(req.InputVisibility)
	var verifiedMutations []agenttool.Mutation
	var postRunVerification agenttool.Verification
	mutationCallback := a.verifiedWorkspaceMutationCallback(
		"ide_agent_post_run",
		runtime.versionService,
		versionAutoSettingsForConfig(&runtime.cfg),
	)
	var accepted *agentexecution.Operation
	runAccepted := func(ctx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		slog.InfoContext(ctx, fmt.Sprintf("[agent-task] run begin id=%s session_id=%s message_len=%d references=%d lore_references=%d style_scenes=%d style_rules=%d selections=%d plan_mode=%v teller_id=%s writing_skill=%s", task.ID(), runtime.sess.ID, len(req.Message), len(req.References), len(req.LoreReferences), len(req.StyleScenes), len(req.StyleRules), len(req.Selections), req.PlanMode, req.TellerID, req.WritingSkill))
		accepted.Wait(ctx)
		_, outputCommitted := conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
		postSettlementCtx := ctx
		if outputCommitted {
			// A durable domain receipt outlives a late caller cancellation. Keep
			// its required projections and mutation hooks consistent with the
			// committed cycle while the workspace task lease is still held.
			postSettlementCtx = context.WithoutCancel(ctx)
		}
		// The durable runtime owns the canonical precedence rule: once an output
		// receipt is acknowledged, late adapter/display errors cannot roll it back.
		// App projections therefore follow the receipt instead of compensating
		// with a second, potentially divergent outcome-status check.
		cycleCommitted := outputCommitted
		if cycleCommitted && len(verifiedMutations) > 0 {
			mutationCallback(postSettlementCtx, verifiedMutations, postRunVerification)
		}
		slog.InfoContext(ctx, fmt.Sprintf("[agent-task] run end id=%s session_id=%s status=%s", task.ID(), runtime.sess.ID, task.Status()))
	}

	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != runtime.workspace || a.session != runtime.sess {
			return ErrAgentContextChanged
		}
		if a.activeTask != nil && !a.activeTask.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, runtime.workspace, true); err != nil {
			return err
		}
		a.activeTask = task
		a.activeWritingRun = &writingTaskRun{task: task, runtime: runtime}
		return nil
	})
	if err != nil {
		return nil, err
	}
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(ctx, task)
	startOptions := runtime.agentOptions(task.ID())
	startOptions.IdleTimeout = appagentruntime.IdleTimeout(runtime.cfg)
	startOptions.ToolResultMaxBytes = appagentruntime.ToolResultMaxBytes(runtime.cfg)
	startOptions.SystemPromptLog = systemPrompt
	startOptions.OnMutationsVerified = func(_ context.Context, mutations []agenttool.Mutation, verification agenttool.Verification) {
		verifiedMutations = append([]agenttool.Mutation(nil), mutations...)
		postRunVerification = verification
	}
	startOptions = s.bindReviewFeedbackInputCommit(startOptions, runtime, req)
	accepted, err = runtime.executionRuntime.Start(acceptCtx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Definition:   builtAgent.Definition,
			Conversation: conversation,
			BookService:  runtime.bookService,
			Request:      req,
			Options:      startOptions,
		},
		Emit: task.Emit,
	})
	releaseAcceptance()
	if err != nil {
		task.RejectStart(err)
		a.unregisterWorkspaceTask(task)
		a.mu.Lock()
		if a.activeTask == task {
			a.activeTask = nil
		}
		if a.activeWritingRun != nil && a.activeWritingRun.task == task {
			a.activeWritingRun = nil
		}
		a.mu.Unlock()
		return nil, err
	}
	if err := task.Start(runAccepted); err != nil {
		task.Abort()
		_ = accepted.Wait(task.Context())
		task.Finish()
		a.unregisterWorkspaceTask(task)
		a.mu.Lock()
		if a.activeTask == task {
			a.activeTask = nil
		}
		if a.activeWritingRun != nil && a.activeWritingRun.task == task {
			a.activeWritingRun = nil
		}
		a.mu.Unlock()
		return nil, err
	}
	if err := s.starts.Remember(agentStartRecord(
		req.CommandID, runtime.workspace, runtime.sess.ID, requestFingerprint, task,
	)); err != nil {
		// The command is already durable at this point. Keep the original Task
		// alive and surface the registry invariant instead of starting another.
		return nil, err
	}

	return task, nil
}

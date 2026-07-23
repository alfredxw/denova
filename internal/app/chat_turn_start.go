package app

import (
	"context"
	"log"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/book"
)

func (a *App) StartTask(ctx context.Context, req agent.ChatRequest) *Task {
	return a.chat().StartTask(ctx, req)
}

func (s *ChatAppService) StartTask(ctx context.Context, req agent.ChatRequest) *Task {
	task, err := s.StartTaskWithError(ctx, req)
	if err != nil {
		log.Printf("[agent-task] 准备 IDE Agent 运行时失败 err=%v", err)
		return nil
	}
	return task
}

// StartTaskWithError preserves preparation failures so HTTP callers can
// distinguish invalid review references from a missing workspace.
func (a *App) StartTaskWithError(ctx context.Context, req agent.ChatRequest) (*Task, error) {
	return a.chat().StartTaskWithError(ctx, req)
}

func (s *ChatAppService) StartTaskWithError(ctx context.Context, req agent.ChatRequest) (*Task, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	req.CommandID = strings.TrimSpace(req.CommandID)
	if req.CommandID == "" {
		return nil, ErrAgentCommandIDRequired
	}
	if err := agentruntime.ValidateCommandID(req.CommandID, agentruntime.DefaultInputLimits()); err != nil {
		return nil, err
	}
	req = agent.CaptureChatRequestCallerInput(req)
	requestFingerprint := agent.ChatRequestSemanticFingerprint(req)
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	sessionID := ""
	selected := a.session
	if a.session != nil {
		sessionID = a.session.ID
	}
	a.mu.RUnlock()
	if workspace == "" || sessionID == "" {
		return nil, ErrNoWorkspace
	}
	if replay, ok, err := s.starts.replay(req.CommandID, workspace, sessionID, requestFingerprint); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	// Recovery Task lifetime and this admission mutex together linearize the
	// stale-Session fence. A structural recovery marks its refresh obligation
	// before it can stop running and remains alive while that obligation is
	// unresolved, so Start must reject it before checking/clearing pending state.
	a.mu.RLock()
	activeBeforeRefresh := activeWritingTaskLocked(a)
	a.mu.RUnlock()
	if activeBeforeRefresh != nil && !activeBeforeRefresh.Finished() {
		return nil, ErrAgentOperationActive
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	ctx = operation.Context()
	if refreshErr := s.retryPendingWritingRecoveryRefresh(ctx, workspace, selected); refreshErr != nil {
		return nil, refreshErr
	}
	if replay, matched, err := s.replayDurableWritingStart(ctx, req, workspace, sessionID, requestFingerprint); err != nil {
		return nil, err
	} else if matched {
		return replay, nil
	}
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

	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		log.Printf("[agent-task] 刷新 Agent Runner 失败 workspace=%s err=%v", runtime.workspace, err)
		return nil, err
	}
	var beforeVersionState book.VersionWorkspaceState
	var hasBeforeVersionState bool
	if runtime.versionService != nil {
		state, err := runtime.versionService.CaptureState()
		if err != nil {
			log.Printf("[versions] 捕获 Agent 运行前状态失败 workspace=%s err=%v", runtime.workspace, err)
		} else {
			beforeVersionState = state
			hasBeforeVersionState = true
		}
	}
	runtimeContexts := agent.IDEWorkspaceRuntimeContextsForRequest(runtime.state, req)
	conversation := agent.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess,
		&runtime.cfg,
		config.AgentKindIDE,
		runtimeContexts.StableTitle,
		runtimeContexts.Stable,
		runtimeContexts.DynamicTitle,
		runtimeContexts.Dynamic,
	)
	var verifiedMutations []agent.ToolMutation
	var postRunVerification agent.PostRunVerification
	mutationCallback := a.automationMutationCallback("ide_agent_post_run")
	var accepted *agent.AcceptedRun
	runAccepted := func(ctx context.Context, task *Task, emit func(agent.Event)) {
		defer a.unregisterWorkspaceTask(task)
		log.Printf("[agent-task] run begin id=%s message_len=%d references=%d lore_references=%d style_scenes=%d style_rules=%d selections=%d plan_mode=%v teller_id=%s writing_skill=%s", task.ID(), len(req.Message), len(req.References), len(req.LoreReferences), len(req.StyleScenes), len(req.StyleRules), len(req.Selections), req.PlanMode, req.TellerID, req.WritingSkill)
		outcome := accepted.Wait(ctx)
		_, inputCommitted := conversation.LastAgentCycleCommitReceipt(agent.HarnessDomainCommitInput)
		_, outputCommitted := conversation.LastAgentCycleCommitReceipt(agent.HarnessDomainCommitOutput)
		postSettlementCtx := ctx
		if inputCommitted || outputCommitted {
			// A durable domain receipt outlives a late caller cancellation. Keep
			// its required projections and mutation hooks consistent with the
			// committed cycle while the workspace task lease is still held.
			postSettlementCtx = context.WithoutCancel(ctx)
		}
		if inputCommitted && !req.ResolvedReviewFeedback.Empty() {
			if err := s.consumeResolvedReviewFeedback(postSettlementCtx, runtime, req); err != nil {
				log.Printf("[reviews] Agent input 已提交但消费评审反馈失败 workspace=%s task_id=%s err=%v", runtime.workspace, task.ID(), err)
				emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			} else {
				emit(agent.Event{Type: "workspace_change", Data: map[string]interface{}{
					"workspace": runtime.workspace, "review_thread_id": req.ResolvedReviewFeedback.PrimaryReviewThreadID(),
					"action": "review_feedback_consumed",
				}})
			}
		}
		// The durable runtime owns the canonical precedence rule: once an output
		// receipt is acknowledged, late adapter/display errors cannot roll it back.
		// App projections therefore follow the receipt instead of compensating
		// with a second, potentially divergent outcome-status check.
		cycleCommitted := outputCommitted
		if cycleCommitted && len(verifiedMutations) > 0 {
			mutationCallback(postSettlementCtx, verifiedMutations, postRunVerification)
		}
		if runtime.versionService != nil && hasBeforeVersionState && cycleCommitted {
			settings := book.DefaultVersionAutoSettings()
			settings.TimedEnabled = runtime.cfg.VersionTimedEnabled
			settings.TimedIntervalMinutes = runtime.cfg.VersionTimedIntervalMinutes
			settings.AgentEnabled = runtime.cfg.VersionAgentEnabled
			settings.AgentCharThreshold = runtime.cfg.VersionAgentCharThreshold
			result, err := runtime.versionService.MaybeCreateAgent(beforeVersionState, settings)
			if err != nil {
				log.Printf("[versions] Agent 自动保存失败 workspace=%s err=%v", runtime.workspace, err)
			} else if result.Skipped {
				log.Printf("[versions] Agent 自动保存跳过 workspace=%s reason=%q chars=%d", runtime.workspace, result.Reason, result.Chars)
			} else if result.Version != nil {
				log.Printf("[versions] Agent 自动保存完成 workspace=%s version=%s chars=%d", runtime.workspace, result.Version.ID, result.Chars)
			}
		} else if runtime.versionService != nil && hasBeforeVersionState {
			log.Printf("[versions] Agent 自动保存跳过：cycle 未形成合法 output receipt workspace=%s task_id=%s outcome=%s input_committed=%t output_committed=%t", runtime.workspace, task.ID(), outcome.Status, inputCommitted, outputCommitted)
		}
		log.Printf("[agent-task] run end id=%s status=%s", task.ID(), task.Status())
	}

	task, err := NewDeferredRegisteredTask(func(task *Task) error {
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
		a.agentRunner = runner
		a.activeTask = task
		a.activeWritingRun = &writingTaskRun{task: task, runtime: runtime}
		return nil
	})
	if err != nil {
		return nil, err
	}
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = runtime.chatService.StartWithOptions(acceptCtx, runner, conversation, runtime.bookService, req, agent.RunOptions{
		AgentKind:          agent.AgentKindIDE,
		TaskID:             task.ID(),
		SessionID:          runtime.sess.ID,
		ReviewThreadID:     req.ResolvedReviewFeedback.PrimaryReviewThreadID(),
		Workspace:          runtime.workspace,
		Mode:               "ide",
		IdleTimeout:        agentIdleTimeout(runtime.cfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(runtime.cfg),
		SystemPromptLog:    systemPrompt,
		OnMutationsVerified: func(_ context.Context, mutations []agent.ToolMutation, verification agent.PostRunVerification) {
			verifiedMutations = append([]agent.ToolMutation(nil), mutations...)
			postRunVerification = verification
		},
	}, task.emit)
	releaseAcceptance()
	if err != nil {
		task.failBeforeStart(err)
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
		_ = accepted.Wait(task.ctx)
		task.finish()
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
	if err := s.starts.remember(writingStartRecord{
		commandID: req.CommandID, workspace: runtime.workspace,
		sessionID: runtime.sess.ID, fingerprint: requestFingerprint, task: task,
	}); err != nil {
		// The command is already durable at this point. Keep the original Task
		// alive and surface the registry invariant instead of starting another.
		return nil, err
	}

	return task, nil
}

func agentIdleTimeout(cfg config.Config) time.Duration {
	if cfg.AgentIdleTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(cfg.AgentIdleTimeoutSeconds) * time.Second
}

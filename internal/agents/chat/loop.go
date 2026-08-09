package chat

import (
	"context"
	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"errors"
	"fmt"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"

	agentplan "denova/internal/agents/plan"
	producttools "denova/internal/agents/tools"
)

type chatLoopAction uint8

const (
	chatLoopContinue chatLoopAction = iota
	chatLoopStop
	chatLoopTerminal
)

type chatLoopResult struct {
	action  chatLoopAction
	outcome agentrun.Outcome
}

type chatAgentLoop struct {
	run           *chatRun
	ctx           context.Context
	cancel        context.CancelFunc
	events        *agent.AsyncIterator[*agent.AgentEvent]
	watcherDone   <-chan struct{}
	planParser    *agentplan.Parser
	finishedTools map[string]struct{}
}

func newChatAgentLoop(run *chatRun, history []*agent.Message, agentMessage string) *chatAgentLoop {
	runCtx, cancelRun := context.WithCancel(contextWithCompactionController(agentrun.ContextWithObserver(run.traceCtx, run.observer), run.conversation, run.emit))
	if _, bound := agent.InvocationIdentityFromContext(runCtx); !bound {
		runCtx = agent.ContextWithInvocationIdentity(runCtx, agent.InvocationIdentity{RunID: run.runID})
	}
	if provider, ok := run.conversation.(toolArtifactStoreProvider); ok {
		runCtx = agent.ContextWithToolArtifactStore(runCtx, provider.ToolArtifactStore())
	}
	if run.req.PlanMode {
		runCtx = agenttoolruntime.ContextWithToolAccessMode(runCtx, agenttoolruntime.ToolAccessModePlanReadOnly)
	}
	if interaction := newRunAskInteraction(run.conversation, run.options, run.emit); interaction != nil {
		runCtx = producttools.ContextWithAskInteraction(runCtx, interaction)
		runCtx = agenttoolruntime.ContextWithApprovalHost(runCtx, interaction)
	}
	cancelOption, cancelAgent := agent.WithCancel()
	runOptions := []agent.AgentRunOption{cancelOption}
	protocolCancel := run.control.wrapProtocolCancel(cancelAgent)
	if run.options.AgentKind == agentrun.AgentKindInteractiveStory {
		runCtx = agentinteractive.WithTurnCancel(runCtx, protocolCancel)
	} else if agentinteractive.IsDirectorPlanRun(run.options.AgentKind, run.options.MaintenanceTask) {
		runCtx = agentinteractive.WithDirectorPlanCancel(runCtx, protocolCancel)
	}
	loop := &chatAgentLoop{
		run:           run,
		ctx:           runCtx,
		cancel:        cancelRun,
		events:        run.runner.Run(runCtx, history, runOptions...),
		watcherDone:   startRunControlWatcher(runCtx, run.options.Controls, cancelAgent, run.control),
		finishedTools: make(map[string]struct{}),
	}
	if run.req.PlanMode {
		planMeta := agentEventMetadata{
			AgentKind:     run.options.AgentKind,
			RunID:         run.runID,
			AgentName:     run.options.RootAgentName,
			RootAgentName: run.options.RootAgentName,
		}
		if run.options.RootAgentName != "" {
			planMeta.RunPath = []string{run.options.RootAgentName}
		}
		loop.planParser = agentplan.NewParser(planMeta.planMetadata(), planEventEmitter(run.emit))
	}
	run.logger.InfoContext(run.ctx, "run_started", slog.Int("history", len(history)), slog.Int("message_len", len(run.req.Message)), slog.Int("agent_message_len", len(agentMessage)), slog.Bool("plan_mode", run.req.PlanMode), slog.String("writing_skill", run.req.WritingSkill), slog.Int("style_scenes", len(run.req.StyleScenes)), slog.Int("style_rules", len(run.req.StyleRules)))
	return loop
}

func (l *chatAgentLoop) execute() agentrun.Outcome {
	defer func() {
		l.cancel()
		<-l.watcherDone
	}()

	for {
		result := l.next()
		switch result.action {
		case chatLoopContinue:
			continue
		case chatLoopStop:
			return l.complete()
		case chatLoopTerminal:
			return result.outcome
		default:
			panic("unhandled chat loop action")
		}
	}
}

func (l *chatAgentLoop) next() chatLoopResult {
	run := l.run
	if err := run.ctx.Err(); err != nil {
		return l.contextCanceled(err)
	}
	event, ok, waitErr := waitForRunnerEvent(l.ctx, l.events, run.options.IdleTimeout, l.cancel)
	if waitErr != nil {
		return l.waitFailed(waitErr)
	}
	if !ok {
		return chatLoopResult{action: chatLoopStop}
	}
	if event.Action != nil && event.Action.Interrupted != nil {
		return l.runnerFailed(event.Action.Interrupted)
	}
	if event.Err != nil {
		return l.runnerFailed(event.Err)
	}
	if event.Output != nil && event.Output.ToolExecution != nil {
		return l.handleToolExecution(event)
	}
	if event.Output == nil || event.Output.MessageOutput == nil {
		run.logger.WarnContext(run.ctx, "invalid_output_skipped", slog.Bool("output_nil", event.Output == nil), slog.Bool("message_output_nil", event.Output != nil && event.Output.MessageOutput == nil))
		return chatLoopResult{action: chatLoopContinue}
	}
	return l.handleOutput(event)
}

func (l *chatAgentLoop) contextCanceled(err error) chatLoopResult {
	run := l.run
	run.logger.WarnContext(run.ctx, "run_interrupted", slog.String("reason", "context"), slog.String("error_class", agentrun.ErrorClass(err.Error())), slog.Int("generated_bytes", run.fullContent.Len()))
	l.flushPlanOutput()
	generatedBytes := run.fullContent.Len()
	terminalOutcome := run.outcomeFor(agentrun.OutcomeAborted, err, err.Error())
	run.finish("aborted", err.Error(), generatedBytes)
	run.emit(agentrun.Event{Type: "aborted", Data: map[string]string{}})
	return chatLoopResult{action: chatLoopTerminal, outcome: terminalOutcome}
}

func (l *chatAgentLoop) waitFailed(waitErr error) chatLoopResult {
	run := l.run
	l.flushPlanOutput()
	terminalContent, terminalThinking := run.snapshotOutput()
	if run.ctx.Err() != nil {
		err := run.ctx.Err()
		run.logger.WarnContext(run.ctx, "run_interrupted", slog.String("reason", "context"), slog.String("error_class", agentrun.ErrorClass(err.Error())), slog.Int("generated_bytes", len(terminalContent)))
		run.finish("aborted", err.Error(), len(terminalContent))
		run.emit(agentrun.Event{Type: "aborted", Data: map[string]string{}})
		return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomeAborted, err, err.Error(), terminalContent, terminalThinking)}
	}
	l.cancel()
	run.logger.ErrorContext(run.ctx, "run_interrupted", slog.String("reason", "idle_timeout"), slog.String("error_class", agentrun.ErrorClass(waitErr.Error())), slog.Int("generated_bytes", len(terminalContent)))
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, waitErr.Error())
	run.finish("error", waitErr.Error(), len(terminalContent))
	run.emit(agentrun.Event{Type: "error", Data: map[string]string{"message": waitErr.Error()}})
	return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomeFailed, waitErr, waitErr.Error(), terminalContent, terminalThinking)}
}

func (l *chatAgentLoop) runnerFailed(runErr error) chatLoopResult {
	run := l.run
	if errors.Is(runErr, errAutomaticContextCompactionDeferred) {
		run.logger.InfoContext(run.ctx, "automatic_context_compaction_deferred_primary_model")
		run.finish("success", "context_maintenance", run.fullContent.Len())
		outcome := agentrun.NewOutcome(agentrun.OutcomeCompleted, nil, "context_maintenance", "", "")
		outcome.MaintenanceOnly = true
		return chatLoopResult{action: chatLoopTerminal, outcome: outcome}
	}
	if run.control.protocolTriggered() && interactiveTurnCompletedByCancel(runErr, run.options.AgentKind, run.conversation, run.fullContent.Len()) {
		run.logger.InfoContext(run.ctx, "interactive_turn_completed_after_submission", slog.Int("generated_bytes", run.fullContent.Len()))
		return chatLoopResult{action: chatLoopStop}
	}
	if run.control.protocolTriggered() && agentinteractive.DirectorPlanCompletedByCancel(runErr, run.options.AgentKind, run.options.MaintenanceTask) {
		run.logger.InfoContext(run.ctx, "interactive_director_plan_completed_after_submission")
		return chatLoopResult{action: chatLoopStop}
	}
	if reason, retrying := agentinteractive.CompletionRetryFromError(runErr); retrying {
		run.logger.InfoContext(run.ctx, "interactive_completion_retry", slog.String("code", reason.Code), slog.Int("generated_bytes", run.fullContent.Len()))
		return chatLoopResult{action: chatLoopContinue}
	}
	if control, controlled := run.control.controlForCancel(runErr); controlled {
		return l.controlled(control)
	}

	run.logger.ErrorContext(run.ctx, "run_interrupted", slog.String("reason", "runner_error"), slog.String("error_class", agentrun.ErrorClass(runErr.Error())), slog.Int("generated_bytes", run.fullContent.Len()))
	l.flushPlanOutput()
	terminalContent, terminalThinking := run.snapshotOutput()
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, runErr.Error())
	run.finish("error", runErr.Error(), len(terminalContent))
	run.emit(agentrun.Event{Type: "error", Data: map[string]string{"message": runErr.Error()}})
	return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomeFailed, runErr, runErr.Error(), terminalContent, terminalThinking)}
}

func (l *chatAgentLoop) controlled(control agentrun.Control) chatLoopResult {
	run := l.run
	l.flushPlanOutput()
	generatedBytes := run.fullContent.Len()
	finalContent, finalThinking := run.snapshotOutput()
	switch control.Kind {
	case agentrun.ControlPreempt:
		if _, persistErr := appendAssistantIfAny(run.conversation, &run.effectiveContent, &run.effectiveThinking, run.assistantMetadata); persistErr != nil {
			run.logger.ErrorContext(run.ctx, "persist_controlled_assistant_failed", slog.String("error_class", agentrun.ErrorClass(persistErr.Error())), slog.String("control", string(control.Kind)))
			run.finish("error", persistErr.Error(), generatedBytes)
			run.emit(agentrun.Event{Type: "error", Data: map[string]string{"message": fmt.Sprintf("生成结果持久化失败: %v", persistErr)}})
			return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomeFailed, persistErr, persistErr.Error(), finalContent, finalThinking)}
		}
		run.finish("preempted", control.Reason, generatedBytes)
		return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomePreempted, nil, control.Reason, finalContent, finalThinking)}
	case agentrun.ControlAbort:
		run.finish("aborted", control.Reason, generatedBytes)
		run.emit(agentrun.Event{Type: "aborted", Data: map[string]string{"reason": control.Reason}})
		return chatLoopResult{action: chatLoopTerminal, outcome: agentrun.NewOutcome(agentrun.OutcomeAborted, nil, control.Reason, finalContent, finalThinking)}
	default:
		return chatLoopResult{action: chatLoopContinue}
	}
}

func (l *chatAgentLoop) complete() agentrun.Outcome {
	run := l.run
	l.flushPlanOutput()
	generatedBytes := run.fullContent.Len()
	finalContent, finalThinking := run.snapshotOutput()
	if _, persistErr := appendAssistantIfAny(run.conversation, &run.effectiveContent, &run.effectiveThinking, run.assistantMetadata); persistErr != nil {
		run.logger.ErrorContext(run.ctx, "persist_assistant_failed", slog.String("error_class", agentrun.ErrorClass(persistErr.Error())), slog.Int("generated_bytes", generatedBytes))
		run.finish("error", persistErr.Error(), generatedBytes)
		run.emit(agentrun.Event{Type: "run_state", Data: map[string]string{
			"run_id":           run.runID,
			"task_id":          run.options.TaskID,
			"agent_kind":       run.options.AgentKind,
			"session_id":       run.options.SessionID,
			"review_thread_id": run.options.ReviewThreadID,
			"root_agent_name":  run.options.RootAgentName,
			"phase":            "finished",
			"status":           "error",
		}})
		run.emit(agentrun.Event{Type: "error", Data: map[string]string{"message": fmt.Sprintf("生成结果持久化失败: %v", persistErr)}})
		return agentrun.NewOutcome(agentrun.OutcomeFailed, persistErr, persistErr.Error(), finalContent, finalThinking)
	}
	if run.resumeInterruption != nil {
		if err := run.conversation.ResolveInterruption(run.resumeInterruption.ID); err != nil {
			run.logger.ErrorContext(run.ctx, "resolve_interruption_failed", slog.String("interruption_id", run.resumeInterruption.ID), slog.String("error_class", agentrun.ErrorClass(err.Error())))
		}
	}
	observedMutations, mutationWarnings := run.observer.ResolvedMutations()
	run.observer.RecordMutations(observedMutations)
	verification := agenttool.VerifyPostRunMutations(run.bookService, observedMutations)
	verification = agenttoolruntime.ApplyMutationWarnings(run.options, verification, mutationWarnings)
	run.observer.RecordVerification(verification)
	if run.options.OnMutationsVerified != nil && len(observedMutations) > 0 {
		run.options.OnMutationsVerified(run.ctx, observedMutations, verification)
	}
	if verification.Mutations > 0 || len(verification.Warnings) > 0 {
		run.logger.InfoContext(run.ctx, "post_run_verification", slog.String("status", verification.Status), slog.Int("mutations", verification.Mutations), slog.Int("checks", len(verification.Checks)), slog.Any("warnings", verification.Warnings))
		run.emit(agentrun.Event{Type: "post_run_verification", Data: verification})
		run.emit(agentrun.Event{Type: "verification", Data: verification})
	}
	run.logger.InfoContext(run.ctx, "run_completed")
	run.finish("success", "", generatedBytes)
	run.emit(agentrun.Event{Type: "run_state", Data: map[string]string{
		"run_id":           run.runID,
		"task_id":          run.options.TaskID,
		"agent_kind":       run.options.AgentKind,
		"session_id":       run.options.SessionID,
		"review_thread_id": run.options.ReviewThreadID,
		"root_agent_name":  run.options.RootAgentName,
		"phase":            "finished",
		"status":           "success",
	}})
	run.emit(agentrun.Event{Type: "done", Data: map[string]string{}})
	return agentrun.NewOutcome(agentrun.OutcomeCompleted, nil, "", finalContent, finalThinking)
}

func (l *chatAgentLoop) flushPlanOutput() {
	before := l.run.fullAssistantOutputSnapshot()
	flushPlanProtocolParser(l.planParser, &l.run.fullContent, l.run.emit)
	discardPlanAssistantContentIfNeeded(l.run.req.PlanMode, l.planParser, &l.run.fullContent, &l.run.fullThinking)
	l.run.captureEffectiveAssistantDelta(before)
}

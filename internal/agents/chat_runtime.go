package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/observability"
)

// chatRun owns the mutable state of one turnExecutor.Run invocation. Keeping this
// state together makes the public runtime entrypoint a small orchestration
// boundary while the context and event-loop stages remain independently
// readable.
type chatRun struct {
	ctx          context.Context
	runner       *agent.Runner
	conversation Conversation
	bookService  *book.Service
	req          ChatRequest
	options      RunOptions
	emit         func(Event)

	logger    *slog.Logger
	policy    LoopPolicy
	workspace string
	ledger    *RunLedger
	rootSpan  *traceSpanHandle
	runID     string
	traceCtx  context.Context
	observer  *RunObserver
	usage     *runTokenUsageCollector
	finished  bool

	assistantMetadata session.MessageMetadata
	subAgentSessions  *subAgentSessionTracker
	toolContext       *toolResultContextRecorder
	mutations         *mutationTracker
	control           *runControlState

	originalMessage    string
	resumeInterruption *session.Interruption
	fullContent        strings.Builder
	fullThinking       strings.Builder
	capturedContent    string
	capturedThinking   string
}

func newChatRun(
	runtime *turnExecutor,
	ctx context.Context,
	runner *agent.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) *chatRun {
	if emit == nil {
		emit = func(Event) {}
	}
	logger := observability.Logger("agent-run")
	policy := DefaultLoopPolicy()
	if runtime != nil {
		policy = runtime.policy.normalized()
	}
	workspace := ""
	if bookService != nil {
		workspace = bookService.Workspace()
	}
	options = options.normalized(workspace)
	options.SystemPromptLog.logForRun(options)
	ledger, ledgerErr := newRunLedgerWithOptions(workspace, policy.RunLedger, options)
	if ledgerErr != nil {
		logger.Warn("run_ledger_unavailable", slog.String("workspace", workspace), slog.Any("error", ledgerErr))
	}
	rootSpan := StartRootTraceSpan(ledger, map[string]any{
		"workspace":        workspace,
		"task_id":          options.TaskID,
		"agent_kind":       options.AgentKind,
		"session_id":       options.SessionID,
		"review_thread_id": options.ReviewThreadID,
		"story_id":         options.StoryID,
		"branch_id":        options.BranchID,
		"turn_id":          options.TurnID,
		"maintenance_task": options.MaintenanceTask,
		"mode":             options.Mode,
	})
	rootSpanID := ""
	if rootSpan != nil {
		rootSpanID = rootSpan.SpanID()
	}
	runID := ledger.ID()
	if runID == "" {
		runID = options.TaskID
	}

	run := &chatRun{
		ctx:              ctx,
		runner:           runner,
		conversation:     conversation,
		bookService:      bookService,
		req:              req,
		options:          options,
		logger:           logger,
		policy:           policy,
		workspace:        workspace,
		ledger:           ledger,
		rootSpan:         rootSpan,
		runID:            runID,
		traceCtx:         ContextWithRunTrace(ctx, runID, ledger, rootSpanID),
		observer:         newRunObserverWithIdentity(ledger, rootSpanID, runID, options.SessionID, options.ReviewThreadID),
		usage:            newRunTokenUsageCollector(runID, options.AgentKind),
		subAgentSessions: newSubAgentSessionTracker(runID),
		toolContext:      newToolResultContextRecorder(conversation),
		mutations:        newMutationTracker(),
		control:          &runControlState{},
		originalMessage:  req.Message,
	}
	run.assistantMetadata = session.MessageMetadata{
		RunID:         runID,
		AgentKind:     options.AgentKind,
		AgentName:     options.RootAgentName,
		RootAgentName: options.RootAgentName,
	}
	if options.RootAgentName != "" {
		run.assistantMetadata.RunPath = []string{options.RootAgentName}
	}
	recorder := newDisplayEventRecorder(conversation)
	run.emit = func(event Event) {
		if run.control.suppressesStreamCanceledError(event) {
			return
		}
		run.mutations.Observe(event)
		recorder.Record(event)
		if err := run.ledger.RecordEvent(event); err != nil {
			run.logger.Warn("run_ledger_event_failed", slog.String("run_id", run.runID), slog.String("event_type", event.Type), slog.Any("error", err))
		}
		emit(event)
	}
	return run
}

func (r *chatRun) execute() (outcome RunOutcome) {
	if r.ledger != nil {
		defer func() {
			if err := r.ledger.Close(); err != nil {
				r.logger.Warn("run_ledger_close_failed", slog.String("run_id", r.runID), slog.Any("error", err))
			}
		}()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("agent run panic: %v", recovered)
			r.logger.Error("panic_recovered", slog.Any("error", recovered))
			markInterruptionIfNeeded(r.conversation, r.resumeInterruption, r.originalMessage, "", fmt.Sprint(recovered))
			r.finish("panic", fmt.Sprint(recovered), 0)
			r.emit(Event{Type: "error", Data: map[string]string{"message": "Agent 异常中断"}})
			content, thinking := r.currentOutput()
			outcome = outcomeFromOutput(RunOutcomeFailed, panicErr, panicErr.Error(), content, thinking)
		}
	}()

	r.recordStarted()
	history, agentMessage, terminal, done := r.prepareContext()
	if done {
		return terminal
	}
	return newChatAgentLoop(r, history, agentMessage).execute()
}

func (r *chatRun) recordStarted() {
	r.emit(Event{Type: "run_state", Data: map[string]string{
		"run_id":           r.runID,
		"task_id":          r.options.TaskID,
		"agent_kind":       r.options.AgentKind,
		"session_id":       r.options.SessionID,
		"review_thread_id": r.options.ReviewThreadID,
		"story_id":         r.options.StoryID,
		"branch_id":        r.options.BranchID,
		"turn_id":          r.options.TurnID,
		"maintenance_task": r.options.MaintenanceTask,
		"root_agent_name":  r.options.RootAgentName,
		"phase":            "started",
	}})
	if err := r.ledger.Record("run_started", map[string]any{
		"workspace":        r.workspace,
		"task_id":          r.options.TaskID,
		"agent_kind":       r.options.AgentKind,
		"session_id":       r.options.SessionID,
		"review_thread_id": r.options.ReviewThreadID,
		"story_id":         r.options.StoryID,
		"branch_id":        r.options.BranchID,
		"turn_id":          r.options.TurnID,
		"maintenance_task": r.options.MaintenanceTask,
		"mode":             r.options.Mode,
		"message":          textSummary{Bytes: len(r.originalMessage), Chars: len([]rune(r.originalMessage)), Preview: safeLogPreview(r.originalMessage, r.policy.RunLedger.PreviewChars)},
		"references":       len(r.req.References),
		"lore_references":  len(r.req.LoreReferences),
		"style_scenes":     len(r.req.StyleScenes),
		"selections":       len(r.req.Selections),
		"plan_mode":        r.req.PlanMode,
		"writing_skill":    r.req.WritingSkill,
	}); err != nil {
		r.logger.Warn("run_ledger_start_failed", slog.String("run_id", r.runID), slog.Any("error", err))
	}
}

func (r *chatRun) prepareContext() ([]*agent.Message, string, RunOutcome, bool) {
	contextBuildStarted := time.Now()
	turn, err := prepareTurnContext(r.traceCtx, turnContextPreparationInput{
		Conversation: r.conversation,
		Request:      r.req,
		BookService:  r.bookService,
		Environment:  newTurnRuntimeEnvironment(r.options.Workspace),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			r.finish("aborted", err.Error(), 0)
			r.emit(Event{Type: "aborted", Data: map[string]string{"reason": err.Error()}})
			return nil, "", r.outcomeFor(RunOutcomeAborted, err, err.Error()), true
		}
		r.logger.Error("prepare_messages_failed", slog.Any("error", err))
		r.finish("error", err.Error(), 0)
		r.emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return nil, "", r.outcomeFor(RunOutcomeFailed, err, err.Error()), true
	}
	r.originalMessage = turn.OriginalMessage
	r.resumeInterruption = turn.ResumeInterruption
	if err := CommitModelInput(r.traceCtx, r.conversation, r.originalMessage, turn.ModelContext); err != nil {
		r.logger.Error("commit_model_input_failed", slog.Any("error", err))
		r.finish("error", err.Error(), 0)
		r.emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return nil, "", r.outcomeFor(RunOutcomeFailed, err, err.Error()), true
	}
	history := turn.ModelContext.Messages
	agentMessage := finalModelUserMessage(history, r.req.Message)
	contextLog := contextBuildLogFromAssembly(r.policy.ContextLedger, r.originalMessage, turn.ModelContext.Context)
	// Emit only after the durable user input is committed, so restored display
	// history keeps the user message before its deterministic Skill cards. This
	// still happens before compaction and the first Agent model call.
	r.emitExplicitSkillLoads(turn.ExplicitSkills)
	if r.options.OnUserMessageCommitted != nil {
		if err := r.options.OnUserMessageCommitted(r.traceCtx); err != nil {
			r.logger.Error("commit_user_message_side_effect_failed", slog.Any("error", err))
			r.finish("error", err.Error(), 0)
			r.emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return nil, "", r.outcomeFor(RunOutcomeFailed, err, err.Error()), true
		}
		r.emit(Event{Type: "workspace_change", Data: map[string]interface{}{
			"workspace":        r.options.Workspace,
			"review_thread_id": r.options.ReviewThreadID,
			"action":           "review_feedback_consumed",
		}})
	}
	history, terminal, done := r.compactContext(history)
	if done {
		return nil, "", terminal, true
	}

	contextLedgerParts := contextLedgerPartsForConversation(contextLog, r.conversation, history)
	if err := r.ledger.RecordContext(contextLedgerParts); err != nil {
		r.logger.Warn("run_ledger_context_failed", slog.String("run_id", r.runID), slog.Any("error", err))
	}
	RecordCompletedTraceSpan(r.traceCtx, "context_build", contextBuildStarted, "success", map[string]any{
		"history_messages":    len(history),
		"context_parts":       len(contextLedgerParts),
		"message_chars":       len([]rune(r.originalMessage)),
		"agent_message_chars": len([]rune(agentMessage)),
		"plan_mode":           r.req.PlanMode,
		"writing_skill":       r.req.WritingSkill,
	})
	r.logger.Info(
		"context_composition",
		slog.String("history", messageListSummary(history)),
		slog.String("original", promptPartSummary(r.originalMessage)),
		slog.String("agent_message", promptPartSummary(agentMessage)),
		slog.String("references", stringListSummary(r.req.References)),
		slog.String("lore_references", stringListSummary(r.req.LoreReferences)),
		slog.String("style_scenes", stringListSummary(r.req.StyleScenes)),
		slog.Int("style_rules", len(r.req.StyleRules)),
		slog.String("selections", selectionListSummary(r.req.Selections)),
		slog.Bool("plan_mode", r.req.PlanMode),
		slog.String("writing_skill", r.req.WritingSkill),
		slog.Bool("resumed", r.resumeInterruption != nil),
	)
	r.logger.Info("context_sources", slog.String("summary", contextLog.String()), slog.Any("sources", contextLog.Audit()))
	if reporter, ok := r.conversation.(ContextSourceReporter); ok {
		if sources := strings.TrimSpace(reporter.ContextSourceSummary()); sources != "" {
			r.logger.Info("conversation_context_sources", slog.String("sources", sources))
		}
	}
	return history, agentMessage, RunOutcome{}, false
}

func finalModelUserMessage(messages []*agent.Message, fallback string) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == agent.User {
			return messages[index].Content
		}
	}
	return fallback
}

func (r *chatRun) compactContext(history []*agent.Message) ([]*agent.Message, RunOutcome, bool) {
	compactor, ok := r.conversation.(ContextCompactionConversation)
	if !ok {
		return history, RunOutcome{}, false
	}
	compactionStarted := time.Now()
	compactedHistory, result, err := compactor.CompactContextIfNeeded(ContextWithRunObserver(r.traceCtx, r.observer), ContextCompactionInput{
		Messages: history,
		Phase:    contextCompactionPhasePreRun,
		Emit:     r.emit,
	})
	if err != nil {
		r.logger.Error("context_compaction_failed", slog.Any("error", err), slog.Int("tokens_before", result.TokensBefore), slog.Int("context_window_tokens", result.ContextWindowTokens))
		r.finish("error", err.Error(), 0)
		r.emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return nil, r.outcomeFor(RunOutcomeFailed, err, err.Error()), true
	}
	if !result.Triggered {
		return compactedHistory, RunOutcome{}, false
	}
	r.logger.Info("context_compacted", slog.String("phase", result.Phase), slog.Int("epoch", result.Epoch), slog.Int("tokens_before", result.TokensBefore), slog.Int("projected_tokens_before", result.ProjectedTokensBefore), slog.Int("tokens_after", result.TokensAfter), slog.Int("projected_tokens_after", result.ProjectedTokensAfter), slog.Int("context_window_tokens", result.ContextWindowTokens))
	if err := r.ledger.Record("context_compaction", map[string]any{
		"phase":                       result.Phase,
		"epoch":                       result.Epoch,
		"tokens_before":               result.TokensBefore,
		"projected_tokens_before":     result.ProjectedTokensBefore,
		"reserved_completion_tokens":  result.ReservedCompletionTokens,
		"reserved_tool_result_tokens": result.ReservedToolResultTokens,
		"tokens_after":                result.TokensAfter,
		"projected_tokens_after":      result.ProjectedTokensAfter,
		"context_window_tokens":       result.ContextWindowTokens,
		"threshold":                   result.Threshold,
	}); err != nil {
		r.logger.Warn("run_ledger_context_compaction_failed", slog.String("run_id", r.runID), slog.Any("error", err))
	}
	RecordCompletedTraceSpan(r.traceCtx, "context_compaction", compactionStarted, "success", map[string]any{
		"phase":                   result.Phase,
		"epoch":                   result.Epoch,
		"tokens_before":           result.TokensBefore,
		"projected_tokens_before": result.ProjectedTokensBefore,
		"tokens_after":            result.TokensAfter,
		"projected_tokens_after":  result.ProjectedTokensAfter,
		"context_window_tokens":   result.ContextWindowTokens,
		"threshold":               result.Threshold,
	})
	return compactedHistory, RunOutcome{}, false
}

func (r *chatRun) finish(status, reason string, generatedBytes int) {
	if r.finished {
		return
	}
	r.finished = true
	r.usage.EmitIfAny(r.emit, generatedBytes)
	traceMetadata := runTraceMetadataForConversation(r.options, r.conversation)
	if !traceMetadata.empty() {
		if err := r.ledger.Record("run_context", traceMetadata.record()); err != nil {
			r.logger.Warn("run_ledger_context_metadata_failed", slog.String("run_id", r.runID), slog.Any("error", err))
		}
	}
	if r.rootSpan != nil {
		r.rootSpan.Finish(status, map[string]any{
			"reason":           strings.TrimSpace(reason),
			"generated_bytes":  generatedBytes,
			"story_id":         traceMetadata.StoryID,
			"branch_id":        traceMetadata.BranchID,
			"turn_id":          traceMetadata.TurnID,
			"maintenance_task": traceMetadata.MaintenanceTask,
		})
	}
	if err := r.ledger.RecordFinish(status, reason, generatedBytes); err != nil {
		r.logger.Warn("run_ledger_finish_failed", slog.String("run_id", r.runID), slog.Any("error", err))
	}
}

func (r *chatRun) snapshotOutput() (string, string) {
	r.capturedContent = r.fullContent.String()
	r.capturedThinking = r.fullThinking.String()
	return r.capturedContent, r.capturedThinking
}

func (r *chatRun) currentOutput() (string, string) {
	if r.fullContent.Len() > 0 || r.fullThinking.Len() > 0 {
		return r.fullContent.String(), r.fullThinking.String()
	}
	return r.capturedContent, r.capturedThinking
}

func (r *chatRun) outcomeFor(status RunOutcomeStatus, err error, reason string) RunOutcome {
	content, thinking := r.currentOutput()
	if r.fullContent.Len() > 0 || r.fullThinking.Len() > 0 {
		content, thinking = r.snapshotOutput()
	}
	return outcomeFromOutput(status, err, reason, content, thinking)
}

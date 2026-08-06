package chat

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

type contextMaintenanceMiddleware struct {
	*agent.BaseMiddleware
	agentKind string
}

// NewContextMaintenanceMiddleware creates the single cleanup-and-compaction
// boundary used before every root model call.
func NewContextMaintenanceMiddleware(agentKind string) agent.Middleware {
	return &contextMaintenanceMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{},
		agentKind:      agentKind,
	}
}

// IsContextMaintenanceMiddleware reports whether middleware owns automatic
// cleanup and compaction. Builders use it only for structural assertions; the
// concrete middleware remains private to this package.
func IsContextMaintenanceMiddleware(middleware agent.Middleware) bool {
	_, ok := middleware.(*contextMaintenanceMiddleware)
	return ok
}

// contextCompactionConversation is the orchestration boundary between the
// model-call middleware and a durable conversation implementation.
type contextCompactionConversation interface {
	CompactContextIfNeeded(context.Context, agentcompaction.Input) ([]*agent.Message, agentcompaction.Result, error)
}

type contextCompactionController struct {
	conversation contextCompactionConversation
	emit         func(agentrun.Event)
}

type contextCompactionContextKey struct{}

func contextWithCompactionController(ctx context.Context, candidate any, emit ...func(agentrun.Event)) context.Context {
	conversation, ok := candidate.(contextCompactionConversation)
	if !ok || conversation == nil {
		return ctx
	}
	controller := &contextCompactionController{conversation: conversation}
	if len(emit) > 0 {
		controller.emit = emit[0]
	}
	return context.WithValue(ctx, contextCompactionContextKey{}, controller)
}

func compactionControllerFromContext(ctx context.Context) *contextCompactionController {
	if ctx == nil {
		return nil
	}
	controller, _ := ctx.Value(contextCompactionContextKey{}).(*contextCompactionController)
	return controller
}

func compactionEventSink(emit func(agentrun.Event)) func(agentcompaction.Event) {
	if emit == nil {
		return nil
	}
	return func(event agentcompaction.Event) {
		emit(agentrun.Event{Type: event.Type, Data: event.Data})
	}
}

var errAutomaticContextCompactionDeferred = errors.New("automatic context compaction deferred the primary model call")

type contextMaintenanceRunStateKey struct{}

// contextMaintenanceRunState is shared by cleanup and compaction planning.
// Exactly one structural context mutation may be selected during an Agent run.
type contextMaintenanceRunState struct {
	mu               sync.Mutex
	structuralChange bool
}

func (state *contextMaintenanceRunState) begin() bool {
	if state == nil {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return !state.structuralChange
}

func (state *contextMaintenanceRunState) commit() {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.structuralChange = true
	state.mu.Unlock()
}

func (m *contextMaintenanceMiddleware) BeforeAgent(ctx context.Context, run *agent.RunContext) (context.Context, *agent.RunContext, error) {
	return context.WithValue(ctx, contextMaintenanceRunStateKey{}, &contextMaintenanceRunState{}), run, nil
}

// BeforeModelCall is the single maintenance seam for both the first model
// request and every post-tool model request. It runs after the final model
// wrapper and call options exist, so the side fork can reuse the real request
// prefix instead of reconstructing a standalone summarizer transcript.
func (m *contextMaintenanceMiddleware) BeforeModelCall(ctx context.Context, call *agent.ModelCall, modelContext *agent.ModelContext) (context.Context, *agent.ModelCall, error) {
	if call == nil || !agent.IsRootInvocation(ctx) {
		return ctx, call, nil
	}
	runState, _ := ctx.Value(contextMaintenanceRunStateKey{}).(*contextMaintenanceRunState)
	if runState == nil || !runState.begin() {
		return ctx, call, nil
	}
	controller := compactionControllerFromContext(ctx)
	if controller == nil || controller.conversation == nil {
		return ctx, call, nil
	}

	next, result, err := prepareContextMaintenance(ctx, controller, call, modelContext)
	if result.Attempted {
		// A failed side fork or cleanup stage is still one maintenance attempt.
		// Do not pay for the same failing structural decision again after a tool
		// call in the current Agent run.
		runState.commit()
	}
	if err != nil {
		if errors.Is(err, errAutomaticContextCompactionDeferred) {
			return ctx, call, err
		}
		// Automatic maintenance must never publish a partial checkpoint. The
		// unchanged primary request still passes through the non-disableable
		// provider input guard, which remains the final hard-limit authority.
		slog.Default().With("component", "agent-run").WarnContext(ctx, "model_step_context_compaction_failed", slog.String("agent_kind", m.agentKind), slog.String("error_class", agentrun.ErrorClass(err.Error())))
		return ctx, call, nil
	}
	return ctx, next, nil
}

// prepareContextMaintenance makes exactly one provider-neutral pressure
// decision against the immutable request snapshot. Storage domains own cleanup
// staging because only they can translate request positions back to canonical
// append-only journal indexes safely.
func prepareContextMaintenance(
	ctx context.Context,
	controller *contextCompactionController,
	call *agent.ModelCall,
	_ *agent.ModelContext,
) (*agent.ModelCall, ContextMaintenanceResult, error) {
	snapshot := call.Snapshot()
	if snapshot == nil {
		return call, ContextMaintenanceResult{}, nil
	}
	tools := snapshot.ResolvedOptions().Tools
	messages := snapshot.Messages()
	pressureConversation, plansPressure := controller.conversation.(ContextPressureConversation)
	var pressureDecision agentcontext.ContextPressureDecision
	plannedObservedPromptTokens := 0
	cleanupAppliedBeforeCompaction := false
	if plansPressure {
		pressurePolicy := pressureConversation.ContextPressurePolicy(messages)
		resolvedOptions := snapshot.ResolvedOptions()
		if resolvedOptions != nil && resolvedOptions.MaxTokens != nil {
			pressurePolicy.CheckpointOutputReserve = max(pressurePolicy.CheckpointOutputReserve, *resolvedOptions.MaxTokens)
		}
		pressurePolicy = agentconversation.WithDynamicPressurePolicy(pressurePolicy, messages)
		executor, executionMode, executorErr := resolveToolResultCleanupExecutor(ctx)
		if executorErr != nil {
			return call, ContextMaintenanceResult{}, executorErr
		}
		pressurePolicy.CleanupExecutionMode = executionMode
		plannedObservedPromptTokens = pressurePolicy.ObservedPromptTokens
		pressureDecision = agentcontext.PlanContextPressure(messages, tools, pressurePolicy)
		if pressureDecision.Action == agentcontext.ContextMaintenanceNone && pressurePolicy.Enabled && agentcompaction.ForkCapacityPressure(
			messages, tools, pressurePolicy, resolvedOptions,
		) {
			pressureDecision.Action = agentcontext.ContextMaintenanceCompaction
			pressureDecision.Reason = "compaction_capacity_reserve"
		}
		if pressureDecision.Action == agentcontext.ContextMaintenanceCompaction &&
			pressureDecision.FullPressure >= 1 && len(pressureDecision.Cleanup.Replacements) > 0 {
			// At a hard overflow, recoverable tool bodies are the first transient
			// projection. If they cannot restore the normal cleanup target, the
			// checkpoint supersedes that projection as the run's sole durable
			// structural operation. This avoids publishing cleanup and compaction
			// records for one model step while still giving compaction the smallest
			// safe primary request.
			emitContextCleanupEvent(controller.emit, "started", pressureDecision, 0, nil)
			cleaned := toolresult.ApplyCleanupPlan(messages, pressureDecision.Cleanup)
			actualReclaimed := max(0, agentcontext.EstimateTokens(messages, tools)-agentcontext.EstimateTokens(cleaned, tools))
			next := *call
			next.Messages = cleaned
			call = &next
			snapshot = call.Snapshot()
			messages = cleaned
			cleanupAppliedBeforeCompaction = true
			emitContextCleanupEvent(controller.emit, "completed", pressureDecision, actualReclaimed, nil)
		}
		switch pressureDecision.Action {
		case agentcontext.ContextMaintenanceNone:
			if pressureDecision.CandidateTokens > 0 || pressureDecision.Pressure >= pressurePolicy.CleanupThreshold ||
				pressureDecision.FullPressure >= pressurePolicy.CleanupThreshold {
				emitContextCleanupEvent(controller.emit, "skipped", pressureDecision, 0, nil)
			}
			return call, ContextMaintenanceResult{Action: agentcontext.ContextMaintenanceNone, Cleanup: pressureDecision}, nil
		case agentcontext.ContextMaintenanceCleanup:
			emitContextCleanupEvent(controller.emit, "started", pressureDecision, 0, nil)
			if err := pressureConversation.StageToolResultCleanup(ctx, messages, pressureDecision.Cleanup); err != nil {
				emitContextCleanupEvent(controller.emit, "failed", pressureDecision, 0, err)
				if pressureDecision.Pressure < pressurePolicy.CompactionThreshold &&
					pressureDecision.FullPressure < pressurePolicy.CompactionThreshold {
					return call, ContextMaintenanceResult{Attempted: true, Action: agentcontext.ContextMaintenanceNone, Cleanup: pressureDecision}, err
				}
				pressureDecision.Action = agentcontext.ContextMaintenanceCompaction
				pressureDecision.Reason = "cleanup_stage_failed_at_compaction_pressure"
				break
			}
			if pressureDecision.Action == agentcontext.ContextMaintenanceCleanup && executor != nil {
				if err := executor.Execute(ctx, snapshot, pressureDecision.Cleanup); err != nil {
					if discarder, ok := controller.conversation.(stagedToolResultCleanupDiscarder); ok {
						discarder.DiscardStagedToolResultCleanup()
					}
					emitContextCleanupEvent(controller.emit, "failed", pressureDecision, 0, err)
					if pressureDecision.Pressure < pressurePolicy.CompactionThreshold &&
						pressureDecision.FullPressure < pressurePolicy.CompactionThreshold {
						return call, ContextMaintenanceResult{Attempted: true, Action: agentcontext.ContextMaintenanceNone, Cleanup: pressureDecision}, err
					}
					pressureDecision.Action = agentcontext.ContextMaintenanceCompaction
					pressureDecision.Reason = "native_cleanup_failed_at_compaction_pressure"
				}
			}
			if pressureDecision.Action == agentcontext.ContextMaintenanceCleanup {
				next := *call
				next.Messages = toolresult.ApplyCleanupPlan(messages, pressureDecision.Cleanup)
				actualReclaimed := max(0, agentcontext.EstimateTokens(messages, tools)-agentcontext.EstimateTokens(next.Messages, tools))
				emitContextCleanupEvent(controller.emit, "completed", pressureDecision, actualReclaimed, nil)
				return &next, ContextMaintenanceResult{Attempted: true, Triggered: true, Action: agentcontext.ContextMaintenanceCleanup, Cleanup: pressureDecision}, nil
			}
		case agentcontext.ContextMaintenanceCompaction:
			// Hard pressure can route directly to compaction because cleanup is
			// below its minimum benefit or would rewrite too much warm suffix.
			// Preserve that skipped-cleanup attribution before the checkpoint
			// event; otherwise the durable ledger cannot explain why cleanup was
			// considered but not selected.
			if !cleanupAppliedBeforeCompaction {
				emitContextCleanupEvent(controller.emit, "skipped", pressureDecision, 0, nil)
			}
			// Continue below. The pressure planner, not the older per-conversation
			// threshold check, owns the cleanup-versus-checkpoint choice.
		default:
			return call, ContextMaintenanceResult{}, fmt.Errorf("unsupported context maintenance action %q", pressureDecision.Action)
		}
	}

	observedPromptTokens, observedEstimateTokens := agentcompaction.LatestPromptUsageCalibration(messages, tools)
	if plannedObservedPromptTokens > observedPromptTokens {
		observedPromptTokens = plannedObservedPromptTokens
		observedEstimateTokens = agentcontext.EstimateTokens(messages, tools)
	}
	candidateFingerprint, candidateGeneration := agentcompaction.CandidateIdentity(messages, 0)
	newMessages, result, err := controller.conversation.CompactContextIfNeeded(ctx, agentcompaction.Input{
		Messages: messages, Tools: tools, Phase: agentcompaction.PhaseModelStep,
		Emit:                   compactionEventSink(controller.emit),
		PrimaryRequestSnapshot: snapshot,
		ObservedPromptTokens:   observedPromptTokens, ObservedEstimateTokens: observedEstimateTokens,
		Planned: plansPressure, Automatic: true, TriggerReason: pressureDecision.Reason,
		CandidateTokens:      pressureDecision.CandidateTokens,
		CandidateFingerprint: candidateFingerprint, CandidateGeneration: candidateGeneration,
	})
	if err != nil || !result.Triggered {
		action := agentcontext.ContextMaintenanceNone
		if plansPressure || err != nil {
			action = agentcontext.ContextMaintenanceCompaction
		}
		maintenance := ContextMaintenanceResult{Attempted: action == agentcontext.ContextMaintenanceCompaction, Action: action, Compaction: result, Cleanup: pressureDecision}
		return call, maintenance, err
	}
	next := *call
	next.Messages = newMessages
	maintenance := ContextMaintenanceResult{Attempted: true, Triggered: true, Action: agentcontext.ContextMaintenanceCompaction, Compaction: result, Cleanup: pressureDecision}
	if result.Degraded {
		if controller.emit != nil {
			controller.emit(agentrun.Event{Type: "context_compaction", Data: map[string]any{
				"phase": result.Phase, "status": "continuation_deferred",
				"reason": "degraded_checkpoint_requires_durable_publish",
			}})
		}
		return call, maintenance, errAutomaticContextCompactionDeferred
	}
	return &next, maintenance, nil
}

func contextCompactionRecordFromResult(result agentcompaction.Result, agentKind string, sourceStart, sourceEnd, retainedTurns int, summary string) session.ContextCompaction {
	result.Summary = summary
	result.RetainedTurns = retainedTurns
	result.TriggerReason = agentcompaction.NormalizeTriggerReason(result.TriggerReason, result.Phase)
	return session.ContextCompaction{
		Type:                 "context_compaction",
		CompactionCheckpoint: agentcompaction.NewCheckpoint(agentKind, result),
		SourceStartIndex:     sourceStart,
		SourceEndIndex:       sourceEnd,
		SourceMessageCount:   sourceEnd - sourceStart,
		CreatedAt:            time.Now().UTC(),
	}
}

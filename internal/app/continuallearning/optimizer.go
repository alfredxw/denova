package continuallearning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	chatagent "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttools "denova/internal/agents/tools"
	"denova/internal/agents/trajectory"
	appagentruntime "denova/internal/app/agentruntime"
	appsettings "denova/internal/app/settings"
	apptask "denova/internal/app/task"

	agentstate "github.com/alfredxw/denova/agent/state"
)

type optimizerRestoreData struct {
	DraftID      string `json:"draft_id"`
	BaseRevision string `json:"base_revision"`
	Summary      string `json:"summary"`
	Trigger      string `json:"trigger"`
}

type managedDraft struct {
	mu      sync.Mutex
	draft   *agentstate.Draft
	history *stateHistory
	settled bool
}

func (draft *managedDraft) publish(ctx context.Context, summary string) error {
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.settled {
		return nil
	}
	published := false
	err := draft.history.withLock(ctx, func() error {
		result, err := draft.draft.Publish(ctx)
		if err != nil {
			return err
		}
		published = true
		if result.CleanupError != nil {
			slog.Warn("[harness-optimizer] published State with deferred cleanup",
				"draft_id", draft.draft.ID(), "error", result.CleanupError)
		}
		_, _, err = draft.history.record(result.Snapshot, summary)
		return err
	})
	if published {
		draft.settled = true
	}
	return err
}

func (draft *managedDraft) discard() {
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.settled {
		return
	}
	if err := draft.draft.Discard(); err != nil {
		slog.Warn("[harness-optimizer] discard draft failed", "draft_id", draft.draft.ID(), "error", err)
		return
	}
	draft.settled = true
}

func (service *Service) StartTask(ctx context.Context, request Request) (*apptask.Task, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	runtime, err := service.requireEnabled()
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
	request.Trigger = normalizeTrigger(request.Trigger)
	if request.Trigger == TriggerManual && strings.TrimSpace(request.Instruction) == "" {
		request.Instruction = "Review recent trajectory evidence and improve Harness State only when the evidence supports a reusable change."
	}
	if request.Trigger == TriggerScheduled {
		request.Instruction = "Review recent trajectory evidence since prior optimization. Make only evidence-backed, reusable Harness State improvements; use a no-op when no change is justified."
	}
	sessionID, err := optimizerSessionID()
	if err != nil {
		return nil, err
	}
	chatRequest := chatagent.CaptureChatRequestCallerInput(chatagent.ChatRequest{
		CommandID: request.CommandID, Message: optimizerMessage(request), Locale: request.Locale,
	})
	identity := apptask.StartIdentity{
		CommandID: request.CommandID, Scope: userScope, SessionID: sessionID,
		Fingerprint: agentexecution.RequestSemanticFingerprint(chatRequest),
	}
	if replay, ok, err := service.starts.Replay(identity); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	if latest := service.starts.Latest(userScope, sessionID).Task; latest != nil && !latest.Finished() {
		return nil, appagentruntime.ErrOperationActive
	}
	if runtime.Execution == nil {
		return nil, agentexecution.ErrUnavailable
	}
	operation, err := service.host.AcquireRootOperation(ctx)
	if err != nil {
		return nil, err
	}
	current, err := service.manager.Current(operation.Context())
	if err != nil {
		operation.Release()
		return nil, err
	}
	draft, err := service.manager.Store().BeginDraft(operation.Context(), current.Revision())
	if err != nil {
		operation.Release()
		return nil, err
	}
	managed := &managedDraft{draft: draft, history: service.history}
	cycle, err := service.buildCycle(operation.Context(), runtime, request, chatRequest, sessionID, managed)
	if err != nil {
		managed.discard()
		operation.Release()
		return nil, err
	}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		return task.BindLifetime(operation.Context())
	})
	if err != nil {
		managed.discard()
		operation.Release()
		return nil, err
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		task.RejectStart(err)
		managed.discard()
		operation.Release()
		return nil, err
	}
	cycle.Options.TaskID = task.ID()
	acceptCtx, releaseAcceptance := apptask.AcceptanceContext(operation.Context(), task)
	accepted, err := runtime.Execution.Start(acceptCtx, agentexecution.StartRequest{Cycle: cycle, Emit: task.Emit})
	releaseAcceptance()
	if err != nil {
		reservation.Rollback()
		task.RejectStart(err)
		managed.discard()
		operation.Release()
		return nil, err
	}
	if err := task.Start(func(runCtx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		defer operation.Release()
		outcome := accepted.Wait(runCtx)
		if outcome.Status != agentrun.OutcomeCompleted {
			managed.discard()
		}
		slog.InfoContext(runCtx, "[harness-optimizer] run settled", "command_id", request.CommandID, "status", outcome.Status, "trigger", request.Trigger)
	}); err != nil {
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
		managed.discard()
		operation.Release()
		return nil, err
	}
	reservation.Commit()
	return task, nil
}

func (service *Service) buildCycle(
	ctx context.Context,
	runtime Runtime,
	request Request,
	chatRequest chatagent.ChatRequest,
	sessionID string,
	draft *managedDraft,
) (agentexecution.Cycle, error) {
	runtimeConfig := runtime.Config
	appsettings.ApplyLocale(&runtimeConfig, request.Locale)
	runtimeConfig.Workspace = draft.draft.Root()
	runtimeConfig.ProjectID = ""
	runtimeConfig.ProjectStateDir = filepath.Join(service.dataDir, "runtime", "harness-optimizer", "drafts", draft.draft.ID())
	target, _, err := agentconversation.GetOrCreateSession(service.sessions, sessionID, &runtimeConfig, config.AgentKindHarnessOptimizer)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if _, err := agentconversation.ApplySession(target, &runtimeConfig, config.AgentKindHarnessOptimizer); err != nil {
		return agentexecution.Cycle{}, err
	}
	adapter, err := trajectory.NewReadAdapter(trajectory.Catalog{
		Sources: service.host.TrajectorySources, Outcomes: service.outcomes,
		Limit: runtime.Config.Labs.ContinualLearningTrajectoryCap,
	})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	binding, err := agenttools.NewReadAdapterBinding(config.AgentToolWorkspaceRead, adapter)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	built, err := appagentruntime.BuildHarnessOptimizerAgent(
		appagentruntime.WithHarnessRun(ctx, request.CommandID), &runtimeConfig, []agenttools.ReadAdapterBinding{binding},
		newOptimizerCompletionGuard(draft.draft.Validate),
	)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	restore, err := encodeRestoreData(optimizerRestoreData{
		DraftID: draft.draft.ID(), BaseRevision: draft.draft.BaseRevision(),
		Summary: optimizerVersionSummary(request), Trigger: request.Trigger,
	})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	return agentexecution.Cycle{
		Definition:   built.Definition,
		Conversation: agentconversation.NewSessionConversationForAgent(target, &runtimeConfig, config.AgentKindHarnessOptimizer),
		Request:      chatRequest,
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindHarnessOptimizer,
			StateRoot: filepath.Join(service.dataDir, "continual-learning"),
			SessionID: sessionID, Mode: RuntimeMode,
			IdleTimeout:        appagentruntime.IdleTimeout(runtimeConfig),
			ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtimeConfig),
			SystemPromptLog:    built.Composition, RestoreData: restore,
		},
		Successor: func(publishCtx context.Context, _ agentrun.OperationID, _ agentrun.Outcome) error {
			return draft.publish(publishCtx, optimizerVersionSummary(request))
		},
	}, nil
}

func (service *Service) PrepareCycle(ctx context.Context, recovery agentexecution.CycleRestoreRequest, binding agentrun.RuntimeBinding) (agentexecution.Cycle, error) {
	runtime, err := service.requireEnabled()
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if binding.AgentKind != agentrun.AgentKindHarnessOptimizer {
		return agentexecution.Cycle{}, agentexecution.ErrCyclePreparationUnavailable
	}
	data := recovery.Options.RestoreData
	if data == nil || data.Type != restoreDataType || data.Version != restoreDataVersion {
		return agentexecution.Cycle{}, agentexecution.ErrCyclePreparationUnavailable
	}
	var restored optimizerRestoreData
	if err := json.Unmarshal(data.Data, &restored); err != nil {
		return agentexecution.Cycle{}, fmt.Errorf("decode Harness Optimizer restore data: %w", err)
	}
	draft, err := service.manager.Store().ResumeDraft(ctx, restored.DraftID, restored.BaseRevision)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	request := Request{
		CommandID: string(recovery.CommandID), Instruction: recovery.Request.Message,
		Trigger: restored.Trigger, Locale: recovery.Request.Locale,
	}
	cycle, err := service.buildCycle(ctx, runtime, request, recovery.Request, binding.SessionID, &managedDraft{draft: draft, history: service.history})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	cycle.Options.TaskID = recovery.Options.TaskID
	return cycle, nil
}

func (service *Service) ActiveTask() *apptask.Task {
	if service == nil || service.initialize() != nil {
		return nil
	}
	sessionID, err := optimizerSessionID()
	if err != nil {
		return nil
	}
	return service.starts.Latest(userScope, sessionID).Task
}

func (service *Service) DisplayTask(taskID string) *apptask.Task {
	task := service.ActiveTask()
	if task == nil || strings.TrimSpace(taskID) == "" || task.ID() != strings.TrimSpace(taskID) {
		return nil
	}
	return task
}

func (service *Service) RuntimeStatus(ctx context.Context) (agentrun.RuntimeStatus, bool) {
	runtime, err := service.requireEnabled()
	if err != nil || runtime.Execution == nil {
		return agentrun.RuntimeStatus{}, false
	}
	sessionID, err := optimizerSessionID()
	if err != nil {
		return agentrun.RuntimeStatus{}, false
	}
	status, err := runtime.Execution.RuntimeStatusProjection(ctx, optimizerRunOptions(service.dataDir, sessionID))
	return status, err == nil
}

func (service *Service) PendingAsk() *session.AskInteraction {
	if service == nil || service.initialize() != nil {
		return nil
	}
	target, err := service.optimizerSession()
	if err != nil {
		return nil
	}
	return target.LivePendingAsk("")
}

func normalizeTrigger(value string) string {
	if strings.TrimSpace(value) == TriggerScheduled {
		return TriggerScheduled
	}
	return TriggerManual
}

func optimizerMessage(request Request) string {
	return strings.TrimSpace(fmt.Sprintf(`[Learning Trigger / 学习触发]
- trigger: %s
- trajectory_index: trajectory://index
- explicit_outcomes: trajectory://outcomes

[Task / 任务]
Evaluate the available trajectory evidence, critique recurring harness failures or durable user preferences, then update the isolated State draft only when a minimal reusable improvement is justified. Read specific session or run resources before drawing conclusions. Never copy project content or private reasoning into State.

[User Instruction / 用户指令]
%s`, normalizeTrigger(request.Trigger), strings.TrimSpace(request.Instruction)))
}

func optimizerVersionSummary(request Request) string {
	if normalizeTrigger(request.Trigger) == TriggerScheduled {
		return "Scheduled Harness State optimization"
	}
	return "Harness State optimization"
}

func encodeRestoreData(value optimizerRestoreData) (*agentrun.RestoreData, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &agentrun.RestoreData{Type: restoreDataType, Version: restoreDataVersion, Data: encoded}, nil
}

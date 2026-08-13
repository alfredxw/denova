package continuallearning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"

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
)

type optimizerRestoreData struct {
	Trigger string `json:"trigger"`
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
	request.Evidence, err = normalizeTrajectoryEvidence(request.Evidence)
	if err != nil {
		return nil, err
	}
	if request.Trigger == TriggerManual && strings.TrimSpace(request.Instruction) == "" {
		request.Instruction = "Review the selected trajectory evidence and improve Harness State only when the evidence supports a reusable change."
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
	cycle, err := service.buildCycle(operation.Context(), runtime, request, chatRequest, sessionID)
	if err != nil {
		operation.Release()
		return nil, err
	}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		return task.BindLifetime(operation.Context())
	})
	if err != nil {
		operation.Release()
		return nil, err
	}
	reservation, err := service.starts.Reserve(apptask.StartRecord{Identity: identity, Task: task})
	if err != nil {
		task.RejectStart(err)
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
		operation.Release()
		return nil, err
	}
	if err := task.Start(func(runCtx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		defer operation.Release()
		outcome := accepted.Wait(runCtx)
		slog.InfoContext(runCtx, "[harness-optimizer] run settled", "command_id", request.CommandID, "status", outcome.Status, "trigger", request.Trigger)
	}); err != nil {
		reservation.Rollback()
		task.Abort()
		_ = accepted.Wait(task.Context())
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
) (agentexecution.Cycle, error) {
	runtimeConfig := runtime.Config
	appsettings.ApplyLocale(&runtimeConfig, request.Locale)
	runtimeConfig.Workspace = service.manager.Root()
	runtimeConfig.ProjectID = ""
	runtimeConfig.ProjectStateDir = filepath.Join(service.dataDir, "runtime", "harness-optimizer")
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
		ctx, &runtimeConfig, []agenttools.ReadAdapterBinding{binding},
		newOptimizerCompletionGuard(service.manager.Validate),
	)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	restore, err := encodeRestoreData(optimizerRestoreData{Trigger: request.Trigger})
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
		Successor: func(recordCtx context.Context, _ agentrun.OperationID, _ agentrun.Outcome) error {
			return service.recordCurrentState(recordCtx, optimizerVersionSummary(request))
		},
	}, nil
}

func (service *Service) recordCurrentState(ctx context.Context, summary string) error {
	return service.history.withLock(ctx, func() error {
		snapshot, err := service.manager.ValidatedSnapshot(ctx)
		if err != nil {
			return err
		}
		_, _, err = service.history.record(snapshot, summary)
		return err
	})
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
	request := Request{
		CommandID: string(recovery.CommandID), Instruction: recovery.Request.Message,
		Trigger: restored.Trigger, Locale: recovery.Request.Locale,
	}
	cycle, err := service.buildCycle(ctx, runtime, request, recovery.Request, binding.SessionID)
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
	if service == nil {
		return nil
	}
	status, ok := service.RuntimeStatus(context.Background())
	if !ok || len(status.PendingInteractions) == 0 {
		return nil
	}
	return chatagent.ProjectPendingInteraction(status.PendingInteractions[0], status)
}

func normalizeTrigger(value string) string {
	if strings.TrimSpace(value) == TriggerScheduled {
		return TriggerScheduled
	}
	return TriggerManual
}

func optimizerMessage(request Request) string {
	evidenceScope := "Discover relevant recent evidence through trajectory://index."
	if request.Evidence != nil {
		if len(request.Evidence) == 0 {
			evidenceScope = "No trajectory was selected. Do not broaden the analysis to other trajectory resources unless the user explicitly asks."
		} else {
			evidenceScope = "Use only these user-selected trajectory resources as the analysis basis:\n- " + strings.Join(request.Evidence, "\n- ")
		}
	}
	return strings.TrimSpace(fmt.Sprintf(`[Optimization Trigger]
- trigger: %s
- trajectory_index: trajectory://index
- explicit_outcomes: trajectory://outcomes

[Analysis Evidence]
%s

[Task]
Evaluate the available trajectory evidence, critique recurring harness failures or durable user preferences, then update the live Harness State directory only when a minimal reusable improvement is justified. Every file edit takes effect immediately. Read specific session or run resources before drawing conclusions. Never copy project content or private reasoning into State.

[User Instruction]
%s`, normalizeTrigger(request.Trigger), evidenceScope, strings.TrimSpace(request.Instruction)))
}

func normalizeTrajectoryEvidence(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) > 500 {
		return nil, fmt.Errorf("trajectory evidence cannot exceed 500 resources")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("invalid trajectory evidence resource")
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed == nil {
			return nil, fmt.Errorf("invalid trajectory evidence resource %q", value)
		}
		segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		if parsed.Scheme != "trajectory" || parsed.Host != "projects" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(segments) != 3 || (segments[1] != "sessions" && segments[1] != "runs") {
			return nil, fmt.Errorf("invalid trajectory evidence resource %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
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

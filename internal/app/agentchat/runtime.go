package agentchat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	chatagent "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
)

func (service *Service) ActiveView(ctx context.Context, binding Binding) ActiveView {
	binding, err := service.ResolveBinding(binding)
	if err != nil || service.host == nil {
		return ActiveView{}
	}
	_, executionRuntime := service.host.BaseRuntime()
	runtime, projected := appagentruntime.RuntimeProjection(ctx, executionRuntime, runtimeOptions(binding, ""))
	active := service.activeRun(binding)
	var taskSnapshot *apptask.Snapshot
	var pendingAsk *session.AskInteraction
	streamAttached := false
	if active != nil && active.task != nil {
		snapshot := active.task.Snapshot()
		taskSnapshot = &snapshot
		streamAttached = !snapshot.Finished
	}
	if projected && len(runtime.PendingInteractions) > 0 {
		pendingAsk = chatagent.ProjectPendingInteraction(runtime.PendingInteractions[0], runtime)
	}
	pendingInterruptionID := ""
	if project, projectErr := service.projectRuntime(ctx, binding.ProjectID); projectErr == nil {
		if conversation, sessionErr := project.store.Get(binding.SessionID); sessionErr == nil {
			if pending := conversation.PendingInterruption(); pending != nil {
				pendingInterruptionID = strings.TrimSpace(pending.ID)
			}
		}
	}
	return ActiveView{
		Task: taskSnapshot, Runtime: runtime, RuntimeProjectionOK: projected,
		StreamAttached: streamAttached, PendingAsk: pendingAsk, PendingInterruptionID: pendingInterruptionID,
	}
}

// DisplayTask resolves only a reconnectable task owned by the exact binding.
func (service *Service) DisplayTask(binding Binding, taskID string) *apptask.Task {
	binding, err := service.ResolveBinding(binding)
	if err != nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	if active := service.activeRun(binding); active != nil && active.task != nil && active.task.ID() == taskID {
		return active.task
	}
	record := service.starts.Latest(binding.ProjectID, binding.SessionID)
	if record.Task != nil && record.Task.ID() == taskID {
		return record.Task
	}
	return nil
}

func (service *Service) MessagesPage(ctx context.Context, binding Binding, before, limit int) (session.HistoryPage, error) {
	binding, err := service.ResolveBinding(binding)
	if err != nil {
		return session.HistoryPage{}, err
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return session.HistoryPage{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (service *Service) AnalyzeContext(ctx context.Context, binding Binding, request chatagent.ChatRequest) (chatagent.ContextAnalysis, error) {
	binding, err := service.ResolveBinding(binding)
	if err != nil {
		return chatagent.ContextAnalysis{}, err
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return chatagent.ContextAnalysis{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return chatagent.ContextAnalysis{}, err
	}
	runtime, request, err := conversationapp.Prepare(ctx, project.conversation(sess), request)
	if err != nil {
		return chatagent.ContextAnalysis{}, err
	}
	inspected, err := conversationapp.InspectPrepared(ctx, runtime, request, runtimeOptions(binding, ""))
	if err != nil {
		return chatagent.ContextAnalysis{}, err
	}
	mode := "ide"
	if runtime.AgentKind == agentrun.AgentKindGeneral || runtime.AgentKind == agentrun.AgentKindHarness {
		mode = "general"
	}
	return chatagent.BuildInspectedContextAnalysis(
		&runtime.Config, runtime.AgentKind, mode, inspected.Composition, inspected.Inspection,
	), nil
}

func (service *Service) AnswerAsk(ctx context.Context, binding Binding, askID string, answers []agentconversation.HostAskAnswer) (agentconversation.HostAskResolution, error) {
	return service.resolveAsk(ctx, binding, askID, session.AskAnswered, answers, "")
}

func (service *Service) CancelAsk(ctx context.Context, binding Binding, askID, reason string) (agentconversation.HostAskResolution, error) {
	return service.resolveAsk(ctx, binding, askID, session.AskCancelled, nil, reason)
}

func (service *Service) resolveAsk(
	ctx context.Context,
	binding Binding,
	askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	binding, err := service.ResolveBinding(binding)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	return project.executionRuntime.ResolveAsk(ctx, runtimeOptions(binding, ""), askID, status, answers, cancelReason)
}

// ClearSession drains exactly one binding and appends the durable clear marker.
func (service *Service) ClearSession(ctx context.Context, binding Binding) error {
	service.admission.Lock()
	defer service.admission.Unlock()
	binding, err := service.ResolveBinding(binding)
	if err != nil {
		return err
	}
	if active := service.activeRun(binding); active != nil && active.task != nil && !active.task.Finished() {
		return appagentruntime.ErrOperationActive
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return err
	}
	if err := project.executionRuntime.ClearSession(ctx, runtimeOptions(binding, "")); err != nil {
		return err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return err
	}
	if err := sess.Clear(); err != nil {
		return err
	}
	service.starts.ReleaseScope(binding.ProjectID, binding.SessionID)
	return nil
}

func (service *Service) SessionBusy(binding Binding) bool {
	if resolved, err := service.ResolveBinding(binding); err == nil {
		binding = resolved
	}
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	active := service.activeRun(binding)
	return active != nil && active.task != nil && !active.task.Finished()
}

func (service *Service) runningBindingKeys() map[string]struct{} {
	if service == nil {
		return nil
	}
	service.mu.RLock()
	runs := make(map[string]*apptask.Task, len(service.active))
	for key, active := range service.active {
		if active != nil && active.task != nil {
			runs[key] = active.task
		}
	}
	service.mu.RUnlock()
	keys := make(map[string]struct{}, len(runs))
	for key, task := range runs {
		if !task.Finished() {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// Activity returns only the stable identities of running conversations. It is
// intentionally independent from Project/session metadata so detached UI tabs
// can observe completion without repeatedly scanning every journal.
func (service *Service) Activity() []Binding {
	keys := service.runningBindingKeys()
	bindings := make([]Binding, 0, len(keys))
	for key := range keys {
		projectID, sessionID, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		bindings = append(bindings, Binding{ProjectID: projectID, SessionID: sessionID})
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ProjectID != bindings[j].ProjectID {
			return bindings[i].ProjectID < bindings[j].ProjectID
		}
		return bindings[i].SessionID < bindings[j].SessionID
	})
	return bindings
}

func (service *Service) requireIdle(binding Binding) error {
	if service.SessionBusy(binding) {
		return fmt.Errorf("%w: AgentChat conversation is running", appagentruntime.ErrOperationActive)
	}
	return nil
}

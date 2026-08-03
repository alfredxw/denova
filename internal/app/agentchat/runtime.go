package agentchat

import (
	"context"
	"fmt"
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
	_, chatService := service.host.BaseRuntime()
	runtime, projected := appagentruntime.RuntimeProjection(ctx, chatService, runtimeOptions(binding, ""))
	active := service.activeRun(binding)
	var taskSnapshot *apptask.Snapshot
	var pendingAsk *session.AskInteraction
	streamAttached := false
	if active != nil && active.task != nil {
		snapshot := active.task.Snapshot()
		taskSnapshot = &snapshot
		streamAttached = !snapshot.Finished
		if active.runtime.Session != nil {
			pendingAsk = active.runtime.Session.LivePendingAsk("")
		}
	}
	return ActiveView{
		Task: taskSnapshot, Runtime: runtime, RuntimeProjectionOK: projected,
		StreamAttached: streamAttached, PendingAsk: pendingAsk,
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
	var pending *session.Interruption
	if strings.TrimSpace(request.Message) != "" {
		pending = runtime.Session.PendingInterruption()
	}
	var compaction *session.ContextCompaction
	if record, ok := runtime.Session.LatestContextCompaction(runtime.AgentKind); ok {
		compaction = &record
	}
	conversation := conversationapp.ProjectConversation(runtime, request)
	if runtime.AgentKind == agentrun.AgentKindGeneral {
		return chatagent.BuildGeneralContextAnalysis(&runtime.Config, runtime.BookService, compaction, pending, request, conversation)
	}
	return chatagent.BuildIDEContextAnalysis(
		&runtime.Config, runtime.State, runtime.IDETeller, runtime.BookService,
		compaction, pending, request, conversation,
	)
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
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return agentconversation.HostAskResolution{}, err
	}
	if service.host == nil {
		return agentconversation.ResolveAsk(ctx, sess, askID, status, answers, cancelReason)
	}
	return service.host.ResolveAsk(
		ctx, sess, binding.ProjectID, binding.Workspace, askID, status, answers, cancelReason,
	)
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
	if err := project.chatService.CloseProjectSessionBindings(ctx, binding.ProjectID, binding.SessionID); err != nil {
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

func (service *Service) requireIdle(binding Binding) error {
	if service.SessionBusy(binding) {
		return fmt.Errorf("%w: AgentChat conversation is running", appagentruntime.ErrOperationActive)
	}
	return nil
}

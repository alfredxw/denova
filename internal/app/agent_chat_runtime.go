package app

import (
	"context"
	"fmt"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
)

// AgentChatActiveView binds display and durable runtime state to one explicit
// project conversation rather than the foreground Writing selection.
type AgentChatActiveView struct {
	Task                *TaskStateSnapshot
	Runtime             agents.RuntimeStatus
	RuntimeProjectionOK bool
	StreamAttached      bool
	PendingAsk          *session.AskInteraction
}

func (a *App) AgentChatActiveView(ctx context.Context, binding AgentChatBinding) AgentChatActiveView {
	return a.agentChat().ActiveView(ctx, binding)
}

func (s *AgentChatAppService) ActiveView(ctx context.Context, binding AgentChatBinding) AgentChatActiveView {
	binding, err := s.resolveBinding(binding)
	if err != nil {
		return AgentChatActiveView{}
	}
	s.app.mu.RLock()
	chatService := s.app.chatService
	s.app.mu.RUnlock()
	if chatService == nil {
		return AgentChatActiveView{}
	}
	runtime, projected := projectAgentRuntime(ctx, chatService, agentChatRunOptions(binding, ""))
	run := s.activeRun(binding)
	var taskSnapshot *TaskStateSnapshot
	var pendingAsk *session.AskInteraction
	streamAttached := false
	if run != nil && run.task != nil {
		snapshot := run.task.Snapshot()
		taskSnapshot = &snapshot
		streamAttached = !snapshot.Finished
		if run.runtime.sess != nil {
			pendingAsk = run.runtime.sess.LivePendingAsk("")
		}
	}
	return AgentChatActiveView{
		Task: taskSnapshot, Runtime: runtime, RuntimeProjectionOK: projected,
		StreamAttached: streamAttached, PendingAsk: pendingAsk,
	}
}

// AgentChatDisplayTask resolves only a task owned by the requested binding.
func (a *App) AgentChatDisplayTask(binding AgentChatBinding, taskID string) *Task {
	return a.agentChat().DisplayTask(binding, taskID)
}

func (s *AgentChatAppService) DisplayTask(binding AgentChatBinding, taskID string) *Task {
	binding, err := s.resolveBinding(binding)
	if err != nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	if run := s.activeRun(binding); run != nil && run.task != nil && run.task.ID() == taskID {
		return run.task
	}
	record := s.starts.latestSessionTask(binding.ProjectID, binding.SessionID)
	if record.Task != nil && record.Task.ID() == taskID {
		return record.Task
	}
	return nil
}

func (a *App) AgentChatMessagesPage(
	ctx context.Context,
	binding AgentChatBinding,
	before, limit int,
) (session.HistoryPage, error) {
	return a.agentChat().MessagesPage(ctx, binding, before, limit)
}

func (s *AgentChatAppService) MessagesPage(
	ctx context.Context,
	binding AgentChatBinding,
	before, limit int,
) (session.HistoryPage, error) {
	binding, err := s.resolveBinding(binding)
	if err != nil {
		return session.HistoryPage{}, err
	}
	project, err := s.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return session.HistoryPage{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (a *App) AnalyzeAgentChatContext(
	ctx context.Context,
	binding AgentChatBinding,
	req agents.ChatRequest,
) (agents.ContextAnalysis, error) {
	return a.agentChat().AnalyzeContext(ctx, binding, req)
}

func (s *AgentChatAppService) AnalyzeContext(
	ctx context.Context,
	binding AgentChatBinding,
	req agents.ChatRequest,
) (agents.ContextAnalysis, error) {
	binding, err := s.resolveBinding(binding)
	if err != nil {
		return agents.ContextAnalysis{}, err
	}
	project, err := s.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return agents.ContextAnalysis{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return agents.ContextAnalysis{}, err
	}
	runtime := ideChatRuntime{
		projectID: project.projectID, projectType: project.projectType, projectState: project.stateRoot, agentKind: project.agentKind,
		app: s.app, sess: sess, state: project.state, bookService: project.bookService,
		chatService: project.chatService, workspace: project.workspace,
		versionService: project.versionService, cfg: project.cfg,
	}
	runtime, req, err = s.app.chat().prepareProjectChatRuntimeSnapshot(ctx, runtime, req)
	if err != nil {
		return agents.ContextAnalysis{}, err
	}
	var pending *session.Interruption
	if strings.TrimSpace(req.Message) != "" {
		pending = runtime.sess.PendingInterruption()
	}
	var compaction *session.ContextCompaction
	if record, ok := runtime.sess.LatestContextCompaction(runtime.agentKind); ok {
		compaction = &record
	}
	conversation := projectSessionConversation(runtime, req)
	if runtime.agentKind == agents.AgentKindGeneral {
		return agents.BuildGeneralContextAnalysis(&runtime.cfg, runtime.bookService, compaction, pending, req, conversation)
	}
	return agents.BuildIDEContextAnalysis(
		&runtime.cfg, runtime.state, runtime.ideTeller, runtime.bookService,
		compaction, pending, req, conversation,
	)
}

func (a *App) AnswerAgentChatAsk(
	ctx context.Context,
	binding AgentChatBinding,
	askID string,
	answers []AgentAskAnswer,
) (AgentAskResolution, error) {
	return a.agentChat().resolveAsk(ctx, binding, askID, session.AskAnswered, answers, "")
}

func (a *App) CancelAgentChatAsk(
	ctx context.Context,
	binding AgentChatBinding,
	askID, reason string,
) (AgentAskResolution, error) {
	return a.agentChat().resolveAsk(ctx, binding, askID, session.AskCancelled, nil, reason)
}

func (s *AgentChatAppService) resolveAsk(
	ctx context.Context,
	binding AgentChatBinding,
	askID, status string,
	answers []AgentAskAnswer,
	cancelReason string,
) (AgentAskResolution, error) {
	binding, err := s.resolveBinding(binding)
	if err != nil {
		return AgentAskResolution{}, err
	}
	project, err := s.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return AgentAskResolution{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return AgentAskResolution{}, err
	}
	return resolveAgentAsk(ctx, sess, askID, status, answers, cancelReason)
}

// ClearAgentChatSession drains exactly one AgentChat binding and appends the
// session clear marker. It does not alter the foreground Writing session.
func (a *App) ClearAgentChatSession(ctx context.Context, binding AgentChatBinding) error {
	return a.agentChat().ClearSession(ctx, binding)
}

func (s *AgentChatAppService) ClearSession(ctx context.Context, binding AgentChatBinding) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	binding, err := s.resolveBinding(binding)
	if err != nil {
		return err
	}
	if run := s.activeRun(binding); run != nil && run.task != nil && !run.task.Finished() {
		return ErrAgentOperationActive
	}
	project, err := s.projectRuntime(ctx, binding.ProjectID)
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
	s.starts.releaseConfigManagerScope(binding.ProjectID, binding.SessionID)
	return nil
}

func (s *AgentChatAppService) SessionBusy(binding AgentChatBinding) bool {
	if resolved, err := s.resolveBinding(binding); err == nil {
		binding = resolved
	}
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	run := s.activeRun(binding)
	return run != nil && run.task != nil && !run.task.Finished()
}

// runningBindingKeys snapshots only active executions for project-list reads.
// The project tree can then annotate every session without repeating path
// canonicalization and service locking for each metadata row.
func (s *AgentChatAppService) runningBindingKeys() map[string]struct{} {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	runs := make(map[string]*Task, len(s.active))
	for key, run := range s.active {
		if run != nil && run.task != nil {
			runs[key] = run.task
		}
	}
	s.mu.RUnlock()
	keys := make(map[string]struct{}, len(runs))
	for key, task := range runs {
		if !task.Finished() {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func (s *AgentChatAppService) requireIdle(binding AgentChatBinding) error {
	if s.SessionBusy(binding) {
		return fmt.Errorf("%w: AgentChat conversation is running", ErrAgentOperationActive)
	}
	return nil
}

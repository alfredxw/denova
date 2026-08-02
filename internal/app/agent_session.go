package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

type AgentAskAnswer = agentconversation.HostAskAnswer
type AgentAskSelectedOption = agentconversation.HostAskSelectedOption
type AgentAskAnswerResult = agentconversation.HostAskAnswerResult
type AgentAskResolution = agentconversation.HostAskResolution

var ErrAgentAskNotFound = agentconversation.ErrAskNotFound

func (a *App) persistAgentCall(agentKind, instruction, response string) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	persistAgentCallWithStore(store, agentKind, instruction, response)
}

func persistAgentCallWithStore(store *session.Store, agentKind, instruction, response string) {
	if store == nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[agent-session] skip persist agent=%s reason=no_session_store", agentKind))
		return
	}
	if err := session.PersistAgentCall(store, agentKind, instruction, response); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-session] persist failed agent=%s err=%v", agentKind, err))
	}
}

func (a *App) AgentSessionMessages(agentKind string) ([]session.HistoryEntry, error) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) AgentSessionMessagesPage(ctx context.Context, agentKind string, before, limit int) (session.HistoryPage, error) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (a *App) ClearAgentSession(agentKind string) error {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	return session.ClearAgentSession(store, agentKind)
}

func persistAgentCallInStore(store *session.Store, agentKind, instruction, response string) error {
	return session.PersistAgentCall(store, agentKind, instruction, response)
}

func clearAgentSessionInStore(store *session.Store, agentKind string) error {
	return session.ClearAgentSession(store, agentKind)
}

func agentSessionFromStore(store *session.Store, agentKind string) (*session.Session, error) {
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return session.AgentSession(store, agentKind)
}

func agentSessionID(agentKind string) (string, bool) {
	return session.AgentSessionID(agentKind)
}

// AnswerSessionAsk answers the exact pending ask in a user IDE session. The
// blocked tool call remains inside the same durable Agent task.
func (a *App) AnswerSessionAsk(ctx context.Context, sessionID, askID string, answers []AgentAskAnswer) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskAnswered, answers, "")
}

func (a *App) CancelSessionAsk(ctx context.Context, sessionID, askID, reason string) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskCancelled, nil, reason)
}

func (a *App) resolveSessionAsk(ctx context.Context, sessionID, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	if a == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.sessionStore
	selected := a.session
	a.mu.RUnlock()
	if store == nil || selected == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = selected.ID
	}
	if isAgentSessionID(sessionID) {
		return AgentAskResolution{}, fmt.Errorf("cannot resolve a fixed Agent ask through the IDE session endpoint: %s", sessionID)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return AgentAskResolution{}, err
	}
	return agentconversation.ResolveAsk(ctx, sess, askID, status, answers, cancelReason)
}

func (a *App) AgentRunTraces(limit int) ([]agentrun.RunTraceSummary, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return []agentrun.RunTraceSummary{}, nil
	}
	return agentrun.ListRunTraces(location, limit)
}

func (a *App) AgentRunTrace(id string) (agentrun.RunTrace, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return agentrun.RunTrace{}, ErrNoWorkspace
	}
	return agentrun.ReadRunTrace(location, id)
}

func (a *App) ExportAgentRunTrace(id string) (agentrun.RunTraceExport, error) {
	location, ok := a.agentRunTraceLocation()
	if !ok {
		return agentrun.RunTraceExport{}, ErrNoWorkspace
	}
	return agentrun.ExportRunTrace(location, id)
}

func (a *App) agentRunTraceLocation() (agentrun.TraceLocation, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	workspace := strings.TrimSpace(a.workspace)
	if workspace == "" {
		return agentrun.TraceLocation{}, false
	}
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = strings.TrimSpace(a.cfg.ProjectStateDir)
	}
	return agentrun.TraceLocation{Workspace: workspace, StateRoot: stateRoot}, true
}

package app

import (
	"log"

	"denova/internal/session"
)

func (a *App) persistAgentCall(agentKind, instruction, response string) {
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	persistAgentCallWithStore(store, agentKind, instruction, response)
}

func persistAgentCallWithStore(store *session.Store, agentKind, instruction, response string) {
	if store == nil {
		log.Printf("[agent-session] skip persist agent=%s reason=no_session_store", agentKind)
		return
	}
	if err := session.PersistAgentCall(store, agentKind, instruction, response); err != nil {
		log.Printf("[agent-session] persist failed agent=%s err=%v", agentKind, err)
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

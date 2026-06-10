package app

import (
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

const (
	loreAgentSessionID                  = "lore-agent"
	tellerAgentSessionID                = "teller-agent"
	interactiveStateAgentSessionID      = "interactive-state-agent"
	interactiveHotChoicesAgentSessionID = "interactive-hot-choices-agent"
	versionSummaryAgentSessionID        = "version-summary-agent"
	toolAgentSessionID                  = "tool-agent"
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
	if err := persistAgentCallInStore(store, agentKind, instruction, response); err != nil {
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
	return clearAgentSessionInStore(store, agentKind)
}

func persistAgentCallInStore(store *session.Store, agentKind, instruction, response string) error {
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return err
	}
	if instruction == "" {
		instruction = "（空输入）"
	}
	if err := sess.Append(schema.UserMessage(instruction)); err != nil {
		return fmt.Errorf("写入 Agent 输入失败: %w", err)
	}
	if response == "" {
		response = "（空输出）"
	}
	if err := sess.Append(schema.AssistantMessage(response, nil)); err != nil {
		return fmt.Errorf("写入 Agent 输出失败: %w", err)
	}
	return nil
}

func clearAgentSessionInStore(store *session.Store, agentKind string) error {
	sess, err := agentSessionFromStore(store, agentKind)
	if err != nil {
		return err
	}
	return sess.Clear()
}

func agentSessionFromStore(store *session.Store, agentKind string) (*session.Session, error) {
	if store == nil {
		return nil, ErrNoWorkspace
	}
	id, ok := agentSessionID(agentKind)
	if !ok {
		return nil, fmt.Errorf("未配置 Agent 会话: %s", agentKind)
	}
	return store.GetOrCreate(id)
}

func agentSessionID(agentKind string) (string, bool) {
	switch agentKind {
	case config.AgentKindLoreEditor:
		return loreAgentSessionID, true
	case config.AgentKindTellerEditor:
		return tellerAgentSessionID, true
	case config.AgentKindInteractiveState:
		return interactiveStateAgentSessionID, true
	case config.AgentKindInteractiveHotChoices:
		return interactiveHotChoicesAgentSessionID, true
	case config.AgentKindVersionSummary:
		return versionSummaryAgentSessionID, true
	case config.AgentKindToolAgent:
		return toolAgentSessionID, true
	default:
		return "", false
	}
}

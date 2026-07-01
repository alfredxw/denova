package session

import (
	"fmt"

	"github.com/cloudwego/eino/schema"

	"denova/config"
)

// AgentSessionID resolves the fixed journal session for a built-in background Agent.
func AgentSessionID(agentKind string) (string, bool) {
	definition, ok := config.LookupAgentKind(agentKind)
	if !ok || definition.SessionID == "" {
		return "", false
	}
	return definition.SessionID, true
}

// AgentSession returns the fixed background Agent journal session.
func AgentSession(store *Store, agentKind string) (*Session, error) {
	if store == nil {
		return nil, fmt.Errorf("session store is nil")
	}
	id, ok := AgentSessionID(agentKind)
	if !ok {
		return nil, fmt.Errorf("未配置 Agent 会话: %s", agentKind)
	}
	return store.GetOrCreate(id)
}

// PersistAgentCall appends a full input/output pair to a background Agent journal.
func PersistAgentCall(store *Store, agentKind, instruction, response string) error {
	sess, err := AgentSession(store, agentKind)
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

// ClearAgentSession appends a clear marker to a background Agent journal.
func ClearAgentSession(store *Store, agentKind string) error {
	sess, err := AgentSession(store, agentKind)
	if err != nil {
		return err
	}
	return sess.Clear()
}

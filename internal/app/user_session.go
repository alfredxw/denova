package app

import (
	"strings"

	"denova/config"
	"denova/internal/agents/session"
)

const defaultUserSessionID = "default"

func activeUserSessionOrCreate(store *session.Store, runtimeCfg *config.Config) (*session.Session, error) {
	if store == nil {
		return nil, ErrNoWorkspace
	}
	activeID, _ := store.ActiveID()
	activeID = strings.TrimSpace(activeID)
	if activeID == "" || isAgentSessionID(activeID) {
		activeID = defaultUserSessionID
	} else if _, err := store.Get(activeID); err != nil {
		activeID = defaultUserSessionID
	}
	var sess *session.Session
	var err error
	if store.Exists(activeID) {
		sess, err = store.Get(activeID)
		if err == nil {
			_, err = ensureExistingSessionConfig(sess, runtimeCfg, config.AgentKindIDE)
		}
	} else {
		seed, seedErr := recentConversationSeed(store, runtimeCfg, config.AgentKindIDE, activeID)
		if seedErr != nil {
			return nil, seedErr
		}
		sess, err = store.GetOrCreateWithRuntimeConfig(activeID, seed)
	}
	if err != nil {
		return nil, err
	}
	if err := store.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	return sess, nil
}

func listUserSessions(store *session.Store, activeID string) ([]session.SessionMeta, error) {
	if store == nil {
		return nil, ErrNoWorkspace
	}
	metas, err := store.List(activeID)
	if err != nil {
		return nil, err
	}
	result := make([]session.SessionMeta, 0, len(metas))
	for _, meta := range metas {
		if isAgentSessionID(meta.ID) {
			continue
		}
		result = append(result, meta)
	}
	return result, nil
}

func isAgentSessionID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, definition := range config.AgentKindDefinitions() {
		if definition.SessionID == id || (definition.SessionID != "" && strings.HasPrefix(id, definition.SessionID+"-")) {
			return true
		}
	}
	return false
}

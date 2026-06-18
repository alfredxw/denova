package app

import (
	"strings"

	"nova/config"
	"nova/internal/session"
)

const defaultUserSessionID = "default"

func activeUserSessionOrCreate(store *session.Store) (*session.Session, error) {
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
	sess, err := store.GetOrCreate(activeID)
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
		if definition.SessionID == id {
			return true
		}
	}
	return false
}

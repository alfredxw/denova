package app

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func (a *App) ClearSession() error {
	return a.chat().ClearSession()
}

func (s *ChatAppService) ClearSession() error {
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainWritingBinding(context.Background(), "")
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a, true); err != nil {
		return err
	}
	if fence.chat == nil {
		return ErrNoWorkspace
	}
	if err := fence.chat.ClearSession(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, StateRoot: fence.stateRoot,
		Workspace: fence.workspace, SessionID: fence.sessionID, Mode: "ide",
	}); err != nil {
		return err
	}
	cursor := fence.selected.ContextCursor()
	return fence.selected.AppendClearMarkerAt(cursor)
}

// Sessions 返回当前 workspace 下的会话列表。
func (a *App) Sessions() ([]session.SessionMeta, error) {
	return a.chat().Sessions()
}

func (s *ChatAppService) Sessions() ([]session.SessionMeta, error) {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	var activeID string
	if a.session != nil {
		activeID = a.session.ID
	}
	a.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return listUserSessions(store, activeID)
}

// CreateSession 新建会话并设置为当前激活会话。
func (a *App) CreateSession(title string) (*session.Session, error) {
	return a.chat().CreateSession(title)
}

func (s *ChatAppService) CreateSession(title string) (*session.Session, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainWritingBinding(context.Background(), "")
	if err != nil {
		return nil, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a, true); err != nil {
		return nil, err
	}

	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	seed, err := agentconversation.RecentSessionSeed(fence.store, &runtimeCfg, config.AgentKindIDE, "")
	if err != nil {
		return nil, err
	}
	sess, err := fence.store.CreateWithRuntimeConfig(title, seed)
	if err != nil {
		return nil, err
	}
	if err := fence.store.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	if a.activeTask == nil || a.activeTask.Finished() {
		a.activeTask = nil
		a.activeWritingRun = nil
	}
	return sess, nil
}

// SwitchSession 切换当前激活会话。
func (a *App) SwitchSession(id string) (*session.Session, error) {
	return a.chat().SwitchSession(id)
}

func (s *ChatAppService) SwitchSession(id string) (*session.Session, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能切换到固定 Agent 会话: %s", id)
	}
	fence, err := s.drainWritingBinding(context.Background(), "")
	if err != nil {
		return nil, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a, true); err != nil {
		return nil, err
	}

	sess, err := fence.store.Get(id)
	if err != nil {
		return nil, err
	}
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	if _, err := agentconversation.EnsureSession(sess, &runtimeCfg, config.AgentKindIDE); err != nil {
		return nil, err
	}
	if err := fence.store.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	if a.activeTask == nil || a.activeTask.Finished() {
		a.activeTask = nil
		a.activeWritingRun = nil
	}
	return sess, nil
}

// RenameSession 修改会话标题。
func (a *App) RenameSession(id, title string) error {
	return a.chat().RenameSession(id, title)
}

func (s *ChatAppService) RenameSession(id, title string) error {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	if isAgentSessionID(id) {
		return fmt.Errorf("不能重命名固定 Agent 会话: %s", id)
	}
	return store.Rename(id, title)
}

// DeleteSession 删除会话；删除当前会话后自动切换到剩余最近会话。
func (a *App) DeleteSession(id string) (*session.Session, error) {
	return a.chat().DeleteSession(id)
}

func (s *ChatAppService) DeleteSession(id string) (*session.Session, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能删除固定 Agent 会话: %s", id)
	}
	fence, err := s.drainWritingBinding(context.Background(), id)
	if err != nil {
		return nil, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	wasActive := fence.selected != nil && fence.selected.ID == id
	if err := fence.validateLocked(a, wasActive); err != nil {
		return nil, err
	}

	userSessions, err := listUserSessions(fence.store, "")
	if err != nil {
		return nil, err
	}
	if len(userSessions) <= 1 {
		return nil, fmt.Errorf("不能删除当前唯一会话")
	}

	if fence.chat == nil {
		return nil, ErrNoWorkspace
	}
	if err := fence.chat.DeleteSessionBindings(context.Background(), agentrun.AgentKindIDE, fence.workspace, id); err != nil {
		return nil, err
	}
	if err := fence.store.Delete(id); err != nil {
		return nil, err
	}
	activeID := ""
	if !wasActive && a.session != nil {
		activeID = a.session.ID
	}
	if activeID == "" {
		metas, err := listUserSessions(fence.store, "")
		if err != nil {
			return nil, err
		}
		if len(metas) == 0 {
			runtimeCfg := config.Config{}
			if a.cfg != nil {
				runtimeCfg = *a.cfg
			}
			sess, _, createErr := agentconversation.GetOrCreateSession(fence.store, "default", &runtimeCfg, config.AgentKindIDE)
			if createErr != nil {
				return nil, createErr
			}
			a.session = sess
			activeID = sess.ID
		} else {
			activeID = metas[0].ID
		}
	}
	sess, err := fence.store.GetOrCreate(activeID)
	if err != nil {
		return nil, err
	}
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	if _, err := agentconversation.EnsureSession(sess, &runtimeCfg, config.AgentKindIDE); err != nil {
		return nil, err
	}
	if err := fence.store.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	if wasActive {
		if a.activeTask == nil || a.activeTask.Finished() {
			a.activeTask = nil
			a.activeWritingRun = nil
		}
	}
	return sess, nil
}

// SessionMessages 返回指定会话或当前会话的完整历史。
func (a *App) SessionMessages(id string) ([]session.HistoryEntry, error) {
	return a.chat().SessionMessages(id)
}

// SessionMessagesPage reads one bounded UI-history page directly from the
// append-only store instead of slicing a fully materialized transcript.
func (a *App) SessionMessagesPage(ctx context.Context, id string, before, limit int) (session.HistoryPage, error) {
	return a.chat().SessionMessagesPage(ctx, id, before, limit)
}

func (s *ChatAppService) SessionMessages(id string) ([]session.HistoryEntry, error) {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	current := a.session
	a.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	if id == "" {
		if current == nil {
			return nil, ErrNoWorkspace
		}
		return current.History(), nil
	}
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能通过创作会话读取固定 Agent 会话: %s", id)
	}
	sess, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (s *ChatAppService) SessionMessagesPage(ctx context.Context, id string, before, limit int) (session.HistoryPage, error) {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	current := a.session
	a.mu.RUnlock()
	if store == nil {
		return session.HistoryPage{}, ErrNoWorkspace
	}
	var sess *session.Session
	var err error
	if id == "" {
		sess = current
		if sess == nil {
			return session.HistoryPage{}, ErrNoWorkspace
		}
	} else {
		if isAgentSessionID(id) {
			return session.HistoryPage{}, fmt.Errorf("不能通过创作会话读取固定 Agent 会话: %s", id)
		}
		sess, err = store.Get(id)
		if err != nil {
			return session.HistoryPage{}, err
		}
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

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
			_, err = agentconversation.EnsureSession(sess, runtimeCfg, config.AgentKindIDE)
		}
	} else {
		seed, seedErr := agentconversation.RecentSessionSeed(store, runtimeCfg, config.AgentKindIDE, activeID)
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
		if !isAgentSessionID(meta.ID) {
			result = append(result, meta)
		}
	}
	return result, nil
}

func isAgentSessionID(id string) bool {
	return agentconversation.IsReservedSessionID(id)
}

// StartTask starts a root Writing Agent task. A running operation is never
// replaced implicitly; callers use the typed Follow Up, Steer, or Abort API.

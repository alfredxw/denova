package app

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"fmt"
	"strings"

	"denova/internal/agents/session"
)

func emitWritingRecoveryRefreshRequired(
	emit func(agentrun.Event),
	action agentexecution.RuntimeRecoveryAction,
	cursor agentrun.Cursor,
) {
	if emit == nil {
		return
	}
	emit(agentrun.Event{Type: agentexecution.RuntimeRecoveryRequiredEventType, Data: map[string]any{
		"code":         agentexecution.RuntimeRecoveryRequiredEventCode,
		"message":      "会话状态刷新失败，请重试恢复 / Session state refresh failed; retry recovery",
		"operation_id": string(action.OperationID),
		"cursor":       uint64(cursor),
	}})
}

func writingRecoveryRefreshKey(workspace, sessionID string) string {
	return strings.TrimSpace(workspace) + "\x00" + strings.TrimSpace(sessionID)
}

// markRecoveryRefreshPending records the process-local projection obligation
// created after a recovered structural commit becomes durable. It belongs to
// ChatAppService rather than a display Task: direct compact/remove recovery and
// a finished recovery Task must fence the exact same long-lived Session.
func (s *ChatAppService) markRecoveryRefreshPending(
	workspace, sessionID string,
	action agentexecution.RuntimeRecoveryAction,
) {
	if s == nil || strings.TrimSpace(workspace) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}
	s.recoveryRefreshMu.Lock()
	if s.recoveryRefreshPending == nil {
		s.recoveryRefreshPending = make(map[string]agentexecution.RuntimeRecoveryAction)
	}
	key := writingRecoveryRefreshKey(workspace, sessionID)
	current, exists := s.recoveryRefreshPending[key]
	currentExact := strings.TrimSpace(string(current.CommandID)) != "" && strings.TrimSpace(string(current.OperationID)) != ""
	incomingExact := strings.TrimSpace(string(action.CommandID)) != "" && strings.TrimSpace(string(action.OperationID)) != ""
	// Direct compact/remove recovery has no public runtime identity at this
	// seam. It may create a generic admission fence, but it must never erase or
	// replace an exact action already owned by a live recovery Task.
	if exists && currentExact && (!incomingExact || current != action) {
		s.recoveryRefreshMu.Unlock()
		return
	}
	s.recoveryRefreshPending[key] = action
	s.recoveryRefreshMu.Unlock()
}

func (s *ChatAppService) hasActiveWritingStructuralRecovery() bool {
	if s == nil || s.app == nil {
		return false
	}
	s.app.mu.RLock()
	run := s.app.activeWritingRun
	s.app.mu.RUnlock()
	return run != nil && run.recoveryStructural && run.task != nil && !run.task.Finished()
}

// pendingRecoveryRefreshAction exposes only the exact public recovery
// identity. Direct structural endpoints can leave an obligation with blank
// IDs; those remain an admission fence but are retried by that endpoint rather
// than advertised as an invalid Agent recovery action.
func (s *ChatAppService) pendingRecoveryRefreshAction(
	workspace, sessionID string,
) (agentexecution.RuntimeRecoveryAction, bool) {
	if s == nil {
		return agentexecution.RuntimeRecoveryAction{}, false
	}
	s.recoveryRefreshMu.Lock()
	defer s.recoveryRefreshMu.Unlock()
	action, ok := s.recoveryRefreshPending[writingRecoveryRefreshKey(workspace, sessionID)]
	if !ok || strings.TrimSpace(string(action.CommandID)) == "" || strings.TrimSpace(string(action.OperationID)) == "" {
		return agentexecution.RuntimeRecoveryAction{}, false
	}
	return action, true
}

// clearRecoveryRefreshObligations is called only after a fresh workspace
// runtime/session generation has been installed. That Session was loaded from
// canonical storage, so process-local obligations owned by the prior
// generation are both stale and already satisfied.
func (s *ChatAppService) clearRecoveryRefreshObligations(workspace string) {
	if s == nil {
		return
	}
	prefix := strings.TrimSpace(workspace) + "\x00"
	s.recoveryRefreshMu.Lock()
	for key := range s.recoveryRefreshPending {
		if strings.HasPrefix(key, prefix) {
			delete(s.recoveryRefreshPending, key)
		}
	}
	s.recoveryRefreshMu.Unlock()
}

// retryPendingWritingRecoveryRefresh is the shared admission fence used by a
// new turn and both manual structural commands. All three read/prepare the
// selected Session and therefore must close the same stale-projection
// obligation first.
func (s *ChatAppService) retryPendingWritingRecoveryRefresh(
	ctx context.Context,
	workspace string,
	selected *session.Session,
) error {
	if s == nil || s.app == nil || selected == nil {
		return nil
	}
	s.app.mu.RLock()
	stillSelected := s.app.workspace == workspace && s.app.session == selected
	s.app.mu.RUnlock()
	if !stillSelected {
		return ErrAgentContextChanged
	}
	_, err := s.retryAnyRecoveryRefresh(ctx, workspace, selected.ID, selected.RefreshCanonical)
	return err
}

// retryRecoveryRefresh executes one matching outstanding projection refresh.
// The obligation remains pending on failure, so an idempotent retry of the
// exact recovery action can close it even though the structural runtime action
// itself has already reached a durable terminal state.
func (s *ChatAppService) retryRecoveryRefresh(
	ctx context.Context,
	workspace, sessionID string,
	action agentexecution.RuntimeRecoveryAction,
	refresh func(context.Context) error,
) (bool, error) {
	if s == nil || refresh == nil {
		return false, nil
	}
	bindingKey := writingRecoveryRefreshKey(workspace, sessionID)
	key := recoveryActionKey(action)
	s.recoveryRefreshMu.Lock()
	defer s.recoveryRefreshMu.Unlock()
	pending, ok := s.recoveryRefreshPending[bindingKey]
	if !ok || recoveryActionKey(pending) != key {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := refresh(context.WithoutCancel(ctx)); err != nil {
		return true, fmt.Errorf("refresh recovered writing session: %w", err)
	}
	delete(s.recoveryRefreshPending, bindingKey)
	return true, nil
}

// retryAnyRecoveryRefresh is the admission fence for a brand-new Writing
// StartTurn. A failed refresh must not be bypassed by replacing the finished
// recovery Task, because that would let the next model context read the stale
// selected Session and discard the only retry handle.
func (s *ChatAppService) retryAnyRecoveryRefresh(
	ctx context.Context,
	workspace, sessionID string,
	refresh func(context.Context) error,
) (bool, error) {
	if s == nil || refresh == nil {
		return false, nil
	}
	bindingKey := writingRecoveryRefreshKey(workspace, sessionID)
	s.recoveryRefreshMu.Lock()
	defer s.recoveryRefreshMu.Unlock()
	if _, ok := s.recoveryRefreshPending[bindingKey]; !ok {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := refresh(context.WithoutCancel(ctx)); err != nil {
		return true, fmt.Errorf("refresh recovered writing session before next turn: %w", err)
	}
	delete(s.recoveryRefreshPending, bindingKey)
	return true, nil
}

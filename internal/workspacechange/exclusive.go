package workspacechange

import "context"

type exclusiveWorkspaceMode uint8

const (
	exclusiveWorkspaceMutation exclusiveWorkspaceMode = iota
	exclusiveWorkspaceSnapshot
)

// WithExclusiveWorkspace runs an unmanaged workspace operation under the same
// mutation lock used by editor saves, Agent writes, review, and history. It is
// intended for operations such as a foreground shell command whose individual
// filesystem effects cannot be expressed as a ChangeSet.
func (s *Service) WithExclusiveWorkspace(ctx context.Context, action func() error) error {
	return s.withExclusiveWorkspace(ctx, action, exclusiveWorkspaceMutation)
}

// WithConsistentWorkspaceSnapshot serializes a read-only workspace snapshot
// with visible mutations without invalidating durable editor-save receipts.
func (s *Service) WithConsistentWorkspaceSnapshot(ctx context.Context, action func() error) error {
	return s.withExclusiveWorkspace(ctx, action, exclusiveWorkspaceSnapshot)
}

func (s *Service) withExclusiveWorkspace(ctx context.Context, action func() error, mode exclusiveWorkspaceMode) error {
	if s == nil {
		return newError(ErrorCodeConflict, "change service is nil", nil)
	}
	if action == nil {
		return newError(ErrorCodeConflict, "exclusive workspace action is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return err
	}
	err := action()
	if mode == exclusiveWorkspaceMutation {
		// An unmanaged command may have changed any path, so an old idempotent
		// editor-save receipt can no longer be trusted.
		s.pendingSaves = map[string]pendingSaveIntent{}
	}
	return err
}

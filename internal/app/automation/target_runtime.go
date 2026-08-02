package automationapp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/agents/session"
	"denova/internal/automation"
)

// acquireTargetRuntime admits the target lifecycle before constructing any
// workspace adapters. The returned operation must remain owned until the
// synchronous call finishes or a background Task has atomically acquired its
// own lease.
func (s *Service) acquireTargetRuntime(ctx context.Context, target automation.ExecutionTarget) (*automationWorkspaceSnapshot, Operation, error) {
	if s == nil || s.host == nil {
		return nil, nil, fmt.Errorf("automation app is unavailable")
	}
	var (
		operation Operation
		err       error
	)
	if target.Kind == automation.TargetKindUser {
		operation, err = s.host.AcquireRootOperation(ctx)
	} else {
		resolved, resolveErr := s.resolveAutomationProjectTarget(target)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		target = resolved
		workspace := canonicalAutomationWorkspace(resolved.Workspace)
		operation, err = s.host.AcquireWorkspaceOperation(ctx, workspace)
	}
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := s.automationSnapshotForTarget(operation.Context(), target)
	if err != nil {
		operation.Release()
		return nil, nil, err
	}
	return snapshot, operation, nil
}

// resolveAutomationProjectTarget follows the stable Project ID first and uses
// the stored path only for legacy definitions written before Project existed.
func (s *Service) resolveAutomationProjectTarget(target automation.ExecutionTarget) (automation.ExecutionTarget, error) {
	if s == nil || s.host == nil {
		return automation.ExecutionTarget{}, fmt.Errorf("automation app is unavailable")
	}
	resolved, err := s.host.ResolveTarget(target)
	if err != nil {
		return automation.ExecutionTarget{}, fmt.Errorf("resolve automation project: %w", err)
	}
	return resolved, nil
}

// automationSnapshotForTarget resolves an execution context without changing the
// workspace selected in the UI. Inactive books are loaded lazily only when a
// task needs to run or its trigger needs evaluation. Returns a snapshot that
// the caller passes to the automation methods.
func (s *Service) automationSnapshotForTarget(ctx context.Context, target automation.ExecutionTarget) (*automationWorkspaceSnapshot, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("automation app is unavailable")
	}
	if target.Kind == automation.TargetKindUser {
		return s.globalAutomationSnapshot()
	}
	runtime, err := s.host.RuntimeForTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	return snapshotFromRuntime(runtime), nil
}

func (s *Service) globalAutomationSnapshot() (*automationWorkspaceSnapshot, error) {
	runtime := s.host.BaseRuntime()
	runtime.Workspace = ""
	runtime.ProjectID = ""
	runtime.StateRoot = ""
	runtime.ProjectType = ""
	runtime.BookState = nil
	runtime.BookService = nil
	runtime.Config.Workspace = ""
	runtime.Config.ProjectID = ""
	runtime.Config.ProjectStateDir = ""
	dataDir := strings.TrimSpace(runtime.DataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(runtime.Config.DataDir())
	}
	if dataDir == "" {
		return nil, fmt.Errorf("user data directory is required for global automation")
	}
	s.globalStoreMu.Lock()
	defer s.globalStoreMu.Unlock()
	if s.globalStore == nil {
		store, err := session.NewStore(filepath.Join(dataDir, "automations", "sessions"))
		if err != nil {
			return nil, fmt.Errorf("open global automation sessions: %w", err)
		}
		s.globalStore = store
	}
	runtime.DataDir = dataDir
	runtime.SessionStore = s.globalStore
	return snapshotFromRuntime(runtime), nil
}

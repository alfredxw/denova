package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	projectfilesapp "denova/internal/app/projectfiles"
	"denova/internal/book"
	"denova/internal/workspace/autosave"
)

// workspaceMutationRuntime is a workspace-scoped snapshot of the services
// needed by unmanaged filesystem mutations. Callers must use these captured
// services instead of resolving the active runtime again while the lease is held.
type workspaceMutationRuntime struct {
	bookService     *book.Service
	versionService  *book.VersionService
	versionSettings book.VersionAutoSettings
}

// withExclusiveWorkspaceMutation binds an unmanaged filesystem mutation to
// both the active App runtime and the shared workspace-change write lease.
func (s *workspaceService) withExclusiveWorkspaceMutation(
	ctx context.Context,
	action func(workspaceMutationRuntime) error,
) error {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.workspace == "" || a.bookService == nil || a.versionService == nil {
		return ErrNoWorkspace
	}
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStoreDir
	}
	changeService, err := workspaceChangeService(a.workspace, stateRoot)
	if err != nil {
		return err
	}
	runtime := workspaceMutationRuntime{
		bookService:     a.bookService,
		versionService:  a.versionService,
		versionSettings: versionAutoSettingsForConfig(a.cfg),
	}
	return changeService.WithExclusiveWorkspace(ctx, func() error {
		return action(runtime)
	})
}

// CreateWorkspaceItem creates one file or directory under the same workspace
// lease used by editor, Agent, review, undo, and redo mutations.
func (a *App) CreateWorkspaceItem(ctx context.Context, path, itemType, content string) error {
	return a.workspaceService().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if err := runtime.bookService.Create(path, itemType, content); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

// DeleteWorkspaceItem creates the existing restore point and removes one file
// or directory as a single workspace-scoped operation.
func (a *App) DeleteWorkspaceItem(ctx context.Context, path string) error {
	return a.workspaceService().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if _, err := runtime.versionService.Create("删除前自动备份", book.VersionSourceManual, runtime.versionSettings); err != nil && !errors.Is(err, book.ErrVersionClean) {
			return err
		}
		if err := runtime.bookService.Delete(path); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

// RenameWorkspaceItem renames one file or directory under the shared write lease.
func (a *App) RenameWorkspaceItem(ctx context.Context, path, newName string) (string, error) {
	var newPath string
	err := a.workspaceService().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		var err error
		newPath, err = runtime.bookService.Rename(path, newName)
		if err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
	return newPath, err
}

// CopyWorkspaceItem copies one file or directory under the shared write lease.
func (a *App) CopyWorkspaceItem(ctx context.Context, from, to string) error {
	return a.workspaceService().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if err := runtime.bookService.Copy(from, to); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

// MoveWorkspaceItem moves one file or directory under the shared write lease.
func (a *App) MoveWorkspaceItem(ctx context.Context, from, to string) error {
	return a.workspaceService().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if err := runtime.bookService.Move(from, to); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

// RecordAutosaveConflict durably preserves every side of a merge conflict in
// the process-wide Denova data directory before a caller resolves it.
func (a *App) RecordAutosaveConflict(ctx context.Context, input autosave.Input) (autosave.AppendResult, error) {
	if a == nil {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: app is nil")
	}
	a.mu.RLock()
	dataDir := ""
	if a.cfg != nil {
		dataDir = strings.TrimSpace(a.cfg.DataDir())
	}
	a.mu.RUnlock()
	if dataDir == "" {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: Denova data directory is not configured")
	}
	result, err := autosave.NewStore(dataDir).Append(ctx, input)
	if err != nil {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[autosave-conflict] recorded resource=%q scope=%q id=%q record_id=%q path=%q", input.Resource, input.Scope, input.ID, result.Record.ID, result.Path))
	return result, nil
}

func (a *App) SearchProjectWorkspace(ctx context.Context, projectID, query string, limit int, options book.SearchOptions) ([]book.SearchResult, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	return a.ProjectFiles().Search(operation.Context(), operation.Layout().ProjectID, query, limit, options)
}

func (a *App) ReplaceProjectWorkspace(ctx context.Context, projectID string, request projectfilesapp.ReplaceRequest) (projectfilesapp.ReplaceResult, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return projectfilesapp.ReplaceResult{}, err
	}
	defer operation.Release()
	result, err := a.ProjectFiles().Replace(operation.Context(), operation.Layout().ProjectID, request)
	if err != nil {
		return projectfilesapp.ReplaceResult{}, err
	}
	if len(result.Files) > 0 {
		paths := make([]string, 0, len(result.Files))
		for _, file := range result.Files {
			paths = append(paths, file.Path)
		}
		a.Automation().CheckTriggersAfterProjectMutation(
			operation.Context(), operation.Layout().ProjectID, "workspace_replace", paths,
		)
	}
	return result, nil
}

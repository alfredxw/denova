package app

import (
	"context"
	"errors"

	"denova/internal/book"
	"denova/internal/workspacechange"
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
func (s *WorkspaceRuntimeManager) withExclusiveWorkspaceMutation(
	ctx context.Context,
	action func(workspaceMutationRuntime) error,
) error {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.workspace == "" || a.bookService == nil || a.versionService == nil {
		return ErrNoWorkspace
	}
	changeService, err := workspacechange.ForWorkspace(a.workspace)
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
	return a.runtime().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
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
	return a.runtime().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
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
	err := a.runtime().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
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
	return a.runtime().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if err := runtime.bookService.Copy(from, to); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

// MoveWorkspaceItem moves one file or directory under the shared write lease.
func (a *App) MoveWorkspaceItem(ctx context.Context, from, to string) error {
	return a.runtime().withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		if err := runtime.bookService.Move(from, to); err != nil {
			return err
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
}

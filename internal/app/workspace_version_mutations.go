package app

import (
	"context"

	"denova/config"
	"denova/internal/book"
	"denova/internal/workspacechange"
)

// VersionStatus 返回当前书籍 workspace 的本地版本状态。
func (a *App) VersionStatus(ctx context.Context) (book.VersionStatus, error) {
	return a.runtime().VersionStatus(ctx)
}

func (s *WorkspaceRuntimeManager) VersionStatus(ctx context.Context) (book.VersionStatus, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionStatus{}, ErrNoWorkspace
	}
	return versionService.Status(s.versionAutoSettings())
}

// VersionHistory 返回当前书籍 workspace 的版本历史。
func (a *App) VersionHistory(ctx context.Context, limit int) ([]book.VersionEntry, error) {
	return a.runtime().VersionHistory(ctx, limit)
}

func (s *WorkspaceRuntimeManager) VersionHistory(ctx context.Context, limit int) ([]book.VersionEntry, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return nil, ErrNoWorkspace
	}
	return versionService.History(limit)
}

// CreateVersion 创建一个手动版本。
func (a *App) CreateVersion(ctx context.Context, message string) (book.VersionCommandResult, error) {
	return a.runtime().CreateVersion(ctx, message)
}

func (s *WorkspaceRuntimeManager) CreateVersion(ctx context.Context, message string) (book.VersionCommandResult, error) {
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	versionService := a.versionService
	settings := versionAutoSettingsForConfig(a.cfg)
	a.mu.RUnlock()
	if workspace == "" || versionService == nil {
		return book.VersionCommandResult{}, ErrNoWorkspace
	}
	message = s.inferVersionMessage(ctx, message, book.VersionSourceManual, versionService, settings)

	// Message inference may call an LLM, so it intentionally happens outside
	// the App lease. Recheck the captured identity before taking the workspace
	// write lease to avoid committing the old summary into a newly selected book.
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspace != workspace || a.versionService != versionService {
		return book.VersionCommandResult{}, ErrWorkspaceChanged
	}
	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	settings = versionAutoSettingsForConfig(a.cfg)
	var result book.VersionCommandResult
	err = changeService.WithConsistentWorkspaceSnapshot(ctx, func() error {
		var createErr error
		result, createErr = versionService.Create(message, book.VersionSourceManual, settings)
		return createErr
	})
	return result, err
}

// VersionDiff 返回目标版本与当前工作区的差异。
func (a *App) VersionDiff(ctx context.Context, id, path string) (book.VersionDiff, error) {
	return a.runtime().VersionDiff(ctx, id, path)
}

func (s *WorkspaceRuntimeManager) VersionDiff(ctx context.Context, id, path string) (book.VersionDiff, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionDiff{}, ErrNoWorkspace
	}
	return versionService.Diff(id, path)
}

// VersionRestorePlan 返回恢复版本前的影响预览。
func (a *App) VersionRestorePlan(ctx context.Context, id string, paths []string) (book.VersionRestorePlan, error) {
	return a.runtime().VersionRestorePlan(ctx, id, paths)
}

func (s *WorkspaceRuntimeManager) VersionRestorePlan(ctx context.Context, id string, paths []string) (book.VersionRestorePlan, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionRestorePlan{}, ErrNoWorkspace
	}
	return versionService.RestorePlan(id, paths, s.versionAutoSettings())
}

// RestoreVersion 将整本书或指定文件恢复到目标版本。
func (a *App) RestoreVersion(ctx context.Context, id string, paths ...[]string) (book.VersionRestoreResult, error) {
	return a.runtime().RestoreVersion(ctx, id, paths...)
}

func (s *WorkspaceRuntimeManager) RestoreVersion(ctx context.Context, id string, paths ...[]string) (book.VersionRestoreResult, error) {
	selectedPaths := restoreRequestPaths(paths)
	var result book.VersionRestoreResult
	err := s.withExclusiveWorkspaceMutation(ctx, func(runtime workspaceMutationRuntime) error {
		var restoreErr error
		result, restoreErr = runtime.versionService.RestoreWithPaths(id, selectedPaths, runtime.versionSettings)
		if restoreErr != nil {
			return restoreErr
		}
		scheduleAutoVersion(runtime.versionService, runtime.versionSettings)
		return nil
	})
	return result, err
}

func restoreRequestPaths(paths [][]string) []string {
	if len(paths) == 0 {
		return nil
	}
	return paths[0]
}

// ScheduleAutoVersion marks the current workspace for an idle automatic version.
func (a *App) ScheduleAutoVersion(ctx context.Context) {
	a.runtime().ScheduleAutoVersion(ctx)
}

func (s *WorkspaceRuntimeManager) ScheduleAutoVersion(ctx context.Context) {
	_ = ctx
	scheduleAutoVersion(s.versionService(), s.versionAutoSettings())
}

func scheduleAutoVersion(versionService *book.VersionService, settings book.VersionAutoSettings) {
	if versionService == nil {
		return
	}
	versionService.ScheduleAutoVersion(settings)
}

func (s *WorkspaceRuntimeManager) versionService() *book.VersionService {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.versionService
}

func (s *WorkspaceRuntimeManager) versionAutoSettings() book.VersionAutoSettings {
	a := s.app
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	return versionAutoSettingsForConfig(cfg)
}

func versionAutoSettingsForConfig(cfg *config.Config) book.VersionAutoSettings {
	settings := book.DefaultVersionAutoSettings()
	if cfg == nil {
		return settings
	}
	settings.TimedEnabled = cfg.VersionTimedEnabled
	settings.TimedIntervalMinutes = cfg.VersionTimedIntervalMinutes
	return settings
}

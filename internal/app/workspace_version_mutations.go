package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	projectfilesapp "denova/internal/app/projectfiles"
	appsettings "denova/internal/app/settings"
	"denova/internal/book"
)

// ProjectFileMutationVersioning binds Project file mutations to the shared
// Writing scheduler when available and otherwise supplies Project settings.
func (a *App) ProjectFileMutationVersioning(projectID, workspace, stateRoot string) projectfilesapp.MutationVersioning {
	fallback := projectfilesapp.MutationVersioning{Settings: book.DefaultVersionAutoSettings()}
	if a == nil {
		return fallback
	}
	a.mu.RLock()
	if a.cfg == nil || strings.TrimSpace(projectID) == "" {
		a.mu.RUnlock()
		return fallback
	}
	cfg := *a.cfg
	if a.versionService != nil && a.cfg.ProjectID == projectID && filepath.Clean(a.workspace) == filepath.Clean(workspace) {
		versioning := projectfilesapp.MutationVersioning{
			Service:  a.versionService,
			Settings: versionAutoSettingsForConfig(a.cfg),
		}
		a.mu.RUnlock()
		return versioning
	}
	a.mu.RUnlock()
	refreshed, err := appsettings.RefreshProject(cfg, workspace, stateRoot)
	if err != nil {
		slog.WarnContext(context.Background(), "[app/projectfiles] failed to load Project version settings; using runtime defaults",
			"project_id", projectID,
			"workspace", workspace,
			"error", err,
		)
		return fallback
	}
	cfg = refreshed
	return projectfilesapp.MutationVersioning{
		Settings: versionAutoSettingsForConfig(&cfg),
	}
}

// VersionStatus 返回当前书籍 workspace 的本地版本状态。
func (a *App) VersionStatus(ctx context.Context) (book.VersionStatus, error) {
	return a.workspaceService().VersionStatus(ctx)
}

func (s *workspaceService) VersionStatus(ctx context.Context) (book.VersionStatus, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionStatus{}, ErrNoWorkspace
	}
	return versionService.Status(s.versionAutoSettings())
}

// VersionHistory 返回当前书籍 workspace 的版本历史。
func (a *App) VersionHistory(ctx context.Context, limit int) ([]book.VersionEntry, error) {
	return a.workspaceService().VersionHistory(ctx, limit)
}

func (s *workspaceService) VersionHistory(ctx context.Context, limit int) ([]book.VersionEntry, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return nil, ErrNoWorkspace
	}
	return versionService.History(limit)
}

// CreateVersion 创建一个手动版本。
func (a *App) CreateVersion(ctx context.Context, message string) (book.VersionCommandResult, error) {
	return a.workspaceService().CreateVersion(ctx, message)
}

func (s *workspaceService) CreateVersion(ctx context.Context, message string) (book.VersionCommandResult, error) {
	runtime, err := s.acquireVersionCreateRuntime(ctx)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	defer runtime.Release()

	message, err = s.inferVersionMessage(runtime.Context(), message, book.VersionSourceManual, runtime)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	if err := runtime.Context().Err(); err != nil {
		return book.VersionCommandResult{}, err
	}
	if !runtime.matches(s.app) {
		return book.VersionCommandResult{}, ErrWorkspaceChanged
	}
	changeService, err := workspaceChangeService(runtime.workspace, runtime.cfg.ProjectStateDir)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	var result book.VersionCommandResult
	err = changeService.WithConsistentWorkspaceSnapshot(runtime.Context(), func() error {
		var createErr error
		result, createErr = runtime.versionService.Create(message, book.VersionSourceManual, runtime.settings)
		return createErr
	})
	return result, err
}

// VersionDiff returns the requested comparison for one saved version.
func (a *App) VersionDiff(ctx context.Context, id, path string, comparison book.VersionDiffComparison) (book.VersionDiff, error) {
	return a.workspaceService().VersionDiff(ctx, id, path, comparison)
}

func (s *workspaceService) VersionDiff(ctx context.Context, id, path string, comparison book.VersionDiffComparison) (book.VersionDiff, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionDiff{}, ErrNoWorkspace
	}
	return versionService.Diff(id, path, comparison)
}

// VersionRestorePlan 返回恢复版本前的影响预览。
func (a *App) VersionRestorePlan(ctx context.Context, id string, paths []string) (book.VersionRestorePlan, error) {
	return a.workspaceService().VersionRestorePlan(ctx, id, paths)
}

func (s *workspaceService) VersionRestorePlan(ctx context.Context, id string, paths []string) (book.VersionRestorePlan, error) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionRestorePlan{}, ErrNoWorkspace
	}
	return versionService.RestorePlan(id, paths, s.versionAutoSettings())
}

// RestoreVersion 将整本书或指定文件恢复到目标版本。
func (a *App) RestoreVersion(ctx context.Context, id string, paths ...[]string) (book.VersionRestoreResult, error) {
	return a.workspaceService().RestoreVersion(ctx, id, paths...)
}

func (s *workspaceService) RestoreVersion(ctx context.Context, id string, paths ...[]string) (book.VersionRestoreResult, error) {
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
	a.workspaceService().ScheduleAutoVersion(ctx)
}

func (s *workspaceService) ScheduleAutoVersion(ctx context.Context) {
	_ = ctx
	scheduleAutoVersion(s.versionService(), s.versionAutoSettings())
}

func scheduleAutoVersion(versionService *book.VersionService, settings book.VersionAutoSettings) {
	if versionService == nil {
		return
	}
	versionService.ScheduleAutoVersion(settings)
}

func (s *workspaceService) versionService() *book.VersionService {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.versionService
}

func (s *workspaceService) versionAutoSettings() book.VersionAutoSettings {
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

// ProjectVersionStatus returns version state for one stable Book Project.
func (a *App) ProjectVersionStatus(ctx context.Context, projectID string) (book.VersionStatus, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return book.VersionStatus{}, err
	}
	defer operation.Release()
	return a.ProjectFiles().VersionStatus(operation.Context(), operation.Layout().ProjectID)
}

func (a *App) ProjectVersionHistory(ctx context.Context, projectID string, limit int) ([]book.VersionEntry, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	if err := operation.Context().Err(); err != nil {
		return nil, err
	}
	return a.ProjectFiles().VersionHistory(operation.Layout().ProjectID, limit)
}

func (a *App) CreateProjectVersion(ctx context.Context, projectID, message string) (book.VersionCommandResult, error) {
	runtime, err := a.acquireProjectVersionCreateRuntime(ctx, projectID)
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	defer runtime.Release()

	message, err = a.inferVersionMessageForResources(runtime.operation.Context(), message, book.VersionSourceManual, versionSummaryResources{
		workspace: runtime.resources.Workspace,
		cfg:       &runtime.cfg, bookService: runtime.resources.Files,
		versionService: runtime.resources.VersionService, sessionStore: runtime.sessionStore,
		settings: runtime.resources.Settings,
	})
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	if err := runtime.operation.Context().Err(); err != nil {
		return book.VersionCommandResult{}, err
	}
	return a.ProjectFiles().CreateVersion(
		runtime.operation.Context(), runtime.resources.ProjectID, message, book.VersionSourceManual,
	)
}

func (a *App) ProjectVersionDiff(ctx context.Context, projectID, versionID, path string, comparison book.VersionDiffComparison) (book.VersionDiff, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return book.VersionDiff{}, err
	}
	defer operation.Release()
	return a.ProjectFiles().VersionDiff(operation.Context(), operation.Layout().ProjectID, versionID, path, comparison)
}

func (a *App) ProjectVersionRestorePlan(ctx context.Context, projectID, versionID string, paths []string) (book.VersionRestorePlan, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return book.VersionRestorePlan{}, err
	}
	defer operation.Release()
	return a.ProjectFiles().VersionRestorePlan(operation.Context(), operation.Layout().ProjectID, versionID, paths)
}

func (a *App) RestoreProjectVersion(ctx context.Context, projectID, versionID string, paths []string) (book.VersionRestoreResult, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return book.VersionRestoreResult{}, err
	}
	defer operation.Release()
	result, err := a.ProjectFiles().RestoreVersion(operation.Context(), operation.Layout().ProjectID, versionID, paths)
	if err != nil {
		return book.VersionRestoreResult{}, err
	}
	if len(result.RestoredPaths) > 0 {
		a.Automation().CheckTriggersAfterProjectMutation(
			operation.Context(), operation.Layout().ProjectID, "version_restore", result.RestoredPaths,
		)
	}
	return result, nil
}

type projectVersionCreateRuntime struct {
	operation        *ProjectOperation
	resources        projectfilesapp.VersionResources
	cfg              config.Config
	sessionStore     *session.Store
	ownsSessionStore bool
}

func (a *App) acquireProjectVersionCreateRuntime(ctx context.Context, projectID string) (*projectVersionCreateRuntime, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	releaseWithError := func(err error) (*projectVersionCreateRuntime, error) {
		operation.Release()
		return nil, err
	}
	resources, err := a.ProjectFiles().ProjectVersions(operation.Layout().ProjectID)
	if err != nil {
		return releaseWithError(err)
	}

	a.mu.RLock()
	var runtimeConfig config.Config
	if a.cfg != nil {
		runtimeConfig = *a.cfg
	}
	var store *session.Store
	if a.cfg != nil && a.cfg.ProjectID == resources.ProjectID {
		store = a.sessionStore
	}
	a.mu.RUnlock()
	if runtimeConfig.DataDir() == "" {
		return releaseWithError(fmt.Errorf("application data directory is unavailable"))
	}
	runtimeConfig.Workspace = resources.Workspace
	runtimeConfig.ProjectID = resources.ProjectID
	runtimeConfig.ProjectStateDir = resources.StateRoot
	runtimeConfig, err = appsettings.RefreshProject(runtimeConfig, resources.Workspace, resources.StateRoot)
	if err != nil {
		return releaseWithError(fmt.Errorf("load Project settings for version creation: %w", err))
	}
	runtime := &projectVersionCreateRuntime{
		operation: operation, resources: resources, cfg: runtimeConfig, sessionStore: store,
	}
	if runtime.sessionStore == nil {
		runtime.sessionStore, err = session.NewStore(operation.Layout().SessionsDir())
		if err != nil {
			return releaseWithError(fmt.Errorf("open Project Agent session store: %w", err))
		}
		runtime.ownsSessionStore = true
	}
	return runtime, nil
}

func (runtime *projectVersionCreateRuntime) Release() {
	if runtime == nil {
		return
	}
	if runtime.ownsSessionStore && runtime.sessionStore != nil {
		_ = runtime.sessionStore.Close()
	}
	if runtime.operation != nil {
		runtime.operation.Release()
	}
}

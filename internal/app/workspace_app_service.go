package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	projectdomain "denova/internal/project"
	"denova/internal/workspace/filewatch"
)

// workspaceService coordinates the active workspace, Book catalog, versions,
// and settings while App remains the API-facing facade.
type workspaceService struct {
	app *App
}

// HasWorkspace 返回是否已绑定 workspace。
func (a *App) HasWorkspace() bool {
	return a.workspaceService().HasWorkspace()
}

func (s *workspaceService) HasWorkspace() bool {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace != ""
}

// Workspace 返回当前 workspace。
func (a *App) Workspace() string {
	return a.workspaceService().Workspace()
}

func (s *workspaceService) Workspace() string {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace
}

// BookService 返回当前作品文件服务。
func (a *App) BookService() *book.Service {
	return a.workspaceService().BookService()
}

func (s *workspaceService) BookService() *book.Service {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookService
}

// Session 返回当前会话。
func (a *App) Session() *session.Session {
	return a.workspaceService().Session()
}

func (s *workspaceService) Session() *session.Session {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.session
}

// ChatService 返回聊天服务。
func (a *App) ChatService() *agentharness.Service {
	return a.workspaceService().ChatService()
}

func (s *workspaceService) ChatService() *agentharness.Service {
	return s.app.chatService
}

// SwitchWorkspace 切换工作区，并重建状态、会话和 Agent Runner。
func (a *App) SwitchWorkspace(ctx context.Context, path string) (string, error) {
	return a.workspaceService().SwitchWorkspace(ctx, path)
}

func (s *workspaceService) SwitchWorkspace(ctx context.Context, path string) (string, error) {
	a := s.app
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("路径无效: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("目录不存在: %s", absPath)
	}
	tasks, scopes, currentWorkspace, err := a.beginWorkspaceTransitionTo(absPath)
	if err != nil {
		return "", err
	}
	defer a.endWorkspaceTransition()
	if err := abortAndWaitTasks(ctx, tasks, currentWorkspace); err != nil {
		// Cancellation stops this caller's wait, not the transition owner. Keep
		// the generations fenced until every previously admitted lease drains,
		// then reinstall a usable generation for the still-current runtime.
		drainErr := waitLifecycleScopes(context.Background(), scopes)
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		if drainErr != nil {
			return "", errors.Join(err, drainErr)
		}
		return "", err
	}
	// Director jobs write story/lore projections outside the foreground Task
	// registry. Quiesce them before buildRuntime reads or initializes either the
	// current or target workspace, including same-path runtime refreshes.
	a.stopWorkspaceDirectorTasks()
	if err := waitLifecycleScopes(context.Background(), scopes); err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	if err := a.closeWorkspaceRuntimeBindings(context.Background(), currentWorkspace, absPath); err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	projectRecord, err := a.projectRegistry.EnsureBook(absPath)
	if err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	layout, err := a.projectRegistry.EnsureState(projectRecord)
	if err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	runtime, err := buildRuntimeExclusively(ctx, a.cfg, layout)
	if err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}

	chatApp := a.chat()
	a.mu.Lock()
	if err := a.replaceWorkspaceScopeLocked(runtime.workspace); err != nil {
		a.mu.Unlock()
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace, absPath)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	if oldKey := lifecycleWorkspaceKey(currentWorkspace); oldKey != "" && oldKey != lifecycleWorkspaceKey(runtime.workspace) {
		delete(a.workspaceScopes, oldKey)
	}
	previousVersionService := a.versionService
	previousInteractiveStore := a.interactive
	previousSessionStore := a.sessionStore
	a.applyRuntime(runtime)
	a.cfg.Workspace = runtime.workspace
	chatApp.clearRecoveryRefreshObligations(runtime.workspace)
	a.mu.Unlock()
	a.syncWorkspaceFileWatcher(runtime.workspace)
	if previousInteractiveStore != nil && previousInteractiveStore != runtime.interactive {
		if closeErr := previousInteractiveStore.Close(); closeErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[app] flush previous interactive journals failed workspace=%s err=%v", currentWorkspace, closeErr))
		}
	}
	if previousSessionStore != nil && previousSessionStore != runtime.sessionStore {
		if closeErr := previousSessionStore.Close(); closeErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[app] flush previous session journals failed workspace=%s err=%v", currentWorkspace, closeErr))
		}
	}
	if previousVersionService != nil && previousVersionService != runtime.versionService {
		previousVersionService.Close()
	}

	_, _ = a.projectRegistry.TouchBook(runtime.workspace)
	return runtime.workspace, nil
}

// RemoveBook 移除书籍记录，不删除磁盘目录。
func (a *App) RemoveBook(path string) (string, error) {
	return a.workspaceService().RemoveBook(path)
}

func (s *workspaceService) RemoveBook(path string) (string, error) {
	a := s.app
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("路径无效: %w", err)
	}
	wasCurrent := a.Workspace() == absPath
	record, found, err := a.projectRegistry.FindByPath(absPath, false)
	if err != nil {
		return "", err
	}
	if found && record.Type == projectdomain.TypeBook {
		if _, err := a.AgentChat().ArchiveProject(record.ID); err != nil {
			return "", err
		}
	}
	if wasCurrent {
		return s.activateFallbackWorkspace(context.Background())
	}
	return a.Workspace(), nil
}

func (s *workspaceService) activateFallbackWorkspace(ctx context.Context) (string, error) {
	a := s.app
	records, err := a.projectRegistry.Books()
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.WorkspacePath == "" {
			continue
		}
		workspace, err := s.SwitchWorkspace(ctx, record.WorkspacePath)
		if err == nil {
			return workspace, nil
		}
		slog.ErrorContext(ctx, fmt.Sprintf("[app/runtime_manager.go] switch to fallback Book failed path=%s err=%v", record.WorkspacePath, err))
	}
	tasks, scopes, currentWorkspace, err := a.beginWorkspaceTransitionTo()
	if err != nil {
		return "", err
	}
	defer a.endWorkspaceTransition()
	if err := abortAndWaitTasks(ctx, tasks, currentWorkspace); err != nil {
		drainErr := waitLifecycleScopes(context.Background(), scopes)
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace)
		if drainErr != nil {
			return "", errors.Join(err, drainErr)
		}
		return "", err
	}
	a.stopWorkspaceDirectorTasks()
	if err := waitLifecycleScopes(context.Background(), scopes); err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	if err := a.closeWorkspaceRuntimeBindings(context.Background(), currentWorkspace); err != nil {
		a.restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace)
		a.restoreWorkspaceDirectorTasks(currentWorkspace)
		return "", err
	}
	a.mu.Lock()
	previousVersionService := a.versionService
	previousInteractiveStore := a.interactive
	previousSessionStore := a.sessionStore
	delete(a.workspaceScopes, lifecycleWorkspaceKey(currentWorkspace))
	a.clearRuntime()
	a.mu.Unlock()
	a.syncWorkspaceFileWatcher("")
	if previousInteractiveStore != nil {
		if closeErr := previousInteractiveStore.Close(); closeErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[app] flush interactive journals while clearing workspace failed workspace=%s err=%v", currentWorkspace, closeErr))
		}
	}
	if previousSessionStore != nil {
		if closeErr := previousSessionStore.Close(); closeErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[app] flush session journals while clearing workspace failed workspace=%s err=%v", currentWorkspace, closeErr))
		}
	}
	if previousVersionService != nil {
		previousVersionService.Close()
	}
	return "", nil
}

// CreateBook 创建新书籍工作区：在 parentDir 下创建以 title 命名的子目录，初始化工作区结构和元信息，然后切换到该工作区。
func (a *App) CreateBook(ctx context.Context, parentDir, title, author, description string) (string, book.BookMeta, error) {
	return a.workspaceService().CreateBook(ctx, parentDir, title, author, description)
}

func (s *workspaceService) CreateBook(ctx context.Context, parentDir, title, author, description string) (string, book.BookMeta, error) {
	a := s.app
	novaDir := ""
	if a.cfg != nil {
		novaDir = strings.TrimSpace(a.cfg.DataDir())
	}
	absParent, err := projectdomain.BookCreationParent(parentDir, novaDir)
	if err != nil {
		return "", book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}

	dir := filepath.Join(absParent, title)
	if _, err := os.Stat(dir); err == nil {
		return "", book.BookMeta{}, fmt.Errorf("目录已存在: %s", dir)
	}
	if novaDir != "" {
		if absNovaDir, err := filepath.Abs(novaDir); err == nil && absParent == filepath.Join(absNovaDir, projectdomain.ContentDirectoryName) {
			legacyDir := filepath.Join(absNovaDir, title)
			legacyType, detectErr := projectdomain.DetectType(legacyDir)
			if legacyDir != dir && detectErr == nil && legacyType == projectdomain.TypeBook {
				return "", book.BookMeta{}, fmt.Errorf("目录已存在: %s", legacyDir)
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", book.BookMeta{}, fmt.Errorf("创建目录失败: %w", err)
	}

	state := book.NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		return "", book.BookMeta{}, fmt.Errorf("初始化工作目录失败: %w", err)
	}

	meta := book.BookMeta{Title: title, Author: author, Description: description}
	meta, err = a.bookMetaStore.Write(dir, meta)
	if err != nil {
		return "", book.BookMeta{}, fmt.Errorf("写入书籍元信息失败: %w", err)
	}

	if _, err := interactive.NewStore(dir).CreateStory(interactive.CreateStoryRequest{}); err != nil {
		return "", book.BookMeta{}, fmt.Errorf("初始化默认故事线失败: %w", err)
	}

	workspace, switchErr := s.SwitchWorkspace(ctx, dir)
	if switchErr != nil {
		return "", book.BookMeta{}, fmt.Errorf("切换工作区失败: %w", switchErr)
	}

	return workspace, meta, nil
}

// Status 返回当前作品状态摘要。
func (a *App) Status() (bool, string) {
	return a.workspaceService().Status()
}

func (s *workspaceService) Status() (bool, string) {
	a := s.app
	a.mu.RLock()
	state := a.bookState
	a.mu.RUnlock()
	if state == nil {
		return false, ""
	}
	return state.HasState(), state.CompactContext()
}

// syncWorkspaceFileWatcher follows the committed runtime workspace. Watcher
// failure is non-fatal because visibility refresh remains authoritative.
func (a *App) syncWorkspaceFileWatcher(workspace string) {
	if a == nil || a.workspaceFiles == nil {
		return
	}
	if err := a.workspaceFiles.SetWorkspace(workspace); err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[filewatch] workspace watcher unavailable; foreground refresh remains active workspace=%q err=%v", workspace, err))
		return
	}
	if workspace != "" {
		slog.InfoContext(context.Background(), fmt.Sprintf("[filewatch] workspace watcher active workspace=%q", workspace))
	}
}

// SubscribeWorkspaceFileChanges returns ephemeral invalidations. Every
// subscription begins with resync, so clients re-read canonical state.
func (a *App) SubscribeWorkspaceFileChanges() (<-chan filewatch.Event, func()) {
	if a == nil || a.workspaceFiles == nil {
		closed := make(chan filewatch.Event)
		close(closed)
		return closed, func() {}
	}
	return a.workspaceFiles.Subscribe()
}

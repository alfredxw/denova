package app

import (
	"context"
	agentexecution "denova/internal/agents/execution"
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

// ProjectID returns the stable Project identity bound to the active workspace.
func (a *App) ProjectID() string {
	return a.workspaceService().ProjectID()
}

func (s *workspaceService) ProjectID() string {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return ""
	}
	return a.cfg.ProjectID
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

// ExecutionRuntime 返回聊天服务。
func (a *App) ExecutionRuntime() *agentexecution.Runtime {
	return a.workspaceService().ExecutionRuntime()
}

func (s *workspaceService) ExecutionRuntime() *agentexecution.Runtime {
	return s.app.executionRuntime
}

// SwitchWorkspace switches the workspace and rebuilds state, sessions, and the Agent runtime.
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
	a.mu.Unlock()
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
	resultWorkspace := a.Workspace()
	if wasCurrent && found && record.Type == projectdomain.TypeBook {
		resultWorkspace, err = s.activateFallbackWorkspaceExcluding(context.Background(), record.ID)
		if err != nil {
			return "", err
		}
	}
	if found && record.Type == projectdomain.TypeBook {
		if _, err := a.ArchiveProject(context.Background(), record.ID); err != nil {
			return "", err
		}
	}
	return resultWorkspace, nil
}

func (s *workspaceService) activateFallbackWorkspace(ctx context.Context) (string, error) {
	return s.activateFallbackWorkspaceExcluding(ctx, "")
}

func (s *workspaceService) activateFallbackWorkspaceExcluding(ctx context.Context, excludedProjectID string) (string, error) {
	a := s.app
	records, err := a.projectRegistry.Books()
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.WorkspacePath == "" || record.ID == excludedProjectID {
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

// BookCreationResult returns the stable identity together with the new content
// directory. Callers never need to rediscover the created Project through the
// mutable foreground Book.
type BookCreationResult struct {
	ProjectID string
	Workspace string
	Meta      book.BookMeta
}

// CreateBook creates and selects a new Book Project below parentDir.
func (a *App) CreateBook(ctx context.Context, parentDir, title, author, description string) (BookCreationResult, error) {
	return a.workspaceService().CreateBook(ctx, parentDir, title, author, description)
}

func (s *workspaceService) CreateBook(ctx context.Context, parentDir, title, author, description string) (BookCreationResult, error) {
	a := s.app
	novaDir := ""
	if a.cfg != nil {
		novaDir = strings.TrimSpace(a.cfg.DataDir())
	}
	absParent, err := projectdomain.BookCreationParent(parentDir, novaDir)
	if err != nil {
		return BookCreationResult{}, fmt.Errorf("路径无效: %w", err)
	}

	dir := filepath.Join(absParent, title)
	if _, err := os.Stat(dir); err == nil {
		return BookCreationResult{}, fmt.Errorf("目录已存在: %s", dir)
	}
	if novaDir != "" {
		if absNovaDir, err := filepath.Abs(novaDir); err == nil && absParent == filepath.Join(absNovaDir, projectdomain.ContentDirectoryName) {
			legacyDir := filepath.Join(absNovaDir, title)
			legacyType, detectErr := projectdomain.DetectType(legacyDir)
			if legacyDir != dir && detectErr == nil && legacyType == projectdomain.TypeBook {
				return BookCreationResult{}, fmt.Errorf("目录已存在: %s", legacyDir)
			}
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return BookCreationResult{}, fmt.Errorf("创建目录失败: %w", err)
	}

	state := book.NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		return BookCreationResult{}, fmt.Errorf("初始化工作目录失败: %w", err)
	}

	meta := book.BookMeta{Title: title, Author: author, Description: description}
	meta, err = a.bookMetaStore.Write(dir, meta)
	if err != nil {
		return BookCreationResult{}, fmt.Errorf("写入书籍元信息失败: %w", err)
	}

	if _, err := interactive.NewStore(dir).CreateStory(interactive.CreateStoryRequest{}); err != nil {
		return BookCreationResult{}, fmt.Errorf("初始化默认故事线失败: %w", err)
	}

	workspace, switchErr := s.SwitchWorkspace(ctx, dir)
	if switchErr != nil {
		return BookCreationResult{}, fmt.Errorf("切换工作区失败: %w", switchErr)
	}
	project, _, err := a.resolveProjectByWorkspace(workspace)
	if err != nil {
		return BookCreationResult{}, fmt.Errorf("resolve created Book Project: %w", err)
	}
	return BookCreationResult{ProjectID: project.ID, Workspace: workspace, Meta: meta}, nil
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
	return state.HasState(), state.WorkspaceContext().Markdown()
}

// SubscribeProjectFileChanges returns ephemeral invalidations for one stable
// Project identity. Every subscription begins with resync.
func (a *App) SubscribeProjectFileChanges(projectID string) (<-chan filewatch.Event, func(), error) {
	if a == nil || a.workspaceFiles == nil {
		closed := make(chan filewatch.Event)
		close(closed)
		return closed, func() {}, nil
	}
	_, layout, err := a.resolveProject(projectID, true)
	if err != nil {
		closed := make(chan filewatch.Event)
		close(closed)
		return closed, func() {}, err
	}
	return a.workspaceFiles.Subscribe(layout.ProjectID, layout.ContentRoot)
}

func (a *App) observeProjectFileChange(event filewatch.Event) {
	if a == nil {
		return
	}
	a.mu.RLock()
	activeFiles := a.bookService
	activeState := a.bookState
	activeWorkspace := a.workspace
	projectBook := a.projectBook
	agentChat := a.agentChatApp
	a.mu.RUnlock()

	if filepath.Clean(activeWorkspace) == filepath.Clean(event.Workspace) {
		if activeFiles != nil {
			activeFiles.InvalidateSummary(event.Paths, event.Resync)
		}
		if activeState != nil {
			activeState.InvalidateChapterPaths(event.Paths, event.Resync)
		}
	}
	if projectBook != nil {
		projectBook.InvalidateSummary(event.ProjectID, event.Paths, event.Resync)
	}
	if agentChat != nil {
		agentChat.InvalidateBookSummary(event.ProjectID, event.Paths, event.Resync)
	}
}

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

// WorkspaceRuntimeManager 负责工作区运行时、书籍元信息、Git 与设置等跨领域基础能力。
type WorkspaceRuntimeManager struct {
	app *App
}

// HasWorkspace 返回是否已绑定 workspace。
func (a *App) HasWorkspace() bool {
	return a.runtime().HasWorkspace()
}

func (s *WorkspaceRuntimeManager) HasWorkspace() bool {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace != ""
}

// Workspace 返回当前 workspace。
func (a *App) Workspace() string {
	return a.runtime().Workspace()
}

func (s *WorkspaceRuntimeManager) Workspace() string {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace
}

// BookState 返回当前作品状态管理器。
func (a *App) BookState() *book.State {
	return a.runtime().BookState()
}

func (s *WorkspaceRuntimeManager) BookState() *book.State {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookState
}

// BookService 返回当前作品文件服务。
func (a *App) BookService() *book.Service {
	return a.runtime().BookService()
}

func (s *WorkspaceRuntimeManager) BookService() *book.Service {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookService
}

// Session 返回当前会话。
func (a *App) Session() *session.Session {
	return a.runtime().Session()
}

func (s *WorkspaceRuntimeManager) Session() *session.Session {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.session
}

// Runner 返回当前 Agent Runner。
func (a *App) Runner() *adk.Runner {
	return a.runtime().Runner()
}

func (s *WorkspaceRuntimeManager) Runner() *adk.Runner {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentRunner
}

// ChatService 返回聊天服务。
func (a *App) ChatService() *agent.ChatService {
	return a.runtime().ChatService()
}

func (s *WorkspaceRuntimeManager) ChatService() *agent.ChatService {
	return s.app.chatService
}

// SwitchWorkspace 切换工作区，并重建状态、会话和 Agent Runner。
func (a *App) SwitchWorkspace(ctx context.Context, path string) (string, error) {
	return a.runtime().SwitchWorkspace(ctx, path)
}

func (s *WorkspaceRuntimeManager) SwitchWorkspace(ctx context.Context, path string) (string, error) {
	a := s.app
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("路径无效: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("目录不存在: %s", absPath)
	}

	runtime, err := buildRuntime(ctx, a.cfg, absPath)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.applyRuntime(runtime)
	a.cfg.Workspace = runtime.workspace
	a.mu.Unlock()

	_ = a.bookRegistry.Touch(runtime.workspace)
	return runtime.workspace, nil
}

// Books 返回最近打开的书籍工作目录，并从 book.json 填充元信息。
func (a *App) Books() []BookRecord {
	return a.runtime().Books()
}

func (s *WorkspaceRuntimeManager) Books() []BookRecord {
	a := s.app
	records := a.bookRegistry.List()
	for i := range records {
		meta, err := a.bookMetaStore.Read(records[i].Path)
		if err != nil {
			continue
		}
		if meta.Title != "" {
			records[i].Name = meta.Title
		}
		records[i].Author = meta.Author
	}
	return records
}

// BookInfo 读取指定路径工作区的书籍元信息。
func (a *App) BookInfo(path string) (book.BookMeta, error) {
	return a.runtime().BookInfo(path)
}

func (s *WorkspaceRuntimeManager) BookInfo(path string) (book.BookMeta, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}
	return s.app.bookMetaStore.Read(absPath)
}

// UpdateBookInfo 更新指定路径工作区的书籍元信息。
func (a *App) UpdateBookInfo(path string, title, author, description string) (book.BookMeta, error) {
	return a.runtime().UpdateBookInfo(path, title, author, description)
}

func (s *WorkspaceRuntimeManager) UpdateBookInfo(path string, title, author, description string) (book.BookMeta, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}
	meta, err := s.app.bookMetaStore.Read(absPath)
	if err != nil {
		return book.BookMeta{}, err
	}
	if title != "" {
		meta.Title = title
	}
	if author != "" {
		meta.Author = author
	}
	// description 允许设为空字符串（清除简介），所以总是更新。
	meta.Description = description
	return s.app.bookMetaStore.Write(absPath, meta)
}

// RemoveBook 移除书籍记录，不删除磁盘目录。
func (a *App) RemoveBook(path string) error {
	return a.runtime().RemoveBook(path)
}

func (s *WorkspaceRuntimeManager) RemoveBook(path string) error {
	return s.app.bookRegistry.Remove(path)
}

// CreateBook 创建新书籍工作区：在 parentDir 下创建以 title 命名的子目录，初始化工作区结构和元信息，然后切换到该工作区。
func (a *App) CreateBook(ctx context.Context, parentDir, title, author, description string) (string, book.BookMeta, error) {
	return a.runtime().CreateBook(ctx, parentDir, title, author, description)
}

func (s *WorkspaceRuntimeManager) CreateBook(ctx context.Context, parentDir, title, author, description string) (string, book.BookMeta, error) {
	a := s.app
	absParent, err := filepath.Abs(parentDir)
	if err != nil {
		return "", book.BookMeta{}, fmt.Errorf("路径无效: %w", err)
	}

	dir := filepath.Join(absParent, title)
	if _, err := os.Stat(dir); err == nil {
		return "", book.BookMeta{}, fmt.Errorf("目录已存在: %s", dir)
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

// GitStatus 返回当前书籍 workspace 的 Git 状态。
func (a *App) GitStatus(ctx context.Context) (book.GitStatus, error) {
	return a.runtime().GitStatus(ctx)
}

func (s *WorkspaceRuntimeManager) GitStatus(ctx context.Context) (book.GitStatus, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitStatus{}, ErrNoWorkspace
	}
	return gitService.Status(ctx)
}

// GitHistory 返回当前书籍 workspace 的 Git 提交历史。
func (a *App) GitHistory(ctx context.Context, limit int) ([]book.GitCommit, error) {
	return a.runtime().GitHistory(ctx, limit)
}

func (s *WorkspaceRuntimeManager) GitHistory(ctx context.Context, limit int) ([]book.GitCommit, error) {
	gitService := s.gitService()
	if gitService == nil {
		return nil, ErrNoWorkspace
	}
	return gitService.History(ctx, limit)
}

// GitDiff 返回当前工作区 Git diff。
func (a *App) GitDiff(ctx context.Context, path string) (string, error) {
	return a.runtime().GitDiff(ctx, path)
}

func (s *WorkspaceRuntimeManager) GitDiff(ctx context.Context, path string) (string, error) {
	gitService := s.gitService()
	if gitService == nil {
		return "", ErrNoWorkspace
	}
	return gitService.Diff(ctx, path)
}

// InitGit 初始化当前书籍 workspace 的 Git 仓库。
func (a *App) InitGit(ctx context.Context) (book.GitCommandResult, error) {
	return a.runtime().InitGit(ctx)
}

func (s *WorkspaceRuntimeManager) InitGit(ctx context.Context) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.Init(ctx)
}

// CreateGitVersion 创建一个书籍版本。
func (a *App) CreateGitVersion(ctx context.Context, message string) (book.GitCommandResult, error) {
	return a.runtime().CreateGitVersion(ctx, message)
}

func (s *WorkspaceRuntimeManager) CreateGitVersion(ctx context.Context, message string) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.CreateVersion(ctx, message)
}

// RollbackGitVersion 将整本书回滚到指定版本。
func (a *App) RollbackGitVersion(ctx context.Context, hash string) (book.GitCommandResult, error) {
	return a.runtime().RollbackGitVersion(ctx, hash)
}

func (s *WorkspaceRuntimeManager) RollbackGitVersion(ctx context.Context, hash string) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.Rollback(ctx, hash)
}

// StashGitChanges 暂存当前未提交内容。
func (a *App) StashGitChanges(ctx context.Context) (book.GitCommandResult, error) {
	return a.runtime().StashGitChanges(ctx)
}

func (s *WorkspaceRuntimeManager) StashGitChanges(ctx context.Context) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.Stash(ctx)
}

// PopGitStash 恢复最近一次暂存内容。
func (a *App) PopGitStash(ctx context.Context) (book.GitCommandResult, error) {
	return a.runtime().PopGitStash(ctx)
}

func (s *WorkspaceRuntimeManager) PopGitStash(ctx context.Context) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.PopStash(ctx)
}

// RunGitCommand 执行受限 Git 命令。
func (a *App) RunGitCommand(ctx context.Context, input string) (book.GitCommandResult, error) {
	return a.runtime().RunGitCommand(ctx, input)
}

func (s *WorkspaceRuntimeManager) RunGitCommand(ctx context.Context, input string) (book.GitCommandResult, error) {
	gitService := s.gitService()
	if gitService == nil {
		return book.GitCommandResult{}, ErrNoWorkspace
	}
	return gitService.RunCommand(ctx, input)
}

// Status 返回当前作品状态摘要。
func (a *App) Status() (bool, string) {
	return a.runtime().Status()
}

func (s *WorkspaceRuntimeManager) Status() (bool, string) {
	a := s.app
	a.mu.RLock()
	state := a.bookState
	a.mu.RUnlock()
	if state == nil {
		return false, ""
	}
	return state.HasState(), state.CompactContext()
}

// Settings 返回当前生效的分层配置快照。
func (a *App) Settings() (config.LayeredSettings, error) {
	return a.runtime().Settings()
}

func (s *WorkspaceRuntimeManager) Settings() (config.LayeredSettings, error) {
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.NovaDir
	}
	a.mu.RUnlock()
	return config.LoadLayered(novaDir, workspace)
}

// UpdateUserSettings 持久化用户级配置并返回最新分层快照。
func (a *App) UpdateUserSettings(settings config.Settings) (config.LayeredSettings, error) {
	return a.runtime().UpdateUserSettings(settings)
}

func (s *WorkspaceRuntimeManager) UpdateUserSettings(settings config.Settings) (config.LayeredSettings, error) {
	a := s.app
	a.mu.RLock()
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.NovaDir
	}
	a.mu.RUnlock()
	if err := config.WriteSettingsFile(config.UserConfigPath(novaDir), settings); err != nil {
		return config.LayeredSettings{}, err
	}
	return s.Settings()
}

// UpdateWorkspaceSettings 持久化当前工作区配置并返回最新分层快照。
func (a *App) UpdateWorkspaceSettings(settings config.Settings) (config.LayeredSettings, error) {
	return a.runtime().UpdateWorkspaceSettings(settings)
}

func (s *WorkspaceRuntimeManager) UpdateWorkspaceSettings(settings config.Settings) (config.LayeredSettings, error) {
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if workspace == "" {
		return config.LayeredSettings{}, fmt.Errorf("当前没有打开的工作区")
	}
	if err := config.WriteSettingsFile(config.WorkspaceConfigPath(workspace), settings); err != nil {
		return config.LayeredSettings{}, err
	}
	return s.Settings()
}

func (s *WorkspaceRuntimeManager) gitService() *book.GitService {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.gitService
}

type runtimeState struct {
	workspace              string
	bookState              *book.State
	bookService            *book.Service
	interactive            *interactive.Store
	sessionStore           *session.Store
	session                *session.Session
	agentRunner            *adk.Runner
	interactiveStoryRunner *adk.Runner
	gitService             *book.GitService
}

func buildRuntime(ctx context.Context, cfg *config.Config, workspace string) (*runtimeState, error) {
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败: %w", err)
	}

	state := book.NewState(absWorkspace)
	if err := state.InitWorkspace(); err != nil {
		return nil, fmt.Errorf("初始化工作目录失败: %w", err)
	}

	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		return nil, fmt.Errorf("创建会话存储失败: %w", err)
	}
	sess, err := store.GetActiveOrCreate()
	if err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	runtimeCfg := *cfg
	runtimeCfg.Workspace = absWorkspace
	agentRunner, err := buildAgentRunner(ctx, &runtimeCfg, state)
	if err != nil {
		return nil, err
	}
	interactiveStoryRunner, err := buildInteractiveStoryRunner(ctx, &runtimeCfg, state, prompts.InteractiveStorySystemInstructionInput{})
	if err != nil {
		return nil, err
	}

	return &runtimeState{
		workspace:              absWorkspace,
		bookState:              state,
		bookService:            book.NewService(absWorkspace),
		interactive:            interactive.NewStore(absWorkspace),
		sessionStore:           store,
		session:                sess,
		agentRunner:            agentRunner,
		interactiveStoryRunner: interactiveStoryRunner,
		gitService:             book.NewGitService(absWorkspace),
	}, nil
}

func buildAgentRunner(ctx context.Context, cfg *config.Config, state *book.State) (*adk.Runner, error) {
	builtAgent, err := agent.Build(ctx, cfg, state)
	if err != nil {
		return nil, fmt.Errorf("构建 Agent 失败: %w", err)
	}
	return agent.NewRunner(ctx, builtAgent), nil
}

func buildInteractiveStoryRunner(ctx context.Context, cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (*adk.Runner, error) {
	builtAgent, err := agent.BuildInteractiveStory(ctx, cfg, state, teller)
	if err != nil {
		return nil, fmt.Errorf("构建互动故事 Agent 失败: %w", err)
	}
	return agent.NewRunner(ctx, builtAgent), nil
}

func appSettingsInt(v *int, fallback int) int {
	if v == nil || *v <= 0 {
		return fallback
	}
	return *v
}

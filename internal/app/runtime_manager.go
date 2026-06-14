package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/session"
)

// WorkspaceRuntimeManager 负责工作区运行时、书籍元信息、本地版本服务与设置等跨领域基础能力。
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

// Books 返回当前 Nova 数据目录下实际存在的书籍工作目录，并从元信息存储填充展示信息。
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
func (a *App) RemoveBook(path string) (string, error) {
	return a.runtime().RemoveBook(path)
}

func (s *WorkspaceRuntimeManager) RemoveBook(path string) (string, error) {
	a := s.app
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("路径无效: %w", err)
	}
	wasCurrent := a.Workspace() == absPath
	if err := a.bookRegistry.Remove(absPath); err != nil {
		return "", err
	}
	if wasCurrent {
		return s.activateFallbackWorkspace(context.Background())
	}
	return a.Workspace(), nil
}

// ReorderBooks 保存书籍管理页的自定义排序。
func (a *App) ReorderBooks(paths []string) error {
	return a.runtime().ReorderBooks(paths)
}

func (s *WorkspaceRuntimeManager) ReorderBooks(paths []string) error {
	return s.app.bookRegistry.Reorder(paths)
}

func (s *WorkspaceRuntimeManager) activateFallbackWorkspace(ctx context.Context) (string, error) {
	a := s.app
	for _, record := range a.bookRegistry.List() {
		if record.Path == "" {
			continue
		}
		workspace, err := s.SwitchWorkspace(ctx, record.Path)
		if err == nil {
			return workspace, nil
		}
		log.Printf("[books] 切换删除后的备用书籍失败 path=%s err=%v", record.Path, err)
	}
	a.mu.Lock()
	a.clearRuntime()
	a.mu.Unlock()
	return "", nil
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
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionCommandResult{}, ErrNoWorkspace
	}
	settings := s.versionAutoSettings()
	message = s.inferVersionMessage(ctx, message, book.VersionSourceManual, versionService, settings)
	return versionService.Create(message, book.VersionSourceManual, settings)
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

// RestoreVersion 将整本书恢复到指定版本。
func (a *App) RestoreVersion(ctx context.Context, id string) (book.VersionCommandResult, error) {
	return a.runtime().RestoreVersion(ctx, id)
}

func (s *WorkspaceRuntimeManager) RestoreVersion(ctx context.Context, id string) (book.VersionCommandResult, error) {
	versionService := s.versionService()
	if versionService == nil {
		return book.VersionCommandResult{}, ErrNoWorkspace
	}
	result, err := versionService.Restore(id, s.versionAutoSettings())
	if err != nil {
		return book.VersionCommandResult{}, err
	}
	if timed, timedErr := versionService.MaybeCreateTimed(s.versionAutoSettings()); timedErr != nil {
		log.Printf("[versions] 恢复版本后定时保存检查失败 err=%v", timedErr)
	} else if !timed.Skipped && timed.Version != nil {
		log.Printf("[versions] 恢复版本后创建定时版本 id=%s", timed.Version.ID)
	}
	_ = ctx
	return result, nil
}

// MaybeCreateTimedVersion 在写操作后按定时策略创建自动版本。
func (a *App) MaybeCreateTimedVersion(ctx context.Context) {
	a.runtime().MaybeCreateTimedVersion(ctx)
}

func (s *WorkspaceRuntimeManager) MaybeCreateTimedVersion(ctx context.Context) {
	_ = ctx
	versionService := s.versionService()
	if versionService == nil {
		return
	}
	result, err := versionService.MaybeCreateTimed(s.versionAutoSettings())
	if err != nil {
		log.Printf("[versions] 定时自动保存失败 err=%v", err)
		return
	}
	if result.Skipped {
		log.Printf("[versions] 定时自动保存跳过 reason=%q", result.Reason)
		return
	}
	if result.Version != nil {
		log.Printf("[versions] 定时自动保存完成 id=%s", result.Version.ID)
	}
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
	path := config.UserConfigPath(novaDir)
	if err := config.WriteSettingsFile(path, settings); err != nil {
		return config.LayeredSettings{}, err
	}
	log.Printf("[settings] 用户配置已保存 path=%s", path)
	layered, err := s.Settings()
	if err != nil {
		return config.LayeredSettings{}, err
	}
	a.mu.Lock()
	applyLayeredSettingsToConfig(a.cfg, layered)
	a.mu.Unlock()
	return layered, nil
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
	path := config.WorkspaceConfigPath(workspace)
	if err := config.WriteSettingsFile(path, settings); err != nil {
		return config.LayeredSettings{}, err
	}
	log.Printf("[settings] 工作区配置已保存 path=%s", path)
	layered, err := s.Settings()
	if err != nil {
		return config.LayeredSettings{}, err
	}
	a.mu.Lock()
	applyLayeredSettingsToConfig(a.cfg, layered)
	a.mu.Unlock()
	return layered, nil
}

func applyLayeredSettingsToConfig(cfg *config.Config, layered config.LayeredSettings) {
	if cfg == nil {
		return
	}
	applySettingsLayerToConfig(cfg, layered.User)
	applySettingsLayerToConfig(cfg, layered.Workspace)

	effective := layered.Effective
	if cfg.OpenAIBaseURL == "" && effective.OpenAIBaseURL != "" {
		cfg.OpenAIBaseURL = effective.OpenAIBaseURL
	}
	if cfg.OpenAIModel == "" && effective.OpenAIModel != "" {
		cfg.OpenAIModel = effective.OpenAIModel
	}
	if len(effective.ModelProfiles) > 0 {
		cfg.ModelProfiles = effective.ModelProfiles
	}
	cfg.AgentModels = effective.AgentModels
	cfg.AgentTools = effective.AgentTools
	cfg.AgentPrompts = effective.AgentPrompts
	if cfg.SkillsDir == "" && effective.SkillsDir != "" {
		cfg.SkillsDir = effective.SkillsDir
	}
	if cfg.NovaDir == "" && layered.Paths.NovaDir != "" {
		cfg.NovaDir = layered.Paths.NovaDir
	}
	if cfg.IDEStoryTellerID == "" && effective.IDEStoryTellerID != "" {
		cfg.IDEStoryTellerID = effective.IDEStoryTellerID
	}
	if effective.MaxIteration != nil {
		cfg.MaxIteration = appSettingsInt(effective.MaxIteration, 50)
	}
	if effective.ModelMaxRetries != nil {
		cfg.ModelMaxRetries = appSettingsInt(effective.ModelMaxRetries, 5)
	}
	if effective.ChapterFilenameFormat != "" {
		cfg.ChapterFilenameFormat = effective.ChapterFilenameFormat
	}
	if effective.VolumeDirFormat != "" {
		cfg.VolumeDirFormat = effective.VolumeDirFormat
	}
	if effective.DraftFlowEnabled != nil {
		cfg.DraftFlowEnabled = *effective.DraftFlowEnabled
	}
	if effective.ChapterGroupMin != nil {
		cfg.ChapterGroupMin = appSettingsInt(effective.ChapterGroupMin, 3)
	}
	if effective.ChapterGroupMax != nil {
		cfg.ChapterGroupMax = appSettingsInt(effective.ChapterGroupMax, 8)
	}
	if effective.InteractiveMaxTokens != nil {
		cfg.InteractiveMaxTokens = appSettingsInt(effective.InteractiveMaxTokens, 0)
	}
	if effective.InteractiveHotChoices != nil {
		cfg.InteractiveHotChoices = *effective.InteractiveHotChoices
	}
	if effective.VersionTimedEnabled != nil {
		cfg.VersionTimedEnabled = *effective.VersionTimedEnabled
	}
	if effective.VersionTimedIntervalMinutes != nil {
		cfg.VersionTimedIntervalMinutes = appSettingsInt(effective.VersionTimedIntervalMinutes, 10)
	}
	if effective.VersionAgentEnabled != nil {
		cfg.VersionAgentEnabled = *effective.VersionAgentEnabled
	}
	if effective.VersionAgentCharThreshold != nil {
		cfg.VersionAgentCharThreshold = appSettingsInt(effective.VersionAgentCharThreshold, 3000)
	}
}

func applySettingsLayerToConfig(cfg *config.Config, settings config.Settings) {
	if settings.OpenAIAPIKey != "" && os.Getenv("OPENAI_API_KEY") == "" {
		cfg.OpenAIAPIKey = settings.OpenAIAPIKey
	}
	if settings.OpenAIBaseURL != "" && os.Getenv("OPENAI_BASE_URL") == "" {
		cfg.OpenAIBaseURL = settings.OpenAIBaseURL
	}
	if settings.OpenAIModel != "" && os.Getenv("OPENAI_MODEL") == "" {
		cfg.OpenAIModel = settings.OpenAIModel
	}
	if len(settings.ModelProfiles) > 0 {
		cfg.ModelProfiles = config.Merge(config.Settings{ModelProfiles: cfg.ModelProfiles}, config.Settings{ModelProfiles: settings.ModelProfiles}).ModelProfiles
	}
	cfg.AgentModels = config.MergeAgentModelSettings(cfg.AgentModels, settings.AgentModels)
	cfg.AgentTools = config.MergeAgentToolSettings(cfg.AgentTools, settings.AgentTools)
	cfg.AgentPrompts = config.MergeAgentPromptSettings(cfg.AgentPrompts, settings.AgentPrompts)
	if settings.SkillsDir != "" && os.Getenv("NOVA_SKILLS_DIR") == "" {
		cfg.SkillsDir = settings.SkillsDir
	}
	if settings.IDEStoryTellerID != "" {
		cfg.IDEStoryTellerID = settings.IDEStoryTellerID
	}
	if settings.MaxIteration != nil {
		cfg.MaxIteration = appSettingsInt(settings.MaxIteration, 50)
	}
	if settings.ModelMaxRetries != nil {
		cfg.ModelMaxRetries = appSettingsInt(settings.ModelMaxRetries, 5)
	}
	if settings.ChapterFilenameFormat != "" {
		cfg.ChapterFilenameFormat = settings.ChapterFilenameFormat
	}
	if settings.VolumeDirFormat != "" {
		cfg.VolumeDirFormat = settings.VolumeDirFormat
	}
	if settings.DraftFlowEnabled != nil {
		cfg.DraftFlowEnabled = *settings.DraftFlowEnabled
	}
	if settings.ChapterGroupMin != nil {
		cfg.ChapterGroupMin = appSettingsInt(settings.ChapterGroupMin, 3)
	}
	if settings.ChapterGroupMax != nil {
		cfg.ChapterGroupMax = appSettingsInt(settings.ChapterGroupMax, 8)
	}
	if settings.InteractiveMaxTokens != nil {
		cfg.InteractiveMaxTokens = appSettingsInt(settings.InteractiveMaxTokens, 0)
	}
	if settings.InteractiveHotChoices != nil {
		cfg.InteractiveHotChoices = *settings.InteractiveHotChoices
	}
	if settings.VersionTimedEnabled != nil {
		cfg.VersionTimedEnabled = *settings.VersionTimedEnabled
	}
	if settings.VersionTimedIntervalMinutes != nil {
		cfg.VersionTimedIntervalMinutes = appSettingsInt(settings.VersionTimedIntervalMinutes, 10)
	}
	if settings.VersionAgentEnabled != nil {
		cfg.VersionAgentEnabled = *settings.VersionAgentEnabled
	}
	if settings.VersionAgentCharThreshold != nil {
		cfg.VersionAgentCharThreshold = appSettingsInt(settings.VersionAgentCharThreshold, 3000)
	}
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
	settings := book.DefaultVersionAutoSettings()
	if cfg == nil {
		return settings
	}
	settings.TimedEnabled = cfg.VersionTimedEnabled
	settings.TimedIntervalMinutes = cfg.VersionTimedIntervalMinutes
	settings.AgentEnabled = cfg.VersionAgentEnabled
	settings.AgentCharThreshold = cfg.VersionAgentCharThreshold
	return settings
}

func appSettingsInt(v *int, fallback int) int {
	if v == nil || *v <= 0 {
		return fallback
	}
	return *v
}

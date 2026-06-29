package app

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/cloudwego/eino/adk"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/session"
)

// App 是 API 层使用的应用门面；具体业务由领域应用服务承接。
type App struct {
	cfg *config.Config

	workspace              string
	bookState              *book.State
	bookService            *book.Service
	interactive            *interactive.Store
	sessionStore           *session.Store
	session                *session.Session
	agentRunner            *adk.Runner
	interactiveStoryRunner *adk.Runner
	chatService            *agent.ChatService
	bookRegistry           *BookRegistry
	bookMetaStore          *BookMetaStore
	versionService         *book.VersionService
	activeTask             *Task
	activeInteractiveTask  *Task
	activeAutomationTasks  map[string]*Task
	activeAutomationRuns   map[string]automationRunState

	runtimeManager *WorkspaceRuntimeManager
	chatApp        *ChatAppService
	interactiveApp *InteractiveAppService
	loreApp        *LoreAppService
	configApp      *ConfigManagerAppService
	automationApp  *AutomationAppService
	skillsApp      *SkillsAppService
	imageApp       *ImageAppService
	servicesOnce   sync.Once

	mu sync.RWMutex
}

// New 创建应用运行时。当 workspace 为空且没有上次打开的 workspace 时，App 进入“无书籍”状态，
// 等待用户在前端书籍管理页选择或新建书籍后再构建 runtime。
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	registry := NewBookRegistry(cfg.NovaDir)
	bookMetaStore := NewBookMetaStore(cfg.NovaDir)
	workspace := cfg.Workspace
	if workspace == "" && cfg.ResumeLastWorkspace {
		if lastWorkspace := registry.Current(); lastWorkspace != "" {
			workspace = lastWorkspace
		}
	}

	app := &App{
		cfg:           cfg,
		chatService:   agent.NewChatService(),
		bookRegistry:  registry,
		bookMetaStore: bookMetaStore,
	}
	app.ensureServices()
	app.StartAutomationScheduler(ctx)

	if workspace == "" {
		log.Printf("[app] 启动时未指定 workspace 且无上次打开的书籍，进入无书籍状态，等待用户在前端选择")
		cfg.Workspace = ""
		return app, nil
	}

	runtime, err := buildRuntime(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = runtime.workspace
	_ = registry.Touch(runtime.workspace)

	app.applyRuntime(runtime)
	return app, nil
}

// ErrNoWorkspace 表示当前 App 尚未绑定任何书籍 workspace。
var ErrNoWorkspace = fmt.Errorf("尚未选择书籍工作区")

func (a *App) ensureServices() {
	a.servicesOnce.Do(func() {
		a.runtimeManager = &WorkspaceRuntimeManager{app: a}
		a.chatApp = &ChatAppService{app: a}
		a.interactiveApp = &InteractiveAppService{app: a}
		a.loreApp = &LoreAppService{app: a}
		a.configApp = &ConfigManagerAppService{app: a}
		a.automationApp = &AutomationAppService{app: a}
		a.skillsApp = &SkillsAppService{app: a}
		a.imageApp = &ImageAppService{app: a}
	})
}

func (a *App) runtime() *WorkspaceRuntimeManager {
	a.ensureServices()
	return a.runtimeManager
}

func (a *App) chat() *ChatAppService {
	a.ensureServices()
	return a.chatApp
}

func (a *App) interactiveService() *InteractiveAppService {
	a.ensureServices()
	return a.interactiveApp
}

func (a *App) lore() *LoreAppService {
	a.ensureServices()
	return a.loreApp
}

func (a *App) images() *ImageAppService {
	a.ensureServices()
	return a.imageApp
}

func (a *App) configManager() *ConfigManagerAppService {
	a.ensureServices()
	return a.configApp
}

func (a *App) automation() *AutomationAppService {
	a.ensureServices()
	return a.automationApp
}

func (a *App) skills() *SkillsAppService {
	a.ensureServices()
	return a.skillsApp
}

func (a *App) applyRuntime(runtime *runtimeState) {
	a.workspace = runtime.workspace
	a.bookState = runtime.bookState
	a.bookService = runtime.bookService
	a.interactive = runtime.interactive
	a.sessionStore = runtime.sessionStore
	a.session = runtime.session
	a.agentRunner = runtime.agentRunner
	a.interactiveStoryRunner = runtime.interactiveStoryRunner
	a.versionService = runtime.versionService
}

func (a *App) clearRuntime() {
	a.workspace = ""
	a.cfg.Workspace = ""
	a.bookState = nil
	a.bookService = nil
	a.interactive = nil
	a.sessionStore = nil
	a.session = nil
	a.agentRunner = nil
	a.interactiveStoryRunner = nil
	a.versionService = nil
}

// RemoteAccessConfig returns the current process-level access policy used by
// the HTTP gateway. Settings updates may change this before a full restart.
func (a *App) RemoteAccessConfig() config.RemoteAccessConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return config.RemoteAccessConfig{}
	}
	return a.cfg.RemoteAccessConfig()
}

// HideChapterBodyLiveOutput reports whether real-time SSE output should
// hide novel chapter body content while preserving tool execution internally.
func (a *App) HideChapterBodyLiveOutput() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg != nil && a.cfg.HideChapterBodyLiveOutput
}

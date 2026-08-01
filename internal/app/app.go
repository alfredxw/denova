package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/concurrency"
	"denova/internal/interactive"
	projectdomain "denova/internal/project"
	"denova/internal/terminal"
	"denova/internal/workspace/filewatch"
)

// App 是 API 层使用的应用门面；具体业务由领域应用服务承接。
type App struct {
	cfg *config.Config

	workspace                       string
	bookState                       *book.State
	bookService                     *book.Service
	interactive                     *interactive.Store
	sessionStore                    *session.Store
	session                         *session.Session
	agentRunner                     *agents.Runner
	interactiveStoryRunner          *agents.Runner
	chatService                     *agentharness.Service
	bookRegistry                    *BookRegistry
	projectRegistry                 *projectdomain.Registry
	bookMetaStore                   *BookMetaStore
	versionService                  *book.VersionService
	activeTask                      *apptask.Task
	activeWritingRun                *writingTaskRun
	activeInteractiveRun            *interactiveTaskRun
	activeLoreImageTask             *apptask.Task
	activeAutomationTasks           map[string]*apptask.Task
	activeAutomationRuns            map[string]automationRunState
	activeAutomationClaims          map[string]*automationRunClaim
	automationTriggers              *automationTriggerCoordinator
	workspaceDirectorTasks          *workspaceDirectorTaskGroup
	workspaceTasks                  map[*apptask.Task]string
	workspaceTaskLeases             map[*apptask.Task]*concurrency.Lease
	workspaceTaskStops              map[*apptask.Task]func() bool
	workspaceTaskReplayReservations map[*apptask.Task]*apptask.ReplayReservation
	workspaceTransition             bool
	workspaceTransitionTargets      map[string]struct{}
	directorGenerator               interactiveDirectorGenerator
	versionSummaryGenerator         versionSummaryGeneratorFunc
	workspaceFiles                  *filewatch.Service
	rootScope                       *concurrency.Scope
	workspaceScopes                 map[string]*concurrency.Scope
	workspaceScopeSequence          uint64
	workspaceGeneration             uint64
	closed                          bool
	closeOnce                       sync.Once
	schedulerCancel                 context.CancelFunc
	schedulerWG                     sync.WaitGroup
	schedulerStarted                bool
	automationEffectWake            chan struct{}
	activeTaskReplay                apptask.ReplayAdmission

	// terminals owns the pty sessions behind the AgentChat terminal tabs. They are decoupled from
	// the workspace: each session keeps its own cwd, so switching books never kills a running command.
	terminals *terminal.Manager

	runtimeManager *WorkspaceRuntimeManager
	chatApp        *ChatAppService
	agentChatApp   *AgentChatAppService
	interactiveApp *InteractiveAppService
	loreApp        *LoreAppService
	configApp      *ConfigManagerAppService
	automationApp  *AutomationAppService
	skillsApp      *SkillsAppService
	imageApp       *ImageAppService
	servicesOnce   sync.Once

	mu sync.RWMutex
}

// SetInteractiveDirectorGeneratorForTest installs an App-scoped Director
// generator so tests do not share mutable package-level state.
func (a *App) SetInteractiveDirectorGeneratorForTest(generator interactiveDirectorGenerator) func() {
	if a == nil {
		return func() {}
	}
	a.mu.Lock()
	previous := a.directorGenerator
	a.directorGenerator = generator
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		a.directorGenerator = previous
		a.mu.Unlock()
	}
}

func (a *App) interactiveDirectorGenerator() interactiveDirectorGenerator {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.directorGenerator
}

// New 创建应用运行时。当 workspace 为空且没有上次打开的 workspace 时，App 进入“无书籍”状态，
// 等待用户在前端书籍管理页选择或新建书籍后再构建 runtime。
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	dataDir := ""
	if cfg != nil {
		dataDir = strings.TrimSpace(cfg.DataDir())
	}
	if dataDir == "" {
		return nil, ErrAgentDataDirRequired
	}
	registry := NewBookRegistry(dataDir)
	bookMetaStore := NewBookMetaStore(dataDir)
	app := &App{
		cfg:                  cfg,
		bookRegistry:         registry,
		projectRegistry:      registry.ProjectRegistry(),
		bookMetaStore:        bookMetaStore,
		workspaceFiles:       filewatch.NewService(),
		terminals:            terminal.NewManager(terminalConfigFromAppConfig(cfg)),
		automationEffectWake: make(chan struct{}, 1),
	}
	chatService, err := agentharness.NewDurableService(
		ctx,
		dataDir,
		agentharness.WithDomainCommitReconciler(app.reconcileHarnessDomainCommit),
		agentharness.WithInputMaterializer(app),
		agentharness.WithTurnRestorer(app.restoreHarnessTurn),
		agentharness.WithStructuralRestorer(app.restoreContextStructuralOperation),
		agentharness.WithHostEffectReconciler(app.reconcileHarnessHostEffect),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize durable agent runtime: %w", err)
	}
	app.chatService = chatService
	workspace := cfg.Workspace
	if workspace == "" && cfg.ResumeLastWorkspace {
		if lastWorkspace := registry.Current(); lastWorkspace != "" {
			workspace = lastWorkspace
		}
	}

	app.mu.Lock()
	if err := app.initializeLifecycleLocked(); err != nil {
		app.mu.Unlock()
		_ = chatService.Close(context.Background())
		return nil, fmt.Errorf("initialize app lifecycle: %w", err)
	}
	app.mu.Unlock()
	app.ensureServices()

	if workspace == "" {
		slog.InfoContext(ctx, "[app] 启动时未指定 workspace 且无上次打开的书籍，进入无书籍状态，等待用户在前端选择")
		cfg.Workspace = ""
		app.StartAutomationScheduler(ctx)
		return app, nil
	}

	projectRecord, err := app.projectRegistry.EnsureBook(workspace)
	if err != nil {
		app.Close()
		return nil, err
	}
	layout, err := app.projectRegistry.EnsureState(projectRecord)
	if err != nil {
		app.Close()
		return nil, err
	}
	runtime, err := buildRuntimeExclusively(ctx, cfg, layout)
	if err != nil {
		app.Close()
		return nil, err
	}
	cfg.Workspace = runtime.workspace
	_ = registry.Touch(runtime.workspace)

	app.mu.Lock()
	if err := app.replaceWorkspaceScopeLocked(runtime.workspace); err != nil {
		app.mu.Unlock()
		app.Close()
		return nil, err
	}
	app.applyRuntime(runtime)
	app.mu.Unlock()
	app.syncWorkspaceFileWatcher(runtime.workspace)
	app.StartAutomationScheduler(ctx)
	return app, nil
}

// ErrNoWorkspace 表示当前 App 尚未绑定任何书籍 workspace。
var ErrNoWorkspace = fmt.Errorf("尚未选择书籍工作区")

// ErrAgentDataDirRequired prevents production App instances from silently
// falling back to a process-local journal that cannot survive restart.
var ErrAgentDataDirRequired = errors.New("agent runtime data directory is required")

// ErrNoWorkspaceOpen 表示请求需要一个已打开的工作区但当前没有。
var ErrNoWorkspaceOpen = errors.New("当前没有打开的工作区")

// ErrAgentOperationActive rejects implicit replacement. Callers must target
// the running operation with Follow Up, Steer, or Abort before starting a new
// root operation.
var ErrAgentOperationActive = errors.New("agent operation is already active")

// ErrWorkspaceTransition prevents a task from binding half to an old
// workspace and half to a newly constructed runtime.
var ErrWorkspaceTransition = errors.New("workspace runtime is transitioning")

// ErrAgentContextChanged means preparation completed against a workspace,
// session, story, or branch that is no longer current at atomic registration.
var ErrAgentContextChanged = errors.New("agent start context changed")

func (a *App) ensureServices() {
	a.servicesOnce.Do(func() {
		if a.projectRegistry == nil && a.bookRegistry != nil {
			a.projectRegistry = a.bookRegistry.ProjectRegistry()
		}
		a.automationTriggers = newAutomationTriggerCoordinator()
		a.runtimeManager = &WorkspaceRuntimeManager{app: a}
		a.chatApp = &ChatAppService{app: a}
		a.agentChatApp = newAgentChatAppService(a)
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

func (a *App) agentChat() *AgentChatAppService {
	a.ensureServices()
	return a.agentChatApp
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
	if a.cfg != nil {
		a.cfg.ProjectID = runtime.projectID
		a.cfg.ProjectStateDir = runtime.projectStateRoot
	}
	a.activeTask = nil
	a.activeWritingRun = nil
	a.activeInteractiveRun = nil
	a.activeLoreImageTask = nil
	a.workspaceDirectorTasks = newWorkspaceDirectorTaskGroup()
}

func (a *App) clearRuntime() {
	a.workspace = ""
	a.cfg.Workspace = ""
	a.cfg.ProjectID = ""
	a.cfg.ProjectStateDir = ""
	a.bookState = nil
	a.bookService = nil
	a.interactive = nil
	a.sessionStore = nil
	a.session = nil
	a.agentRunner = nil
	a.interactiveStoryRunner = nil
	a.versionService = nil
	a.activeTask = nil
	a.activeWritingRun = nil
	a.activeInteractiveRun = nil
	a.activeLoreImageTask = nil
}

func (a *App) stopWorkspaceDirectorTasks() {
	a.mu.Lock()
	tasks := a.workspaceDirectorTasks
	a.workspaceDirectorTasks = nil
	a.mu.Unlock()
	tasks.Close()
}

// restoreWorkspaceDirectorTasks reinstalls a fresh owner after a failed
// runtime rebuild left the old workspace active. The stopped group is never
// reused: conversations admitted after the transition bind to the new owner.
func (a *App) restoreWorkspaceDirectorTasks(workspace string) {
	if a == nil || strings.TrimSpace(workspace) == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.workspace == workspace && a.workspaceDirectorTasks == nil {
		a.workspaceDirectorTasks = newWorkspaceDirectorTaskGroup()
	}
}

func (a *App) directorTasksForWorkspace(workspace string) *workspaceDirectorTaskGroup {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspace != workspace {
		return nil
	}
	return a.workspaceDirectorTasks
}

// Close stops background work owned by the current workspace runtime.
func (a *App) Close() {
	if a == nil {
		return
	}
	a.closeOnce.Do(func() {
		a.ensureServices()
		a.mu.Lock()
		a.closed = true
		rootScope := a.rootScope
		schedulerCancel := a.schedulerCancel
		versionService := a.versionService
		workspaceFiles := a.workspaceFiles
		interactiveStore := a.interactive
		sessionStore := a.sessionStore
		a.mu.Unlock()

		if workspaceFiles != nil {
			workspaceFiles.Close()
		}
		if a.terminals != nil {
			a.terminals.CloseAll()
		}
		// Admission closes before cancellation so no task can slip between the
		// final registry snapshot and the resource barrier.
		rootScope.BeginClose()
		if schedulerCancel != nil {
			schedulerCancel()
		}
		if a.automationTriggers != nil {
			a.automationTriggers.Close()
		}
		if a.agentChatApp != nil {
			a.agentChatApp.Close(context.Background())
		}
		a.abortOwnedAgentTasks(context.Background())
		a.stopWorkspaceDirectorTasks()
		a.schedulerWG.Wait()
		if err := rootScope.Wait(context.Background()); err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[app] wait lifecycle scope failed: %v", err))
		}
		if interactiveStore != nil {
			if err := interactiveStore.Close(); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] flush interactive conversation indexes failed: %v", err))
			}
		}
		if sessionStore != nil {
			if err := sessionStore.Close(); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] flush Agent conversation indexes failed: %v", err))
			}
		}
		if versionService != nil {
			versionService.Close()
		}
		if a.chatService != nil {
			if err := a.chatService.Close(context.Background()); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] close durable agent runtime failed: %v", err))
			}
		}
	})
}

func (a *App) abortOwnedAgentTasks(ctx context.Context) {
	if a == nil {
		return
	}
	a.mu.RLock()
	unique := make(map[*apptask.Task]struct{}, 3+len(a.activeAutomationTasks)+len(a.workspaceTasks))
	add := func(task *apptask.Task) {
		if task != nil {
			unique[task] = struct{}{}
		}
	}
	if a.activeTask != nil {
		add(a.activeTask)
	}
	if a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil {
		add(a.activeInteractiveRun.task)
	}
	if a.activeLoreImageTask != nil {
		add(a.activeLoreImageTask)
	}
	for _, task := range a.activeAutomationTasks {
		add(task)
	}
	for task := range a.workspaceTasks {
		add(task)
	}
	a.mu.RUnlock()
	tasks := make([]*apptask.Task, 0, len(unique))
	for task := range unique {
		tasks = append(tasks, task)
	}
	if err := abortAndWaitTasks(ctx, tasks, "app_close"); err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[app] wait for owned agent tasks failed: %v", err))
	}
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

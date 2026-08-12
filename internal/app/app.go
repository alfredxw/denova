package app

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agents/session"
	activityapp "denova/internal/app/activity"
	agentchatapp "denova/internal/app/agentchat"
	appagentruntime "denova/internal/app/agentruntime"
	automationapp "denova/internal/app/automation"
	bookapp "denova/internal/app/book"
	configmanagerapp "denova/internal/app/configmanager"
	continuallearningapp "denova/internal/app/continuallearning"
	imageapp "denova/internal/app/image"
	interactiveapp "denova/internal/app/interactive"
	loreapp "denova/internal/app/lore"
	modelsapp "denova/internal/app/models"
	projectbookapp "denova/internal/app/projectbook"
	projectfilesapp "denova/internal/app/projectfiles"
	resourcecatalogapp "denova/internal/app/resourcecatalog"
	settingsapp "denova/internal/app/settings"
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
	executionRuntime                *agentexecution.Runtime
	projectRegistry                 *projectdomain.Registry
	bookMetaStore                   *book.MetaStore
	versionService                  *book.VersionService
	activeTask                      *apptask.Task
	activeWritingRun                *writingTaskRun
	activeInteractiveRun            *interactiveTaskRun
	workspaceDirectorTasks          *interactiveapp.DirectorTaskGroup
	workspaceTasks                  map[*apptask.Task]string
	workspaceTaskLeases             map[*apptask.Task]*concurrency.Lease
	workspaceTaskStops              map[*apptask.Task]func() bool
	workspaceTaskReplayReservations map[*apptask.Task]*apptask.ReplayReservation
	workspaceTransition             bool
	workspaceTransitionTargets      map[string]struct{}
	directorGenerator               interactiveapp.DirectorGenerator
	versionSummaryGenerator         versionSummaryGeneratorFunc
	workspaceFiles                  *filewatch.Service
	rootScope                       *concurrency.Scope
	workspaceScopes                 map[string]*concurrency.Scope
	workspaceScopeSequence          uint64
	projectScopes                   map[string]*concurrency.Scope
	projectScopeSequence            uint64
	projectTransitions              map[string]struct{}
	projectTasks                    map[*apptask.Task]*projectTaskRegistration
	workspaceGeneration             uint64
	closed                          bool
	closeOnce                       sync.Once
	activeTaskReplay                apptask.ReplayAdmission

	// terminals owns the pty sessions behind the AgentChat terminal tabs. They are decoupled from
	// the workspace: each session keeps its own cwd, so switching books never kills a running command.
	terminals *terminal.Manager

	workspaceApp      *workspaceService
	chatApp           *ChatAppService
	agentChatApp      *agentchatapp.Service
	interactiveApp    *InteractiveAppService
	loreApp           *loreapp.Service
	configApp         *configmanagerapp.Service
	continualLearning *continuallearningapp.Service
	automationApp     *automationapp.Service
	activityApp       *activityapp.Service
	bookApp           *bookapp.Service
	resourceCatalog   *resourcecatalogapp.Service
	settingsApp       *settingsapp.Service
	modelsApp         *modelsapp.Service
	imageApp          *imageapp.Service
	projectBook       *projectbookapp.Service
	projectFiles      *projectfilesapp.Service
	servicesOnce      sync.Once

	mu sync.RWMutex
}

// New creates the application runtime. When neither an explicit nor resumable
// workspace exists, App stays unbound until the user selects or creates a Book.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	dataDir := ""
	if cfg != nil {
		dataDir = strings.TrimSpace(cfg.DataDir())
	}
	if dataDir == "" {
		return nil, ErrAgentDataDirRequired
	}
	registry := projectdomain.NewRegistry(dataDir)
	bookMetaStore := book.NewMetaStore(dataDir)
	app := &App{
		cfg:             cfg,
		projectRegistry: registry,
		bookMetaStore:   bookMetaStore,
		workspaceFiles:  filewatch.NewService(),
		terminals:       terminal.NewManager(terminalConfigFromAppConfig(cfg)),
	}
	app.automationApp = automationapp.NewService(automationHost{app: app})
	executionRuntime, err := agentexecution.NewAgentRuntime(
		ctx,
		dataDir,
		agentexecution.WithProfiles(app.executionProfiles()...),
		agentexecution.WithChildDefinitionResolver(agentexecution.ChildDefinitionResolverFunc(app.prepareChildDefinition)),
		agentexecution.WithHostEffectReconciler(app.automationApp.ReconcileHostEffect),
		agentexecution.WithPermissionRuleStore(agentexecution.PermissionRuleStore{
			Load: func(context.Context) ([]config.AgentApprovalRule, error) {
				layered, err := app.SettingsService().Snapshot(settingsapp.Global())
				if err != nil {
					return nil, err
				}
				return config.NormalizeAgentApprovalRules(layered.Effective.AgentApprovalRules), nil
			},
			Persist: func(ctx context.Context, rule config.AgentApprovalRule) error {
				_, err := app.SettingsService().EnsureAgentApprovalRule(rule)
				return err
			},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize Agent runtime: %w", err)
	}
	app.executionRuntime = executionRuntime
	workspace := cfg.Workspace
	if workspace == "" && cfg.ResumeLastWorkspace {
		if lastWorkspace := registry.CurrentBookPath(); lastWorkspace != "" {
			workspace = lastWorkspace
		}
	}

	app.mu.Lock()
	if err := app.initializeLifecycleLocked(); err != nil {
		app.mu.Unlock()
		_ = executionRuntime.Close(context.Background())
		return nil, fmt.Errorf("initialize app lifecycle: %w", err)
	}
	app.mu.Unlock()
	app.ensureServices()

	if workspace == "" {
		slog.InfoContext(ctx, "[app] 启动时未指定 workspace 且无上次打开的书籍，进入无书籍状态，等待用户在前端选择")
		cfg.Workspace = ""
		app.Automation().StartScheduler(ctx)
		app.ContinualLearning().StartScheduler(ctx)
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
	_, _ = registry.TouchBook(runtime.workspace)

	app.mu.Lock()
	if err := app.replaceWorkspaceScopeLocked(runtime.workspace); err != nil {
		app.mu.Unlock()
		app.Close()
		return nil, err
	}
	app.applyRuntime(runtime)
	app.mu.Unlock()
	app.Automation().StartScheduler(ctx)
	app.ContinualLearning().StartScheduler(ctx)
	return app, nil
}

// ErrNoWorkspace 表示当前 App 尚未绑定任何书籍 workspace。
var ErrNoWorkspace = appagentruntime.ErrNoWorkspace

// ErrAgentDataDirRequired prevents production App instances from silently
// falling back to a process-local journal that cannot survive restart.
var ErrAgentDataDirRequired = errors.New("agent runtime data directory is required")

// ErrNoWorkspaceOpen means a request requires an open workspace.
var ErrNoWorkspaceOpen = settingsapp.ErrProjectRequired

// ErrAgentOperationActive rejects implicit replacement. Callers must target
// the running operation with Follow Up, Steer, or Abort before starting a new
// root operation.
var ErrAgentOperationActive = appagentruntime.ErrOperationActive

// ErrWorkspaceTransition prevents a task from binding half to an old
// workspace and half to a newly constructed runtime.
var ErrWorkspaceTransition = appagentruntime.ErrWorkspaceTransition

// ErrAgentContextChanged means preparation completed against a workspace,
// session, story, or branch that is no longer current at atomic registration.
var ErrAgentContextChanged = appagentruntime.ErrContextChanged

func (a *App) ensureServices() {
	a.servicesOnce.Do(func() {
		a.workspaceApp = &workspaceService{app: a}
		a.chatApp = &ChatAppService{
			app: a, starts: apptask.NewStartRegistry(apptask.StartRegistryOptions{Label: "Writing"}),
		}
		a.agentChatApp = agentchatapp.NewService(agentChatHost{app: a}, a.projectRegistry)
		a.interactiveApp = &InteractiveAppService{app: a}
		a.configApp = configmanagerapp.NewService(configManagerHost{app: a})
		a.continualLearning = continuallearningapp.NewService(continualLearningHost{app: a})
		if a.automationApp == nil {
			a.automationApp = automationapp.NewService(automationHost{app: a})
		}
		dataDir := ""
		if a.cfg != nil {
			dataDir = a.cfg.DataDir()
		}
		a.activityApp = activityapp.NewService(dataDir, a.automationApp)
		a.bookApp = bookapp.NewService(dataDir, a.projectRegistry, a.bookMetaStore)
		a.resourceCatalog = resourcecatalogapp.NewService(dataDir, resourceCatalogHost{app: a})
		a.settingsApp = settingsapp.NewService(settingsHost{app: a})
		a.modelsApp = modelsapp.NewService(modelHost{app: a})
		a.imageApp = imageapp.NewService(imageHost{app: a})
		a.loreApp = loreapp.NewService(loreHost{app: a}, a.imageApp)
		a.projectBook = projectbookapp.NewService(a.projectRegistry)
		if a.workspaceFiles != nil {
			a.workspaceFiles.SetObserver(a.observeProjectFileChange)
		}
		projectFileOptions := []projectfilesapp.ServiceOption(nil)
		if a.cfg != nil {
			projectFileOptions = append(projectFileOptions, projectfilesapp.WithTreeEntryLimit(a.cfg.ProjectFileTreeEntryLimit))
		}
		a.projectFiles = projectfilesapp.NewServiceWithBookVersioning(a.projectRegistry, a, projectFileOptions...)
	})
}

func (a *App) workspaceService() *workspaceService {
	a.ensureServices()
	return a.workspaceApp
}

func (a *App) chat() *ChatAppService {
	a.ensureServices()
	return a.chatApp
}

// AgentChat exposes the cohesive project-scoped conversation service.
func (a *App) AgentChat() *agentchatapp.Service {
	a.ensureServices()
	return a.agentChatApp
}

// ProjectBook exposes Book resources through stable Project identity without
// changing the foreground Writing workspace.
func (a *App) ProjectBook() *projectbookapp.Service {
	a.ensureServices()
	return a.projectBook
}

// ProjectFiles exposes Project-scoped file browsing and editing without
// changing the foreground Writing workspace.
func (a *App) ProjectFiles() *projectfilesapp.Service {
	a.ensureServices()
	return a.projectFiles
}

func (a *App) interactiveService() *InteractiveAppService {
	a.ensureServices()
	return a.interactiveApp
}

// Lore exposes the cohesive lore application service.
func (a *App) Lore() *loreapp.Service {
	a.ensureServices()
	return a.loreApp
}

// Images exposes shared image generation for writing and game modes.
func (a *App) Images() *imageapp.Service {
	a.ensureServices()
	return a.imageApp
}

// ConfigManager exposes scoped configuration conversations directly.
func (a *App) ConfigManager() *configmanagerapp.Service {
	a.ensureServices()
	return a.configApp
}

// ContinualLearning exposes user-level Harness State and optimization.
func (a *App) ContinualLearning() *continuallearningapp.Service {
	a.ensureServices()
	return a.continualLearning
}

// Automation exposes the automation domain service without duplicating its API
// on the root composition type.
func (a *App) Automation() *automationapp.Service {
	a.ensureServices()
	return a.automationApp
}

// Activity exposes the unified notification and badge projection.
func (a *App) Activity() *activityapp.Service {
	a.ensureServices()
	return a.activityApp
}

// BookAssets exposes book cover and export operations.
func (a *App) BookAssets() *bookapp.Service {
	a.ensureServices()
	return a.bookApp
}

// ResourceCatalog exposes reusable creator resources shared by writing and
// game modes without duplicating their API on the root composition type.
func (a *App) ResourceCatalog() *resourcecatalogapp.Service {
	a.ensureServices()
	return a.resourceCatalog
}

// SettingsService exposes layered settings persistence while App retains only
// the process-local refresh effects.
func (a *App) SettingsService() *settingsapp.Service {
	a.ensureServices()
	return a.settingsApp
}

// Models exposes provider discovery and connection validation shared by all
// writing and game model configuration surfaces.
func (a *App) Models() *modelsapp.Service {
	a.ensureServices()
	return a.modelsApp
}

func (a *App) applyRuntime(runtime *runtimeState) {
	a.workspace = runtime.workspace
	a.bookState = runtime.bookState
	a.bookService = runtime.bookService
	a.interactive = runtime.interactive
	a.sessionStore = runtime.sessionStore
	a.session = runtime.session
	a.versionService = runtime.versionService
	if a.cfg != nil {
		a.cfg.ProjectID = runtime.projectID
		a.cfg.ProjectStateDir = runtime.projectStateRoot
	}
	a.activeTask = nil
	a.activeWritingRun = nil
	a.activeInteractiveRun = nil
	a.workspaceDirectorTasks = interactiveapp.NewDirectorTaskGroup()
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
	a.versionService = nil
	a.activeTask = nil
	a.activeWritingRun = nil
	a.activeInteractiveRun = nil
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
		a.workspaceDirectorTasks = interactiveapp.NewDirectorTaskGroup()
	}
}

func (a *App) directorTasksForWorkspace(workspace string) *interactiveapp.DirectorTaskGroup {
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
		if rootScope != nil {
			rootScope.BeginClose()
		}
		if a.automationApp != nil {
			if err := a.automationApp.Close(context.Background()); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] close automation service failed: %v", err))
			}
		}
		if a.continualLearning != nil {
			if err := a.continualLearning.Close(context.Background()); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] close continual learning service failed: %v", err))
			}
		}
		if a.agentChatApp != nil {
			a.agentChatApp.Close(context.Background())
		}
		if a.projectFiles != nil {
			a.projectFiles.Close()
		}
		a.abortOwnedAgentTasks(context.Background())
		a.stopWorkspaceDirectorTasks()
		if rootScope != nil {
			if err := rootScope.Wait(context.Background()); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app] wait lifecycle scope failed: %v", err))
			}
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
		if a.executionRuntime != nil {
			if err := a.executionRuntime.Close(context.Background()); err != nil {
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
	unique := make(map[*apptask.Task]struct{}, 3+len(a.workspaceTasks)+len(a.projectTasks))
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
	for task := range a.workspaceTasks {
		add(task)
	}
	for task := range a.projectTasks {
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

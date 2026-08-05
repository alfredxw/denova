package automationapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
	"denova/internal/book"
	"denova/internal/concurrency"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// Legacy test names remain local to this package while the tests are migrated
// from the former root package. Production exposes only Service and Host.
type AutomationAppService = Service
type ProjectLayout = projectdomain.Layout

var (
	ErrAgentCommandIDRequired = ErrCommandIDRequired
	ErrAgentCommandConflict   = ErrCommandConflict
	ErrAgentReplayCapacity    = ErrReplayCapacity
)

type BookRegistry struct {
	projects *projectdomain.Registry
}

func NewBookRegistry(dataDir string) *BookRegistry {
	return &BookRegistry{projects: projectdomain.NewRegistry(dataDir)}
}

func (registry *BookRegistry) ProjectRegistry() *projectdomain.Registry {
	if registry == nil {
		return nil
	}
	return registry.projects
}

func (registry *BookRegistry) Touch(workspace string) error {
	if registry == nil || registry.projects == nil {
		return errors.New("project registry is unavailable")
	}
	_, err := registry.projects.TouchBook(workspace)
	return err
}

type cachedTestRuntime struct {
	runtime Runtime
	close   sync.Once
}

func (runtime *cachedTestRuntime) Close() {
	if runtime == nil {
		return
	}
	runtime.close.Do(func() {
		if runtime.runtime.SessionStore != nil {
			_ = runtime.runtime.SessionStore.Close()
		}
	})
}

// App is a test-only Host implementation. It deliberately models the same
// lifecycle fences as the root composition without importing that package.
type App struct {
	mu sync.RWMutex

	cfg             *config.Config
	workspace       string
	bookState       *book.State
	bookService     *book.Service
	sessionStore    *session.Store
	chatService     *agentharness.Service
	bookRegistry    *BookRegistry
	projectRegistry *projectdomain.Registry

	automationApp         *Service
	runtimes              map[string]*cachedTestRuntime
	automationTriggers    *automationTriggerCoordinator
	automationEffectWake  chan struct{}
	activeAutomationTasks map[string]*apptask.Task

	rootScope                   *concurrency.Scope
	workspaceScopes             map[string]*concurrency.Scope
	workspaceTasks              map[*apptask.Task]string
	workspaceTaskLeases         map[*apptask.Task]*concurrency.Lease
	workspaceTaskStops          map[*apptask.Task]func() bool
	workspaceReplayReservations map[*apptask.Task]*apptask.ReplayReservation
	activeTaskReplay            apptask.ReplayAdmission
	workspaceTransition         bool
	transitionTargets           map[string]struct{}
	closed                      bool
	closeOnce                   sync.Once
}

func New(ctx context.Context, cfg *config.Config) (*App, error) {
	if cfg == nil || strings.TrimSpace(cfg.DataDir()) == "" {
		return nil, errors.New("agent runtime data directory is required")
	}
	dataDir := cfg.DataDir()
	registry := NewBookRegistry(dataDir)
	application := &App{cfg: cfg, bookRegistry: registry, projectRegistry: registry.ProjectRegistry()}
	workspace := strings.TrimSpace(cfg.Workspace)
	if workspace != "" {
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return nil, err
		}
		record, err := application.projectRegistry.EnsureBook(workspace)
		if err != nil {
			return nil, err
		}
		layout, err := application.projectRegistry.EnsureState(record)
		if err != nil {
			return nil, err
		}
		state := book.NewState(record.WorkspacePath)
		if err := state.InitWorkspace(); err != nil {
			return nil, err
		}
		store, err := session.NewStore(layout.SessionsDir())
		if err != nil {
			return nil, err
		}
		cfg.Workspace = record.WorkspacePath
		cfg.ProjectID = record.ID
		cfg.ProjectStateDir = layout.StateRoot
		application.workspace = record.WorkspacePath
		application.bookState = state
		application.bookService = book.NewService(record.WorkspacePath)
		application.sessionStore = store
	}
	application.ensureServices()
	chatService, err := agentharness.NewDurableService(
		ctx,
		dataDir,
		agentharness.WithHostEffectReconciler(application.automationApp.ReconcileHostEffect),
	)
	if err != nil {
		application.Close()
		return nil, err
	}
	application.mu.Lock()
	application.chatService = chatService
	application.mu.Unlock()
	application.automationApp.StartScheduler(ctx)
	return application, nil
}

func (application *App) ensureServices() {
	if application == nil {
		return
	}
	application.mu.Lock()
	defer application.mu.Unlock()
	if application.cfg == nil {
		application.cfg = &config.Config{}
	}
	if application.projectRegistry == nil && application.bookRegistry != nil {
		application.projectRegistry = application.bookRegistry.ProjectRegistry()
	}
	if application.runtimes == nil {
		application.runtimes = make(map[string]*cachedTestRuntime)
	}
	_ = application.initializeLifecycleLocked()
	if application.automationApp == nil {
		application.automationApp = NewServiceForTestHost(application)
		if application.automationEffectWake != nil {
			application.automationApp.effectWake = application.automationEffectWake
		} else {
			application.automationEffectWake = application.automationApp.effectWake
		}
		application.automationTriggers = application.automationApp.triggers
		application.activeAutomationTasks = application.automationApp.activeTasks
	}
}

func NewServiceForTestHost(host Host) *Service {
	return NewService(host)
}

func (application *App) automation() *Service {
	application.ensureServices()
	application.mu.RLock()
	defer application.mu.RUnlock()
	return application.automationApp
}

func (application *App) Close() {
	if application == nil {
		return
	}
	application.closeOnce.Do(func() {
		application.ensureServices()
		application.mu.Lock()
		application.closed = true
		rootScope := application.rootScope
		service := application.automationApp
		chatService := application.chatService
		store := application.sessionStore
		runtimes := make([]*cachedTestRuntime, 0, len(application.runtimes))
		for _, runtime := range application.runtimes {
			runtimes = append(runtimes, runtime)
		}
		application.mu.Unlock()
		if rootScope != nil {
			rootScope.BeginClose()
		}
		if service != nil {
			if err := service.Close(context.Background()); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[automation-test] close service failed: %v", err))
			}
		}
		if rootScope != nil {
			_ = rootScope.Wait(context.Background())
		}
		for _, runtime := range runtimes {
			runtime.Close()
		}
		if store != nil {
			_ = store.Close()
		}
		if chatService != nil {
			_ = chatService.Close(context.Background())
		}
	})
}

func (application *App) initializeLifecycleLocked() error {
	if application.closed {
		return concurrency.ErrClosed
	}
	if application.rootScope == nil {
		application.rootScope = concurrency.NewRoot("automation-test")
	}
	if application.workspaceScopes == nil {
		application.workspaceScopes = make(map[string]*concurrency.Scope)
	}
	if application.workspaceTasks == nil {
		application.workspaceTasks = make(map[*apptask.Task]string)
		application.workspaceTaskLeases = make(map[*apptask.Task]*concurrency.Lease)
		application.workspaceTaskStops = make(map[*apptask.Task]func() bool)
		application.workspaceReplayReservations = make(map[*apptask.Task]*apptask.ReplayReservation)
	}
	return nil
}

func (application *App) scopeLocked(workspace string) (*concurrency.Scope, error) {
	if err := application.initializeLifecycleLocked(); err != nil {
		return nil, err
	}
	key := canonicalAutomationWorkspace(workspace)
	if key == "" {
		return application.rootScope, nil
	}
	if scope := application.workspaceScopes[key]; scope != nil {
		return scope, nil
	}
	scope, err := application.rootScope.Child("workspace:" + key)
	if err != nil {
		return nil, err
	}
	application.workspaceScopes[key] = scope
	return scope, nil
}

type testOperation struct {
	ctx   context.Context
	lease *concurrency.Lease
}

func (operation *testOperation) Context() context.Context { return operation.ctx }
func (operation *testOperation) Release() {
	if operation != nil && operation.lease != nil {
		operation.lease.Release()
	}
}

func (application *App) acquireOperation(ctx context.Context, workspace string) (Operation, error) {
	application.mu.Lock()
	defer application.mu.Unlock()
	if application.closed {
		return nil, concurrency.ErrClosed
	}
	key := canonicalAutomationWorkspace(workspace)
	if application.workspaceTransition {
		if _, fenced := application.transitionTargets[key]; fenced {
			return nil, concurrency.ErrClosing
		}
	}
	scope, err := application.scopeLocked(workspace)
	if err != nil {
		return nil, err
	}
	opCtx, lease, err := scope.AcquireContext(ctx)
	if err != nil {
		return nil, err
	}
	return &testOperation{ctx: opCtx, lease: lease}, nil
}

func (application *App) registerTask(task *apptask.Task, workspace string) error {
	application.mu.Lock()
	defer application.mu.Unlock()
	if task == nil {
		return errors.New("cannot register a nil Task")
	}
	if application.closed {
		return concurrency.ErrClosed
	}
	scope, err := application.scopeLocked(workspace)
	if err != nil {
		return err
	}
	lease, err := scope.Acquire()
	if err != nil {
		return err
	}
	replay, err := application.activeTaskReplay.Reserve(task)
	if err != nil {
		lease.Release()
		return err
	}
	application.workspaceTasks[task] = canonicalAutomationWorkspace(workspace)
	application.workspaceTaskLeases[task] = lease
	application.workspaceTaskStops[task] = context.AfterFunc(scope.Context(), task.Abort)
	application.workspaceReplayReservations[task] = replay
	return nil
}

func (application *App) unregisterWorkspaceTask(task *apptask.Task) {
	if application == nil || task == nil {
		return
	}
	application.mu.Lock()
	if _, ok := application.workspaceTasks[task]; !ok {
		application.mu.Unlock()
		return
	}
	lease := application.workspaceTaskLeases[task]
	stop := application.workspaceTaskStops[task]
	replay := application.workspaceReplayReservations[task]
	delete(application.workspaceTasks, task)
	delete(application.workspaceTaskLeases, task)
	delete(application.workspaceTaskStops, task)
	delete(application.workspaceReplayReservations, task)
	application.mu.Unlock()
	if stop != nil {
		stop()
	}
	if lease != nil {
		lease.Release()
	}
	if replay != nil {
		replay.Release()
	}
	application.automation().SignalReconciliation()
}

func (application *App) CurrentWorkspace() string {
	application.mu.RLock()
	defer application.mu.RUnlock()
	return application.workspace
}

func (application *App) CurrentRuntime() (Runtime, error) {
	application.mu.RLock()
	if strings.TrimSpace(application.workspace) == "" {
		application.mu.RUnlock()
		return Runtime{}, ErrNoWorkspace
	}
	var cfg config.Config
	if application.cfg != nil {
		cfg = *application.cfg
	}
	runtime := Runtime{
		ProjectID: cfg.ProjectID, ProjectType: projectdomain.TypeBook,
		StateRoot: cfg.ProjectStateDir, Workspace: application.workspace,
		DataDir: cfg.DataDir(), Config: cfg, BookState: application.bookState,
		BookService: application.bookService, SessionStore: application.sessionStore,
		ChatService: application.chatService,
	}
	application.mu.RUnlock()
	if layered, err := config.LoadLayeredWithStartupConfigAt(
		runtime.DataDir, runtime.Workspace, config.ProjectConfigPath(runtime.StateRoot),
	); err == nil {
		if layered.Effective.MaxIteration != nil && *layered.Effective.MaxIteration > 0 {
			runtime.Config.MaxIteration = *layered.Effective.MaxIteration
		}
	}
	return runtime, nil
}

func (application *App) BaseRuntime() Runtime {
	application.mu.RLock()
	defer application.mu.RUnlock()
	var cfg config.Config
	if application.cfg != nil {
		cfg = *application.cfg
	}
	return Runtime{DataDir: cfg.DataDir(), Config: cfg, ChatService: application.chatService}
}

func (application *App) ResolveTarget(target automation.ExecutionTarget) (automation.ExecutionTarget, error) {
	if target.Kind == automation.TargetKindUser {
		return automation.ExecutionTarget{Kind: automation.TargetKindUser}, nil
	}
	if application.projectRegistry == nil {
		workspace := canonicalAutomationWorkspace(target.Workspace)
		if workspace == "" {
			workspace = canonicalAutomationWorkspace(application.CurrentWorkspace())
		}
		if workspace == "" {
			return automation.ExecutionTarget{}, ErrNoWorkspace
		}
		return automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: target.ProjectID, Workspace: workspace}, nil
	}
	if id := strings.TrimSpace(target.ProjectID); id != "" {
		if record, err := application.projectRegistry.Get(id); err == nil {
			return automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: record.ID, Workspace: record.WorkspacePath}, nil
		}
	}
	record, found, err := application.projectRegistry.FindByPath(target.Workspace, false)
	if err != nil {
		return automation.ExecutionTarget{}, err
	}
	if !found {
		return automation.ExecutionTarget{}, errors.New("directory is not a registered project")
	}
	return automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: record.ID, Workspace: record.WorkspacePath}, nil
}

func (application *App) RuntimeForTarget(ctx context.Context, target automation.ExecutionTarget) (Runtime, error) {
	resolved, err := application.ResolveTarget(target)
	if err != nil {
		return Runtime{}, err
	}
	if current, currentErr := application.CurrentRuntime(); currentErr == nil &&
		(current.ProjectID == resolved.ProjectID || canonicalAutomationWorkspace(current.Workspace) == canonicalAutomationWorkspace(resolved.Workspace)) {
		return current, nil
	}
	key := strings.TrimSpace(resolved.ProjectID)
	application.mu.RLock()
	cached := application.runtimes[key]
	application.mu.RUnlock()
	if cached != nil {
		return cached.runtime, nil
	}
	if application.projectRegistry == nil {
		return Runtime{}, ErrNoWorkspace
	}
	record, err := application.projectRegistry.Get(key)
	if err != nil {
		return Runtime{}, err
	}
	layout, err := application.projectRegistry.EnsureState(record)
	if err != nil {
		return Runtime{}, err
	}
	var cfg config.Config
	if application.cfg != nil {
		cfg = *application.cfg
	}
	cfg.Workspace = record.WorkspacePath
	cfg.ProjectID = record.ID
	cfg.ProjectStateDir = layout.StateRoot
	var state *book.State
	if record.Type == projectdomain.TypeBook {
		state = book.NewState(record.WorkspacePath)
		if err := state.InitWorkspace(); err != nil {
			return Runtime{}, err
		}
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		return Runtime{}, err
	}
	runtime := Runtime{
		ProjectID: record.ID, ProjectType: record.Type, StateRoot: layout.StateRoot,
		Workspace: record.WorkspacePath, DataDir: cfg.DataDir(), Config: cfg,
		BookState: state, BookService: book.NewService(record.WorkspacePath),
		SessionStore: store, ChatService: application.BaseRuntime().ChatService,
	}
	entry := &cachedTestRuntime{runtime: runtime}
	application.mu.Lock()
	if existing := application.runtimes[key]; existing != nil {
		application.mu.Unlock()
		entry.Close()
		return existing.runtime, nil
	}
	application.runtimes[key] = entry
	application.mu.Unlock()
	_ = ctx
	return runtime, nil
}

func (application *App) Catalog() (Catalog, error) {
	base := application.BaseRuntime()
	catalog := Catalog{DataDir: base.DataDir, CurrentWorkspace: application.CurrentWorkspace()}
	if application.projectRegistry == nil {
		return catalog, nil
	}
	records, err := application.projectRegistry.List(false)
	if err != nil {
		return catalog, err
	}
	for _, record := range records {
		layout, err := application.projectRegistry.Layout(record)
		if err != nil {
			continue
		}
		catalog.Projects = append(catalog.Projects, automation.ProjectLocation{
			ProjectID: record.ID, Workspace: record.WorkspacePath, StateRoot: layout.StateRoot,
		})
	}
	return catalog, nil
}

func (application *App) AcquireRootOperation(ctx context.Context) (Operation, error) {
	return application.acquireOperation(ctx, "")
}
func (application *App) AcquireProjectOperation(ctx context.Context, projectID string) (Operation, error) {
	resolved, err := application.ResolveTarget(automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	return application.acquireOperation(ctx, resolved.Workspace)
}
func (application *App) AcquireWorkspaceOperation(ctx context.Context, workspace string) (Operation, error) {
	return application.acquireOperation(ctx, workspace)
}
func (application *App) RegisterTask(task *apptask.Task, workspace string) error {
	return application.registerTask(task, workspace)
}
func (application *App) UnregisterTask(task *apptask.Task) {
	application.unregisterWorkspaceTask(task)
}

func (application *App) Workspace() string { return application.CurrentWorkspace() }
func (application *App) Projects(includeArchived bool) ([]projectdomain.Record, error) {
	if application.projectRegistry == nil {
		return nil, nil
	}
	return application.projectRegistry.List(includeArchived)
}

func (application *App) Automations() ([]automation.Task, error) {
	return application.automation().List()
}
func (application *App) AutomationInbox() ([]automation.TriggerInboxItem, error) {
	return application.automation().Inbox()
}
func (application *App) CreateAutomation(task automation.Task) (automation.Task, error) {
	return application.automation().Create(task)
}
func (application *App) UpdateAutomation(id string, task automation.Task) (automation.Task, error) {
	return application.automation().Update(id, task)
}
func (application *App) RunDueAutomations(ctx context.Context, now time.Time) []automation.RunResult {
	return application.automation().RunDue(ctx, now)
}
func (application *App) CheckAutomationTriggers(ctx context.Context, id string) ([]automation.TriggerInboxItem, error) {
	return application.automation().CheckTriggers(ctx, id)
}
func (application *App) CheckAutomationTriggersAfterWorkspaceMutation(ctx context.Context, source string, paths []string) {
	application.automation().CheckTriggersAfterWorkspaceMutation(ctx, source, paths)
}
func (application *App) ActiveAutomationRuns() []automation.ActiveRun {
	return application.automation().ActiveAutomationRuns()
}
func (application *App) ActiveAutomationTaskByRunID(runID string) (*apptask.Task, automation.RunRecord, bool) {
	return application.automation().ActiveAutomationTaskByRunID(runID)
}
func (application *App) StartAutomationTaskCommand(ctx context.Context, id, commandID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
	return application.automation().StartTaskCommand(ctx, id, commandID, evidence)
}
func (application *App) ContinueAutomationRun(ctx context.Context, runID, commandID, message string) (*apptask.Task, automation.RunRecord, error) {
	return application.automation().ContinueRun(ctx, runID, commandID, message)
}
func (application *App) AbortAutomationRunCommand(ctx context.Context, runID, commandID string, operationID agentrun.OperationID, reason string) (agentrun.CommandReceipt, error) {
	return application.automation().AbortRunCommand(ctx, runID, commandID, operationID, reason)
}
func (application *App) reconcileHarnessHostEffect(ctx context.Context, committed agenttoolruntime.CommittedToolMutation) error {
	return application.automation().ReconcileHostEffect(ctx, committed)
}

func (application *App) automationSnapshot() *automationWorkspaceSnapshot {
	runtime, err := application.CurrentRuntime()
	if err != nil {
		return nil
	}
	return snapshotFromRuntime(runtime)
}

func (application *App) automationMutationCallback(_ string) func(context.Context, []agenttool.Mutation, agenttool.Verification) {
	return func(context.Context, []agenttool.Mutation, agenttool.Verification) {
		application.automation().SignalReconciliation()
	}
}

func (application *App) beginWorkspaceTransitionTo(workspaces ...string) ([]*apptask.Task, []*concurrency.Scope, string, error) {
	application.mu.Lock()
	defer application.mu.Unlock()
	if application.workspaceTransition {
		return nil, nil, application.workspace, concurrency.ErrClosing
	}
	application.workspaceTransition = true
	application.transitionTargets = make(map[string]struct{}, len(workspaces)+1)
	all := append([]string{application.workspace}, workspaces...)
	var scopes []*concurrency.Scope
	seen := make(map[*concurrency.Scope]struct{})
	for _, workspace := range all {
		key := canonicalAutomationWorkspace(workspace)
		if key == "" {
			continue
		}
		application.transitionTargets[key] = struct{}{}
		scope, err := application.scopeLocked(workspace)
		if err != nil {
			return nil, nil, application.workspace, err
		}
		if _, ok := seen[scope]; !ok {
			scope.BeginClose()
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	return nil, scopes, application.workspace, nil
}

func (application *App) endWorkspaceTransition() {
	application.mu.Lock()
	application.workspaceTransition = false
	application.transitionTargets = nil
	application.mu.Unlock()
}

func waitLifecycleScopes(ctx context.Context, scopes []*concurrency.Scope) error {
	for _, scope := range scopes {
		if err := scope.Wait(ctx); err != nil {
			return err
		}
	}
	return nil
}

type WorkspaceChangeMutationHooks struct {
	ScheduleAutoVersion bool
	AutomationSource    string
	Paths               []string
}

func (application *App) WithWorkspaceChangeMutation(
	ctx context.Context,
	expectedWorkspace string,
	action func(*workspacechange.Service) (WorkspaceChangeMutationHooks, error),
) (string, error) {
	runtime, err := application.CurrentRuntime()
	if err != nil {
		return "", err
	}
	if canonicalAutomationWorkspace(expectedWorkspace) != canonicalAutomationWorkspace(runtime.Workspace) {
		return "", errors.New("workspace changed")
	}
	operation, err := application.AcquireWorkspaceOperation(ctx, runtime.Workspace)
	if err != nil {
		return "", err
	}
	defer operation.Release()
	changes, err := workspacechange.ForWorkspaceAt(runtime.Workspace, runtime.StateRoot)
	if err != nil {
		return "", err
	}
	hooks, err := action(changes)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(hooks.AutomationSource) != "" && len(hooks.Paths) > 0 {
		application.automation().checkTriggersAfterWorkspaceMutation(
			operation.Context(), snapshotFromRuntime(runtime), hooks.AutomationSource, hooks.Paths,
		)
	}
	return runtime.Workspace, nil
}

func appRuntimeBindingForTest(binding agentrun.RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}

func automationRuntimeBindingForTest(workspace, sessionID, taskID string, projectIDs ...string) runstate.BindingRef {
	projectID := ""
	if len(projectIDs) > 0 {
		projectID = projectIDs[0]
	}
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindAutomation, ProjectID: projectID,
		Workspace: workspace, SessionID: sessionID, TaskID: taskID,
	})
}

func settledTaskWithReplay(t *testing.T, content string) *apptask.Task {
	t.Helper()
	task := apptask.New(func(_ context.Context, _ *apptask.Task, emit func(agentrun.Event)) {
		emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": strings.Repeat(content, 32)}})
		emit(agentrun.Event{Type: "done", Data: map[string]any{}})
	})
	<-task.Done()
	return task
}

var _ Host = (*App)(nil)

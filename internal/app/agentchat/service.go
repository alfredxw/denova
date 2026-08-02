package agentchat

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	conversationapp "denova/internal/app/conversation"
	appsettings "denova/internal/app/settings"
	apptask "denova/internal/app/task"
	"denova/internal/book"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

type projectRuntime struct {
	projectID      string
	projectType    projectdomain.Type
	agentKind      string
	stateRoot      string
	workspace      string
	state          *book.State
	store          *session.Store
	bookService    *book.Service
	versionService *book.VersionService
	chatService    *agentharness.Service
	cfg            config.Config
	closeOnce      sync.Once
}

func (runtime *projectRuntime) conversation(sess *session.Session) conversationapp.Runtime {
	if runtime == nil {
		return conversationapp.Runtime{}
	}
	return conversationapp.Runtime{
		ProjectID: runtime.projectID, ProjectType: runtime.projectType, ProjectState: runtime.stateRoot,
		AgentKind: runtime.agentKind, Session: sess, State: runtime.state,
		BookService: runtime.bookService, ChatService: runtime.chatService, Workspace: runtime.workspace,
		VersionService: runtime.versionService, Config: runtime.cfg,
	}
}

func (runtime *projectRuntime) close() {
	if runtime == nil {
		return
	}
	runtime.closeOnce.Do(func() {
		if runtime.store != nil {
			if err := runtime.store.Close(); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] close project session store failed workspace=%q err=%v", runtime.workspace, err))
			}
		}
		if runtime.versionService != nil {
			runtime.versionService.Close()
		}
	})
}

type run struct {
	binding         Binding
	commandID       string
	task            *apptask.Task
	runtime         conversationapp.Runtime
	recovery        *agentharness.RecoveryObservation
	recoveryActions map[string]agentrun.CommandReceipt
}

// Service owns project runtime caching, scoped admission, reconnectable tasks,
// and durable Agent commands for every AgentChat conversation.
type Service struct {
	host     Host
	registry *projectdomain.Registry

	admission    sync.Mutex
	projectBuild sync.Mutex
	starts       apptask.StartRegistry

	mu       sync.RWMutex
	closed   bool
	projects map[string]*projectRuntime
	active   map[string]*run
}

func NewService(host Host, registry *projectdomain.Registry) *Service {
	return &Service{
		host: host, registry: registry,
		starts:   apptask.NewStartRegistry(apptask.StartRegistryOptions{Label: "Agent Chat"}),
		projects: make(map[string]*projectRuntime), active: make(map[string]*run),
	}
}

func bindingKey(binding Binding) string {
	owner := strings.TrimSpace(binding.ProjectID)
	if owner == "" {
		owner = strings.TrimSpace(binding.Workspace)
	}
	return owner + "\x00" + strings.TrimSpace(binding.SessionID)
}

func runtimeOptions(binding Binding, taskID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: binding.agentKind, ProjectID: binding.ProjectID, StateRoot: binding.stateRoot,
		TaskID: strings.TrimSpace(taskID), SessionID: binding.SessionID,
		Workspace: binding.Workspace, Mode: RuntimeMode,
	}
}

// ResolveBinding upgrades compatibility paths to stable Project identity and
// validates the complete conversation scope.
func (service *Service) ResolveBinding(binding Binding) (Binding, error) {
	if service == nil || service.registry == nil {
		return Binding{}, appagentruntime.ErrNoWorkspace
	}
	projectID := strings.TrimSpace(binding.ProjectID)
	var (
		record projectdomain.Record
		layout projectdomain.Layout
		err    error
	)
	if projectID == "" && strings.TrimSpace(binding.Workspace) != "" {
		record, layout, err = service.registry.ResolveByPath(binding.Workspace, true)
	} else {
		record, layout, err = service.registry.Resolve(projectID, true)
		if err != nil && strings.TrimSpace(binding.Workspace) == "" && projectID != "" {
			// Temporary compatibility for callers that still pass a directory in
			// the project field. The returned binding is immediately upgraded.
			record, layout, err = service.registry.ResolveByPath(projectID, true)
		}
	}
	if err != nil {
		return Binding{}, err
	}
	binding.ProjectID = record.ID
	binding.Workspace = layout.ContentRoot
	binding.stateRoot = layout.StateRoot
	switch record.Type {
	case projectdomain.TypeBook:
		binding.agentKind = agentrun.AgentKindIDE
	case projectdomain.TypeGeneral:
		binding.agentKind = agentrun.AgentKindGeneral
	default:
		return Binding{}, fmt.Errorf("unsupported project type %q", record.Type)
	}
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	if binding.SessionID == "" {
		return Binding{}, fmt.Errorf("AgentChat session is required / AgentChat 会话不能为空")
	}
	return binding, nil
}

// ResolveWorkspace validates a temporary path-based client binding without
// changing the foreground Book.
func (service *Service) ResolveWorkspace(workspace string) (string, error) {
	if service == nil || service.registry == nil {
		return "", appagentruntime.ErrNoWorkspace
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", fmt.Errorf("AgentChat project workspace is required / AgentChat 项目目录不能为空")
	}
	_, layout, err := service.registry.ResolveByPath(workspace, true)
	if err != nil {
		return "", fmt.Errorf("AgentChat project is not registered / AgentChat 项目尚未注册: %s: %w", workspace, err)
	}
	return layout.ContentRoot, nil
}

// ProjectRuntime returns a stable dependency snapshot while the Service keeps
// ownership of its cached stores and lifecycle.
func (service *Service) ProjectRuntime(ctx context.Context, projectID string) (ProjectRuntime, error) {
	runtime, err := service.projectRuntime(ctx, projectID)
	if err != nil {
		return ProjectRuntime{}, err
	}
	return ProjectRuntime{Conversation: runtime.conversation(nil), SessionStore: runtime.store}, nil
}

func (service *Service) projectRuntime(ctx context.Context, projectID string) (*projectRuntime, error) {
	service.projectBuild.Lock()
	defer service.projectBuild.Unlock()
	service.mu.RLock()
	project := service.projects[projectID]
	closed := service.closed
	service.mu.RUnlock()
	if closed {
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if project != nil {
		return project, nil
	}
	if service.host == nil || service.registry == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	runtimeCfg, chatService := service.host.BaseRuntime()
	if chatService == nil || strings.TrimSpace(runtimeCfg.DataDir()) == "" {
		return nil, appagentruntime.ErrNoWorkspace
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return nil, err
	}
	workspace := layout.ContentRoot
	changes, err := workspacechange.ForWorkspaceAt(workspace, layout.StateRoot)
	if err != nil {
		return nil, err
	}
	var state *book.State
	var versionService *book.VersionService
	agentKind := agentrun.AgentKindGeneral
	if record.Type == projectdomain.TypeBook {
		state = book.NewState(workspace)
		if err := changes.WithExclusiveWorkspace(ctx, state.InitWorkspace); err != nil {
			return nil, err
		}
		versionService = book.NewVersionService(workspace)
		agentKind = agentrun.AgentKindIDE
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		if versionService != nil {
			versionService.Close()
		}
		return nil, err
	}
	runtimeCfg.Workspace = workspace
	runtimeCfg.ProjectID = record.ID
	runtimeCfg.ProjectStateDir = layout.StateRoot
	runtimeCfg, err = appsettings.RefreshProject(runtimeCfg, workspace, layout.StateRoot)
	if err != nil {
		_ = store.Close()
		if versionService != nil {
			versionService.Close()
		}
		return nil, err
	}
	runtimeCfg.ProjectID = record.ID
	project = &projectRuntime{
		projectID: record.ID, projectType: record.Type, agentKind: agentKind, stateRoot: layout.StateRoot,
		workspace: workspace, state: state, store: store, bookService: book.NewService(workspace),
		versionService: versionService, chatService: chatService, cfg: runtimeCfg,
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		project.close()
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if existing := service.projects[projectID]; existing != nil {
		service.mu.Unlock()
		project.close()
		return existing, nil
	}
	service.projects[projectID] = project
	service.mu.Unlock()
	return project, nil
}

func (service *Service) activeRun(binding Binding) *run {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.active[bindingKey(binding)]
}

func (service *Service) installActiveRun(active *run) error {
	if active == nil || active.task == nil {
		return fmt.Errorf("cannot install an empty AgentChat run")
	}
	key := bindingKey(active.binding)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return fmt.Errorf("AgentChat service is closed")
	}
	if current := service.active[key]; current != nil && current.task != nil && !current.task.Finished() {
		return appagentruntime.ErrOperationActive
	}
	service.active[key] = active
	return nil
}

func (service *Service) releaseActiveRun(active *run) {
	if service == nil || active == nil {
		return
	}
	key := bindingKey(active.binding)
	service.mu.Lock()
	if service.active[key] == active {
		delete(service.active, key)
	}
	service.mu.Unlock()
}

// CloseProject drains one cached Project before archive or relink. A running
// conversation is never aborted implicitly by project management.
func (service *Service) CloseProject(ctx context.Context, projectID string) error {
	if service == nil {
		return nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	service.mu.Lock()
	for _, active := range service.active {
		if active != nil && active.binding.ProjectID == projectID && active.task != nil && !active.task.Finished() {
			service.mu.Unlock()
			return fmt.Errorf("%w: project has a running Agent conversation", appagentruntime.ErrOperationActive)
		}
	}
	project := service.projects[projectID]
	delete(service.projects, projectID)
	service.mu.Unlock()
	if project != nil {
		defer project.close()
	}
	if service.host != nil {
		_, chatService := service.host.BaseRuntime()
		if chatService != nil {
			return chatService.CloseProjectBindings(ctx, projectID)
		}
	}
	return nil
}

// Close aborts all user-level conversations before the shared durable runtime
// closes. Foreground Book switches deliberately do not call it.
func (service *Service) Close(ctx context.Context) {
	if service == nil {
		return
	}
	service.admission.Lock()
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		service.admission.Unlock()
		return
	}
	service.closed = true
	runs := make([]*run, 0, len(service.active))
	for _, active := range service.active {
		runs = append(runs, active)
	}
	projects := make([]*projectRuntime, 0, len(service.projects))
	for _, project := range service.projects {
		projects = append(projects, project)
	}
	service.mu.Unlock()
	service.admission.Unlock()

	for _, active := range runs {
		if active != nil && active.task != nil {
			active.task.Abort()
		}
	}
	for _, active := range runs {
		if active == nil || active.task == nil {
			continue
		}
		select {
		case <-active.task.Done():
		case <-ctx.Done():
			return
		}
	}
	for _, project := range projects {
		project.close()
	}
}

func canonicalWorkspaceKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if canonical, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = canonical
	}
	return filepath.Clean(abs)
}

func refreshRuntimeConfig(project *projectRuntime) (config.Config, error) {
	if project == nil {
		return config.Config{}, appagentruntime.ErrNoWorkspace
	}
	return appsettings.RefreshProject(project.cfg, project.workspace, project.stateRoot)
}

func getOrCreateConversation(project *projectRuntime, binding Binding) (*session.Session, error) {
	runtimeCfg, err := refreshRuntimeConfig(project)
	if err != nil {
		return nil, err
	}
	sess, _, err := agentconversation.GetOrCreateSession(project.store, binding.SessionID, &runtimeCfg, binding.agentKind)
	return sess, err
}

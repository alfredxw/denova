package agentchat

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"denova/config"
	agentattachment "denova/internal/agents/attachment"
	chatagent "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
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

// MaterializeAttachments persists one project-scoped Session upload before
// command admission, while Project runtime resolution remains server-owned.
func (service *Service) MaterializeAttachments(ctx context.Context, binding Binding, commandID string, request *chatagent.ChatRequest) error {
	if request == nil || len(request.AttachmentUploads) == 0 {
		return nil
	}
	resolved, err := service.ResolveBinding(binding)
	if err != nil {
		return err
	}
	project, err := service.projectRuntime(ctx, resolved.ProjectID)
	if err != nil {
		return err
	}
	files, err := agentattachment.Materialize(project.stateRoot, agentattachment.SessionScope(resolved.SessionID), commandID, request.AttachmentUploads)
	if err != nil {
		return fmt.Errorf("materialize AgentChat attachments: %w", err)
	}
	request.AttachmentUploads = nil
	request.AttachedFiles = files
	request.AttachmentIDs = make([]string, 0, len(files))
	for _, file := range files {
		request.AttachmentIDs = append(request.AttachmentIDs, file.ID)
	}
	return nil
}

type projectRuntime struct {
	projectID        string
	projectType      projectdomain.Type
	agentKind        string
	stateRoot        string
	workspace        string
	state            *book.State
	store            *session.Store
	bookService      *book.Service
	versionService   *book.VersionService
	executionRuntime *agentexecution.Runtime
	cfg              config.Config
	used             uint64
	closeOnce        sync.Once
}

func (runtime *projectRuntime) conversation(sess *session.Session) conversationapp.Runtime {
	if runtime == nil {
		return conversationapp.Runtime{}
	}
	return conversationapp.Runtime{
		ProjectID: runtime.projectID, ProjectType: runtime.projectType, ProjectStore: runtime.stateRoot,
		AgentKind: runtime.agentKind, Session: sess, State: runtime.state,
		BookService: runtime.bookService, ExecutionRuntime: runtime.executionRuntime, Workspace: runtime.workspace,
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
	})
}

type run struct {
	binding         Binding
	commandID       string
	task            *apptask.Task
	runtime         conversationapp.Runtime
	request         ChatRequest
	policy          TurnPolicy
	recovery        *agentexecution.RecoveryObservation
	recoveryActions map[string]agentrun.CommandReceipt
}

type cachedProjectSessionStore struct {
	dir    string
	store  *session.Store
	used   uint64
	pinned bool
}

const (
	maxCachedProjectRuntimes      = 8
	maxCachedProjectSessionStores = 16
)

// Service owns project runtime caching, scoped admission, reconnectable tasks,
// and durable Agent commands for every AgentChat conversation.
type Service struct {
	host     Host
	registry *projectdomain.Registry

	admission     sync.Mutex
	projectBuild  sync.Mutex
	storeMu       sync.Mutex
	storeSequence uint64
	starts        apptask.StartRegistry

	mu              sync.RWMutex
	closed          bool
	projects        map[string]*projectRuntime
	projectSequence uint64
	active          map[string]*run
	stores          map[string]cachedProjectSessionStore
}

func NewService(host Host, registry *projectdomain.Registry) *Service {
	return &Service{
		host: host, registry: registry,
		// Independent AgentChat conversations may run concurrently; process-wide
		// replay admission remains the authoritative upper bound.
		starts: apptask.NewStartRegistry(apptask.StartRegistryOptions{
			Label:           "Agent Chat",
			ReplayByteLimit: apptask.DefaultActiveReplayByteLimit,
		}),
		projects: make(map[string]*projectRuntime), active: make(map[string]*run),
		stores: make(map[string]cachedProjectSessionStore),
	}
}

func (service *Service) projectSessionStore(projectID, dir string, pin bool) (*session.Store, error) {
	projectID = strings.TrimSpace(projectID)
	dir = filepath.Clean(dir)
	service.storeMu.Lock()
	if cached := service.stores[projectID]; cached.store != nil && cached.dir == dir {
		service.storeSequence++
		cached.used = service.storeSequence
		cached.pinned = cached.pinned || pin
		service.stores[projectID] = cached
		service.storeMu.Unlock()
		return cached.store, nil
	}
	store, err := session.NewStore(dir)
	if err != nil {
		service.storeMu.Unlock()
		return nil, err
	}
	service.storeSequence++
	service.stores[projectID] = cachedProjectSessionStore{dir: dir, store: store, used: service.storeSequence, pinned: pin}
	for len(service.stores) > maxCachedProjectSessionStores {
		victim := ""
		var oldest uint64
		for candidate, cached := range service.stores {
			if candidate == projectID || cached.pinned {
				continue
			}
			if victim == "" || cached.used < oldest {
				victim, oldest = candidate, cached.used
			}
		}
		if victim == "" {
			break
		}
		delete(service.stores, victim)
	}
	service.storeMu.Unlock()
	return store, nil
}

func (service *Service) evictProjectSessionStore(projectID string, store *session.Store) {
	service.storeMu.Lock()
	if cached := service.stores[strings.TrimSpace(projectID)]; cached.store == store {
		delete(service.stores, strings.TrimSpace(projectID))
	}
	service.storeMu.Unlock()
}

func (service *Service) discardProjectSessionStore(projectID string) *session.Store {
	service.storeMu.Lock()
	defer service.storeMu.Unlock()
	cached := service.stores[strings.TrimSpace(projectID)]
	delete(service.stores, strings.TrimSpace(projectID))
	return cached.store
}

func bindingKey(binding Binding) string {
	return strings.TrimSpace(binding.ProjectID) + "\x00" + strings.TrimSpace(binding.SessionID)
}

func runtimeOptions(binding Binding, taskID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: binding.agentKind, ProjectID: binding.ProjectID, StateRoot: binding.stateRoot,
		TaskID: strings.TrimSpace(taskID), SessionID: binding.SessionID,
		Workspace: binding.Workspace, Mode: RuntimeMode,
	}
}

// ResolveBinding derives runtime paths from stable Project identity and
// validates the complete conversation scope.
func (service *Service) ResolveBinding(binding Binding) (Binding, error) {
	if service == nil || service.registry == nil {
		return Binding{}, appagentruntime.ErrNoWorkspace
	}
	projectID := strings.TrimSpace(binding.ProjectID)
	if projectID == "" {
		return Binding{}, fmt.Errorf("AgentChat Project ID is required / AgentChat 项目标识不能为空")
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return Binding{}, err
	}
	binding.ProjectID = record.ID
	binding.Workspace = layout.ContentRoot
	binding.stateRoot = layout.StoreRoot
	switch record.Type {
	case projectdomain.TypeBook:
		binding.agentKind = agentrun.AgentKindIDE
	case projectdomain.TypeGeneral, projectdomain.TypeAgents:
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
	service.mu.Lock()
	project := service.projects[projectID]
	closed := service.closed
	if project != nil {
		service.projectSequence++
		project.used = service.projectSequence
	}
	service.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if project != nil {
		return project, nil
	}
	if service.host == nil || service.registry == nil {
		return nil, appagentruntime.ErrNoWorkspace
	}
	runtimeCfg, executionRuntime := service.host.BaseRuntime()
	if executionRuntime == nil || strings.TrimSpace(runtimeCfg.DataDir()) == "" {
		return nil, appagentruntime.ErrNoWorkspace
	}
	record, layout, err := service.registry.Resolve(projectID, true)
	if err != nil {
		return nil, err
	}
	workspace := layout.ContentRoot
	changes, err := workspacechange.ForWorkspaceAt(workspace, layout.StoreRoot)
	if err != nil {
		return nil, err
	}
	var state *book.State
	var versionService *book.VersionService
	agentKind := agentrun.AgentKindGeneral
	switch record.Type {
	case projectdomain.TypeBook:
		agentKind = agentrun.AgentKindIDE
		state = book.NewState(workspace)
		if err := changes.WithExclusiveWorkspace(ctx, state.InitWorkspace); err != nil {
			return nil, err
		}
		versionService, err = service.host.ProjectVersionService(record.ID)
		if err != nil {
			return nil, fmt.Errorf("resolve shared Project version service: %w", err)
		}
	case projectdomain.TypeGeneral:
		agentKind = agentrun.AgentKindGeneral
	case projectdomain.TypeAgents:
		agentKind = agentrun.AgentKindGeneral
		versionService, err = service.host.ProjectVersionService(record.ID)
		if err != nil {
			return nil, fmt.Errorf("resolve shared Agents Project version service: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported project type %q", record.Type)
	}
	store, err := service.projectSessionStore(record.ID, layout.SessionsDir(), true)
	if err != nil {
		return nil, err
	}
	runtimeCfg.Workspace = workspace
	runtimeCfg.ProjectID = record.ID
	runtimeCfg.ProjectStoreDir = layout.StoreRoot
	runtimeCfg, err = appsettings.RefreshProject(runtimeCfg, workspace, layout.StoreRoot)
	if err != nil {
		service.evictProjectSessionStore(record.ID, store)
		_ = store.Close()
		return nil, err
	}
	runtimeCfg.ProjectID = record.ID
	project = &projectRuntime{
		projectID: record.ID, projectType: record.Type, agentKind: agentKind, stateRoot: layout.StoreRoot,
		workspace: workspace, state: state, store: store, bookService: book.NewService(workspace),
		versionService: versionService, executionRuntime: executionRuntime, cfg: runtimeCfg,
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		service.evictProjectSessionStore(record.ID, store)
		project.close()
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if existing := service.projects[projectID]; existing != nil {
		service.projectSequence++
		existing.used = service.projectSequence
		service.mu.Unlock()
		project.close()
		return existing, nil
	}
	service.projectSequence++
	project.used = service.projectSequence
	service.projects[projectID] = project
	evictedID, evicted := service.evictProjectRuntimeLocked(projectID)
	service.mu.Unlock()
	if evicted != nil {
		// Capacity eviction drops only cache ownership. In-flight callers keep a
		// valid immutable dependency snapshot and canonical journals remain the
		// synchronization boundary if this Project is opened again.
		service.evictProjectSessionStore(evictedID, evicted.store)
	}
	return project, nil
}

func (service *Service) evictProjectRuntimeLocked(protected string) (string, *projectRuntime) {
	if len(service.projects) <= maxCachedProjectRuntimes {
		return "", nil
	}
	victim := ""
	var oldest uint64
	for projectID, project := range service.projects {
		if projectID == protected || project == nil {
			continue
		}
		running := false
		for _, active := range service.active {
			if active != nil && active.binding.ProjectID == projectID && active.task != nil && !active.task.Finished() {
				running = true
				break
			}
		}
		if running {
			continue
		}
		if victim == "" || project.used < oldest {
			victim, oldest = projectID, project.used
		}
	}
	if victim == "" {
		return "", nil
	}
	evicted := service.projects[victim]
	delete(service.projects, victim)
	return victim, evicted
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
	evictedID, evicted := service.evictProjectRuntimeLocked("")
	service.mu.Unlock()
	if evicted != nil {
		service.evictProjectSessionStore(evictedID, evicted.store)
	}
}

// InvalidateBookSummary discards rebuildable Book projections for one cached
// Project without opening a cold Project runtime.
func (service *Service) InvalidateBookSummary(projectID string, paths []string, resync bool) {
	if service == nil {
		return
	}
	service.mu.RLock()
	project := service.projects[strings.TrimSpace(projectID)]
	service.mu.RUnlock()
	if project == nil || project.bookService == nil {
		return
	}
	project.bookService.InvalidateSummary(paths, resync)
	if project.state != nil {
		project.state.InvalidateChapterPaths(paths, resync)
	}
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
	store := service.discardProjectSessionStore(projectID)
	if project != nil {
		defer project.close()
	} else if store != nil {
		defer func() {
			if err := store.Close(); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] close project session catalog failed project_id=%s err=%v", projectID, err))
			}
		}()
	}
	if service.host != nil {
		_, executionRuntime := service.host.BaseRuntime()
		if executionRuntime != nil {
			return executionRuntime.DeleteProjectBindings(ctx, projectID)
		}
	}
	return nil
}

// Close aborts all user-level conversations before the shared Agent runtime
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
	service.storeMu.Lock()
	stores := make([]*session.Store, 0, len(service.stores))
	for _, cached := range service.stores {
		stores = append(stores, cached.store)
	}
	service.stores = make(map[string]cachedProjectSessionStore)
	service.storeMu.Unlock()

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
	for _, store := range stores {
		if store != nil {
			if err := store.Close(); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[app/agentchat] close cached project session store failed err=%v", err))
			}
		}
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

func getOrCreateConversation(project *projectRuntime, binding Binding) (*session.Session, bool, error) {
	runtimeCfg, err := refreshRuntimeConfig(project)
	if err != nil {
		return nil, false, err
	}
	created := !project.store.Exists(binding.SessionID)
	sess, _, err := agentconversation.GetOrCreateSession(project.store, binding.SessionID, &runtimeCfg, binding.agentKind)
	return sess, created, err
}

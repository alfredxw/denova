package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
	"denova/internal/workspacechange"
)

const agentChatRuntimeMode = "agent_chat"

// AgentChatBinding is the explicit project conversation identity carried by
// every user-level AgentChat request. It never reads or mutates App.workspace.
type AgentChatBinding struct {
	ProjectID string `json:"project_id"`
	SessionID string `json:"session_id"`
	// Workspace is accepted only as a temporary compatibility input for older
	// local clients. Resolved bindings always carry ProjectID as their identity.
	Workspace string `json:"workspace,omitempty"`
	agentKind string
	stateRoot string
}

type agentChatProjectRuntime struct {
	projectID      string
	projectType    ProjectType
	agentKind      string
	stateRoot      string
	workspace      string
	state          *book.State
	store          *session.Store
	bookService    *book.Service
	versionService *book.VersionService
	chatService    *agents.ChatService
	cfg            config.Config
	closeOnce      sync.Once
}

func (runtime *agentChatProjectRuntime) close() {
	if runtime == nil {
		return
	}
	runtime.closeOnce.Do(func() {
		if runtime.store != nil {
			if err := runtime.store.Close(); err != nil {
				log.Printf("[app/agent_chat_app_service.go] closing project session store failed workspace=%q err=%v", runtime.workspace, err)
			}
		}
		if runtime.versionService != nil {
			runtime.versionService.Close()
		}
	})
}

type agentChatRun struct {
	binding         AgentChatBinding
	commandID       string
	task            *Task
	runtime         ideChatRuntime
	recovery        *agents.RecoveryObservation
	recoveryActions map[string]agents.CommandReceipt
}

// AgentChatAppService owns user-level project conversations. Runs are keyed by
// project plus session, so different conversations may execute concurrently
// while a second root turn for the same conversation is rejected.
type AgentChatAppService struct {
	app          *App
	admission    sync.Mutex
	projectBuild sync.Mutex
	starts       writingStartRegistry

	mu       sync.RWMutex
	closed   bool
	projects map[string]*agentChatProjectRuntime
	active   map[string]*agentChatRun
}

func newAgentChatAppService(app *App) *AgentChatAppService {
	return &AgentChatAppService{
		app: app, projects: make(map[string]*agentChatProjectRuntime), active: make(map[string]*agentChatRun),
	}
}

func agentChatBindingKey(binding AgentChatBinding) string {
	owner := strings.TrimSpace(binding.ProjectID)
	if owner == "" {
		owner = strings.TrimSpace(binding.Workspace)
	}
	return owner + "\x00" + strings.TrimSpace(binding.SessionID)
}

func agentChatRunOptions(binding AgentChatBinding, taskID string) agents.RunOptions {
	return agents.RunOptions{
		AgentKind: binding.agentKind, ProjectID: binding.ProjectID, StateRoot: binding.stateRoot,
		TaskID: strings.TrimSpace(taskID), SessionID: binding.SessionID,
		Workspace: binding.Workspace, Mode: agentChatRuntimeMode,
	}
}

// StartAgentChatTask starts one project-scoped turn without switching the
// foreground Writing book or session.
func (a *App) StartAgentChatTask(ctx context.Context, binding AgentChatBinding, req agents.ChatRequest) (*Task, error) {
	return a.agentChat().StartTask(ctx, binding, req)
}

func (s *AgentChatAppService) StartTask(ctx context.Context, binding AgentChatBinding, req agents.ChatRequest) (*Task, error) {
	s.admission.Lock()
	defer s.admission.Unlock()

	var err error
	binding, err = s.resolveBinding(binding)
	if err != nil {
		return nil, err
	}
	req.CommandID = strings.TrimSpace(req.CommandID)
	if req.CommandID == "" {
		return nil, ErrAgentCommandIDRequired
	}
	if err := agents.ValidateCommandID(req.CommandID); err != nil {
		return nil, err
	}
	req = agents.CaptureChatRequestCallerInput(req)
	fingerprint := agents.ChatRequestSemanticFingerprint(req)
	if replay, ok, err := s.starts.replay(req.CommandID, binding.ProjectID, binding.SessionID, fingerprint); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	if run := s.activeRun(binding); run != nil && run.task != nil && !run.task.Finished() {
		return nil, ErrAgentOperationActive
	}

	project, err := s.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return nil, err
	}
	seedCfg, err := refreshConversationRuntimeConfig(project.cfg, project.workspace, project.stateRoot)
	if err != nil {
		return nil, err
	}
	// The first accepted turn and the selector endpoint share this constructor,
	// so a new conversation always snapshots the same recent/default policy.
	sess, _, err := getOrCreateConversationSession(project.store, binding.SessionID, &seedCfg, binding.agentKind)
	if err != nil {
		return nil, err
	}
	runtime := ideChatRuntime{
		projectID: project.projectID, projectType: project.projectType, projectState: project.stateRoot, agentKind: project.agentKind,
		app: s.app, sess: sess, state: project.state, bookService: project.bookService,
		chatService: project.chatService, workspace: project.workspace,
		versionService: project.versionService, cfg: project.cfg,
	}
	runtime, req, err = s.app.chat().prepareProjectChatRuntimeSnapshot(ctx, runtime, req)
	if err != nil {
		return nil, err
	}
	runner, systemPrompt, err := buildProjectAgentRunnerWithComposition(ctx, runtime)
	if err != nil {
		return nil, err
	}
	conversation := projectSessionConversation(runtime, req)

	var verifiedMutations []agents.ToolMutation
	var postRunVerification agents.PostRunVerification
	mutationCallback := s.app.verifiedWorkspaceMutationCallback(
		"agent_chat_post_run", runtime.versionService, versionAutoSettingsForConfig(&runtime.cfg),
	)
	var accepted *agents.AcceptedRun
	run := &agentChatRun{binding: binding, commandID: req.CommandID, runtime: runtime}
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		run.task = task
		return s.installActiveRun(run)
	})
	if err != nil {
		return nil, err
	}
	reservation, err := s.starts.reserve(writingStartRecord{
		commandID: req.CommandID, workspace: binding.ProjectID, sessionID: binding.SessionID,
		fingerprint: fingerprint, task: task,
	})
	if err != nil {
		task.failBeforeStart(err)
		s.releaseActiveRun(run)
		return nil, err
	}

	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	startOptions := s.app.chat().bindReviewFeedbackInputCommit(
		agentChatStartOptions(run, req.ResolvedReviewFeedback.PrimaryReviewThreadID(), systemPrompt, func(mutations []agents.ToolMutation, verification agents.PostRunVerification) {
			verifiedMutations = append([]agents.ToolMutation(nil), mutations...)
			postRunVerification = verification
		}),
		runtime,
		req,
	)
	accepted, err = runtime.chatService.StartWithOptions(
		acceptCtx, runner, conversation, runtime.bookService, req,
		startOptions,
		task.emit,
	)
	releaseAcceptance()
	if err != nil {
		reservation.rollback()
		task.failBeforeStart(err)
		s.releaseActiveRun(run)
		if errors.Is(err, agents.ErrInvalidCommand) {
			return nil, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, req.CommandID)
		}
		return nil, err
	}

	if err := task.Start(func(runCtx context.Context, task *Task, emit func(agents.Event)) {
		defer s.releaseActiveRun(run)
		log.Printf("[agent-chat-run] begin task_id=%s project_id=%s workspace=%q session_id=%s message_len=%d", task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID, len(req.Message))
		accepted.Wait(runCtx)
		_, outputCommitted := conversation.LastAgentCycleCommitReceipt(agents.HarnessDomainCommitOutput)
		postSettlementCtx := runCtx
		if outputCommitted {
			postSettlementCtx = context.WithoutCancel(runCtx)
		}
		if outputCommitted && len(verifiedMutations) > 0 {
			mutationCallback(postSettlementCtx, verifiedMutations, postRunVerification)
		}
		log.Printf("[agent-chat-run] end task_id=%s project_id=%s workspace=%q session_id=%s status=%s", task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID, task.Status())
	}); err != nil {
		reservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.ctx)
		task.finish()
		s.releaseActiveRun(run)
		return nil, err
	}
	reservation.commit()
	return task, nil
}

func agentChatStartOptions(
	run *agentChatRun,
	reviewThreadID string,
	systemPrompt agents.SystemPromptComposition,
	onVerified func([]agents.ToolMutation, agents.PostRunVerification),
) agents.RunOptions {
	options := agentChatRunOptions(run.binding, run.task.ID())
	options.ReviewThreadID = strings.TrimSpace(reviewThreadID)
	options.IdleTimeout = agentIdleTimeout(run.runtime.cfg)
	options.ToolResultMaxBytes = agentToolResultMaxBytes(run.runtime.cfg)
	options.SystemPromptLog = systemPrompt
	options.OnMutationsVerified = func(_ context.Context, mutations []agents.ToolMutation, verification agents.PostRunVerification) {
		onVerified(mutations, verification)
	}
	return options
}

func (s *AgentChatAppService) resolveBinding(binding AgentChatBinding) (AgentChatBinding, error) {
	if s == nil || s.app == nil {
		return AgentChatBinding{}, ErrNoWorkspace
	}
	projectID := strings.TrimSpace(binding.ProjectID)
	if projectID == "" && strings.TrimSpace(binding.Workspace) != "" {
		record, _, err := s.app.resolveProjectByWorkspace(binding.Workspace)
		if err != nil {
			return AgentChatBinding{}, err
		}
		projectID = record.ID
	}
	record, layout, err := s.app.resolveProject(projectID, true)
	if err != nil && strings.TrimSpace(binding.Workspace) == "" && projectID != "" {
		// Temporary migration path for callers that still pass a directory in
		// the first argument. The resolved binding is immediately upgraded to ID.
		record, layout, err = s.app.resolveProjectByWorkspace(projectID)
	}
	if err != nil {
		return AgentChatBinding{}, err
	}
	binding.ProjectID = record.ID
	binding.Workspace = layout.ContentRoot
	binding.stateRoot = layout.StateRoot
	switch record.Type {
	case projectdomain.TypeBook:
		binding.agentKind = agents.AgentKindIDE
	case projectdomain.TypeGeneral:
		binding.agentKind = agents.AgentKindGeneral
	default:
		return AgentChatBinding{}, fmt.Errorf("unsupported project type %q", record.Type)
	}
	// Promote compatibility runtimes installed by older tests/clients under a
	// path key. Production runtimes are always keyed by stable project ID.
	s.mu.Lock()
	if s.projects[binding.ProjectID] == nil {
		if legacy := s.projects[binding.Workspace]; legacy != nil {
			legacy.projectID = binding.ProjectID
			legacy.projectType = record.Type
			legacy.agentKind = binding.agentKind
			legacy.stateRoot = binding.stateRoot
			s.projects[binding.ProjectID] = legacy
			delete(s.projects, binding.Workspace)
		}
	}
	s.mu.Unlock()
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	if binding.SessionID == "" {
		return AgentChatBinding{}, fmt.Errorf("AgentChat session is required")
	}
	return binding, nil
}

func (s *AgentChatAppService) projectRuntime(ctx context.Context, projectID string) (*agentChatProjectRuntime, error) {
	s.projectBuild.Lock()
	defer s.projectBuild.Unlock()
	s.mu.RLock()
	project := s.projects[projectID]
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if project != nil {
		return project, nil
	}

	// Start admission serializes first construction, so a workspace runtime is
	// built exactly once without a separate promise/cache state machine.
	a := s.app
	a.mu.RLock()
	chatService := a.chatService
	var runtimeCfg config.Config
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	a.mu.RUnlock()
	if chatService == nil || strings.TrimSpace(runtimeCfg.DataDir()) == "" {
		return nil, ErrNoWorkspace
	}
	record, layout, err := a.resolveProject(projectID, true)
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
	agentKind := agents.AgentKindGeneral
	if record.Type == projectdomain.TypeBook {
		state = book.NewState(workspace)
		if err := changes.WithExclusiveWorkspace(ctx, state.InitWorkspace); err != nil {
			return nil, err
		}
		versionService = book.NewVersionService(workspace)
		agentKind = agents.AgentKindIDE
	}
	store, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		return nil, err
	}
	runtimeCfg.Workspace = workspace
	runtimeCfg.ProjectID = record.ID
	runtimeCfg.ProjectStateDir = layout.StateRoot
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(runtimeCfg.DataDir(), workspace, layout.ConfigPath()); loadErr != nil {
		store.Close()
		if versionService != nil {
			versionService.Close()
		}
		return nil, loadErr
	} else {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
		runtimeCfg.Workspace = workspace
		runtimeCfg.ProjectID = record.ID
		runtimeCfg.ProjectStateDir = layout.StateRoot
	}
	project = &agentChatProjectRuntime{
		projectID: record.ID, projectType: record.Type, agentKind: agentKind, stateRoot: layout.StateRoot,
		workspace: workspace, state: state, store: store, bookService: book.NewService(workspace),
		versionService: versionService, chatService: chatService, cfg: runtimeCfg,
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		project.close()
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if existing := s.projects[projectID]; existing != nil {
		s.mu.Unlock()
		project.close()
		return existing, nil
	}
	s.projects[projectID] = project
	s.mu.Unlock()
	return project, nil
}

func (s *AgentChatAppService) activeRun(binding AgentChatBinding) *agentChatRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[agentChatBindingKey(binding)]
}

func (s *AgentChatAppService) installActiveRun(run *agentChatRun) error {
	if run == nil || run.task == nil {
		return fmt.Errorf("cannot install an empty AgentChat run")
	}
	key := agentChatBindingKey(run.binding)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("AgentChat service is closed")
	}
	if current := s.active[key]; current != nil && current.task != nil && !current.task.Finished() {
		return ErrAgentOperationActive
	}
	s.active[key] = run
	return nil
}

func (s *AgentChatAppService) releaseActiveRun(run *agentChatRun) {
	if s == nil || run == nil {
		return
	}
	key := agentChatBindingKey(run.binding)
	s.mu.Lock()
	if s.active[key] == run {
		delete(s.active, key)
	}
	s.mu.Unlock()
}

// closeProject drains the cached runtime for a project before archive or
// relink. Active conversations are never aborted implicitly by project
// management actions.
func (s *AgentChatAppService) closeProject(ctx context.Context, projectID string) error {
	if s == nil {
		return nil
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	s.mu.Lock()
	for _, run := range s.active {
		if run != nil && run.binding.ProjectID == projectID && run.task != nil && !run.task.Finished() {
			s.mu.Unlock()
			return fmt.Errorf("%w: project has a running Agent conversation", ErrAgentOperationActive)
		}
	}
	project := s.projects[projectID]
	delete(s.projects, projectID)
	s.mu.Unlock()
	if project != nil {
		defer project.close()
	}
	if s.app != nil {
		s.app.mu.RLock()
		chatService := s.app.chatService
		s.app.mu.RUnlock()
		if chatService != nil {
			if err := chatService.CloseProjectBindings(ctx, projectID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Close aborts every user-level conversation before the shared durable runtime
// is closed. It is intentionally not called by foreground workspace switches.
func (s *AgentChatAppService) Close(ctx context.Context) {
	if s == nil {
		return
	}
	s.admission.Lock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.admission.Unlock()
		return
	}
	s.closed = true
	runs := make([]*agentChatRun, 0, len(s.active))
	for _, run := range s.active {
		runs = append(runs, run)
	}
	projects := make([]*agentChatProjectRuntime, 0, len(s.projects))
	for _, project := range s.projects {
		projects = append(projects, project)
	}
	s.mu.Unlock()
	s.admission.Unlock()

	for _, run := range runs {
		if run != nil && run.task != nil {
			run.task.Abort()
		}
	}
	for _, run := range runs {
		if run != nil && run.task != nil {
			select {
			case <-run.task.Done():
			case <-ctx.Done():
				return
			}
		}
	}
	for _, project := range projects {
		project.close()
	}
}

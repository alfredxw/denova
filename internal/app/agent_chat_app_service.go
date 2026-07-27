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
	"denova/internal/workspacechange"
)

const agentChatRuntimeMode = "agent_chat"

// AgentChatBinding is the explicit project conversation identity carried by
// every user-level AgentChat request. It never reads or mutates App.workspace.
type AgentChatBinding struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
}

type agentChatProjectRuntime struct {
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
	return strings.TrimSpace(binding.Workspace) + "\x00" + strings.TrimSpace(binding.SessionID)
}

func agentChatRunOptions(binding AgentChatBinding, taskID string) agents.RunOptions {
	return agents.RunOptions{
		AgentKind: agents.AgentKindIDE, TaskID: strings.TrimSpace(taskID),
		SessionID: binding.SessionID, Workspace: binding.Workspace, Mode: agentChatRuntimeMode,
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
	if replay, ok, err := s.starts.replay(req.CommandID, binding.Workspace, binding.SessionID, fingerprint); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	if run := s.activeRun(binding); run != nil && run.task != nil && !run.task.Finished() {
		return nil, ErrAgentOperationActive
	}

	project, err := s.projectRuntime(ctx, binding.Workspace)
	if err != nil {
		return nil, err
	}
	// AgentChat blank tabs are local drafts. The stable client-generated session ID becomes
	// durable only when the first real turn reaches admission.
	sess, err := project.store.GetOrCreate(binding.SessionID)
	if err != nil {
		return nil, err
	}
	runtime := ideChatRuntime{
		app: s.app, sess: sess, state: project.state, bookService: project.bookService,
		chatService: project.chatService, workspace: project.workspace,
		versionService: project.versionService, cfg: project.cfg,
	}
	runtime, req, err = s.app.chat().prepareIDEChatRuntimeSnapshot(ctx, runtime, req)
	if err != nil {
		return nil, err
	}
	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		return nil, err
	}
	runtimeContexts := agents.IDEWorkspaceRuntimeContextsForRequest(runtime.state, req)
	conversation := agents.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)

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
		commandID: req.CommandID, workspace: binding.Workspace, sessionID: binding.SessionID,
		fingerprint: fingerprint, task: task,
	})
	if err != nil {
		task.failBeforeStart(err)
		s.releaseActiveRun(run)
		return nil, err
	}

	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = runtime.chatService.StartWithOptions(
		acceptCtx, runner, conversation, runtime.bookService, req,
		agentChatStartOptions(run, req.ResolvedReviewFeedback.PrimaryReviewThreadID(), systemPrompt, func(mutations []agents.ToolMutation, verification agents.PostRunVerification) {
			verifiedMutations = append([]agents.ToolMutation(nil), mutations...)
			postRunVerification = verification
		}),
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
		log.Printf("[agent-chat-run] begin task_id=%s workspace=%q session_id=%s message_len=%d", task.ID(), binding.Workspace, binding.SessionID, len(req.Message))
		accepted.Wait(runCtx)
		_, inputCommitted := conversation.LastAgentCycleCommitReceipt(agents.HarnessDomainCommitInput)
		_, outputCommitted := conversation.LastAgentCycleCommitReceipt(agents.HarnessDomainCommitOutput)
		postSettlementCtx := runCtx
		if inputCommitted || outputCommitted {
			postSettlementCtx = context.WithoutCancel(runCtx)
		}
		if inputCommitted && !req.ResolvedReviewFeedback.Empty() {
			if consumeErr := s.app.chat().consumeResolvedReviewFeedback(postSettlementCtx, runtime, req); consumeErr != nil {
				log.Printf("[agent-chat-run] consuming review feedback failed task_id=%s workspace=%q session_id=%s err=%v", task.ID(), binding.Workspace, binding.SessionID, consumeErr)
				emit(agents.Event{Type: "error", Data: map[string]string{"message": consumeErr.Error()}})
			} else {
				emit(agents.Event{Type: "workspace_change", Data: map[string]interface{}{
					"workspace": binding.Workspace, "review_thread_id": req.ResolvedReviewFeedback.PrimaryReviewThreadID(),
					"action": "review_feedback_consumed",
				}})
			}
		}
		if outputCommitted && len(verifiedMutations) > 0 {
			mutationCallback(postSettlementCtx, verifiedMutations, postRunVerification)
		}
		log.Printf("[agent-chat-run] end task_id=%s workspace=%q session_id=%s status=%s", task.ID(), binding.Workspace, binding.SessionID, task.Status())
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
	workspace, err := s.app.canonicalAgentChatWorkspace(binding.Workspace)
	if err != nil {
		return AgentChatBinding{}, err
	}
	binding.Workspace = workspace
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	if binding.SessionID == "" {
		return AgentChatBinding{}, fmt.Errorf("AgentChat session is required")
	}
	return binding, nil
}

func (s *AgentChatAppService) projectRuntime(ctx context.Context, workspace string) (*agentChatProjectRuntime, error) {
	s.projectBuild.Lock()
	defer s.projectBuild.Unlock()
	s.mu.RLock()
	project := s.projects[workspace]
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
	state := book.NewState(workspace)
	changes, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	if err := changes.WithExclusiveWorkspace(ctx, state.InitWorkspace); err != nil {
		return nil, err
	}
	store, err := session.NewStore(state.SessionDir())
	if err != nil {
		return nil, err
	}
	runtimeCfg.Workspace = workspace
	project = &agentChatProjectRuntime{
		workspace: workspace, state: state, store: store, bookService: book.NewService(workspace),
		versionService: book.NewVersionService(workspace), chatService: chatService, cfg: runtimeCfg,
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		project.close()
		return nil, fmt.Errorf("AgentChat service is closed")
	}
	if existing := s.projects[workspace]; existing != nil {
		s.mu.Unlock()
		project.close()
		return existing, nil
	}
	s.projects[workspace] = project
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

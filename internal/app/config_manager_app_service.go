package app

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/book"
)

type ConfigManagerAppService struct {
	app        *App
	admission  sync.Mutex
	starts     writingStartRegistry
	recoveries configManagerRecoveryRegistry
}

type configManagerTaskRuntime struct {
	cfg            config.Config
	workspace      string
	state          *book.State
	sessionStore   *session.Store
	bookService    *book.Service
	versionService *book.VersionService
	chatService    *agents.ChatService
	bookRegistry   *BookRegistry
}

const configManagerRequestContextValueMaxBytes = 2048

type ConfigManagerRequest struct {
	CommandID   string            `json:"command_id"`
	Instruction string            `json:"instruction"`
	Origin      string            `json:"origin,omitempty"`
	ResourceID  string            `json:"resource_id,omitempty"`
	StoryID     string            `json:"story_id,omitempty"`
	BranchID    string            `json:"branch_id,omitempty"`
	References  []string          `json:"references,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
	Locale      string            `json:"-"`
}

func (a *App) StartConfigManagerTask(ctx context.Context, req ConfigManagerRequest) *Task {
	return a.configManager().StartTask(ctx, req)
}

func (s *ConfigManagerAppService) StartTask(ctx context.Context, req ConfigManagerRequest) *Task {
	task, err := s.StartTaskWithError(ctx, req)
	if err != nil {
		log.Printf("[config-manager] start failed command_id=%s err=%v", strings.TrimSpace(req.CommandID), err)
		return nil
	}
	return task
}

func (a *App) StartConfigManagerTaskWithError(ctx context.Context, req ConfigManagerRequest) (*Task, error) {
	return a.configManager().StartTaskWithError(ctx, req)
}

func (s *ConfigManagerAppService) StartTaskWithError(ctx context.Context, req ConfigManagerRequest) (*Task, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	req.CommandID = strings.TrimSpace(req.CommandID)
	if req.CommandID == "" {
		return nil, ErrAgentCommandIDRequired
	}
	if err := agents.ValidateCommandID(req.CommandID); err != nil {
		return nil, err
	}
	a := s.app
	if a == nil {
		return nil, ErrNoWorkspace
	}
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if strings.TrimSpace(workspace) == "" {
		return nil, ErrNoWorkspace
	}
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return nil, err
	}
	message := buildConfigManagerMessage(req)
	chatReq := agents.CaptureChatRequestCallerInput(agents.ChatRequest{
		CommandID: req.CommandID, Message: message, LoreReferences: append([]string(nil), req.References...), Locale: req.Locale,
	})
	fingerprint := agents.ChatRequestSemanticFingerprint(chatReq)
	if replay, ok, err := s.starts.replay(req.CommandID, workspace, sessionID, fingerprint); err != nil {
		return nil, err
	} else if ok {
		return replay, nil
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	ctx = operation.Context()

	a.mu.RLock()
	var capturedConfig config.Config
	if a.cfg != nil {
		capturedConfig = *a.cfg
	}
	runtime := configManagerTaskRuntime{
		cfg: capturedConfig, workspace: a.workspace, state: a.bookState, sessionStore: a.sessionStore,
		bookService: a.bookService, versionService: a.versionService, chatService: a.chatService, bookRegistry: a.bookRegistry,
	}
	available := a.cfg != nil && a.bookState != nil && a.sessionStore != nil && a.chatService != nil && strings.TrimSpace(a.workspace) != ""
	a.mu.RUnlock()
	if replay, matched, err := s.replayDurableStart(
		ctx, runtime.chatService, runtime.bookService, chatReq, runtime.workspace, sessionID, fingerprint,
	); err != nil {
		return nil, err
	} else if matched {
		return replay, nil
	}
	if !available {
		return nil, ErrNoWorkspace
	}
	runtimeCfg := runtime.cfg
	runtimeCfg.Workspace = runtime.workspace
	if runtime.bookRegistry != nil {
		for _, record := range runtime.bookRegistry.List() {
			runtimeCfg.AutomationWorkspaces = append(runtimeCfg.AutomationWorkspaces, record.Path)
		}
	}
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		runtimeCfg.DataDir(), runtime.workspace, config.ProjectConfigPath(runtimeCfg.ProjectStateDir),
	); loadErr == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[config-manager] load layered settings failed workspace=%s err=%v", runtime.workspace, loadErr)
	}
	applyRequestLocaleToConfig(&runtimeCfg, req.Locale)
	resourceSkills, err := loadConfigManagerResourceSkills(ctx, &runtimeCfg, req)
	if err != nil {
		return nil, fmt.Errorf("load config manager resource Skills / 加载配置管理资源 Skills 失败: %w", err)
	}
	runner, systemPrompt, err := buildConfigManagerRunnerWithComposition(ctx, &runtimeCfg, runtime.state, resourceSkills...)
	if err != nil {
		return nil, err
	}
	sess, err := runtime.sessionStore.GetOrCreate(sessionID)
	if err != nil {
		return nil, err
	}
	conversation := agents.NewSessionConversationForAgent(sess, &runtimeCfg, config.AgentKindConfigManager)
	var accepted *agents.AcceptedRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspace != runtime.workspace || a.chatService != runtime.chatService {
			return ErrAgentContextChanged
		}
		return a.registerWorkspaceTaskLocked(task, runtime.workspace, true)
	})
	if err != nil {
		return nil, err
	}
	startReservation, err := s.starts.reserve(writingStartRecord{
		commandID: req.CommandID, workspace: runtime.workspace, sessionID: sess.ID,
		fingerprint: fingerprint, task: task,
	})
	if err != nil {
		task.failBeforeStart(err)
		a.unregisterWorkspaceTask(task)
		return nil, err
	}
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = runtime.chatService.StartWithOptions(acceptCtx, runner, conversation, runtime.bookService, chatReq, agents.RunOptions{
		AgentKind: agents.AgentKindConfigManager, StateRoot: runtimeCfg.ProjectStateDir,
		TaskID: task.ID(), SessionID: sess.ID, Workspace: runtime.workspace,
		Mode: "config_manager", IdleTimeout: agentIdleTimeout(runtimeCfg), ToolResultMaxBytes: agentToolResultMaxBytes(runtimeCfg),
		SystemPromptLog: systemPrompt,
		OnMutationsVerified: a.verifiedWorkspaceMutationCallback(
			"config_manager_post_run",
			runtime.versionService,
			versionAutoSettingsForConfig(&runtimeCfg),
		),
	}, task.emit)
	releaseAcceptance()
	if err != nil {
		startReservation.rollback()
		task.failBeforeStart(err)
		a.unregisterWorkspaceTask(task)
		return nil, err
	}
	if err := task.Start(func(ctx context.Context, task *Task, _ func(agents.Event)) {
		defer a.unregisterWorkspaceTask(task)
		log.Printf("[config-manager] run begin id=%s session_id=%s origin=%s resource_id=%s story_id=%s branch_id=%s message_len=%d", task.ID(), sess.ID, req.Origin, req.ResourceID, req.StoryID, req.BranchID, len(message))
		accepted.Wait(ctx)
		log.Printf("[config-manager] run end id=%s status=%s", task.ID(), task.Status())
	}); err != nil {
		startReservation.rollback()
		task.Abort()
		_ = accepted.Wait(task.ctx)
		task.finish()
		a.unregisterWorkspaceTask(task)
		return nil, err
	}
	startReservation.commit()
	return task, nil
}

func (a *App) ConfigManagerMessages(req ConfigManagerRequest) ([]session.HistoryEntry, error) {
	return a.configManager().Messages(req)
}

func (a *App) ConfigManagerMessagesPage(ctx context.Context, req ConfigManagerRequest, before, limit int) (session.HistoryPage, error) {
	return a.configManager().MessagesPage(ctx, req, before, limit)
}

func (s *ConfigManagerAppService) Messages(req ConfigManagerRequest) ([]session.HistoryEntry, error) {
	store := s.sessionStore()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return nil, err
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (s *ConfigManagerAppService) MessagesPage(ctx context.Context, req ConfigManagerRequest, before, limit int) (session.HistoryPage, error) {
	store := s.sessionStore()
	if store == nil {
		return session.HistoryPage{}, ErrNoWorkspace
	}
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return session.HistoryPage{}, err
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		return session.HistoryPage{}, err
	}
	return sess.ReadHistoryPage(ctx, before, limit)
}

func (a *App) ClearConfigManagerSession(req ConfigManagerRequest) error {
	return a.configManager().Clear(req)
}

func (s *ConfigManagerAppService) Clear(req ConfigManagerRequest) error {
	store := s.sessionStore()
	if store == nil {
		return ErrNoWorkspace
	}
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return err
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		return err
	}
	return sess.Clear()
}

func (s *ConfigManagerAppService) sessionStore() *session.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionStore
}

func buildConfigManagerMessage(req ConfigManagerRequest) string {
	instruction := strings.TrimSpace(req.Instruction)
	var lines []string
	lines = append(lines, "【模块上下文】")
	appendKV := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			value, truncated := trimStringToUTF8Bytes(value, configManagerRequestContextValueMaxBytes)
			if truncated {
				value += "\n  ...（已按请求上下文上限截断）"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", key, strings.TrimSpace(value)))
		}
	}
	appendKV("origin", req.Origin)
	appendKV("resource_id", req.ResourceID)
	appendKV("story_id", req.StoryID)
	appendKV("branch_id", req.BranchID)
	keys := make([]string, 0, len(req.Context))
	for key := range req.Context {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendKV(key, req.Context[key])
	}
	if len(req.References) > 0 {
		lines = append(lines, "- references: "+strings.Join(req.References, ", "))
	}
	lines = append(lines, "", "【用户指令】", instruction)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func configManagerSessionID(req ConfigManagerRequest) (string, error) {
	base, ok := agentSessionID(config.AgentKindConfigManager)
	if !ok {
		return "", fmt.Errorf("未配置 Agent 会话: %s", config.AgentKindConfigManager)
	}
	scopeValues := []string{
		strings.TrimSpace(req.Origin),
		strings.TrimSpace(req.StoryID),
		strings.TrimSpace(req.BranchID),
		strings.TrimSpace(req.ResourceID),
	}
	hasScope := false
	for _, value := range scopeValues {
		if value != "" {
			hasScope = true
			break
		}
	}
	if !hasScope {
		return base, nil
	}
	segments := []string{base, configManagerSessionSegment(req.Origin)}
	if story := configManagerSessionSegment(req.StoryID); story != "" {
		segments = append(segments, "story", story)
	}
	if branch := configManagerSessionSegment(req.BranchID); branch != "" {
		segments = append(segments, "branch", branch)
	}
	if resource := configManagerSessionSegment(req.ResourceID); resource != "" {
		segments = append(segments, "resource", resource)
	}
	sum := sha1.Sum([]byte(strings.Join(scopeValues, "\x00")))
	segments = append(segments, fmt.Sprintf("%x", sum)[:12])
	return strings.Join(segments, "-"), nil
}

func configManagerSessionSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == ' ' || r == '/' || r == ':' || r == '.' {
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(b.String(), "-")
	if segment == "" {
		return "scope"
	}
	const maxSegmentLen = 48
	if len(segment) > maxSegmentLen {
		return strings.Trim(segment[:maxSegmentLen], "-")
	}
	return segment
}

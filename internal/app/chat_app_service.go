package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/session"
)

// ChatAppService 负责普通创作 Agent 任务与会话管理。
type ChatAppService struct {
	app *App
}

type ideChatRuntime struct {
	sess           *session.Session
	state          *book.State
	bookService    *book.Service
	chatService    *agent.ChatService
	workspace      string
	versionService *book.VersionService
	cfg            config.Config
}

// ClearSession 为当前会话追加上下文清理标记。
func (a *App) ClearSession() error {
	return a.chat().ClearSession()
}

func (s *ChatAppService) ClearSession() error {
	a := s.app
	a.mu.RLock()
	sess := a.session
	a.mu.RUnlock()
	if sess == nil {
		return ErrNoWorkspace
	}
	return sess.Clear()
}

// Sessions 返回当前 workspace 下的会话列表。
func (a *App) Sessions() ([]session.SessionMeta, error) {
	return a.chat().Sessions()
}

func (s *ChatAppService) Sessions() ([]session.SessionMeta, error) {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	var activeID string
	if a.session != nil {
		activeID = a.session.ID
	}
	a.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return listUserSessions(store, activeID)
}

// CreateSession 新建会话并设置为当前激活会话。
func (a *App) CreateSession(title string) (*session.Session, error) {
	return a.chat().CreateSession(title)
}

func (s *ChatAppService) CreateSession(title string) (*session.Session, error) {
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionStore == nil {
		return nil, ErrNoWorkspace
	}
	s.abortActiveTaskLocked()

	sess, err := a.sessionStore.Create(title)
	if err != nil {
		return nil, err
	}
	if err := a.sessionStore.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	a.activeTask = nil
	return sess, nil
}

// SwitchSession 切换当前激活会话。
func (a *App) SwitchSession(id string) (*session.Session, error) {
	return a.chat().SwitchSession(id)
}

func (s *ChatAppService) SwitchSession(id string) (*session.Session, error) {
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionStore == nil {
		return nil, ErrNoWorkspace
	}
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能切换到固定 Agent 会话: %s", id)
	}
	s.abortActiveTaskLocked()

	sess, err := a.sessionStore.Get(id)
	if err != nil {
		return nil, err
	}
	if err := a.sessionStore.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	a.activeTask = nil
	return sess, nil
}

// RenameSession 修改会话标题。
func (a *App) RenameSession(id, title string) error {
	return a.chat().RenameSession(id, title)
}

func (s *ChatAppService) RenameSession(id, title string) error {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	if isAgentSessionID(id) {
		return fmt.Errorf("不能重命名固定 Agent 会话: %s", id)
	}
	return store.Rename(id, title)
}

// DeleteSession 删除会话；删除当前会话后自动切换到剩余最近会话。
func (a *App) DeleteSession(id string) (*session.Session, error) {
	return a.chat().DeleteSession(id)
}

func (s *ChatAppService) DeleteSession(id string) (*session.Session, error) {
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessionStore == nil {
		return nil, ErrNoWorkspace
	}
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能删除固定 Agent 会话: %s", id)
	}

	userSessions, err := listUserSessions(a.sessionStore, "")
	if err != nil {
		return nil, err
	}
	if len(userSessions) <= 1 {
		return nil, fmt.Errorf("不能删除当前唯一会话")
	}

	wasActive := a.session != nil && a.session.ID == id
	if wasActive {
		s.abortActiveTaskLocked()
	}
	if err := a.sessionStore.Delete(id); err != nil {
		return nil, err
	}
	activeID := ""
	if !wasActive && a.session != nil {
		activeID = a.session.ID
	}
	if activeID == "" {
		metas, err := listUserSessions(a.sessionStore, "")
		if err != nil {
			return nil, err
		}
		if len(metas) == 0 {
			sess, createErr := a.sessionStore.GetOrCreate("default")
			if createErr != nil {
				return nil, createErr
			}
			a.session = sess
			activeID = sess.ID
		} else {
			activeID = metas[0].ID
		}
	}
	sess, err := a.sessionStore.GetOrCreate(activeID)
	if err != nil {
		return nil, err
	}
	if err := a.sessionStore.SetActiveID(sess.ID); err != nil {
		return nil, err
	}
	a.session = sess
	if wasActive {
		a.activeTask = nil
	}
	return sess, nil
}

// SessionMessages 返回指定会话或当前会话的完整历史。
func (a *App) SessionMessages(id string) ([]session.HistoryEntry, error) {
	return a.chat().SessionMessages(id)
}

func (s *ChatAppService) SessionMessages(id string) ([]session.HistoryEntry, error) {
	a := s.app
	a.mu.RLock()
	store := a.sessionStore
	current := a.session
	a.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	if id == "" {
		if current == nil {
			return nil, ErrNoWorkspace
		}
		return current.History(), nil
	}
	if isAgentSessionID(id) {
		return nil, fmt.Errorf("不能通过创作会话读取固定 Agent 会话: %s", id)
	}
	sess, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

// StartTask 启动后台 Agent 任务。如果有正在运行的任务，先终止它。
func (a *App) StartTask(req agent.ChatRequest) *Task {
	return a.chat().StartTask(req)
}

func (s *ChatAppService) StartTask(req agent.ChatRequest) *Task {
	runtime, req, err := s.prepareIDEChatRuntime(req, true)
	if err != nil {
		log.Printf("[agent-task] 未选择 workspace，无法启动任务 err=%v", err)
		return nil
	}

	runner, err := buildAgentRunner(context.Background(), &runtime.cfg, runtime.state)
	if err != nil {
		log.Printf("[agent-task] 刷新 Agent Runner 失败 workspace=%s err=%v", runtime.workspace, err)
		return nil
	}
	a := s.app
	a.mu.Lock()
	if a.workspace == runtime.workspace {
		a.agentRunner = runner
	}
	a.mu.Unlock()

	var beforeVersionState book.VersionWorkspaceState
	var hasBeforeVersionState bool
	if runtime.versionService != nil {
		state, err := runtime.versionService.CaptureState()
		if err != nil {
			log.Printf("[versions] 捕获 Agent 运行前状态失败 workspace=%s err=%v", runtime.workspace, err)
		} else {
			beforeVersionState = state
			hasBeforeVersionState = true
		}
	}

	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[agent-task] run begin id=%s message_len=%d references=%d lore_references=%d style_references=%d style_rules=%d selections=%d plan_mode=%v", task.ID(), len(req.Message), len(req.References), len(req.LoreReferences), len(req.StyleReferences), len(req.StyleRules), len(req.Selections), req.PlanMode)
		runtime.chatService.RunWithOptions(ctx, runner, agent.NewSessionConversationForAgent(runtime.sess, &runtime.cfg, config.AgentKindIDE), runtime.bookService, req, agent.RunOptions{
			AgentKind:           agent.AgentKindIDE,
			TaskID:              task.ID(),
			SessionID:           runtime.sess.ID,
			Workspace:           runtime.workspace,
			Mode:                "ide",
			SystemPromptLog:     agent.BuildInstructionComposition(&runtime.cfg, runtime.state, ideStoryTellerForConfig(&runtime.cfg)),
			OnMutationsVerified: a.automationMutationCallback("ide_agent_post_run"),
		}, emit)
		if runtime.versionService != nil && hasBeforeVersionState {
			settings := book.DefaultVersionAutoSettings()
			settings.TimedEnabled = runtime.cfg.VersionTimedEnabled
			settings.TimedIntervalMinutes = runtime.cfg.VersionTimedIntervalMinutes
			settings.AgentEnabled = runtime.cfg.VersionAgentEnabled
			settings.AgentCharThreshold = runtime.cfg.VersionAgentCharThreshold
			result, err := runtime.versionService.MaybeCreateAgent(beforeVersionState, settings)
			if err != nil {
				log.Printf("[versions] Agent 自动保存失败 workspace=%s err=%v", runtime.workspace, err)
			} else if result.Skipped {
				log.Printf("[versions] Agent 自动保存跳过 workspace=%s reason=%q chars=%d", runtime.workspace, result.Reason, result.Chars)
			} else if result.Version != nil {
				log.Printf("[versions] Agent 自动保存完成 workspace=%s version=%s chars=%d", runtime.workspace, result.Version.ID, result.Chars)
			}
		}
		log.Printf("[agent-task] run end id=%s status=%s", task.ID(), task.Status())
	})

	a.mu.Lock()
	a.activeTask = task
	a.mu.Unlock()

	return task
}

func (a *App) AnalyzeContext(req agent.ChatRequest) (agent.ContextAnalysis, error) {
	return a.chat().AnalyzeContext(req)
}

func (s *ChatAppService) AnalyzeContext(req agent.ChatRequest) (agent.ContextAnalysis, error) {
	runtime, req, err := s.prepareIDEChatRuntime(req, false)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	var pending *session.Interruption
	if shouldResume := strings.TrimSpace(req.Message); shouldResume != "" {
		pending = runtime.sess.PendingInterruption()
	}
	var compaction *session.ContextCompaction
	if record, ok := runtime.sess.LatestContextCompaction(config.AgentKindIDE); ok {
		compaction = &record
	}
	return agent.BuildIDEContextAnalysis(&runtime.cfg, runtime.state, ideStoryTellerForConfig(&runtime.cfg), runtime.bookService, runtime.sess.GetEffectiveMessages(), runtime.sess.MessageCountTotal(), compaction, pending, req)
}

func (a *App) CompactContext(ctx context.Context) (agent.ContextCompactionResult, error) {
	return a.chat().CompactContext(ctx)
}

func (s *ChatAppService) CompactContext(ctx context.Context) (agent.ContextCompactionResult, error) {
	runtime, _, err := s.prepareIDEChatRuntime(agent.ChatRequest{}, false)
	if err != nil {
		return agent.ContextCompactionResult{}, err
	}
	conversation := agent.NewSessionConversationForAgent(runtime.sess, &runtime.cfg, config.AgentKindIDE)
	_, result, err := conversation.CompactContextIfNeeded(ctx, agent.ContextCompactionInput{
		Messages:       runtime.sess.GetEffectiveMessages(),
		Phase:          "manual",
		Force:          true,
		KeepLatestUser: true,
	})
	if err != nil {
		return result, err
	}
	if !result.Triggered {
		return result, fmt.Errorf("没有可压缩的上下文")
	}
	return result, nil
}

func (a *App) RemoveContextCompaction() (bool, error) {
	return a.chat().RemoveContextCompaction()
}

func (s *ChatAppService) RemoveContextCompaction() (bool, error) {
	a := s.app
	a.mu.RLock()
	sess := a.session
	a.mu.RUnlock()
	if sess == nil {
		return false, ErrNoWorkspace
	}
	_, removed, err := sess.RemoveLatestContextCompaction(config.AgentKindIDE, "user_removed")
	return removed, err
}

func (s *ChatAppService) prepareIDEChatRuntime(req agent.ChatRequest, abortRunning bool) (ideChatRuntime, agent.ChatRequest, error) {
	a := s.app
	a.mu.Lock()
	if a.session == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		return ideChatRuntime{}, req, ErrNoWorkspace
	}
	if abortRunning && a.activeTask != nil && a.activeTask.Status() == TaskRunning {
		log.Printf("[agent-task] replace running task id=%s", a.activeTask.ID())
		a.activeTask.Abort()
	}

	runtime := ideChatRuntime{
		sess:           a.session,
		state:          a.bookState,
		bookService:    a.bookService,
		chatService:    a.chatService,
		workspace:      a.workspace,
		versionService: a.versionService,
		cfg:            *a.cfg,
	}
	runtime.cfg.Workspace = runtime.workspace
	novaDir := runtime.cfg.NovaDir
	a.mu.Unlock()

	if layered, err := config.LoadLayered(novaDir, runtime.workspace); err == nil {
		applyLayeredSettingsToConfig(&runtime.cfg, layered)
		runtime.cfg.IDEStoryTellerID = layered.Effective.IDEStoryTellerID
		if runtime.cfg.IDEStoryTellerID == "" {
			runtime.cfg.IDEStoryTellerID = "classic"
		}
		log.Printf("[agent-task] load ide teller id=%s workspace=%s", runtime.cfg.IDEStoryTellerID, runtime.workspace)

		if len(req.StyleReferences) == 0 {
			teller := loadInteractiveTeller(novaDir, runtime.cfg.IDEStoryTellerID)
			if len(teller.StyleRules) > 0 {
				converted := convertTellerStyleRules(novaDir, teller.StyleRules)
				req.StyleRules = converted
				log.Printf("[agent-task] inject teller style rules teller_id=%s count=%d rules=%q", teller.ID, len(converted), appStyleRuleNames(converted))
			}
		}
	} else {
		log.Printf("[agent-task] load layered settings failed workspace=%s err=%v", runtime.workspace, err)
	}
	return runtime, req, nil
}

// ActiveTask 返回当前活跃任务（可能为 nil）。
func (a *App) ActiveTask() *Task {
	return a.chat().ActiveTask()
}

func (s *ChatAppService) ActiveTask() *Task {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeTask
}

// AbortTask 终止当前活跃任务。
func (a *App) AbortTask() {
	a.chat().AbortTask()
}

func (s *ChatAppService) AbortTask() {
	a := s.app
	a.mu.RLock()
	task := a.activeTask
	a.mu.RUnlock()
	if task != nil {
		task.Abort()
	}
}

func appStyleRuleNames(rules []agent.StyleRule) []string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		names = append(names, fmt.Sprintf("%s -> %v", rule.Scene, rule.Styles))
	}
	return names
}

func convertTellerStyleRules(novaDir string, rules []interactive.StyleRule) []agent.StyleRule {
	converted := make([]agent.StyleRule, 0, len(rules))
	for _, r := range rules {
		styles := resolveStyleRulePaths(novaDir, r.Styles)
		if strings.TrimSpace(r.Scene) == "" || len(styles) == 0 {
			continue
		}
		converted = append(converted, agent.StyleRule{Scene: r.Scene, Styles: styles})
	}
	return converted
}

func resolveStyleRulePaths(novaDir string, styles []string) []string {
	paths := make([]string, 0, len(styles))
	seen := make(map[string]bool, len(styles))
	for _, style := range styles {
		path := resolveStyleRulePath(novaDir, style)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func resolveStyleRulePath(novaDir string, style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		return ""
	}
	if !isStyleRuleFile(style) {
		return ""
	}
	if filepath.IsAbs(style) || novaDir == "" {
		return filepath.Clean(style)
	}
	cleanStyle := filepath.Clean(filepath.FromSlash(style))
	slashStyle := filepath.ToSlash(cleanStyle)
	if slashStyle == "." || slashStyle == ".." || strings.HasPrefix(slashStyle, "../") {
		return ""
	}
	if strings.HasPrefix(slashStyle, "styles/") {
		cleanStyle = filepath.FromSlash(strings.TrimPrefix(slashStyle, "styles/"))
	}
	if strings.HasPrefix(slashStyle, "setting/styles/") {
		cleanStyle = filepath.FromSlash(strings.TrimPrefix(slashStyle, "setting/styles/"))
	}
	return filepath.Join(book.UserStyleDir(novaDir), cleanStyle)
}

func isStyleRuleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}

func (s *ChatAppService) abortActiveTaskLocked() {
	if s.app.activeTask != nil && s.app.activeTask.Status() == TaskRunning {
		log.Printf("[agent-task] abort due to session switch/delete id=%s", s.app.activeTask.ID())
		s.app.activeTask.Abort()
	}
}

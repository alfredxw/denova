package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/interactive"
	"nova/internal/session"
)

type TellerAgentResult struct {
	Message string               `json:"message"`
	Action  string               `json:"action"`
	Teller  interactive.Teller   `json:"teller"`
	Tellers []interactive.Teller `json:"tellers"`
}

// InteractiveAppService 负责互动故事、剧情分支、导演和互动 Agent 任务。
type InteractiveAppService struct {
	app *App
}

func (a *App) InteractiveStories() (interactive.Index, error) {
	return a.interactiveService().InteractiveStories()
}

func (s *InteractiveAppService) InteractiveStories() (interactive.Index, error) {
	store := s.store()
	if store == nil {
		return interactive.Index{}, ErrNoWorkspace
	}
	return store.Index()
}

func (a *App) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStory(req)
}

func (s *InteractiveAppService) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	return store.CreateStory(req)
}

func (a *App) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().UpdateInteractiveStory(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	return store.UpdateStory(storyID, req)
}

func (a *App) DeleteInteractiveStory(storyID string) error {
	return a.interactiveService().DeleteInteractiveStory(storyID)
}

func (s *InteractiveAppService) DeleteInteractiveStory(storyID string) error {
	a := s.app
	a.mu.RLock()
	store := a.interactive
	sessionStore := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
	}
	if err := store.DeleteStory(storyID); err != nil {
		return err
	}
	if sessionStore != nil {
		return sessionStore.DeleteByPrefix("interactive-story-" + storyID + "-")
	}
	return nil
}

func (a *App) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	return a.interactiveService().InteractiveSnapshot(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	store := s.store()
	if store == nil {
		return interactive.Snapshot{}, ErrNoWorkspace
	}
	return store.Snapshot(storyID, branchID)
}

func (a *App) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	return a.interactiveService().CreateInteractiveBranch(storyID, req)
}

func (s *InteractiveAppService) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	store := s.store()
	if store == nil {
		return interactive.BranchSummary{}, ErrNoWorkspace
	}
	return store.CreateBranch(storyID, req)
}

func (a *App) SwitchInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().SwitchInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) SwitchInteractiveBranch(storyID, branchID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.SwitchBranch(storyID, branchID)
}

func (a *App) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	return a.interactiveService().SwitchInteractiveTurnVersion(storyID, req)
}

func (s *InteractiveAppService) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.SwitchTurnVersion(storyID, req)
}

func (a *App) DeleteInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().DeleteInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) DeleteInteractiveBranch(storyID, branchID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.DeleteBranch(storyID, branchID)
}

func (a *App) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	return a.interactiveService().InteractiveBranches(storyID)
}

func (s *InteractiveAppService) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	store := s.store()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return store.Branches(storyID)
}

func (a *App) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	return a.interactiveService().AppendInteractiveTurn(storyID, branchID, user, narrative)
}

func (s *InteractiveAppService) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	store := s.store()
	if store == nil {
		return interactive.TurnEvent{}, ErrNoWorkspace
	}
	return store.AppendTurn(storyID, interactive.AppendTurnRequest{
		BranchID:  branchID,
		User:      user,
		Narrative: narrative,
	})
}

// StartInteractiveTask 启动互动模式 Agent 任务，输出写回 interactive/story。
func (a *App) StartInteractiveTask(storyID, branchID, message string, styleReferences []string) *Task {
	return a.interactiveService().StartInteractiveTask(storyID, branchID, message, styleReferences)
}

func (s *InteractiveAppService) StartInteractiveTask(storyID, branchID, message string, styleReferences []string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleReferences, "")
}

func (a *App) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleReferences []string) *Task {
	return a.interactiveService().StartInteractiveRegenerateTask(storyID, branchID, turnID, message, styleReferences)
}

func (s *InteractiveAppService) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleReferences []string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleReferences, turnID)
}

func (s *InteractiveAppService) startInteractiveTask(storyID, branchID, message string, styleReferences []string, rewindTurnID string) *Task {
	a := s.app
	a.mu.Lock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		log.Printf("[interactive-agent-task] 未选择 workspace，无法启动任务")
		return nil
	}
	if a.activeInteractiveTask != nil && a.activeInteractiveTask.Status() == TaskRunning {
		log.Printf("[interactive-agent-task] replace running task id=%s", a.activeInteractiveTask.ID())
		a.activeInteractiveTask.Abort()
	}

	store := a.interactive
	state := a.bookState
	bookService := a.bookService
	chatService := a.chatService
	sessionStore := a.sessionStore
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.NovaDir
	a.mu.Unlock()

	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
		runtimeCfg.InteractiveMaxTokens = appSettingsInt(layered.Effective.InteractiveMaxTokens, 0)
		log.Printf("[interactive-agent-task] load interactive settings max_tokens=%d workspace=%s", runtimeCfg.InteractiveMaxTokens, workspace)
	} else {
		log.Printf("[interactive-agent-task] load interactive settings failed workspace=%s err=%v", workspace, err)
	}

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		log.Printf("[interactive-agent-task] 读取互动故事上下文失败 story_id=%s branch_id=%s err=%v", storyID, branchID, err)
		return nil
	}
	teller := loadInteractiveTeller(novaDir, storyCtx.Meta.StoryTellerID)
	runtimeCfg.InteractiveReplyTargetChars = storyCtx.Meta.ReplyTargetChars
	var styleRules []agent.StyleRule
	if len(styleReferences) == 0 {
		styleRules = convertTellerStyleRules(novaDir, teller.StyleRules)
		if len(styleRules) > 0 {
			log.Printf("[interactive-agent-task] inject teller style rules teller_id=%s count=%d rules=%q", teller.ID, len(styleRules), appStyleRuleNames(styleRules))
		}
	}
	log.Printf("[interactive-agent-task] use story settings story_id=%s teller_id=%s target_chars=%d style_rules=%d", storyID, teller.ID, runtimeCfg.InteractiveReplyTargetChars, len(styleRules))
	runner, err := buildInteractiveStoryRunner(context.Background(), &runtimeCfg, state, interactiveStoryTellerSystemInput(teller))
	if err != nil {
		log.Printf("[interactive-agent-task] 刷新互动故事 Agent Runner 失败 workspace=%s err=%v", workspace, err)
		return nil
	}
	a.mu.Lock()
	if a.workspace == workspace {
		a.interactiveStoryRunner = runner
	}
	a.mu.Unlock()

	if strings.TrimSpace(rewindTurnID) != "" {
		if err := store.RewindToTurnParent(storyID, interactive.RewindTurnRequest{BranchID: branchID, TurnID: rewindTurnID}); err != nil {
			log.Printf("[interactive-agent-task] 回退互动故事分支失败 story_id=%s branch_id=%s turn_id=%s err=%v", storyID, branchID, rewindTurnID, err)
			return nil
		}
		log.Printf("[interactive-agent-task] rewind branch for regeneration story_id=%s branch_id=%s turn_id=%s", storyID, branchID, rewindTurnID)
	}

	req := agent.ChatRequest{
		Message:         message,
		StyleReferences: styleReferences,
		StyleRules:      styleRules,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars)
	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-agent-task] run begin id=%s story_id=%s branch_id=%s rewind_turn_id=%s message_len=%d style_references=%d", task.ID(), storyID, branchID, rewindTurnID, len(message), len(styleReferences))
		chatService.RunWithOptions(ctx, runner, conversation, bookService, req, agent.RunOptions{
			AgentKind: agent.AgentKindInteractiveStory,
			TaskID:    task.ID(),
			Workspace: workspace,
			Mode:      "interactive",
		}, emit)
		if turn, stateReady, ok := conversation.LastTurnForState(); ok && !stateReady && ctx.Err() == nil {
			startInteractiveStateTask(&runtimeCfg, conversation, turn, sessionStore)
		}
		log.Printf("[interactive-agent-task] run end id=%s status=%s", task.ID(), task.Status())
	})

	a.mu.Lock()
	a.activeInteractiveTask = task
	a.mu.Unlock()

	return task
}

func (a *App) InteractiveTellers() ([]interactive.Teller, error) {
	return a.interactiveService().InteractiveTellers()
}

func (s *InteractiveAppService) InteractiveTellers() ([]interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).List()
}

func (a *App) InteractiveTeller(id string) (interactive.Teller, error) {
	return a.interactiveService().InteractiveTeller(id)
}

func (s *InteractiveAppService) InteractiveTeller(id string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Get(id)
}

func (a *App) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().CreateInteractiveTeller(teller)
}

func (s *InteractiveAppService) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Create(teller)
}

func (a *App) UpdateInteractiveTeller(id string, teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().UpdateInteractiveTeller(id, teller)
}

func (s *InteractiveAppService) UpdateInteractiveTeller(id string, teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Update(id, teller)
}

func (a *App) DeleteInteractiveTeller(id string) error {
	return a.interactiveService().DeleteInteractiveTeller(id)
}

func (s *InteractiveAppService) DeleteInteractiveTeller(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Delete(id)
}

func (a *App) TellerAgentMessages() ([]session.HistoryEntry, error) {
	return a.interactiveService().TellerAgentMessages()
}

func (s *InteractiveAppService) TellerAgentMessages() ([]session.HistoryEntry, error) {
	store := s.sessionStore()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := agentSessionFromStore(store, config.AgentKindTellerEditor)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) ClearTellerAgentSession() error {
	return a.interactiveService().ClearTellerAgentSession()
}

func (s *InteractiveAppService) ClearTellerAgentSession() error {
	store := s.sessionStore()
	if store == nil {
		return ErrNoWorkspace
	}
	return clearAgentSessionInStore(store, config.AgentKindTellerEditor)
}

func (a *App) StartTellerAgentTask(instruction string, targetID string, references []string) *Task {
	return a.interactiveService().StartTellerAgentTask(instruction, targetID, references)
}

func (s *InteractiveAppService) StartTellerAgentTask(instruction string, targetID string, references []string) *Task {
	cfg := s.cfg()
	sessionStore := s.sessionStore()
	if cfg == nil || cfg.NovaDir == "" || sessionStore == nil {
		return nil
	}
	runtimeCfg := *cfg
	library := interactive.NewTellerLibrary(runtimeCfg.NovaDir)
	targetID = strings.TrimSpace(targetID)
	sess, err := agentSessionFromStore(sessionStore, config.AgentKindTellerEditor)
	if err != nil {
		log.Printf("[teller-agent-task] 加载会话失败 err=%v", err)
		return nil
	}
	history := sess.GetEffectiveMessages()

	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		instruction = strings.TrimSpace(instruction)
		if err := sess.Append(schema.UserMessage(instruction)); err != nil {
			emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return
		}
		log.Printf("[teller-agent-task] run begin id=%s target_id=%s references=%d instruction_len=%d", task.ID(), targetID, len(references), len(instruction))
		if err := rejectUnsupportedTellerAgentInstruction(instruction); err != nil {
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 拒绝不支持的指令 target_id=%s err=%v", targetID, err)
			return
		}

		emitLoreToolCall(emit, "teller-read", "读取导演配置", fmt.Sprintf(`{"target_id":%q}`, targetID))
		tellers, err := library.List()
		if err != nil {
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 读取导演失败 target_id=%s err=%v", targetID, err)
			return
		}
		if targetID != "" && !tellerExists(tellers, targetID) {
			err := fmt.Errorf("目标导演不存在: %s", targetID)
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 目标导演不存在 target_id=%s", targetID)
			return
		}
		emitLoreToolResult(emit, "teller-read", fmt.Sprintf("已读取导演配置，共 %d 个。", len(tellers)))

		emitLoreToolCall(emit, "teller-plan", "生成导演编辑方案", fmt.Sprintf(`{"selected_teller_id":%q,"references":%d,"history_messages":%d}`, targetID, len(references), len(history)))
		plan, err := agent.StreamTellerEditPlan(ctx, &runtimeCfg, instruction, tellers, targetID, references, history, emit)
		if err != nil {
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 生成编辑方案失败 target_id=%s err=%v", targetID, err)
			return
		}
		emitLoreToolResult(emit, "teller-plan", fmt.Sprintf("已生成 %s 方案：%s。", plan.Action, plan.Message))

		applyTargetID := strings.TrimSpace(plan.Teller.ID)
		emitLoreToolCall(emit, "teller-apply", "应用导演变更", fmt.Sprintf(`{"action":%q,"teller_id":%q}`, plan.Action, applyTargetID))
		var teller interactive.Teller
		if plan.Action == "create" {
			teller, err = library.Create(plan.Teller)
		} else {
			teller, err = library.Update(applyTargetID, plan.Teller)
		}
		if err != nil {
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 应用变更失败 action=%s teller_id=%s err=%v", plan.Action, applyTargetID, err)
			return
		}
		nextTellers, err := library.List()
		if err != nil {
			emitTellerError(sess, emit, err)
			log.Printf("[teller-agent-task] 刷新导演列表失败 action=%s teller_id=%s err=%v", plan.Action, teller.ID, err)
			return
		}
		result := TellerAgentResult{
			Message: plan.Message,
			Action:  plan.Action,
			Teller:  teller,
			Tellers: nextTellers,
		}
		emitLoreToolResult(emit, "teller-apply", tellerResultMessage(result))
		_ = sess.Append(schema.AssistantMessage(tellerResultMessage(result), nil))
		emit(agent.Event{Type: "teller_result", Data: result})
		log.Printf("[teller-agent-task] run done id=%s action=%s teller_id=%s tellers=%d", task.ID(), plan.Action, teller.ID, len(nextTellers))
	})
}

// ActiveInteractiveTask 返回当前互动模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *Task {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeInteractiveTask
}

// AbortInteractiveTask 终止当前互动模式活跃任务。
func (a *App) AbortInteractiveTask() {
	a.interactiveService().AbortInteractiveTask()
}

func (s *InteractiveAppService) AbortInteractiveTask() {
	a := s.app
	a.mu.RLock()
	task := a.activeInteractiveTask
	a.mu.RUnlock()
	if task != nil {
		task.Abort()
	}
}

func (s *InteractiveAppService) store() *interactive.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.interactive
}

func (s *InteractiveAppService) cfg() *config.Config {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (s *InteractiveAppService) sessionStore() *session.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionStore
}

func emitTellerError(sess *session.Session, emit func(agent.Event), err error) {
	message := err.Error()
	_ = sess.Append(schema.AssistantMessage("执行失败："+message, nil))
	emit(agent.Event{Type: "error", Data: map[string]string{"message": message}})
}

func tellerResultMessage(result TellerAgentResult) string {
	action := "创建"
	if result.Action == "update" {
		action = "修改"
	}
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "导演 Agent 已完成"
	}
	if result.Teller.Name != "" {
		message += fmt.Sprintf("（%s：%s）", action, result.Teller.Name)
	}
	return message
}

func tellerExists(tellers []interactive.Teller, id string) bool {
	for _, teller := range tellers {
		if teller.ID == id {
			return true
		}
	}
	return false
}

func rejectUnsupportedTellerAgentInstruction(instruction string) error {
	normalized := strings.ToLower(strings.TrimSpace(instruction))
	deleteWords := []string{"删除", "删掉", "移除", "remove", "delete"}
	tellerWords := []string{"导演", "讲述者", "story director", "director", "story teller", "teller", "规则包"}
	for _, deleteWord := range deleteWords {
		if !strings.Contains(normalized, deleteWord) {
			continue
		}
		for _, tellerWord := range tellerWords {
			if strings.Contains(normalized, tellerWord) {
				return fmt.Errorf("导演 Agent 当前只支持创建或修改单个导演，不支持删除")
			}
		}
	}
	return nil
}

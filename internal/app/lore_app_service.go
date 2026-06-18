package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/session"
)

// LoreAppService 负责资料库 CRUD 和资料库 Agent。
type LoreAppService struct {
	app *App
}

func (a *App) LoreItems() ([]book.LoreItem, error) {
	return a.lore().LoreItems()
}

func (s *LoreAppService) LoreItems() ([]book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return nil, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).List()
}

func (a *App) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().CreateLoreItem(input)
}

func (s *LoreAppService) CreateLoreItem(input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Create(input)
}

func (a *App) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	return a.lore().UpdateLoreItem(id, input)
}

func (s *LoreAppService) UpdateLoreItem(id string, input book.LoreItemInput) (book.LoreItem, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreItem{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Update(id, input)
}

func (a *App) DeleteLoreItem(id string) error {
	return a.lore().DeleteLoreItem(id)
}

func (s *LoreAppService) DeleteLoreItem(id string) error {
	state := s.bookState()
	if state == nil {
		return ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).Delete(id)
}

func (a *App) RunLoreAgent(ctx context.Context, instruction string, references []string) (book.LoreApplyResult, error) {
	return a.lore().RunLoreAgent(ctx, instruction, references)
}

func (s *LoreAppService) RunLoreAgent(ctx context.Context, instruction string, references []string) (book.LoreApplyResult, error) {
	a := s.app
	a.mu.RLock()
	state := a.bookState
	cfg := a.cfg
	workspace := a.workspace
	a.mu.RUnlock()
	if state == nil || cfg == nil {
		return book.LoreApplyResult{}, ErrNoWorkspace
	}
	runtimeCfg := *cfg
	runtimeCfg.Workspace = workspace
	var history []*schema.Message
	var sess *session.Session
	if store := s.sessionStore(); store != nil {
		loaded, err := agentSessionFromStore(store, config.AgentKindLoreEditor)
		if err != nil {
			return book.LoreApplyResult{}, err
		}
		sess = loaded
		history = sess.GetEffectiveMessages()
		if err := sess.Append(schema.UserMessage(strings.TrimSpace(instruction))); err != nil {
			return book.LoreApplyResult{}, err
		}
	}
	store := book.NewLoreStore(state.Workspace())
	items, err := store.List()
	if err != nil {
		return book.LoreApplyResult{}, err
	}
	plan, err := agent.GenerateLoreEditPlan(ctx, &runtimeCfg, instruction, items, references, history)
	if err != nil {
		if sess != nil {
			_ = sess.Append(schema.AssistantMessage("执行失败："+err.Error(), nil))
		}
		return book.LoreApplyResult{}, err
	}
	result, err := store.ApplyOperations(plan.Message, plan.Ops)
	if sess != nil {
		if err != nil {
			_ = sess.Append(schema.AssistantMessage("执行失败："+err.Error(), nil))
		} else {
			_ = sess.Append(schema.AssistantMessage(loreResultMessage(result), nil))
		}
	}
	return result, err
}

func (a *App) LoreAgentMessages() ([]session.HistoryEntry, error) {
	return a.lore().LoreAgentMessages()
}

func (s *LoreAppService) LoreAgentMessages() ([]session.HistoryEntry, error) {
	store := s.sessionStore()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := agentSessionFromStore(store, config.AgentKindLoreEditor)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) ClearLoreAgentSession() error {
	return a.lore().ClearLoreAgentSession()
}

func (s *LoreAppService) ClearLoreAgentSession() error {
	store := s.sessionStore()
	if store == nil {
		return ErrNoWorkspace
	}
	return clearAgentSessionInStore(store, config.AgentKindLoreEditor)
}

func (a *App) StartLoreAgentTask(instruction string, references []string) *Task {
	return a.lore().StartLoreAgentTask(instruction, references)
}

func (s *LoreAppService) StartLoreAgentTask(instruction string, references []string) *Task {
	a := s.app
	a.mu.Lock()
	state := a.bookState
	cfg := a.cfg
	workspace := a.workspace
	sessionStore := a.sessionStore
	bookService := a.bookService
	chatService := a.chatService
	a.mu.Unlock()
	if state == nil || cfg == nil || sessionStore == nil {
		return nil
	}
	runtimeCfg := *cfg
	runtimeCfg.Workspace = workspace
	sess, err := agentSessionFromStore(sessionStore, config.AgentKindLoreEditor)
	if err != nil {
		log.Printf("[lore-agent-task] load session failed err=%v", err)
		return nil
	}

	if layered, err := config.LoadLayered(runtimeCfg.NovaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[lore-agent-task] load layered settings failed workspace=%s err=%v", workspace, err)
	}
	runner, err := buildLoreAgentRunner(context.Background(), &runtimeCfg, state)
	if err != nil {
		log.Printf("[lore-agent-task] 构建资料库 Agent Runner 失败 workspace=%s err=%v", workspace, err)
		return nil
	}

	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		req := agent.ChatRequest{
			Message:        strings.TrimSpace(instruction),
			LoreReferences: references,
		}
		log.Printf("[lore-agent-task] run begin id=%s message_len=%d lore_references=%d", task.ID(), len(req.Message), len(req.LoreReferences))
		chatService.RunWithOptions(ctx, runner, agent.NewSessionConversation(sess), bookService, req, agent.RunOptions{
			AgentKind:           agent.AgentKindLoreEditor,
			TaskID:              task.ID(),
			SessionID:           sess.ID,
			Workspace:           workspace,
			Mode:                "lore",
			OnMutationsVerified: a.automationMutationCallback("lore_agent_post_run"),
		}, emit)
		log.Printf("[lore-agent-task] run end id=%s status=%s", task.ID(), task.Status())
	})
}

func (s *LoreAppService) bookState() *book.State {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookState
}

func (s *LoreAppService) sessionStore() *session.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionStore
}

func emitLoreToolCall(emit func(agent.Event), id, name, args string) {
	emit(agent.Event{Type: "tool_call", Data: map[string]any{
		"id":   id,
		"name": name,
		"args": args,
	}})
}

func emitLoreToolResult(emit func(agent.Event), id, content string) {
	emit(agent.Event{Type: "tool_result", Data: map[string]any{
		"id":      id,
		"content": content,
	}})
}

func emitLoreError(sess *session.Session, emit func(agent.Event), err error) {
	message := err.Error()
	_ = sess.Append(schema.AssistantMessage("执行失败："+message, nil))
	emit(agent.Event{Type: "error", Data: map[string]string{"message": message}})
}

func loreResultMessage(result book.LoreApplyResult) string {
	changed := []string{}
	if len(result.Created) > 0 {
		changed = append(changed, fmt.Sprintf("新增 %d", len(result.Created)))
	}
	if len(result.Updated) > 0 {
		changed = append(changed, fmt.Sprintf("更新 %d", len(result.Updated)))
	}
	if len(result.DeletedIDs) > 0 {
		changed = append(changed, fmt.Sprintf("删除 %d", len(result.DeletedIDs)))
	}
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "资料库 Agent 已完成"
	}
	if len(changed) > 0 {
		message += "（" + strings.Join(changed, "，") + "）"
	}
	if len(result.Created) > 0 {
		message += "\n新增：" + loreItemNames(result.Created)
	}
	if len(result.Updated) > 0 {
		message += "\n更新：" + loreItemNames(result.Updated)
	}
	if len(result.DeletedIDs) > 0 {
		message += "\n删除：" + strings.Join(result.DeletedIDs, "，")
	}
	return message
}

func loreItemNames(items []book.LoreItem) string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return strings.Join(names, "，")
}

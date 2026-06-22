package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/session"
)

type ConfigManagerAppService struct {
	app *App
}

type ConfigManagerRequest struct {
	Instruction string            `json:"instruction"`
	Origin      string            `json:"origin,omitempty"`
	ResourceID  string            `json:"resource_id,omitempty"`
	StoryID     string            `json:"story_id,omitempty"`
	BranchID    string            `json:"branch_id,omitempty"`
	References  []string          `json:"references,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
}

func (a *App) StartConfigManagerTask(req ConfigManagerRequest) *Task {
	return a.configManager().StartTask(req)
}

func (s *ConfigManagerAppService) StartTask(req ConfigManagerRequest) *Task {
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
	if layered, err := config.LoadLayered(runtimeCfg.NovaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[config-manager] load layered settings failed workspace=%s err=%v", workspace, err)
	}
	sess, err := agentSessionFromStore(sessionStore, config.AgentKindConfigManager)
	if err != nil {
		log.Printf("[config-manager] load session failed err=%v", err)
		return nil
	}
	runner, err := buildConfigManagerRunner(context.Background(), &runtimeCfg, state)
	if err != nil {
		log.Printf("[config-manager] build runner failed workspace=%s err=%v", workspace, err)
		return nil
	}
	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		message := buildConfigManagerMessage(req)
		log.Printf("[config-manager] run begin id=%s origin=%s resource_id=%s story_id=%s branch_id=%s message_len=%d", task.ID(), req.Origin, req.ResourceID, req.StoryID, req.BranchID, len(message))
		chatService.RunWithOptions(ctx, runner, agent.NewSessionConversationForAgent(sess, &runtimeCfg, config.AgentKindConfigManager), bookService, agent.ChatRequest{
			Message:        message,
			LoreReferences: req.References,
		}, agent.RunOptions{
			AgentKind:           agent.AgentKindConfigManager,
			TaskID:              task.ID(),
			SessionID:           sess.ID,
			Workspace:           workspace,
			Mode:                "config_manager",
			SystemPromptLog:     agent.BuildConfigManagerInstructionComposition(&runtimeCfg, state),
			OnMutationsVerified: a.automationMutationCallback("config_manager_post_run"),
		}, emit)
		log.Printf("[config-manager] run end id=%s status=%s", task.ID(), task.Status())
	})
}

func (a *App) ConfigManagerMessages() ([]session.HistoryEntry, error) {
	return a.configManager().Messages()
}

func (s *ConfigManagerAppService) Messages() ([]session.HistoryEntry, error) {
	store := s.sessionStore()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := agentSessionFromStore(store, config.AgentKindConfigManager)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) ClearConfigManagerSession() error {
	return a.configManager().Clear()
}

func (s *ConfigManagerAppService) Clear() error {
	store := s.sessionStore()
	if store == nil {
		return ErrNoWorkspace
	}
	return clearAgentSessionInStore(store, config.AgentKindConfigManager)
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
			lines = append(lines, fmt.Sprintf("- %s: %s", key, strings.TrimSpace(value)))
		}
	}
	appendKV("origin", req.Origin)
	appendKV("resource_id", req.ResourceID)
	appendKV("story_id", req.StoryID)
	appendKV("branch_id", req.BranchID)
	for key, value := range req.Context {
		appendKV(key, value)
	}
	if len(req.References) > 0 {
		lines = append(lines, "- references: "+strings.Join(req.References, ", "))
	}
	lines = append(lines, "", "【用户指令】", instruction)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

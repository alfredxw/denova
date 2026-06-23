package app

import (
	"context"
	"crypto/sha1"
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
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		log.Printf("[config-manager] resolve session failed origin=%s resource_id=%s story_id=%s branch_id=%s err=%v", req.Origin, req.ResourceID, req.StoryID, req.BranchID, err)
		return nil
	}
	sess, err := sessionStore.GetOrCreate(sessionID)
	if err != nil {
		log.Printf("[config-manager] load session failed session_id=%s err=%v", sessionID, err)
		return nil
	}
	resourceSkills := loadConfigManagerResourceSkills(context.Background(), &runtimeCfg, req)
	runner, err := buildConfigManagerRunner(context.Background(), &runtimeCfg, state, resourceSkills...)
	if err != nil {
		log.Printf("[config-manager] build runner failed workspace=%s err=%v", workspace, err)
		return nil
	}
	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		message := buildConfigManagerMessage(req)
		log.Printf("[config-manager] run begin id=%s session_id=%s origin=%s resource_id=%s story_id=%s branch_id=%s message_len=%d", task.ID(), sess.ID, req.Origin, req.ResourceID, req.StoryID, req.BranchID, len(message))
		chatService.RunWithOptions(ctx, runner, agent.NewSessionConversationForAgent(sess, &runtimeCfg, config.AgentKindConfigManager), bookService, agent.ChatRequest{
			Message:        message,
			LoreReferences: req.References,
		}, agent.RunOptions{
			AgentKind:           agent.AgentKindConfigManager,
			TaskID:              task.ID(),
			SessionID:           sess.ID,
			Workspace:           workspace,
			Mode:                "config_manager",
			SystemPromptLog:     agent.BuildConfigManagerInstructionComposition(&runtimeCfg, state, resourceSkills...),
			OnMutationsVerified: a.automationMutationCallback("config_manager_post_run"),
		}, emit)
		log.Printf("[config-manager] run end id=%s status=%s", task.ID(), task.Status())
	})
}

func (a *App) ConfigManagerMessages(req ConfigManagerRequest) ([]session.HistoryEntry, error) {
	return a.configManager().Messages(req)
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

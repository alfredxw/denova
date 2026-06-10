package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/automation"
	"nova/internal/book"
	"nova/internal/session"
)

type AutomationAppService struct {
	app *App
}

func (a *App) StartAutomationScheduler(ctx context.Context) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[automation] scheduler panic recovered err=%v", recovered)
			}
		}()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[automation] scheduler stopped err=%v", ctx.Err())
				return
			case now := <-ticker.C:
				a.runAutomationSchedulerTick(ctx, now)
			}
		}
	}()
}

func (a *App) runAutomationSchedulerTick(ctx context.Context, now time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automation] scheduler tick panic recovered workspace=%q err=%v", a.Workspace(), recovered)
		}
	}()
	a.RunDueAutomations(ctx, now)
}

func (a *App) Automations() ([]automation.Task, error) {
	return a.automation().List()
}

func (s *AutomationAppService) List() ([]automation.Task, error) {
	store := s.store()
	return store.List()
}

func (a *App) CreateAutomation(task automation.Task) (automation.Task, error) {
	return a.automation().Create(task)
}

func (s *AutomationAppService) Create(task automation.Task) (automation.Task, error) {
	return s.store().Create(task)
}

func (a *App) UpdateAutomation(id string, task automation.Task) (automation.Task, error) {
	return a.automation().Update(id, task)
}

func (s *AutomationAppService) Update(id string, task automation.Task) (automation.Task, error) {
	return s.store().Update(id, task)
}

func (a *App) DeleteAutomation(id string) error {
	return a.automation().Delete(id)
}

func (s *AutomationAppService) Delete(id string) error {
	return s.store().Delete(id)
}

func (a *App) RunAutomation(ctx context.Context, id, trigger string) (automation.RunResult, error) {
	return a.automation().Run(ctx, id, trigger)
}

func (s *AutomationAppService) Run(ctx context.Context, id, trigger string) (result automation.RunResult, err error) {
	task, err := s.store().Get(id)
	if err != nil {
		return automation.RunResult{}, err
	}
	run := automation.RunRecord{
		ID:        automation.NewRunID(),
		TaskID:    task.ID,
		Scope:     task.Scope,
		Workspace: s.workspace(),
		Trigger:   normalizeAutomationTrigger(trigger),
		Status:    automation.RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("automation panic recovered: %v", recovered)
			log.Printf("[automation] panic recovered task_id=%s scope=%s workspace=%q trigger=%s err=%v", task.ID, task.Scope, run.Workspace, run.Trigger, recovered)
		}
		if err != nil {
			run.Status = automation.RunStatusFailed
			run.Error = err.Error()
			run.FinishedAt = time.Now().UTC()
			if updated, appendErr := s.store().AppendRun(task.ID, run); appendErr == nil {
				result = automation.RunResult{Task: updated, Run: run}
			}
		}
	}()

	log.Printf("[automation] run begin task_id=%s scope=%s workspace=%q trigger=%s template=%s", task.ID, task.Scope, run.Workspace, run.Trigger, task.Template)
	runtimeCfg := s.runtimeConfig()
	runtimeCfg = constrainAutomationTools(runtimeCfg, task.WritePolicy)
	run.ToolManifest = automationToolManifest(&runtimeCfg)
	taskInstruction := agent.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WritePolicy:  task.WritePolicy,
		OutputPolicy: task.OutputPolicy,
		OutputPath:   task.OutputPath,
		Workspace:    run.Workspace,
	}
	runner, buildErr := buildAutomationAgentRunner(ctx, &runtimeCfg, s.bookState(), taskInstruction)
	if buildErr != nil {
		err = buildErr
		return result, err
	}
	conversation := &automationConversation{}
	var runError string
	emit := func(ev agent.Event) {
		switch ev.Type {
		case "error":
			runError = eventMessage(ev.Data)
		case "tool_call":
			log.Printf("[automation] tool call task_id=%s data=%v", task.ID, ev.Data)
		case "tool_result":
			log.Printf("[automation] tool result task_id=%s data=%v", task.ID, ev.Data)
		}
	}
	s.app.ChatService().Run(ctx, runner, conversation, s.app.BookService(), agent.ChatRequest{
		Message: buildAutomationUserMessage(task, run),
	}, emit)
	if runError != "" {
		err = fmt.Errorf("%s", runError)
		return result, err
	}
	output := conversation.Output()
	if strings.TrimSpace(output) == "" {
		output = "自动化任务已完成，Agent 未返回文字摘要。"
	}
	run.Summary = strings.TrimSpace(output)
	if path, writeErr := s.writeOptionalOutput(task, output, runtimeCfg); writeErr != nil {
		err = writeErr
		return result, err
	} else if path != "" {
		run.OutputPath = path
	}
	run.Status = automation.RunStatusSuccess
	run.FinishedAt = time.Now().UTC()
	updated, err := s.store().AppendRun(task.ID, run)
	if err != nil {
		return automation.RunResult{}, err
	}
	log.Printf("[automation] run done task_id=%s scope=%s workspace=%q trigger=%s status=%s output_path=%q", task.ID, task.Scope, run.Workspace, run.Trigger, run.Status, run.OutputPath)
	return automation.RunResult{Task: updated, Run: run}, nil
}

func (a *App) RunDueAutomations(ctx context.Context, now time.Time) []automation.RunResult {
	return a.automation().RunDue(ctx, now)
}

func (s *AutomationAppService) RunDue(ctx context.Context, now time.Time) []automation.RunResult {
	tasks, err := s.List()
	if err != nil {
		log.Printf("[automation] list due tasks failed err=%v", err)
		return nil
	}
	results := []automation.RunResult{}
	for _, task := range tasks {
		if !automation.Due(now, task) {
			continue
		}
		result, err := s.Run(ctx, task.ID, automation.TriggerSchedule)
		if err != nil {
			log.Printf("[automation] due task failed task_id=%s scope=%s workspace=%q err=%v", task.ID, task.Scope, s.workspace(), err)
		}
		results = append(results, result)
	}
	return results
}

func (s *AutomationAppService) store() *automation.Store {
	a := s.app
	a.mu.RLock()
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.NovaDir
	}
	workspace := a.workspace
	a.mu.RUnlock()
	return automation.NewStore(novaDir, workspace)
}

func (s *AutomationAppService) workspace() string {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace
}

func (s *AutomationAppService) bookState() *book.State {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.bookState
}

type automationConversation struct {
	output string
}

func (c *automationConversation) PrepareMessages(_, agentMessage string) ([]*schema.Message, error) {
	return []*schema.Message{schema.UserMessage(agentMessage)}, nil
}

func (c *automationConversation) AppendAssistant(content string) error {
	c.output = content
	return nil
}

func (c *automationConversation) AppendAssistantWithThinking(content, _ string) error {
	c.output = content
	return nil
}

func (c *automationConversation) MarkInterrupted(_, _, _ string) error {
	return nil
}

func (c *automationConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *automationConversation) ResolveInterruption(string) error {
	return nil
}

func (c *automationConversation) Output() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.output)
}

func (s *AutomationAppService) runtimeConfig() config.Config {
	a := s.app
	a.mu.RLock()
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	workspace := a.workspace
	novaDir := runtimeCfg.NovaDir
	a.mu.RUnlock()
	runtimeCfg.Workspace = workspace
	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[automation] load layered settings failed workspace=%s err=%v", workspace, err)
	}
	return runtimeCfg
}

func (s *AutomationAppService) writeOptionalOutput(task automation.Task, output string, cfg config.Config) (string, error) {
	if task.OutputPolicy != automation.OutputPolicyOptionalFile || strings.TrimSpace(task.OutputPath) == "" {
		return "", nil
	}
	if !automationTaskAllowsFileWrite(task.WritePolicy) {
		return "", fmt.Errorf("task write policy does not allow file output")
	}
	if !config.ResolveAgentTools(&cfg, config.AgentKindAutomation).FileWrite {
		return "", fmt.Errorf("Automation Agent file_write tool is disabled")
	}
	bookService := s.app.BookService()
	if bookService == nil {
		return "", ErrNoWorkspace
	}
	rel := filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(task.OutputPath), "/"))
	if rel == "" {
		return "", fmt.Errorf("output_path is required")
	}
	if err := bookService.WriteFile(rel, output); err != nil {
		return "", err
	}
	return rel, nil
}

func normalizeAutomationTrigger(trigger string) string {
	if trigger == automation.TriggerSchedule {
		return automation.TriggerSchedule
	}
	return automation.TriggerManual
}

func automationTaskAllowsFileWrite(policy string) bool {
	return policy == automation.WritePolicyAllowFileWrite || policy == automation.WritePolicyAllowLoreAndFileWrite
}

func automationTaskAllowsLoreWrite(policy string) bool {
	return policy == automation.WritePolicyAllowLoreWrite || policy == automation.WritePolicyAllowLoreAndFileWrite
}

func constrainAutomationTools(cfg config.Config, writePolicy string) config.Config {
	resolved := config.ResolveAgentTools(&cfg, config.AgentKindAutomation)
	cfg.AgentTools.Automation = config.AgentToolOverride{
		FileRead:     boolPointer(resolved.FileRead),
		FileWrite:    boolPointer(resolved.FileWrite && automationTaskAllowsFileWrite(writePolicy)),
		ShellExecute: boolPointer(resolved.ShellExecute),
		Skills:       boolPointer(resolved.Skills),
		LoreRead:     boolPointer(resolved.LoreRead),
		LoreWrite:    boolPointer(resolved.LoreWrite && automationTaskAllowsLoreWrite(writePolicy)),
		Todo:         boolPointer(resolved.Todo),
	}
	return cfg
}

func automationToolManifest(cfg *config.Config) []automation.ToolManifestItem {
	tools := config.ResolveAgentTools(cfg, config.AgentKindAutomation)
	return []automation.ToolManifestItem{
		{Source: config.AgentToolFileRead, Allowed: tools.FileRead},
		{Source: config.AgentToolFileWrite, Allowed: tools.FileWrite},
		{Source: config.AgentToolShellExecute, Allowed: tools.ShellExecute},
		{Source: config.AgentToolSkills, Allowed: tools.Skills},
		{Source: config.AgentToolLoreRead, Allowed: tools.LoreRead},
		{Source: config.AgentToolLoreWrite, Allowed: tools.LoreWrite},
		{Source: config.AgentToolTodo, Allowed: tools.Todo},
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func eventMessage(data interface{}) string {
	switch typed := data.(type) {
	case map[string]string:
		return strings.TrimSpace(typed["message"])
	case map[string]interface{}:
		return strings.TrimSpace(fmt.Sprint(typed["message"]))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(data))
	}
}

func buildAutomationUserMessage(task automation.Task, run automation.RunRecord) string {
	var sb strings.Builder
	sb.WriteString("执行 Nova 自动化任务。\n\n")
	sb.WriteString(fmt.Sprintf("任务名称：%s\n", task.Name))
	sb.WriteString(fmt.Sprintf("模板：%s\n", task.Template))
	sb.WriteString(fmt.Sprintf("触发来源：%s\n", run.Trigger))
	sb.WriteString(fmt.Sprintf("写入策略：%s\n", task.WritePolicy))
	sb.WriteString(fmt.Sprintf("输出策略：%s\n", task.OutputPolicy))
	if task.OutputPath != "" {
		sb.WriteString(fmt.Sprintf("输出文件：%s\n", task.OutputPath))
	}
	sb.WriteString("\n用户 Prompt：\n")
	if task.Prompt != "" {
		sb.WriteString(task.Prompt)
	} else {
		sb.WriteString(defaultAutomationPrompt(task.Template))
	}
	sb.WriteString("\n\n请你自行使用可用工具读取完成任务所需的工作区文件、资料库和状态；先定位范围，再读取和写入。")
	return sb.String()
}

func defaultAutomationPrompt(template string) string {
	switch template {
	case automation.TemplateMemoryConsolidation:
		return "整理最近创作和互动信息，输出长期稳定记忆、待确认记忆和不应沉淀的短期噪音。"
	case automation.TemplateReview:
		return "对选定内容做结构、连续性、设定一致性和语言问题检查，按严重程度输出建议。"
	case automation.TemplateContinueWriting:
		return "续写下一段或下一章。请自行读取大纲、章节组细纲、进度、角色状态、资料库和最近章节，确定目标章节路径并写入正文；完成后按需同步 progress.md 和 setting/character-states.md。"
	default:
		return "根据所选上下文完成用户自定义自动化任务。"
	}
}

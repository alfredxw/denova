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

type automationRunState struct {
	Run    automation.RunRecord
	TaskID string
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
	run := s.newRunRecord(task, trigger)
	conversation := &automationConversation{}
	return s.runAutomation(ctx, task, run, conversation, nil)
}

func (a *App) StartAutomationTask(ctx context.Context, id, trigger string) (*Task, automation.RunRecord, error) {
	return a.automation().StartTask(ctx, id, trigger)
}

func (s *AutomationAppService) StartTask(ctx context.Context, id, trigger string) (*Task, automation.RunRecord, error) {
	taskDef, err := s.store().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if active, run, ok := s.activeTaskForAutomation(taskDef.ID); ok {
		log.Printf("[automation] attach active run task_id=%s run_id=%s status=%s", taskDef.ID, run.ID, active.Status())
		return active, run, nil
	}

	run := s.newRunRecord(taskDef, trigger)
	conversation, err := s.newRunConversation(run, taskDef)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}

	task := NewTask(func(taskCtx context.Context, task *Task, emit func(agent.Event)) {
		emit(agent.Event{Type: "automation_run", Data: run})
		result, _ := s.runAutomation(taskCtx, taskDef, run, conversation, emit)
		if result.Run.ID != "" {
			emit(agent.Event{Type: "automation_run", Data: result.Run})
		}
		s.clearActiveAutomationTask(taskDef.ID, run.ID)
	})
	s.setActiveAutomationTask(taskDef.ID, run.ID, task, run)
	return task, run, nil
}

func (a *App) ContinueAutomationRun(ctx context.Context, runID, message string) (*Task, automation.RunRecord, error) {
	return a.automation().ContinueRun(ctx, runID, message)
}

func (s *AutomationAppService) ContinueRun(ctx context.Context, runID, message string) (*Task, automation.RunRecord, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, automation.RunRecord{}, fmt.Errorf("message is required")
	}
	if active, run, ok := s.ActiveAutomationTaskByRunID(runID); ok {
		log.Printf("[automation] attach active follow-up run_id=%s status=%s", runID, active.Status())
		return active, run, nil
	}
	run, err := s.automationRunByID(runID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, automation.RunRecord{}, fmt.Errorf("automation run %s has no session history", runID)
	}
	taskDef, err := s.store().Get(run.TaskID)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	conversation, err := s.newRunConversation(run, taskDef)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	activeRun := run
	activeRun.Status = automation.RunStatusRunning
	activeRun.Error = ""
	task := NewTask(func(taskCtx context.Context, task *Task, emit func(agent.Event)) {
		emit(agent.Event{Type: "automation_run", Data: activeRun})
		s.runAutomationFollowUp(taskCtx, taskDef, activeRun, conversation, message, emit)
		finalRun := run
		if taskCtx.Err() != nil {
			finalRun.Status = automation.RunStatusAborted
			finalRun.Error = taskCtx.Err().Error()
		}
		emit(agent.Event{Type: "automation_run", Data: finalRun})
		s.clearActiveAutomationTask(taskDef.ID, run.ID)
	})
	s.setActiveAutomationTask(taskDef.ID, run.ID, task, activeRun)
	return task, activeRun, nil
}

func (s *AutomationAppService) ActiveAutomationRuns() []automation.ActiveRun {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	result := make([]automation.ActiveRun, 0, len(s.app.activeAutomationRuns))
	for _, state := range s.app.activeAutomationRuns {
		task := s.app.activeAutomationTasks[state.TaskID]
		if task == nil || task.Status() != TaskRunning {
			continue
		}
		result = append(result, automation.ActiveRun{Run: state.Run, TaskID: state.TaskID})
	}
	return result
}

func (a *App) ActiveAutomationRuns() []automation.ActiveRun {
	return a.automation().ActiveAutomationRuns()
}

func (s *AutomationAppService) ActiveAutomationTaskByRunID(runID string) (*Task, automation.RunRecord, bool) {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.activeAutomationRuns == nil {
		return nil, automation.RunRecord{}, false
	}
	state, ok := s.app.activeAutomationRuns[runID]
	if !ok {
		return nil, automation.RunRecord{}, false
	}
	task := s.app.activeAutomationTasks[state.TaskID]
	if task == nil || task.Status() != TaskRunning {
		return nil, automation.RunRecord{}, false
	}
	return task, state.Run, true
}

func (a *App) ActiveAutomationTaskByRunID(runID string) (*Task, automation.RunRecord, bool) {
	return a.automation().ActiveAutomationTaskByRunID(runID)
}

func (s *AutomationAppService) AbortAutomationRun(runID string) bool {
	task, _, ok := s.ActiveAutomationTaskByRunID(runID)
	if !ok {
		return false
	}
	task.Abort()
	return true
}

func (a *App) AbortAutomationRun(runID string) bool {
	return a.automation().AbortAutomationRun(runID)
}

func (s *AutomationAppService) AutomationRunMessages(runID string) ([]session.HistoryEntry, error) {
	run, err := s.automationRunByID(runID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, fmt.Errorf("automation run %s has no session history", runID)
	}
	s.app.mu.RLock()
	store := s.app.sessionStore
	s.app.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := store.Get(run.SessionID)
	if err != nil {
		return nil, err
	}
	return sess.History(), nil
}

func (a *App) AutomationRunMessages(sessionID string) ([]session.HistoryEntry, error) {
	return a.automation().AutomationRunMessages(sessionID)
}

func (s *AutomationAppService) automationRunByID(runID string) (automation.RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return automation.RunRecord{}, fmt.Errorf("run_id is required")
	}
	if _, run, ok := s.ActiveAutomationTaskByRunID(runID); ok {
		return run, nil
	}
	tasks, err := s.List()
	if err != nil {
		return automation.RunRecord{}, err
	}
	for _, task := range tasks {
		for _, run := range task.RecentRuns {
			if run.ID == runID {
				return run, nil
			}
		}
	}
	return automation.RunRecord{}, fmt.Errorf("automation run %s not found", runID)
}

func (s *AutomationAppService) runAutomation(ctx context.Context, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, emit func(agent.Event)) (result automation.RunResult, err error) {
	errorForwarded := false
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
			if emit != nil && !errorForwarded {
				emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
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
	var runError string
	forward := func(ev agent.Event) {
		switch ev.Type {
		case "error":
			runError = eventMessage(ev.Data)
			errorForwarded = true
		case "tool_call":
			log.Printf("[automation] tool call task_id=%s data=%v", task.ID, ev.Data)
		case "tool_result":
			log.Printf("[automation] tool result task_id=%s data=%v", task.ID, ev.Data)
		}
		if emit != nil {
			emit(ev)
		}
	}
	s.app.ChatService().RunWithOptions(ctx, runner, conversation, s.app.BookService(), agent.ChatRequest{
		Message: buildAutomationUserMessage(task, run),
	}, agent.RunOptions{
		AgentKind: agent.AgentKindAutomation,
		TaskID:    run.ID,
		SessionID: run.SessionID,
		Workspace: run.Workspace,
		Mode:      "automation",
	}, forward)
	if ctx.Err() != nil {
		output := conversation.Output()
		run.Summary = strings.TrimSpace(output)
		run.Status = automation.RunStatusAborted
		run.Error = ctx.Err().Error()
		run.FinishedAt = time.Now().UTC()
		updated, appendErr := s.store().AppendRun(task.ID, run)
		if appendErr != nil {
			return automation.RunResult{}, appendErr
		}
		log.Printf("[automation] run aborted task_id=%s scope=%s workspace=%q trigger=%s", task.ID, task.Scope, run.Workspace, run.Trigger)
		return automation.RunResult{Task: updated, Run: run}, nil
	}
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

func (s *AutomationAppService) runAutomationFollowUp(ctx context.Context, task automation.Task, run automation.RunRecord, conversation automationOutputConversation, message string, emit func(agent.Event)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[automation] follow-up panic recovered task_id=%s run_id=%s err=%v", task.ID, run.ID, recovered)
			emit(agent.Event{Type: "error", Data: map[string]string{"message": fmt.Sprintf("automation follow-up panic recovered: %v", recovered)}})
		}
	}()
	log.Printf("[automation] follow-up begin task_id=%s run_id=%s message_len=%d", task.ID, run.ID, len(message))
	runtimeCfg := s.runtimeConfig()
	runtimeCfg = constrainAutomationTools(runtimeCfg, task.WritePolicy)
	taskInstruction := agent.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WritePolicy:  task.WritePolicy,
		OutputPolicy: task.OutputPolicy,
		OutputPath:   task.OutputPath,
		Workspace:    run.Workspace,
	}
	runner, err := buildAutomationAgentRunner(ctx, &runtimeCfg, s.bookState(), taskInstruction)
	if err != nil {
		emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	s.app.ChatService().RunWithOptions(ctx, runner, conversation, s.app.BookService(), agent.ChatRequest{
		Message: message,
	}, agent.RunOptions{
		AgentKind: agent.AgentKindAutomation,
		TaskID:    run.ID,
		SessionID: run.SessionID,
		Workspace: run.Workspace,
		Mode:      "automation",
	}, emit)
	log.Printf("[automation] follow-up end task_id=%s run_id=%s", task.ID, run.ID)
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
		_, run, err := s.StartTask(ctx, task.ID, automation.TriggerSchedule)
		if err != nil {
			log.Printf("[automation] due task failed task_id=%s scope=%s workspace=%q err=%v", task.ID, task.Scope, s.workspace(), err)
			continue
		}
		results = append(results, automation.RunResult{Task: task, Run: run})
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

func (s *AutomationAppService) newRunRecord(task automation.Task, trigger string) automation.RunRecord {
	run := automation.RunRecord{
		ID:        automation.NewRunID(),
		TaskID:    task.ID,
		Scope:     task.Scope,
		Workspace: s.workspace(),
		Trigger:   normalizeAutomationTrigger(trigger),
		Status:    automation.RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	run.SessionID = automationRunSessionID(run.ID)
	return run
}

func (s *AutomationAppService) newRunConversation(run automation.RunRecord, task automation.Task) (*automationRunConversation, error) {
	s.app.mu.RLock()
	store := s.app.sessionStore
	s.app.mu.RUnlock()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := store.GetOrCreate(run.SessionID)
	if err != nil {
		return nil, err
	}
	title := fmt.Sprintf("%s · %s · %s", strings.TrimSpace(task.Name), run.Trigger, run.StartedAt.Local().Format("2006-01-02 15:04"))
	if strings.TrimSpace(task.Name) == "" {
		title = fmt.Sprintf("Automation · %s · %s", run.Trigger, run.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	if err := sess.Rename(title); err != nil {
		return nil, err
	}
	return &automationRunConversation{base: agent.NewSessionConversation(sess)}, nil
}

func (s *AutomationAppService) activeTaskForAutomation(taskID string) (*Task, automation.RunRecord, bool) {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.activeAutomationTasks == nil {
		return nil, automation.RunRecord{}, false
	}
	task := s.app.activeAutomationTasks[taskID]
	if task == nil || task.Status() != TaskRunning {
		return nil, automation.RunRecord{}, false
	}
	for _, state := range s.app.activeAutomationRuns {
		if state.TaskID == taskID {
			return task, state.Run, true
		}
	}
	return nil, automation.RunRecord{}, false
}

func (s *AutomationAppService) setActiveAutomationTask(taskID, runID string, task *Task, run automation.RunRecord) {
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	if s.app.activeAutomationTasks == nil {
		s.app.activeAutomationTasks = make(map[string]*Task)
	}
	if s.app.activeAutomationRuns == nil {
		s.app.activeAutomationRuns = make(map[string]automationRunState)
	}
	s.app.activeAutomationTasks[taskID] = task
	s.app.activeAutomationRuns[runID] = automationRunState{Run: run, TaskID: taskID}
}

func (s *AutomationAppService) clearActiveAutomationTask(taskID, runID string) {
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	if s.app.activeAutomationTasks != nil {
		delete(s.app.activeAutomationTasks, taskID)
	}
	if s.app.activeAutomationRuns != nil {
		delete(s.app.activeAutomationRuns, runID)
	}
}

func automationRunSessionID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = automation.NewRunID()
	}
	return "automation-run-" + runID
}

type automationOutputConversation interface {
	agent.Conversation
	Output() string
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

type automationRunConversation struct {
	base   *agent.SessionConversation
	output string
}

func (c *automationRunConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	return c.base.PrepareMessages(originalMessage, agentMessage)
}

func (c *automationRunConversation) AppendAssistant(content string) error {
	c.output = content
	return c.base.AppendAssistant(content)
}

func (c *automationRunConversation) AppendAssistantWithThinking(content, _ string) error {
	c.output = content
	return c.base.AppendAssistant(content)
}

func (c *automationRunConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	return c.base.AppendDisplayEvent(event)
}

func (c *automationRunConversation) UpdateDisplayToolStatus(id, name, status string) error {
	return c.base.UpdateDisplayToolStatus(id, name, status)
}

func (c *automationRunConversation) AppendDisplayToolArgs(id, name, delta string) error {
	return c.base.AppendDisplayToolArgs(id, name, delta)
}

func (c *automationRunConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	return c.base.UpdateDisplayToolResult(id, name, status, result)
}

func (c *automationRunConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	return c.base.MarkInterrupted(userMessage, assistantContent, reason)
}

func (c *automationRunConversation) PendingInterruption() *session.Interruption {
	return c.base.PendingInterruption()
}

func (c *automationRunConversation) ResolveInterruption(id string) error {
	return c.base.ResolveInterruption(id)
}

func (c *automationRunConversation) Output() string {
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
		WebSearch:    boolPointer(resolved.WebSearch),
	}
	return cfg
}

func automationToolManifest(cfg *config.Config) []automation.ToolManifestItem {
	tools := config.ResolveAgentTools(cfg, config.AgentKindAutomation)
	capabilities := config.ResolveAgentToolManifest(tools)
	result := make([]automation.ToolManifestItem, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, automation.ToolManifestItem{Source: capability.Source, Allowed: capability.Allowed})
	}
	return result
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

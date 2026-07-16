package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/automation"
	"denova/internal/book"
	"denova/internal/session"
)

type AutomationAppService struct {
	app      *App
	snapshot *automationWorkspaceSnapshot
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
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return automation.RunResult{}, ErrNoWorkspace
		}
		return scoped.Run(ctx, id, trigger)
	}
	task, err := s.store().Get(id)
	if err != nil {
		return automation.RunResult{}, err
	}
	run := s.newRunRecord(task, trigger)
	conversation := &automationConversation{}
	return s.runAutomation(ctx, task, run, conversation, nil)
}

func (a *App) StartAutomationTaskWithEvidence(ctx context.Context, id, trigger string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	return a.automation().StartTaskWithEvidence(ctx, id, trigger, evidence)
}

func (s *AutomationAppService) StartTaskWithEvidence(ctx context.Context, id, trigger string, evidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return nil, automation.RunRecord{}, ErrNoWorkspace
		}
		return scoped.StartTaskWithEvidence(ctx, id, trigger, evidence)
	}
	return s.startTaskWithSourceRun(ctx, id, trigger, "", evidence)
}

func (s *AutomationAppService) startTaskWithSourceRun(ctx context.Context, id, trigger, sourceRunID string, triggerEvidence []automation.TriggerEvidence) (*Task, automation.RunRecord, error) {
	taskDef, err := s.store().Get(id)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}

	run := s.newRunRecord(taskDef, trigger)
	sourceRunID = strings.TrimSpace(sourceRunID)
	if trigger == automation.TriggerWriteConfirmation && sourceRunID != "" {
		sourceRun, sourceErr := s.automationRunByID(sourceRunID)
		if sourceErr != nil {
			return nil, automation.RunRecord{}, sourceErr
		}
		run = sourceRun
		run.Trigger = automation.TriggerWriteConfirmation
		run.Status = automation.RunStatusRunning
		run.Error = ""
		run.FinishedAt = time.Time{}
		run.SourceRunID = sourceRunID
	} else {
		run.SourceRunID = sourceRunID
	}
	run.TriggerEvidence = boundedRunTriggerEvidence(triggerEvidence)
	claim, owner, err := s.reserveActiveAutomationRun(ctx, taskDef.ID, run)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if !owner {
		log.Printf("[automation] attach active run workspace=%q task_id=%s run_id=%s status=%s", claim.workspace, taskDef.ID, claim.run.ID, claim.task.Status())
		return claim.task, claim.run, nil
	}
	claimActivated := false
	defer func() {
		if !claimActivated {
			s.releaseAutomationClaim(claim)
		}
	}()
	conversation, err := s.newRunConversation(run, taskDef)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}

	start := make(chan struct{})
	task := NewTask(func(taskCtx context.Context, task *Task, emit func(agent.Event)) {
		select {
		case <-start:
		case <-taskCtx.Done():
			return
		}
		defer s.clearActiveAutomationTask(taskDef.ID, run.ID)
		emit(agent.Event{Type: "automation_run", Data: run})
		result, _ := s.runAutomation(taskCtx, taskDef, run, conversation, emit)
		if result.Run.ID != "" {
			emit(agent.Event{Type: "automation_run", Data: result.Run})
		}
	})
	if !s.activateAutomationClaim(claim, task) {
		task.Abort()
		close(start)
		return nil, automation.RunRecord{}, fmt.Errorf("automation run claim was released before activation")
	}
	claimActivated = true
	close(start)
	return task, run, nil
}

func (a *App) ContinueAutomationRun(ctx context.Context, runID, message string) (*Task, automation.RunRecord, error) {
	return a.automation().ContinueRun(ctx, runID, message)
}

func (s *AutomationAppService) ContinueRun(ctx context.Context, runID, message string) (*Task, automation.RunRecord, error) {
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return nil, automation.RunRecord{}, ErrNoWorkspace
		}
		return scoped.ContinueRun(ctx, runID, message)
	}
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
	activeRun := run
	activeRun.Status = automation.RunStatusRunning
	activeRun.Error = ""
	claim, owner, err := s.reserveActiveAutomationRun(ctx, taskDef.ID, activeRun)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	if !owner {
		return claim.task, claim.run, nil
	}
	claimActivated := false
	defer func() {
		if !claimActivated {
			s.releaseAutomationClaim(claim)
		}
	}()
	conversation, err := s.newRunConversation(run, taskDef)
	if err != nil {
		return nil, automation.RunRecord{}, err
	}
	start := make(chan struct{})
	task := NewTask(func(taskCtx context.Context, task *Task, emit func(agent.Event)) {
		select {
		case <-start:
		case <-taskCtx.Done():
			return
		}
		defer s.clearActiveAutomationTask(taskDef.ID, run.ID)
		emit(agent.Event{Type: "automation_run", Data: activeRun})
		s.runAutomationFollowUp(taskCtx, taskDef, activeRun, conversation, message, emit)
		finalRun := run
		if taskCtx.Err() != nil {
			finalRun.Status = automation.RunStatusAborted
			finalRun.Error = taskCtx.Err().Error()
		}
		emit(agent.Event{Type: "automation_run", Data: finalRun})
	})
	if !s.activateAutomationClaim(claim, task) {
		task.Abort()
		close(start)
		return nil, automation.RunRecord{}, fmt.Errorf("automation run claim was released before activation")
	}
	claimActivated = true
	close(start)
	return task, activeRun, nil
}

func (s *AutomationAppService) AutomationRunMessages(runID string) ([]session.HistoryEntry, error) {
	run, err := s.automationRunByID(runID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, fmt.Errorf("automation run %s has no session history", runID)
	}
	store := s.sessionStore()
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
			if run.ID == runID && canonicalAutomationWorkspace(run.Workspace) == canonicalAutomationWorkspace(s.workspace()) {
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
	runtimeCfg := s.runtimeConfigForTask(task)
	writeMode, writeScope := effectiveAutomationWriteModeScope(task, run)
	runtimeCfg = constrainAutomationTools(runtimeCfg, writeMode, writeScope)
	run.ToolManifest = automationToolManifest(&runtimeCfg)
	taskInstruction := agent.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WriteMode:    writeMode,
		WriteScope:   writeScope,
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
	chatService := s.chatService()
	bookService := s.bookService()
	if chatService == nil || bookService == nil {
		return automation.RunResult{}, ErrNoWorkspace
	}
	chatService.RunWithOptions(ctx, runner, conversation, bookService, agent.ChatRequest{
		Message: s.buildAutomationUserMessage(task, run, writeMode, writeScope),
	}, agent.RunOptions{
		AgentKind:           agent.AgentKindAutomation,
		TaskID:              run.ID,
		SessionID:           run.SessionID,
		Workspace:           run.Workspace,
		Mode:                "automation",
		IdleTimeout:         agentIdleTimeout(runtimeCfg),
		ToolResultMaxBytes:  agentToolResultMaxBytes(runtimeCfg),
		OnMutationsVerified: s.automationMutationCallback("automation_agent_post_run"),
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
	if path, writeErr := s.writeOptionalOutput(task, output, runtimeCfg, writeMode, writeScope); writeErr != nil {
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
	if inboxErr := s.createWriteConfirmationInboxIfNeeded(updated, run, output); inboxErr != nil {
		log.Printf("[automation] create write confirmation inbox failed task_id=%s run_id=%s err=%v", task.ID, run.ID, inboxErr)
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
	runtimeCfg := s.runtimeConfigForTask(task)
	writeMode, writeScope := effectiveAutomationWriteModeScope(task, run)
	runtimeCfg = constrainAutomationTools(runtimeCfg, writeMode, writeScope)
	taskInstruction := agent.AutomationTaskInstruction{
		Name:         task.Name,
		Template:     task.Template,
		Prompt:       task.Prompt,
		WriteMode:    writeMode,
		WriteScope:   writeScope,
		OutputPolicy: task.OutputPolicy,
		OutputPath:   task.OutputPath,
		Workspace:    run.Workspace,
	}
	runner, err := buildAutomationAgentRunner(ctx, &runtimeCfg, s.bookState(), taskInstruction)
	if err != nil {
		emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	chatService := s.chatService()
	bookService := s.bookService()
	if chatService == nil || bookService == nil {
		emit(agent.Event{Type: "error", Data: map[string]string{"message": ErrNoWorkspace.Error()}})
		return
	}
	chatService.RunWithOptions(ctx, runner, conversation, bookService, agent.ChatRequest{
		Message: message,
	}, agent.RunOptions{
		AgentKind:           agent.AgentKindAutomation,
		TaskID:              run.ID,
		SessionID:           run.SessionID,
		Workspace:           run.Workspace,
		Mode:                "automation",
		IdleTimeout:         agentIdleTimeout(runtimeCfg),
		ToolResultMaxBytes:  agentToolResultMaxBytes(runtimeCfg),
		OnMutationsVerified: s.automationMutationCallback("automation_agent_post_run"),
	}, emit)
	log.Printf("[automation] follow-up end task_id=%s run_id=%s", task.ID, run.ID)
}

func (a *App) RunDueAutomations(ctx context.Context, now time.Time) []automation.RunResult {
	return a.automation().RunDue(ctx, now)
}

func (s *AutomationAppService) RunDue(ctx context.Context, now time.Time) []automation.RunResult {
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return nil
		}
		return scoped.RunDue(ctx, now)
	}
	_, results, err := s.processTriggers(ctx, "", now.UTC(), "scheduler")
	if err != nil {
		log.Printf("[automation] process due triggers failed err=%v", err)
		return nil
	}
	return results
}

func (s *AutomationAppService) store() *automation.Store {
	if s.snapshot != nil {
		return automation.NewStore(s.snapshot.novaDir, s.snapshot.workspace)
	}
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
	if s.snapshot != nil {
		return s.snapshot.workspace
	}
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspace
}

func (s *AutomationAppService) bookState() *book.State {
	if s.snapshot != nil {
		return s.snapshot.bookState
	}
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
	store := s.sessionStore()
	cfg := s.configSnapshot()
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
	return &automationRunConversation{base: agent.NewSessionConversationForAgent(sess, &cfg, config.AgentKindAutomation)}, nil
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

func (c *automationConversation) AppendAssistantWithMetadata(content, _ string, _ session.MessageMetadata) error {
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

func (c *automationRunConversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	c.output = content
	return c.base.AppendAssistantWithMetadata(content, thinking, metadata)
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
	if s.snapshot != nil {
		return s.snapshot.cfg
	}
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
	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[automation] load layered settings failed workspace=%s err=%v", workspace, err)
	}
	return runtimeCfg
}

func (s *AutomationAppService) runtimeConfigForTask(task automation.Task) config.Config {
	runtimeCfg := s.runtimeConfig()
	if profileID := strings.TrimSpace(task.ModelProfileID); profileID != "" {
		runtimeCfg.AgentModels.Automation.ProfileID = profileID
	}
	return runtimeCfg
}

func (s *AutomationAppService) createWriteConfirmationInboxIfNeeded(task automation.Task, run automation.RunRecord, output string) error {
	if task.WriteMode != automation.WriteModeConfirmWrite || run.Trigger == automation.TriggerWriteConfirmation {
		return nil
	}
	if strings.TrimSpace(task.WriteScope) == "" || task.WriteScope == automation.WriteScopeNone {
		return nil
	}
	store := s.store()
	fingerprint := automation.EvidenceFingerprint(task.ID, automation.InboxPurposeWriteConfirmation, run.ID)
	if existing, ok, err := store.FindOpenInboxItem(task.ID, automation.InboxPurposeWriteConfirmation, fingerprint); err != nil {
		return err
	} else if ok {
		log.Printf("[automation] write confirmation inbox already open task_id=%s run_id=%s inbox_id=%s", task.ID, run.ID, existing.ID)
		return nil
	}
	_, err := store.CreateInboxItem(automation.TriggerInboxItem{
		TaskID:       task.ID,
		TriggerID:    automation.InboxPurposeWriteConfirmation,
		Purpose:      automation.InboxPurposeWriteConfirmation,
		Scope:        task.Scope,
		Workspace:    run.Workspace,
		Status:       automation.InboxStatusPending,
		ActionPolicy: automation.ActionPolicyConfirm,
		NotifyPolicy: automation.NotifyPolicyInbox,
		Title:        fmt.Sprintf("写入确认：%s", task.Name),
		Summary:      trimForTriggerSnippet(output, 1400),
		Evidence: []automation.TriggerEvidence{{
			Source:  "automation_run",
			Title:   run.ID,
			Ref:     run.ID,
			Snippet: trimForTriggerSnippet(output, 900),
		}},
		Fingerprint: fingerprint,
		SourceRunID: run.ID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	return err
}

func (s *AutomationAppService) writeOptionalOutput(task automation.Task, output string, cfg config.Config, writeMode, writeScope string) (string, error) {
	if task.OutputPolicy != automation.OutputPolicyOptionalFile || strings.TrimSpace(task.OutputPath) == "" {
		return "", nil
	}
	if !automationTaskAllowsFileWrite(writeMode, writeScope) {
		return "", fmt.Errorf("task write mode/scope does not allow file output")
	}
	if !config.ResolveAgentTools(&cfg, config.AgentKindAutomation).FileWrite {
		return "", fmt.Errorf("Automation Agent file_write tool is disabled")
	}
	bookService := s.bookService()
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
	switch trigger {
	case automation.TriggerSchedule, automation.TriggerCondition, automation.TriggerInboxConfirmation, automation.TriggerWriteConfirmation:
		return trigger
	default:
		return automation.TriggerManual
	}
}

func effectiveAutomationWriteModeScope(task automation.Task, run automation.RunRecord) (string, string) {
	mode := strings.TrimSpace(task.WriteMode)
	if mode == "" {
		mode = automation.WriteModeReadOnly
	}
	scope := strings.TrimSpace(task.WriteScope)
	if mode == automation.WriteModeReadOnly {
		return automation.WriteModeReadOnly, automation.WriteScopeNone
	}
	if mode == automation.WriteModeConfirmWrite && run.Trigger != automation.TriggerWriteConfirmation {
		return automation.WriteModeReadOnly, automation.WriteScopeNone
	}
	if scope == "" || scope == automation.WriteScopeNone {
		scope = automation.WriteScopeFile
	}
	return automation.WriteModeAutoWrite, scope
}

func automationTaskAllowsFileWrite(writeMode, writeScope string) bool {
	if writeMode != automation.WriteModeAutoWrite {
		return false
	}
	return writeScope == automation.WriteScopeFile || writeScope == automation.WriteScopeLoreAndFile
}

func automationTaskAllowsLoreWrite(writeMode, writeScope string) bool {
	if writeMode != automation.WriteModeAutoWrite {
		return false
	}
	return writeScope == automation.WriteScopeLore || writeScope == automation.WriteScopeLoreAndFile
}

func constrainAutomationTools(cfg config.Config, writeMode, writeScope string) config.Config {
	resolved := config.ResolveAgentTools(&cfg, config.AgentKindAutomation)
	cfg.AgentTools.Automation = config.AgentToolOverride{
		FileRead:     boolPointer(resolved.FileRead),
		FileWrite:    boolPointer(resolved.FileWrite && automationTaskAllowsFileWrite(writeMode, writeScope)),
		ShellExecute: boolPointer(resolved.ShellExecute),
		Skills:       boolPointer(resolved.Skills),
		LoreRead:     boolPointer(resolved.LoreRead),
		LoreWrite:    boolPointer(resolved.LoreWrite && automationTaskAllowsLoreWrite(writeMode, writeScope)),
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

func (s *AutomationAppService) buildAutomationUserMessage(task automation.Task, run automation.RunRecord, writeMode, writeScope string) string {
	var sb strings.Builder
	sb.WriteString("执行 Denova 自动化任务。\n\n")
	sb.WriteString(fmt.Sprintf("任务名称：%s\n", task.Name))
	sb.WriteString(fmt.Sprintf("触发来源：%s\n", run.Trigger))
	sb.WriteString(fmt.Sprintf("执行模式：%s\n", writeMode))
	sb.WriteString(fmt.Sprintf("写入范围：%s\n", writeScope))
	sb.WriteString(fmt.Sprintf("输出策略：%s\n", task.OutputPolicy))
	if task.OutputPath != "" {
		sb.WriteString(fmt.Sprintf("输出文件：%s\n", task.OutputPath))
	}
	if len(run.TriggerEvidence) > 0 {
		sb.WriteString("\n本次触发范围（有界证据，优先处理这些新增内容）：\n")
		for _, item := range run.TriggerEvidence {
			sb.WriteString(formatTriggerEvidenceLine(item))
		}
	}
	if run.Trigger == automation.TriggerWriteConfirmation {
		sb.WriteString("\n写入确认：用户已经确认执行上一轮只读方案。请只在写入范围内落实方案，不要扩大修改范围。\n")
		if sourceRunID := strings.TrimSpace(run.SourceRunID); sourceRunID != "" {
			if sourceRun, err := s.automationRunByID(sourceRunID); err == nil && strings.TrimSpace(sourceRun.Summary) != "" {
				sb.WriteString("已确认方案摘要：\n")
				sb.WriteString(trimForTriggerSnippet(sourceRun.Summary, 2500))
				sb.WriteString("\n")
			} else if err != nil {
				log.Printf("[automation] load source run summary failed source_run_id=%s err=%v", sourceRunID, err)
			}
		}
	} else if task.WriteMode == automation.WriteModeConfirmWrite {
		sb.WriteString("\n写入确认模式：本轮强制只读。请输出具体写入方案/修订建议，包括建议修改的路径、资料库项和原因；不要实际写入。用户确认后会启动第二个写入 run。\n")
	}
	sb.WriteString("\n用户 Prompt：\n")
	if task.Prompt != "" {
		sb.WriteString(task.Prompt)
	} else {
		sb.WriteString(automation.GenericTaskPrompt)
	}
	sb.WriteString("\n\n请你自行使用可用工具读取完成任务所需的工作区文件、资料库和状态；先定位范围，再读取和写入。")
	return sb.String()
}

func boundedRunTriggerEvidence(evidence []automation.TriggerEvidence) []automation.TriggerEvidence {
	const maxItems = 12
	if len(evidence) == 0 {
		return nil
	}
	limit := len(evidence)
	if limit > maxItems {
		limit = maxItems
	}
	out := make([]automation.TriggerEvidence, 0, limit)
	for i := 0; i < limit; i++ {
		item := evidence[i]
		item.Source = trimForTriggerSnippet(strings.TrimSpace(item.Source), 80)
		item.Title = trimForTriggerSnippet(strings.TrimSpace(item.Title), 160)
		item.Ref = trimForTriggerSnippet(strings.TrimSpace(item.Ref), 240)
		item.Snippet = trimForTriggerSnippet(strings.TrimSpace(item.Snippet), 600)
		out = append(out, item)
	}
	return out
}

func formatTriggerEvidenceLine(item automation.TriggerEvidence) string {
	source := strings.TrimSpace(item.Source)
	if source == "" {
		source = "unknown"
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "(untitled)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- [%s] %s", source, title))
	if ref := strings.TrimSpace(item.Ref); ref != "" {
		sb.WriteString(fmt.Sprintf(" — %s", ref))
	}
	sb.WriteString("\n")
	if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", snippet))
	}
	return sb.String()
}

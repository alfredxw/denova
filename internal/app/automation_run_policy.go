package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
	"denova/internal/automation"
)

// runtimeConfigForTask returns the runtime config from a snapshot, applying
// task-specific model profile overrides.
func runtimeConfigForTask(snap *automationWorkspaceSnapshot, task automation.Task) config.Config {
	runtimeCfg := snap.cfg
	if profileID := strings.TrimSpace(task.ModelProfileID); profileID != "" {
		runtimeCfg.AgentModels.Automation.ProfileID = profileID
	}
	return runtimeCfg
}

func (s *AutomationAppService) createWriteConfirmationInboxIfNeeded(snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord, output string) error {
	if task.WriteMode != automation.WriteModeConfirmWrite || run.Trigger == automation.TriggerWriteConfirmation {
		return nil
	}
	if strings.TrimSpace(task.WriteScope) == "" || task.WriteScope == automation.WriteScopeNone {
		return nil
	}
	store := storeForSnapshot(snap)
	fingerprint := automation.EvidenceFingerprint(task.ID, automation.InboxPurposeWriteConfirmation, run.ID)
	inboxID := "write-confirmation-" + fingerprint
	item, created, err := store.EnsureInboxItem(context.Background(), automation.TriggerInboxItem{
		ID:           inboxID,
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
	if err == nil && !created {
		slog.InfoContext(context.Background(), fmt.Sprintf("[automation] write confirmation inbox replayed task_id=%s run_id=%s inbox_id=%s", task.ID, run.ID, item.ID))
	}
	return err
}

func (s *AutomationAppService) writeOptionalOutput(snap *automationWorkspaceSnapshot, task automation.Task, output string, cfg config.Config, writeMode, writeScope string) (string, error) {
	if task.OutputPolicy != automation.OutputPolicyOptionalFile || strings.TrimSpace(task.OutputPath) == "" {
		return "", nil
	}
	if writeMode == automation.WriteModeReadOnly {
		return "", nil
	}
	if !automationTaskAllowsFileWrite(writeMode, writeScope) {
		return "", fmt.Errorf("task write mode/scope does not allow file output")
	}
	if !config.ResolveAgentTools(&cfg, config.AgentKindAutomation).Allows(config.AgentToolWorkspaceWrite) {
		return "", fmt.Errorf("Automation Agent workspace_write capability is disabled")
	}
	bookService := snap.bookService
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
	override := make(config.AgentToolOverride, len(config.AgentToolCapabilities()))
	for _, capability := range config.AgentToolCapabilities() {
		override[capability.Source] = resolved.Allows(capability.Source)
	}
	override[config.AgentToolWorkspaceWrite] = resolved.Allows(config.AgentToolWorkspaceWrite) && automationTaskAllowsFileWrite(writeMode, writeScope)
	override[config.AgentToolLoreWrite] = resolved.Allows(config.AgentToolLoreWrite) && automationTaskAllowsLoreWrite(writeMode, writeScope)
	cfg.AgentTools.Automation = override
	return cfg
}

func constrainGlobalAutomationTools(cfg config.Config) config.Config {
	resolved := config.ResolveAgentTools(&cfg, config.AgentKindAutomation)
	override := make(config.AgentToolOverride, len(config.AgentToolCapabilities()))
	for _, capability := range config.AgentToolCapabilities() {
		override[capability.Source] = false
	}
	for _, capability := range []string{config.AgentToolSkills, config.AgentToolTodo, config.AgentToolWebSearch, config.AgentToolWebFetch} {
		override[capability] = resolved.Allows(capability)
	}
	cfg.AgentTools.Automation = override
	return cfg
}

func automationToolManifest(cfg *config.Config) []automation.ToolManifestItem {
	tools := config.ResolveAgentTools(cfg, config.AgentKindAutomation)
	capabilities := config.ResolveAgentToolManifest(tools)
	result := make([]automation.ToolManifestItem, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, automation.ToolManifestItem{Source: capability.Capability, Allowed: capability.Allowed})
	}
	return result
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
	var confirmedSummary string
	if run.Trigger == automation.TriggerWriteConfirmation {
		if sourceRunID := strings.TrimSpace(run.SourceRunID); sourceRunID != "" {
			if sourceRun, err := s.automationRunByID(nil, sourceRunID); err == nil {
				confirmedSummary = trimForTriggerSnippet(sourceRun.Summary, 2500)
			} else if err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[automation] load source run summary failed source_run_id=%s err=%v", sourceRunID, err))
			}
		}
	}
	return automation.BuildRunUserMessage(task, run, writeMode, writeScope, confirmedSummary)
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

package automationapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/automation"
	projectdomain "denova/internal/project"
)

// runtimeConfigForTask applies the optional model override for semantic
// trigger evaluation. AgentChat applies the same override for actual runs.
func runtimeConfigForTask(snap *automationWorkspaceSnapshot, task automation.Task) config.Config {
	runtimeCfg := snap.cfg
	if profileID := strings.TrimSpace(task.ModelProfileID); profileID != "" {
		switch projectAgentKind(snap) {
		case agentrun.AgentKindIDE:
			runtimeCfg.AgentModels.IDE.ProfileID = profileID
		case agentrun.AgentKindGeneral:
			runtimeCfg.AgentModels.General.ProfileID = profileID
		}
	}
	return runtimeCfg
}

func projectAgentKind(snap *automationWorkspaceSnapshot) string {
	if snap == nil {
		return ""
	}
	switch snap.projectType {
	case projectdomain.TypeBook:
		return agentrun.AgentKindIDE
	case projectdomain.TypeGeneral:
		return agentrun.AgentKindGeneral
	default:
		return ""
	}
}

// automationInvocationManifest captures the Project Agent's effective tools in
// the run ledger. Automation adds no capability layer: its Prompt uses exactly
// the tools enabled for the owning Project Agent.
func automationInvocationManifest(snap *automationWorkspaceSnapshot) ([]automation.ToolManifestItem, error) {
	agentKind := projectAgentKind(snap)
	if agentKind == "" || snap == nil || strings.TrimSpace(snap.projectID) == "" {
		return nil, fmt.Errorf("automation execution requires a target Project Agent")
	}
	projectTools := config.ResolveAgentTools(&snap.cfg, agentKind)
	manifest := config.ResolveAgentToolManifest(projectTools)
	result := make([]automation.ToolManifestItem, 0, len(manifest))
	for _, capability := range manifest {
		result = append(result, automation.ToolManifestItem{Source: capability.Capability, Allowed: capability.Allowed})
	}
	return result, nil
}

func normalizeAutomationTrigger(trigger string) string {
	switch trigger {
	case automation.TriggerSchedule, automation.TriggerCondition, automation.TriggerInboxConfirmation, automation.TriggerWriteConfirmation:
		return trigger
	default:
		return automation.TriggerManual
	}
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

func (s *Service) buildAutomationUserMessage(task automation.Task, run automation.RunRecord) string {
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
	return automation.BuildRunUserMessage(task, run, confirmedSummary)
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

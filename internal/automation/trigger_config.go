package automation

import (
	"fmt"
	"strings"
	"time"
)

func NormalizeInboxItem(item TriggerInboxItem) (TriggerInboxItem, error) {
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = newID("inbox")
	}
	item.TaskID = strings.TrimSpace(item.TaskID)
	if item.TaskID == "" {
		return TriggerInboxItem{}, fmt.Errorf("task_id is required")
	}
	item.TriggerID = strings.TrimSpace(item.TriggerID)
	if item.TriggerID == "" {
		return TriggerInboxItem{}, fmt.Errorf("trigger_id is required")
	}
	item.Scope = strings.TrimSpace(item.Scope)
	if item.Scope == "" {
		item.Scope = ScopeWorkspace
	}
	if item.Scope != ScopeUser && item.Scope != ScopeWorkspace {
		return TriggerInboxItem{}, fmt.Errorf("invalid scope %q", item.Scope)
	}
	item.Workspace = strings.TrimSpace(item.Workspace)
	item.Purpose = normalizeInboxPurpose(item.Purpose)
	item.SourceRunID = strings.TrimSpace(item.SourceRunID)
	item.RunID = strings.TrimSpace(item.RunID)
	item.Status = normalizeInboxStatus(item.Status)
	item.ActionPolicy = normalizeActionPolicy(item.ActionPolicy, ActionPolicyConfirm)
	item.NotifyPolicy = normalizeNotifyPolicy(item.NotifyPolicy, NotifyPolicyInbox)
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		item.Title = "Automation trigger"
	}
	item.Summary = strings.TrimSpace(item.Summary)
	item.Fingerprint = strings.TrimSpace(item.Fingerprint)
	if item.Fingerprint == "" {
		item.Fingerprint = item.TaskID + ":" + item.TriggerID + ":" + item.Title
	}
	if item.Evidence == nil {
		item.Evidence = []TriggerEvidence{}
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	return item, nil
}

func normalizeTriggers(triggers []TriggerDefinition, fallbackSchedule Schedule) []TriggerDefinition {
	out := make([]TriggerDefinition, 0, len(triggers))
	for i, trigger := range triggers {
		normalized := normalizeTriggerDefinition(trigger, i, fallbackSchedule)
		if normalized.Type == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func normalizeTriggerDefinition(trigger TriggerDefinition, index int, fallbackSchedule Schedule) TriggerDefinition {
	trigger.Type = strings.TrimSpace(trigger.Type)
	if trigger.Type == "" {
		trigger.Type = TriggerTypeManual
	}
	trigger = migrateLegacySemanticTrigger(trigger)
	if !validTriggerType(trigger.Type) {
		trigger.Type = TriggerTypeManual
	}
	trigger.ID = strings.TrimSpace(trigger.ID)
	if trigger.ID == "" {
		trigger.ID = stableTriggerID(trigger.Type, index)
	}
	trigger.Name = strings.TrimSpace(trigger.Name)
	trigger.SemanticCondition = strings.TrimSpace(trigger.SemanticCondition)
	if trigger.ChapterBatchSize < 1 {
		trigger.ChapterBatchSize = 5
	}
	trigger.ActionPolicy = ""
	trigger.NotifyPolicy = normalizeNotifyPolicy(trigger.NotifyPolicy, defaultNotifyPolicyForTrigger(trigger.Type))
	if trigger.Type == TriggerTypeSchedule {
		if trigger.Schedule.Kind == "" {
			trigger.Schedule = fallbackSchedule
		}
		if normalized, err := NormalizeSchedule(trigger.Schedule); err == nil {
			trigger.Schedule = normalized
		}
	}
	return trigger
}

func legacyScheduleTrigger(schedule Schedule) TriggerDefinition {
	return TriggerDefinition{
		ID:           TriggerTypeSchedule,
		Type:         TriggerTypeSchedule,
		Enabled:      schedule.Kind != "" && schedule.Kind != ScheduleManual,
		NotifyPolicy: NotifyPolicySilent,
		Schedule:     schedule,
	}
}

func firstScheduleTrigger(triggers []TriggerDefinition) (TriggerDefinition, bool) {
	for _, trigger := range triggers {
		if trigger.Type == TriggerTypeSchedule {
			return trigger, true
		}
	}
	return TriggerDefinition{}, false
}

func EffectiveActionPolicy(task Task, _ TriggerDefinition) string {
	mode, _ := normalizeWriteModeScope(task.WriteMode, task.WriteScope, task.WritePolicy)
	return actionPolicyForWriteMode(mode)
}

func EffectiveNotifyPolicy(task Task, trigger TriggerDefinition) string {
	if EffectiveActionPolicy(task, trigger) == ActionPolicyConfirm {
		return NotifyPolicyInbox
	}
	return normalizeNotifyPolicy(trigger.NotifyPolicy, defaultNotifyPolicyForTrigger(trigger.Type))
}

func normalizeActionPolicy(policy, fallback string) string {
	policy = strings.TrimSpace(policy)
	switch policy {
	case ActionPolicyConfirm, ActionPolicyAutoRun, ActionPolicyNotifyOnly:
		return policy
	default:
		if fallback == "" {
			return ActionPolicyConfirm
		}
		return fallback
	}
}

func actionPolicyForWriteMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case WriteModeReadOnly, WriteModeConfirmWrite, WriteModeAutoWrite:
		return ActionPolicyAutoRun
	default:
		return ActionPolicyAutoRun
	}
}

func normalizeNotifyPolicy(policy, fallback string) string {
	policy = strings.TrimSpace(policy)
	switch policy {
	case NotifyPolicyInbox, NotifyPolicySilent:
		return policy
	default:
		if fallback == "" {
			return NotifyPolicyInbox
		}
		return fallback
	}
}

func normalizeInboxStatus(status string) string {
	switch strings.TrimSpace(status) {
	case InboxStatusDismissed, InboxStatusConfirmed, InboxStatusAutoRun:
		return strings.TrimSpace(status)
	default:
		return InboxStatusPending
	}
}

func defaultNotifyPolicyForTrigger(triggerType string) string {
	if triggerType == TriggerTypeSchedule {
		return NotifyPolicySilent
	}
	return NotifyPolicyInbox
}

func stableTriggerID(triggerType string, index int) string {
	switch triggerType {
	case TriggerTypeSchedule:
		return TriggerTypeSchedule
	case TriggerTypeManual:
		return TriggerTypeManual
	case TriggerTypeChapterBatch:
		return fmt.Sprintf("%s_%d", TriggerTypeChapterBatch, index+1)
	default:
		return fmt.Sprintf("%s_%d", triggerType, index+1)
	}
}

func validTriggerType(triggerType string) bool {
	switch triggerType {
	case TriggerTypeManual, TriggerTypeSchedule, TriggerTypeSemantic, TriggerTypeChapterBatch:
		return true
	default:
		return false
	}
}

func normalizeInboxPurpose(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case InboxPurposeWriteConfirmation:
		return InboxPurposeWriteConfirmation
	default:
		return InboxPurposeTrigger
	}
}

func migrateLegacySemanticTrigger(trigger TriggerDefinition) TriggerDefinition {
	condition := strings.TrimSpace(trigger.SemanticCondition)
	switch trigger.Type {
	case "chapter_ready_for_review":
		trigger.Type = TriggerTypeSemantic
		if condition == "" {
			trigger.SemanticCondition = "章节剧情或正文达到需要质量检查、连续性检查或完成度检查的状态"
		}
	case "interactive_new_character":
		trigger.Type = TriggerTypeSemantic
		if condition == "" {
			trigger.SemanticCondition = "最近章节剧情中有新的重要角色登场"
		}
	case "interactive_character_state_changed":
		trigger.Type = TriggerTypeSemantic
		if condition == "" {
			trigger.SemanticCondition = "最近章节剧情中已有角色的处境、关系、能力、情绪或阵营发生重要变化"
		}
	}
	return trigger
}

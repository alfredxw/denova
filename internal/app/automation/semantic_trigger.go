package automationapp

import (
	"context"
	apptask "denova/internal/app/task"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"denova/config"
	agentmodeltask "denova/internal/agents/modeltask"
	"denova/internal/automation"
)

const semanticTriggerConfidenceThreshold = 0.55

var errDurableTriggerActionRetry = errors.New("durable trigger action must be retried")

type semanticTriggerEvaluationFunc func(context.Context, *config.Config, string) (string, error)

type durableTriggerRunStarter func(context.Context, string, string, string, []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error)

// processDurableSemanticTrigger owns one complete evaluation/action cycle. Its
// filesystem lease spans model evaluation and side-effect reconciliation, so a
// competing process either observes a completed receipt or resumes persisted
// state after the previous process exits.
func (s *Service) processDurableSemanticTrigger(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	listedTask automation.Task,
	listedTrigger automation.TriggerDefinition,
	stateKey string,
) (item automation.TriggerInboxItem, run automation.RunResult, processed bool, err error) {
	return s.processDurableSemanticTriggerWithStarter(
		ctx,
		snap,
		store,
		now,
		listedTask,
		listedTrigger,
		stateKey,
		func(ctx context.Context, taskID, trigger, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
			return s.startTaskWithSourceRunID(ctx, snap, taskID, trigger, "", runID, evidence)
		},
	)
}

func (s *Service) processDurableSemanticTriggerWithStarter(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	listedTask automation.Task,
	listedTrigger automation.TriggerDefinition,
	stateKey string,
	start durableTriggerRunStarter,
) (item automation.TriggerInboxItem, run automation.RunResult, processed bool, err error) {
	release, err := store.AcquireTriggerEvaluationLease(ctx, automationTaskStoreID(listedTask), stateKey)
	if err != nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}
	defer func() {
		if releaseErr := release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	task, err := store.Get(automationTaskStoreID(listedTask))
	if err != nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}
	trigger, ok := enabledSemanticTrigger(task, listedTrigger.ID)
	if !ok {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, nil
	}
	state := task.TriggerState[stateKey]
	record, ready, err := s.claimOrResumeSemanticTrigger(ctx, snap, store, now, task, trigger, stateKey, state)
	if err != nil || !ready {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}

	if record.Status == automation.TriggerEvaluationStatusClaimed {
		record, err = s.evaluateClaimedSemanticTrigger(ctx, snap, store, now, task, stateKey, record)
		if err != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
		}
	}
	if record.Status == automation.TriggerEvaluationStatusCompleted {
		return automation.TriggerInboxItem{}, automation.RunResult{}, true, nil
	}
	if record.Status != automation.TriggerEvaluationStatusDecided {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, fmt.Errorf("semantic evaluation %s has invalid status %q", record.ID, record.Status)
	}

	if record.Action != nil {
		item, run, processed, err = s.reconcileDurableTriggerAction(
			ctx, snap, store, task, record, start,
		)
		if err != nil {
			return item, run, processed, err
		}
	}
	if _, err = store.CompleteTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, record.ID, record.IntentHash, time.Now().UTC()); err != nil {
		return item, run, false, err
	}
	return item, run, true, nil
}

func (s *Service) claimOrResumeSemanticTrigger(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	task automation.Task,
	trigger automation.TriggerDefinition,
	stateKey string,
	state automation.TriggerState,
) (automation.TriggerEvaluationRecord, bool, error) {
	if existing := state.Evaluation; existing != nil && existing.Status != automation.TriggerEvaluationStatusCompleted {
		if existing.TaskID != task.ID || existing.TriggerID != trigger.ID {
			return automation.TriggerEvaluationRecord{}, false, fmt.Errorf("pending semantic evaluation does not belong to task trigger %s/%s", task.ID, trigger.ID)
		}
		return *existing, true, nil
	}

	claim, observed, err := s.semanticTriggerClaim(snap, now, task, trigger, state)
	if err != nil || !observed {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	if claim.ObservationFingerprint == state.LastObservationFingerprint {
		// Completion already advanced the durable observation barrier. Chapter
		// metadata/content edits inside the same batch are intentionally not a
		// new batch identity; a changed condition produces a different claim.
		return automation.TriggerEvaluationRecord{}, false, nil
	}
	// Trigger states written before the durable coordinator have no receipt.
	// Preserve their completed observation marker instead of re-running history.
	if state.Evaluation == nil && claim.ObservationFingerprint == state.LastEvidenceFingerprint {
		return automation.TriggerEvaluationRecord{}, false, nil
	}
	record, disposition, err := store.ClaimTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, claim)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	if disposition == automation.TriggerEvaluationReplayed {
		return record, false, nil
	}
	return record, true, nil
}

func (s *Service) semanticTriggerClaim(snap *automationWorkspaceSnapshot, now time.Time, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerEvaluationRecord, bool, error) {
	condition := strings.TrimSpace(trigger.SemanticCondition)
	if condition == "" {
		return automation.TriggerEvaluationRecord{}, false, nil
	}
	batch, _, observed, err := s.nextChapterBatchTriggerScope(snap, task, trigger, state, true, false)
	if err != nil || !observed {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	triggerContext := automation.BoundedTriggerContext(automation.TriggerContext{
		Source:   "semantic_chapter_batch",
		Summary:  fmt.Sprintf("Semantic trigger check for chapter batch %d: %d non-empty chapters reached. Only evaluate this batch scope.", batch.Number, batch.End),
		Evidence: batch.Evidence,
	})
	if strings.TrimSpace(triggerContext.Summary) == "" && len(triggerContext.Evidence) == 0 {
		return automation.TriggerEvaluationRecord{}, false, nil
	}
	observationParts := append(s.triggerFingerprintParts(snap, task), task.ID, trigger.ID, condition, batch.Fingerprint)
	observation := automation.EvidenceFingerprint(observationParts...)
	instruction, err := buildSemanticTriggerInstruction(task, trigger, triggerContext)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	claim, err := automation.NewSemanticTriggerEvaluation(automation.SemanticTriggerIntent{
		Scope:                  task.Scope,
		ProjectID:              snap.projectID,
		Workspace:              snap.workspace,
		TaskID:                 task.ID,
		TriggerID:              trigger.ID,
		Condition:              condition,
		ObservationFingerprint: observation,
		Instruction:            instruction,
		Context:                triggerContext,
		ActionPolicy:           automation.EffectiveActionPolicy(task, trigger),
		NotifyPolicy:           automation.EffectiveNotifyPolicy(task, trigger),
	}, now)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	return claim, true, nil
}

func (s *Service) evaluateClaimedSemanticTrigger(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	task automation.Task,
	stateKey string,
	record automation.TriggerEvaluationRecord,
) (automation.TriggerEvaluationRecord, error) {
	runtimeCfg := runtimeConfigForTask(snap, task)
	evaluator := s.semanticEvaluator
	if evaluator == nil {
		evaluator = agentmodeltask.GenerateAutomationTriggerEvaluation
	}
	raw, err := evaluator(ctx, &runtimeCfg, record.Instruction)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, fmt.Errorf("semantic evaluator task_id=%s trigger_id=%s: %w", task.ID, record.TriggerID, err)
	}
	evaluation, err := automation.ParseSemanticEvaluation(raw)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, fmt.Errorf("semantic evaluator invalid output task_id=%s trigger_id=%s raw=%s: %w", task.ID, record.TriggerID, trimForTriggerSnippet(raw, 300), err)
	}
	evaluation, err = automation.ValidateSemanticEvaluationEvidence(evaluation, record.Context)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, fmt.Errorf("semantic evaluator referenced unclaimed evidence task_id=%s trigger_id=%s: %w", task.ID, record.TriggerID, err)
	}
	var match *automation.TriggerMatch
	if evaluation.Matched && evaluation.Confidence >= semanticTriggerConfidenceThreshold {
		title := evaluation.Title
		if title == "" {
			title = "Semantic trigger matched"
		}
		summary := evaluation.Reason
		if summary == "" {
			summary = fmt.Sprintf("Semantic condition matched: %s", record.Condition)
		}
		canonical := automation.TriggerMatch{
			TaskID:      record.TaskID,
			TriggerID:   record.TriggerID,
			Title:       title,
			Summary:     summary,
			Evidence:    record.Context.Evidence,
			Fingerprint: automation.EvidenceFingerprint(record.Scope, record.Workspace, record.TaskID, record.TriggerID, record.Condition, record.ObservationFingerprint, evaluation.Reason),
		}
		match = &canonical
	}
	return store.DecideTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, record.ID, record.IntentHash, evaluation, match, now)
}

func (s *Service) reconcileDurableTriggerAction(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	task automation.Task,
	record automation.TriggerEvaluationRecord,
	start durableTriggerRunStarter,
) (automation.TriggerInboxItem, automation.RunResult, bool, error) {
	if record.Match == nil || record.Action == nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, true, nil
	}
	match := *record.Match
	plan := *record.Action
	visibleItem := automation.TriggerInboxItem{}
	item := automation.TriggerInboxItem{}
	if plan.InboxID != "" {
		status := automation.InboxStatusPending
		var created bool
		var err error
		item, created, err = store.EnsureInboxItem(ctx, automation.TriggerInboxItem{
			ID:           plan.InboxID,
			TaskID:       record.TaskID,
			TriggerID:    record.TriggerID,
			Scope:        record.Scope,
			ProjectID:    record.ProjectID,
			Workspace:    record.Workspace,
			Status:       status,
			ActionPolicy: plan.ActionPolicy,
			NotifyPolicy: plan.NotifyPolicy,
			Title:        match.Title,
			Summary:      match.Summary,
			Evidence:     match.Evidence,
			Fingerprint:  match.Fingerprint,
			CreatedAt:    record.DecidedAt,
			UpdatedAt:    record.DecidedAt,
		})
		if err != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
		}
		if created {
			visibleItem = item
		}
	}
	if plan.RunID == "" {
		return visibleItem, automation.RunResult{}, true, nil
	}
	if start == nil {
		return visibleItem, automation.RunResult{}, false, fmt.Errorf("durable semantic trigger run starter is required")
	}
	_, startedRun, startErr := start(ctx, automationTaskStoreID(task), runTriggerForEvaluation(record), plan.RunID, match.Evidence)
	if startedRun.ID != "" && startedRun.ID != plan.RunID {
		return visibleItem, automation.RunResult{}, false, fmt.Errorf("durable trigger run identity changed: got %s want %s", startedRun.ID, plan.RunID)
	}
	if startErr != nil {
		if startedRun.ID == "" {
			return visibleItem, automation.RunResult{}, false, fmt.Errorf("%w: %w", errDurableTriggerActionRetry, startErr)
		}
		if receiptErr := validateAutomationRunRootReceipt(startedRun); receiptErr == nil && !startedRun.RuntimeRecoveryRequired {
			// The action crossed the durable runtime boundary. Caller transport or
			// bookkeeping failure cannot make the trigger submit it again.
		} else if startedRun.Status == automation.RunStatusFailed {
			if item.ID != "" {
				failed, markErr := store.MarkInboxItemRunStartFailed(item.ID, fmt.Sprintf("%s\n\n自动执行启动失败：%s。请确认后手动重试。", match.Summary, startErr.Error()))
				if markErr != nil {
					return automation.TriggerInboxItem{}, automation.RunResult{}, false, markErr
				}
				if visibleItem.ID != "" {
					visibleItem = failed
				}
			}
			return visibleItem, automation.RunResult{}, false, fmt.Errorf("%w: %w", errDurableTriggerActionRetry, startErr)
		} else {
			return visibleItem, automation.RunResult{}, false, fmt.Errorf("%w: %w", errDurableTriggerActionRetry, startErr)
		}
	}
	if receiptErr := validateAutomationRunRootReceipt(startedRun); receiptErr != nil {
		return visibleItem, automation.RunResult{}, false, fmt.Errorf("%w: automation run %s has no valid durable root receipt: %v", errDurableTriggerActionRetry, startedRun.ID, receiptErr)
	}
	if startedRun.RuntimeRecoveryRequired {
		return visibleItem, automation.RunResult{Task: task, Run: startedRun}, false, fmt.Errorf("%w: automation run %s requires explicit runtime recovery", errDurableTriggerActionRetry, startedRun.ID)
	}
	if item.ID != "" {
		updated, attachErr := store.AttachInboxRun(item.ID, startedRun.ID)
		if attachErr != nil {
			return visibleItem, automation.RunResult{}, false, attachErr
		}
		if visibleItem.ID != "" {
			visibleItem = updated
		}
	}
	return visibleItem, automation.RunResult{Task: task, Run: startedRun}, true, nil
}

func runTriggerForEvaluation(record automation.TriggerEvaluationRecord) string {
	if record.TriggerType == automation.TriggerTypeSchedule {
		return automation.TriggerSchedule
	}
	return automation.TriggerCondition
}

func enabledSemanticTrigger(task automation.Task, triggerID string) (automation.TriggerDefinition, bool) {
	for _, trigger := range task.Triggers {
		if trigger.ID == triggerID && trigger.Enabled && trigger.Type == automation.TriggerTypeSemantic {
			return trigger, true
		}
	}
	return automation.TriggerDefinition{}, false
}

func buildSemanticTriggerInstruction(task automation.Task, trigger automation.TriggerDefinition, ctx automation.TriggerContext) (string, error) {
	payload, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode bounded semantic trigger context: %w", err)
	}
	instruction := fmt.Sprintf(`请判断当前有界创作上下文是否满足这个自动化语义触发条件。

任务名称：%s
触发器名称：%s
语义条件：%s

判定要求：
- 只根据下方 JSON 中的 summary 和 evidence 判断，不要补充不存在的剧情。
- “新角色登场”“角色状态变化”“章节完成质检”等都只是语义条件的一种，由你统一判断是否已经发生。
- 如果证据不足、只是可能发生、或上下文没有新增相关内容，matched 必须为 false。
- confidence 取 0 到 1；低于 0.55 视为不触发。
- evidence_refs 只能引用 evidence.ref 或 evidence.title 中已有值。
- 只输出 JSON：{"matched": boolean, "confidence": number, "reason": string, "title": string, "evidence_refs": string[]}

有界上下文 JSON：
%s`, strings.TrimSpace(task.Name), strings.TrimSpace(trigger.Name), strings.TrimSpace(trigger.SemanticCondition), string(payload))
	return instruction, nil
}

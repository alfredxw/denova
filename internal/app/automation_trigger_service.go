package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agent"
	"denova/internal/automation"
	"denova/internal/book"
)

const semanticTriggerConfidenceThreshold = 0.55

var semanticTriggerEvaluator = agent.GenerateAutomationTriggerEvaluation

type chapterBatchTriggerScope struct {
	Number            int
	End               int
	Fingerprint       string
	LegacyFingerprint string
	Evidence          []automation.TriggerEvidence
}

func (a *App) AutomationInbox() ([]automation.TriggerInboxItem, error) {
	return a.automation().Inbox()
}

func (s *AutomationAppService) Inbox() ([]automation.TriggerInboxItem, error) {
	return s.store().ListInbox()
}

func (a *App) CheckAutomationTriggers(ctx context.Context, id string) ([]automation.TriggerInboxItem, error) {
	return a.automation().CheckTriggers(ctx, id)
}

func (s *AutomationAppService) CheckTriggers(ctx context.Context, id string) ([]automation.TriggerInboxItem, error) {
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return nil, ErrNoWorkspace
		}
		return scoped.CheckTriggers(ctx, id)
	}
	items, _, err := s.processTriggers(ctx, strings.TrimSpace(id), time.Now().UTC(), "manual_check")
	return items, err
}

func (a *App) CheckAutomationTriggersAfterWorkspaceMutation(ctx context.Context, source string, paths []string) {
	a.automation().CheckTriggersAfterWorkspaceMutation(ctx, source, paths)
}

func (s *AutomationAppService) CheckTriggersAfterWorkspaceMutation(ctx context.Context, source string, paths []string) {
	_ = ctx // request cancellation must not own App background trigger work
	scoped := s.workspaceScoped()
	if scoped == nil {
		return
	}
	targets := scoped.chapterContentMutationPaths(paths)
	if len(targets) == 0 {
		return
	}
	if scoped.app == nil || scoped.app.automationTriggers == nil || !scoped.app.automationTriggers.Enqueue(scoped, source, targets) {
		log.Printf("[automation-trigger] mutation check skipped because app lifecycle is closed source=%s workspace=%q targets=%q", source, scoped.workspace(), targets)
	}
}

func (s *AutomationAppService) CheckContentTriggersForWorkspaceMutation(ctx context.Context, source string, paths []string) ([]automation.TriggerInboxItem, error) {
	scoped := s.workspaceScoped()
	if scoped == nil || len(scoped.chapterContentMutationPaths(paths)) == 0 {
		return nil, nil
	}
	items, _, err := scoped.processContentTriggers(ctx, time.Now().UTC(), source)
	return items, err
}

func (a *App) ConfirmAutomationInboxItem(ctx context.Context, id string) (automation.InboxActionResult, error) {
	return a.automation().ConfirmInboxItem(ctx, id)
}

func (s *AutomationAppService) ConfirmInboxItem(ctx context.Context, id string) (automation.InboxActionResult, error) {
	if s.snapshot == nil {
		scoped := s.workspaceScoped()
		if scoped == nil {
			return automation.InboxActionResult{}, ErrNoWorkspace
		}
		return scoped.ConfirmInboxItem(ctx, id)
	}
	store := s.store()
	item, err := store.GetInboxItem(id)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	task, err := store.Get(item.TaskID)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	if item.Status != automation.InboxStatusPending {
		return automation.InboxActionResult{}, fmt.Errorf("automation inbox item %s is not pending", id)
	}
	trigger := automation.TriggerInboxConfirmation
	sourceRunID := ""
	if item.Purpose == automation.InboxPurposeWriteConfirmation {
		trigger = automation.TriggerWriteConfirmation
		sourceRunID = item.SourceRunID
	}
	_, run, err := s.startTaskWithSourceRun(ctx, task.ID, trigger, sourceRunID, item.Evidence)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	updated, err := store.ConfirmInboxItem(id, run.ID)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	return automation.InboxActionResult{Item: updated, Run: &run}, nil
}

func (a *App) DismissAutomationInboxItem(id string) (automation.TriggerInboxItem, error) {
	return a.automation().DismissInboxItem(id)
}

func (s *AutomationAppService) DismissInboxItem(id string) (automation.TriggerInboxItem, error) {
	return s.store().DismissInboxItem(id)
}

func (a *App) MarkAutomationInboxItemRead(id string) (automation.TriggerInboxItem, error) {
	return a.automation().MarkInboxItemRead(id)
}

func (s *AutomationAppService) MarkInboxItemRead(id string) (automation.TriggerInboxItem, error) {
	return s.store().MarkInboxItemRead(id)
}

func (s *AutomationAppService) processTriggers(ctx context.Context, onlyTaskID string, now time.Time, source string) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	return s.processTriggersMatching(ctx, onlyTaskID, now, source, nil)
}

func (s *AutomationAppService) processContentTriggers(ctx context.Context, now time.Time, source string) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	return s.processTriggersMatching(ctx, "", now, source, func(trigger automation.TriggerDefinition) bool {
		return trigger.Type == automation.TriggerTypeChapterBatch || trigger.Type == automation.TriggerTypeSemantic
	})
}

func (s *AutomationAppService) processTriggersMatching(ctx context.Context, onlyTaskID string, now time.Time, source string, includeTrigger func(automation.TriggerDefinition) bool) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	unlock := triggerExecutionLocks.lock(s.workspace())
	defer unlock()
	tasks, err := s.List()
	if err != nil {
		return nil, nil, err
	}
	items := []automation.TriggerInboxItem{}
	runs := []automation.RunResult{}
	for _, task := range tasks {
		if onlyTaskID != "" && task.ID != onlyTaskID {
			continue
		}
		if !task.Enabled {
			continue
		}
		for _, trigger := range task.Triggers {
			if !trigger.Enabled {
				continue
			}
			if includeTrigger != nil && !includeTrigger(trigger) {
				continue
			}
			stateKey := s.triggerStateKey(task, trigger)
			match, nextState, matched, err := s.evaluateTrigger(ctx, now, source, task, trigger)
			if err != nil {
				log.Printf("[automation-trigger] evaluate failed task_id=%s trigger_id=%s type=%s err=%v", task.ID, trigger.ID, trigger.Type, err)
				_, _ = s.store().UpdateTriggerState(task.ID, stateKey, nextState)
				continue
			}
			if !matched {
				_, _ = s.store().UpdateTriggerState(task.ID, stateKey, nextState)
				continue
			}
			item, run, processed, err := s.processTriggerMatch(ctx, now, task, trigger, match)
			if err != nil {
				log.Printf("[automation-trigger] process failed task_id=%s trigger_id=%s type=%s err=%v", task.ID, trigger.ID, trigger.Type, err)
				continue
			}
			if processed {
				nextState.LastMatchedAt = now
				nextState.LastEvidenceFingerprint = match.Fingerprint
			}
			if item.ID != "" {
				items = append(items, item)
			}
			if run.Run.ID != "" {
				runs = append(runs, run)
			}
			_, _ = s.store().UpdateTriggerState(task.ID, stateKey, nextState)
		}
	}
	return items, runs, nil
}

func (s *AutomationAppService) chapterContentMutationPaths(paths []string) []string {
	workspace := s.workspace()
	seen := map[string]bool{}
	targets := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) && strings.TrimSpace(workspace) != "" {
			rel, err := filepath.Rel(workspace, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = rel
			} else {
				canonicalWorkspace := canonicalAutomationWorkspace(workspace)
				canonicalPath := path
				if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
					canonicalPath = resolved
				}
				if rel, relErr := filepath.Rel(canonicalWorkspace, canonicalPath); relErr == nil {
					path = rel
				}
			}
		}
		path = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(path), "./"))
		if !isChapterContentMutationPath(path) || seen[path] {
			continue
		}
		seen[path] = true
		targets = append(targets, path)
	}
	return targets
}

func isChapterContentMutationPath(path string) bool {
	path = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(path), "./"))
	if !strings.HasPrefix(path, "chapters/") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".txt"
}

func (s *AutomationAppService) evaluateTrigger(ctx context.Context, now time.Time, source string, task automation.Task, trigger automation.TriggerDefinition) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	state := task.TriggerState[s.triggerStateKey(task, trigger)]
	state.LastCheckedAt = now
	switch trigger.Type {
	case automation.TriggerTypeSchedule:
		return s.evaluateScheduleTrigger(now, task, trigger, state)
	case automation.TriggerTypeChapterBatch:
		return s.evaluateChapterBatchTrigger(now, task, trigger, state)
	case automation.TriggerTypeSemantic:
		return s.evaluateSemanticTrigger(ctx, source, task, trigger, state)
	default:
		return automation.TriggerMatch{}, state, false, nil
	}
}

func (s *AutomationAppService) triggerStateKey(task automation.Task, trigger automation.TriggerDefinition) string {
	if task.Scope != automation.ScopeUser {
		return trigger.ID
	}
	return trigger.ID + "@workspace:" + automation.EvidenceFingerprint(canonicalAutomationWorkspace(s.workspace()))
}

func (s *AutomationAppService) triggerFingerprintParts(task automation.Task) []string {
	if task.Scope != automation.ScopeUser {
		return nil
	}
	return []string{"workspace=" + canonicalAutomationWorkspace(s.workspace())}
}

func (s *AutomationAppService) nextChapterBatchTriggerScope(task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState, includeContent bool, dedupeBatchState bool) (chapterBatchTriggerScope, automation.TriggerState, bool, error) {
	batchSize := trigger.ChapterBatchSize
	if batchSize < 1 {
		batchSize = 5
	}
	bookService := s.bookService()
	if bookService == nil {
		return chapterBatchTriggerScope{}, state, false, nil
	}
	summary, err := bookService.Summary()
	if err != nil {
		return chapterBatchTriggerScope{}, state, false, err
	}
	chapters := make([]book.ChapterSummary, 0, len(summary.Chapters))
	for _, chapter := range summary.Chapters {
		if chapter.Words > 0 {
			chapters = append(chapters, chapter)
		}
	}
	if len(chapters) < batchSize {
		return chapterBatchTriggerScope{}, state, false, nil
	}
	batchNumber := len(chapters) / batchSize
	batchEnd := batchNumber * batchSize
	batchStart := batchEnd - batchSize
	batch := chapters[batchStart:batchEnd]
	fingerprintParts := append(s.triggerFingerprintParts(task), task.ID, trigger.ID, fmt.Sprintf("batch_size=%d", batchSize), fmt.Sprintf("batch=%d", batchNumber))
	legacyFingerprintParts := append([]string(nil), fingerprintParts...)
	evidence := make([]automation.TriggerEvidence, 0, len(batch))
	source := "chapter_batch"
	if trigger.Type == automation.TriggerTypeSemantic {
		source = "semantic_chapter_batch"
	}
	for _, chapter := range batch {
		fingerprintParts = append(fingerprintParts, chapter.Path)
		legacyFingerprintParts = append(legacyFingerprintParts, chapter.Path, fmt.Sprintf("words=%d", chapter.Words), chapter.UpdatedAt)
		snippet := fmt.Sprintf("batch=%d words=%d status=%s updated=%s", batchNumber, chapter.Words, chapter.Status, chapter.UpdatedAt)
		if includeContent {
			if content, err := bookService.ReadFile(chapter.Path); err == nil {
				snippet = fmt.Sprintf("%s\ncontent_excerpt=%s", snippet, trimForTriggerSnippet(content, 1400))
			} else {
				log.Printf("[automation-trigger] read chapter batch evidence failed path=%s err=%v", chapter.Path, err)
			}
		}
		evidence = append(evidence, automation.TriggerEvidence{
			Source:  source,
			Title:   chapter.DisplayTitle,
			Ref:     chapter.Path,
			Snippet: snippet,
		})
	}
	scope := chapterBatchTriggerScope{
		Number:            batchNumber,
		End:               batchEnd,
		Fingerprint:       automation.EvidenceFingerprint(fingerprintParts...),
		LegacyFingerprint: automation.EvidenceFingerprint(legacyFingerprintParts...),
		Evidence:          evidence,
	}
	if dedupeBatchState {
		if scope.Fingerprint == state.LastEvidenceFingerprint || scope.Fingerprint == state.LastObservationFingerprint || scope.LegacyFingerprint == state.LastEvidenceFingerprint || scope.LegacyFingerprint == state.LastObservationFingerprint {
			state.LastEvidenceFingerprint = scope.Fingerprint
			state.LastObservationFingerprint = scope.Fingerprint
			return chapterBatchTriggerScope{}, state, false, nil
		}
		state.LastObservationFingerprint = scope.Fingerprint
	}
	return scope, state, true, nil
}

func (s *AutomationAppService) evaluateChapterBatchTrigger(now time.Time, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	batch, nextState, matched, err := s.nextChapterBatchTriggerScope(task, trigger, state, false, true)
	if err != nil {
		return automation.TriggerMatch{}, nextState, false, err
	}
	if !matched {
		return automation.TriggerMatch{}, nextState, false, nil
	}
	title := fmt.Sprintf("%s reached chapter batch %d", task.Name, batch.Number)
	summaryText := fmt.Sprintf("Chapter batch %d is ready: %d non-empty chapters reached at %s.", batch.Number, batch.End, now.Local().Format("2006-01-02 15:04"))
	return automation.TriggerMatch{
		TaskID:      task.ID,
		TriggerID:   trigger.ID,
		Title:       title,
		Summary:     summaryText,
		Evidence:    batch.Evidence,
		Fingerprint: batch.Fingerprint,
	}, nextState, true, nil
}

func (s *AutomationAppService) evaluateScheduleTrigger(now time.Time, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	last := state.LastMatchedAt
	if last.IsZero() && task.LastRun != nil {
		if task.Scope != automation.ScopeUser || canonicalAutomationWorkspace(task.LastRun.Workspace) == canonicalAutomationWorkspace(s.workspace()) {
			last = task.LastRun.StartedAt
		}
	}
	if !scheduleDueForTrigger(now, last, trigger.Schedule) {
		return automation.TriggerMatch{}, state, false, nil
	}
	minute := now.Truncate(time.Minute).Format(time.RFC3339)
	fingerprintParts := append(s.triggerFingerprintParts(task), task.ID, trigger.ID, trigger.Schedule.Cron, minute)
	fingerprint := automation.EvidenceFingerprint(fingerprintParts...)
	match := automation.TriggerMatch{
		TaskID:      task.ID,
		TriggerID:   trigger.ID,
		Title:       fmt.Sprintf("%s scheduled trigger", task.Name),
		Summary:     fmt.Sprintf("Schedule %s is due at %s.", trigger.Schedule.Kind, now.Local().Format("2006-01-02 15:04")),
		Fingerprint: fingerprint,
		Evidence: []automation.TriggerEvidence{{
			Source:  "schedule",
			Title:   trigger.Schedule.Kind,
			Snippet: trigger.Schedule.Cron,
		}},
	}
	return match, state, true, nil
}

func (s *AutomationAppService) evaluateSemanticTrigger(ctx context.Context, source string, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	condition := strings.TrimSpace(trigger.SemanticCondition)
	if condition == "" {
		return automation.TriggerMatch{}, state, false, nil
	}
	batch, nextState, matched, err := s.nextChapterBatchTriggerScope(task, trigger, state, true, false)
	if err != nil {
		return automation.TriggerMatch{}, nextState, false, err
	}
	if !matched {
		return automation.TriggerMatch{}, nextState, false, nil
	}
	triggerCtx := automation.BoundedTriggerContext(automation.TriggerContext{
		Source:   source,
		Summary:  fmt.Sprintf("Semantic trigger check for chapter batch %d: %d non-empty chapters reached. Only evaluate this batch scope.", batch.Number, batch.End),
		Evidence: batch.Evidence,
	})
	if strings.TrimSpace(triggerCtx.Summary) == "" && len(triggerCtx.Evidence) == 0 {
		return automation.TriggerMatch{}, nextState, false, nil
	}
	observationParts := append(s.triggerFingerprintParts(task), task.ID, trigger.ID, condition, batch.Fingerprint)
	observation := automation.EvidenceFingerprint(observationParts...)
	if observation == state.LastObservationFingerprint || observation == state.LastEvidenceFingerprint {
		nextState.LastObservationFingerprint = observation
		return automation.TriggerMatch{}, nextState, false, nil
	}
	instruction := buildSemanticTriggerInstruction(task, trigger, triggerCtx)
	runtimeCfg := s.runtimeConfigForTask(task)
	raw, err := semanticTriggerEvaluator(ctx, &runtimeCfg, instruction)
	if err != nil {
		log.Printf("[automation-trigger] semantic evaluator failed task_id=%s trigger_id=%s err=%v", task.ID, trigger.ID, err)
		return automation.TriggerMatch{}, nextState, false, nil
	}
	eval, err := automation.ParseSemanticEvaluation(raw)
	if err != nil {
		log.Printf("[automation-trigger] semantic evaluator invalid output task_id=%s trigger_id=%s err=%v raw=%s", task.ID, trigger.ID, err, trimForTriggerSnippet(raw, 300))
		return automation.TriggerMatch{}, nextState, false, nil
	}
	nextState.LastObservationFingerprint = observation
	if !eval.Matched || eval.Confidence < semanticTriggerConfidenceThreshold {
		return automation.TriggerMatch{}, nextState, false, nil
	}
	title := eval.Title
	if title == "" {
		title = "Semantic trigger matched"
	}
	summary := eval.Reason
	if summary == "" {
		summary = fmt.Sprintf("Semantic condition matched: %s", condition)
	}
	match := automation.TriggerMatch{
		TaskID:      task.ID,
		TriggerID:   trigger.ID,
		Title:       title,
		Summary:     summary,
		Evidence:    triggerCtx.Evidence,
		Fingerprint: automation.EvidenceFingerprint(append(s.triggerFingerprintParts(task), task.ID, trigger.ID, condition, observation, eval.Reason)...),
	}
	return match, nextState, true, nil
}

func (s *AutomationAppService) processTriggerMatch(ctx context.Context, now time.Time, task automation.Task, trigger automation.TriggerDefinition, match automation.TriggerMatch) (automation.TriggerInboxItem, automation.RunResult, bool, error) {
	store := s.store()
	if trigger.Type == automation.TriggerTypeChapterBatch {
		if existing, ok, err := store.FindInboxItemByEvidence(task.ID, trigger.ID, match.Evidence); err != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
		} else if ok {
			if existing.Status == automation.InboxStatusPending || existing.Status == automation.InboxStatusAutoRun {
				return existing, automation.RunResult{}, true, nil
			}
			return automation.TriggerInboxItem{}, automation.RunResult{}, true, nil
		}
	}
	if existing, ok, err := store.FindOpenInboxItem(task.ID, trigger.ID, match.Fingerprint); err != nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	} else if ok {
		return existing, automation.RunResult{}, true, nil
	}

	actionPolicy := automation.EffectiveActionPolicy(task, trigger)
	notifyPolicy := automation.EffectiveNotifyPolicy(task, trigger)
	if actionPolicy == automation.ActionPolicyConfirm {
		notifyPolicy = automation.NotifyPolicyInbox
	}
	item := automation.TriggerInboxItem{}
	shouldCreateInbox := notifyPolicy == automation.NotifyPolicyInbox || actionPolicy == automation.ActionPolicyConfirm
	if shouldCreateInbox {
		status := automation.InboxStatusPending
		if actionPolicy == automation.ActionPolicyAutoRun {
			status = automation.InboxStatusAutoRun
		}
		var err error
		item, err = store.CreateInboxItem(automation.TriggerInboxItem{
			TaskID:       task.ID,
			TriggerID:    trigger.ID,
			Scope:        task.Scope,
			Workspace:    s.workspace(),
			Status:       status,
			ActionPolicy: actionPolicy,
			NotifyPolicy: notifyPolicy,
			Title:        match.Title,
			Summary:      match.Summary,
			Evidence:     match.Evidence,
			Fingerprint:  match.Fingerprint,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if err != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
		}
	}
	if actionPolicy != automation.ActionPolicyAutoRun {
		return item, automation.RunResult{}, true, nil
	}
	_, run, err := s.startTaskWithSourceRun(ctx, task.ID, runTriggerForDefinition(trigger), "", match.Evidence)
	if err != nil {
		if item.ID != "" {
			log.Printf("[automation-trigger] auto-run start failed after inbox created task_id=%s trigger_id=%s inbox_id=%s err=%v", task.ID, trigger.ID, item.ID, err)
			failed, markErr := store.MarkInboxItemRunStartFailed(item.ID, fmt.Sprintf("%s\n\n自动执行启动失败：%s。请确认后手动重试。", match.Summary, err.Error()))
			if markErr != nil {
				return automation.TriggerInboxItem{}, automation.RunResult{}, false, markErr
			}
			return failed, automation.RunResult{}, true, nil
		}
		return item, automation.RunResult{}, false, err
	}
	if item.ID != "" {
		if updated, attachErr := store.AttachInboxRun(item.ID, run.ID); attachErr == nil {
			item = updated
		}
	}
	return item, automation.RunResult{Task: task, Run: run}, true, nil
}

func buildSemanticTriggerInstruction(task automation.Task, trigger automation.TriggerDefinition, ctx automation.TriggerContext) string {
	payload, _ := json.MarshalIndent(ctx, "", "  ")
	return fmt.Sprintf(`请判断当前有界创作上下文是否满足这个自动化语义触发条件。

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
}

func scheduleDueForTrigger(now, last time.Time, schedule automation.Schedule) bool {
	return automation.ScheduleDue(now, last, schedule)
}

func runTriggerForDefinition(trigger automation.TriggerDefinition) string {
	if trigger.Type == automation.TriggerTypeSchedule {
		return automation.TriggerSchedule
	}
	return automation.TriggerCondition
}

func trimForTriggerSnippet(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

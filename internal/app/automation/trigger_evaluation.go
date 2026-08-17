package automationapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"denova/internal/automation"
	"denova/internal/book"
)

type chapterBatchTriggerScope struct {
	Number            int
	End               int
	Fingerprint       string
	LegacyFingerprint string
	Evidence          []automation.TriggerEvidence
}

func (s *Service) evaluateTrigger(_ context.Context, snap *automationWorkspaceSnapshot, now time.Time, _ string, task automation.Task, trigger automation.TriggerDefinition) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	state := task.TriggerState[s.triggerStateKey(snap, task, trigger)]
	state.LastCheckedAt = now
	switch trigger.Type {
	case automation.TriggerTypeSchedule:
		return s.evaluateScheduleTrigger(snap, now, task, trigger, state)
	case automation.TriggerTypeChapterBatch:
		return s.evaluateChapterBatchTrigger(snap, now, task, trigger, state)
	case automation.TriggerTypeSemantic:
		return automation.TriggerMatch{}, state, false, fmt.Errorf("semantic trigger evaluation requires the durable coordinator")
	case automation.TriggerTypeManual:
		return automation.TriggerMatch{}, state, false, nil
	default:
		return automation.TriggerMatch{}, state, false, fmt.Errorf("unsupported automation trigger type %q", trigger.Type)
	}
}

func (s *Service) triggerStateKey(snap *automationWorkspaceSnapshot, task automation.Task, trigger automation.TriggerDefinition) string {
	if task.Scope != automation.ScopeUser {
		return trigger.ID
	}
	return trigger.ID + "@workspace:" + automation.EvidenceFingerprint(canonicalAutomationWorkspace(snap.workspace))
}

func (s *Service) triggerFingerprintParts(snap *automationWorkspaceSnapshot, task automation.Task) []string {
	if task.Scope != automation.ScopeUser {
		return nil
	}
	return []string{"workspace=" + canonicalAutomationWorkspace(snap.workspace)}
}

func (s *Service) nextChapterBatchTriggerScope(snap *automationWorkspaceSnapshot, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState, includeContent bool, dedupeBatchState bool) (chapterBatchTriggerScope, automation.TriggerState, bool, error) {
	batchSize := trigger.ChapterBatchSize
	if batchSize < 1 {
		batchSize = 5
	}
	bookService := snap.bookService
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
	fingerprintParts := append(s.triggerFingerprintParts(snap, task), task.ID, trigger.ID, fmt.Sprintf("batch_size=%d", batchSize), fmt.Sprintf("batch=%d", batchNumber))
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
				slog.ErrorContext(context.Background(), fmt.Sprintf("[automation-trigger] read chapter batch evidence failed path=%s err=%v", chapter.Path, err))
			}
		}
		evidence = append(evidence, automation.TriggerEvidence{Source: source, Title: chapter.DisplayTitle, Ref: chapter.Path, Snippet: snippet})
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

func (s *Service) evaluateChapterBatchTrigger(snap *automationWorkspaceSnapshot, now time.Time, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	batch, nextState, matched, err := s.nextChapterBatchTriggerScope(snap, task, trigger, state, false, true)
	if err != nil {
		return automation.TriggerMatch{}, nextState, false, err
	}
	if !matched {
		return automation.TriggerMatch{}, nextState, false, nil
	}
	return automation.TriggerMatch{
		TaskID:      task.ID,
		TriggerID:   trigger.ID,
		Title:       fmt.Sprintf("%s reached chapter batch %d", task.Name, batch.Number),
		Summary:     fmt.Sprintf("Chapter batch %d is ready: %d non-empty chapters reached at %s.", batch.Number, batch.End, now.Local().Format(book.DisplayTimeFormat)),
		Evidence:    batch.Evidence,
		Fingerprint: batch.Fingerprint,
	}, nextState, true, nil
}

func (s *Service) evaluateScheduleTrigger(snap *automationWorkspaceSnapshot, now time.Time, task automation.Task, trigger automation.TriggerDefinition, state automation.TriggerState) (automation.TriggerMatch, automation.TriggerState, bool, error) {
	last := state.LastMatchedAt
	if last.IsZero() && task.LastRun != nil && (task.Scope != automation.ScopeUser || canonicalAutomationWorkspace(task.LastRun.Workspace) == canonicalAutomationWorkspace(snap.workspace)) {
		last = task.LastRun.StartedAt
	}
	if !scheduleDueForTrigger(now, last, trigger.Schedule) {
		return automation.TriggerMatch{}, state, false, nil
	}
	minute := now.Truncate(time.Minute).Format(time.RFC3339)
	fingerprintParts := append(s.triggerFingerprintParts(snap, task), task.ID, trigger.ID, trigger.Schedule.Cron, minute)
	return automation.TriggerMatch{
		TaskID: task.ID, TriggerID: trigger.ID,
		Title:       fmt.Sprintf("%s scheduled trigger", task.Name),
		Summary:     fmt.Sprintf("Schedule %s is due at %s.", trigger.Schedule.Kind, now.Local().Format(book.DisplayTimeFormat)),
		Fingerprint: automation.EvidenceFingerprint(fingerprintParts...),
		Evidence:    []automation.TriggerEvidence{{Source: "schedule", Title: trigger.Schedule.Kind, Snippet: trigger.Schedule.Cron}},
	}, state, true, nil
}

func scheduleDueForTrigger(now, last time.Time, schedule automation.Schedule) bool {
	return automation.ScheduleDue(now, last, schedule)
}

func trimForTriggerSnippet(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

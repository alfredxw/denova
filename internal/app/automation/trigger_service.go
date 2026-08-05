package automationapp

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"denova/internal/automation"
)

func (s *Service) Inbox() ([]automation.TriggerInboxItem, error) {
	return s.storeAllWorkspaces().ListInbox()
}

func (s *Service) CheckTriggers(ctx context.Context, id string) ([]automation.TriggerInboxItem, error) {
	task, getErr := s.storeAllWorkspaces().Get(id)
	if getErr != nil {
		return nil, getErr
	}
	if task.ArchivedAt != nil {
		return nil, fmt.Errorf("%w: task_id=%s", automation.ErrTaskArchived, automationTaskStoreID(task))
	}
	snap, operation, targetErr := s.acquireTargetRuntime(ctx, task.Target)
	if targetErr != nil {
		return nil, targetErr
	}
	defer operation.Release()
	items, _, err := s.processTriggers(operation.Context(), snap, strings.TrimSpace(id), time.Now().UTC(), "manual_check")
	return items, err
}

func (s *Service) CheckTriggersAfterWorkspaceMutation(ctx context.Context, source string, paths []string) {
	_ = ctx // Request cancellation must not own process background trigger work.
	workspace := strings.TrimSpace(s.host.CurrentWorkspace())
	if workspace == "" {
		return
	}
	snap, operation, err := s.acquireTargetRuntime(context.Background(), automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: workspace})
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[automation-trigger] mutation check admission failed source=%s workspace=%q err=%v", source, workspace, err))
		return
	}
	defer operation.Release()
	s.checkTriggersAfterWorkspaceMutation(ctx, snap, source, paths)
}

// CheckTriggersAfterProjectMutation evaluates content triggers for one stable
// Project without consulting the foreground Book.
func (s *Service) CheckTriggersAfterProjectMutation(ctx context.Context, projectID, source string, paths []string) {
	_ = ctx // Request cancellation must not own process background trigger work.
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	target := automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: projectID}
	snap, operation, err := s.acquireTargetRuntime(context.Background(), target)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[automation-trigger] Project mutation check admission failed source=%s project_id=%q err=%v", source, projectID, err))
		return
	}
	defer operation.Release()
	s.checkTriggersAfterWorkspaceMutation(ctx, snap, source, paths)
}

// checkTriggersAfterWorkspaceMutation evaluates content triggers for an
// explicit workspace/editor mutation bound to the given snapshot. Agent tool
// mutations reach trigger evaluation only through the durable HostEffect
// outbox, so a mutation cannot authorize automation before output commit.
func (s *Service) checkTriggersAfterWorkspaceMutation(ctx context.Context, snap *automationWorkspaceSnapshot, source string, paths []string) {
	targets := s.chapterContentMutationPaths(snap, paths)
	if len(targets) == 0 {
		return
	}
	if s.triggers == nil || !s.triggers.Enqueue(s, snap, source, targets) {
		slog.WarnContext(ctx, fmt.Sprintf("[automation-trigger] mutation check skipped because app lifecycle is closed source=%s workspace=%q targets=%q", source, snap.workspace, targets))
	}
}

func (s *Service) CheckContentTriggersForWorkspaceMutation(ctx context.Context, source string, paths []string) ([]automation.TriggerInboxItem, error) {
	workspace := strings.TrimSpace(s.host.CurrentWorkspace())
	if workspace == "" {
		return nil, nil
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: workspace})
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	targets := s.chapterContentMutationPaths(snap, paths)
	if len(targets) == 0 {
		return nil, nil
	}
	items, _, err := s.processContentTriggers(operation.Context(), snap, time.Now().UTC(), source)
	return items, err
}

func (s *Service) ConfirmInboxItem(ctx context.Context, id string) (automation.InboxActionResult, error) {
	store := s.storeAllWorkspaces()
	item, err := store.GetInboxItem(id)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	target := automation.ExecutionTarget{Kind: automation.TargetKindUser}
	if strings.TrimSpace(item.ProjectID) != "" || strings.TrimSpace(item.Workspace) != "" {
		target = automation.ExecutionTarget{
			Kind:      automation.TargetKindWorkspace,
			ProjectID: item.ProjectID,
			Workspace: item.Workspace,
		}
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, target)
	if err != nil {
		return automation.InboxActionResult{}, err
	}
	defer operation.Release()
	return s.confirmInboxItemWithStarter(operation.Context(), store, snap, item.ID, func(ctx context.Context, taskID, trigger, sourceRunID, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		return s.startTaskWithSourceRunID(ctx, snap, taskID, trigger, sourceRunID, runID, evidence)
	})
}

func (s *Service) DismissInboxItem(id string) (automation.TriggerInboxItem, error) {
	return s.storeAllWorkspaces().DismissInboxItem(id)
}

func (s *Service) MarkInboxItemRead(id string) (automation.TriggerInboxItem, error) {
	return s.storeAllWorkspaces().MarkInboxItemRead(id)
}

func (s *Service) processTriggers(ctx context.Context, snap *automationWorkspaceSnapshot, onlyTaskID string, now time.Time, source string) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	return s.processTriggersMatching(ctx, snap, onlyTaskID, now, source, nil)
}

func (s *Service) processContentTriggers(ctx context.Context, snap *automationWorkspaceSnapshot, now time.Time, source string) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	return s.processTriggersMatching(ctx, snap, "", now, source, func(trigger automation.TriggerDefinition) bool {
		return trigger.Type == automation.TriggerTypeChapterBatch || trigger.Type == automation.TriggerTypeSemantic
	})
}

func (s *Service) processTriggersMatching(ctx context.Context, snap *automationWorkspaceSnapshot, onlyTaskID string, now time.Time, source string, includeTrigger func(automation.TriggerDefinition) bool) ([]automation.TriggerInboxItem, []automation.RunResult, error) {
	unlock := triggerExecutionLocks.Lock(snap.workspace)
	defer unlock()
	target := automation.ExecutionTarget{Kind: automation.TargetKindUser}
	if workspace := strings.TrimSpace(snap.workspace); workspace != "" {
		target = automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: snap.projectID, Workspace: workspace}
	}
	store := storeForSnapshot(snap)
	tasks, err := store.ListForTriggerEvaluation(target)
	if err != nil {
		return nil, nil, err
	}
	items := []automation.TriggerInboxItem{}
	runs := []automation.RunResult{}
	for _, task := range tasks {
		if onlyTaskID != "" && !automation.TaskMatchesID(task, onlyTaskID) {
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
			stateKey := s.triggerStateKey(snap, task, trigger)
			var item automation.TriggerInboxItem
			var run automation.RunResult
			var processed bool
			switch trigger.Type {
			case automation.TriggerTypeSemantic:
				item, run, processed, err = s.processDurableSemanticTrigger(ctx, snap, store, now, task, trigger, stateKey)
			case automation.TriggerTypeSchedule, automation.TriggerTypeChapterBatch:
				item, run, processed, err = s.processDurableBuiltInTrigger(ctx, snap, store, now, task, trigger, stateKey)
			case automation.TriggerTypeManual:
				continue
			default:
				err = fmt.Errorf("unsupported automation trigger type %q", trigger.Type)
			}
			if item.ID != "" {
				items = append(items, item)
			}
			if run.Run.ID != "" {
				runs = append(runs, run)
			}
			if err != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[automation-trigger] durable processing failed source=%s task_id=%s trigger_id=%s type=%s processed=%t err=%v", source, task.ID, trigger.ID, trigger.Type, processed, err))
				if errors.Is(err, errDurableTriggerActionRetry) {
					continue
				}
				return items, runs, err
			}
		}
	}
	return items, runs, nil
}

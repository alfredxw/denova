package automationapp

import (
	"context"
	"log/slog"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/automation"
)

// These fields are API projections only. The trigger store strips them from
// new delivery records, so Agent execution has exactly one source of truth.
func projectRunView(run automation.RunRecord, view agentexecution.OperationProjection) automation.RunRecord {
	run.RuntimeCommandID = string(view.Receipt.CommandID)
	run.RuntimeOperationID = string(view.Receipt.OperationID)
	run.RuntimeReceiptCursor = uint64(view.Receipt.Cursor)
	run.RuntimeRecoveryRequired = false
	run.Error = ""
	run.Summary = trimForTriggerSnippet(view.Content, 8*1024)
	run.FinishedAt = view.FinishedAt
	run.Status = string(view.Phase)
	if view.Queued {
		run.Status = "queued"
	}
	if view.Outcome != nil {
		switch view.Outcome.Status {
		case agentrun.OutcomeCompleted:
			run.Status = automation.RunStatusSuccess
		case agentrun.OutcomeAborted, agentrun.OutcomePreempted:
			run.Status = automation.RunStatusAborted
		case agentrun.OutcomeFailed:
			run.Status = automation.RunStatusFailed
			run.Error = view.Outcome.Reason
		case agentrun.OutcomeSuspended:
			run.Status = string(agentrun.RunPhaseSuspended)
		}
	}
	return run
}

func (s *Service) projectRun(ctx context.Context, snap *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) automation.RunRecord {
	view, found, err := commandProjection(ctx, snap, task, run)
	if err == nil && found {
		return projectRunView(run, view)
	}
	if err != nil {
		slog.WarnContext(ctx, "[automation] Project Agent result unavailable", "run_id", run.ID, "err", err)
	}
	if run.DeliveryStatus != "" {
		run.Status = run.DeliveryStatus
		run.Error = ""
	}
	return run
}

func (s *Service) projectTasks(tasks []automation.Task, err error) ([]automation.Task, error) {
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		task := &tasks[i]
		runtime, err := s.host.RuntimeForTarget(context.Background(), task.Target)
		if err != nil {
			slog.Warn("[automation] Project Agent runtime unavailable", "project_id", task.Target.ProjectID, "err", err)
			for j := range task.RecentRuns {
				if task.RecentRuns[j].DeliveryStatus != "" {
					task.RecentRuns[j].Status = task.RecentRuns[j].DeliveryStatus
				}
			}
			if task.LastRun != nil && task.LastRun.DeliveryStatus != "" {
				run := *task.LastRun
				run.Status = run.DeliveryStatus
				task.LastRun = &run
			}
			continue
		}
		snap := snapshotFromRuntime(runtime)
		for j := range task.RecentRuns {
			task.RecentRuns[j] = s.projectRun(context.Background(), snap, *task, task.RecentRuns[j])
		}
		if task.LastRun != nil {
			run := s.projectRun(context.Background(), snap, *task, *task.LastRun)
			task.LastRun = &run
		}
	}
	return tasks, nil
}

func (s *Service) ActiveAutomationRuns() []automation.ActiveRun {
	tasks, err := s.List()
	if err != nil {
		return nil
	}
	var active []automation.ActiveRun
	for _, task := range tasks {
		for _, run := range task.RecentRuns {
			if run.Status == automation.RunStatusRunning {
				active = append(active, automation.ActiveRun{Run: run, TaskID: automationTaskStoreID(task)})
			}
		}
	}
	return active
}

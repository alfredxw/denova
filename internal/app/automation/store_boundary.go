package automationapp

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"denova/config"
	"denova/internal/automation"
	"denova/internal/book"
)

func (s *Service) RunDue(ctx context.Context, now time.Time) []automation.RunResult {
	tasks, err := s.storeAllWorkspaces().List()
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[automation] list scheduler targets failed err=%v", err))
		return nil
	}
	targets := map[string]automation.ExecutionTarget{}
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		key := task.Target.Kind + "\x00" + canonicalAutomationWorkspace(task.Target.Workspace)
		targets[key] = task.Target
	}
	results := []automation.RunResult{}
	for _, target := range targets {
		snap, operation, targetErr := s.acquireTargetRuntime(ctx, target)
		if targetErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[automation] resolve scheduler target failed kind=%s workspace=%q err=%v", target.Kind, target.Workspace, targetErr))
			continue
		}
		results = append(results, s.runDueWithSnapshot(operation.Context(), snap, now)...)
		operation.Release()
	}
	return results
}

func (s *Service) runDueWithSnapshot(ctx context.Context, snap *automationWorkspaceSnapshot, now time.Time) []automation.RunResult {
	_, results, err := s.processTriggers(ctx, snap, "", now.UTC(), "scheduler")
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[automation] process due triggers failed err=%v", err))
		return nil
	}
	return results
}

// storeAllWorkspaces builds a user catalog over registered Projects. Project
// state roots own persistence; workspace paths remain execution targets only.
func (s *Service) storeAllWorkspaces() *automation.Store {
	if s == nil || s.host == nil {
		return automation.NewStore("", "")
	}
	catalog, err := s.host.Catalog()
	store := automation.NewStore(catalog.DataDir, catalog.CurrentWorkspace)
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[automation] resolve Project catalog failed err=%v", err))
		return store
	}
	return store.WithProjects(catalog.Projects...)
}

// storeForSnapshot builds a store scoped to the snapshot's workspace.
func storeForSnapshot(snap *automationWorkspaceSnapshot) *automation.Store {
	if snap == nil {
		return automation.NewStore("", "")
	}
	if strings.TrimSpace(snap.projectID) != "" && strings.TrimSpace(snap.stateRoot) != "" {
		return automation.NewProjectStore(snap.novaDir, snap.projectID, snap.workspace, snap.stateRoot)
	}
	return automation.NewStore(snap.novaDir, snap.workspace)
}

// automationTaskStoreID is the unambiguous persistence locator. RunRecord
// keeps the local task ID for API compatibility, while every store mutation
// uses CatalogID so equal imported IDs in different scopes/workspaces cannot
// resolve to the wrong definition.
func automationTaskStoreID(task automation.Task) string {
	if catalogID := strings.TrimSpace(task.CatalogID); catalogID != "" {
		return catalogID
	}
	return strings.TrimSpace(task.ID)
}

// automationTargetForRun resolves the durable Agent binding. User-scoped
// definitions may execute against a specific workspace; in that case the run
// workspace, not the definition's global target, owns recovery and control.
func automationTargetForRun(task automation.Task, run automation.RunRecord) automation.ExecutionTarget {
	if workspace := strings.TrimSpace(run.Workspace); workspace != "" {
		return automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, WorkspaceID: run.ProjectID, Workspace: workspace}
	}
	if strings.TrimSpace(task.Target.Kind) != "" {
		return task.Target
	}
	return automation.ExecutionTarget{Kind: automation.TargetKindUser}
}

func (s *Service) newRunRecord(snap *automationWorkspaceSnapshot, task automation.Task, trigger string) automation.RunRecord {
	run := automation.RunRecord{
		ID:        automation.NewRunID(),
		TaskID:    task.ID,
		ProjectID: snap.projectID,
		Scope:     task.Scope,
		Workspace: snap.workspace,
		Trigger:   normalizeAutomationTrigger(trigger),
		Status:    automation.RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	run.SessionID = automationRunSessionID(run.ID)
	return run
}

func (s *Service) newRunConversation(snap *automationWorkspaceSnapshot, run automation.RunRecord, task automation.Task) (*automationRunConversation, error) {
	store := snap.sessionStore
	if store == nil {
		return nil, ErrNoWorkspace
	}
	runtimeCfg := runtimeConfigForTask(snap, task)
	sess, _, err := agentconversation.GetOrCreateSession(store, run.SessionID, &runtimeCfg, config.AgentKindAutomation)
	if err != nil {
		return nil, err
	}
	if _, err := agentconversation.ApplySession(sess, &runtimeCfg, config.AgentKindAutomation); err != nil {
		return nil, err
	}
	title := fmt.Sprintf("%s · %s · %s", strings.TrimSpace(task.Name), run.Trigger, run.StartedAt.Local().Format(book.DisplayTimeFormat))
	if strings.TrimSpace(task.Name) == "" {
		title = fmt.Sprintf("Automation · %s · %s", run.Trigger, run.StartedAt.Local().Format(book.DisplayTimeFormat))
	}
	if err := sess.Rename(title); err != nil {
		return nil, err
	}
	return &automationRunConversation{
		base:          agentconversation.NewSessionConversationForAgent(sess, &runtimeCfg, config.AgentKindAutomation),
		runtimeConfig: runtimeCfg,
	}, nil
}

func automationRunSessionID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = automation.NewRunID()
	}
	return "automation-run-" + runID
}

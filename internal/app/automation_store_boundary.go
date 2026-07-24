package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	"denova/internal/book"
)

func (a *App) RunDueAutomations(ctx context.Context, now time.Time) []automation.RunResult {
	return a.automation().RunDue(ctx, now)
}

func (s *AutomationAppService) RunDue(ctx context.Context, now time.Time) []automation.RunResult {
	tasks, err := s.storeAllWorkspaces().List()
	if err != nil {
		log.Printf("[automation] list scheduler targets failed err=%v", err)
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
			log.Printf("[automation] resolve scheduler target failed kind=%s workspace=%q err=%v", target.Kind, target.Workspace, targetErr)
			continue
		}
		results = append(results, s.runDueWithSnapshot(operation.Context(), snap, now)...)
		operation.Release()
	}
	return results
}

func (s *AutomationAppService) runDueWithSnapshot(ctx context.Context, snap *automationWorkspaceSnapshot, now time.Time) []automation.RunResult {
	_, results, err := s.processTriggers(ctx, snap, "", now.UTC(), "scheduler")
	if err != nil {
		log.Printf("[automation] process due triggers failed err=%v", err)
		return nil
	}
	return results
}

// storeAllWorkspaces builds a store that includes all known workspaces (from
// the book registry plus the current workspace). Used by CRUD operations that
// need visibility across all books.
func (s *AutomationAppService) storeAllWorkspaces() *automation.Store {
	a := s.app
	a.mu.RLock()
	novaDir := ""
	if a.cfg != nil {
		novaDir = a.cfg.DataDir()
	}
	workspace := a.workspace
	registry := a.bookRegistry
	a.mu.RUnlock()
	store := automation.NewStore(novaDir, workspace)
	if registry == nil {
		return store
	}
	books := registry.List()
	workspaces := make([]string, 0, len(books)+1)
	for _, book := range books {
		workspaces = append(workspaces, book.Path)
	}
	if strings.TrimSpace(workspace) != "" {
		workspaces = append(workspaces, workspace)
	}
	return store.WithWorkspaces(workspaces...)
}

// storeForSnapshot builds a store scoped to the snapshot's workspace.
func storeForSnapshot(snap *automationWorkspaceSnapshot) *automation.Store {
	if snap == nil {
		return automation.NewStore("", "")
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
		return automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, Workspace: workspace}
	}
	if strings.TrimSpace(task.Target.Kind) != "" {
		return task.Target
	}
	return automation.ExecutionTarget{Kind: automation.TargetKindUser}
}

func (s *AutomationAppService) newRunRecord(snap *automationWorkspaceSnapshot, task automation.Task, trigger string) automation.RunRecord {
	run := automation.RunRecord{
		ID:        automation.NewRunID(),
		TaskID:    task.ID,
		Scope:     task.Scope,
		Workspace: snap.workspace,
		Trigger:   normalizeAutomationTrigger(trigger),
		Status:    automation.RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	run.SessionID = automationRunSessionID(run.ID)
	return run
}

func (s *AutomationAppService) newRunConversation(snap *automationWorkspaceSnapshot, run automation.RunRecord, task automation.Task) (*automationRunConversation, error) {
	store := snap.sessionStore
	cfg := snap.cfg
	if store == nil {
		return nil, ErrNoWorkspace
	}
	sess, err := store.GetOrCreate(run.SessionID)
	if err != nil {
		return nil, err
	}
	title := fmt.Sprintf("%s · %s · %s", strings.TrimSpace(task.Name), run.Trigger, run.StartedAt.Local().Format(book.DisplayTimeFormat))
	if strings.TrimSpace(task.Name) == "" {
		title = fmt.Sprintf("Automation · %s · %s", run.Trigger, run.StartedAt.Local().Format(book.DisplayTimeFormat))
	}
	if err := sess.Rename(title); err != nil {
		return nil, err
	}
	return &automationRunConversation{base: agents.NewSessionConversationForAgent(sess, &cfg, config.AgentKindAutomation)}, nil
}

func automationRunSessionID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = automation.NewRunID()
	}
	return "automation-run-" + runID
}

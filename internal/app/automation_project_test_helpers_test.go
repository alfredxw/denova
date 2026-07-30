package app

import (
	"os"
	"testing"

	"denova/internal/automation"
	projectdomain "denova/internal/project"
)

func registerAutomationProjectForTest(t *testing.T, application *App, workspace string) *automation.Store {
	t.Helper()
	layout := registerBookProjectForTest(t, application, workspace)
	return automation.NewProjectStore(application.cfg.DataDir(), layout.ProjectID, layout.ContentRoot, layout.StateRoot)
}

func registerBookProjectForTest(t *testing.T, application *App, workspace string) ProjectLayout {
	t.Helper()
	if application == nil || application.cfg == nil {
		t.Fatal("application config is unavailable")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewBookRegistry(application.cfg.DataDir())
	record, err := registry.ProjectRegistry().EnsureBook(workspace)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.ProjectRegistry().EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	application.bookRegistry = registry
	application.projectRegistry = registry.ProjectRegistry()
	application.cfg.ProjectID = record.ID
	application.cfg.ProjectStateDir = layout.StateRoot
	return layout
}

// projectAutomationStoreForTest mirrors the production Project persistence
// boundary without forcing integration tests through unrelated API handlers.
func projectAutomationStoreForTest(
	t *testing.T,
	novaDir string,
	registry *projectdomain.Registry,
	workspace string,
) *automation.Store {
	t.Helper()
	record, found, err := registry.FindByPath(workspace, false)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("Project is not registered for workspace %q", workspace)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	return automation.NewProjectStore(novaDir, record.ID, record.WorkspacePath, layout.StateRoot)
}

func appAutomationStoreForTest(t *testing.T, application *App, workspace string) *automation.Store {
	t.Helper()
	if application == nil || application.projectRegistry == nil {
		t.Fatal("application Project registry is unavailable")
	}
	novaDir := ""
	if application.cfg != nil {
		novaDir = application.cfg.DataDir()
	}
	return projectAutomationStoreForTest(t, novaDir, application.projectRegistry, workspace)
}

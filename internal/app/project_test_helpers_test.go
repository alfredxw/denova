package app

import (
	"os"
	"testing"

	projectdomain "denova/internal/project"
)

func registerBookProjectForTest(t *testing.T, application *App, workspace string) ProjectLayout {
	t.Helper()
	if application == nil || application.cfg == nil {
		t.Fatal("application config is unavailable")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(application.cfg.DataDir())
	record, err := registry.EnsureBook(workspace)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	application.projectRegistry = registry
	application.cfg.ProjectID = record.ID
	application.cfg.ProjectStateDir = layout.StateRoot
	return layout
}

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/skills"
	"denova/internal/project"
)

func TestSkillUpdateScanDoesNotMigrateInactiveProjects(t *testing.T) {
	data, workspace := t.TempDir(), t.TempDir()
	registry := project.NewRegistry(data)
	record, err := registry.Add(workspace, project.TypeGeneral, "Inactive project")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.Layout(record)
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(workspace, ".nova", "skills", "legacy", skills.SkillFileName)
	content := skills.DefaultContent("legacy", "Local Skill")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	application := &App{cfg: &config.Config{DenovaDir: data}, projectRegistry: registry}
	application.ResourceExchange().UpdateDue(context.Background(), time.Now(), application.ApplyResourcePlan)
	for _, path := range []string{filepath.Join(workspace, "skills"), layout.StoreRoot} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("background scan created an unrelated directory %s: %v", path, err)
		}
	}
	if got, err := os.ReadFile(legacy); err != nil || string(got) != content {
		t.Fatalf("background scan changed legacy content: %v", err)
	}
}

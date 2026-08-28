package harnessstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
)

func TestLoadRejectsInvalidUserContributionWithoutFailingAgentBuild(t *testing.T) {
	dataDir := t.TempDir()
	stateRoot := filepath.Join(dataDir, stateDirectoryName)
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, toolsFilePath), []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateRoot, "prompts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "prompts", "general.md"), []byte("must not apply partially"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DenovaDir: dataDir, Labs: config.ResolvedLabs{DeveloperMode: true, HarnessStateEnabled: true}}

	harness, err := Load(context.Background(), cfg)
	if err != nil {
		t.Fatalf("invalid user State should not fail the base Agent build: %v", err)
	}
	if got := harness.Prompt(config.AgentKindGeneral); got != "" {
		t.Fatalf("invalid user contribution was partially applied: %q", got)
	}
	manager, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := manager.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Diagnostics) == 0 {
		t.Fatal("management inspection should preserve diagnostics for the invalid live State")
	}
}

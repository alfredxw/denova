package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/presetlayout"
)

func TestAppStartupMigratesPresetLayout(t *testing.T) {
	dataRoot := t.TempDir()
	writePresetLayoutTestFile(t, filepath.Join(dataRoot, "story-tellers", "custom.json"), "startup")

	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", DenovaDir: dataRoot, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	assertPresetLayoutTestFile(t, filepath.Join(presetlayout.NarrativeStyles(dataRoot), "custom.json"), "startup")
	assertPresetLayoutTestFile(t, filepath.Join(dataRoot, "backups", presetLayoutBackupDirectory, "story-tellers", "custom.json"), "startup")
}

func TestPresetLayoutMigrationMovesCatalogsAfterPreservingOriginals(t *testing.T) {
	dataRoot := t.TempDir()
	files := []struct {
		oldRelative    string
		destination    string
		backupRelative string
		content        string
	}{
		{"story-tellers/custom.json", filepath.Join(presetlayout.NarrativeStyles(dataRoot), "custom.json"), "story-tellers/custom.json", "narrative"},
		{"image-presets/custom.json", filepath.Join(presetlayout.Image(dataRoot), "custom.json"), "image-presets/custom.json", "image"},
		{"game-planning-templates/custom.json", filepath.Join(presetlayout.GamePlanning(dataRoot), "custom.json"), "game-planning-templates/custom.json", "planning"},
		{"story-director-modules/event-packages/custom.json", filepath.Join(presetlayout.EventPackages(dataRoot), "custom.json"), "story-director-modules/event-packages/custom.json", "event"},
		{"story-director-modules/rule-systems/custom.json", filepath.Join(presetlayout.RuleSystems(dataRoot), "custom.json"), "story-director-modules/rule-systems/custom.json", "rule"},
		{"story-director-modules/actor-states/custom.json", filepath.Join(presetlayout.ActorStates(dataRoot), "custom.json"), "story-director-modules/actor-states/custom.json", "state"},
		{"story-directors/custom.json", filepath.Join(presetlayout.LegacyGamePresets(dataRoot), "custom.json"), "story-directors/custom.json", "legacy"},
	}
	for _, file := range files {
		writePresetLayoutTestFile(t, filepath.Join(dataRoot, filepath.FromSlash(file.oldRelative)), file.content)
	}

	if err := migratePresetLayout(dataRoot); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		assertPresetLayoutTestFile(t, file.destination, file.content)
		assertPresetLayoutTestFile(t, filepath.Join(dataRoot, "backups", presetLayoutBackupDirectory, filepath.FromSlash(file.backupRelative)), file.content)
	}
	for _, oldDirectory := range []string{"story-tellers", "image-presets", "game-planning-templates", "story-director-modules", "story-directors"} {
		if _, err := os.Stat(filepath.Join(dataRoot, oldDirectory)); !os.IsNotExist(err) {
			t.Fatalf("old preset directory still exists: %s (err=%v)", oldDirectory, err)
		}
	}

	receiptBefore, err := os.ReadFile(presetLayoutMigrationReceiptPath(dataRoot))
	if err != nil {
		t.Fatal(err)
	}
	var receipt presetLayoutMigrationReceipt
	if err := json.Unmarshal(receiptBefore, &receipt); err != nil {
		t.Fatal(err)
	}
	wantBackups := []string{
		"backups/preset-layout-v1/story-tellers",
		"backups/preset-layout-v1/image-presets",
		"backups/preset-layout-v1/game-planning-templates",
		"backups/preset-layout-v1/story-director-modules",
		"backups/preset-layout-v1/story-directors",
	}
	if receipt.Version != presetLayoutMigrationVersion || receipt.CompletedAt.IsZero() || !slices.Equal(receipt.Backups, wantBackups) {
		t.Fatalf("migration receipt = %#v", receipt)
	}
	if err := migratePresetLayout(dataRoot); err != nil {
		t.Fatalf("retry completed migration: %v", err)
	}
	receiptAfter, err := os.ReadFile(presetLayoutMigrationReceiptPath(dataRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("completed migration rewrote its receipt")
	}
}

func TestPresetLayoutMigrationResumesAfterSourceWasPreserved(t *testing.T) {
	dataRoot := t.TempDir()
	source := filepath.Join(dataRoot, "story-tellers")
	writePresetLayoutTestFile(t, filepath.Join(source, "custom.json"), "resume")
	backup := filepath.Join(dataRoot, "backups", presetLayoutBackupDirectory, "story-tellers")
	if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, backup); err != nil {
		t.Fatal(err)
	}

	if err := migratePresetLayout(dataRoot); err != nil {
		t.Fatal(err)
	}
	assertPresetLayoutTestFile(t, filepath.Join(presetlayout.NarrativeStyles(dataRoot), "custom.json"), "resume")
	assertPresetLayoutTestFile(t, filepath.Join(backup, "custom.json"), "resume")
}

func TestPresetLayoutMigrationRejectsSourceDestinationConflict(t *testing.T) {
	dataRoot := t.TempDir()
	source := filepath.Join(dataRoot, "image-presets", "custom.json")
	destination := filepath.Join(presetlayout.Image(dataRoot), "custom.json")
	writePresetLayoutTestFile(t, source, "source")
	writePresetLayoutTestFile(t, destination, "destination")

	err := migratePresetLayout(dataRoot)
	if err == nil || !strings.Contains(err.Error(), "both source and destination") {
		t.Fatalf("conflict error = %v", err)
	}
	assertPresetLayoutTestFile(t, source, "source")
	assertPresetLayoutTestFile(t, destination, "destination")
	if _, err := os.Stat(presetLayoutMigrationReceiptPath(dataRoot)); !os.IsNotExist(err) {
		t.Fatalf("conflicting migration wrote a receipt: %v", err)
	}
}

func writePresetLayoutTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPresetLayoutTestFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content at %s = %q, want %q", path, content, want)
	}
}

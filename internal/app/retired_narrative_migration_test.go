package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/presetlayout"
)

func TestAppStartupRetiresReleasedNarrativeStyleWithBackup(t *testing.T) {
	raw, err := os.ReadFile("testdata/direct-erotica-v0.4.4.json")
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := t.TempDir()
	path := filepath.Join(presetlayout.NarrativeStyles(dataRoot), "direct-erotica.json")
	writePresetLayoutTestFile(t, path, string(raw))
	for range 2 {
		application, err := New(context.Background(), &config.Config{
			OpenAIModel: "test-model", DenovaDir: dataRoot, ResumeLastWorkspace: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		application.Close()
		items, err := teller.NewLibrary(dataRoot).List()
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ID == "direct-erotica" {
				t.Fatal("retired default must not reappear as a custom style")
			}
		}
		backup, err := os.ReadFile(filepath.Join(dataRoot, "backups", "default-prompts-v0.4.4", "direct-erotica.json"))
		if err != nil || !bytes.Equal(backup, raw) {
			t.Fatalf("expected exact released preset backup: %v", err)
		}
		for _, load := range []func(string, string) teller.Definition{interactiveapp.LoadWritingTeller, interactiveapp.LoadGameTeller} {
			if got := load(dataRoot, "direct-erotica"); got.ID != "rhythm" {
				t.Fatalf("retired selection must fall back for Writing and Game, got %q", got.ID)
			}
		}
	}
}

func TestAppStartupPreservesEditedRetiredNarrativeStyle(t *testing.T) {
	raw, err := os.ReadFile("testdata/direct-erotica-v0.4.4.json")
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if err := json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	original["name"] = "My custom narrative style"
	// Direct file edits do not necessarily set builtin_overridden.
	custom, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := t.TempDir()
	path := filepath.Join(presetlayout.NarrativeStyles(dataRoot), "direct-erotica.json")
	writePresetLayoutTestFile(t, path, string(custom))
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", DenovaDir: dataRoot, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, custom) {
		t.Fatalf("custom preset must be preserved byte-for-byte: %v", err)
	}
	item, err := teller.NewLibrary(dataRoot).Get("direct-erotica")
	if err != nil || !item.Custom || item.Name != "My custom narrative style" {
		t.Fatalf("expected retained user-owned style: id=%q custom=%v name=%q err=%v", item.ID, item.Custom, item.Name, err)
	}
}

func TestRetiredNarrativeMigrationPreservesOverridesAndExtensions(t *testing.T) {
	raw, err := os.ReadFile("testdata/direct-erotica-v0.4.4.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"builtin_overridden", "custom_extension"} {
		t.Run(field, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			document[field] = true
			custom, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			dataRoot := t.TempDir()
			path := filepath.Join(presetlayout.NarrativeStyles(dataRoot), "direct-erotica.json")
			writePresetLayoutTestFile(t, path, string(custom))
			if err := migrateRetiredNarrativeStyle(dataRoot); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, custom) {
				t.Fatalf("user-owned document must remain intact: %v", err)
			}
		})
	}
}

func TestRetiredNarrativeMigrationBackupConflictAndRetry(t *testing.T) {
	raw, err := os.ReadFile("testdata/direct-erotica-v0.4.4.json")
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	raw = compact.Bytes()
	dataRoot := t.TempDir()
	path := filepath.Join(presetlayout.NarrativeStyles(dataRoot), "direct-erotica.json")
	backup := filepath.Join(dataRoot, "backups", "default-prompts-v0.4.4", "direct-erotica.json")
	writePresetLayoutTestFile(t, path, string(raw))
	writePresetLayoutTestFile(t, backup, "Existing backup")
	if err := migrateRetiredNarrativeStyle(dataRoot); err == nil {
		t.Fatal("expected conflicting backup to prevent retirement")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("original must survive failed backup: %v", err)
	}
	writePresetLayoutTestFile(t, backup, string(raw))
	if err := migrateRetiredNarrativeStyle(dataRoot); err != nil {
		t.Fatalf("retry with existing original backup failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("retired preset should leave the active catalog: %v", err)
	}
	got, err = os.ReadFile(backup)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("formatted original must be preserved exactly: %v", err)
	}
}

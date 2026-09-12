package book

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInitWorkspaceMigratesReleasedCreatorWithBackup(t *testing.T) {
	legacy, err := os.ReadFile("testdata/creator-v0.4.4.md")
	if err != nil {
		t.Fatal(err)
	}
	legacy = bytes.ReplaceAll(legacy, []byte("\r\n"), []byte("\n"))
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(newline, func(t *testing.T) {
			workspace := t.TempDir()
			original := bytes.ReplaceAll(legacy, []byte("\n"), []byte(newline))
			path := filepath.Join(workspace, CreatorFileName)
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := NewState(workspace).InitWorkspace(); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || bytes.Equal(got, original) || string(got) != CreatorTemplate {
					t.Fatalf("expected current template after migration: err=%v", err)
				}
				backup, err := os.ReadFile(path + ".v0.4.4.bak")
				if err != nil || !bytes.Equal(backup, original) {
					t.Fatalf("expected exact original backup: err=%v", err)
				}
			}
		})
	}
}

func TestInitWorkspacePreservesCustomizedCreator(t *testing.T) {
	legacy, err := os.ReadFile("testdata/creator-v0.4.4.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range [][]byte{[]byte("# Creative instructions\nUse first-person narration.\n"), append(legacy, []byte("\nMy custom rule.\n")...), {}} {
		workspace := t.TempDir()
		path := filepath.Join(workspace, CreatorFileName)
		if err := os.WriteFile(path, original, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := NewState(workspace).InitWorkspace(); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("custom creator was changed: err=%v", err)
		}
		if _, err := os.Stat(path + ".v0.4.4.bak"); !os.IsNotExist(err) {
			t.Fatalf("custom creator must not create a migration backup: %v", err)
		}
	}
}

func TestCreatorMigrationRequiresOriginalBackupAndRetries(t *testing.T) {
	legacy, err := os.ReadFile("testdata/creator-v0.4.4.md")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, CreatorFileName)
	backup := path + ".v0.4.4.bak"
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("Existing backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCreatorTemplate(workspace); err == nil {
		t.Fatal("expected conflicting backup to prevent migration")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("original must survive failed backup: %v", err)
	}
	if err := os.WriteFile(backup, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCreatorTemplate(workspace); err != nil {
		t.Fatalf("retry with an already preserved original failed: %v", err)
	}
}

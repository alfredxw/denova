package resourceexchange

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/agents/skills"
)

func TestLegacySkillMigrationRetainsConsentAndModifiedState(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "automatic"}[automatic], func(t *testing.T) {
			s := testService(t)
			ctx := context.Background()
			directory := filepath.Join(s.root, "skills", "legacy")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			raw := []byte(skills.DefaultContent("legacy", "Legacy Skill"))
			if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), raw, 0644); err != nil {
				t.Fatal(err)
			}
			// This is deliberately not a current baseline. Migration must never grant
			// automatic replacement simply by hashing whatever is present now.
			state := skills.RemoteState{Source: skills.RemoteArchiveSource{URL: "https://github.com/author/repository", Ref: "main"}, SourcePath: "skills/legacy", Digest: "old-baseline", AutoUpdate: automatic}
			original := jsonBytes(t, state)
			if err := os.WriteFile(filepath.Join(directory, ".denova-source.json"), original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.MigrateSources(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.MigrateSources(ctx); err != nil {
				t.Fatal(err)
			}
			items, err := s.Installations(ctx)
			if err != nil || len(items) != 1 {
				t.Fatalf("installations: %v %v", items, err)
			}
			item := items[0]
			mode := "manual"
			if automatic {
				mode = "auto_apply"
			}
			if item.UpdateMode != mode || item.LocalState != "modified" || item.Source.Path != "skills/legacy" {
				t.Fatalf("migration changed trust: %#v", item)
			}
			if _, err := os.Stat(filepath.Join(directory, ".denova-source.json")); !os.IsNotExist(err) {
				t.Fatal("legacy source still writable")
			}
			content, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
			if err != nil || string(content) != string(raw) {
				t.Fatal("migration changed Skill content")
			}
			entries, err := os.ReadDir(filepath.Join(s.root, "resource-exchange", "transactions"))
			if err != nil || len(entries) != 1 {
				t.Fatalf("migration backup: %v %v", entries, err)
			}
		})
	}
}

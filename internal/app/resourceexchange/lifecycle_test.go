package resourceexchange

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"denova/internal/agents/skills"
	"denova/internal/platform"
	"denova/internal/revisionfile"
)

func TestRestoreIsAtomicAndDoesNotOverwriteLaterEdits(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	preview, candidate := previewFixture(t, s)
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: []string{"skill"}})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := s.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	backups, err := s.Backups(ctx, installed.ID)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %+v %v", backups, err)
	}
	raw, err := s.DownloadBackup(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := platform.ArchiveFiles(raw); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.root, "skills", "fixture", "SKILL.md")
	before, _ := os.ReadFile(file)
	if err := os.WriteFile(file, []byte("local draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlanRestore(ctx, plan.ID); !errors.Is(err, ErrLocalModified) {
		t.Fatalf("accepted later edit: %v", err)
	}
	if err := os.WriteFile(file, before, 0600); err != nil {
		t.Fatal(err)
	}
	restore, err := s.PlanRestore(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, restore.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("new Skill not removed: %v", err)
	}
	if items, err := s.Installations(ctx); err != nil || len(items) != 0 {
		t.Fatalf("source record not restored: %+v %v", items, err)
	}
}

func TestExportDefinitionFrozenSnapshotAndStableResourceIDs(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	preview, candidate := previewFixture(t, s)
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: []string{"skill"}})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := s.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs := []LocalRef{}
	for _, binding := range installed.Bindings {
		refs = append(refs, binding.Local)
	}
	request := ExportRequest{InstallationID: installed.ID, Package: installed.Package, Resources: refs}
	definition, err := s.SaveExportDefinition(ctx, ExportDefinition{ExportRequest: request})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveExportDefinition(ctx, ExportDefinition{ExportRequest: request}); !errors.Is(err, revisionfile.ErrRevisionConflict) {
		t.Fatalf("overwrote definition: %v", err)
	}
	exported, err := s.PrepareExport(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range installed.Bindings {
		if !slices.ContainsFunc(exported.Resources, func(resource Resource) bool { return resource.ID == binding.ResourceID }) {
			t.Fatal("source resource identity changed")
		}
	}
	before, err := s.ReadExport(ctx, exported.ID)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.root, "skills", "fixture", "SKILL.md")
	if err := os.WriteFile(file, []byte(skills.DefaultContent("fixture", "Edited later")), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadExport(ctx, exported.ID)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("reviewed export changed with local edits", err)
	}
	if err := s.DeleteExportDefinition(ctx, definition.Package.ID, definition.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("deleting selection removed source", err)
	}
}

func updateFixture(t *testing.T, s *Service, text string, withImage bool) Preview {
	t.Helper()
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "auto-pack", Name: "Automatic fixture"}, Resources: []Resource{{ID: "skill", Kind: "skill", Path: "skill"}}}
	files := map[string][]byte{"skill/SKILL.md": []byte(skills.DefaultContent("automatic", text))}
	if withImage {
		manifest.Resources = append(manifest.Resources, Resource{ID: "image", Kind: "preset.image", Path: "image.json"})
		files["image.json"] = []byte(`{"name":"Image","prompt":"Draw"}`)
	}
	files["denova-pack.json"] = jsonBytes(t, manifest)
	preview, err := s.previewFiles(context.Background(), Source{Kind: "github", URL: "https://github.com/author/package", Ref: "main"}, files)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func TestAutomaticUpdateReusesPendingPreviewAndPreservesLocalWork(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	preview := updateFixture(t, s, "First", true)
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: []string{"skill"}, UpdateMode: "auto_apply"})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := s.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	next := updateFixture(t, s, "Second", true)
	item, snapshot, err := s.loadInstallation(ctx, installed.ID)
	if err != nil {
		t.Fatal(err)
	}
	item.CheckedAt = time.Now().UTC()
	item.RemoteState = "update_available"
	item.PendingPreviewID = next.ID
	file, _ := resolveTarget(s.root, s.registry, installationTarget(item.ID))
	if _, err := revisionfile.ReplaceIfRevision(ctx, file, snapshot.Revision, jsonBytes(t, item), revisionfile.Options{}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.UpdateDue(ctx, time.Now(), func(context.Context, string) (Installation, error) { calls++; return Installation{}, ErrResourcesBusy })
	if calls != 1 {
		t.Fatalf("pending update not attempted: %d", calls)
	}
	if _, _, err := s.loadPreview(next.ID); err != nil {
		t.Fatal("busy update discarded frozen bytes", err)
	}
	// Restart preserves the pending preview and retries without another download.
	s = New(s.root, s.registry, s.catalog, platform.New(s.root, s.registry))
	s.UpdateDue(ctx, time.Now(), s.Apply)
	item, _, err = s.loadInstallation(ctx, installed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Bindings) != 1 || item.Bindings[0].SourceDigest != next.Candidates[0].Resources[0].Digest {
		t.Fatalf("update broadened selection or failed: %+v", item)
	}
	local := filepath.Join(s.root, "skills", "automatic", "local.txt")
	if err := os.WriteFile(local, []byte("Keep this"), 0600); err != nil {
		t.Fatal(err)
	}
	third := updateFixture(t, s, "Third", true)
	_, err = s.Plan(ctx, PlanRequest{PreviewID: third.ID, CandidateID: third.Candidates[0].ID, Resources: []string{"skill"}, InstallationID: installed.ID})
	if !errors.Is(err, ErrLocalModified) {
		t.Fatalf("untracked local file was not protected: %v", err)
	}
	if err := s.SetUpdateMode(ctx, installed.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	s.UpdateDue(ctx, time.Now().Add(48*time.Hour), func(context.Context, string) (Installation, error) {
		t.Fatal("manual subscription updated automatically")
		return Installation{}, nil
	})
}

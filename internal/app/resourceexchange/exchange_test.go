package resourceexchange

import (
	"bytes"
	"context"
	"denova/internal/book/lore"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"denova/internal/agents/skills"
	"denova/internal/app/resourcecatalog"
	"denova/internal/platform"
	"denova/internal/project"
	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

type skillSource struct{ root string }

func (s skillSource) SkillDirectories(resourcecatalog.SkillTarget) ([]skills.Directory, error) {
	return []skills.Directory{{Scope: skills.ScopeUser, Path: filepath.Join(s.root, "skills"), Writable: true}}, nil
}
func testService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	registry := project.NewRegistry(root)
	return New(root, registry, resourcecatalog.NewService(root, skillSource{root}), platform.New(root, registry))
}
func jsonBytes(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func previewFixture(t *testing.T, s *Service) (Preview, PackagePreview) {
	t.Helper()
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "mixed", Name: "Mixed"}, Resources: []Resource{{ID: "image", Kind: "preset.image", Path: "image.json"}, {ID: "skill", Kind: "skill", Path: "skill", Requires: []string{"image"}}}}
	files := map[string][]byte{"denova-pack.json": jsonBytes(t, manifest), "image.json": []byte(`{"name":"Fixture","description":"Image","prompt":"Draw a tree"}`), "skill/SKILL.md": []byte("---\nname: fixture\ndescription: A fixture\n---\nRun the fixture.\n")}
	preview, err := s.previewFiles(context.Background(), Source{Kind: "file", Filename: "fixture.zip"}, files)
	if err != nil {
		t.Fatal(err)
	}
	return preview, preview.Candidates[0]
}
func TestMixedInstallRestartExportAndCAS(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	preview, candidate := previewFixture(t, s)
	request := PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: []string{"skill"}}
	plan, err := s.Plan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("dependencies not included: %#v", plan.Items)
	}
	// Reload all application state before applying the persisted plan.
	s = New(s.root, s.registry, s.catalog, platform.New(s.root, s.registry))
	installed, err := s.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Apply(ctx, plan.ID)
	if err != nil || again.ID != installed.ID {
		t.Fatalf("retry: %v", err)
	}
	image := slices.IndexFunc(installed.Bindings, func(binding Binding) bool { return binding.Local.Kind == "preset.image" })
	definition, err := s.catalog.ImagePreset(installed.Bindings[image].Local.ID)
	if err != nil || len(definition.Slots) != 1 || definition.Slots[0].Content != "Draw a tree" {
		t.Fatalf("image: %#v %v", definition, err)
	}
	document, err := s.catalog.SkillDocument(ctx, resourcecatalog.GlobalSkills(), skills.ScopeUser, "fixture")
	if err != nil || !strings.Contains(document.Content, "Run the fixture") {
		t.Fatalf("skill: %#v %v", document, err)
	}
	refs := []LocalRef{}
	for _, binding := range installed.Bindings {
		refs = append(refs, binding.Local)
	}
	raw, err := s.Export(ctx, ExportRequest{Package: installed.Package, Resources: refs})
	if err != nil {
		t.Fatal(err)
	}
	next := testService(t)
	copy, err := next.Preview(ctx, Source{Kind: "file", Filename: "copy.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, resource := range copy.Candidates[0].Resources {
		ids = append(ids, resource.ID)
	}
	nextPlan, err := next.Plan(ctx, PlanRequest{PreviewID: copy.ID, CandidateID: copy.Candidates[0].ID, Resources: ids})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Apply(ctx, nextPlan.ID); err != nil {
		t.Fatal(err)
	}
	// Concurrent writes invalidate the whole batch, including its installation row.
	third := testService(t)
	p, c := previewFixture(t, third)
	stale, err := third.Plan(ctx, PlanRequest{PreviewID: p.ID, CandidateID: c.ID, Resources: []string{"skill"}})
	if err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(third.root, "skills", "fixture", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("local draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := third.Apply(ctx, stale.ID); err == nil {
		t.Fatal("stale plan committed")
	}
	rows, err := third.Installations(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("partial installation: %v %v", rows, err)
	}
	for _, change := range stale.Changes {
		if change.Target.Path == "skills/fixture/SKILL.md" {
			continue
		}
		snapshot, err := third.snapshot(ctx, change.Target)
		if err != nil || snapshot.Exists {
			t.Fatalf("partial file: %#v %v", change.Target, err)
		}
	}
}
func TestInterruptedTransactionRecoversAfterRelocation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "old")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	target := FileTarget{Path: "styles/fixture.md"}
	old := []byte("before")
	next := []byte("after")
	if err := writeFiles(root, map[string][]byte{target.Path: next}); err != nil {
		t.Fatal(err)
	}
	txn := transaction{ID: uuid.NewString(), State: "applying", Changes: []fileChange{{Target: target, Expected: revisionfile.Revision(old), Before: old, Existed: true, After: next}}}
	if err := saveTransaction(ctx, root, txn); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "new")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := Recover(moved, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(moved, target.Path))
	if err != nil || string(raw) != "before" {
		t.Fatalf("recover: %q %v", raw, err)
	}
	if err := Recover(moved, nil); err != nil {
		t.Fatal(err)
	}
}
func TestRecoveryPreservesExternalEdit(t *testing.T) {
	root := t.TempDir()
	target := FileTarget{Path: "fixture.json"}
	if err := writeFiles(root, map[string][]byte{target.Path: []byte("external")}); err != nil {
		t.Fatal(err)
	}
	txn := transaction{ID: uuid.NewString(), State: "applying", Changes: []fileChange{{Target: target, Expected: revisionfile.Revision([]byte("before")), Before: []byte("before"), Existed: true, After: []byte("after")}}}
	if err := saveTransaction(context.Background(), root, txn); err != nil {
		t.Fatal(err)
	}
	if err := Recover(root, nil); err == nil {
		t.Fatal("external edit overwritten")
	}
	raw, _ := os.ReadFile(filepath.Join(root, target.Path))
	if string(raw) != "external" {
		t.Fatal(string(raw))
	}
}

func TestMixedExtensionBatchPinsFrozenCodeAndGrants(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	files := map[string][]byte{}
	for _, fixture := range []string{"runtime", "tool"} {
		part, err := readFiles(filepath.Join("..", "..", "platform", "testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		for name, raw := range part {
			files["plugin/"+name] = raw
		}
	}
	files["skill/SKILL.md"] = []byte("---\nname: mixed-extension\ndescription: Mixed extension\n---\nUse the extension.\n")
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "extension-bundle", Name: "Extension bundle"}, Resources: []Resource{{ID: "plugin", Kind: "extension.plugin", Path: "plugin"}, {ID: "skill", Kind: "skill", Path: "skill", Requires: []string{"plugin"}}}}
	files["denova-pack.json"] = jsonBytes(t, manifest)
	preview, err := s.previewFiles(ctx, Source{Kind: "file", Filename: "bundle.zip"}, files)
	if err != nil {
		t.Fatal(err)
	}
	request := PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: []string{"skill"}}
	denied, err := s.Plan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, denied.ID); err == nil {
		t.Fatal("missing consent accepted")
	}
	plugins, err := s.platform.List(platform.Plugin)
	if err != nil || len(plugins) != 0 {
		t.Fatalf("partial plugin install: %v %v", plugins, err)
	}
	if _, err := os.Stat(filepath.Join(s.root, "skills", "mixed-extension", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("partial Skill install: %v", err)
	}
	request.Grants = map[string][]string{"plugin": preview.Candidates[0].Resources[0].Extension.Manifest.Permissions.Required}
	approved, err := s.Plan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Apply(ctx, approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	plugins, err = s.platform.List(platform.Plugin)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("plugin install: %v %v", plugins, err)
	}
	if plugins[0].CurrentRelease != preview.Candidates[0].Resources[0].Extension.Digest {
		t.Fatal("installed code differs from preview")
	}
	raw, err := s.Export(ctx, ExportRequest{Native: true, Resources: []LocalRef{result.Bindings[0].Local}})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Preview(ctx, Source{Kind: "file", Filename: "plugin.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if again.Candidates[0].Resources[0].Extension.Digest != plugins[0].CurrentRelease {
		t.Fatal("export changed extension release")
	}
	referenced, err := s.Plan(ctx, PlanRequest{PreviewID: again.ID, CandidateID: again.Candidates[0].ID, Resources: []string{"extension"}})
	if err != nil {
		t.Fatal(err)
	}
	if referenced.Items[0].Action != "reference" {
		t.Fatal("existing owned extension acquired another owner")
	}
	if _, err := s.Apply(ctx, referenced.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProjectResourcesAndAttachmentsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	workspace := filepath.Join(s.root, "projects", "book")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(workspace, project.TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "project-fixture", Name: "Project fixture"}, Resources: []Resource{{ID: "lore", Kind: "lore.item", Path: "lore.json", Assets: []string{"image.png"}}, {ID: "opening", Kind: "game.opening", Path: "opening.json"}, {ID: "cover", Kind: "project.cover", Path: "cover.json", Assets: []string{"image.png"}}}}
	files := map[string][]byte{"denova-pack.json": jsonBytes(t, manifest), "lore.json": []byte(`{"name":"Hero","type":"character","content":"A hero","image":{"asset_path":"image.png","alt_text":"Portrait"}}`), "opening.json": []byte(`{"title":"Arrival","content":"You arrive."}`), "cover.json": []byte(`{"asset_path":"image.png"}`), "image.png": picture.Bytes()}
	preview, err := s.previewFiles(ctx, Source{Kind: "file", Filename: "project.zip"}, files)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: []string{"lore", "opening", "cover"}, ProjectID: record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	refs := []LocalRef{}
	for _, binding := range plan.Installation.Bindings {
		refs = append(refs, binding.Local)
	}
	raw, err := s.Export(ctx, ExportRequest{Package: plan.Installation.Package, Resources: refs})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := platform.ArchiveFiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range archive {
		if strings.HasSuffix(name, ".json") && strings.Contains(string(data), workspace) {
			t.Fatalf("export contains host path: %s", name)
		}
		if strings.HasSuffix(name, "resource.json") && (bytes.Contains(data, []byte("profile_id")) || bytes.Contains(data, []byte("created_at"))) {
			t.Fatalf("export contains local image metadata: %s", name)
		}
	}
	copy, err := s.Preview(ctx, Source{Kind: "file", Filename: "copy.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, resource := range copy.Candidates[0].Resources {
		ids = append(ids, resource.ID)
	}
	another := filepath.Join(s.root, "projects", "copy")
	if err := os.MkdirAll(another, 0700); err != nil {
		t.Fatal(err)
	}
	other, err := s.registry.Add(another, project.TypeBook, "Copy")
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.Plan(ctx, PlanRequest{PreviewID: copy.ID, CandidateID: copy.Candidates[0].ID, Resources: ids, ProjectID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, next.ID); err != nil {
		t.Fatal(err)
	}
	items, err := lore.NewStore(another).ListAll()
	if err != nil || len(items) != 1 || items[0].Image == nil || items[0].Image.AltText != "Portrait" {
		t.Fatalf("Lore roundtrip: %v %v", items, err)
	}
}

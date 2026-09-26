package resourceexchange

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/book/lore"
	"denova/internal/platform"
	"denova/internal/project"
)

func TestLoreMaterialsRoundTripPreservesSharingAndAssociationText(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	addProject := func(name string) (string, string) {
		t.Helper()
		dir := filepath.Join(s.root, "projects", name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		record, err := s.registry.Add(dir, project.TypeBook, name)
		if err != nil {
			t.Fatal(err)
		}
		return record.ID, dir
	}
	projectID, dir := addProject("source")
	store := lore.NewStore(dir)
	for _, name := range []string{"Hero", "Scene"} {
		if _, err := store.Create(lore.ItemInput{ID: strings.ToLower(name), Name: name, Type: "character", Content: name + " setting"}); err != nil {
			t.Fatal(err)
		}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	// A minimal PCM WAV exercises the real upload and import validators.
	wave := make([]byte, 46)
	copy(wave, "RIFF")
	binary.LittleEndian.PutUint32(wave[4:], uint32(len(wave)-8))
	copy(wave[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wave[16:], 16)
	binary.LittleEndian.PutUint16(wave[20:], 1)
	binary.LittleEndian.PutUint16(wave[22:], 1)
	binary.LittleEndian.PutUint32(wave[24:], 8000)
	binary.LittleEndian.PutUint32(wave[28:], 16000)
	binary.LittleEndian.PutUint16(wave[32:], 2)
	binary.LittleEndian.PutUint16(wave[34:], 16)
	copy(wave[36:], "data")
	binary.LittleEndian.PutUint32(wave[40:], 2)
	files := map[string][]byte{"portrait.png": picture.Bytes(), "duplicate.png": picture.Bytes(), "voice.wav": wave}
	for _, name := range []string{"portrait.png", "duplicate.png", "voice.wav"} {
		if _, err := store.UploadMaterial(ctx, "hero", name, files[name]); err != nil {
			t.Fatal(err)
		}
	}
	hero, err := store.Get("hero")
	if err != nil {
		t.Fatal(err)
	}
	shared := hero.ResolvedMaterials[0].ID
	for _, mutation := range []struct {
		item string
		data lore.MaterialMutation
	}{
		{"hero", lore.MaterialMutation{Op: "update", AssetID: shared, Name: "Portrait", Description: "Keep this face."}},
		{"hero", lore.MaterialMutation{Op: "cover", AssetID: shared}},
		{"scene", lore.MaterialMutation{Op: "link", AssetID: shared, Name: "Cameo", Description: "Use in the background."}},
	} {
		if _, err := store.MutateMaterial(mutation.item, mutation.data); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := s.Export(ctx, ExportRequest{Package: PackageInfo{ID: "lore-materials", Name: "Lore materials"}, Resources: []LocalRef{
		{Kind: "lore.item", Scope: "project", ProjectID: projectID, ID: "hero"},
		{Kind: "lore.item", Scope: "project", ProjectID: projectID, ID: "scene"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := platform.ArchiveFiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	mediaCount := 0
	for name, data := range archive {
		if strings.HasPrefix(name, "assets/") {
			mediaCount++
		}
		if strings.HasSuffix(name, ".json") && (bytes.Contains(data, []byte(dir)) || bytes.Contains(data, []byte(shared))) {
			t.Fatalf("export leaked local identity: %s", name)
		}
	}
	if mediaCount != 3 {
		t.Fatalf("shared file must be exported once; distinct identical files must remain distinct: %d", mediaCount)
	}
	targetID, targetDir := addProject("target")
	preview, err := s.Preview(ctx, Source{Kind: "file", Filename: "materials.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, resource := range preview.Candidates[0].Resources {
		ids = append(ids, resource.ID)
	}
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: ids, ProjectID: targetID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := lore.NewStore(targetDir).ListAll()
	if err != nil || len(items) != 2 {
		t.Fatalf("imported items: %+v, %v", items, err)
	}
	byName := map[string]lore.Item{}
	for _, item := range items {
		byName[item.Name] = item
	}
	gotHero, gotScene := byName["Hero"], byName["Scene"]
	if len(gotHero.ResolvedMaterials) != 3 || len(gotScene.ResolvedMaterials) != 1 || gotHero.Image == nil || gotScene.Image != nil {
		t.Fatalf("materials or explicit cover were lost: %+v", items)
	}
	portrait, cameo := gotHero.ResolvedMaterials[0], gotScene.ResolvedMaterials[0]
	if portrait.ID == shared || portrait.ID != cameo.ID || portrait.Path != cameo.Path || gotHero.Materials.CoverAssetID != portrait.ID {
		t.Fatalf("IDs must be remapped once per shared file: %+v %+v", portrait, cameo)
	}
	if portrait.Name != "Portrait" || portrait.Description != "Keep this face." || cameo.Name != "Cameo" || cameo.Description != "Use in the background." {
		t.Fatalf("association text changed: %+v %+v", portrait, cameo)
	}
	for _, material := range gotHero.ResolvedMaterials {
		content, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(material.Path)))
		if err != nil || !bytes.Equal(content, files[material.OriginalName]) {
			t.Fatalf("material bytes changed: %s %v", material.Path, err)
		}
	}
	// Undoing an import must not break versions that already reference its media.
	restore, err := s.PlanRestore(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, restore.ID); err != nil {
		t.Fatal(err)
	}
	for _, material := range gotHero.ResolvedMaterials {
		if _, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(material.Path))); err != nil {
			t.Fatalf("undo removed immutable media: %v", err)
		}
	}
	if items, err := lore.NewStore(targetDir).ListAll(); err != nil || len(items) != 0 {
		t.Fatalf("undo did not restore metadata: %+v %v", items, err)
	}
}

func TestRemoteLoreMaterialsRoundTripWithoutFetching(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	dirs := map[string]string{}
	projects := map[string]string{}
	for _, name := range []string{"remote-source", "remote-target"} {
		dir := filepath.Join(service.root, "projects", name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		record, err := service.registry.Add(dir, project.TypeBook, name)
		if err != nil {
			t.Fatal(err)
		}
		dirs[name], projects[name] = dir, record.ID
	}
	store := lore.NewStore(dirs["remote-source"])
	const url = "https://unreachable.invalid/portrait.png?token=exact%2Fvalue"
	refs := []LocalRef{}
	for _, id := range []string{"hero", "scene"} {
		if _, err := store.Create(lore.ItemInput{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
		item, err := store.RemoteMaterial(ctx, id, lore.MaterialMutation{Op: "remote", URL: url, Name: id + " reference", Description: "Description for " + id})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.MutateMaterial(id, lore.MaterialMutation{Op: "cover", AssetID: item.ResolvedMaterials[0].ID}); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, LocalRef{Kind: "lore.item", Scope: "project", ProjectID: projects["remote-source"], ID: id})
	}
	raw, err := service.Export(ctx, ExportRequest{Package: PackageInfo{ID: "remote-materials", Name: "Remote materials"}, Resources: refs})
	if err != nil {
		t.Fatal(err)
	}
	files, err := platform.ArchiveFiles(raw)
	if err != nil {
		t.Fatal(err)
	}
	for name := range files {
		if strings.HasPrefix(name, "assets/") {
			t.Fatalf("remote export created media file: %s", name)
		}
	}
	preview, err := service.Preview(ctx, Source{Kind: "file", Filename: "remote.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, resource := range preview.Candidates[0].Resources {
		ids = append(ids, resource.ID)
	}
	plan, err := service.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: ids, ProjectID: projects["remote-target"]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := lore.NewStore(dirs["remote-target"]).ListAll()
	if err != nil || len(items) != 2 {
		t.Fatal(items, err)
	}
	sharedID := items[0].ResolvedMaterials[0].ID
	for _, item := range items {
		if len(item.ResolvedMaterials) != 1 {
			t.Fatal("lost materials", item)
		}
		material := item.ResolvedMaterials[0]
		if material.ID != sharedID || material.URL != url || material.Path != "" || material.Name != item.Name+" reference" || material.Description != "Description for "+item.Name || item.Image.ImageURL != url || item.Materials.CoverAssetID != sharedID {
			t.Fatalf("remote round trip changed material: %+v", item)
		}
	}
}

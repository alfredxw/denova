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

package versions

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"denova/internal/book/lore"
)

func TestLoreRestoreRetainsNewMediaAndRestoresDependencies(t *testing.T) {
	workspace := t.TempDir()
	service := newVersionTestService(t, workspace)
	defer service.Close()
	store := lore.NewStore(workspace)
	old := &lore.Image{ImagePath: "assets/lore/images/hero/old.png", MetaPath: "assets/lore/images/hero/meta.json"}
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Image: old}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, workspace, old.ImagePath, "old-image")
	writeFile(t, workspace, old.MetaPath, "old-source")
	first, err := service.Create("legacy", VersionSourceManual, DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	asset := lore.Asset{ID: "asset_new", Path: "assets/lore/media/asset_new/file.wav", MIMEType: "audio/wav", Source: lore.AssetSource{Kind: "upload"}}
	writeFile(t, workspace, asset.Path, "new-audio")
	if _, err := store.AttachAsset("hero", asset, lore.MaterialEntry{Description: "Ambient reference"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, workspace, "chapters/new.md", "new chapter")
	if _, err := service.Create("new media", VersionSourceManual, DefaultAutoSettings()); err != nil {
		t.Fatal(err)
	}
	plan, err := service.RestorePlan(first.Version.ID, nil, DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.RetainedMedia, asset.Path) {
		t.Fatalf("retained file not disclosed: %+v", plan)
	}
	result, err := service.Restore(first.Version.ID, DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == nil || result.Status.Clean {
		t.Fatal("retained media must be reported as outside target")
	}
	got, err := os.ReadFile(filepath.Join(workspace, asset.Path))
	if err != nil || string(got) != "new-audio" {
		t.Fatal("history asset lost", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "chapters/new.md")); !os.IsNotExist(err) {
		t.Fatal("non-media restore behavior changed", err)
	}
	item, err := store.ReadAny("hero")
	if err != nil || item.Materials != nil || len(item.ResolvedMaterials) != 1 {
		t.Fatal("legacy restore", item, err)
	}
	if err := os.Remove(filepath.Join(workspace, old.ImagePath)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, old.MetaPath)); err != nil {
		t.Fatal(err)
	}
	plan, err = service.RestorePlan(first.Version.ID, []string{lore.ItemsRelativePath}, DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Paths, old.ImagePath) || !slices.Contains(plan.Paths, old.MetaPath) {
		t.Fatal("dependencies absent", plan)
	}
	if _, err := service.RestoreWithPaths(first.Version.ID, []string{lore.ItemsRelativePath}, DefaultAutoSettings()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, old.ImagePath)); err != nil || string(got) != "old-image" {
		t.Fatal("dependency not restored", err)
	}
}

func TestLoreRestoreRemoteCoverKeepsURLWithoutFileDependencies(t *testing.T) {
	workspace := t.TempDir()
	service := newVersionTestService(t, workspace)
	defer service.Close()
	store := lore.NewStore(workspace)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero"}); err != nil {
		t.Fatal(err)
	}
	item, err := store.RemoteMaterial(context.Background(), "hero", lore.MaterialMutation{Op: "remote", URL: "https://unreachable.invalid/old.png"})
	if err != nil {
		t.Fatal(err)
	}
	id := item.ResolvedMaterials[0].ID
	if _, err = store.MutateMaterial("hero", lore.MaterialMutation{Op: "cover", AssetID: id}); err != nil {
		t.Fatal(err)
	}
	first, err := service.Create("remote cover", VersionSourceManual, DefaultAutoSettings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoteMaterial(context.Background(), "hero", lore.MaterialMutation{Op: "remote", AssetID: id, URL: "https://unreachable.invalid/new.png"}); err != nil {
		t.Fatal(err)
	}
	plan, err := service.RestorePlan(first.Version.ID, []string{lore.ItemsRelativePath}, DefaultAutoSettings())
	if err != nil || len(plan.Paths) != 1 || plan.Paths[0] != lore.ItemsRelativePath {
		t.Fatal(plan, err)
	}
	if _, err := service.RestoreWithPaths(first.Version.ID, []string{lore.ItemsRelativePath}, DefaultAutoSettings()); err != nil {
		t.Fatal(err)
	}
	restored, err := store.ReadAny("hero")
	if err != nil || restored.Image.ImageURL != "https://unreachable.invalid/old.png" || restored.Materials.CoverAssetID != id {
		t.Fatal(restored, err)
	}
}

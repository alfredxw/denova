package lore

import (
	"bytes"
	"os"
	"testing"
)

func TestMaterialsLegacyReadAndTextSave(t *testing.T) {
	s := NewStore(t.TempDir())
	item, err := s.Create(ItemInput{ID: "hero", Name: "Hero", Type: "character", Image: &Image{ImagePath: "assets/lore/images/hero/old.png", MIMEType: "image/png"}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.itemsPath())
	got, err := s.ReadAny(item.ID)
	if err != nil || got.Materials != nil || len(got.ResolvedMaterials) != 1 || got.Image == nil {
		t.Fatalf("legacy read: %+v %v", got, err)
	}
	after, _ := os.ReadFile(s.itemsPath())
	if !bytes.Equal(before, after) {
		t.Fatal("read changed disk")
	}
	got, err = s.Update(item.ID, ItemInput{Name: "Renamed", Type: "character", Content: "New text"})
	if err != nil || got.Materials != nil {
		t.Fatalf("text save promoted materials: %+v %v", got, err)
	}
	got, err = s.MutateMaterial(item.ID, MaterialMutation{Op: "remove", AssetID: got.ResolvedMaterials[0].ID})
	if err != nil || got.Image != nil || len(got.ResolvedMaterials) != 0 {
		t.Fatalf("remove: %+v %v", got, err)
	}
	got, err = s.ReadAny(item.ID)
	if err != nil || got.Materials == nil || got.Image != nil {
		t.Fatalf("legacy image resurrected: %+v %v", got, err)
	}
}

func TestMaterialsShareDescriptionsAndPreserveText(t *testing.T) {
	s := NewStore(t.TempDir())
	a, _ := s.Create(ItemInput{ID: "a", Name: "A"})
	b, _ := s.Create(ItemInput{ID: "b", Name: "B"})
	asset := Asset{ID: "asset_1", Path: "assets/lore/media/asset_1/file.png", MIMEType: "image/png", OriginalName: "test.png", Source: AssetSource{Kind: "upload"}}
	a, err := s.AttachAsset(a.ID, asset, MaterialEntry{Description: "Clothing reference"})
	if err != nil || a.Image != nil {
		t.Fatalf("append must not set cover: %+v %v", a, err)
	}
	b, err = s.MutateMaterial(b.ID, MaterialMutation{Op: "link", AssetID: asset.ID, Description: "Background"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.MutateMaterial(a.ID, MaterialMutation{Op: "cover", AssetID: asset.ID})
	if err != nil || a.Image == nil {
		t.Fatal("cover", err)
	}
	a, err = s.Update(a.ID, ItemInput{Name: "A edited", Content: "keep this", Image: &Image{ImagePath: "stale.png"}})
	if err != nil || len(a.ResolvedMaterials) != 1 || a.Image.ImagePath != asset.Path {
		t.Fatalf("text save lost media: %+v %v", a, err)
	}
	a, err = s.MutateMaterial(a.ID, MaterialMutation{Op: "cover"})
	if err != nil || a.Image != nil || len(a.ResolvedMaterials) != 1 {
		t.Fatal("clear cover removed material", err)
	}
	a, err = s.MutateMaterial(a.ID, MaterialMutation{Op: "remove", AssetID: asset.ID})
	if err != nil || a.Content != "keep this" {
		t.Fatal("material mutation lost text", err)
	}
	b, err = s.ReadAny(b.ID)
	if err != nil || b.ResolvedMaterials[0].Description != "Background" {
		t.Fatal("shared association changed", err)
	}
	c, err := s.loadOrCreate()
	if err != nil || len(c.Assets) != 1 {
		t.Fatalf("dedup: %+v %v", c, err)
	}
}

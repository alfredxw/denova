package lore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func materialPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func materialWAV() []byte {
	data := make([]byte, 60)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 52)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 16)
	return data
}
func TestMaterialUploadsPreserveTextRejectInvalidAndRecoverCanceled(t *testing.T) {
	s := NewStore(t.TempDir())
	item, err := s.Create(ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Before"})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{materialPNG(t), materialWAV()} {
		updated, err := s.UploadMaterial(context.Background(), item.ID, "../reference", data)
		if err != nil {
			t.Fatal(err)
		}
		material := updated.ResolvedMaterials[len(updated.ResolvedMaterials)-1]
		disk, err := os.ReadFile(filepath.Join(s.workspace, filepath.FromSlash(material.Path)))
		if err != nil || !bytes.Equal(data, disk) {
			t.Fatalf("file not committed: %v", err)
		}
		if material.OriginalName != "reference" || updated.Image != nil {
			t.Fatal("unsafe name or implicit cover", updated)
		}
	}
	_, err = s.Update(item.ID, ItemInput{Name: item.Name, Type: item.Type, Content: "Stale", BaseRevision: item.UpdatedAt})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale edit was not rejected: %v", err)
	}
	current, _ := s.ReadAny(item.ID)
	_, err = s.Update(item.ID, ItemInput{Name: item.Name, Type: item.Type, Content: "After", BaseRevision: current.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	current, _ = s.ReadAny(item.ID)
	if current.Content != "After" || len(current.ResolvedMaterials) != 2 {
		t.Fatal("text edit dropped media", current)
	}
	before, _ := os.ReadFile(s.itemsPath())
	for _, bad := range [][]byte{[]byte("invalid"), materialWAV()[:45]} {
		if _, err := s.UploadMaterial(context.Background(), item.ID, "bad.png", bad); !errors.Is(err, ErrMaterialInvalid) {
			t.Fatal("invalid upload accepted", err)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.UploadMaterial(canceled, item.ID, "cancel.png", materialPNG(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.itemsPath())
	if !bytes.Equal(before, after) {
		t.Fatal("failed upload modified metadata")
	}
	dirs, _ := os.ReadDir(filepath.Join(s.workspace, "assets/lore/media"))
	if len(dirs) != 2 {
		t.Fatalf("uncommitted upload leaked: %v", dirs)
	}
	if _, err := s.MutateMaterial(item.ID, MaterialMutation{Op: "cover", AssetID: current.ResolvedMaterials[1].ID}); err == nil {
		t.Fatal("audio accepted as cover")
	}
}

func TestInvalidNewMaterialsNeverFallBackToLegacy(t *testing.T) {
	for _, material := range []string{`null`, `{}`, `{"entries":[{"asset_id":"missing"}]}`, `{"entries":[],"cover_asset_id":"missing"}`} {
		raw := []byte(`{"version":2,"items":[{"id":"hero","name":"Hero","image":{"image_path":"old.png"},"materials":` + material + `}]}`)
		if _, err := decodeLoreCollectionJSON(raw); err == nil {
			t.Fatalf("invalid materials accepted: %s", raw)
		}
	}
}

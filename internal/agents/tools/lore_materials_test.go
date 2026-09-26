package tools

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book/lore"
	agent "github.com/alfredxw/denova/agent"
)

func TestLoreMaterialDiscoveryThenNativeImageRead(t *testing.T) {
	workspace, state := t.TempDir(), t.TempDir()
	store := lore.NewStore(workspace)
	item, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character"})
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	read, ctx := imageReadDefinition(t, workspace, state, 1024)
	item, err = store.UploadMaterial(ctx, item.ID, "portrait.png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	item, err = store.MutateMaterial(item.ID, lore.MaterialMutation{Op: "update", AssetID: item.ResolvedMaterials[0].ID, Description: "Use only the clothing"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := newLoreMaterialsTool(workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := list.Tool.Run(ctx, `{"item_id":"hero","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Materials []lore.Material `json:"materials"`
		Next      int             `json:"next_offset"`
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Materials) != 1 || page.Materials[0].Description != "Use only the clothing" || page.Next != -1 {
		t.Fatal("wrong selection metadata", result)
	}
	input, _ := json.Marshal(map[string]string{"path": page.Materials[0].Path})
	result, err = read.Tool.Run(agent.ContextWithToolCall(ctx, "read-selected-material", "read"), string(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attachments) != 1 || result.Attachments[0].MediaType != "image/png" {
		t.Fatal("image was not delivered as native media", result)
	}
	// Model input keeps an immutable copy even if the original file disappears.
	if err := os.Remove(filepath.Join(workspace, page.Materials[0].Path)); err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatal("selected media has no recovery artifact", result)
	}
}

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	runtimeapp "denova/internal/app"
	"denova/internal/book"
)

func TestLoreItemImageGenerateAPIUpdatesItem(t *testing.T) {
	application, imageServer := newLoreImageTestApplication(t)
	defer imageServer.Close()
	server := NewServer(application, "0")
	item, err := application.CreateLoreItem(book.LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", Content: "谨慎。"})
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodPost, "/api/lore/items/"+item.ID+"/image/generate", map[string]string{
		"instruction": "夜色氛围",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("generate status = %d body=%s", resp.Code, resp.Body.String())
	}
	var updated book.LoreItem
	decodeResponse(t, resp.Body.Bytes(), &updated)
	if updated.Image == nil || !strings.HasPrefix(updated.Image.ImagePath, "assets/lore/images/hero/") {
		t.Fatalf("generated item missing image: %#v", updated)
	}
	if _, err := application.BookService().ReadFile(updated.Image.MetaPath); err != nil {
		t.Fatalf("metadata should be saved: %v", err)
	}
	if filepath.Ext(updated.Image.ImagePath) != ".png" {
		t.Fatalf("image path should be png: %s", updated.Image.ImagePath)
	}
}

func TestLoreImagesGenerateStreamSkipsExistingByDefault(t *testing.T) {
	application, imageServer := newLoreImageTestApplication(t)
	defer imageServer.Close()
	server := NewServer(application, "0")
	withImage, err := application.CreateLoreItem(book.LoreItemInput{ID: "with-image", Type: "character", Name: "已有图", Importance: "major", Content: "已有。"})
	if err != nil {
		t.Fatal(err)
	}
	withoutImage, err := application.CreateLoreItem(book.LoreItemInput{ID: "without-image", Type: "location", Name: "无图地点", Importance: "important", Content: "地点。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewLoreStore(application.BookService().Workspace()).SetImage(withImage.ID, &book.LoreItemImage{
		Schema:    "lore_item_image.v1",
		ImagePath: "assets/lore/images/with-image/old/image.png",
		MetaPath:  "assets/lore/images/with-image/old/meta.json",
		ProfileID: "default",
		Provider:  "openai",
		Model:     "gpt-image-1",
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodPost, "/api/lore/images/generate/stream", map[string]any{
		"item_ids": []string{withImage.ID, withoutImage.ID},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"status":"skipped"`) || !strings.Contains(body, `"item_id":"with-image"`) {
		t.Fatalf("stream should report skipped existing image:\n%s", body)
	}
	if !strings.Contains(body, `"status":"success"`) || !strings.Contains(body, `"item_id":"without-image"`) {
		t.Fatalf("stream should report generated item:\n%s", body)
	}
	items, err := application.LoreItems()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]book.LoreItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if byID[withImage.ID].Image == nil || byID[withImage.ID].Image.ImagePath != "assets/lore/images/with-image/old/image.png" {
		t.Fatalf("existing image should be preserved: %#v", byID[withImage.ID])
	}
	if byID[withoutImage.ID].Image == nil || !strings.HasPrefix(byID[withoutImage.ID].Image.ImagePath, "assets/lore/images/without-image/") {
		t.Fatalf("missing image should be generated: %#v", byID[withoutImage.ID])
	}
}

func newLoreImageTestApplication(t *testing.T) (*runtimeapp.App, *httptest.Server) {
	t.Helper()
	var calls int
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created":       123,
			"output_format": "png",
			"quality":       "high",
			"size":          "2048x2048",
			"data": []map[string]any{{
				"b64_json":       base64.StdEncoding.EncodeToString(loreImageTestPNGBytes()),
				"revised_prompt": "revised prompt",
			}},
		})
	}))
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
		ImageAPIKey:         "test-key",
		ImageAPIBaseURL:     imageServer.URL,
		ImageAPIModel:       "gpt-image-1",
	})
	if err != nil {
		imageServer.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if calls == 0 {
			t.Fatalf("image server was not called")
		}
	})
	return application, imageServer
}

func loreImageTestPNGBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}
}

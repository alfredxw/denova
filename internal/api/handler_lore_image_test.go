package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/config"
	runtimeapp "denova/internal/app"
	"denova/internal/book/lore"
)

func TestLoreItemImageGenerateAPIUpdatesItem(t *testing.T) {
	application, imageServer := newLoreImageTestApplication(t)
	defer imageServer.Close()
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book/lore"
	item, err := application.ProjectBook().CreateLoreItem(projectID, lore.ItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", Content: "谨慎。"})
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSONRequest(t, server, http.MethodPost, base+"/items/"+item.ID+"/image/generate", map[string]string{
		"instruction": "夜色氛围",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("generate status = %d body=%s", resp.Code, resp.Body.String())
	}
	var updated lore.Item
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

func TestLoreItemImageUploadAPIUpdatesItem(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book/lore"
	item, err := application.ProjectBook().CreateLoreItem(projectID, lore.ItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", Content: "谨慎。"})
	if err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "portrait.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(loreImageTestPNGBytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	resp := ut.PerformRequest(
		server.engine.Engine,
		http.MethodPost,
		base+"/items/"+item.ID+"/image/upload",
		&ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()},
		ut.Header{Key: "Content-Type", Value: writer.FormDataContentType()},
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", resp.Code, resp.Body.String())
	}
	var updated lore.Item
	decodeResponse(t, resp.Body.Bytes(), &updated)
	if updated.Image == nil || updated.Image.Provider != "user_upload" || updated.Image.MIMEType != "image/png" {
		t.Fatalf("uploaded item missing image: %#v", updated)
	}
	if !strings.HasPrefix(updated.Image.ImagePath, "assets/lore/images/hero/") || filepath.Ext(updated.Image.ImagePath) != ".png" {
		t.Fatalf("unexpected image path: %s", updated.Image.ImagePath)
	}
	if _, err := application.BookService().ReadFile(updated.Image.MetaPath); err != nil {
		t.Fatalf("metadata should be saved: %v", err)
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
	if err := config.WriteSettingsFile(config.UserConfigPath(root), config.Settings{
		ImageAPIProfiles: []config.ImageAPIProfileSettings{{
			ID: config.DefaultImageAPIProfileID, APIKey: "test-key", BaseURL: imageServer.URL, Model: "gpt-image-1",
		}},
	}); err != nil {
		imageServer.Close()
		t.Fatal(err)
	}
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
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

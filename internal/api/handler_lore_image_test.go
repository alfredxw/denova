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
	var imageCalls int
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			modelRequest := string(encoded)
			for _, want := range []string{"image_preset", "互动游戏", "电影化", "光照", "肌肤", "姿势"} {
				if !strings.Contains(modelRequest, want) {
					t.Fatalf("visual guidance model request missing %q: %s", want, modelRequest)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-lore-visual-guidance",
				"object":  "chat.completion",
				"created": 123,
				"model":   "test-model",
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"visual_guidance":"黑色短发、灰色眼睛，神情谨慎沉静，穿深色风衣；夜色氛围下采用半身三分之二侧身站姿，双手自然放松，柔和侧逆光勾勒轮廓，自然肌肤毛孔与血色，浅景深；潮湿石砌街巷笼罩薄雾，暖黄路灯形成视觉焦点。"}`,
					},
					"finish_reason": "stop",
				}},
			})
		case "/images/generations":
			imageCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			prompt, _ := body["prompt"].(string)
			if strings.Contains(prompt, "资料类型：角色") {
				for _, want := range []string{"黑色短发", "谨慎沉静", "深色风衣", "夜色氛围", "侧身站姿", "侧逆光", "肌肤毛孔"} {
					if !strings.Contains(prompt, want) {
						t.Fatalf("character image prompt missing refined guidance %q: %s", want, prompt)
					}
				}
			}
			if strings.Contains(prompt, "资料类型：地点") {
				for _, want := range []string{"石砌街巷", "薄雾", "暖黄路灯"} {
					if !strings.Contains(prompt, want) {
						t.Fatalf("location image prompt missing refined guidance %q: %s", want, prompt)
					}
				}
			}
			for _, forbidden := range []string{"角色资料卡", "谨慎。", "地点。"} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("lore image prompt leaked raw lore %q: %s", forbidden, prompt)
				}
			}
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
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	root := t.TempDir()
	application, err := runtimeapp.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		OpenAIAPIKey:        "test-key",
		OpenAIBaseURL:       imageServer.URL,
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
		if imageCalls == 0 {
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

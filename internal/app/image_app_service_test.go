package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

func TestGenerateImageSavesOpenAIResultToAssets(t *testing.T) {
	workspace := t.TempDir()
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created":       123,
			"output_format": "png",
			"quality":       "high",
			"size":          "4096x2304",
			"data": []map[string]any{{
				"b64_json":       base64.StdEncoding.EncodeToString(testPNGBytes()),
				"revised_prompt": "revised prompt",
			}},
		})
	}))
	defer server.Close()

	application := &App{
		cfg: &config.Config{
			Workspace:       workspace,
			ImageAPIKey:     "test-key",
			ImageAPIBaseURL: server.URL,
			ImageAPIModel:   "gpt-image-1",
		},
		workspace:   workspace,
		bookService: book.NewService(workspace),
	}
	result, err := application.GenerateImage(context.Background(), imagegen.GenerateRequest{
		Prompt:  "a quiet writing desk",
		Quality: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestBody["prompt"] != "a quiet writing desk" || requestBody["model"] != "gpt-image-1" {
		t.Fatalf("unexpected OpenAI request body: %#v", requestBody)
	}
	if _, ok := requestBody["size"]; ok {
		t.Fatalf("size should be chosen by the caller, got request body: %#v", requestBody)
	}
	if len(result.Images) != 1 {
		t.Fatalf("saved images = %d", len(result.Images))
	}
	saved := result.Images[0]
	if !strings.HasPrefix(saved.Path, "assets/image/generated/") || !strings.HasSuffix(saved.Path, ".png") {
		t.Fatalf("unexpected saved path: %s", saved.Path)
	}
	if saved.MIMEType != "image/png" || saved.SizeBytes != len(testPNGBytes()) || saved.RevisedPrompt != "revised prompt" {
		t.Fatalf("unexpected saved metadata: %#v", saved)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(saved.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(testPNGBytes()) {
		t.Fatalf("saved image bytes mismatch")
	}
}

func testPNGBytes() []byte {
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

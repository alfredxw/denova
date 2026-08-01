package asset

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	imagegen "denova/internal/image/generation"
)

func TestServiceGenerateSavesInteractiveImageAndMeta(t *testing.T) {
	workspace := t.TempDir()
	generator := &interactiveFakeGenerator{
		result: imagegen.Result{
			ProfileID:    "default",
			Provider:     "openai",
			Model:        "gpt-image-1",
			Size:         "1024x1024",
			Quality:      "medium",
			OutputFormat: "png",
			Images: []imagegen.Image{{
				Data:          []byte("image-bytes"),
				MIMEType:      "image/png",
				Extension:     "png",
				RevisedPrompt: "revised",
			}},
		},
	}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 6, 27, 1, 2, 3, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.GenerateInteractive(context.Background(), &config.Config{}, book.NewService(workspace), InteractiveGenerateRequest{
		StoryID:  "story/one",
		BranchID: "main",
		TurnID:   "turn-1",
		Prompt:   "画出当前回合",
		AltText:  "互动图像",
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if generator.request.Prompt != "画出当前回合" {
		t.Fatalf("prompt = %q", generator.request.Prompt)
	}
	wantImagePath := "assets/interactive/images/story-one/main/turn-1/20260627-010203-abcd1234/image.png"
	if result.Schema != InteractiveResultSchema || result.ImagePath != wantImagePath {
		t.Fatalf("unexpected result: %#v", result)
	}
	imageBytes, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.ImagePath)))
	if err != nil {
		t.Fatalf("read image failed: %v", err)
	}
	if string(imageBytes) != "image-bytes" {
		t.Fatalf("image bytes = %q", string(imageBytes))
	}
	metaBytes, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatalf("read meta failed: %v", err)
	}
	var meta interactiveMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal meta failed: %v", err)
	}
	if meta.Schema != InteractiveResultSchema || meta.Source != interactiveSourceTool || meta.ImagePath != result.ImagePath || meta.Prompt != "画出当前回合" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
}

type interactiveFakeGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
	err     error
}

func (f *interactiveFakeGenerator) Generate(_ context.Context, _ *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error) {
	f.request = request
	return f.result, f.err
}

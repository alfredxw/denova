package illustration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

type fakeImageGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
}

func (g *fakeImageGenerator) Generate(_ context.Context, _ *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error) {
	g.request = request
	return g.result, nil
}

func TestGenerateWritesImageAndMetaUnderIllustrations(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.Create("chapters/ch01.md", "file", "# 第一章\n\n雨夜。"); err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	generator := &fakeImageGenerator{result: imagegen.Result{
		ProfileID:    "default",
		Provider:     "openai",
		Model:        "gpt-image-1",
		Size:         "4096x2304",
		Quality:      "high",
		OutputFormat: "png",
		Images: []imagegen.Image{{
			Data:          []byte("fake-png"),
			MIMEType:      "image/png",
			Extension:     "png",
			RevisedPrompt: "revised prompt",
		}},
	}}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 6, 27, 12, 30, 0, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.Generate(context.Background(), &config.Config{Workspace: workspace}, bookService, GenerateRequest{
		ChapterPath: "chapters/ch01.md",
		Prompt:      "rainy alley, cinematic",
		AltText:     "雨夜小巷",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.HasPrefix(result.ImagePath, "assets/illustrations/ch01/20260627-123000-abcd1234/image.") {
		t.Fatalf("image path = %q", result.ImagePath)
	}
	if result.MetaPath != "assets/illustrations/ch01/20260627-123000-abcd1234/meta.json" {
		t.Fatalf("meta path = %q", result.MetaPath)
	}
	if result.Markdown != "![雨夜小巷]("+result.ImagePath+")" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	imageBytes, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.ImagePath)))
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	if string(imageBytes) != "fake-png" {
		t.Fatalf("image bytes = %q", string(imageBytes))
	}
	metaBytes, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta Meta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("meta json: %v", err)
	}
	if meta.Schema != ResultSchema || meta.Source != sourceTool || meta.Prompt != "rainy alley, cinematic" || meta.RevisedPrompt != "revised prompt" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	if generator.request.N != 1 {
		t.Fatalf("image request should generate one image, got %#v", generator.request)
	}
}

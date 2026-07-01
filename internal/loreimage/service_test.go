package loreimage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	"denova/internal/imagegen"
)

func TestGenerateSavesLoreImageAndMetadata(t *testing.T) {
	workspace := t.TempDir()
	generator := &fakeImageGenerator{result: imagegen.Result{
		ProfileID:    "default",
		Provider:     "openai",
		Model:        "gpt-image-1",
		Size:         "2048x2048",
		OutputFormat: "png",
		Images:       []imagegen.Image{{Data: []byte("image"), Extension: "png", MIMEType: "image/png", RevisedPrompt: "revised"}},
	}}
	service := NewServiceWithGenerator(generator)
	service.now = func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.Generate(context.Background(), &config.Config{}, book.NewService(workspace), GenerateRequest{
		Item: book.LoreItem{
			ID:               "hero",
			Type:             "character",
			Name:             "林川",
			Tags:             []string{"主角"},
			BriefDescription: "角色 林川。谨慎。",
			Content:          "## 林川\n\n谨慎而疲惫。",
		},
		Instruction:       "夜色氛围",
		ImagePresetID:     "game-cg",
		ImagePresetPrompt: "电影感光影",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != ResultSchema || result.ImagePath != "assets/lore/images/hero/20260701-120000-abcd1234/image.png" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.MetaPath != "assets/lore/images/hero/20260701-120000-abcd1234/meta.json" || result.ImagePresetID != "game-cg" {
		t.Fatalf("unexpected metadata paths: %#v", result)
	}
	assertFile(t, workspace, result.ImagePath, "image")
	meta, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema": "lore_item_image.v1"`, `"item_id": "hero"`, `"image_preset_id": "game-cg"`, `"prompt":`} {
		if !strings.Contains(string(meta), want) {
			t.Fatalf("metadata missing %q:\n%s", want, string(meta))
		}
	}
	if !strings.Contains(generator.request.Prompt, "电影感光影") || !strings.Contains(generator.request.Prompt, "夜色氛围") || !strings.Contains(generator.request.Prompt, "林川") {
		t.Fatalf("prompt missing expected context:\n%s", generator.request.Prompt)
	}
}

func TestBuildPromptBoundsLoreContent(t *testing.T) {
	prompt := BuildPrompt(GenerateRequest{
		Item: book.LoreItem{
			ID:               "rule",
			Type:             "rule",
			Name:             "长规则",
			BriefDescription: strings.Repeat("简介", 1000),
			Content:          strings.Repeat("正文", 5000),
		},
		Instruction:       strings.Repeat("要求", 1000),
		ImagePresetPrompt: strings.Repeat("风格", 3000),
	})
	if len([]rune(prompt)) > maxPresetChars+maxBriefChars+maxContentChars+maxInstructionChars+600 {
		t.Fatalf("prompt is not bounded, runes=%d", len([]rune(prompt)))
	}
	if !strings.Contains(prompt, "资料类型：规则") || !strings.Contains(prompt, "资料名称：长规则") {
		t.Fatalf("prompt missing lore identity:\n%s", prompt)
	}
}

func TestGenerateStopsBeforeWritingWhenContextCanceledAfterModel(t *testing.T) {
	workspace := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	generator := &fakeImageGenerator{
		cancel: cancel,
		result: imagegen.Result{
			ProfileID:    "default",
			Provider:     "openai",
			Model:        "gpt-image-1",
			Size:         "2048x2048",
			OutputFormat: "png",
			Images:       []imagegen.Image{{Data: []byte("image"), Extension: "png", MIMEType: "image/png"}},
		},
	}
	service := NewServiceWithGenerator(generator)

	_, err := service.Generate(ctx, &config.Config{}, book.NewService(workspace), GenerateRequest{
		Item: book.LoreItem{
			ID:      "hero",
			Type:    "character",
			Name:    "林川",
			Content: "谨慎。",
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate error = %v, want context canceled", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "assets")); !os.IsNotExist(err) {
		t.Fatalf("assets should not be written after cancellation, err=%v", err)
	}
}

type fakeImageGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
	err     error
	cancel  context.CancelFunc
}

func (f *fakeImageGenerator) Generate(ctx context.Context, cfg *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error) {
	f.request = request
	if f.cancel != nil {
		f.cancel()
	}
	if f.err != nil {
		return imagegen.Result{}, f.err
	}
	return f.result, nil
}

func assertFile(t *testing.T, workspace, relPath, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", relPath, string(data), want)
	}
}

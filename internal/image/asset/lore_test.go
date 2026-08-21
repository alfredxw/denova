package asset

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
	"denova/internal/book/lore"
	imagegen "denova/internal/image/generation"
)

func TestGenerateSavesLoreImageAndMetadata(t *testing.T) {
	workspace := t.TempDir()
	generator := &loreFakeGenerator{result: imagegen.Result{
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

	result, err := service.GenerateLore(context.Background(), &config.Config{}, book.NewService(workspace), LoreGenerateRequest{
		Item: lore.Item{
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
	if result.Schema != LoreResultSchema || result.ImagePath != "assets/lore/images/hero/20260701-120000-abcd1234/image.png" {
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

func TestUploadLoreSavesValidatedImageAndMetadata(t *testing.T) {
	workspace := t.TempDir()
	service := NewServiceWithGenerator(nil)
	service.now = func() time.Time { return time.Date(2026, 7, 2, 13, 30, 0, 0, time.UTC) }
	service.suffix = func() string { return "upload01" }
	data := loreTestPNGBytes()

	result, err := service.UploadLore(context.Background(), book.NewService(workspace), LoreUploadRequest{
		Item:     lore.Item{ID: "hero", Type: "character", Name: "林川"},
		Filename: "portrait.png",
		Data:     data,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ImagePath != "assets/lore/images/hero/20260702-133000-upload01/image.png" || result.MetaPath != "assets/lore/images/hero/20260702-133000-upload01/meta.json" {
		t.Fatalf("unexpected upload paths: %#v", result)
	}
	if result.Provider != "user_upload" || result.ProfileID != "manual" || result.MIMEType != "image/png" || result.SizeBytes != len(data) {
		t.Fatalf("unexpected upload metadata: %#v", result)
	}
	imageData, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.ImagePath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(imageData) != string(data) {
		t.Fatal("uploaded image bytes were not preserved")
	}
	meta, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.MetaPath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"source": "user_upload"`, `"source_name": "portrait.png"`, `"item_id": "hero"`} {
		if !strings.Contains(string(meta), want) {
			t.Fatalf("metadata missing %q:\n%s", want, string(meta))
		}
	}
}

func TestUploadLoreRejectsInvalidImageBeforeWriting(t *testing.T) {
	workspace := t.TempDir()
	service := NewServiceWithGenerator(nil)

	_, err := service.UploadLore(context.Background(), book.NewService(workspace), LoreUploadRequest{
		Item:     lore.Item{ID: "hero", Name: "林川"},
		Filename: "portrait.png",
		Data:     []byte("not an image"),
	})
	if !errors.Is(err, ErrLoreImageUploadInvalid) {
		t.Fatalf("UploadLore error = %v, want invalid image", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "assets")); !os.IsNotExist(statErr) {
		t.Fatalf("assets should not be written for an invalid upload, err=%v", statErr)
	}
}

func TestBuildLorePromptBoundsLoreContent(t *testing.T) {
	prompt := BuildLorePrompt(LoreGenerateRequest{
		Item: lore.Item{
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
	if !strings.Contains(prompt, "Lore type: rule") || !strings.Contains(prompt, "Lore name: 长规则") {
		t.Fatalf("prompt missing lore identity:\n%s", prompt)
	}
}

func TestBuildLorePromptPreservesCustomFinalPrompt(t *testing.T) {
	const prompt = "masterpiece, 1girl, portrait, rim lighting"
	got := BuildLorePrompt(LoreGenerateRequest{
		Prompt: prompt, Item: lore.Item{ID: "hero", Name: "林川"},
		ImagePresetPrompt: "must not be appended", Instruction: "must not be appended",
	})
	if got != prompt {
		t.Fatalf("custom prompt changed: %q", got)
	}
}

func TestGenerateStopsBeforeWritingWhenContextCanceledAfterModel(t *testing.T) {
	workspace := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	generator := &loreFakeGenerator{
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

	_, err := service.GenerateLore(ctx, &config.Config{}, book.NewService(workspace), LoreGenerateRequest{
		Item: lore.Item{
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

type loreFakeGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
	err     error
	cancel  context.CancelFunc
}

func (f *loreFakeGenerator) Generate(ctx context.Context, cfg *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error) {
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

func loreTestPNGBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82,
	}
}

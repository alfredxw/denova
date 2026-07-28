package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	"denova/internal/illustration"
	"denova/internal/imagegen"
)

func TestParseChapterIllustrationToolResult(t *testing.T) {
	payload := illustration.Result{
		Schema:      illustration.ResultSchema,
		ChapterPath: "chapters/ch01.md",
		ImagePath:   "assets/illustrations/ch01/run/image.png",
		MetaPath:    "assets/illustrations/ch01/run/meta.json",
		Markdown:    "![图](assets/illustrations/ch01/run/image.png)",
		AltText:     "图",
		ProfileID:   "default",
		Provider:    "openai",
		Model:       "gpt-image-1",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := parseChapterIllustrationToolResult(generateImageToolName, string(raw)+"\n\n[Denova tool result metadata]\nschema: tool_result.v1")
	if err != nil {
		t.Fatalf("parseChapterIllustrationToolResult() error = %v", err)
	}
	if parsed == nil || parsed.ImagePath != payload.ImagePath || parsed.MetaPath != payload.MetaPath {
		t.Fatalf("unexpected parsed result: %#v", parsed)
	}
}

func TestParseLegacyChapterIllustrationToolResult(t *testing.T) {
	payload := illustration.Result{
		Schema:    illustration.ResultSchema,
		ImagePath: "assets/illustrations/ch01/run/image.png",
		MetaPath:  "assets/illustrations/ch01/run/meta.json",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := parseChapterIllustrationToolResult(generateChapterIllustrationToolName, string(raw))
	if err != nil {
		t.Fatalf("parseChapterIllustrationToolResult() error = %v", err)
	}
	if parsed == nil || parsed.MetaPath != payload.MetaPath {
		t.Fatalf("legacy result was not parsed: %#v", parsed)
	}
}

func TestParseGeneratedImageToolTarget(t *testing.T) {
	payload := generatedImageToolResult{
		Schema: generatedImageResultSchema,
		Images: []generatedImageToolImage{{
			Path: "assets/image/generated/20260627-test-01.png",
		}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if target := parseGeneratedImageToolTarget(generateImageToolName, string(raw)); target != payload.Images[0].Path {
		t.Fatalf("target = %q", target)
	}
}

func TestMergeImagePresetToolPromptPrependsPreset(t *testing.T) {
	got := mergeImagePresetToolPrompt(&config.Config{ImagePresetToolPrompt: "## 请求（tool_request）\n\n真实光影"}, "雨夜小巷，少女回头")
	for _, required := range []string{"# 图像风格要求", "真实光影", "# 本次图像请求", "雨夜小巷"} {
		if !strings.Contains(got, required) {
			t.Fatalf("merged prompt missing %q:\n%s", required, got)
		}
	}
	if strings.Index(got, "真实光影") > strings.Index(got, "雨夜小巷") {
		t.Fatalf("preset should be prepended before image request:\n%s", got)
	}
}

func TestPersistGeneratedImagesReturnsSuccessfulFilesAndFailures(t *testing.T) {
	workspace := t.TempDir()
	createdAt := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	result, err := persistGeneratedImages(book.NewService(workspace), generateImageInput{AltText: "场景"}, imagegen.Result{
		ProfileID: "profile", Provider: "openai", Model: "image-model", OutputFormat: "png",
		Images: []imagegen.Image{
			{Extension: "png"},
			{Data: []byte("valid-image"), Extension: "png", MIMEType: "image/png"},
		},
	}, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || len(result.Images) != 1 || len(result.Failures) != 1 ||
		result.Failures[0].Index != 0 || result.Failures[0].Code != "empty_image" {
		t.Fatalf("partial image receipt = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(result.Images[0].Path))); err != nil {
		t.Fatalf("successful image was not persisted: %v", err)
	}
}

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
	imageasset "denova/internal/image/asset"
	imagegen "denova/internal/image/generation"
)

func TestParseChapterIllustrationToolResult(t *testing.T) {
	payload := imageasset.IllustrationResult{
		Schema:      imageasset.IllustrationResultSchema,
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
	payload := imageasset.IllustrationResult{
		Schema:    imageasset.IllustrationResultSchema,
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

func TestNewGeneratedImageToolResultRetainsCanonicalPaths(t *testing.T) {
	value := generatedImageToolResult{
		Schema: generatedImageResultSchema, Status: "success", Provider: "openai", Model: "image-model",
		Images: []generatedImageToolImage{
			{Path: "assets/image/generated/first.png", Markdown: "![first](assets/image/generated/first.png)", RevisedPrompt: "provider-expanded-first"},
			{Path: "assets/image/generated/second.png", Markdown: "![second](assets/image/generated/second.png)", RevisedPrompt: "provider-expanded-second"},
		},
	}
	result, err := newGeneratedImageToolResult(value)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata.Target != value.Images[0].Path {
		t.Fatalf("target = %q, want %q", result.Metadata.Target, value.Images[0].Path)
	}
	if !strings.Contains(result.ModelContent, "provider-expanded-first") {
		t.Fatalf("same-turn model result lost provider output: %s", result.ModelContent)
	}
	if strings.Contains(string(result.Details), "provider-expanded") {
		t.Fatalf("retained details should omit provider prompt output: %s", result.Details)
	}
	var receipt generatedImageReceiptDetails
	if err := json.Unmarshal(result.Details, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Schema != generatedImageReceiptSchema || len(receipt.Images) != 2 ||
		receipt.Images[0].Path != value.Images[0].Path || receipt.Images[1].Path != value.Images[1].Path {
		t.Fatalf("retained image receipt = %#v", receipt)
	}
}

func TestGeneratedImageReceiptUsesMetadataAsMutationTarget(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "chapter illustration",
			value: imageasset.IllustrationResult{
				Schema: imageasset.IllustrationResultSchema, ImagePath: "assets/illustrations/ch01/image.png",
				MetaPath: "assets/illustrations/ch01/meta.json",
			},
			want: "assets/illustrations/ch01/meta.json",
		},
		{
			name: "interactive image",
			value: imageasset.InteractiveResult{
				Schema: imageasset.InteractiveResultSchema, ImagePath: "assets/interactive/images/turn.png",
				MetaPath: "assets/interactive/images/turn.json",
			},
			want: "assets/interactive/images/turn.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := newGeneratedImageToolResult(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if result.Metadata.Target != tt.want {
				t.Fatalf("target = %q, want %q", result.Metadata.Target, tt.want)
			}
		})
	}
}

func TestMergeImagePresetToolPromptPrependsPreset(t *testing.T) {
	got := mergeImagePresetToolPrompt(&config.Config{ImagePresetToolPrompt: "## 请求（tool_request）\n\n真实光影"}, "雨夜小巷，少女回头")
	for _, required := range []string{"# Image Style Requirements", "真实光影", "# Current Image Request", "雨夜小巷"} {
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

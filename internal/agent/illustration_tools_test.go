package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"nova/config"
	"nova/internal/illustration"
)

func TestParseChapterIllustrationToolResultAndTracksMutationTarget(t *testing.T) {
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
	parsed, err := parseChapterIllustrationToolResult(generateImageToolName, string(raw)+"\n\n[Nova tool result metadata]\nschema: tool_result.v1")
	if err != nil {
		t.Fatalf("parseChapterIllustrationToolResult() error = %v", err)
	}
	if parsed == nil || parsed.ImagePath != payload.ImagePath || parsed.MetaPath != payload.MetaPath {
		t.Fatalf("unexpected parsed result: %#v", parsed)
	}

	tracker := newMutationTracker()
	tracker.Observe(Event{Type: "tool_call", Data: map[string]interface{}{
		"id":   "call-image",
		"name": generateImageToolName,
		"args": `{"purpose":"chapter_illustration","target_path":"chapters/ch01.md","prompt":"雨夜"}`,
	}})
	tracker.Observe(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-image",
		"name":    generateImageToolName,
		"content": string(raw),
		"target":  payload.MetaPath,
	}})
	mutations := tracker.Mutations()
	if len(mutations) != 1 {
		t.Fatalf("expected one image mutation, got %#v", mutations)
	}
	if mutations[0].Source != ToolSourceImage || mutations[0].Target != payload.MetaPath || !mutations[0].RequiresPostCheck {
		t.Fatalf("unexpected mutation: %#v", mutations[0])
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
	for _, required := range []string{"# 图像方案（原样注入）", "真实光影", "# 本次图像请求", "雨夜小巷"} {
		if !strings.Contains(got, required) {
			t.Fatalf("merged prompt missing %q:\n%s", required, got)
		}
	}
	if strings.Index(got, "真实光影") > strings.Index(got, "雨夜小巷") {
		t.Fatalf("preset should be prepended before image request:\n%s", got)
	}
}

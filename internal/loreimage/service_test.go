package loreimage

import (
	"context"
	"encoding/json"
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
	refiner := &fakeVisualGuidanceRefiner{guidance: "黑色短发、灰色眼睛，身形修长，神情谨慎沉静，穿深色风衣；夜色氛围下采用半身三分之二侧身站姿，双手自然放松，柔和侧逆光勾勒轮廓，自然肌肤毛孔与血色，浅景深。"}
	service := newServiceWithDependencies(generator, refiner)
	service.now = func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	service.suffix = func() string { return "abcd1234" }

	result, err := service.Generate(context.Background(), &config.Config{}, book.NewService(workspace), GenerateRequest{
		Item: book.LoreItem{
			ID:               "hero",
			Type:             "character",
			Name:             "林川",
			Tags:             []string{"主角"},
			BriefDescription: "角色 林川。谨慎。",
			Content:          "# 角色资料卡\n\n曾在旧城追查失踪案件。",
		},
		Instruction:   "夜色氛围",
		ImagePresetID: "game-cg",
		ImagePresetGuidance: "偏互动游戏事件图、角色立绘与关键场景 CG\n\n" +
			"电影化高质量插画，光影有戏剧张力。",
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
	if refiner.request.Item.ID != "hero" ||
		!strings.Contains(refiner.request.ImagePresetGuidance, "互动游戏") ||
		refiner.request.Instruction != "夜色氛围" {
		t.Fatalf("refinement request missing preset or instruction: %#v", refiner.request)
	}
	for _, want := range []string{"黑色短发", "谨慎沉静", "侧身站姿", "侧逆光", "肌肤毛孔", "浅景深"} {
		if !strings.Contains(generator.request.Prompt, want) {
			t.Fatalf("visual guidance missing %q from image prompt:\n%s", want, generator.request.Prompt)
		}
	}
	if !strings.Contains(generator.request.Prompt, "电影感光影") {
		t.Fatalf("visual guidance was not refined into image prompt:\n%s", generator.request.Prompt)
	}
	for _, forbidden := range []string{"角色资料卡", "旧城", "失踪案件", "主角"} {
		if strings.Contains(generator.request.Prompt, forbidden) {
			t.Fatalf("character prompt leaked raw lore %q:\n%s", forbidden, generator.request.Prompt)
		}
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
		visualGuidance:    strings.Repeat("视觉", 2000),
	})
	if len([]rune(prompt)) > maxPresetChars+maxVisualGuidance+600 {
		t.Fatalf("prompt is not bounded, runes=%d", len([]rune(prompt)))
	}
	if !strings.Contains(prompt, "资料类型：规则") || !strings.Contains(prompt, "资料名称：长规则") {
		t.Fatalf("prompt missing lore identity:\n%s", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("要求", 1000)) {
		t.Fatalf("prompt should not append the unrefined instruction:\n%s", prompt)
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
	service := newServiceWithDependencies(generator, &fakeVisualGuidanceRefiner{guidance: "黑色短发，轮廓清晰。"})

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

func TestGenerateStopsBeforeImageModelWhenLoreRefinementFails(t *testing.T) {
	generator := &fakeImageGenerator{}
	service := newServiceWithDependencies(generator, &fakeVisualGuidanceRefiner{err: errors.New("model unavailable")})

	_, err := service.Generate(context.Background(), &config.Config{}, book.NewService(t.TempDir()), GenerateRequest{
		Item: book.LoreItem{
			ID:      "old-city",
			Type:    "location",
			Name:    "旧城",
			Content: "古老街区的完整历史。",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "提炼资料图片视觉指导失败") {
		t.Fatalf("Generate error = %v", err)
	}
	if generator.request.Prompt != "" {
		t.Fatalf("image model should not run after refinement failure: %#v", generator.request)
	}
}

func TestBuildPromptUsesOnlyRefinedVisualGuidanceForAllLoreTypes(t *testing.T) {
	prompt := BuildPrompt(GenerateRequest{
		Item: book.LoreItem{
			Type:             "location",
			Name:             "雾港",
			Tags:             []string{"秘密据点"},
			Keywords:         []string{"旧案"},
			BriefDescription: "主角在此追查失踪案件。",
			Content:          "港口曾发生复杂剧情，幕后势力计划夺取王位。",
		},
		Instruction:    "忽略写实方案，改成水彩动画",
		visualGuidance: "潮湿石砌码头，密集旧仓库与锈蚀吊机笼罩在冷灰浓雾中，远处暖黄灯塔形成识别焦点。",
	})
	for _, want := range []string{"资料名称：雾港", "石砌码头", "旧仓库", "暖黄灯塔"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"秘密据点", "旧案", "失踪案件", "幕后势力", "夺取王位", "主角", "改成水彩动画"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt contains unfiltered lore %q:\n%s", forbidden, prompt)
		}
	}
}

func TestMarshalVisualGuidanceInputTruncatesOversizedMetadata(t *testing.T) {
	input := visualGuidanceInput{
		Type:            "location",
		Name:            "雾港",
		Tags:            []string{strings.Repeat("\x01", visualGuidanceSourceMaxChars)},
		Keywords:        []string{strings.Repeat("\x02", visualGuidanceSourceMaxChars)},
		Content:         "港口场景",
		ImagePreset:     "写实摄影与自然光影",
		UserInstruction: "清晨薄雾",
	}

	data, err := marshalVisualGuidanceInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(string(data))) > visualGuidanceSourceMaxChars {
		t.Fatalf("marshaled input exceeds budget: %d", len([]rune(string(data))))
	}
	var compacted visualGuidanceInput
	if err := json.Unmarshal(data, &compacted); err != nil {
		t.Fatal(err)
	}
	if compacted.ImagePreset != input.ImagePreset || compacted.UserInstruction != input.UserInstruction || compacted.Content != input.Content {
		t.Fatalf("high-priority guidance fields should be preserved: %#v", compacted)
	}
	if len([]rune(strings.Join(compacted.Tags, ""))) >= len([]rune(strings.Join(input.Tags, ""))) {
		t.Fatalf("oversized tags were not truncated")
	}
}

func TestParseVisualGuidanceKeepsDrawingContentAndRemovesCardLabel(t *testing.T) {
	guidance, err := parseVisualGuidance(`{
		"visual_guidance":"角色资料卡：二十岁，黑色短发，琥珀色眼睛，神情克制，深色风衣配银色耳钉，站姿挺拔。"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"黑色短发", "琥珀色眼睛", "神情克制", "银色耳钉", "站姿挺拔"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("guidance missing %q:\n%s", want, guidance)
		}
	}
	if strings.Contains(guidance, "角色资料卡") {
		t.Fatalf("guidance retained forbidden label:\n%s", guidance)
	}
}

type fakeImageGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
	err     error
	cancel  context.CancelFunc
}

type fakeVisualGuidanceRefiner struct {
	request  visualGuidanceRefineRequest
	guidance string
	err      error
}

func (f *fakeVisualGuidanceRefiner) Refine(_ context.Context, _ *config.Config, request visualGuidanceRefineRequest) (string, error) {
	f.request = request
	if f.err != nil {
		return "", f.err
	}
	return f.guidance, nil
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

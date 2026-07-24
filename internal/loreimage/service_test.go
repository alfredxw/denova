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
	refiner := &fakeCharacterTraitsRefiner{traits: "- 外貌：黑发灰眼，身形修长\n- 性格：谨慎沉静"}
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
	if refiner.item.ID != "hero" || !strings.Contains(generator.request.Prompt, "黑发灰眼") || !strings.Contains(generator.request.Prompt, "谨慎沉静") {
		t.Fatalf("character traits were not refined into image prompt:\n%s", generator.request.Prompt)
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
	service := newServiceWithDependencies(generator, &fakeCharacterTraitsRefiner{traits: "- 外貌：黑发"})

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

func TestGenerateStopsBeforeImageModelWhenCharacterRefinementFails(t *testing.T) {
	generator := &fakeImageGenerator{}
	service := newServiceWithDependencies(generator, &fakeCharacterTraitsRefiner{err: errors.New("model unavailable")})

	_, err := service.Generate(context.Background(), &config.Config{}, book.NewService(t.TempDir()), GenerateRequest{
		Item: book.LoreItem{
			ID:      "hero",
			Type:    "character",
			Name:    "林川",
			Content: "角色资料卡：主角经历。",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "提炼角色图片特点失败") {
		t.Fatalf("Generate error = %v", err)
	}
	if generator.request.Prompt != "" {
		t.Fatalf("image model should not run after refinement failure: %#v", generator.request)
	}
}

func TestBuildPromptUsesOnlyRefinedTraitsForCharacterLore(t *testing.T) {
	prompt := BuildPrompt(GenerateRequest{
		Item: book.LoreItem{
			Type:             "character",
			Name:             "沈凝",
			Tags:             []string{"反派"},
			Keywords:         []string{"秘密组织"},
			BriefDescription: "角色资料卡：组织首领。",
			Content:          "她计划夺取王位，并与主角存在宿怨。",
		},
		characterTraits: "- 外貌：银色长发，蓝灰色眼睛\n- 性格：冷静克制\n角色资料卡",
	})
	for _, want := range []string{"资料名称：沈凝", "银色长发", "冷静克制"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"角色资料卡", "组织首领", "秘密组织", "夺取王位", "主角", "反派"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt contains unfiltered character lore %q:\n%s", forbidden, prompt)
		}
	}
}

func TestParseCharacterTraitsKeepsVisualFieldsAndRemovesCardLabel(t *testing.T) {
	traits, err := parseCharacterTraits(`{
		"appearance":"角色资料卡：二十岁，黑色短发，琥珀色眼睛",
		"personality":"外冷内热，神情克制",
		"attire_accessories":"深色风衣，银色耳钉",
		"other_visual_traits":"站姿挺拔"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"外貌：", "黑色短发", "性格：", "外冷内热", "服装与配饰：", "银色耳钉", "其他可视特点：", "站姿挺拔"} {
		if !strings.Contains(traits, want) {
			t.Fatalf("traits missing %q:\n%s", want, traits)
		}
	}
	if strings.Contains(traits, "角色资料卡") {
		t.Fatalf("traits retained forbidden label:\n%s", traits)
	}
}

type fakeImageGenerator struct {
	request imagegen.GenerateRequest
	result  imagegen.Result
	err     error
	cancel  context.CancelFunc
}

type fakeCharacterTraitsRefiner struct {
	item   book.LoreItem
	traits string
	err    error
}

func (f *fakeCharacterTraitsRefiner) Refine(_ context.Context, _ *config.Config, item book.LoreItem) (string, error) {
	f.item = item
	if f.err != nil {
		return "", f.err
	}
	return f.traits, nil
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

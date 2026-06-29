package imagepreset

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryMaterializesBuiltins(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	presets, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 3 {
		t.Fatalf("built-in image presets = %d, want 3: %#v", len(presets), presets)
	}
	ids := map[string]bool{}
	for _, preset := range presets {
		ids[preset.ID] = true
		if preset.Custom {
			t.Fatalf("built-in preset marked custom: %#v", preset)
		}
	}
	for _, id := range []string{DefaultID, "realistic", "2d-illustration"} {
		if !ids[id] {
			t.Fatalf("missing built-in preset %s: %#v", id, presets)
		}
	}
}

func TestPresetPromptNormalizesAndRoundTrips(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	longPrompt := "  " + strings.Repeat("图", MaxPromptChars+20) + "  "
	created, err := lib.Create(Preset{
		ID:          "visual",
		Name:        "视觉方案",
		Description: "自定义视觉方案",
		Prompt:      longPrompt,
		Tags:        []string{"自定义", "自定义"},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(created.Slots) != 1 {
		t.Fatalf("created slots = %d, want 1: %#v", len(created.Slots), created.Slots)
	}
	if got := len([]rune(created.Slots[0].Content)); got != MaxPromptChars {
		t.Fatalf("created slot prompt chars = %d, want %d", got, MaxPromptChars)
	}
	if len(created.Slots) != 1 || created.Slots[0].Target != TargetToolRequest {
		t.Fatalf("legacy prompt should become tool_request slot: %#v", created.Slots)
	}
	if len(created.Tags) != 1 {
		t.Fatalf("tags should be deduped: %#v", created.Tags)
	}
	loaded, err := lib.Get("visual")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded.Prompt != created.Prompt {
		t.Fatalf("prompt should round trip, got %q want %q", loaded.Prompt, created.Prompt)
	}
	if loaded.PromptForTargets(TargetToolRequest) == "" {
		t.Fatalf("tool request prompt should be readable: %#v", loaded)
	}
}

func TestLegacyPromptFileLoadsAsToolRequestSlot(t *testing.T) {
	dir := t.TempDir()
	presetDir := filepath.Join(dir, "image-presets")
	if err := os.MkdirAll(presetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(presetDir, "legacy.json"), []byte(`{
  "version": 1,
  "id": "legacy",
  "name": "旧图像方案",
  "description": "旧格式",
  "prompt": "旧风格 prompt",
  "tags": ["旧"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewLibrary(dir).Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != Version {
		t.Fatalf("version = %d, want %d", loaded.Version, Version)
	}
	if len(loaded.Slots) != 1 || loaded.Slots[0].Target != TargetToolRequest || loaded.Slots[0].Content != "旧风格 prompt" {
		t.Fatalf("legacy prompt not converted to tool_request slot: %#v", loaded.Slots)
	}
	if loaded.PromptForTargets(TargetAgentSystem) != "" {
		t.Fatalf("legacy prompt should not become agent_system: %#v", loaded)
	}
}

func TestPresetSlotsNormalizeAndReadTargets(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	created, err := lib.Create(Preset{
		ID:   "split",
		Name: "分层图像方案",
		Slots: []Slot{
			{ID: "sys", Name: "系统", Target: TargetAgentSystem, Enabled: true, Content: "理解规则"},
			{ID: "tool", Name: "请求", Target: TargetToolRequest, Enabled: true, Content: "原样风格"},
			{ID: "off", Name: "关闭", Target: TargetToolRequest, Enabled: false, Content: "不应出现"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := created.PromptForTargets(TargetAgentSystem); !strings.Contains(got, "理解规则") || strings.Contains(got, "原样风格") {
		t.Fatalf("agent system prompt mismatch:\n%s", got)
	}
	if got := created.PromptForTargets(TargetToolRequest); !strings.Contains(got, "原样风格") || strings.Contains(got, "不应出现") {
		t.Fatalf("tool request prompt mismatch:\n%s", got)
	}
}

func TestCustomPresetUpdateAndDelete(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	created, err := lib.Create(Preset{ID: "custom", Name: "旧方案", Prompt: "旧 prompt"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := lib.Update(created.ID, Preset{Name: "新方案", Prompt: "新 prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "新方案" || len(updated.Slots) != 1 || updated.Slots[0].Content != "新 prompt" {
		t.Fatalf("unexpected updated preset: %#v", updated)
	}
	if err := lib.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Get(created.ID); err == nil {
		t.Fatalf("deleted preset should not load")
	}
}

func TestPresetUpdateRejectsStaleRevision(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	created, err := lib.Create(Preset{ID: "custom", Name: "旧方案", Prompt: "旧 prompt"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := lib.Update(created.ID, Preset{Name: "Agent 方案", Prompt: "Agent prompt"}, created.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Update(created.ID, Preset{Name: "前端旧方案", Prompt: "前端旧 prompt"}, created.UpdatedAt); !errors.Is(err, ErrPresetRevisionConflict) {
		t.Fatalf("expected preset revision conflict, got %v", err)
	}
	got, err := lib.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != agent.Name {
		t.Fatalf("stale save should not overwrite Agent preset: %#v", got)
	}
}

func TestBuiltinPresetCannotBeDeleted(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if err := lib.Delete(DefaultID); err == nil {
		t.Fatalf("expected built-in delete to fail")
	}
}

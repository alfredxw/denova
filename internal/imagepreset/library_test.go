package imagepreset

import (
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
	if got := len([]rune(created.Prompt)); got != MaxPromptChars {
		t.Fatalf("created prompt chars = %d, want %d", got, MaxPromptChars)
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
	if updated.Name != "新方案" || updated.Prompt != "新 prompt" {
		t.Fatalf("unexpected updated preset: %#v", updated)
	}
	if err := lib.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Get(created.ID); err == nil {
		t.Fatalf("deleted preset should not load")
	}
}

func TestBuiltinPresetCannotBeDeleted(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if err := lib.Delete(DefaultID); err == nil {
		t.Fatalf("expected built-in delete to fail")
	}
}

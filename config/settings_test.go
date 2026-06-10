package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSettingsValues(t *testing.T) {
	s := DefaultSettings()
	if s.OpenAIBaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL: %s", s.OpenAIBaseURL)
	}
	if s.OpenAIModel != "deepseek-v4-pro" {
		t.Fatalf("Model: %s", s.OpenAIModel)
	}
	if s.AutoSaveEnabled == nil || *s.AutoSaveEnabled != true {
		t.Fatalf("AutoSaveEnabled default")
	}
	if s.MaxIteration == nil || *s.MaxIteration != 50 {
		t.Fatalf("MaxIteration default")
	}
	if s.InteractiveStageFontSize == nil || *s.InteractiveStageFontSize != 16 {
		t.Fatalf("InteractiveStageFontSize default")
	}
	if s.InteractiveStageLineHeight == nil || *s.InteractiveStageLineHeight != 1.78 {
		t.Fatalf("InteractiveStageLineHeight default")
	}
	if s.ChapterFilenameFormat != "第{N}章-{title}.md" {
		t.Fatalf("ChapterFilenameFormat default: %s", s.ChapterFilenameFormat)
	}
	if s.InteractiveHotChoices == nil || *s.InteractiveHotChoices != true {
		t.Fatalf("InteractiveHotChoices default")
	}
	if s.AgentModels.ToolAgent.EnableThinking == nil || *s.AgentModels.ToolAgent.EnableThinking {
		t.Fatalf("ToolAgent thinking should default off")
	}
	if s.UIFontFamily != "system-sans" {
		t.Fatalf("UIFontFamily default: %s", s.UIFontFamily)
	}
	if s.UIFontSize == nil || *s.UIFontSize != 12 {
		t.Fatalf("UIFontSize default")
	}
	if s.ReadingFontFamily != "source-han-serif" {
		t.Fatalf("ReadingFontFamily default: %s", s.ReadingFontFamily)
	}
	if s.ReadingFontSize == nil || *s.ReadingFontSize != 18 {
		t.Fatalf("ReadingFontSize default")
	}
	if s.Language != "auto" {
		t.Fatalf("Language default: %s", s.Language)
	}
}

func TestMergeOverridesNonZero(t *testing.T) {
	parent := Settings{
		OpenAIBaseURL:              "https://parent",
		OpenAIModel:                "p-model",
		MaxIteration:               intPtr(10),
		UIFontFamily:               "system-sans",
		UIFontSize:                 intPtr(12),
		ReadingFontFamily:          "source-han-serif",
		ReadingFontSize:            intPtr(18),
		Language:                   "auto",
		InteractiveMaxTokens:       intPtr(0),
		InteractiveHotChoices:      boolPtr(true),
		InteractiveStageFontSize:   intPtr(16),
		InteractiveStageLineHeight: floatPtr(1.78),
	}
	child := Settings{
		OpenAIModel:                "c-model", // override
		MaxIteration:               nil,       // 继承 parent
		UIFontFamily:               "humanist-sans",
		UIFontSize:                 intPtr(13),
		ReadingFontFamily:          "system-serif",
		ReadingFontSize:            intPtr(20),
		Language:                   "en-US",
		InteractiveMaxTokens:       intPtr(4000),
		InteractiveHotChoices:      boolPtr(false),
		InteractiveStageFontSize:   intPtr(18),
		InteractiveStageLineHeight: floatPtr(1.95),
	}
	out := Merge(parent, child)
	if out.OpenAIBaseURL != "https://parent" {
		t.Fatalf("BaseURL should inherit: %s", out.OpenAIBaseURL)
	}
	if out.OpenAIModel != "c-model" {
		t.Fatalf("Model should override: %s", out.OpenAIModel)
	}
	if out.MaxIteration == nil || *out.MaxIteration != 10 {
		t.Fatalf("MaxIteration should inherit parent")
	}
	if out.UIFontFamily != "humanist-sans" {
		t.Fatalf("UIFontFamily should override parent: %s", out.UIFontFamily)
	}
	if out.UIFontSize == nil || *out.UIFontSize != 13 {
		t.Fatalf("UIFontSize should override parent")
	}
	if out.ReadingFontFamily != "system-serif" {
		t.Fatalf("ReadingFontFamily should override parent: %s", out.ReadingFontFamily)
	}
	if out.ReadingFontSize == nil || *out.ReadingFontSize != 20 {
		t.Fatalf("ReadingFontSize should override parent")
	}
	if out.Language != "en-US" {
		t.Fatalf("Language should override parent: %s", out.Language)
	}
	if out.InteractiveMaxTokens == nil || *out.InteractiveMaxTokens != 4000 {
		t.Fatalf("InteractiveMaxTokens should override parent")
	}
	if out.InteractiveHotChoices == nil || *out.InteractiveHotChoices != false {
		t.Fatalf("InteractiveHotChoices should override parent")
	}
	if out.InteractiveStageFontSize == nil || *out.InteractiveStageFontSize != 18 {
		t.Fatalf("InteractiveStageFontSize should override parent")
	}
	if out.InteractiveStageLineHeight == nil || *out.InteractiveStageLineHeight != 1.95 {
		t.Fatalf("InteractiveStageLineHeight should override parent")
	}
}

func TestMergePointerExplicitOverride(t *testing.T) {
	parent := Settings{AutoSaveEnabled: boolPtr(true)}
	child := Settings{AutoSaveEnabled: boolPtr(false)}
	out := Merge(parent, child)
	if out.AutoSaveEnabled == nil || *out.AutoSaveEnabled != false {
		t.Fatalf("explicit false should override true")
	}
}

func TestReadSettingsFileMissingReturnsZero(t *testing.T) {
	s, err := ReadSettingsFile(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if s.OpenAIModel != "" {
		t.Fatalf("missing file should yield zero value")
	}
}

func TestWriteThenReadSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AutoSaveEnabled: boolPtr(false), Language: "en-US"}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "abc" {
		t.Fatalf("model")
	}
	if out.AutoSaveEnabled == nil || *out.AutoSaveEnabled != false {
		t.Fatalf("auto save")
	}
	if out.Language != "en-US" {
		t.Fatalf("language")
	}
}

func TestWriteSettingsFileFiltersInvalidLanguage(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", Language: "fr-FR"}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Language != "" {
		t.Fatalf("invalid language should be filtered: %q", out.Language)
	}
}

func TestWriteSettingsFileFiltersNovaDir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", NovaDir: "/tmp/ignored"}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatalf("settings file should not be empty")
	}
	if strings.Contains(string(data), "nova_dir") {
		t.Fatalf("nova_dir should not be persisted in editable settings: %s", string(data))
	}
}

func TestLoadLayeredAppliesAllLayers(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}

	user := Settings{OpenAIModel: "user-model", MaxIteration: intPtr(20)}
	wsCfg := Settings{OpenAIModel: "ws-model"}
	if err := WriteSettingsFile(filepath.Join(home, "config.toml"), user); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(ws, ".nova", "config.toml"), wsCfg); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Effective.OpenAIModel != "ws-model" {
		t.Fatalf("workspace should win: %s", layered.Effective.OpenAIModel)
	}
	if layered.Effective.MaxIteration == nil || *layered.Effective.MaxIteration != 20 {
		t.Fatalf("user MaxIteration should inherit: %v", layered.Effective.MaxIteration)
	}
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("raw user should be preserved")
	}
}

func TestLoadLayeredIgnoresNovaDirFromEditableLayers(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("nova_dir = \"/tmp/user\"\nopenai_model = \"user-model\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".nova", "config.toml"), []byte("nova_dir = \"/tmp/ws\"\nopenai_model = \"ws-model\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.User.NovaDir != "" || layered.Workspace.NovaDir != "" {
		t.Fatalf("nova_dir should be filtered from editable layers: user=%q workspace=%q", layered.User.NovaDir, layered.Workspace.NovaDir)
	}
	if layered.Effective.NovaDir != normalizePath(home) {
		t.Fatalf("editable layers should not override startup nova_dir: %q", layered.Effective.NovaDir)
	}
	if layered.Effective.OpenAIModel != "ws-model" {
		t.Fatalf("other editable fields should still merge: %q", layered.Effective.OpenAIModel)
	}
}

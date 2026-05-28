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
	if s.InteractiveReplyTargetChars == nil || *s.InteractiveReplyTargetChars != 1200 {
		t.Fatalf("InteractiveReplyTargetChars default")
	}
	if s.InteractiveStageFontSize == nil || *s.InteractiveStageFontSize != 16 {
		t.Fatalf("InteractiveStageFontSize default")
	}
	if s.InteractiveStageLineHeight == nil || *s.InteractiveStageLineHeight != 1.78 {
		t.Fatalf("InteractiveStageLineHeight default")
	}
	if s.UIFontFamily != "system-sans" {
		t.Fatalf("UIFontFamily default: %s", s.UIFontFamily)
	}
	if s.ReadingFontFamily != "source-han-serif" {
		t.Fatalf("ReadingFontFamily default: %s", s.ReadingFontFamily)
	}
}

func TestMergeOverridesNonZero(t *testing.T) {
	parent := Settings{
		OpenAIBaseURL:               "https://parent",
		OpenAIModel:                 "p-model",
		MaxIteration:                intPtr(10),
		UIFontFamily:                "system-sans",
		ReadingFontFamily:           "source-han-serif",
		InteractiveReplyTargetChars: intPtr(1200),
		InteractiveMaxTokens:        intPtr(0),
		InteractiveStageFontSize:    intPtr(16),
		InteractiveStageLineHeight:  floatPtr(1.78),
	}
	child := Settings{
		OpenAIModel:                 "c-model", // override
		MaxIteration:                nil,       // 继承 parent
		UIFontFamily:                "humanist-sans",
		ReadingFontFamily:           "system-serif",
		InteractiveReplyTargetChars: intPtr(800),
		InteractiveMaxTokens:        intPtr(4000),
		InteractiveStageFontSize:    intPtr(18),
		InteractiveStageLineHeight:  floatPtr(1.95),
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
	if out.ReadingFontFamily != "system-serif" {
		t.Fatalf("ReadingFontFamily should override parent: %s", out.ReadingFontFamily)
	}
	if out.InteractiveReplyTargetChars == nil || *out.InteractiveReplyTargetChars != 800 {
		t.Fatalf("InteractiveReplyTargetChars should override parent")
	}
	if out.InteractiveMaxTokens == nil || *out.InteractiveMaxTokens != 4000 {
		t.Fatalf("InteractiveMaxTokens should override parent")
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

func TestMergeStyleRules(t *testing.T) {
	parent := Settings{StyleRules: []StyleRule{{Scene: "打斗", Styles: []string{"古龙.md"}}}}
	// nil 切片视为未设置，应继承
	out := Merge(parent, Settings{})
	if len(out.StyleRules) != 1 || out.StyleRules[0].Scene != "打斗" {
		t.Fatalf("nil child should inherit parent: %+v", out.StyleRules)
	}
	// 显式空切片视为清空
	out = Merge(parent, Settings{StyleRules: []StyleRule{}})
	if len(out.StyleRules) != 0 {
		t.Fatalf("empty slice should clear: %+v", out.StyleRules)
	}
	// 非空切片应整体覆盖
	out = Merge(parent, Settings{StyleRules: []StyleRule{{Scene: "对话", Styles: []string{"温吞.md"}}}})
	if len(out.StyleRules) != 1 || out.StyleRules[0].Scene != "对话" {
		t.Fatalf("non-empty child should override: %+v", out.StyleRules)
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
	in := Settings{OpenAIModel: "abc", AutoSaveEnabled: boolPtr(false)}
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

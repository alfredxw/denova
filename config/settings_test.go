package config

import (
	"encoding/json"
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
	if s.OpenAIContextWindowTokens == nil || *s.OpenAIContextWindowTokens != DefaultContextWindowTokens {
		t.Fatalf("OpenAIContextWindowTokens default")
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
	if s.ChapterFilenameFormat != "ch{order:05}-{chapter}-{title}.md" {
		t.Fatalf("ChapterFilenameFormat default: %s", s.ChapterFilenameFormat)
	}
	if s.VolumeDirFormat != "v{order:05}-{volume}" {
		t.Fatalf("VolumeDirFormat default: %s", s.VolumeDirFormat)
	}
	if s.InteractiveHotChoices == nil || *s.InteractiveHotChoices != true {
		t.Fatalf("InteractiveHotChoices default")
	}
	if s.AgentModels.ToolAgent.EnableThinking == nil || *s.AgentModels.ToolAgent.EnableThinking {
		t.Fatalf("ToolAgent thinking should default off")
	}
	if s.UIFontFamily != "apple-system" {
		t.Fatalf("UIFontFamily default: %s", s.UIFontFamily)
	}
	if s.UIFontSize == nil || *s.UIFontSize != 14 {
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
	if s.Theme != "dark" {
		t.Fatalf("Theme default: %s", s.Theme)
	}
	if s.MotionIntensity != "system" {
		t.Fatalf("MotionIntensity default: %s", s.MotionIntensity)
	}
	if s.UpdateCheckEnabled == nil || *s.UpdateCheckEnabled != true {
		t.Fatalf("UpdateCheckEnabled default")
	}
	if s.BackendPort == nil || *s.BackendPort != 8080 {
		t.Fatalf("BackendPort default")
	}
	if s.FrontendPort == nil || *s.FrontendPort != 5173 {
		t.Fatalf("FrontendPort default")
	}
	if s.AllowLANAccess == nil || *s.AllowLANAccess {
		t.Fatalf("AllowLANAccess should default off")
	}
}

func TestMergeOverridesNonZero(t *testing.T) {
	parent := Settings{
		OpenAIBaseURL:              "https://parent",
		OpenAIModel:                "p-model",
		OpenAIContextWindowTokens:  intPtr(DefaultContextWindowTokens),
		MaxIteration:               intPtr(10),
		UIFontFamily:               "apple-system",
		UIFontSize:                 intPtr(14),
		ReadingFontFamily:          "source-han-serif",
		ReadingFontSize:            intPtr(18),
		Language:                   "auto",
		Theme:                      "dark",
		MotionIntensity:            "system",
		UpdateCheckEnabled:         boolPtr(true),
		ChapterFilenameFormat:      "old-chapter",
		VolumeDirFormat:            "old-volume",
		BackendPort:                intPtr(8080),
		FrontendPort:               intPtr(5173),
		AllowLANAccess:             boolPtr(false),
		InteractiveMaxTokens:       intPtr(0),
		InteractiveHotChoices:      boolPtr(true),
		InteractiveStageFontSize:   intPtr(16),
		InteractiveStageLineHeight: floatPtr(1.78),
	}
	child := Settings{
		OpenAIModel:                "c-model", // override
		OpenAIContextWindowTokens:  intPtr(1000000),
		MaxIteration:               nil, // 继承 parent
		UIFontFamily:               "humanist-sans",
		UIFontSize:                 intPtr(13),
		ReadingFontFamily:          "system-serif",
		ReadingFontSize:            intPtr(20),
		Language:                   "en-US",
		Theme:                      "light",
		MotionIntensity:            "reduced",
		UpdateCheckEnabled:         boolPtr(false),
		ChapterFilenameFormat:      "new-chapter",
		VolumeDirFormat:            "new-volume",
		BackendPort:                intPtr(18080),
		FrontendPort:               intPtr(15173),
		AllowLANAccess:             boolPtr(true),
		RemoteAccessUsername:       "reader",
		RemoteAccessPasswordHash:   "$2a$10$hash",
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
	if out.OpenAIContextWindowTokens == nil || *out.OpenAIContextWindowTokens != 1000000 {
		t.Fatalf("OpenAIContextWindowTokens should override parent")
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
	if out.Theme != "light" {
		t.Fatalf("Theme should override parent: %s", out.Theme)
	}
	if out.MotionIntensity != "reduced" {
		t.Fatalf("MotionIntensity should override parent: %s", out.MotionIntensity)
	}
	if out.UpdateCheckEnabled == nil || *out.UpdateCheckEnabled != false {
		t.Fatalf("UpdateCheckEnabled should override parent")
	}
	if out.ChapterFilenameFormat != "new-chapter" || out.VolumeDirFormat != "new-volume" {
		t.Fatalf("filename formats should override parent: %#v", out)
	}
	if out.BackendPort == nil || *out.BackendPort != 18080 {
		t.Fatalf("BackendPort should override parent")
	}
	if out.FrontendPort == nil || *out.FrontendPort != 15173 {
		t.Fatalf("FrontendPort should override parent")
	}
	if out.AllowLANAccess == nil || !*out.AllowLANAccess {
		t.Fatalf("AllowLANAccess should override parent")
	}
	if out.RemoteAccessUsername != "reader" || out.RemoteAccessPasswordHash == "" || !out.RemoteAccessPasswordSet {
		t.Fatalf("remote access credentials should override parent: %#v", out)
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

func TestWriteSettingsFileFiltersInvalidTheme(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", Theme: "neon"}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Theme != "" {
		t.Fatalf("invalid theme should be filtered: %q", out.Theme)
	}
}

func TestWriteSettingsFileFiltersInvalidMotionIntensity(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", MotionIntensity: "chaotic"}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.MotionIntensity != "" {
		t.Fatalf("invalid motion intensity should be filtered: %q", out.MotionIntensity)
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

func TestWriteSettingsFileFiltersInvalidBackendPort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", BackendPort: intPtr(70000)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.BackendPort != nil {
		t.Fatalf("invalid backend_port should be filtered: %v", *out.BackendPort)
	}
}

func TestWriteSettingsFileFiltersInvalidFrontendPort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", FrontendPort: intPtr(70000)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.FrontendPort != nil {
		t.Fatalf("invalid frontend_port should be filtered: %v", *out.FrontendPort)
	}
}

func TestPrepareUserSettingsForWriteHashesRemoteAccessPassword(t *testing.T) {
	enabled := true
	prepared, err := PrepareUserSettingsForWrite(Settings{}, Settings{
		AllowLANAccess:       &enabled,
		RemoteAccessUsername: " reader ",
		RemoteAccessPassword: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RemoteAccessUsername != "reader" {
		t.Fatalf("username should be trimmed: %q", prepared.RemoteAccessUsername)
	}
	if prepared.RemoteAccessPassword != "" {
		t.Fatalf("plain password should be cleared")
	}
	if prepared.RemoteAccessPasswordHash == "" || !prepared.RemoteAccessPasswordSet {
		t.Fatalf("password hash should be set: %#v", prepared)
	}
	if !CheckRemoteAccessPassword(prepared.RemoteAccessPasswordHash, "secret") {
		t.Fatalf("password hash should verify")
	}
	data, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "remote_access_password_hash") {
		t.Fatalf("password hash should not be exposed in JSON: %s", string(data))
	}
}

func TestPrepareUserSettingsForWritePreservesRemoteAccessPasswordHash(t *testing.T) {
	enabled := true
	existing := Settings{RemoteAccessPasswordHash: "$2a$10$existing", RemoteAccessPasswordSet: true}
	prepared, err := PrepareUserSettingsForWrite(existing, Settings{
		AllowLANAccess:       &enabled,
		RemoteAccessUsername: "reader",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.RemoteAccessPasswordHash != existing.RemoteAccessPasswordHash {
		t.Fatalf("password hash should be preserved")
	}
}

func TestPrepareUserSettingsForWriteRejectsEnabledRemoteAccessWithoutCredentials(t *testing.T) {
	enabled := true
	if _, err := PrepareUserSettingsForWrite(Settings{}, Settings{AllowLANAccess: &enabled}); err == nil {
		t.Fatalf("enabled remote access should require credentials")
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

func TestLoadLayeredIgnoresStartupPortsFromWorkspaceLayer(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(home, "config.toml"), Settings{BackendPort: intPtr(18080), FrontendPort: intPtr(15173)}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".nova", "config.toml"), []byte("backend_port = 19090\nfrontend_port = 16173\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Workspace.BackendPort != nil {
		t.Fatalf("workspace backend_port should be filtered")
	}
	if layered.Workspace.FrontendPort != nil {
		t.Fatalf("workspace frontend_port should be filtered")
	}
	if layered.Effective.BackendPort == nil || *layered.Effective.BackendPort != 18080 {
		t.Fatalf("user backend_port should remain effective")
	}
	if layered.Effective.FrontendPort == nil || *layered.Effective.FrontendPort != 15173 {
		t.Fatalf("user frontend_port should remain effective")
	}
	if !strings.HasSuffix(layered.Access.LocalURL, ":15173") || !strings.HasSuffix(layered.Access.LANURL, ":15173") {
		t.Fatalf("access URLs should use frontend_port: %+v", layered.Access)
	}
}

func TestLoadLayeredIgnoresRemoteAccessFromWorkspaceLayer(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(home, "config.toml"), Settings{
		AllowLANAccess:           boolPtr(true),
		RemoteAccessUsername:     "user",
		RemoteAccessPasswordHash: "$2a$10$user",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".nova", "config.toml"), []byte("allow_lan_access = false\nremote_access_username = \"workspace\"\nremote_access_password_hash = \"workspace-hash\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Workspace.AllowLANAccess != nil || layered.Workspace.RemoteAccessUsername != "" || layered.Workspace.RemoteAccessPasswordHash != "" {
		t.Fatalf("workspace remote access settings should be filtered: %#v", layered.Workspace)
	}
	if layered.Effective.AllowLANAccess == nil || !*layered.Effective.AllowLANAccess || layered.Effective.RemoteAccessUsername != "user" {
		t.Fatalf("user remote access settings should remain effective: %#v", layered.Effective)
	}
}

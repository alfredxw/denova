package config

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if s.VersionTimedEnabled == nil || !*s.VersionTimedEnabled {
		t.Fatalf("VersionTimedEnabled should default on")
	}
	if s.VersionTimedIntervalMinutes == nil || *s.VersionTimedIntervalMinutes != 10 {
		t.Fatalf("VersionTimedIntervalMinutes should default to 10")
	}
	if s.HideChapterBodyLiveOutput == nil || *s.HideChapterBodyLiveOutput {
		t.Fatalf("HideChapterBodyLiveOutput should default off")
	}
	if s.MaxIteration != nil {
		t.Fatalf("MaxIteration should default to unset")
	}
	if s.AgentIdleTimeoutSeconds == nil || *s.AgentIdleTimeoutSeconds != DefaultAgentIdleTimeoutSeconds {
		t.Fatalf("AgentIdleTimeoutSeconds default")
	}
	if s.AgentToolResultLimitKB == nil || *s.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("AgentToolResultLimitKB default")
	}
	if s.AgentToolParallelism == nil || *s.AgentToolParallelism != DefaultAgentToolParallelism {
		t.Fatalf("AgentToolParallelism default")
	}
	if len(s.TerminalCommands) != 2 ||
		s.TerminalCommands[0] != (TerminalCommandSettings{ID: "codex", Name: "Codex CLI", Command: DefaultTerminalCodexCommand, Enabled: true}) ||
		s.TerminalCommands[1] != (TerminalCommandSettings{ID: "claude", Name: "Claude Code", Command: DefaultTerminalClaudeCommand, Enabled: true}) {
		t.Fatalf("terminal CLI defaults: %#v", s.TerminalCommands)
	}
	if s.TraceCaptureLevel != DefaultTraceCaptureLevel || s.TraceExporter != DefaultTraceExporter {
		t.Fatalf("trace defaults: capture=%q exporter=%q", s.TraceCaptureLevel, s.TraceExporter)
	}
	if s.TraceRetentionRuns == nil || *s.TraceRetentionRuns != DefaultTraceRetentionRuns {
		t.Fatalf("TraceRetentionRuns default")
	}
	if s.InteractiveStageFontSize == nil || *s.InteractiveStageFontSize != 16 {
		t.Fatalf("InteractiveStageFontSize default")
	}
	if s.InteractiveStageLineHeight == nil || *s.InteractiveStageLineHeight != 1.78 {
		t.Fatalf("InteractiveStageLineHeight default")
	}
	if s.IDEStoryTellerID != "rhythm" || s.InteractiveStoryTellerID != "rhythm" {
		t.Fatalf("narrative style defaults: writing=%q game=%q", s.IDEStoryTellerID, s.InteractiveStoryTellerID)
	}
	if s.ChapterFilenameFormat != "ch{order:05}-{chapter}-{title}.md" {
		t.Fatalf("ChapterFilenameFormat default: %s", s.ChapterFilenameFormat)
	}
	if s.VolumeDirFormat != "v{order:05}-{volume}" {
		t.Fatalf("VolumeDirFormat default: %s", s.VolumeDirFormat)
	}
	if s.AgentModels.IDE.EnableThinking == nil || !*s.AgentModels.IDE.EnableThinking {
		t.Fatalf("IDE thinking should default on")
	}
	if s.AgentModels.ConfigManager.EnableThinking == nil || !*s.AgentModels.ConfigManager.EnableThinking {
		t.Fatalf("ConfigManager thinking should default on")
	}
	if s.AgentModels.InteractiveStory.EnableThinking == nil || *s.AgentModels.InteractiveStory.EnableThinking {
		t.Fatalf("InteractiveStory extended thinking should default off")
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
	if s.WritingSkillDefault != DefaultWritingSkillName {
		t.Fatalf("WritingSkillDefault default: %s", s.WritingSkillDefault)
	}
	if s.IDEImagePresetID != "game-cg" {
		t.Fatalf("IDEImagePresetID default: %s", s.IDEImagePresetID)
	}
	if len(s.SubAgents) != 0 {
		t.Fatalf("SubAgents should come from editable config layers, not Go defaults: %#v", s.SubAgents)
	}
	if s.GeneralSubAgents.Default == nil || *s.GeneralSubAgents.Default {
		t.Fatalf("GeneralSubAgents default fallback should be disabled")
	}
	if s.GeneralSubAgents.IDE == nil || !*s.GeneralSubAgents.IDE {
		t.Fatalf("GeneralSubAgents should default enabled for IDE")
	}
	if s.GeneralSubAgents.Automation == nil || !*s.GeneralSubAgents.Automation {
		t.Fatalf("GeneralSubAgents should default enabled for automation")
	}
	if s.GeneralSubAgents.InteractiveStory != nil || s.GeneralSubAgents.ConfigManager != nil {
		t.Fatalf("GeneralSubAgents should not explicitly enable interactive story or config manager by default")
	}
}

func TestMergeOverridesNonZero(t *testing.T) {
	parent := Settings{
		OpenAIBaseURL:              "https://parent",
		OpenAIModel:                "p-model",
		OpenAIContextWindowTokens:  intPtr(DefaultContextWindowTokens),
		MaxIteration:               intPtr(10),
		AgentIdleTimeoutSeconds:    intPtr(120),
		AgentToolResultLimitKB:     intPtr(0),
		AgentToolParallelism:       intPtr(4),
		UIFontFamily:               "apple-system",
		UIFontSize:                 intPtr(14),
		ReadingFontFamily:          "source-han-serif",
		ReadingFontSize:            intPtr(18),
		Language:                   "auto",
		Theme:                      "dark",
		MotionIntensity:            "system",
		UpdateCheckEnabled:         boolPtr(true),
		HideChapterBodyLiveOutput:  boolPtr(false),
		ChapterFilenameFormat:      "old-chapter",
		VolumeDirFormat:            "old-volume",
		BackendPort:                intPtr(8080),
		FrontendPort:               intPtr(5173),
		AllowLANAccess:             boolPtr(false),
		WritingSkillDefault:        "novel-standard",
		IDEImagePresetID:           "realistic",
		InteractiveStageFontSize:   intPtr(16),
		InteractiveStageLineHeight: floatPtr(1.78),
	}
	child := Settings{
		OpenAIModel:                "c-model", // override
		OpenAIContextWindowTokens:  intPtr(1000000),
		MaxIteration:               nil, // 继承 parent
		AgentIdleTimeoutSeconds:    intPtr(240),
		AgentToolResultLimitKB:     intPtr(64),
		AgentToolParallelism:       intPtr(12),
		UIFontFamily:               "humanist-sans",
		UIFontSize:                 intPtr(13),
		ReadingFontFamily:          "system-serif",
		ReadingFontSize:            intPtr(20),
		Language:                   "en-US",
		Theme:                      "light",
		MotionIntensity:            "reduced",
		UpdateCheckEnabled:         boolPtr(false),
		HideChapterBodyLiveOutput:  boolPtr(true),
		ChapterFilenameFormat:      "new-chapter",
		VolumeDirFormat:            "new-volume",
		BackendPort:                intPtr(18080),
		FrontendPort:               intPtr(15173),
		AllowLANAccess:             boolPtr(true),
		WritingSkillDefault:        "scene-first",
		IDEImagePresetID:           "2d-illustration",
		RemoteAccessUsername:       "reader",
		RemoteAccessPasswordHash:   "$2a$10$hash",
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
	if out.AgentIdleTimeoutSeconds == nil || *out.AgentIdleTimeoutSeconds != 240 {
		t.Fatalf("AgentIdleTimeoutSeconds should override parent")
	}
	if out.AgentToolResultLimitKB == nil || *out.AgentToolResultLimitKB != 64 {
		t.Fatalf("AgentToolResultLimitKB should override parent")
	}
	if out.AgentToolParallelism == nil || *out.AgentToolParallelism != 12 {
		t.Fatalf("AgentToolParallelism should override parent")
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
	if out.HideChapterBodyLiveOutput == nil || *out.HideChapterBodyLiveOutput != true {
		t.Fatalf("HideChapterBodyLiveOutput should override parent")
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
	if out.WritingSkillDefault != "scene-first" {
		t.Fatalf("WritingSkillDefault should override parent: %s", out.WritingSkillDefault)
	}
	if out.IDEImagePresetID != "2d-illustration" {
		t.Fatalf("IDEImagePresetID should override parent: %s", out.IDEImagePresetID)
	}
	if out.RemoteAccessUsername != "reader" || out.RemoteAccessPasswordHash == "" || !out.RemoteAccessPasswordSet {
		t.Fatalf("remote access credentials should override parent: %#v", out)
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

func TestMergeTerminalCommandsReplacesRegistryInConfiguredOrder(t *testing.T) {
	parent := Settings{TerminalCommands: DefaultTerminalCommands()}
	child := Settings{TerminalCommands: []TerminalCommandSettings{
		{ID: "aider", Name: "Aider", Command: "aider --model sonnet", Enabled: true},
		{ID: "codex", Name: "Codex Nightly", Command: "codex --full-auto", Enabled: false},
	}}
	out := Merge(parent, child)
	if len(out.TerminalCommands) != 2 || out.TerminalCommands[0] != child.TerminalCommands[0] || out.TerminalCommands[1] != child.TerminalCommands[1] {
		t.Fatalf("terminal commands should override: %#v", out)
	}
}

func TestWriteSettingsFileMigratesAndTrimsLegacyTerminalCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	in := Settings{
		TerminalCodexCommand:  "  codex --full-auto  ",
		TerminalClaudeCommand: "  claude --resume  ",
	}
	if err := WriteSettingsFile(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.TerminalCommands) != 2 || out.TerminalCommands[0].Command != "codex --full-auto" || out.TerminalCommands[1].Command != "claude --resume" {
		t.Fatalf("legacy terminal commands should migrate and trim: %#v", out.TerminalCommands)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "terminal_codex_command") || strings.Contains(string(data), "terminal_claude_command") {
		t.Fatalf("legacy terminal command keys should not be written back: %s", data)
	}
}

func TestPrepareUserSettingsForWriteAcceptsUnboundedTerminalRegistry(t *testing.T) {
	commands := make([]TerminalCommandSettings, 48)
	for index := range commands {
		commands[index] = TerminalCommandSettings{
			ID:      fmt.Sprintf("cli-%d", index),
			Name:    fmt.Sprintf("CLI %d", index),
			Command: fmt.Sprintf("cli-%d --interactive", index),
			Enabled: true,
		}
	}
	prepared, err := PrepareUserSettingsForWrite(Settings{}, Settings{TerminalCommands: commands})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.TerminalCommands) != len(commands) {
		t.Fatalf("terminal registry was truncated: got=%d want=%d", len(prepared.TerminalCommands), len(commands))
	}
}

func TestPrepareUserSettingsForWritePreservesTerminalRegistryForOlderClients(t *testing.T) {
	existing := Settings{TerminalCommands: []TerminalCommandSettings{
		{ID: "aider", Name: "Aider", Command: "aider", Enabled: true},
	}}
	prepared, err := PrepareUserSettingsForWrite(existing, Settings{Language: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.TerminalCommands) != 1 || prepared.TerminalCommands[0] != existing.TerminalCommands[0] {
		t.Fatalf("older client erased terminal registry: %#v", prepared.TerminalCommands)
	}
}

func TestPrepareUserSettingsForWriteRejectsAmbiguousTerminalRegistry(t *testing.T) {
	_, err := PrepareUserSettingsForWrite(Settings{}, Settings{TerminalCommands: []TerminalCommandSettings{
		{ID: "aider", Name: "Aider", Command: "aider", Enabled: true},
		{ID: "aider", Name: "Other", Command: "other", Enabled: true},
	}})
	if !errors.Is(err, ErrInvalidTerminalCommand) {
		t.Fatalf("duplicate terminal IDs should fail with ErrInvalidTerminalCommand, got %v", err)
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

func TestWriteSettingsFileNormalizesAgentIdleTimeout(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AgentIdleTimeoutSeconds: intPtr(7200)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentIdleTimeoutSeconds == nil || *out.AgentIdleTimeoutSeconds != 7200 {
		t.Fatalf("agent idle timeout should preserve positive values, got %v", out.AgentIdleTimeoutSeconds)
	}
}

func TestWriteSettingsFileAllowsUnlimitedAgentIdleTimeout(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AgentIdleTimeoutSeconds: intPtr(0)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentIdleTimeoutSeconds == nil || *out.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("agent idle timeout should preserve explicit 0, got %v", out.AgentIdleTimeoutSeconds)
	}
}

func TestWriteSettingsFileFiltersNegativeAgentIdleTimeout(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AgentIdleTimeoutSeconds: intPtr(-1)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentIdleTimeoutSeconds != nil {
		t.Fatalf("negative agent idle timeout should be filtered, got %v", *out.AgentIdleTimeoutSeconds)
	}
}

func TestWriteSettingsFileMapsZeroToolResultLimitToHighDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AgentToolResultLimitKB: intPtr(0)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentToolResultLimitKB == nil || *out.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("agent tool result limit should persist the high default, got %v", out.AgentToolResultLimitKB)
	}
}

func TestWriteSettingsFileFiltersNegativeAgentToolResultLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	in := Settings{OpenAIModel: "abc", AgentToolResultLimitKB: intPtr(-1)}
	if err := WriteSettingsFile(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.AgentToolResultLimitKB != nil {
		t.Fatalf("negative agent tool result limit should be filtered, got %v", *out.AgentToolResultLimitKB)
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

func TestLoadLayeredKeepsGeneralSettingsUserScopedAndAppliesWorkspaceAgentOverrides(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}

	user := Settings{OpenAIModel: "user-model", MaxIteration: intPtr(20)}
	wsCfg := Settings{
		OpenAIModel: "ws-model",
		AgentTools: AgentToolSettings{
			IDE: AgentToolOverride{AgentToolShell: false},
		},
	}
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
	if layered.Effective.OpenAIModel != "user-model" {
		t.Fatalf("general settings should stay user-scoped: %s", layered.Effective.OpenAIModel)
	}
	if layered.Effective.MaxIteration == nil || *layered.Effective.MaxIteration != 20 {
		t.Fatalf("user MaxIteration should inherit: %v", layered.Effective.MaxIteration)
	}
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("raw user should be preserved")
	}
	if layered.Workspace.OpenAIModel != "" {
		t.Fatalf("workspace general setting should be filtered: %s", layered.Workspace.OpenAIModel)
	}
	if enabled, present := layered.Effective.AgentTools.IDE[AgentToolShell]; !present || enabled {
		t.Fatalf("workspace Agent override should remain effective: %#v", layered.Effective.AgentTools.IDE)
	}
}

func TestLoadLayeredPublishesResolvedAgentToolCatalogAndManifests(t *testing.T) {
	novaDir := t.TempDir()
	if err := WriteSettingsFile(UserConfigPath(novaDir), Settings{
		AgentTools: AgentToolSettings{IDE: AgentToolOverride{AgentToolShell: false}},
	}); err != nil {
		t.Fatal(err)
	}
	layered, err := LoadLayered(novaDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(layered.AgentToolCapabilities) != len(AgentToolCapabilities()) {
		t.Fatalf("tool capability catalog length = %d, want %d", len(layered.AgentToolCapabilities), len(AgentToolCapabilities()))
	}
	ide, found := layered.ResolvedAgentToolManifests[AgentKindIDE]
	if !found || len(ide) == 0 {
		t.Fatalf("resolved IDE manifest is missing: %#v", layered.ResolvedAgentToolManifests)
	}
	shell, found := resolvedManifestCapability(ide, AgentToolShell)
	if !found || shell.Allowed || shell.Availability != AgentToolAvailabilityUnavailable ||
		shell.UnavailableReasonKey != AgentToolUnavailableDisabledByPolicy {
		t.Fatalf("resolved IDE shell = %#v", shell)
	}
	wantShell := "bash"
	if layered.Runtime.GOOS == "windows" {
		wantShell = "pwsh"
	}
	if len(shell.ToolNames) != 1 || shell.ToolNames[0] != wantShell {
		t.Fatalf("resolved shell tools = %#v, want [%s]", shell.ToolNames, wantShell)
	}
	for _, kind := range []string{AgentKindVersionSummary, AgentKindToolAgent, AgentKindContextCompaction} {
		manifest, present := layered.ResolvedAgentToolManifests[kind]
		if !present || manifest == nil || len(manifest) != 0 {
			t.Fatalf("model-only manifest %q = %#v, present=%v", kind, manifest, present)
		}
	}
}

func TestLoadLayeredPublishesCanonicalResolvedAgentContexts(t *testing.T) {
	novaDir := t.TempDir()
	lowThreshold := 0.20
	disableToolContext := false
	if err := WriteSettingsFile(UserConfigPath(novaDir), Settings{
		AgentContexts: AgentContextSettings{
			Default: AgentContextOverride{CompactionThreshold: &lowThreshold},
			IDE:     AgentContextOverride{ToolResultContextEnabled: &disableToolContext},
		},
	}); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(novaDir, "")
	if err != nil {
		t.Fatal(err)
	}
	ide, found := layered.ResolvedAgentContexts[AgentKindIDE]
	if !found {
		t.Fatalf("resolved IDE context is missing: %#v", layered.ResolvedAgentContexts)
	}
	if ide.CompactionThreshold != 0.50 || ide.ToolResultContextEnabled {
		t.Fatalf("resolved IDE context = %#v", ide)
	}
	for _, definition := range AgentKindDefinitions() {
		if _, ok := layered.ResolvedAgentContexts[definition.Kind]; !ok {
			t.Fatalf("resolved context is missing agent kind %q", definition.Kind)
		}
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
	if layered.Effective.OpenAIModel != "user-model" {
		t.Fatalf("workspace general fields should not override user settings: %q", layered.Effective.OpenAIModel)
	}
}

func TestPrepareWorkspaceAgentSettingsForWritePreservesLegacyGeneralValues(t *testing.T) {
	existing := Settings{
		OpenAIModel: "legacy-workspace-model",
		AgentTools: AgentToolSettings{
			IDE: AgentToolOverride{AgentToolShell: true},
		},
	}
	incoming := Settings{
		OpenAIModel: "ignored-new-model",
		AgentModels: AgentModelSettings{
			IDE: AgentModelOverride{ProfileID: "ignored-workspace-profile"},
		},
		AgentTools: AgentToolSettings{
			IDE: AgentToolOverride{AgentToolShell: false},
		},
	}

	prepared := PrepareWorkspaceAgentSettingsForWrite(existing, incoming)
	if prepared.OpenAIModel != "legacy-workspace-model" {
		t.Fatalf("legacy general value should remain reversible on disk: %q", prepared.OpenAIModel)
	}
	if prepared.AgentModels.IDE.ProfileID != "" {
		t.Fatalf("workspace model selection must remain user-scoped: %#v", prepared.AgentModels)
	}
	if enabled, present := prepared.AgentTools.IDE[AgentToolShell]; !present || enabled {
		t.Fatalf("workspace Agent override should be replaced: %#v", prepared.AgentTools.IDE)
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
	if !strings.HasSuffix(layered.Access.LocalURL, ":18080") || !strings.HasSuffix(layered.Access.LANURL, ":18080") {
		t.Fatalf("access URLs should use backend_port: %+v", layered.Access)
	}
}

func TestLoadLayeredIgnoresAgentModelsFromWorkspaceLayer(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(home, "config.toml"), Settings{
		AgentModels: AgentModelSettings{InteractiveStory: AgentModelOverride{ProfileID: "user-model"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(ws, ".nova", "config.toml"), Settings{
		AgentModels: AgentModelSettings{InteractiveStory: AgentModelOverride{ProfileID: "workspace-model"}},
	}); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Workspace.AgentModels.InteractiveStory.ProfileID != "" {
		t.Fatalf("workspace agent model should be filtered: %#v", layered.Workspace.AgentModels)
	}
	if layered.Effective.AgentModels.InteractiveStory.ProfileID != "user-model" {
		t.Fatalf("user agent model should remain effective: %#v", layered.Effective.AgentModels)
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

func TestLoadLayeredIgnoresLLMInputLogFromWorkspaceLayer(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".nova", "config.toml"), []byte("llm_input_log_enabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Workspace.LLMInputLogEnabled != nil {
		t.Fatalf("workspace llm input log setting should be filtered")
	}
	if layered.Effective.LLMInputLogEnabled == nil || *layered.Effective.LLMInputLogEnabled {
		t.Fatalf("workspace llm input log should not become effective: %#v", layered.Effective.LLMInputLogEnabled)
	}
}

func TestLoadLayeredIgnoresTraceDebugSettingsFromWorkspaceLayer(t *testing.T) {
	home := t.TempDir()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("trace_capture_level = \"debug\"\ntrace_exporter = \"local\"\ntrace_retention_runs = 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".nova", "config.toml"), []byte("trace_capture_level = \"off\"\ntrace_exporter = \"otlp\"\ntrace_retention_runs = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	layered, err := LoadLayered(home, ws)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Workspace.TraceCaptureLevel != "" || layered.Workspace.TraceExporter != "" || layered.Workspace.TraceRetentionRuns != nil {
		t.Fatalf("workspace trace debug settings should be filtered: %#v", layered.Workspace)
	}
	if layered.Effective.TraceCaptureLevel != "debug" || layered.Effective.TraceExporter != "local" || layered.Effective.TraceRetentionRuns == nil || *layered.Effective.TraceRetentionRuns != 7 {
		t.Fatalf("user trace debug settings should remain effective: %#v", layered.Effective)
	}
}

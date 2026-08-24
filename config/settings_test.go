package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
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
	if s.ProjectFileTreeEntryLimit == nil || *s.ProjectFileTreeEntryLimit != DefaultProjectFileTreeEntryLimit {
		t.Fatalf("ProjectFileTreeEntryLimit default")
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
	if s.AgentModels.IDE.ThinkingLevel != string(providers.ThinkingLevelMedium) {
		t.Fatalf("IDE thinking level = %q, want medium", s.AgentModels.IDE.ThinkingLevel)
	}
	if s.AgentModels.ConfigManager.ThinkingLevel != string(providers.ThinkingLevelMedium) {
		t.Fatalf("ConfigManager thinking level = %q, want medium", s.AgentModels.ConfigManager.ThinkingLevel)
	}
	if s.AgentModels.InteractiveStory.ThinkingLevel != string(providers.ThinkingLevelMinimal) {
		t.Fatalf("InteractiveStory thinking level = %q, want minimal", s.AgentModels.InteractiveStory.ThinkingLevel)
	}
	if s.AgentModels.ToolAgent.ThinkingLevel != string(providers.ThinkingLevelOff) {
		t.Fatalf("ToolAgent thinking level = %q, want off", s.AgentModels.ToolAgent.ThinkingLevel)
	}
	if s.UIFontFamily != "apple-system" {
		t.Fatalf("UIFontFamily default: %s", s.UIFontFamily)
	}
	if s.UIFontSize == nil || *s.UIFontSize != 14 {
		t.Fatalf("UIFontSize default")
	}
	if s.ReadingFontFamily != "apple-system" {
		t.Fatalf("ReadingFontFamily default: %s", s.ReadingFontFamily)
	}
	if s.ReadingFontSize == nil || *s.ReadingFontSize != 18 {
		t.Fatalf("ReadingFontSize default")
	}
	if s.SourceEditorFontFamily != "mono" {
		t.Fatalf("SourceEditorFontFamily default: %s", s.SourceEditorFontFamily)
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
		ReadingFontFamily:          "apple-system",
		ReadingFontSize:            intPtr(18),
		SourceEditorFontFamily:     "mono",
		Language:                   "auto",
		Theme:                      "dark",
		MotionIntensity:            "system",
		UpdateCheckEnabled:         boolPtr(true),
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
		SourceEditorFontFamily:     "custom:Sarasa Mono SC",
		Language:                   "en-US",
		Theme:                      "light",
		MotionIntensity:            "reduced",
		UpdateCheckEnabled:         boolPtr(false),
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
	if out.SourceEditorFontFamily != "custom:Sarasa Mono SC" {
		t.Fatalf("SourceEditorFontFamily should override parent: %s", out.SourceEditorFontFamily)
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

func TestMergeTerminalCommandsPreservesExplicitlyEmptyRegistry(t *testing.T) {
	child := Settings{TerminalCommands: []TerminalCommandSettings{}}
	out := Merge(Settings{TerminalCommands: DefaultTerminalCommands()}, child)
	if out.TerminalCommands == nil || len(out.TerminalCommands) != 0 {
		t.Fatalf("explicit empty terminal registry should override defaults: %#v", out.TerminalCommands)
	}
}

func TestWriteSettingsFilePreservesExplicitlyEmptyTerminalRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, Settings{TerminalCommands: []TerminalCommandSettings{}}); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.TerminalCommands == nil || len(out.TerminalCommands) != 0 {
		t.Fatalf("explicit empty terminal registry was not preserved: %#v", out.TerminalCommands)
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

func TestWriteSettingsFileMapsZeroToolResultLimitToDefault(t *testing.T) {
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
		t.Fatalf("agent tool result limit should persist the default, got %v", out.AgentToolResultLimitKB)
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
	if layered.Inherited.User.OpenAIModel == "user-model" {
		t.Fatalf("user inheritance must exclude the user layer")
	}
	if layered.Inherited.Workspace.OpenAIModel != "user-model" {
		t.Fatalf("workspace inheritance must include the user layer: %s", layered.Inherited.Workspace.OpenAIModel)
	}
	if enabled, present := layered.Effective.AgentTools.IDE[AgentToolShell]; !present || enabled {
		t.Fatalf("workspace Agent override should remain effective: %#v", layered.Effective.AgentTools.IDE)
	}
}

func TestWithResolvedLabsNormalizesInheritedValues(t *testing.T) {
	invalidCap := 0
	resolved := withResolvedLabs(Settings{Labs: LabSettings{ContinualLearningTrajectoryCap: &invalidCap}})
	if got := resolved.Labs.ContinualLearningTrajectoryCap; got == nil || *got != DefaultContinualLearningTrajectoryCap {
		t.Fatalf("inherited trajectory cap = %v, want %d", got, DefaultContinualLearningTrajectoryCap)
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
	for _, kind := range []string{AgentKindVersionSummary, AgentKindToolAgent} {
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

func TestWorkspaceSettingsCannotOverrideUserSafetyOrShellEnvironment(t *testing.T) {
	prepared := PrepareWorkspaceAgentSettingsForWrite(Settings{}, Settings{
		AgentApprovalMode:     AgentApprovalFullAccess,
		ShellEnvironmentMode:  ShellEnvironmentProcess,
		ShellEnvironmentShell: "/tmp/untrusted-shell",
		AgentBashPath:         "/tmp/untrusted-bash",
		AgentPrompts: AgentPromptSettings{
			IDE: AgentPromptOverride{SystemPrompt: "workspace prompt"},
		},
	})
	if prepared.AgentApprovalMode != "" || prepared.ShellEnvironmentMode != "" ||
		prepared.ShellEnvironmentShell != "" || prepared.AgentBashPath != "" {
		t.Fatalf("workspace retained user-owned execution settings: %#v", prepared)
	}
	if prepared.AgentPrompts.IDE.SystemPrompt != "workspace prompt" {
		t.Fatalf("workspace Agent customization was lost: %#v", prepared.AgentPrompts)
	}
}

func TestLoadLayeredIgnoresPersistedWorkspaceSafetyOrShellEnvironment(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	if err := WriteSettingsFile(UserConfigPath(novaDir), Settings{
		AgentApprovalMode:     AgentApprovalAsk,
		ShellEnvironmentMode:  ShellEnvironmentAuto,
		ShellEnvironmentShell: "/bin/zsh",
		AgentBashPath:         "/bin/bash",
	}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(WorkspaceConfigPath(workspace), Settings{
		AgentApprovalMode:     AgentApprovalFullAccess,
		ShellEnvironmentMode:  ShellEnvironmentProcess,
		ShellEnvironmentShell: "/tmp/untrusted-shell",
		AgentBashPath:         "/tmp/untrusted-bash",
	}); err != nil {
		t.Fatal(err)
	}
	layered, err := LoadLayered(novaDir, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Effective.AgentApprovalMode != AgentApprovalAsk ||
		layered.Effective.ShellEnvironmentMode != ShellEnvironmentAuto ||
		layered.Effective.ShellEnvironmentShell != "/bin/zsh" ||
		layered.Effective.AgentBashPath != "/bin/bash" {
		t.Fatalf("workspace changed user execution settings: %#v", layered.Effective)
	}
	if layered.Workspace.AgentApprovalMode != "" || layered.Workspace.ShellEnvironmentMode != "" ||
		layered.Workspace.ShellEnvironmentShell != "" || layered.Workspace.AgentBashPath != "" {
		t.Fatalf("workspace safety settings leaked into public layer: %#v", layered.Workspace)
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

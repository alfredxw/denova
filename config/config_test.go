package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsDenovaDir(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DENOVA_DIR", "")
	t.Setenv("NOVA_DIR", "")

	cfg := Load()
	want := normalizePath("./.denova")
	if cfg.NovaDir != want {
		t.Fatalf("默认 NovaDir 不符合预期: want=%s got=%s", want, cfg.NovaDir)
	}
	if cfg.DenovaDir != want {
		t.Fatalf("默认 DenovaDir 不符合预期: want=%s got=%s", want, cfg.DenovaDir)
	}
}

func TestLoadDoesNotDefaultWorkspaceToCurrentDir(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DENOVA_DIR", "")
	t.Setenv("NOVA_DIR", "")
	t.Setenv("DENOVA_WORKSPACE", "")
	t.Setenv("NOVA_WORKSPACE", "")

	cfg := Load()
	if cfg.Workspace != "" {
		t.Fatalf("未显式指定 workspace 时不应默认打开当前目录: got=%s", cfg.Workspace)
	}
	if !cfg.ResumeLastWorkspace {
		t.Fatalf("未显式指定 workspace 时应允许恢复上次打开的书籍")
	}
}

func TestLoadNovaDirFromEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(t.TempDir(), "nova-data")
	t.Setenv("DENOVA_DIR", "")
	t.Setenv("NOVA_DIR", dir)

	cfg := Load()
	if cfg.NovaDir != dir {
		t.Fatalf("环境变量 NovaDir 不符合预期: want=%s got=%s", dir, cfg.NovaDir)
	}
	if cfg.DenovaDir != dir {
		t.Fatalf("环境变量 DenovaDir 不符合预期: want=%s got=%s", dir, cfg.DenovaDir)
	}
}

func TestLoadDenovaDirEnvOverridesLegacyNovaDir(t *testing.T) {
	t.Chdir(t.TempDir())
	denovaDir := filepath.Join(t.TempDir(), "denova-data")
	legacyDir := filepath.Join(t.TempDir(), "nova-data")
	t.Setenv("DENOVA_DIR", denovaDir)
	t.Setenv("NOVA_DIR", legacyDir)

	cfg := Load()
	if cfg.DenovaDir != denovaDir {
		t.Fatalf("DENOVA_DIR should override NOVA_DIR: want=%s got=%s", denovaDir, cfg.DenovaDir)
	}
	if cfg.NovaDir != denovaDir {
		t.Fatalf("legacy NovaDir should mirror DENOVA_DIR: want=%s got=%s", denovaDir, cfg.NovaDir)
	}
}

func TestNormalizePathExpandsRelativeAndHome(t *testing.T) {
	relative := "data/nova"
	abs, err := filepath.Abs(relative)
	if err != nil {
		t.Fatal(err)
	}
	if got := normalizePath(relative); got != abs {
		t.Fatalf("相对路径未转绝对路径: want=%s got=%s", abs, got)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("当前环境无 home 目录")
	}
	want := filepath.Join(home, ".nova")
	if got := normalizePath("~/.nova"); got != want {
		t.Fatalf("~ 路径未正确展开: want=%s got=%s", want, got)
	}
}

func TestLoadWithWorkspaceMergesLayers(t *testing.T) {
	novaDir := t.TempDir()
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", novaDir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"),
		Settings{OpenAIModel: "user-model", Language: "zh-CN", WritingSkillDefault: "novel-lite", IDEImagePresetID: "realistic"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(ws, ".nova", "config.toml"),
		Settings{OpenAIModel: "ws-model", Language: "en-US", WritingSkillDefault: "novel-heavy", IDEImagePresetID: "2d-illustration"}); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenAIModel != "ws-model" {
		t.Fatalf("Workspace override expected, got %s", cfg.OpenAIModel)
	}
	if cfg.Language != "en-US" {
		t.Fatalf("workspace language override expected, got %s", cfg.Language)
	}
	if cfg.WritingSkillDefault != "novel-heavy" {
		t.Fatalf("workspace writing skill default expected, got %s", cfg.WritingSkillDefault)
	}
	if cfg.IDEImagePresetID != "2d-illustration" {
		t.Fatalf("workspace image preset default expected, got %s", cfg.IDEImagePresetID)
	}
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("user layer raw value lost")
	}
}

func TestLoadWithWorkspaceAllowsUnlimitedAgentIdleTimeout(t *testing.T) {
	novaDir := t.TempDir()
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", novaDir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("NOVA_AGENT_IDLE_TIMEOUT_SECONDS", "")

	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"),
		Settings{AgentIdleTimeoutSeconds: intPtr(0)}); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("agent idle timeout should allow explicit 0, got %d", cfg.AgentIdleTimeoutSeconds)
	}
	if layered.Effective.AgentIdleTimeoutSeconds == nil || *layered.Effective.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("effective agent idle timeout should preserve explicit 0")
	}
}

func TestLoadWithWorkspaceMapsZeroToolResultLimitToHighDefault(t *testing.T) {
	novaDir := t.TempDir()
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", novaDir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"),
		Settings{AgentToolResultLimitKB: intPtr(0)}); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("agent tool result limit should map 0 to the high default, got %d", cfg.AgentToolResultLimitKB)
	}
	if layered.Effective.AgentToolResultLimitKB == nil || *layered.Effective.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("effective agent tool result limit should expose the high default")
	}
}

func TestLoadWithWorkspaceDefaultsLLMInputLogDisabled(t *testing.T) {
	novaDir := t.TempDir()
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", novaDir)

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLMInputLogEnabled {
		t.Fatalf("llm input log should default to disabled")
	}
	if layered.Effective.LLMInputLogEnabled == nil || *layered.Effective.LLMInputLogEnabled {
		t.Fatalf("effective llm input log should default to false: %#v", layered.Effective.LLMInputLogEnabled)
	}
}

func TestLoadWithWorkspaceReadsUserLLMInputLogSetting(t *testing.T) {
	novaDir := t.TempDir()
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", novaDir)
	enabled := true

	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"),
		Settings{LLMInputLogEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LLMInputLogEnabled {
		t.Fatalf("llm input log should read user setting")
	}
	if layered.Effective.LLMInputLogEnabled == nil || !*layered.Effective.LLMInputLogEnabled {
		t.Fatalf("effective llm input log should be true")
	}
}

func TestLoadWithWorkspaceUsesGlobalConfigNovaDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	novaDir := filepath.Join(root, "global-nova")
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("nova_dir = \"./global-nova\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(novaDir, "config.toml"), Settings{OpenAIModel: "user-model"}); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	wantNovaDir := normalizePath("./global-nova")
	if cfg.NovaDir != wantNovaDir {
		t.Fatalf("global nova_dir should locate user config: want=%s got=%s", wantNovaDir, cfg.NovaDir)
	}
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("user config should be loaded from global nova_dir")
	}
}

func TestLoadWithWorkspaceUsesGlobalConfigAsBaseLayer(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("openai_model = \"global-model\"\nskills_dir = \"./global-skills\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenAIModel != "global-model" {
		t.Fatalf("global config should be effective when user/workspace unset: %s", cfg.OpenAIModel)
	}
	if layered.Global.OpenAIModel != "global-model" {
		t.Fatalf("global layer should be exposed: %s", layered.Global.OpenAIModel)
	}
}

func TestLoadWithWorkspaceAllowsGlobalUnlimitedAgentIdleTimeout(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("agent_idle_timeout_seconds = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("global agent idle timeout should allow explicit 0, got %d", cfg.AgentIdleTimeoutSeconds)
	}
	if layered.Global.AgentIdleTimeoutSeconds == nil || *layered.Global.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("global layer should preserve explicit 0")
	}
}

func TestLoadWithWorkspaceMapsGlobalZeroToolResultLimitToHighDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("agent_tool_result_limit_kb = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("global agent tool result limit should map 0 to the high default, got %d", cfg.AgentToolResultLimitKB)
	}
	if layered.Global.AgentToolResultLimitKB == nil || *layered.Global.AgentToolResultLimitKB != DefaultAgentToolResultLimitKB {
		t.Fatalf("global layer should expose the high default")
	}
}

func TestLoadWithWorkspaceUsesConfiguredStartupPorts(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	ws := t.TempDir()
	t.Setenv("NOVA_DIR", "")
	t.Setenv("NOVA_BACKEND_PORT", "")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("backend_port = 18080\nfrontend_port = 15173\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, layered, err := LoadWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BackendPort != 18080 {
		t.Fatalf("global backend_port should be effective: %d", cfg.BackendPort)
	}
	if layered.Effective.BackendPort == nil || *layered.Effective.BackendPort != 18080 {
		t.Fatalf("effective backend_port should be exposed")
	}
	if cfg.FrontendPort != 15173 {
		t.Fatalf("global frontend_port should be effective: %d", cfg.FrontendPort)
	}
	if layered.Effective.FrontendPort == nil || *layered.Effective.FrontendPort != 15173 {
		t.Fatalf("effective frontend_port should be exposed")
	}
}

func TestLoadStartupPortEnvOverridesConfig(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("NOVA_DIR", "")
	t.Setenv("NOVA_BACKEND_PORT", "19090")
	t.Setenv("NOVA_FRONTEND_PORT", "16173")

	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("backend_port = 18080\nfrontend_port = 15173\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.BackendPort != 19090 {
		t.Fatalf("NOVA_BACKEND_PORT should override config: %d", cfg.BackendPort)
	}
	if cfg.FrontendPort != 16173 {
		t.Fatalf("NOVA_FRONTEND_PORT should override config: %d", cfg.FrontendPort)
	}
}

func TestLoadStartupDenovaPortEnvOverridesLegacy(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("NOVA_DIR", "")
	t.Setenv("DENOVA_BACKEND_PORT", "19090")
	t.Setenv("NOVA_BACKEND_PORT", "18080")
	t.Setenv("DENOVA_FRONTEND_PORT", "16173")
	t.Setenv("NOVA_FRONTEND_PORT", "15173")

	cfg := Load()
	if cfg.BackendPort != 19090 {
		t.Fatalf("DENOVA_BACKEND_PORT should override NOVA_BACKEND_PORT: %d", cfg.BackendPort)
	}
	if cfg.FrontendPort != 16173 {
		t.Fatalf("DENOVA_FRONTEND_PORT should override NOVA_FRONTEND_PORT: %d", cfg.FrontendPort)
	}
}

func TestLoadAgentIdleTimeoutEnvAllowsZero(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NOVA_DIR", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("NOVA_AGENT_IDLE_TIMEOUT_SECONDS", "0")

	cfg := Load()
	if cfg.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("NOVA_AGENT_IDLE_TIMEOUT_SECONDS=0 should disable idle timeout, got %d", cfg.AgentIdleTimeoutSeconds)
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsNovaDirToHomeNova(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NOVA_DIR", "")

	cfg := Load()
	want := normalizePath("./.nova")
	if cfg.NovaDir != want {
		t.Fatalf("默认 NovaDir 不符合预期: want=%s got=%s", want, cfg.NovaDir)
	}
}

func TestLoadDoesNotDefaultWorkspaceToCurrentDir(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NOVA_DIR", "")
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
	t.Setenv("NOVA_DIR", dir)

	cfg := Load()
	if cfg.NovaDir != dir {
		t.Fatalf("环境变量 NovaDir 不符合预期: want=%s got=%s", dir, cfg.NovaDir)
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
		Settings{OpenAIModel: "user-model", Language: "zh-CN", WritingSkillDefault: "novel-lite"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettingsFile(filepath.Join(ws, ".nova", "config.toml"),
		Settings{OpenAIModel: "ws-model", Language: "en-US", WritingSkillDefault: "novel-heavy"}); err != nil {
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
	if layered.User.OpenAIModel != "user-model" {
		t.Fatalf("user layer raw value lost")
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

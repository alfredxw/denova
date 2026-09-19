package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
	"denova/internal/hostruntime"

	"github.com/Masterminds/semver/v3"
)

// MinimumVersion is the oldest supported App Server baseline. Newer versions
// are admitted without an upper bound; RPC failures retain their own errors.
const MinimumVersion = "0.130.0"
const infrastructureTimeout = 15 * time.Second

var ErrVersionUnsupported = errors.New("unsupported App Server version")

// ProcessOptions contains host-local paths, never portable Project identities.
// Home is the user's shared Codex configuration and credential directory.
type ProcessOptions struct {
	Executable string
	Home       string
	// API is an execution-local routing snapshot; nil retains CLI configuration.
	API *config.ResolvedModelSettings
}

// Connect starts one owned App Server using the user's Codex configuration.
// The empty working directory and per-process tool policy keep Project writes
// on Denova's journalled tool path; account and network settings remain shared.
func Connect(ctx context.Context, options ProcessOptions) (*Client, error) {
	if !filepath.IsAbs(options.Executable) || !filepath.IsAbs(options.Home) {
		return nil, errors.New("App Server executable and shared home must be absolute paths")
	}
	setup, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	versionCmd := exec.CommandContext(setup, options.Executable, "--version")
	configureProcess(versionCmd)
	versionOutput, err := versionCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read App Server version: %w", err)
	}
	version, err := parseSupportedVersion(string(versionOutput))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.Home, 0o700); err != nil {
		return nil, err
	}
	cwd, err := os.MkdirTemp("", "denova-engine-")
	if err != nil {
		return nil, err
	}
	args := []string{"app-server", "-c", `web_search="disabled"`, "-c", "project_doc_max_bytes=0"}
	for _, feature := range []string{"shell_tool", "unified_exec", "apply_patch_freeform", "multi_agent", "apps", "browser_use", "browser_use_external", "computer_use", "in_app_browser", "image_generation", "plugins", "hooks", "goals", "memories", "workspace_dependencies", "shell_snapshot"} {
		args = append(args, "-c", "features."+feature+"=false")
	}
	environment := sharedEnvironment(os.Environ(), options.Home)
	if options.API != nil {
		args, environment = apiLaunch(args, environment, *options.API)
	}
	cmd := exec.Command(options.Executable, args...)
	cmd.Dir = cwd
	cmd.Env = hostruntime.WithSystemProxy(setup, environment)
	configureProcess(cmd)
	reader, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(cwd)
		return nil, err
	}
	writer, err := cmd.StdinPipe()
	if err != nil {
		_ = reader.Close()
		_ = os.RemoveAll(cwd)
		return nil, err
	}
	// Raw engine diagnostics may contain account paths or prompts. Structured
	// protocol failures are surfaced by the host; do not copy stderr to logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		_ = os.RemoveAll(cwd)
		return nil, fmt.Errorf("start App Server: %w", err)
	}
	c := newClient(reader, writer, func() { _ = writer.Close(); _ = cmd.Process.Kill() })
	c.cwd = cwd
	if options.API != nil {
		c.apiModel = options.API.Model
	}
	c.version = version
	c.workers.Add(1)
	go c.worker("wait for process", func() {
		err := cmd.Wait()
		if err != nil {
			c.fail(fmt.Errorf("App Server exited: %w", err))
		} else {
			c.fail(ErrDisconnected)
		}
		// cwd is the exact directory created by this Connect call, never a
		// caller-supplied path or a Project workspace.
		_ = os.RemoveAll(cwd)
	})
	var initialized struct {
		CodexHome string `json:"codexHome"`
	}
	if err := c.call(setup, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "denova", "version": "1"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, &initialized); err != nil {
		_ = c.Close()
		return nil, err
	}
	if filepath.Clean(initialized.CodexHome) != filepath.Clean(options.Home) {
		_ = c.Close()
		return nil, errors.New("App Server did not use the selected Codex home")
	}
	if err := c.send(setup, packet{Method: "initialized", Params: json.RawMessage(`{}`)}); err != nil {
		_ = c.Close()
		return nil, err
	}
	slog.InfoContext(ctx, "[external-runtime] connected to shared Codex configuration", "home", options.Home, "version", version)
	return c, nil
}

func parseSupportedVersion(output string) (string, error) {
	value, ok := strings.CutPrefix(strings.TrimSpace(output), "codex-cli ")
	if !ok {
		return "", fmt.Errorf("%w: unrecognized CLI version output", ErrVersionUnsupported)
	}
	version, err := semver.StrictNewVersion(strings.TrimPrefix(strings.TrimSpace(value), "v"))
	if err != nil {
		return "", fmt.Errorf("%w: invalid CLI semantic version: %v", ErrVersionUnsupported, err)
	}
	if version.LessThan(semver.MustParse(MinimumVersion)) {
		return "", fmt.Errorf("%w: minimum %s, installed %s", ErrVersionUnsupported, MinimumVersion, version)
	}
	return version.String(), nil
}

func sharedEnvironment(environment []string, home string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "CODEX_HOME") {
			continue
		}
		result = append(result, entry)
	}
	// Share the user's account/provider configuration instead of copying tokens
	// or rewriting their config. Login and logout intentionally affect this home.
	return append(result, "CODEX_HOME="+home)
}

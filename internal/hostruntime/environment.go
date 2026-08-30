// Package hostruntime resolves the host environment and executables used by
// Agent processes. It captures only an exported environment snapshot and
// never evaluates shell configuration in another shell.
package hostruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const maxSnapshotBytes = 4 * 1024 * 1024

// EnvironmentMode controls whether Agent processes inherit the current
// process environment or a login-shell environment captured at startup.
type EnvironmentMode string

const (
	EnvironmentAuto    EnvironmentMode = "auto"
	EnvironmentProcess EnvironmentMode = "process"
)

type EnvironmentOptions struct {
	Mode  EnvironmentMode
	Shell string
}

type EnvironmentSnapshot struct {
	Environment []string
	Shell       string
}

var snapshotCache = struct {
	sync.Mutex
	values map[string]EnvironmentSnapshot
}{values: make(map[string]EnvironmentSnapshot)}

// Resolve returns an immutable process environment snapshot. Windows remains
// unchanged; Unix Auto mode captures one login-shell snapshot per process and
// configuration key.
func ResolveEnvironment(ctx context.Context, options EnvironmentOptions) (EnvironmentSnapshot, error) {
	mode := normalizeMode(options.Mode)
	if runtime.GOOS == "windows" || mode == EnvironmentProcess {
		return EnvironmentSnapshot{Environment: normalizedEnvironment(os.Environ())}, nil
	}
	shell, err := resolveConfiguredLoginShell(options.Shell)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("resolve user home for shell environment: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return EnvironmentSnapshot{}, errors.New("resolve user home for shell environment: home directory is empty")
	}
	key := string(mode) + "\x00" + shell + "\x00" + home
	snapshotCache.Lock()
	cached, ok := snapshotCache.values[key]
	snapshotCache.Unlock()
	if ok {
		return cloneSnapshot(cached), nil
	}
	snapshot, captureErr := capture(ctx, shell, home, os.Environ())
	if captureErr == nil {
		snapshotCache.Lock()
		snapshotCache.values[key] = cloneSnapshot(snapshot)
		snapshotCache.Unlock()
	}
	return snapshot, captureErr
}

// resolveConfiguredLoginShell treats a persisted host path as a preference,
// not durable identity. Moving the data directory to another operating system
// therefore falls back to that host's login shell instead of disabling Agent
// shell tools until the user edits Settings.
func resolveConfiguredLoginShell(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		shell, err := resolveLoginShell(configured)
		if err == nil {
			return shell, nil
		}
		slog.Warn("[internal/hostruntime/environment.go] configured login shell is unavailable; using current-host default",
			"configured_shell", configured, "error", err)
	}
	shell, err := resolveLoginShell("")
	if err == nil {
		return shell, nil
	}
	// SHELL belongs to the current process rather than Denova data, but it can
	// still be stale in a GUI launch. Use the platform baseline as the last
	// current-host fallback.
	platformShell := "/bin/bash"
	if runtime.GOOS == "darwin" {
		platformShell = "/bin/zsh"
	}
	fallback, fallbackErr := resolveLoginShell(platformShell)
	if fallbackErr == nil {
		slog.Warn("[internal/hostruntime/environment.go] process login shell is unavailable; using platform default",
			"configured_shell", strings.TrimSpace(os.Getenv("SHELL")), "fallback_shell", fallback, "error", err)
		return fallback, nil
	}
	return "", errors.Join(err, fmt.Errorf("resolve platform login shell %q: %w", platformShell, fallbackErr))
}

func capture(ctx context.Context, shell, home string, base []string) (EnvironmentSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("create shell environment marker: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	begin := []byte("\x00DENOVA_ENV_BEGIN_" + token + "\x00")
	end := []byte("\x00DENOVA_ENV_END_" + token + "\x00")
	script := "printf '\\0DENOVA_ENV_BEGIN_" + token + "\\0'; /usr/bin/env -0; printf '\\0DENOVA_ENV_END_" + token + "\\0'"
	command := exec.CommandContext(ctx, shell, loginInteractiveArgs(shell, script)...)
	command.Dir = home
	command.Env = captureBaseEnvironment(base, home)
	command.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	command.Stdout = &limitedBuffer{buffer: &stdout, limit: maxSnapshotBytes}
	command.Stderr = &limitedBuffer{buffer: &stderr, limit: maxSnapshotBytes}
	if err := command.Run(); err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("capture exported environment with %s: %w%s", shell, err, boundedStderr(stderr.String()))
	}
	payload, err := markerPayload(stdout.Bytes(), begin, end)
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("capture exported environment with %s: %w%s", shell, err, boundedStderr(stderr.String()))
	}
	environment, err := parseNULSnapshot(payload)
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("capture exported environment with %s: %w", shell, err)
	}
	return EnvironmentSnapshot{Environment: environment, Shell: shell}, nil
}

func resolveLoginShell(configured string) (string, error) {
	candidate := strings.TrimSpace(configured)
	if candidate == "" {
		candidate = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if candidate == "" {
		if runtime.GOOS == "darwin" {
			candidate = "/bin/zsh"
		} else {
			candidate = "/bin/bash"
		}
	}
	if expanded, ok := expandHomePath(candidate); ok {
		candidate = expanded
	}
	if !filepath.IsAbs(candidate) {
		resolved, err := exec.LookPath(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve login shell %q: %w", candidate, err)
		}
		candidate = resolved
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("inspect login shell %q: %w", candidate, err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("login shell %q is not executable", candidate)
	}
	return candidate, nil
}

func expandHomePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value != "~" && !strings.HasPrefix(value, "~/") && !strings.HasPrefix(value, `~\`) {
		return value, false
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return value, false
	}
	if value == "~" {
		return home, true
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimLeft(value[1:], `/\`))), true
}

func loginInteractiveArgs(shell, script string) []string {
	switch strings.ToLower(filepath.Base(shell)) {
	case "bash":
		return []string{"-ilc", script}
	case "fish":
		return []string{"-lic", script}
	default:
		// zsh, ksh, and other Bourne-compatible login shells accept this shape.
		return []string{"-ilc", script}
	}
}

func markerPayload(output, begin, end []byte) ([]byte, error) {
	start := bytes.Index(output, begin)
	if start < 0 {
		return nil, errors.New("start marker was not produced")
	}
	start += len(begin)
	finish := bytes.Index(output[start:], end)
	if finish < 0 {
		return nil, errors.New("end marker was not produced")
	}
	return append([]byte(nil), output[start:start+finish]...), nil
}

func parseNULSnapshot(payload []byte) ([]string, error) {
	parts := bytes.Split(payload, []byte{0})
	environment := make(map[string]string, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		name, value, ok := bytes.Cut(part, []byte{'='})
		if !ok || len(name) == 0 || bytes.IndexByte(name, 0) >= 0 || bytes.IndexByte(value, 0) >= 0 {
			return nil, errors.New("login shell produced an invalid environment entry")
		}
		key := string(name)
		if unsafeInheritedShellVariable(key) {
			continue
		}
		environment[key] = string(value)
	}
	if strings.TrimSpace(environment["PATH"]) == "" {
		return nil, errors.New("login shell produced no PATH")
	}
	delete(environment, "PWD")
	delete(environment, "OLDPWD")
	delete(environment, "SHLVL")
	delete(environment, "_")
	return sortedEnvironment(environment), nil
}

func captureBaseEnvironment(base []string, home string) []string {
	environment := environmentMap(base)
	environment["HOME"] = home
	for key := range environment {
		if unsafeInheritedShellVariable(key) {
			delete(environment, key)
		}
	}
	return sortedEnvironment(environment)
}

func normalizedEnvironment(source []string) []string {
	environment := environmentMap(source)
	for key := range environment {
		if unsafeInheritedShellVariable(key) {
			delete(environment, key)
		}
	}
	return sortedEnvironment(environment)
}

func unsafeInheritedShellVariable(name string) bool {
	return name == "BASH_ENV" || name == "ENV" || name == "CDPATH" ||
		name == "BASHOPTS" || name == "SHELLOPTS" || strings.HasPrefix(name, "BASH_FUNC_")
}

func environmentMap(source []string) map[string]string {
	result := make(map[string]string, len(source))
	for _, entry := range source {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" && !strings.ContainsRune(name, 0) && !strings.ContainsRune(value, 0) {
			result[name] = value
		}
	}
	return result
}

func sortedEnvironment(environment map[string]string) []string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+environment[name])
	}
	return result
}

func normalizeMode(mode EnvironmentMode) EnvironmentMode {
	if mode == EnvironmentProcess {
		return mode
	}
	return EnvironmentAuto
}

func cloneSnapshot(snapshot EnvironmentSnapshot) EnvironmentSnapshot {
	snapshot.Environment = append([]string(nil), snapshot.Environment...)
	return snapshot
}

func boundedStderr(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 2 * 1024
	if len(value) > limit {
		value = value[:limit] + "\n[stderr truncated]"
	}
	return ": " + value
}

type limitedBuffer struct {
	buffer *bytes.Buffer
	limit  int
}

func (writer *limitedBuffer) Write(value []byte) (int, error) {
	if writer == nil || writer.buffer == nil {
		return 0, errors.New("shell environment buffer is unavailable")
	}
	if writer.buffer.Len()+len(value) > writer.limit {
		return 0, fmt.Errorf("shell environment output exceeds %d bytes", writer.limit)
	}
	return writer.buffer.Write(value)
}

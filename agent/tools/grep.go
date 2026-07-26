package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

const maxGrepCursorOffset = 10_000

type grepTarget struct {
	searchPath string
	glob       string
}

// Grep runs ripgrep once per requested target, then applies a stable global
// cursor over the de-duplicated textual result. Cursor state is self-contained
// and bound to the normalized query fingerprint.
func (workspace *LocalWorkspace) Grep(ctx context.Context, request GrepRequest) (SearchResult, error) {
	if workspace == nil || workspace.Root() == "" {
		return SearchResult{}, errors.New("workspace is not configured")
	}
	if strings.TrimSpace(request.Pattern) == "" {
		return SearchResult{}, errors.New("grep pattern is required")
	}
	request = normalizeGrepRequest(request)
	if request.Mode != "content" && request.Mode != "files" && request.Mode != "count" {
		return SearchResult{}, fmt.Errorf("unsupported grep mode %q", request.Mode)
	}
	if request.ContextBefore < 0 || request.ContextAfter < 0 {
		return SearchResult{}, errors.New("grep context cannot be negative")
	}
	if len(request.Paths) > maxSearchPaths {
		return SearchResult{}, fmt.Errorf("grep paths cannot exceed %d entries", maxSearchPaths)
	}
	limits := workspace.Limits()
	if request.Limit <= 0 {
		request.Limit = limits.DefaultDirectoryItems
	}
	if request.Limit > limits.MaxResultEntries {
		return SearchResult{}, fmt.Errorf("grep limit cannot exceed %d", limits.MaxResultEntries)
	}
	offset, err := decodeGrepCursor(request.Cursor, request)
	if err != nil {
		return SearchResult{}, err
	}
	if offset > maxGrepCursorOffset {
		return SearchResult{}, fmt.Errorf("grep cursor exceeds the %d-entry pagination window; narrow the query", maxGrepCursorOffset)
	}
	targets, warnings, err := workspace.grepTargets(request.Paths)
	if err != nil {
		return SearchResult{}, err
	}

	// Cursor input is model-controlled. Do not let a forged offset turn map
	// preallocation into an immediate memory spike; entries grow only as rg
	// actually produces validated output and the pagination window is bounded.
	seen := make(map[string]struct{}, request.Limit+1)
	entries := make([]string, 0, request.Limit)
	eligible, outputBytes := 0, 0
	truncated := false
	oversizedEntry := false
	consume := func(line string) bool {
		line = filepath.ToSlash(strings.TrimSuffix(line, "\r"))
		line = strings.TrimPrefix(line, "./")
		line = strings.TrimPrefix(line, `.\`)
		if line != "--" {
			if _, duplicate := seen[line]; duplicate {
				return true
			}
			seen[line] = struct{}{}
		}
		if eligible < offset {
			eligible++
			return true
		}
		if len(entries) >= request.Limit {
			truncated = true
			return false
		}
		additional := len(line)
		if len(entries) > 0 {
			additional++
		}
		if outputBytes+additional > limits.MaxResultBytes {
			oversizedEntry = len(entries) == 0
			truncated = true
			return false
		}
		entries = append(entries, line)
		outputBytes += additional
		eligible++
		return true
	}

	for index, target := range targets {
		stopped, runErr := workspace.runGrepTarget(ctx, request, target, limits.MaxResultBytes, consume)
		if runErr != nil {
			return SearchResult{}, runErr
		}
		if stopped {
			truncated = true
			break
		}
		if len(entries) >= request.Limit && index < len(targets)-1 {
			truncated = true
			break
		}
	}
	if oversizedEntry {
		return SearchResult{}, fmt.Errorf("grep entry exceeds the %d-byte result limit; narrow the query", limits.MaxResultBytes)
	}
	result := SearchResult{Entries: entries, Truncated: truncated, Warnings: warnings}
	if truncated {
		next, err := encodeGrepCursor(offset+len(entries), request)
		if err != nil {
			return SearchResult{}, fmt.Errorf("encode grep cursor: %w", err)
		}
		result.NextCursor = next
	}
	return result, nil
}

func (workspace *LocalWorkspace) runGrepTarget(
	ctx context.Context,
	request GrepRequest,
	target grepTarget,
	maxBytes int,
	consume func(string) bool,
) (bool, error) {
	args := grepArguments(request, target)
	command := exec.CommandContext(ctx, workspace.ripgrepExecutable, args...)
	command.Dir = workspace.root
	stdout, err := command.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("create grep stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return false, fmt.Errorf("create grep stderr: %w", err)
	}
	if err := workspace.verifyRootIdentity(); err != nil {
		return false, err
	}
	if err := command.Start(); err != nil {
		return false, fmt.Errorf("start ripgrep: %w", err)
	}
	diagnostics := readProcessDiagnostics(stderr, maxBytes)
	stopped := false
	var scanErr error
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), maxBytes+1)
	for scanner.Scan() {
		if err := contextError(ctx); err != nil {
			scanErr = err
			_ = command.Process.Kill()
			break
		}
		if !utf8.ValidString(scanner.Text()) {
			scanErr = errors.New("grep encountered non-UTF-8 output")
			_ = command.Process.Kill()
			break
		}
		if !consume(scanner.Text()) {
			stopped = true
			_ = command.Process.Kill()
			break
		}
	}
	if scanErr == nil && scanner.Err() != nil {
		scanErr = fmt.Errorf("read ripgrep output: %w", scanner.Err())
		_ = command.Process.Kill()
	}
	diagnostic := <-diagnostics
	waitErr := command.Wait()
	if scanErr != nil {
		return stopped, scanErr
	}
	if diagnostic.err != nil {
		return stopped, diagnostic.err
	}
	if waitErr != nil && !stopped {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 1 {
			return false, fmt.Errorf("ripgrep failed: %s", boundedString(diagnostic.content, maxBytes))
		}
	}
	return stopped, nil
}

func grepArguments(request GrepRequest, target grepTarget) []string {
	args := []string{
		"--no-config", "--color=never", "--no-messages", "--no-require-git",
		"--no-ignore-global", "--no-ignore-parent", "--sort=path", "--hidden", "--glob", "!.git/**",
	}
	switch request.Mode {
	case "files":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count-matches", "--with-filename")
	default:
		args = append(args, "--with-filename", "--line-number")
		if request.ContextBefore > 0 {
			args = append(args, "--before-context", fmt.Sprintf("%d", request.ContextBefore))
		}
		if request.ContextAfter > 0 {
			args = append(args, "--after-context", fmt.Sprintf("%d", request.ContextAfter))
		}
	}
	if !request.CaseSensitive {
		args = append(args, "--ignore-case")
	}
	if !request.Gitignore {
		args = append(args, "--no-ignore")
	}
	if strings.Contains(request.Pattern, "\n") {
		args = append(args, "--multiline", "--multiline-dotall")
	}
	if target.glob != "" {
		args = append(args, "--glob", target.glob)
	}
	return append(args, "-e", request.Pattern, "--", filepath.FromSlash(target.searchPath))
}

func (workspace *LocalWorkspace) grepTargets(inputs []string) ([]grepTarget, []string, error) {
	targets := make([]grepTarget, 0, len(inputs))
	warnings := make([]string, 0)
	for _, input := range inputs {
		input = filepath.ToSlash(strings.TrimSpace(input))
		if input == "" {
			return nil, nil, errors.New("grep paths must not contain an empty entry")
		}
		if hasResourceScheme(input) {
			return nil, nil, fmt.Errorf("grep only supports workspace paths: %s", input)
		}
		if hasGlobMeta(input) {
			if filepath.IsAbs(input) || strings.HasPrefix(input, "/") || strings.HasPrefix(input, "!") ||
				hasParentComponent(input) || !doublestar.ValidatePathPattern(input) {
				return nil, nil, fmt.Errorf("grep path pattern must be workspace-relative and valid: %s", input)
			}
			targets = append(targets, grepTarget{searchPath: ".", glob: path.Clean(input)})
			continue
		}
		relative, _, err := workspace.stat(input, true)
		if err != nil {
			if len(inputs) == 1 {
				return nil, nil, err
			}
			warnings = append(warnings, fmt.Sprintf("Skipped missing path %q. / 已跳过不存在的路径 %q。", input, input))
			continue
		}
		targets = append(targets, grepTarget{searchPath: relative})
	}
	if len(targets) == 0 {
		return nil, nil, errors.New("none of the requested grep paths exists")
	}
	return targets, warnings, nil
}

func normalizeRequestedPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	result := make([]string, len(paths))
	for index, value := range paths {
		result[index] = filepath.ToSlash(strings.TrimSpace(value))
	}
	return result
}

func normalizeGrepRequest(request GrepRequest) GrepRequest {
	request.Mode = strings.TrimSpace(request.Mode)
	if request.Mode == "" {
		request.Mode = "content"
	}
	request.Paths = normalizeRequestedPaths(request.Paths)
	return request
}

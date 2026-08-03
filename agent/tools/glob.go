package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

type globTargetKind uint8

const (
	globTargetPattern globTargetKind = iota
	globTargetFile
	globTargetDirectory
)

type globTarget struct {
	kind  globTargetKind
	value string
}

// Glob discovers de-duplicated files and directories without following
// directory symlinks. Ripgrep supplies mature hidden/.gitignore semantics;
// doublestar is used only for matching the already-safe relative candidates.
func (workspace *LocalWorkspace) Glob(ctx context.Context, request GlobRequest) (SearchResult, error) {
	if workspace == nil || workspace.Root() == "" {
		return SearchResult{}, errors.New("workspace is not configured")
	}
	limits := workspace.Limits()
	request = normalizeGlobRequest(request)
	limit := request.Limit
	if limit <= 0 {
		limit = limits.DefaultDirectoryItems
	}
	after, err := decodeGlobCursor(request.Cursor, request)
	if err != nil {
		return SearchResult{}, err
	}
	targets, warnings, err := workspace.globTargets(request.Paths)
	if err != nil {
		return SearchResult{}, err
	}
	args := []string{
		"--files", "--null", "--no-config", "--no-messages", "--no-require-git",
		"--no-ignore-global", "--no-ignore-parent", "--sort=path", "--glob", "!.git/**",
	}
	if request.Hidden {
		args = append(args, "--hidden")
	}
	if !request.Gitignore {
		args = append(args, "--no-ignore")
	}
	command := exec.CommandContext(ctx, workspace.ripgrepExecutable, args...)
	command.Dir = workspace.root
	stdout, err := command.StdoutPipe()
	if err != nil {
		return SearchResult{}, fmt.Errorf("create glob stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return SearchResult{}, fmt.Errorf("create glob stderr: %w", err)
	}
	if err := workspace.verifyRootIdentity(); err != nil {
		return SearchResult{}, err
	}
	if err := command.Start(); err != nil {
		return SearchResult{}, fmt.Errorf("start workspace glob: %w", err)
	}
	diagnostics := readProcessDiagnostics(stderr, limits.MaxResultBytes)

	capacity := min(limit, limits.MaxResultBytes)
	if capacity < limits.MaxResultBytes {
		capacity++
	}
	capacity = min(capacity, maxWorkspaceScanEntries)
	matches := make([]string, 0, min(capacity, limits.DefaultDirectoryItems))
	seenDirectories := make(map[string]struct{})
	eligible := 0
	add := func(candidate string) bool {
		if strings.HasSuffix(candidate, "/") {
			if _, exists := seenDirectories[candidate]; exists {
				return true
			}
			seenDirectories[candidate] = struct{}{}
		}
		if candidate <= after {
			return true
		}
		eligible++
		index := sort.SearchStrings(matches, candidate)
		if index < len(matches) && matches[index] == candidate {
			eligible--
			return true
		}
		if index >= capacity {
			return true
		}
		matches = append(matches, "")
		copy(matches[index+1:], matches[index:])
		matches[index] = candidate
		if len(matches) > capacity {
			matches = matches[:capacity]
		}
		return true
	}

	reader := bufio.NewReaderSize(stdout, 64*1024)
	stoppedEarly := false
	var loopErr error
	for {
		if err := contextError(ctx); err != nil {
			_ = command.Process.Kill()
			loopErr = err
			break
		}
		record, readErr := reader.ReadString(0)
		if len(record) > 0 {
			record = strings.TrimSuffix(record, "\x00")
			candidate := filepath.ToSlash(record)
			if !utf8.ValidString(candidate) {
				_ = command.Process.Kill()
				loopErr = errors.New("glob encountered a non-UTF-8 workspace path")
				break
			}
			for _, entry := range globCandidatePaths(candidate) {
				if matchesGlobTargets(entry, targets) {
					add(entry)
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = command.Process.Kill()
				loopErr = fmt.Errorf("read glob output: %w", readErr)
			}
			break
		}
	}
	diagnostic := <-diagnostics
	waitErr := command.Wait()
	if loopErr != nil {
		return SearchResult{}, loopErr
	}
	if diagnostic.err != nil {
		return SearchResult{}, diagnostic.err
	}
	if waitErr != nil && !stoppedEarly {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 1 {
			return SearchResult{}, fmt.Errorf("workspace glob failed: %s", boundedString(diagnostic.content, limits.MaxResultBytes))
		}
	}
	stoppedEarly, err = workspace.addGlobDirectories(ctx, request, targets, add)
	if err != nil {
		return SearchResult{}, err
	}
	if stoppedEarly {
		return SearchResult{}, errors.New("glob directory scan stopped before producing a stable page / 目录扫描未完成，无法生成稳定分页")
	}
	visible := min(limit, len(matches))
	outputBytes := 0
	for index := 0; index < visible; index++ {
		additional := len(matches[index])
		if index > 0 {
			additional++
		}
		if outputBytes+additional > limits.MaxResultBytes {
			visible = index
			break
		}
		outputBytes += additional
	}
	if visible == 0 && len(matches) > 0 {
		return SearchResult{}, fmt.Errorf("glob entry exceeds the %d-byte shared result budget", limits.MaxResultBytes)
	}
	result := SearchResult{Entries: append([]string(nil), matches[:visible]...), Truncated: eligible > visible, Warnings: warnings}
	if result.Truncated && visible > 0 {
		result.NextCursor, err = encodeGlobCursor(result.Entries[visible-1], request)
		if err != nil {
			return SearchResult{}, fmt.Errorf("encode glob cursor: %w", err)
		}
	}
	return result, nil
}

func normalizeGlobRequest(request GlobRequest) GlobRequest {
	request.Paths = normalizeRequestedPaths(request.Paths)
	return request
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

func (workspace *LocalWorkspace) globTargets(inputs []string) ([]globTarget, []string, error) {
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	targets := make([]globTarget, 0, len(inputs))
	warnings := make([]string, 0)
	for _, input := range inputs {
		input = filepath.ToSlash(strings.TrimSpace(input))
		if input == "" {
			return nil, nil, errors.New("glob paths must not contain an empty entry")
		}
		if hasResourceScheme(input) {
			return nil, nil, fmt.Errorf("glob only supports workspace paths: %s", input)
		}
		if hasGlobMeta(input) {
			if filepath.IsAbs(input) || strings.HasPrefix(input, "/") || hasParentComponent(input) || !doublestar.ValidatePathPattern(input) {
				return nil, nil, fmt.Errorf("glob pattern must be workspace-relative and valid: %s", input)
			}
			targets = append(targets, globTarget{kind: globTargetPattern, value: path.Clean(input)})
			continue
		}
		relative, info, err := workspace.stat(input, true)
		if err != nil {
			if len(inputs) == 1 {
				return nil, nil, err
			}
			warnings = append(warnings, fmt.Sprintf("Skipped missing path %q. / 已跳过不存在的路径 %q。", input, input))
			continue
		}
		kind := globTargetFile
		if info.IsDir() {
			kind = globTargetDirectory
		}
		targets = append(targets, globTarget{kind: kind, value: relative})
	}
	if len(targets) == 0 {
		return nil, nil, errors.New("none of the requested glob paths exists")
	}
	return targets, warnings, nil
}

func matchesGlobTargets(candidate string, targets []globTarget) bool {
	plain := strings.TrimSuffix(candidate, "/")
	for _, target := range targets {
		switch target.kind {
		case globTargetFile:
			if plain == target.value {
				return true
			}
		case globTargetDirectory:
			if target.value == "." || strings.HasPrefix(plain, target.value+"/") {
				return true
			}
		case globTargetPattern:
			matched, _ := doublestar.Match(target.value, plain)
			if matched {
				return true
			}
		}
	}
	return false
}

func globCandidatePaths(file string) []string {
	result := []string{file}
	directory := path.Dir(file)
	for directory != "." && directory != "/" {
		result = append(result, directory+"/")
		directory = path.Dir(directory)
	}
	return result
}

func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[") || strings.Contains(value, "{")
}

func hasParentComponent(value string) bool {
	for _, component := range strings.Split(filepath.ToSlash(value), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

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
	limit := request.Limit
	if limit <= 0 {
		limit = limits.DefaultDirectoryItems
	}
	if limit > limits.MaxResultEntries {
		return SearchResult{}, fmt.Errorf("glob limit cannot exceed %d", limits.MaxResultEntries)
	}
	if len(request.Paths) > maxSearchPaths {
		return SearchResult{}, fmt.Errorf("glob paths cannot exceed %d entries", maxSearchPaths)
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

	seen := make(map[string]struct{}, limit+1)
	matches := make([]string, 0, min(limit+1, limits.DefaultDirectoryItems))
	outputBytes := 0
	truncated := false
	oversizedEntry := false
	add := func(candidate string) bool {
		if _, exists := seen[candidate]; exists {
			return true
		}
		if len(matches) >= limit {
			truncated = true
			return false
		}
		additional := len(candidate)
		if len(matches) > 0 {
			additional++
		}
		if outputBytes+additional > limits.MaxResultBytes {
			oversizedEntry = len(matches) == 0
			truncated = true
			return false
		}
		seen[candidate] = struct{}{}
		matches = append(matches, candidate)
		outputBytes += additional
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
				if matchesGlobTargets(entry, targets) && !add(entry) {
					stoppedEarly = true
					_ = command.Process.Kill()
					break
				}
			}
			if stoppedEarly {
				break
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
	if !stoppedEarly {
		stoppedEarly, err = workspace.addGlobDirectories(ctx, request, targets, add)
		if err != nil {
			return SearchResult{}, err
		}
		if stoppedEarly {
			truncated = true
		}
	}
	if oversizedEntry {
		return SearchResult{}, fmt.Errorf("glob entry exceeds the %d-byte result limit; narrow the path", limits.MaxResultBytes)
	}
	sort.Strings(matches)
	return SearchResult{Entries: matches, Truncated: truncated, Warnings: warnings}, nil
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

package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alfredxw/denova/adk"
	"github.com/bmatcuk/doublestar/v4"
)

const (
	workspaceSearchMaxEntries      = 10_000
	workspaceSearchMaxResultBytes  = 1024 * 1024
	workspaceResultTruncatedMarker = "[workspace result truncated at the 1 MiB safety limit; narrow path or pattern]"
)

type workspaceListInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Workspace-relative or absolute directory path; defaults to the workspace root."`
}

type workspaceGlobInput struct {
	Pattern string `json:"pattern" jsonschema_description:"Workspace-relative glob pattern such as **/*.go or chapters/*.md."`
	Path    string `json:"path,omitempty" jsonschema_description:"Workspace-relative or absolute directory to search; defaults to the workspace root."`
}

type workspaceGrepInput struct {
	Pattern         string `json:"pattern" jsonschema_description:"Ripgrep regular expression to search for."`
	Path            string `json:"path,omitempty" jsonschema_description:"Workspace-relative or absolute file or directory to search; defaults to the workspace root."`
	Glob            string `json:"glob,omitempty" jsonschema_description:"Optional glob filter such as *.go or **/*.md."`
	OutputMode      string `json:"output_mode,omitempty" jsonschema:"enum=content,enum=files_with_matches,enum=count" jsonschema_description:"content, files_with_matches, or count; defaults to files_with_matches."`
	Context         int    `json:"context,omitempty" jsonschema_description:"Context lines before and after each content match."`
	BeforeLines     int    `json:"before_lines,omitempty" jsonschema_description:"Lines before each content match; ignored when context is set."`
	AfterLines      int    `json:"after_lines,omitempty" jsonschema_description:"Lines after each content match; ignored when context is set."`
	ShowLineNumbers *bool  `json:"show_line_numbers,omitempty" jsonschema_description:"Show line numbers in content mode; defaults to true."`
	CaseInsensitive bool   `json:"case_insensitive,omitempty" jsonschema_description:"Use case-insensitive matching."`
	FileType        string `json:"type,omitempty" jsonschema_description:"Optional ripgrep file type such as go, py, js, or rust."`
	HeadLimit       int    `json:"head_limit,omitempty" jsonschema_description:"Maximum returned output lines or entries; zero uses the byte safety limit."`
	Offset          int    `json:"offset,omitempty" jsonschema_description:"Number of output lines or entries to skip before head_limit."`
	Multiline       bool   `json:"multiline,omitempty" jsonschema_description:"Enable matches that span lines."`
}

func newWorkspaceListTool(backend *agentFilesystemBackend) (adk.BaseTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	return adk.InferTool("ls", workspaceListToolDescription, func(ctx context.Context, input workspaceListInput) (string, error) {
		return listWorkspaceDirectory(ctx, backend, input.Path)
	})
}

func newWorkspaceGlobTool(backend *agentFilesystemBackend) (adk.BaseTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	return adk.InferTool("glob", workspaceGlobToolDescription, func(ctx context.Context, input workspaceGlobInput) (string, error) {
		return globWorkspace(ctx, backend, input)
	})
}

func newWorkspaceGrepTool(backend *agentFilesystemBackend) (adk.BaseTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	return adk.InferTool("grep", workspaceGrepToolDescription, func(ctx context.Context, input workspaceGrepInput) (string, error) {
		return grepWorkspace(ctx, backend, input)
	})
}

func listWorkspaceDirectory(ctx context.Context, backend *agentFilesystemBackend, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		input = "."
	}
	_, relative, info, err := backend.validateExistingPath(input, true)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("ls path is not a directory: %s", input)
	}
	root, err := backend.openRoot()
	if err != nil {
		return "", err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), relative)
	if err != nil {
		return "", fmt.Errorf("list workspace directory %s: %w", input, err)
	}
	collector := newWorkspaceResultCollector()
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name := entry.Name()
		if relative != "." {
			name = path.Join(relative, name)
		}
		if entry.IsDir() {
			name += "/"
		}
		if !collector.Add(name) {
			break
		}
	}
	return collector.Result("No files found in the selected directory."), nil
}

func globWorkspace(ctx context.Context, backend *agentFilesystemBackend, input workspaceGlobInput) (string, error) {
	pattern := filepath.ToSlash(strings.TrimSpace(input.Pattern))
	if pattern == "" {
		return "", fmt.Errorf("glob pattern is required")
	}
	if strings.HasPrefix(pattern, "/") || hasParentPathComponent(pattern) || !doublestar.ValidatePathPattern(pattern) {
		return "", fmt.Errorf("glob pattern must be a valid workspace-relative pattern: %s", input.Pattern)
	}
	base := strings.TrimSpace(input.Path)
	if base == "" {
		base = "."
	}
	_, relative, info, err := backend.validateExistingPath(base, true)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("glob path is not a directory: %s", base)
	}
	fullPattern := pattern
	if relative != "." {
		fullPattern = path.Join(relative, pattern)
	}
	root, err := backend.openRoot()
	if err != nil {
		return "", err
	}
	defer root.Close()

	matches := make([]string, 0, 128)
	truncated := false
	err = doublestar.GlobWalk(root.FS(), fullPattern, func(match string, entry fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(matches) >= workspaceSearchMaxEntries {
			truncated = true
			return fs.SkipAll
		}
		match = filepath.ToSlash(match)
		if entry.IsDir() {
			match += "/"
		}
		matches = append(matches, match)
		return nil
	}, doublestar.WithFailOnIOErrors(), doublestar.WithNoFollow())
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return "", fmt.Errorf("glob workspace: %w", err)
	}
	sort.Strings(matches)
	collector := newWorkspaceResultCollector()
	for _, match := range matches {
		if !collector.Add(match) {
			truncated = true
			break
		}
	}
	collector.truncated = collector.truncated || truncated
	return collector.Result("No files matched the glob pattern."), nil
}

func grepWorkspace(ctx context.Context, backend *agentFilesystemBackend, input workspaceGrepInput) (string, error) {
	if strings.TrimSpace(input.Pattern) == "" {
		return "", fmt.Errorf("grep pattern is required")
	}
	if input.Context < 0 || input.BeforeLines < 0 || input.AfterLines < 0 || input.HeadLimit < 0 || input.Offset < 0 {
		return "", fmt.Errorf("grep context, pagination, and limits cannot be negative")
	}
	mode := strings.TrimSpace(input.OutputMode)
	if mode == "" {
		mode = "files_with_matches"
	}
	switch mode {
	case "content", "files_with_matches", "count":
	default:
		return "", fmt.Errorf("unsupported grep output_mode %q", input.OutputMode)
	}
	searchPath := strings.TrimSpace(input.Path)
	if searchPath == "" {
		searchPath = "."
	}
	_, relative, _, err := backend.validateExistingPath(searchPath, true)
	if err != nil {
		return "", err
	}

	args := []string{"--color=never", "--no-messages"}
	switch mode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count-matches", "--with-filename")
	case "content":
		args = append(args, "--with-filename")
		showLines := input.ShowLineNumbers == nil || *input.ShowLineNumbers
		if showLines {
			args = append(args, "--line-number")
		}
		before, after := input.BeforeLines, input.AfterLines
		if input.Context > 0 {
			before, after = input.Context, input.Context
		}
		if before > 0 {
			args = append(args, "-B", fmt.Sprintf("%d", before))
		}
		if after > 0 {
			args = append(args, "-A", fmt.Sprintf("%d", after))
		}
	}
	if input.CaseInsensitive {
		args = append(args, "--ignore-case")
	}
	if input.Multiline {
		args = append(args, "--multiline", "--multiline-dotall")
	}
	if glob := strings.TrimSpace(input.Glob); glob != "" {
		args = append(args, "--glob", glob)
	}
	if fileType := strings.TrimSpace(input.FileType); fileType != "" {
		args = append(args, "--type", fileType)
	}
	args = append(args, "-e", input.Pattern, "--", filepath.FromSlash(relative))

	command := exec.CommandContext(ctx, "rg", args...)
	command.Dir = backend.Workspace()
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("create grep stdout pipe: %w", err)
	}
	var stderr boundedBytesBuffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("grep requires ripgrep (rg) in PATH")
		}
		return "", fmt.Errorf("start ripgrep: %w", err)
	}

	collector := newWorkspaceResultCollector()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), workspaceSearchMaxResultBytes+1)
	seen := 0
	returned := 0
	stoppedEarly := false
	for scanner.Scan() {
		if seen < input.Offset {
			seen++
			continue
		}
		if input.HeadLimit > 0 && returned >= input.HeadLimit {
			stoppedEarly = true
			break
		}
		seen++
		returned++
		line := strings.TrimPrefix(scanner.Text(), "./")
		line = strings.TrimPrefix(line, `.\`)
		if !collector.Add(line) {
			stoppedEarly = true
			break
		}
	}
	if stoppedEarly {
		collector.truncated = true
		_ = command.Process.Kill()
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if scanErr != nil {
		return "", fmt.Errorf("read ripgrep output: %w", scanErr)
	}
	if waitErr != nil && !stoppedEarly {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) {
			return "", fmt.Errorf("ripgrep failed: %w", waitErr)
		}
		if exitError.ExitCode() != 1 {
			diagnostic := strings.TrimSpace(stderr.String())
			if stderr.truncated {
				diagnostic += "\n[ripgrep stderr truncated]"
			}
			return "", fmt.Errorf("ripgrep failed with exit code %d: %s", exitError.ExitCode(), diagnostic)
		}
	}
	return collector.Result("No matches found."), nil
}

func hasParentPathComponent(pattern string) bool {
	for _, component := range strings.Split(pattern, "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

type workspaceResultCollector struct {
	builder   strings.Builder
	entries   int
	truncated bool
}

func newWorkspaceResultCollector() *workspaceResultCollector {
	return &workspaceResultCollector{}
}

func (collector *workspaceResultCollector) Add(line string) bool {
	if collector == nil || collector.truncated {
		return false
	}
	separatorBytes := 0
	if collector.entries > 0 {
		separatorBytes = 1
	}
	payloadLimit := workspaceSearchMaxResultBytes - len(workspaceResultTruncatedMarker) - 1
	if collector.entries >= workspaceSearchMaxEntries || collector.builder.Len()+separatorBytes+len(line) > payloadLimit {
		collector.truncated = true
		return false
	}
	if separatorBytes != 0 {
		collector.builder.WriteByte('\n')
	}
	collector.builder.WriteString(line)
	collector.entries++
	return true
}

func (collector *workspaceResultCollector) Result(empty string) string {
	if collector == nil || collector.entries == 0 {
		if collector != nil && collector.truncated {
			return workspaceResultTruncatedMarker
		}
		return empty
	}
	result := collector.builder.String()
	if collector.truncated {
		result += "\n" + workspaceResultTruncatedMarker
	}
	return result
}

type boundedBytesBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (buffer *boundedBytesBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := workspaceSearchMaxResultBytes - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return written, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		buffer.truncated = true
	}
	_, _ = buffer.buffer.Write(data)
	return written, nil
}

func (buffer *boundedBytesBuffer) String() string {
	if buffer == nil {
		return ""
	}
	return buffer.buffer.String()
}

var workspaceListToolDescription = strings.TrimSpace(`List one directory inside the active workspace.
- path may be workspace-relative or absolute and defaults to the workspace root.
- Results are workspace-relative, sorted entries; directories end with /.

列出当前 workspace 内的一个目录。
- path 可使用 workspace 相对路径或绝对路径，默认是 workspace 根目录。
- 结果按名称排序并使用 workspace 相对路径；目录以 / 结尾。`)

var workspaceGlobToolDescription = strings.TrimSpace(`Find workspace files and directories by a bounded glob search.
- pattern is workspace-relative and supports **, *, ?, character classes, and brace alternatives.
- path optionally selects a directory inside the workspace.

使用有界 glob 搜索查找 workspace 内的文件和目录。
- pattern 相对于 workspace，支持 **、*、?、字符类和花括号候选。
- path 可选，用于限定 workspace 内的搜索目录。`)

var workspaceGrepToolDescription = strings.TrimSpace(`Search text inside the active workspace with ripgrep.
- Supports regex, glob/type filters, content/files/count modes, context lines, and pagination.
- Results are capped at 1 MiB; narrow path, pattern, or head_limit when truncated.
- The search path is validated against the workspace before ripgrep starts.

使用 ripgrep 搜索当前 workspace 内的文本。
- 支持正则、glob/type 过滤、正文/文件/计数模式、上下文行和分页。
- 结果上限为 1 MiB；发生截断时请缩小 path、pattern 或 head_limit。
- 启动 ripgrep 前会先校验搜索路径位于 workspace 内。`)

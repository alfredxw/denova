package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	defaultReadLines        = 2000
	defaultResultBytes      = 1024 * 1024
	defaultResultEntries    = 10_000
	resultTruncatedMarker   = "[workspace result truncated at the 1 MiB safety limit; narrow path or pattern]"
	readWindowExceededError = "selected read_file window exceeds %d bytes; use a narrower offset/limit or split the long line"
)

// ReadRequest selects one bounded window of a UTF-8 file.
type ReadRequest struct {
	Path   string
	Offset int
	Limit  int
}

// ReadResult is a workspace-relative path plus the selected source text.
type ReadResult struct {
	Path    string
	Offset  int
	Limit   int
	Content string
}

// TextEdit is an exact replacement evaluated against the original snapshot.
type TextEdit struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

// GrepRequest describes a bounded ripgrep-compatible search.
type GrepRequest struct {
	Pattern         string
	Path            string
	Glob            string
	OutputMode      string
	Context         int
	BeforeLines     int
	AfterLines      int
	ShowLineNumbers *bool
	CaseInsensitive bool
	FileType        string
	HeadLimit       int
	Offset          int
	Multiline       bool
}

// Reader is the narrow filesystem port used by read_file.
type Reader interface {
	Read(context.Context, ReadRequest) (ReadResult, error)
}

// Searcher is the narrow filesystem port shared by ls, glob, and grep.
type Searcher interface {
	List(context.Context, string) ([]string, error)
	Glob(context.Context, string, string) ([]string, error)
	Grep(context.Context, GrepRequest) ([]string, error)
}

// Writer is the narrow filesystem port shared by write_file and edit_file.
// Implementations may write directly, stage changes for review, or reject all
// mutations while keeping a stable model schema.
type Writer interface {
	Write(context.Context, string, string) (WriteReceipt, error)
	Edit(context.Context, string, []TextEdit) (WriteReceipt, error)
}

// WriteReceipt is provider-neutral mutation evidence.
type WriteReceipt struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// LocalWorkspace is a reusable, symlink-safe filesystem adapter rooted at one
// directory. All model-provided paths cross os.Root before I/O.
type LocalWorkspace struct {
	root string
}

// OpenWorkspace validates and canonicalizes a local workspace root.
func OpenWorkspace(root string) (*LocalWorkspace, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("workspace path is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize workspace path: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory: %s", canonical)
	}
	return &LocalWorkspace{root: filepath.Clean(canonical)}, nil
}

// Root returns the canonical command working directory.
func (workspace *LocalWorkspace) Root() string {
	if workspace == nil {
		return ""
	}
	return workspace.root
}

func (workspace *LocalWorkspace) openRoot() (*os.Root, error) {
	if workspace == nil || workspace.root == "" {
		return nil, errors.New("workspace is not configured")
	}
	root, err := os.OpenRoot(workspace.root)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	return root, nil
}

func (workspace *LocalWorkspace) relative(input string, allowRoot bool) (string, error) {
	if workspace == nil || workspace.root == "" {
		return "", errors.New("workspace is not configured")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("workspace path is required")
	}
	var relative string
	var err error
	if filepath.IsAbs(input) {
		relative, err = filepath.Rel(workspace.root, filepath.Clean(input))
		if err != nil {
			return "", fmt.Errorf("resolve workspace-relative path: %w", err)
		}
	} else {
		relative = filepath.Clean(filepath.FromSlash(input))
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path is outside the active workspace: %s", input)
	}
	if relative == "." && !allowRoot {
		return "", errors.New("path must identify an item inside the active workspace")
	}
	return filepath.ToSlash(relative), nil
}

// Read returns a bounded UTF-8 source window.
func (workspace *LocalWorkspace) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	relative, err := workspace.relative(request.Path, false)
	if err != nil {
		return ReadResult{}, err
	}
	offset, limit := normalizeReadWindow(request.Offset, request.Limit)
	root, err := workspace.openRoot()
	if err != nil {
		return ReadResult{}, err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(relative))
	if err != nil {
		return ReadResult{}, fmt.Errorf("open workspace file %s: %w", relative, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ReadResult{}, fmt.Errorf("inspect workspace file %s: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return ReadResult{}, fmt.Errorf("read_file only supports regular text files: %s", relative)
	}
	content, err := selectReadWindow(ctx, file, offset, limit)
	if err != nil {
		return ReadResult{}, err
	}
	if !utf8.ValidString(content) {
		return ReadResult{}, fmt.Errorf("read_file only supports UTF-8 text files: %s", relative)
	}
	return ReadResult{Path: relative, Offset: offset, Limit: limit, Content: content}, nil
}

// List returns one directory level in stable lexical order.
func (workspace *LocalWorkspace) List(ctx context.Context, input string) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		input = "."
	}
	relative, err := workspace.relative(input, true)
	if err != nil {
		return nil, err
	}
	root, err := workspace.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), filepath.FromSlash(relative))
	if err != nil {
		return nil, fmt.Errorf("list workspace directory %s: %w", input, err)
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if relative != "." {
			name = path.Join(relative, name)
		}
		if entry.IsDir() {
			name += "/"
		}
		result = append(result, name)
		if len(result) >= defaultResultEntries {
			result = append(result, resultTruncatedMarker)
			break
		}
	}
	return result, nil
}

// Glob returns workspace-relative matches without following directory symlinks.
func (workspace *LocalWorkspace) Glob(ctx context.Context, base, pattern string) ([]string, error) {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return nil, errors.New("glob pattern is required")
	}
	if strings.HasPrefix(pattern, "/") || hasParentComponent(pattern) || !doublestar.ValidatePathPattern(pattern) {
		return nil, fmt.Errorf("glob pattern must be a valid workspace-relative pattern: %s", pattern)
	}
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	relative, err := workspace.relative(base, true)
	if err != nil {
		return nil, err
	}
	fullPattern := pattern
	if relative != "." {
		fullPattern = path.Join(relative, pattern)
	}
	root, err := workspace.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result := make([]string, 0, 128)
	err = doublestar.GlobWalk(root.FS(), fullPattern, func(match string, entry fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(result) >= defaultResultEntries {
			return fs.SkipAll
		}
		match = filepath.ToSlash(match)
		if entry.IsDir() {
			match += "/"
		}
		result = append(result, match)
		return nil
	}, doublestar.WithFailOnIOErrors(), doublestar.WithNoFollow())
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return nil, fmt.Errorf("glob workspace: %w", err)
	}
	sort.Strings(result)
	if len(result) >= defaultResultEntries {
		result = append(result, resultTruncatedMarker)
	}
	return result, nil
}

// Grep runs ripgrep with structured arguments and bounded output.
func (workspace *LocalWorkspace) Grep(ctx context.Context, request GrepRequest) ([]string, error) {
	if strings.TrimSpace(request.Pattern) == "" {
		return nil, errors.New("grep pattern is required")
	}
	if request.Context < 0 || request.BeforeLines < 0 || request.AfterLines < 0 || request.HeadLimit < 0 || request.Offset < 0 {
		return nil, errors.New("grep context, pagination, and limits cannot be negative")
	}
	mode := strings.TrimSpace(request.OutputMode)
	if mode == "" {
		mode = "files_with_matches"
	}
	if mode != "content" && mode != "files_with_matches" && mode != "count" {
		return nil, fmt.Errorf("unsupported grep output_mode %q", request.OutputMode)
	}
	searchPath := request.Path
	if strings.TrimSpace(searchPath) == "" {
		searchPath = "."
	}
	relative, err := workspace.relative(searchPath, true)
	if err != nil {
		return nil, err
	}
	args := grepArguments(request, mode, relative)
	command := exec.CommandContext(ctx, "rg", args...)
	command.Dir = workspace.root
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create ripgrep stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create ripgrep stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start ripgrep: %w", err)
	}
	type diagnosticResult struct {
		content []byte
		err     error
	}
	diagnostic := make(chan diagnosticResult, 1)
	go func() {
		result := diagnosticResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				result.err = fmt.Errorf("read ripgrep stderr panic: %v", recovered)
			}
			diagnostic <- result
		}()
		result.content, result.err = readBoundedAndDrain(stderr, defaultResultBytes)
	}()
	output, readErr := io.ReadAll(io.LimitReader(stdout, defaultResultBytes+1))
	truncated := len(output) > defaultResultBytes
	if truncated && command.Process != nil {
		_ = command.Process.Kill()
		output = output[:defaultResultBytes]
	}
	stderrResult := <-diagnostic
	// Drain both pipes before Wait. os/exec closes StdoutPipe/StderrPipe from
	// Wait, so waiting first races the diagnostic reader for short-lived rg
	// processes and can surface a spurious "file already closed" error.
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if readErr != nil {
		return nil, fmt.Errorf("read ripgrep output: %w", readErr)
	}
	if stderrResult.err != nil {
		return nil, stderrResult.err
	}
	if waitErr != nil && !truncated {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 1 {
			return nil, fmt.Errorf("ripgrep failed: %s", boundedString(string(stderrResult.content), defaultResultBytes))
		}
	}
	lines := splitBoundedLines(string(output), request.Offset, request.HeadLimit)
	for index := range lines {
		lines[index] = strings.TrimPrefix(lines[index], "./")
		lines[index] = strings.TrimPrefix(lines[index], `.\`)
	}
	if truncated {
		lines = append(lines, resultTruncatedMarker)
	}
	return lines, nil
}

// Write replaces one file, creating its parent directories inside the root.
func (workspace *LocalWorkspace) Write(ctx context.Context, input, content string) (WriteReceipt, error) {
	if err := ctx.Err(); err != nil {
		return WriteReceipt{}, err
	}
	relative, err := workspace.relative(input, false)
	if err != nil {
		return WriteReceipt{}, err
	}
	root, err := workspace.openRoot()
	if err != nil {
		return WriteReceipt{}, err
	}
	defer root.Close()
	parent := filepath.Dir(filepath.FromSlash(relative))
	if parent != "." {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return WriteReceipt{}, fmt.Errorf("create workspace directory: %w", err)
		}
	}
	if err := root.WriteFile(filepath.FromSlash(relative), []byte(content), 0o644); err != nil {
		return WriteReceipt{}, fmt.Errorf("write workspace file %s: %w", relative, err)
	}
	return WriteReceipt{Path: relative, Bytes: len(content)}, nil
}

// Edit applies non-overlapping exact replacements against one snapshot.
func (workspace *LocalWorkspace) Edit(ctx context.Context, input string, edits []TextEdit) (WriteReceipt, error) {
	if len(edits) == 0 {
		return WriteReceipt{}, errors.New("at least one edit is required")
	}
	read, err := workspace.Read(ctx, ReadRequest{Path: input, Limit: int(^uint(0) >> 1)})
	if err != nil {
		return WriteReceipt{}, err
	}
	content := read.Content
	type replacement struct {
		start, end int
		value      string
	}
	var replacements []replacement
	for _, edit := range edits {
		if edit.OldString == "" {
			return WriteReceipt{}, errors.New("edit old_string must not be empty")
		}
		starts := allIndexes(content, edit.OldString)
		if len(starts) == 0 {
			return WriteReceipt{}, errors.New("edit old_string was not found")
		}
		if len(starts) > 1 && !edit.ReplaceAll {
			return WriteReceipt{}, errors.New("edit old_string is ambiguous; set replace_all or provide more context")
		}
		if !edit.ReplaceAll {
			starts = starts[:1]
		}
		for _, start := range starts {
			replacements = append(replacements, replacement{start: start, end: start + len(edit.OldString), value: edit.NewString})
		}
	}
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	for index := 1; index < len(replacements); index++ {
		if replacements[index].start < replacements[index-1].end {
			return WriteReceipt{}, errors.New("edit replacements overlap")
		}
	}
	var output strings.Builder
	cursor := 0
	for _, replacement := range replacements {
		output.WriteString(content[cursor:replacement.start])
		output.WriteString(replacement.value)
		cursor = replacement.end
	}
	output.WriteString(content[cursor:])
	return workspace.Write(ctx, read.Path, output.String())
}

func normalizeReadWindow(offset, limit int) (int, int) {
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultReadLines
	}
	return offset, limit
}

func selectReadWindow(ctx context.Context, source io.Reader, offset, limit int) (string, error) {
	reader := bufio.NewReaderSize(source, 64*1024)
	var selected strings.Builder
	line, selectedLines := 1, 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fragment, err := reader.ReadSlice('\n')
		selecting := line >= offset && selectedLines < limit
		if selecting && len(fragment) > 0 {
			if selected.Len()+len(fragment) > defaultResultBytes {
				return "", fmt.Errorf(readWindowExceededError, defaultResultBytes)
			}
			selected.Write(fragment)
		}
		lineEnded := len(fragment) > 0 && fragment[len(fragment)-1] == '\n'
		if lineEnded || (errors.Is(err, io.EOF) && len(fragment) > 0) {
			if selecting {
				selectedLines++
			}
			line++
			if selectedLines >= limit {
				break
			}
		}
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if !errors.Is(err, io.EOF) {
				return "", fmt.Errorf("read workspace file: %w", err)
			}
			break
		}
	}
	return selected.String(), nil
}

func grepArguments(request GrepRequest, mode, relative string) []string {
	args := []string{"--color=never", "--no-messages"}
	switch mode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count-matches", "--with-filename")
	case "content":
		args = append(args, "--with-filename")
		if request.ShowLineNumbers == nil || *request.ShowLineNumbers {
			args = append(args, "--line-number")
		}
		before, after := request.BeforeLines, request.AfterLines
		if request.Context > 0 {
			before, after = request.Context, request.Context
		}
		if before > 0 {
			args = append(args, "-B", fmt.Sprintf("%d", before))
		}
		if after > 0 {
			args = append(args, "-A", fmt.Sprintf("%d", after))
		}
	}
	if request.CaseInsensitive {
		args = append(args, "--ignore-case")
	}
	if request.Multiline {
		args = append(args, "--multiline", "--multiline-dotall")
	}
	if glob := strings.TrimSpace(request.Glob); glob != "" {
		args = append(args, "--glob", glob)
	}
	if fileType := strings.TrimSpace(request.FileType); fileType != "" {
		args = append(args, "--type", fileType)
	}
	return append(args, "-e", request.Pattern, "--", filepath.FromSlash(relative))
}

func splitBoundedLines(content string, offset, limit int) []string {
	content = boundedString(content, defaultResultBytes)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	if offset >= len(lines) {
		return nil
	}
	if offset > 0 {
		lines = lines[offset:]
	}
	if limit > 0 && len(lines) > limit {
		lines = append(lines[:limit], resultTruncatedMarker)
	}
	return lines
}

func boundedString(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "\n" + resultTruncatedMarker
}

func hasParentComponent(pattern string) bool {
	for _, component := range strings.Split(pattern, "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

func allIndexes(content, needle string) []int {
	var result []int
	for cursor := 0; cursor <= len(content)-len(needle); {
		index := strings.Index(content[cursor:], needle)
		if index < 0 {
			break
		}
		index += cursor
		result = append(result, index)
		cursor = index + len(needle)
	}
	return result
}

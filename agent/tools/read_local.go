package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const readWindowExceededError = "selected read window exceeds %d bytes; use a narrower offset/limit or split the long line"

type localTextReadInput struct {
	Path   string `json:"path" jsonschema_description:"Absolute or workspace-relative path of the UTF-8 text file to read."`
	Offset int    `json:"offset,omitempty" jsonschema:"minimum=1" jsonschema_description:"One-based first line to return; defaults to 1."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum selected lines to return; defaults to 2000."`
}

// LocalTextAdapter reads bounded UTF-8 source windows.
func LocalTextAdapter(workspace *LocalWorkspace) (ReadAdapter, error) {
	if workspace == nil {
		return nil, errors.New("local text adapter workspace is nil")
	}
	return NewReadAdapter("local_text", func(ctx context.Context, input string) (bool, error) {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if hasResourceScheme(input) {
			return false, nil
		}
		_, info, err := workspace.stat(input, false)
		if err != nil {
			return false, err
		}
		return info.Mode().IsRegular(), nil
	}, func(ctx context.Context, input localTextReadInput) (ReadResult, error) {
		return workspace.readText(ctx, input)
	})
}

func (workspace *LocalWorkspace) readText(ctx context.Context, input localTextReadInput) (ReadResult, error) {
	relative, info, err := workspace.stat(input.Path, false)
	if err != nil {
		return ReadResult{}, err
	}
	if !info.Mode().IsRegular() {
		return ReadResult{}, fmt.Errorf("read local_text only supports regular files: %s", relative)
	}
	limits := workspace.Limits()
	offset, limit := normalizeReadWindow(input.Offset, input.Limit, limits.DefaultReadLines)
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
	content, returned, truncated, err := selectReadWindow(ctx, file, offset, limit, limits.MaxResultBytes)
	if err != nil {
		return ReadResult{}, err
	}
	if !utf8.ValidString(content) {
		return ReadResult{}, fmt.Errorf("read local_text only supports UTF-8 text files: %s", relative)
	}
	result := ReadResult{Path: relative, Kind: "local_text", Content: content, Offset: offset, Limit: returned, Truncated: truncated}
	if truncated && returned > 0 {
		result.NextOffset = offset + returned
	}
	return result, nil
}

type directoryReadInput struct {
	Path   string `json:"path" jsonschema_description:"Absolute or workspace-relative directory path to read."`
	Depth  int    `json:"depth,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum directory depth; defaults to the workspace read policy."`
	Hidden bool   `json:"hidden,omitempty" jsonschema_description:"Include dot-prefixed entries; defaults to false."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum returned entries; defaults to the workspace read policy."`
}

// DirectoryAdapter reads a stable bounded directory tree.
func DirectoryAdapter(workspace *LocalWorkspace) (ReadAdapter, error) {
	if workspace == nil {
		return nil, errors.New("directory adapter workspace is nil")
	}
	return NewReadAdapter("directory", func(ctx context.Context, input string) (bool, error) {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if hasResourceScheme(input) {
			return false, nil
		}
		_, info, err := workspace.stat(input, true)
		if err != nil {
			return false, err
		}
		return info.IsDir(), nil
	}, func(ctx context.Context, input directoryReadInput) (ReadResult, error) {
		return workspace.readDirectory(ctx, input)
	})
}

func (workspace *LocalWorkspace) readDirectory(ctx context.Context, input directoryReadInput) (ReadResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	relative, info, err := workspace.stat(input.Path, true)
	if err != nil {
		return ReadResult{}, err
	}
	if !info.IsDir() {
		return ReadResult{}, fmt.Errorf("read directory adapter requires a directory: %s", relative)
	}
	limits := workspace.Limits()
	depth := input.Depth
	if depth <= 0 {
		depth = limits.DefaultDirectoryDepth
	}
	if depth > limits.MaxDirectoryDepth {
		return ReadResult{}, fmt.Errorf("directory depth cannot exceed %d", limits.MaxDirectoryDepth)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = limits.DefaultDirectoryItems
	}
	if limit > limits.MaxResultEntries {
		return ReadResult{}, fmt.Errorf("directory limit cannot exceed %d", limits.MaxResultEntries)
	}
	root, err := workspace.openRoot()
	if err != nil {
		return ReadResult{}, err
	}
	defer root.Close()
	entries := make([]string, 0, min(limit, defaultDirectoryItems))
	outputBytes := 0
	truncated := false
	scanBudget := &workspaceScanBudget{}
	var walk func(string, int) error
	walk = func(directory string, level int) error {
		children, err := readWorkspaceDirectory(ctx, root, directory, scanBudget)
		if err != nil {
			return fmt.Errorf("read workspace directory %s: %w", directory, err)
		}
		for _, child := range children {
			if err := contextError(ctx); err != nil {
				return err
			}
			if !input.Hidden && strings.HasPrefix(child.Name(), ".") {
				continue
			}
			if !utf8.ValidString(child.Name()) {
				return fmt.Errorf("directory contains a non-UTF-8 entry under %s", directory)
			}
			if len(entries) >= limit {
				truncated = true
				return nil
			}
			childPath := child.Name()
			if directory != "." {
				childPath = path.Join(directory, child.Name())
			}
			entry := childPath
			if child.IsDir() {
				entry += "/"
			}
			contentBytes := len(entry)
			if len(entries) > 0 {
				contentBytes++
			}
			if outputBytes+contentBytes > limits.MaxResultBytes {
				if len(entries) == 0 {
					return fmt.Errorf("directory entry exceeds the %d-byte result limit", limits.MaxResultBytes)
				}
				truncated = true
				return nil
			}
			entries = append(entries, entry)
			outputBytes += contentBytes
			if child.IsDir() {
				if level < depth {
					if err := walk(childPath, level+1); err != nil {
						return err
					}
					if truncated {
						return nil
					}
				}
			}
		}
		return nil
	}
	if err := walk(relative, 1); err != nil {
		return ReadResult{}, err
	}
	result := ReadResult{
		Path: relative, Kind: "directory", Content: strings.Join(entries, "\n"),
		Limit: len(entries), Truncated: truncated,
	}
	if !truncated {
		result.Total = len(entries)
	}
	return result, nil
}

func normalizeReadWindow(offset, limit, defaultLimit int) (int, int) {
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	return offset, limit
}

func selectReadWindow(ctx context.Context, source io.Reader, offset, limit, maxBytes int) (string, int, bool, error) {
	reader := bufio.NewReaderSize(source, 64*1024)
	var selected strings.Builder
	line, selectedLines := 1, 0
	for {
		if err := contextError(ctx); err != nil {
			return "", 0, false, err
		}
		selecting := line >= offset && selectedLines < limit
		var current strings.Builder
		for {
			fragment, err := reader.ReadSlice('\n')
			if selecting && len(fragment) > 0 {
				if current.Len()+len(fragment) > maxBytes {
					if selectedLines == 0 {
						return "", 0, false, fmt.Errorf(readWindowExceededError, maxBytes)
					}
					return selected.String(), selectedLines, true, nil
				}
				current.Write(fragment)
			}
			lineEnded := len(fragment) > 0 && fragment[len(fragment)-1] == '\n'
			if lineEnded || (errors.Is(err, io.EOF) && len(fragment) > 0) {
				if selecting {
					if selected.Len()+current.Len() > maxBytes {
						if selectedLines == 0 {
							return "", 0, false, fmt.Errorf(readWindowExceededError, maxBytes)
						}
						return selected.String(), selectedLines, true, nil
					}
					selected.WriteString(current.String())
					selectedLines++
				}
				line++
			}
			if err == nil || errors.Is(err, bufio.ErrBufferFull) {
				if lineEnded {
					break
				}
				continue
			}
			if !errors.Is(err, io.EOF) {
				return "", 0, false, fmt.Errorf("read workspace file: %w", err)
			}
			if len(fragment) == 0 {
				return selected.String(), selectedLines, false, nil
			}
			break
		}
		if selectedLines >= limit {
			_, peekErr := reader.Peek(1)
			return selected.String(), selectedLines, peekErr == nil, nil
		}
	}
}

func hasResourceScheme(value string) bool {
	value = strings.TrimSpace(value)
	separator := strings.Index(value, "://")
	if separator <= 0 {
		return false
	}
	for _, character := range value[:separator] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '+' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

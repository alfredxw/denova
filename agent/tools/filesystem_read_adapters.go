package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type localTextReadInput struct {
	Path       string `json:"path" jsonschema_description:"Project-relative or absolute local path of the UTF-8 text file to read. External paths remain subject to permission."`
	Offset     int    `json:"offset,omitempty" jsonschema:"minimum=1" jsonschema_description:"One-based first line to return; defaults to 1."`
	ByteOffset int    `json:"byte_offset,omitempty" jsonschema:"minimum=0" jsonschema_description:"Zero-based UTF-8 byte offset within the selected first line, used only with an exact next_byte_offset continuation."`
	Limit      int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum selected lines to return; defaults to 2000."`
}

// LocalTextAdapter reads bounded UTF-8 source windows.
func LocalTextAdapter(workspace *LocalWorkspace) (ReadAdapter, error) {
	if workspace == nil {
		return nil, errors.New("local text adapter workspace is nil")
	}
	return NewReadAdapter(toolsetIdentity("tools.read.local_text", workspace.Identity()), "local_text", func(ctx context.Context, input string) (bool, error) {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if hasResourceScheme(input) {
			return false, nil
		}
		target, err := workspace.resolveReadPath(input, false)
		if err != nil {
			return false, err
		}
		return target.info.Mode().IsRegular(), nil
	}, func(ctx context.Context, input localTextReadInput) (ReadResult, error) {
		return workspace.readText(ctx, input)
	})
}

func (workspace *LocalWorkspace) readText(ctx context.Context, input localTextReadInput) (ReadResult, error) {
	target, err := workspace.resolveReadPath(input.Path, false)
	if err != nil {
		return ReadResult{}, err
	}
	if !target.info.Mode().IsRegular() {
		return ReadResult{}, fmt.Errorf("read local_text only supports regular files: %s", target.display)
	}
	limits := workspace.Limits()
	offset, limit := normalizeReadWindow(input.Offset, input.Limit, limits.DefaultReadLines)
	file, err := os.Open(target.absolute)
	if err != nil {
		return ReadResult{}, fmt.Errorf("open filesystem file %s: %w", target.display, err)
	}
	defer file.Close()
	selection, err := selectReadWindow(ctx, file, offset, input.ByteOffset, limit, limits.MaxResultBytes)
	if err != nil {
		return ReadResult{}, err
	}
	if !utf8.ValidString(selection.content) {
		return ReadResult{}, fmt.Errorf("read local_text only supports UTF-8 text files: %s", target.display)
	}
	return ReadResult{
		Path: target.display, Kind: "local_text", Content: selection.content,
		Offset: offset, ByteOffset: input.ByteOffset, Limit: selection.returned,
		Truncated: selection.truncated, NextOffset: selection.nextOffset, NextByteOffset: selection.nextByteOffset,
	}, nil
}

type directoryReadInput struct {
	Path   string `json:"path" jsonschema_description:"Project-relative or absolute local directory path to read. External paths remain subject to permission."`
	Depth  int    `json:"depth,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum directory depth; defaults to the filesystem read policy."`
	Hidden bool   `json:"hidden,omitempty" jsonschema_description:"Include dot-prefixed entries; defaults to false."`
	Limit  int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum returned entries; defaults to the filesystem read policy."`
}

// DirectoryAdapter reads a stable bounded directory tree.
func DirectoryAdapter(workspace *LocalWorkspace) (ReadAdapter, error) {
	if workspace == nil {
		return nil, errors.New("directory adapter workspace is nil")
	}
	return NewReadAdapter(toolsetIdentity("tools.read.directory", workspace.Identity()), "directory", func(ctx context.Context, input string) (bool, error) {
		if err := contextError(ctx); err != nil {
			return false, err
		}
		if hasResourceScheme(input) {
			return false, nil
		}
		target, err := workspace.resolveReadPath(input, true)
		if err != nil {
			return false, err
		}
		return target.info.IsDir(), nil
	}, func(ctx context.Context, input directoryReadInput) (ReadResult, error) {
		return workspace.readDirectory(ctx, input)
	})
}

func (workspace *LocalWorkspace) readDirectory(ctx context.Context, input directoryReadInput) (ReadResult, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	target, err := workspace.resolveReadPath(input.Path, true)
	if err != nil {
		return ReadResult{}, err
	}
	if !target.info.IsDir() {
		return ReadResult{}, fmt.Errorf("read directory adapter requires a directory: %s", target.display)
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
	rootPath := workspace.root
	startDirectory := target.relative
	if target.scope == FilesystemScopeExternal {
		rootPath = target.absolute
		startDirectory = "."
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return ReadResult{}, fmt.Errorf("open filesystem directory %s: %w", target.display, err)
	}
	defer root.Close()
	entries := make([]string, 0, min(limit, defaultDirectoryItems))
	outputBytes := 0
	truncated := false
	scanBudget := &filesystemScanBudget{}
	var walk func(string, int) error
	walk = func(directory string, level int) error {
		children, err := readFilesystemDirectory(ctx, root, directory, scanBudget)
		if err != nil {
			return fmt.Errorf("read filesystem directory %s: %w", directory, err)
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
			if target.scope == FilesystemScopeExternal {
				entry = filepath.ToSlash(filepath.Join(target.absolute, filepath.FromSlash(childPath)))
			}
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
	if err := walk(startDirectory, 1); err != nil {
		return ReadResult{}, err
	}
	result := ReadResult{
		Path: target.display, Kind: "directory", Content: strings.Join(entries, "\n"),
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

type readWindowSelection struct {
	content        string
	returned       int
	truncated      bool
	nextOffset     int
	nextByteOffset int
}

func selectReadWindow(ctx context.Context, source io.Reader, offset, byteOffset, limit, maxBytes int) (readWindowSelection, error) {
	reader := bufio.NewReaderSize(source, 64*1024)
	var selected strings.Builder
	line, selectedLines := 1, 0
	for {
		if err := contextError(ctx); err != nil {
			return readWindowSelection{}, err
		}
		selecting := line >= offset && selectedLines < limit
		var current strings.Builder
		lineBytes := 0
		for {
			fragment, err := reader.ReadSlice('\n')
			fragmentStart := lineBytes
			lineBytes += len(fragment)
			if selecting && len(fragment) > 0 {
				visible := fragment
				visibleStart := fragmentStart
				if line == offset && byteOffset > fragmentStart {
					drop := min(byteOffset-fragmentStart, len(fragment))
					visible = fragment[drop:]
					visibleStart += drop
				}
				if line == offset && visibleStart == byteOffset && len(visible) > 0 && !utf8.RuneStart(visible[0]) {
					return readWindowSelection{}, fmt.Errorf("read byte_offset=%d is not on a UTF-8 boundary", byteOffset)
				}
				available := maxBytes - selected.Len() - current.Len()
				if len(visible) > available {
					combined := current.String() + string(visible)
					prefixBytes := min(maxBytes-selected.Len(), len(combined))
					for prefixBytes > 0 && !utf8.ValidString(combined[:prefixBytes]) {
						prefixBytes--
					}
					if prefixBytes == 0 && selectedLines > 0 {
						return readWindowSelection{
							content: selected.String(), returned: selectedLines, truncated: true,
							nextOffset: line,
						}, nil
					}
					if prefixBytes == 0 {
						return readWindowSelection{}, fmt.Errorf("read result budget %d leaves no room for one UTF-8 character", maxBytes)
					}
					lineVisibleStart := 0
					if line == offset {
						lineVisibleStart = byteOffset
					}
					current.Reset()
					current.WriteString(combined[:prefixBytes])
					selected.WriteString(current.String())
					return readWindowSelection{
						content: selected.String(), returned: selectedLines + 1, truncated: true,
						nextOffset: line, nextByteOffset: lineVisibleStart + prefixBytes,
					}, nil
				}
				current.Write(visible)
			}
			lineEnded := len(fragment) > 0 && fragment[len(fragment)-1] == '\n'
			if lineEnded || (errors.Is(err, io.EOF) && len(fragment) > 0) {
				if selecting {
					lineContentBytes := lineBytes
					if lineEnded {
						lineContentBytes--
					}
					if line == offset && byteOffset > lineContentBytes {
						return readWindowSelection{}, fmt.Errorf("read byte_offset=%d exceeds line %d length", byteOffset, offset)
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
				return readWindowSelection{}, fmt.Errorf("read filesystem file: %w", err)
			}
			if len(fragment) == 0 {
				return readWindowSelection{content: selected.String(), returned: selectedLines}, nil
			}
			break
		}
		if selectedLines >= limit {
			_, peekErr := reader.Peek(1)
			truncated := peekErr == nil
			selection := readWindowSelection{content: selected.String(), returned: selectedLines, truncated: truncated}
			if truncated {
				selection.nextOffset = line
			}
			return selection, nil
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

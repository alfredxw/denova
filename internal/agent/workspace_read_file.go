package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alfredxw/denova/adk"
)

const workspaceReadFileResultSchema = "workspace_file.read.v2"

// Keep one selected window bounded even when a file contains a single very
// large line.
const workspaceReadFileMaxSelectedBytes = 1024 * 1024

var workspaceReadFileToolDescription = fmt.Sprintf(`Read a text file and return a bounded, line-numbered selection.
- file_path must be inside the active workspace and may be absolute or workspace-relative.
- By default this tool reads up to %d lines from line 1. Use offset and limit to continue reading later sections.
- The first result line is JSON pagination metadata.
- The selected text after the metadata is returned in cat -n format.

读取文本文件，返回有界的带行号选段。
- file_path 必须位于当前 workspace 内，可使用绝对路径或 workspace 相对路径。
- 默认从第 1 行开始最多读取 %d 行；需要继续读取后续部分时使用 offset 和 limit。
- 返回结果第一行是 JSON 分页元数据。
- 元数据后的选段使用 cat -n 行号格式。`, agentFileReadDefaultLimitLines, agentFileReadDefaultLimitLines)

type workspaceReadFileInput struct {
	FilePath string `json:"file_path" jsonschema_description:"Absolute or workspace-relative path of the text file to read."`
	Offset   int    `json:"offset,omitempty" jsonschema_description:"One-based first line to return; defaults to 1."`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"Maximum selected lines to return; defaults to 2000."`
}

type workspaceReadFileMetadata struct {
	Schema   string `json:"schema"`
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

func newWorkspaceReadFileTool(backend *agentFilesystemBackend) (adk.BaseTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	return adk.InferTool("read_file", workspaceReadFileToolDescription, func(ctx context.Context, input workspaceReadFileInput) (string, error) {
		offset, limit := normalizeWorkspaceReadWindow(input.Offset, input.Limit)
		filePath, content, err := backend.ReadFileSelection(ctx, input.FilePath, offset, limit)
		if err != nil {
			return "", err
		}
		metadata, err := json.Marshal(workspaceReadFileMetadata{
			Schema:   workspaceReadFileResultSchema,
			FilePath: filePath,
			Offset:   offset,
			Limit:    limit,
		})
		if err != nil {
			return "", fmt.Errorf("serialize read_file metadata: %w", err)
		}
		return string(metadata) + "\n" + formatWorkspaceLineNumbers(content, offset), nil
	})
}

func (b *agentFilesystemBackend) ReadFileSelection(ctx context.Context, input string, offset, limit int) (string, string, error) {
	filePath, rel, _, err := b.validateExistingPath(input, false)
	if err != nil {
		return "", "", err
	}
	root, err := b.openRoot()
	if err != nil {
		return "", "", err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("file not found: %s", filePath)
		}
		return "", "", fmt.Errorf("open workspace file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", "", fmt.Errorf("inspect workspace file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("read_file only supports regular text files: %s", filePath)
	}
	content, err := selectWorkspaceFileWindow(ctx, file, offset, limit)
	if err != nil {
		return "", "", err
	}
	if !utf8.ValidString(content) {
		return "", "", fmt.Errorf("read_file only supports UTF-8 text files: %s", filePath)
	}
	return filePath, content, nil
}

func selectWorkspaceFileWindow(ctx context.Context, source io.Reader, offset, limit int) (string, error) {
	offset, limit = normalizeWorkspaceReadWindow(offset, limit)
	reader := bufio.NewReaderSize(&contextFileReader{ctx: ctx, reader: source}, 64*1024)
	var selected strings.Builder
	lineNumber := 1
	selectedLines := 0
	for {
		fragment, err := reader.ReadSlice('\n')
		selecting := lineNumber >= offset && selectedLines < limit
		if selecting && len(fragment) > 0 {
			if selected.Len()+len(fragment) > workspaceReadFileMaxSelectedBytes {
				return "", fmt.Errorf(
					"selected read_file window exceeds %d bytes; use a narrower offset/limit or split the long line",
					workspaceReadFileMaxSelectedBytes,
				)
			}
			selected.Write(fragment)
		}
		lineEnded := len(fragment) > 0 && fragment[len(fragment)-1] == '\n'
		if lineEnded || (errors.Is(err, io.EOF) && len(fragment) > 0) {
			if selecting {
				selectedLines++
			}
			lineNumber++
			if selectedLines >= limit {
				break
			}
		}
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if err != io.EOF {
				return "", fmt.Errorf("error reading file: %w", err)
			}
			break
		}
	}
	return selected.String(), nil
}

type contextFileReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextFileReader) Read(buffer []byte) (int, error) {
	if r.ctx != nil {
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		default:
		}
	}
	return r.reader.Read(buffer)
}

func normalizeWorkspaceReadWindow(offset, limit int) (int, int) {
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = agentFileReadDefaultLimitLines
	}
	return offset, limit
}

func formatWorkspaceLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var result strings.Builder
	for index, line := range lines {
		if index < len(lines)-1 || strings.HasSuffix(content, "\n") {
			fmt.Fprintf(&result, "%6d\t%s\n", startLine+index, line)
		} else {
			fmt.Fprintf(&result, "%6d\t%s", startLine+index, line)
		}
	}
	return result.String()
}

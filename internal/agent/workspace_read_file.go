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

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const workspaceReadFileResultSchema = "workspace_file.read.v2"

// Keep one selected window bounded even when a file contains a single very
// large line.
const workspaceReadFileMaxSelectedBytes = 1024 * 1024

var workspaceReadFileToolDescription = fmt.Sprintf(`Read a text file and return a bounded, line-numbered selection.
- file_path must be an absolute path.
- By default this tool reads up to %d lines from line 1. Use offset and limit to continue reading later sections.
- The first result line is JSON pagination metadata.
- The selected text after the metadata is returned in cat -n format.

读取文本文件，返回有界的带行号选段。
- file_path 必须是绝对路径。
- 默认从第 1 行开始最多读取 %d 行；需要继续读取后续部分时使用 offset 和 limit。
- 返回结果第一行是 JSON 分页元数据。
- 元数据后的选段使用 cat -n 行号格式。`, agentFileReadDefaultLimitLines, agentFileReadDefaultLimitLines)

type workspaceReadFileInput struct {
	FilePath string `json:"file_path" jsonschema:"required,description=Absolute path of the text file to read"`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=One-based first line to return; defaults to 1"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Maximum selected lines to return; defaults to 2000"`
}

type workspaceReadFileMetadata struct {
	Schema   string `json:"schema"`
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

// workspaceFileSelectionReader lets the production backend keep reads rooted
// inside the active workspace while selecting only the requested window.
type workspaceFileSelectionReader interface {
	ReadFileSelection(context.Context, *filesystem.ReadRequest) (string, error)
}

func newWorkspaceReadFileTool(backend filesystem.Backend, workspaces ...string) (tool.BaseTool, error) {
	if backend == nil {
		return nil, fmt.Errorf("filesystem backend is nil")
	}
	workspace := ""
	if len(workspaces) > 0 {
		workspace = strings.TrimSpace(workspaces[0])
	}
	return utils.InferTool("read_file", workspaceReadFileToolDescription, func(ctx context.Context, input workspaceReadFileInput) (string, error) {
		filePath, _, err := resolveWorkspaceReadPath(workspace, input.FilePath)
		if err != nil {
			return "", err
		}
		offset, limit := normalizeWorkspaceReadWindow(input.Offset, input.Limit)
		content, err := readWorkspaceFileSelection(ctx, backend, &filesystem.ReadRequest{
			FilePath: filePath,
			Offset:   offset,
			Limit:    limit,
		})
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

func readWorkspaceFileSelection(ctx context.Context, backend filesystem.Backend, req *filesystem.ReadRequest) (string, error) {
	if reader, ok := backend.(workspaceFileSelectionReader); ok {
		return reader.ReadFileSelection(ctx, req)
	}
	selected, err := backend.Read(ctx, req)
	if err != nil {
		return "", err
	}
	if selected == nil {
		return "", fmt.Errorf("no content found at path: %s", req.FilePath)
	}
	if len(selected.Content) > workspaceReadFileMaxSelectedBytes {
		return "", fmt.Errorf(
			"selected read_file window exceeds %d bytes; use a narrower offset/limit or split the long line",
			workspaceReadFileMaxSelectedBytes,
		)
	}
	return selected.Content, nil
}

func (b *agentFilesystemBackend) ReadFileSelection(ctx context.Context, req *filesystem.ReadRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("read request is nil")
	}
	if b == nil || b.Backend == nil {
		return "", fmt.Errorf("filesystem backend is nil")
	}
	filePath, rel, err := resolveWorkspaceReadPath(b.workspace, req.FilePath)
	if err != nil {
		return "", err
	}
	var file *os.File
	if b.workspace != "" {
		root, rootErr := os.OpenRoot(b.workspace)
		if rootErr != nil {
			return "", rootErr
		}
		defer root.Close()
		file, err = root.Open(filepath.FromSlash(rel))
	} else {
		file, err = os.Open(filePath)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", filePath)
		}
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	return selectWorkspaceFileWindow(ctx, file, req.Offset, req.Limit)
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

func resolveWorkspaceReadPath(workspace, input string) (absolute, relative string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("file_path is required")
	}
	if !filepath.IsAbs(input) {
		return "", "", fmt.Errorf("file_path must be absolute: %s", input)
	}
	absolute = filepath.Clean(input)
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return absolute, "", nil
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	relative, err = filepath.Rel(filepath.Clean(workspace), absolute)
	if err != nil {
		return "", "", err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("file_path is outside the active workspace: %s", absolute)
	}
	return absolute, filepath.ToSlash(relative), nil
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
	lines := strings.Split(content, "\n")
	var result strings.Builder
	for index, line := range lines {
		if index < len(lines)-1 {
			fmt.Fprintf(&result, "%6d\t%s\n", startLine+index, line)
		} else {
			fmt.Fprintf(&result, "%6d\t%s", startLine+index, line)
		}
	}
	return result.String()
}

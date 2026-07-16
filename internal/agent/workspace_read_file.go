package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const workspaceReadFileResultSchema = "workspace_file.read.v1"

// Keep one selected window bounded even when a file contains a single very
// large line. The complete file is still streamed through SHA-256.
const workspaceReadFileMaxSelectedBytes = 1024 * 1024

var workspaceReadFileToolDescription = fmt.Sprintf(`Read a text file and return a bounded, line-numbered selection plus the revision of the complete file.
- file_path must be an absolute path.
- By default this tool reads up to %d lines from line 1. Use offset and limit to continue reading later sections.
- The first result line is JSON metadata. revision is always the sha256 of the complete file, even when offset/limit returns only part of it.
- Pass that revision as base_revision to edit_file or write_file. Re-read after a revision conflict.
- The selected text after the metadata is returned in cat -n format.

读取文本文件，返回有界的带行号选段，以及完整文件的版本号。
- file_path 必须是绝对路径。
- 默认从第 1 行开始最多读取 %d 行；需要继续读取后续部分时使用 offset 和 limit。
- 返回结果第一行是 JSON 元数据。即使 offset/limit 只返回部分内容，revision 也始终是完整文件的 sha256。
- 调用 edit_file 或 write_file 时必须把该 revision 作为 base_revision 传入；版本冲突后重新读取。
- 元数据后的选段使用 cat -n 行号格式。`, agentFileReadDefaultLimitLines, agentFileReadDefaultLimitLines)

type workspaceReadFileInput struct {
	FilePath string `json:"file_path" jsonschema:"required,description=Absolute path of the text file to read"`
	Offset   int    `json:"offset,omitempty" jsonschema:"description=One-based first line to return; defaults to 1"`
	Limit    int    `json:"limit,omitempty" jsonschema:"description=Maximum selected lines to return; defaults to 2000"`
}

type workspaceReadFileMetadata struct {
	Schema        string `json:"schema"`
	FilePath      string `json:"file_path"`
	Revision      string `json:"revision"`
	RevisionScope string `json:"revision_scope"`
	Offset        int    `json:"offset"`
	Limit         int    `json:"limit"`
}

type workspaceFileSnapshot struct {
	Content  string
	Revision string
}

// workspaceFileSnapshotReader lets the production local backend hash the open
// file descriptor while selecting lines. That keeps a partial read and its
// full-file revision bound to one filesystem snapshot.
type workspaceFileSnapshotReader interface {
	ReadFileSnapshot(context.Context, *filesystem.ReadRequest) (workspaceFileSnapshot, error)
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
		snapshot, err := readWorkspaceFileSnapshot(ctx, backend, &filesystem.ReadRequest{
			FilePath: filePath,
			Offset:   offset,
			Limit:    limit,
		})
		if err != nil {
			return "", err
		}
		metadata, err := json.Marshal(workspaceReadFileMetadata{
			Schema:        workspaceReadFileResultSchema,
			FilePath:      filePath,
			Revision:      snapshot.Revision,
			RevisionScope: "full_file",
			Offset:        offset,
			Limit:         limit,
		})
		if err != nil {
			return "", fmt.Errorf("serialize read_file metadata: %w", err)
		}
		return string(metadata) + "\n" + formatWorkspaceLineNumbers(snapshot.Content, offset), nil
	})
}

func readWorkspaceFileSnapshot(ctx context.Context, backend filesystem.Backend, req *filesystem.ReadRequest) (workspaceFileSnapshot, error) {
	if reader, ok := backend.(workspaceFileSnapshotReader); ok {
		return reader.ReadFileSnapshot(ctx, req)
	}
	// Generic backends do not expose a revisioned snapshot API. Ask for every
	// line once, then derive both the selection and revision from that response.
	full, err := backend.Read(ctx, &filesystem.ReadRequest{
		FilePath: req.FilePath,
		Offset:   1,
		Limit:    math.MaxInt,
	})
	if err != nil {
		return workspaceFileSnapshot{}, err
	}
	if full == nil {
		return workspaceFileSnapshot{}, fmt.Errorf("no content found at path: %s", req.FilePath)
	}
	return snapshotFromReader(ctx, strings.NewReader(full.Content), req.Offset, req.Limit)
}

func (b *agentFilesystemBackend) ReadFileSnapshot(ctx context.Context, req *filesystem.ReadRequest) (workspaceFileSnapshot, error) {
	if req == nil {
		return workspaceFileSnapshot{}, fmt.Errorf("read request is nil")
	}
	if b == nil || b.Backend == nil {
		return workspaceFileSnapshot{}, fmt.Errorf("filesystem backend is nil")
	}
	filePath, rel, err := resolveWorkspaceReadPath(b.workspace, req.FilePath)
	if err != nil {
		return workspaceFileSnapshot{}, err
	}
	var file *os.File
	if b.workspace != "" {
		root, rootErr := os.OpenRoot(b.workspace)
		if rootErr != nil {
			return workspaceFileSnapshot{}, rootErr
		}
		defer root.Close()
		file, err = root.Open(filepath.FromSlash(rel))
	} else {
		file, err = os.Open(filePath)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceFileSnapshot{}, fmt.Errorf("file not found: %s", filePath)
		}
		return workspaceFileSnapshot{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	return snapshotFromReader(ctx, file, req.Offset, req.Limit)
}

func snapshotFromReader(ctx context.Context, source io.Reader, offset, limit int) (workspaceFileSnapshot, error) {
	offset, limit = normalizeWorkspaceReadWindow(offset, limit)
	hash := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(&contextFileReader{ctx: ctx, reader: source}, hash), 64*1024)
	var selected strings.Builder
	lineNumber := 1
	selectedLines := 0
	for {
		fragment, err := reader.ReadSlice('\n')
		selecting := lineNumber >= offset && selectedLines < limit
		if selecting && len(fragment) > 0 {
			if selected.Len()+len(fragment) > workspaceReadFileMaxSelectedBytes {
				return workspaceFileSnapshot{}, fmt.Errorf(
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
		}
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				continue
			}
			if err != io.EOF {
				return workspaceFileSnapshot{}, fmt.Errorf("error reading file: %w", err)
			}
			break
		}
	}
	return workspaceFileSnapshot{
		Content:  selected.String(),
		Revision: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
	}, nil
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

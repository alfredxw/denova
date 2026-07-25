package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// DefinitionOption customizes a standard tool's runtime contract without
// changing its model-visible schema.
type DefinitionOption func(*agent.ToolDescriptor)

// WithCapability associates a product-defined authorization capability.
func WithCapability(capability string) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) { descriptor.Capability = capability }
}

// WithMaxResultBytes overrides the model-context result ceiling.
func WithMaxResultBytes(limit int) DefinitionOption {
	return func(descriptor *agent.ToolDescriptor) { descriptor.MaxResultBytes = limit }
}

func applyDefinitionOptions(descriptor agent.ToolDescriptor, options []DefinitionOption) agent.ToolDescriptor {
	for _, option := range options {
		if option != nil {
			option(&descriptor)
		}
	}
	return descriptor
}

func readDescriptor(options ...DefinitionOption) agent.ToolDescriptor {
	return applyDefinitionOptions(agent.ToolDescriptor{
		Source:           agent.ToolSourceRead,
		Execution:        agent.ToolExecutionParallelRead,
		Recovery:         agent.ToolRecoveryReadOnly,
		ResultProjection: agent.ToolResultBoundedModelContext,
		Steering:         agent.SteeringFinishCurrent,
		MaxResultBytes:   defaultResultBytes,
	}, options)
}

func writeDescriptor(options ...DefinitionOption) agent.ToolDescriptor {
	return applyDefinitionOptions(agent.ToolDescriptor{
		Source:            agent.ToolSourceWrite,
		Execution:         agent.ToolExecutionWorkspaceExclusive,
		Recovery:          agent.ToolRecoveryReconcilable,
		ResultProjection:  agent.ToolResultBoundedModelContext,
		Steering:          agent.SteeringFinishCurrent,
		MutatesWorkspace:  true,
		MaxResultBytes:    defaultResultBytes,
		RequiresPostCheck: true,
	}, options)
}

type readFileInput struct {
	FilePath string `json:"file_path" jsonschema_description:"Absolute or workspace-relative path of the text file to read."`
	Offset   int    `json:"offset,omitempty" jsonschema_description:"One-based first line to return; defaults to 1."`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"Maximum selected lines to return; defaults to 2000."`
}

type readFileMetadata struct {
	Schema   string `json:"schema"`
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

// ReadFile defines the bounded, line-numbered read_file tool.
func ReadFile(reader Reader, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if reader == nil {
		return agent.ToolDefinition{}, errors.New("read_file Reader is nil")
	}
	tool, err := agent.InferTool("read_file", readFileDescription, func(ctx context.Context, input readFileInput) (string, error) {
		result, err := reader.Read(ctx, ReadRequest{Path: input.FilePath, Offset: input.Offset, Limit: input.Limit})
		if err != nil {
			return "", err
		}
		metadata, err := json.Marshal(readFileMetadata{
			Schema: "workspace_file.read.v2", FilePath: result.Path,
			Offset: result.Offset, Limit: result.Limit,
		})
		if err != nil {
			return "", fmt.Errorf("serialize read_file metadata: %w", err)
		}
		return string(metadata) + "\n" + lineNumbers(result.Content, result.Offset), nil
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: readDescriptor(options...)}, err
}

type listInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Workspace-relative or absolute directory path; defaults to the workspace root."`
}

// List defines the stable ls tool.
func List(searcher Searcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("ls Searcher is nil")
	}
	tool, err := agent.InferTool("ls", listDescription, func(ctx context.Context, input listInput) (string, error) {
		entries, err := searcher.List(ctx, input.Path)
		if err != nil {
			return "", err
		}
		return joinedResult(entries, "No files found in the selected directory."), nil
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: readDescriptor(options...)}, err
}

type globInput struct {
	Pattern string `json:"pattern" jsonschema_description:"Workspace-relative glob pattern such as **/*.go or chapters/*.md."`
	Path    string `json:"path,omitempty" jsonschema_description:"Workspace-relative or absolute directory to search; defaults to the workspace root."`
}

// Glob defines the stable glob tool.
func Glob(searcher Searcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("glob Searcher is nil")
	}
	tool, err := agent.InferTool("glob", globDescription, func(ctx context.Context, input globInput) (string, error) {
		entries, err := searcher.Glob(ctx, input.Path, input.Pattern)
		if err != nil {
			return "", err
		}
		return joinedResult(entries, "No files matched the glob pattern."), nil
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: readDescriptor(options...)}, err
}

type grepInput struct {
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

// Grep defines the stable grep tool.
func Grep(searcher Searcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("grep Searcher is nil")
	}
	tool, err := agent.InferTool("grep", grepDescription, func(ctx context.Context, input grepInput) (string, error) {
		entries, err := searcher.Grep(ctx, GrepRequest{
			Pattern: input.Pattern, Path: input.Path, Glob: input.Glob, OutputMode: input.OutputMode,
			Context: input.Context, BeforeLines: input.BeforeLines, AfterLines: input.AfterLines,
			ShowLineNumbers: input.ShowLineNumbers, CaseInsensitive: input.CaseInsensitive,
			FileType: input.FileType, HeadLimit: input.HeadLimit, Offset: input.Offset, Multiline: input.Multiline,
		})
		if err != nil {
			return "", err
		}
		return joinedResult(entries, "No matches found."), nil
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: readDescriptor(options...)}, err
}

type writeFileInput struct {
	FilePath string `json:"file_path" jsonschema_description:"Absolute or workspace-relative path of the file to replace."`
	Content  string `json:"content" jsonschema_description:"Complete new file content."`
}

// WriteFile defines a complete-file replacement tool over an injected Writer.
func WriteFile(writer Writer, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if writer == nil {
		return agent.ToolDefinition{}, errors.New("write_file Writer is nil")
	}
	tool, err := agent.InferTool("write_file", writeDescription, func(ctx context.Context, input writeFileInput) (agent.ToolResult, error) {
		receipt, err := writer.Write(ctx, input.FilePath, input.Content)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return structuredWriteReceipt("workspace_file.write.v1", receipt)
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: writeDescriptor(options...)}, err
}

type editFileInput struct {
	FilePath string         `json:"file_path" jsonschema_description:"Absolute or workspace-relative path of the single file to edit."`
	Edits    []editFileEdit `json:"edits" jsonschema:"minItems=1" jsonschema_description:"One or more non-overlapping exact replacements evaluated against the same original file snapshot."`
}

type editFileEdit struct {
	OldString  string `json:"old_string" jsonschema_description:"Exact non-empty text to replace in the original file snapshot."`
	NewString  string `json:"new_string" jsonschema_description:"Replacement text; an empty string deletes the matched text."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every exact occurrence of old_string; defaults to false."`
}

// EditFile defines exact text replacement over an injected Writer.
func EditFile(writer Writer, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if writer == nil {
		return agent.ToolDefinition{}, errors.New("edit_file Writer is nil")
	}
	tool, err := agent.InferTool("edit_file", editDescription, func(ctx context.Context, input editFileInput) (agent.ToolResult, error) {
		edits := make([]TextEdit, 0, len(input.Edits))
		for _, edit := range input.Edits {
			edits = append(edits, TextEdit{OldString: edit.OldString, NewString: edit.NewString, ReplaceAll: edit.ReplaceAll})
		}
		receipt, err := writer.Edit(ctx, input.FilePath, edits)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return structuredWriteReceipt("workspace_file.edit.v1", receipt)
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: writeDescriptor(options...)}, err
}

func marshalWriteReceipt(schema string, receipt WriteReceipt) (string, error) {
	payload := struct {
		Schema string `json:"schema"`
		WriteReceipt
	}{Schema: schema, WriteReceipt: receipt}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("serialize workspace mutation receipt: %w", err)
	}
	return string(data), nil
}

func structuredWriteReceipt(schema string, receipt WriteReceipt) (agent.ToolResult, error) {
	content, err := marshalWriteReceipt(schema, receipt)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result := agent.TextToolResult(content)
	result.Details = json.RawMessage(content)
	return result, nil
}

func lineNumbers(content string, start int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var result strings.Builder
	for index, line := range lines {
		fmt.Fprintf(&result, "%6d\t%s", start+index, line)
		if index < len(lines)-1 || strings.HasSuffix(content, "\n") {
			result.WriteByte('\n')
		}
	}
	return result.String()
}

func joinedResult(entries []string, empty string) string {
	if len(entries) == 0 {
		return empty
	}
	return boundedString(strings.Join(entries, "\n"), defaultResultBytes)
}

const readFileDescription = `Read a UTF-8 text file and return one bounded, line-numbered selection. file_path must stay inside the active workspace. Use offset and limit to continue later sections.

读取 UTF-8 文本文件并返回有界、带行号的选段。file_path 必须位于当前工作区内；使用 offset 和 limit 继续读取后续内容。`

const listDescription = `List one workspace directory in stable order. Paths are workspace-relative and directories end with /.

稳定列出工作区内一层目录；路径相对工作区，目录以 / 结尾。`

const globDescription = `Find workspace files with a bounded recursive glob such as **/*.go. Patterns cannot escape the workspace and directory symlinks are not followed.

使用有界递归 glob（如 **/*.go）查找工作区文件；模式不能越出工作区，也不会跟随目录符号链接。`

const grepDescription = `Search workspace text with ripgrep syntax and bounded output. Prefer files_with_matches unless exact matching lines are needed.

使用 ripgrep 语法在工作区内搜索文本，输出有硬上限；除非需要具体匹配行，否则优先 files_with_matches。`

const writeDescription = `Replace one complete workspace file through the configured mutation boundary. Use edit_file for localized changes.

通过已配置的变更边界完整替换一个工作区文件；局部修改请使用 edit_file。`

const editDescription = `Apply exact, non-overlapping replacements against one original workspace file snapshot.

基于同一份工作区文件原始快照应用精确、互不重叠的文本替换。`

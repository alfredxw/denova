package tools

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// GlobRequest describes one bounded, workspace-relative path discovery call.
type GlobRequest struct {
	Paths     []string
	Hidden    bool
	Gitignore bool
	Limit     int
}

// GrepRequest describes one bounded ripgrep-compatible content search.
type GrepRequest struct {
	Pattern       string
	Paths         []string
	Mode          string
	CaseSensitive bool
	Gitignore     bool
	ContextBefore int
	ContextAfter  int
	Limit         int
	Cursor        string
}

// SearchResult is shared by glob and grep without leaking local process
// details into the model-visible Interface.
type SearchResult struct {
	Entries    []string
	Truncated  bool
	NextCursor string
	Warnings   []string
}

// Searcher is the reusable workspace-search seam behind glob and grep.
type Searcher interface {
	Glob(context.Context, GlobRequest) (SearchResult, error)
	Grep(context.Context, GrepRequest) (SearchResult, error)
}

type globInput struct {
	Paths     []string `json:"paths,omitempty" jsonschema:"minItems=1,maxItems=256" jsonschema_description:"Workspace-relative files, directories, or glob patterns. Omit to discover from the workspace root."`
	Hidden    *bool    `json:"hidden,omitempty" jsonschema_description:"Include dot-prefixed paths; defaults to true."`
	Gitignore *bool    `json:"gitignore,omitempty" jsonschema_description:"Respect .gitignore rules; defaults to true."`
	Limit     int      `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum returned paths; defaults to the workspace search policy."`
}

// Glob defines multi-path workspace discovery. Directory reading remains the
// responsibility of read; glob only answers path-pattern questions.
func Glob(searcher Searcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("glob Searcher is nil")
	}
	descriptor := readDescriptor(options...)
	tool, err := agent.InferTool("glob", `Find workspace files or directories by path. A call may include several files, directories, or glob patterns; results are de-duplicated and bounded. Use read on a directory when you need its structure.

按路径查找工作区文件或目录。一次可传入多个文件、目录或 glob 模式；结果会去重并受限。需要理解目录结构时请使用 read。`, func(ctx context.Context, input globInput) (agent.ToolResult, error) {
		result, err := searcher.Glob(ctx, GlobRequest{
			Paths: input.Paths, Hidden: boolDefault(input.Hidden, true),
			Gitignore: boolDefault(input.Gitignore, true), Limit: input.Limit,
		})
		if err != nil {
			return agent.ToolResult{}, err
		}
		return searchToolResult("glob", result, descriptor.MaxResultBytes, nil)
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: descriptor}, err
}

type grepInput struct {
	Pattern       string   `json:"pattern" jsonschema_description:"Ripgrep regular expression to search for; a newline automatically enables multiline matching."`
	Paths         []string `json:"paths,omitempty" jsonschema:"minItems=1,maxItems=256" jsonschema_description:"Workspace-relative files, directories, or glob patterns. Omit to search the workspace root."`
	Mode          string   `json:"mode,omitempty" jsonschema:"enum=content,enum=files,enum=count" jsonschema_description:"content, files, or count; defaults to content."`
	CaseSensitive *bool    `json:"case_sensitive,omitempty" jsonschema_description:"Use case-sensitive matching; defaults to true."`
	Gitignore     *bool    `json:"gitignore,omitempty" jsonschema_description:"Respect .gitignore rules; defaults to true."`
	ContextBefore int      `json:"context_before,omitempty" jsonschema:"minimum=0" jsonschema_description:"Lines before each match in content mode."`
	ContextAfter  int      `json:"context_after,omitempty" jsonschema:"minimum=0" jsonschema_description:"Lines after each match in content mode."`
	Limit         int      `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum returned lines or entries; defaults to the workspace search policy."`
	Cursor        string   `json:"cursor,omitempty" jsonschema_description:"Opaque continuation cursor returned by an earlier identical grep call."`
}

// Grep defines bounded multi-path text search over ripgrep.
func Grep(searcher Searcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("grep Searcher is nil")
	}
	descriptor := readDescriptor(options...)
	tool, err := agent.InferTool("grep", `Search workspace text with ripgrep regular expressions. Content mode returns copyable paths, stable line numbers, and original context suitable for edit.old_string. A cursor is valid only for the identical query while the workspace remains unchanged; restart from the first page after mutations.

使用 ripgrep 正则搜索工作区文本。content 模式返回可复制路径、稳定行号和原始上下文，可用于 edit.old_string。游标仅适用于工作区未变化时的同一查询；工作区变更后请从第一页重新搜索。`, func(ctx context.Context, input grepInput) (agent.ToolResult, error) {
		request := GrepRequest{
			Pattern: input.Pattern, Paths: input.Paths, Mode: input.Mode,
			CaseSensitive: boolDefault(input.CaseSensitive, true),
			Gitignore:     boolDefault(input.Gitignore, true),
			ContextBefore: input.ContextBefore, ContextAfter: input.ContextAfter,
			Limit: input.Limit, Cursor: input.Cursor,
		}
		request = normalizeGrepRequest(request)
		offset, err := decodeGrepCursor(request.Cursor, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		result, err := searcher.Grep(ctx, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return searchToolResult("grep", result, descriptor.MaxResultBytes, func(returned int) (string, error) {
			return encodeGrepCursor(offset+returned, request)
		})
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: descriptor}, err
}

type searchEnvelope struct {
	Schema   string          `json:"schema"`
	Status   string          `json:"status"`
	Source   searchSource    `json:"source"`
	Limits   searchLimits    `json:"limits"`
	Warnings []string        `json:"warnings,omitempty"`
	Recovery *searchRecovery `json:"recovery,omitempty"`
}

type searchSource struct {
	Kind string `json:"kind"`
}

type searchLimits struct {
	Returned   int    `json:"returned"`
	Truncated  bool   `json:"truncated"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type searchRecovery struct {
	Retryable  bool   `json:"retryable"`
	Suggestion string `json:"suggestion"`
}

func searchToolResult(
	kind string,
	result SearchResult,
	maxResultBytes int,
	continuation func(int) (string, error),
) (agent.ToolResult, error) {
	build := func(visible int) (agent.ToolResult, error) {
		truncated := result.Truncated || visible < len(result.Entries)
		nextCursor := result.NextCursor
		if truncated && continuation != nil {
			var err error
			nextCursor, err = continuation(visible)
			if err != nil {
				return agent.ToolResult{}, fmt.Errorf("encode %s continuation: %w", kind, err)
			}
		} else if visible < len(result.Entries) {
			nextCursor = ""
		}
		status := "success"
		limits := searchLimits{Returned: visible, Truncated: truncated, NextCursor: nextCursor}
		var recovery *searchRecovery
		if truncated {
			status = "partial"
			recovery = &searchRecovery{
				Retryable:  true,
				Suggestion: "Use next_cursor when present, otherwise narrow paths or pattern. / 如有 next_cursor 请继续分页，否则缩小路径或模式。",
			}
		}
		metadata := searchEnvelope{
			Schema: "workspace.search.v1", Status: status,
			Source: searchSource{Kind: kind}, Limits: limits,
			Warnings: result.Warnings, Recovery: recovery,
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("serialize %s result: %w", kind, err)
		}
		content := string(encoded)
		if visible > 0 {
			content += "\n" + strings.Join(result.Entries[:visible], "\n")
		}
		return agent.ToolResult{
			ModelContent: content, DisplayContent: content, Details: encoded,
			Status: agent.ToolResultSuccess,
		}, nil
	}
	full, err := build(len(result.Entries))
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(full.ModelContent) <= maxResultBytes && len(full.Details) <= maxResultBytes {
		return full, nil
	}
	low, high := 0, len(result.Entries)-1
	bestVisible := -1
	var best agent.ToolResult
	for low <= high {
		middle := low + (high-low)/2
		candidate, err := build(middle)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if len(candidate.ModelContent) <= maxResultBytes && len(candidate.Details) <= maxResultBytes {
			best, bestVisible = candidate, middle
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if bestVisible < 0 {
		return agent.ToolResult{}, fmt.Errorf("%s result metadata exceeds the %d-byte result limit", kind, maxResultBytes)
	}
	if len(result.Entries) > 0 && bestVisible == 0 {
		return agent.ToolResult{}, fmt.Errorf("%s result leaves no room for one complete entry within the %d-byte result limit", kind, maxResultBytes)
	}
	return best, nil
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

type grepCursor struct {
	Version     int    `json:"v"`
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

func encodeGrepCursor(offset int, request GrepRequest) (string, error) {
	cursor := grepCursor{Version: 1, Offset: offset, Fingerprint: grepRequestFingerprint(request)}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeGrepCursor(value string, request GrepRequest) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, errors.New("grep cursor is invalid")
	}
	var cursor grepCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || cursor.Offset < 0 {
		return 0, errors.New("grep cursor is invalid")
	}
	if cursor.Fingerprint != grepRequestFingerprint(request) {
		return 0, errors.New("grep cursor does not belong to this query")
	}
	return cursor.Offset, nil
}

func grepRequestFingerprint(request GrepRequest) string {
	request.Cursor = ""
	request.Limit = 0
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

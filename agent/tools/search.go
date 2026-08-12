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
	Cursor    string
}

// GrepRequest describes one bounded, command-shaped ripgrep search. Command
// uses Denova's literal rg grammar; it is never passed through a shell.
type GrepRequest struct {
	Command string
	Cursor  string
}

// SearchResult is shared by glob and grep without leaking local process
// details into the model-visible Interface.
type SearchResult struct {
	Entries    []string
	Truncated  bool
	NextCursor string
	Warnings   []string
}

// GlobSearcher is the reusable workspace path-discovery seam. Identity covers
// workspace scope, search policy, and implementation semantics.
type GlobSearcher interface {
	Identity() agent.CapabilityIdentity
	Glob(context.Context, GlobRequest) (SearchResult, error)
}

// GrepSearcher is the reusable workspace text-search seam. Identity covers
// workspace scope, search policy, and implementation semantics. The interface
// is deliberately separate from GlobSearcher because command compilation and
// logical result pagination are grep-specific responsibilities.
type GrepSearcher interface {
	Identity() agent.CapabilityIdentity
	Grep(context.Context, GrepRequest) (SearchResult, error)
}

// Searcher composes both search capabilities for hosts that expose one
// workspace adapter. Tool factories depend on the narrower interface above.
type Searcher interface {
	GlobSearcher
	GrepSearcher
}

type globInput struct {
	Paths     []string `json:"paths,omitempty" jsonschema:"minItems=1" jsonschema_description:"Workspace-relative files, directories, or glob patterns. Omit to discover from the workspace root."`
	Hidden    *bool    `json:"hidden,omitempty" jsonschema_description:"Include dot-prefixed paths; defaults to true."`
	Gitignore *bool    `json:"gitignore,omitempty" jsonschema_description:"Respect .gitignore rules; defaults to true."`
	Limit     int      `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum returned paths; defaults to the workspace search policy."`
	Cursor    string   `json:"cursor,omitempty" jsonschema_description:"Opaque continuation cursor returned by an earlier identical glob call."`
}

// Glob defines multi-path workspace discovery. Directory reading remains the
// responsibility of read; glob only answers path-pattern questions.
func Glob(searcher GlobSearcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("glob GlobSearcher is nil")
	}
	if err := validateAdapterIdentity("glob Searcher", searcher.Identity()); err != nil {
		return agent.ToolDefinition{}, err
	}
	descriptor := readDescriptor(options...)
	descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRerun
	tool, err := agent.InferTool("glob", `Find workspace files or directories by path. A call may include several files, directories, or glob patterns; results are de-duplicated and bounded. Use read on a directory when you need its structure.

按路径查找工作区文件或目录。一次可传入多个文件、目录或 glob 模式；结果会去重并受限。需要理解目录结构时请使用 read。`, func(ctx context.Context, input globInput) (agent.ToolResult, error) {
		request := normalizeGlobRequest(GlobRequest{
			Paths: input.Paths, Hidden: boolDefault(input.Hidden, true),
			Gitignore: boolDefault(input.Gitignore, true), Limit: input.Limit, Cursor: input.Cursor,
		})
		result, err := searcher.Glob(ctx, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return searchToolResult("glob", result, descriptor.MaxResultBytes, func(returned int) (string, error) {
			if returned <= 0 || returned > len(result.Entries) {
				return "", nil
			}
			return encodeGlobCursor(result.Entries[returned-1], request)
		})
	})
	return agent.ToolDefinition{
		Tool: tool, Descriptor: descriptor,
		ImplementationIdentity: toolsetIdentity("tools.glob", searcher.Identity()),
	}, err
}

type grepInput struct {
	Command string `json:"command" jsonschema:"minLength=3,maxLength=65536" jsonschema_description:"One literal ripgrep command or a pipeline containing only literal rg stages. This is not a shell command: only | between rg stages is supported; redirects, substitutions, and external paths are rejected. The first stage supports native PATTERN [PATH ...] and repeated -e/--regexp syntax. Later stages filter the preceding stdout and cannot specify workspace paths. Example: rg -n -e TODO -e FIXME agent internal | rg -i 'urgent|blocking'."`
	Cursor  string `json:"cursor,omitempty" jsonschema:"maxLength=8192" jsonschema_description:"Opaque next_cursor returned by the same normalized grep command."`
}

// Grep defines bounded workspace text search using a controlled rg command.
func Grep(searcher GrepSearcher, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if searcher == nil {
		return agent.ToolDefinition{}, errors.New("grep GrepSearcher is nil")
	}
	if err := validateAdapterIdentity("grep Searcher", searcher.Identity()); err != nil {
		return agent.ToolDefinition{}, err
	}
	descriptor := readDescriptor(options...)
	descriptor.ResultRecoveryKind = agent.ToolResultRecoveryRerun
	tool, err := agent.InferTool("grep", `Search workspace text with one controlled rg command or an rg-only pipeline. Commands never run through a shell. Safe ripgrep search flags and native PATTERN [PATH ...] or repeated -e/--regexp syntax are supported. The first stage searches workspace-relative paths; each later rg stage filters only the preceding stdout. Redirects, substitutions, non-rg pipeline stages, external configuration, process-spawning flags, and paths outside the workspace are rejected. Results are deterministic and bounded; repeat the identical command with next_cursor to continue after a partial result.

使用一条受控的 rg 命令或仅由 rg 组成的管道搜索工作区文本，命令绝不会经过 shell。支持安全的 ripgrep 参数，以及原生 PATTERN [PATH ...] 和重复 -e/--regexp 语法。第一段搜索工作区相对路径，后续每段 rg 只能过滤上一段的标准输出。重定向、替换、非 rg 管道段、外部配置、可启动进程的参数和工作区外路径都会被拒绝。结果稳定且有界；结果为 partial 时，用相同 command 携带 next_cursor 继续。`, func(ctx context.Context, input grepInput) (agent.ToolResult, error) {
		request := normalizeGrepRequest(GrepRequest{Command: input.Command, Cursor: input.Cursor})
		result, err := searcher.Grep(ctx, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		state, err := decodeGrepCursor(request.Cursor, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return searchToolResult("grep", result, descriptor.MaxResultBytes, func(returned int) (string, error) {
			next := grepCursorState{
				Offset: state.Offset + returned,
				Prefix: advanceGrepPrefix(state.Prefix, result.Entries[:returned]),
			}
			return encodeGrepCursor(next, request)
		})
	})
	return agent.ToolDefinition{
		Tool: tool, Descriptor: descriptor,
		ImplementationIdentity: toolsetIdentity("tools.grep", searcher.Identity()),
	}, err
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
				Suggestion: "Use next_cursor when present, otherwise narrow the search scope. / 如有 next_cursor 请继续分页，否则缩小搜索范围。",
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
	Prefix      string `json:"p,omitempty"`
}

type grepCursorState struct {
	Offset int
	Prefix string
}

type globCursor struct {
	Version     int    `json:"v"`
	After       string `json:"after"`
	Fingerprint string `json:"f"`
}

func encodeGlobCursor(after string, request GlobRequest) (string, error) {
	cursor := globCursor{Version: 1, After: after, Fingerprint: globRequestFingerprint(request)}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeGlobCursor(value string, request GlobRequest) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", errors.New("glob cursor is invalid / glob 游标无效")
	}
	var cursor globCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || strings.TrimSpace(cursor.After) == "" {
		return "", errors.New("glob cursor is invalid / glob 游标无效")
	}
	if cursor.Fingerprint != globRequestFingerprint(request) {
		return "", errors.New("glob cursor does not belong to this query / glob 游标不属于当前查询")
	}
	return cursor.After, nil
}

func globRequestFingerprint(request GlobRequest) string {
	request.Cursor = ""
	request.Limit = 0
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

func encodeGrepCursor(state grepCursorState, request GrepRequest) (string, error) {
	cursor := grepCursor{
		Version: 2, Offset: state.Offset, Prefix: state.Prefix,
		Fingerprint: grepRequestFingerprint(request),
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeGrepCursor(value string, request GrepRequest) (grepCursorState, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return grepCursorState{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return grepCursorState{}, errors.New("grep cursor is invalid / grep 游标无效")
	}
	var cursor grepCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 2 || cursor.Offset < 0 ||
		(cursor.Offset == 0 && cursor.Prefix != "") || (cursor.Offset > 0 && cursor.Prefix == "") {
		return grepCursorState{}, errors.New("grep cursor is invalid / grep 游标无效")
	}
	if cursor.Fingerprint != grepRequestFingerprint(request) {
		return grepCursorState{}, errors.New("grep cursor does not belong to this query / grep 游标不属于当前查询")
	}
	return grepCursorState{Offset: cursor.Offset, Prefix: cursor.Prefix}, nil
}

func grepRequestFingerprint(request GrepRequest) string {
	request.Cursor = ""
	encoded, _ := json.Marshal(struct {
		Policy  int         `json:"policy"`
		Request GrepRequest `json:"request"`
	}{Policy: grepCommandPolicyVersion, Request: request})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

func advanceGrepPrefix(prefix string, entries []string) string {
	for _, entry := range entries {
		hash := sha256.New()
		_, _ = fmt.Fprintf(hash, "denova-grep-prefix-v1\x00%s\x00%d:", prefix, len(entry))
		_, _ = hash.Write([]byte(entry))
		prefix = hex.EncodeToString(hash.Sum(nil)[:16])
	}
	return prefix
}

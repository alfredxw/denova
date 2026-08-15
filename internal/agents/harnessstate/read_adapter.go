package harnessstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
	agenttools "github.com/alfredxw/denova/agent/tools"
)

const (
	harnessStateScheme     = "harness"
	harnessStateHost       = "state"
	defaultStateReadLines  = 2000
	maximumStateReadLines  = 10000
	harnessReadAdapterName = "harness_state"
)

type harnessStateReadInput struct {
	Path       string `json:"path" jsonschema_description:"Harness State URI: harness://state/current for the manifest or harness://state/<relative-path> for a file."`
	Offset     int    `json:"offset,omitempty" jsonschema:"minimum=1" jsonschema_description:"One-based first line; defaults to 1."`
	ByteOffset int    `json:"byte_offset,omitempty" jsonschema:"minimum=0" jsonschema_description:"Zero-based UTF-8 byte offset within the selected first line for exact continuation."`
	Limit      int    `json:"limit,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum selected lines; defaults to 2000 and cannot exceed 10000."`
}

type harnessStateManifest struct {
	Schema      string                  `json:"schema"`
	Revision    string                  `json:"revision"`
	Files       []stateFileInfo         `json:"files"`
	ScriptTools []ScriptToolMetadata    `json:"script_tools,omitempty"`
	Diagnostics []agentstate.Diagnostic `json:"diagnostics,omitempty"`
}

type stateFileInfo struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// NewReadAdapter exposes the current Harness State through the ordinary read
// tool without leaking the State store into application orchestration.
func NewReadAdapter(manager *Manager) (agenttools.ReadAdapter, error) {
	if manager == nil || manager.Store() == nil {
		return nil, errors.New("Harness State manager is unavailable")
	}
	digest := sha256.Sum256([]byte(manager.Root()))
	return agenttools.NewReadAdapter(
		agent.CapabilityIdentity{
			Kind: "denova.harness_state.read", Version: 1,
			ConfigHash: hex.EncodeToString(digest[:]),
		},
		harnessReadAdapterName,
		func(ctx context.Context, resourcePath string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			parsed, err := url.Parse(strings.TrimSpace(resourcePath))
			if err != nil || !strings.EqualFold(parsed.Scheme, harnessStateScheme) {
				return false, nil
			}
			return strings.EqualFold(parsed.Host, harnessStateHost), nil
		},
		func(ctx context.Context, input harnessStateReadInput) (agenttools.ReadResult, error) {
			return readHarnessState(ctx, manager, input)
		},
	)
}

func readHarnessState(ctx context.Context, manager *Manager, input harnessStateReadInput) (agenttools.ReadResult, error) {
	resource, err := parseHarnessStateURI(input.Path)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	inspection, err := manager.Inspect(ctx)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	if resource.path == "" {
		files := inspection.Snapshot.Files()
		manifest := harnessStateManifest{
			Schema: "harness.state.v1", Revision: inspection.Snapshot.Revision,
			Files: make([]stateFileInfo, len(files)), ScriptTools: inspection.Harness.ScriptToolMetadata(),
			Diagnostics: inspection.Diagnostics,
		}
		for index, file := range files {
			manifest.Files[index] = stateFileInfo{Path: file.Path, Bytes: len(file.Content)}
		}
		sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return agenttools.ReadResult{}, fmt.Errorf("encode Harness State manifest: %w", err)
		}
		return agenttools.ReadResult{Path: "harness://state/current", Kind: harnessReadAdapterName, Content: string(encoded)}, nil
	}
	content, err := inspection.Snapshot.Read(resource.path)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	if !utf8.Valid(content) {
		return agenttools.ReadResult{}, fmt.Errorf("Harness State file %s is not valid UTF-8", resource.path)
	}
	window, err := stateTextWindow(string(content), input.Offset, input.ByteOffset, input.Limit)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	window.Path = "harness://state/" + resource.path
	window.Kind = harnessReadAdapterName
	return window, nil
}

type harnessStateResource struct {
	path string
}

func parseHarnessStateURI(value string) (harnessStateResource, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !strings.EqualFold(parsed.Scheme, harnessStateScheme) || !strings.EqualFold(parsed.Host, harnessStateHost) {
		return harnessStateResource{}, errors.New("Harness State path must use harness://state/<relative-path>")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return harnessStateResource{}, errors.New("Harness State URI must not contain a query or fragment")
	}
	escapedPath := strings.Trim(parsed.EscapedPath(), "/")
	if escapedPath == "" {
		return harnessStateResource{}, errors.New("Harness State URI requires current or a relative file path")
	}
	if escapedPath == "current" {
		return harnessStateResource{}, nil
	}
	resource := harnessStateResource{}
	for _, part := range strings.Split(escapedPath, "/") {
		value, err := url.PathUnescape(part)
		if err != nil || value == "" || value == "." || value == ".." || strings.Contains(value, "\\") {
			return harnessStateResource{}, errors.New("Harness State file path is invalid")
		}
		resource.path += value + "/"
	}
	resource.path = strings.TrimSuffix(resource.path, "/")
	return resource, nil
}

func stateTextWindow(content string, offset, byteOffset, limit int) (agenttools.ReadResult, error) {
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultStateReadLines
	}
	if limit > maximumStateReadLines {
		return agenttools.ReadResult{}, fmt.Errorf("Harness State read limit cannot exceed %d", maximumStateReadLines)
	}
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if offset > len(lines) {
		return agenttools.ReadResult{Offset: offset, ByteOffset: byteOffset, Total: len(lines)}, nil
	}
	start := offset - 1
	end := min(len(lines), start+limit)
	selected := append([]string(nil), lines[start:end]...)
	if byteOffset > 0 {
		if byteOffset > len(selected[0]) || !utf8.ValidString(selected[0][byteOffset:]) {
			return agenttools.ReadResult{}, errors.New("Harness State byte_offset is not a valid UTF-8 boundary in the selected line")
		}
		selected[0] = selected[0][byteOffset:]
	}
	truncated := end < len(lines)
	nextOffset := 0
	if truncated {
		nextOffset = end + 1
	}
	return agenttools.ReadResult{
		Content: strings.Join(selected, ""), Offset: offset, ByteOffset: byteOffset,
		Limit: len(selected), Total: len(lines), Truncated: truncated, NextOffset: nextOffset,
	}, nil
}

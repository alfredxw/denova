package trajectory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	agentrun "denova/internal/agents/run"

	agenttools "github.com/alfredxw/denova/agent/tools"
)

const runResourceSchema = "denova.trajectory.run.v2"

type runResourceManifest struct {
	Schema      string                   `json:"schema"`
	Type        string                   `json:"type"`
	Project     Source                   `json:"project"`
	Summary     agentrun.RunTraceSummary `json:"summary"`
	RecordCount int                      `json:"record_count"`
}

type llmInputProjectionState struct {
	CallID       string
	InputHash    string
	MessagesHash string
	SystemHash   string
	ToolsHash    string
	Messages     []json.RawMessage
	Ready        bool
}

type llmInputComponentState struct {
	State  string `json:"state"`
	Count  int    `json:"count"`
	SHA256 string `json:"sha256"`
}

type llmInputProjection struct {
	Schema             string                 `json:"schema"`
	Mode               string                 `json:"mode"`
	CallID             string                 `json:"call_id,omitempty"`
	BaseCallID         string                 `json:"base_call_id,omitempty"`
	BaseInputSHA256    string                 `json:"base_input_sha256,omitempty"`
	BaseMessagesSHA256 string                 `json:"base_messages_sha256,omitempty"`
	BaseMessageCount   int                    `json:"base_message_count,omitempty"`
	MessageCount       int                    `json:"message_count"`
	MessagesSHA256     string                 `json:"messages_sha256"`
	Messages           []any                  `json:"messages,omitempty"`
	AddedMessages      []any                  `json:"added_messages,omitempty"`
	System             llmInputComponentState `json:"system"`
	Tools              llmInputComponentState `json:"tools_state"`
	ToolDefinitions    []any                  `json:"tools,omitempty"`
	InputSHA256        string                 `json:"input_sha256"`
	ResetReasons       []string               `json:"reset_reasons,omitempty"`
	Metadata           map[string]any         `json:"metadata"`
}

func (catalog Catalog) readRunResource(ctx context.Context, resource string, source Source, runID string, input readInput, limit int) (agenttools.ReadResult, error) {
	offset := input.Offset
	if offset <= 0 {
		offset = 1
	}
	selectedRecords := make([]string, 0, limit)
	projectionState := llmInputProjectionState{}
	summary, recordCount, err := agentrun.ScanRunTrace(
		agentrun.TraceLocation{Workspace: source.Workspace, StateRoot: source.StateRoot},
		runID,
		func(index int, record agentrun.RunTraceRecord) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			redacted, err := redactRunTraceRecord(record, source)
			if err != nil {
				return err
			}
			if err := projectLLMInput(&redacted, &projectionState); err != nil {
				return err
			}
			lineNumber := index + 2
			if lineNumber < offset || lineNumber >= offset+limit {
				return nil
			}
			line, err := marshalJSONLine(redacted)
			if err != nil {
				return err
			}
			selectedRecords = append(selectedRecords, line)
			return nil
		},
	)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	summary.Path = ""
	lines := make([]string, 0, min(limit, recordCount+1))
	if offset == 1 {
		manifest := runResourceManifest{
			Schema:      runResourceSchema,
			Type:        "run_summary",
			Project:     source,
			Summary:     summary,
			RecordCount: recordCount,
		}
		line, err := marshalRedactedJSONLine(manifest, source)
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		lines = append(lines, line)
	}
	total := recordCount + 1
	lines = append(lines, selectedRecords...)
	if len(lines) > 0 && input.ByteOffset > 0 {
		if input.ByteOffset > len(lines[0]) || !utf8.ValidString(lines[0][input.ByteOffset:]) {
			return agenttools.ReadResult{}, errors.New("trajectory byte_offset is not a valid UTF-8 boundary in the selected line")
		}
		lines[0] = lines[0][input.ByteOffset:]
	}
	truncated := offset-1+len(lines) < total
	nextOffset := 0
	if truncated {
		nextOffset = offset + len(lines)
	}
	return agenttools.ReadResult{
		Path: resource, Kind: "trajectory_run", Content: strings.Join(lines, ""),
		Offset: offset, ByteOffset: input.ByteOffset, Limit: len(lines), Total: total, Unit: "lines",
		Truncated: truncated, NextOffset: nextOffset,
	}, nil
}

func redactRunTraceRecord(record agentrun.RunTraceRecord, source Source) (agentrun.RunTraceRecord, error) {
	if record.Data == nil {
		return record, nil
	}
	redacted, ok := redactTrajectoryValue(record.Data, source).(map[string]any)
	if !ok {
		return agentrun.RunTraceRecord{}, errors.New("redacted run trace data is not an object")
	}
	record.Data = redacted
	return record, nil
}

func projectLLMInput(record *agentrun.RunTraceRecord, state *llmInputProjectionState) error {
	if record == nil || state == nil || record.Type != "llm_input" {
		return nil
	}
	content, ok := record.Data["content"].(map[string]any)
	if !ok {
		*state = llmInputProjectionState{}
		return nil
	}
	messages, ok := jsonSequence(content, "messages")
	if !ok {
		*state = llmInputProjectionState{}
		return nil
	}
	tools, ok := jsonSequence(content, "tools")
	if !ok {
		*state = llmInputProjectionState{}
		return nil
	}
	messageItems, err := canonicalItems(messages)
	if err != nil {
		return fmt.Errorf("canonicalize llm_input messages: %w", err)
	}
	systemMessages := make([]any, 0)
	for _, message := range messages {
		if isSystemMessage(message) {
			systemMessages = append(systemMessages, message)
		}
	}
	inputHash, err := hashJSON(content)
	if err != nil {
		return fmt.Errorf("hash llm_input content: %w", err)
	}
	messagesHash, err := hashJSON(messages)
	if err != nil {
		return fmt.Errorf("hash llm_input messages: %w", err)
	}
	systemHash, err := hashJSON(systemMessages)
	if err != nil {
		return fmt.Errorf("hash llm_input system messages: %w", err)
	}
	toolsHash, err := hashJSON(tools)
	if err != nil {
		return fmt.Errorf("hash llm_input tools: %w", err)
	}

	callID, _ := record.Data["call_id"].(string)
	metadata := make(map[string]any, len(content))
	for key, value := range content {
		if key != "messages" && key != "tools" {
			metadata[key] = value
		}
	}
	projection := llmInputProjection{
		Schema:         "denova.trajectory.llm_input.v1",
		Mode:           "snapshot",
		CallID:         strings.TrimSpace(callID),
		MessageCount:   len(messages),
		MessagesSHA256: messagesHash,
		System:         llmInputComponentState{State: "included", Count: len(systemMessages), SHA256: systemHash},
		Tools:          llmInputComponentState{State: "included", Count: len(tools), SHA256: toolsHash},
		InputSHA256:    inputHash,
		Metadata:       metadata,
	}
	messagesAppendOnly := hasMessagePrefix(state.Messages, messageItems)
	appendOnly := state.Ready && state.SystemHash == systemHash && state.ToolsHash == toolsHash && messagesAppendOnly
	if appendOnly {
		projection.Mode = "append"
		projection.BaseCallID = state.CallID
		projection.BaseInputSHA256 = state.InputHash
		projection.BaseMessagesSHA256 = state.MessagesHash
		projection.BaseMessageCount = len(state.Messages)
		projection.AddedMessages = messages[len(state.Messages):]
		projection.System.State = "unchanged"
		projection.Tools.State = "unchanged"
	} else {
		projection.Messages = messages
		projection.ToolDefinitions = tools
		if state.Ready {
			projection.Mode = "reset"
			if state.SystemHash != systemHash {
				projection.ResetReasons = append(projection.ResetReasons, "system_changed")
			}
			if state.ToolsHash != toolsHash {
				projection.ResetReasons = append(projection.ResetReasons, "tools_changed")
			}
			if !messagesAppendOnly {
				projection.ResetReasons = append(projection.ResetReasons, "messages_not_append_only")
			}
		}
	}
	record.Data["content"] = projection
	*state = llmInputProjectionState{
		CallID: strings.TrimSpace(callID), InputHash: inputHash, MessagesHash: messagesHash,
		SystemHash: systemHash, ToolsHash: toolsHash, Messages: messageItems, Ready: true,
	}
	return nil
}

func jsonSequence(content map[string]any, key string) ([]any, bool) {
	value, exists := content[key]
	if !exists {
		return nil, false
	}
	sequence, ok := value.([]any)
	return sequence, ok
}

func canonicalItems(items []any) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(items))
	for index, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		result[index] = encoded
	}
	return result, nil
}

func hasMessagePrefix(previous, current []json.RawMessage) bool {
	if len(previous) > len(current) {
		return false
	}
	for index := range previous {
		if !bytes.Equal(previous[index], current[index]) {
			return false
		}
	}
	return true
}

func isSystemMessage(value any) bool {
	message, ok := value.(map[string]any)
	if !ok {
		return false
	}
	role, _ := message["role"].(string)
	return role == "system" || role == "developer"
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func marshalJSONLine(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

func marshalRedactedJSONLine(value any, source Source) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return "", err
	}
	return marshalJSONLine(redactTrajectoryValue(document, source))
}

package agentui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	appsvc "denova/internal/app"
)

// StreamEncoder writes Agent events using the AI SDK UI message stream
// protocol. It only translates display transport; model context remains owned
// by the existing Go Agent runtime.
type StreamEncoder struct {
	w         io.Writer
	requestID string

	started  bool
	finished bool

	textID        string
	textSeq       int
	reasonID      string
	reasonSeq     int
	toolSeq       int
	toolInputs    map[string]string
	startedTool   map[string]string
	availableTool map[string]bool
}

func NewStreamEncoder(w io.Writer, requestID string) *StreamEncoder {
	return &StreamEncoder{
		w:             w,
		requestID:     strings.TrimSpace(requestID),
		toolInputs:    make(map[string]string),
		startedTool:   make(map[string]string),
		availableTool: make(map[string]bool),
	}
}

func (e *StreamEncoder) WriteEvent(ev appsvc.AgentEvent) error {
	if e.finished {
		return nil
	}
	if err := e.ensureStarted(ev); err != nil {
		return err
	}
	data := eventDataMap(ev.Data)
	meta := providerMetadataFromData(data)

	switch ev.Type {
	case "chunk":
		if err := e.closeReasoning(); err != nil {
			return err
		}
		return e.writeTextDelta(readString(data, "content"), meta, readString(data, "display_segment_id"))
	case "thinking":
		if err := e.closeText(); err != nil {
			return err
		}
		return e.writeReasoningDelta(readString(data, "content"), meta, readString(data, "display_segment_id"))
	case "tool_call":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeToolCall(data, meta)
	case "tool_args_delta":
		return e.writeToolArgsDelta(data)
	case "tool_started":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeToolStarted(data, meta)
	case "tool_result":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		if err := e.writeToolResult(data, meta); err != nil {
			return err
		}
		if data["interactive_image"] != nil || data["interactive_image_error"] != nil {
			return e.writeData(DataTypeInteractiveImage, eventID(data, "interactive-image"), data)
		}
		return nil
	case "ask_pending", "ask_resolved":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeAsk, eventID(data, "ask"), data)
	case "workspace_change":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeWorkspaceChange, eventID(data, "workspace-change"), data)
	case "context_compaction":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeContextCompaction, eventID(data, "context-compaction"), data)
	case "interactive_image":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeInteractiveImage, eventID(data, "interactive-image"), data)
	case "proposed_plan":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeProposedPlan, eventID(data, "proposed-plan"), data)
	case "rule_roll":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeData(DataTypeRuleRoll, eventID(data, "rule-roll"), data)
	case "token_usage":
		return e.writeData(DataTypeTokenUsage, eventID(data, "token-usage"), data)
	case "error":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		message := firstNonEmpty(readString(data, "message"), readString(data, "error"), "Agent request failed")
		return e.writeChunk(map[string]any{"type": "error", "errorText": CorrelatedErrorMessage(message, e.requestID)})
	case "aborted":
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		return e.writeChunk(map[string]any{"type": "abort", "reason": firstNonEmpty(readString(data, "message"), "user cancelled")})
	case "done":
		return e.Finish("stop")
	default:
		if err := e.closeOpenContent(); err != nil {
			return err
		}
		payload := cloneMap(data)
		payload["event"] = ev.Type
		return e.writeData(DataTypeActivity, eventID(data, ev.Type), payload)
	}
}

// CorrelatedErrorMessage appends the server-issued request ID to a user-visible
// streaming error. The bilingual label keeps the transport usable before the
// client-side locale is available.
func CorrelatedErrorMessage(message, requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return message
	}
	return message + " · 日志 ID / Log ID: " + requestID
}

func (e *StreamEncoder) Finish(reason string) error {
	if e.finished {
		return nil
	}
	if err := e.closeOpenContent(); err != nil {
		return err
	}
	if err := e.settlePendingTools(); err != nil {
		return err
	}
	if err := e.writeChunk(map[string]any{"type": "finish", "finishReason": firstNonEmpty(reason, "stop")}); err != nil {
		return err
	}
	if _, err := fmt.Fprint(e.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	e.finished = true
	return nil
}

func (e *StreamEncoder) ensureStarted(ev appsvc.AgentEvent) error {
	if e.started {
		return nil
	}
	data := eventDataMap(ev.Data)
	start := map[string]any{
		"type":      "start",
		"messageId": eventID(data, "assistant"),
	}
	if metadata := messageMetadataFromData(data); len(metadata) > 0 {
		start["messageMetadata"] = metadata
	}
	e.started = true
	return e.writeChunk(start)
}

func (e *StreamEncoder) writeTextDelta(delta string, providerMetadata map[string]any, segmentID string) error {
	if delta == "" {
		return nil
	}
	if e.textID != "" && segmentID != "" && e.textID != segmentID {
		if err := e.closeText(); err != nil {
			return err
		}
	}
	if e.textID == "" {
		if segmentID != "" {
			e.textID = segmentID
		} else {
			e.textSeq++
			e.textID = fmt.Sprintf("text-%d", e.textSeq)
		}
		start := map[string]any{"type": "text-start", "id": e.textID}
		if len(providerMetadata) > 0 {
			start["providerMetadata"] = providerMetadata
		}
		if err := e.writeChunk(start); err != nil {
			return err
		}
	}
	chunk := map[string]any{"type": "text-delta", "id": e.textID, "delta": delta}
	if len(providerMetadata) > 0 {
		chunk["providerMetadata"] = providerMetadata
	}
	return e.writeChunk(chunk)
}

func (e *StreamEncoder) writeReasoningDelta(delta string, providerMetadata map[string]any, segmentID string) error {
	if delta == "" {
		return nil
	}
	if e.reasonID != "" && segmentID != "" && e.reasonID != segmentID {
		if err := e.closeReasoning(); err != nil {
			return err
		}
	}
	if e.reasonID == "" {
		if segmentID != "" {
			e.reasonID = segmentID
		} else {
			e.reasonSeq++
			e.reasonID = fmt.Sprintf("reasoning-%d", e.reasonSeq)
		}
		start := map[string]any{"type": "reasoning-start", "id": e.reasonID}
		if len(providerMetadata) > 0 {
			start["providerMetadata"] = providerMetadata
		}
		if err := e.writeChunk(start); err != nil {
			return err
		}
	}
	chunk := map[string]any{"type": "reasoning-delta", "id": e.reasonID, "delta": delta}
	if len(providerMetadata) > 0 {
		chunk["providerMetadata"] = providerMetadata
	}
	return e.writeChunk(chunk)
}

func (e *StreamEncoder) closeOpenContent() error {
	if err := e.closeText(); err != nil {
		return err
	}
	return e.closeReasoning()
}

func (e *StreamEncoder) closeText() error {
	if e.textID == "" {
		return nil
	}
	id := e.textID
	e.textID = ""
	return e.writeChunk(map[string]any{"type": "text-end", "id": id})
}

func (e *StreamEncoder) closeReasoning() error {
	if e.reasonID == "" {
		return nil
	}
	id := e.reasonID
	e.reasonID = ""
	return e.writeChunk(map[string]any{"type": "reasoning-end", "id": id})
}

func (e *StreamEncoder) writeToolCall(data map[string]any, providerMetadata map[string]any) error {
	toolID := toolCallID(data, &e.toolSeq)
	toolName := firstNonEmpty(readString(data, "name"), "unknown_tool")
	if e.startedTool[toolID] == "" {
		chunk := map[string]any{
			"type":       "tool-input-start",
			"toolCallId": toolID,
			"toolName":   toolName,
			"dynamic":    true,
		}
		if len(providerMetadata) > 0 {
			chunk["providerMetadata"] = providerMetadata
		}
		if err := e.writeChunk(chunk); err != nil {
			return err
		}
		e.startedTool[toolID] = toolName
	}
	args := readString(data, "args")
	if args == "" {
		return nil
	}
	e.toolInputs[toolID] += args
	return e.writeChunk(map[string]any{
		"type":           "tool-input-delta",
		"toolCallId":     toolID,
		"inputTextDelta": args,
	})
}

func (e *StreamEncoder) writeToolArgsDelta(data map[string]any) error {
	toolID := toolCallID(data, &e.toolSeq)
	delta := readString(data, "delta")
	if delta == "" {
		return nil
	}
	e.toolInputs[toolID] += delta
	return e.writeChunk(map[string]any{
		"type":           "tool-input-delta",
		"toolCallId":     toolID,
		"inputTextDelta": delta,
	})
}

func (e *StreamEncoder) writeToolStarted(data map[string]any, providerMetadata map[string]any) error {
	toolID := toolCallID(data, &e.toolSeq)
	toolName := firstNonEmpty(e.startedTool[toolID], readString(data, "name"), "unknown_tool")
	if e.startedTool[toolID] == "" {
		if err := e.writeToolCall(map[string]any{
			"id": toolID, "name": toolName,
		}, providerMetadata); err != nil {
			return err
		}
	}
	return e.writeToolInputAvailable(toolID, toolName, providerMetadata)
}

func (e *StreamEncoder) writeToolResult(data map[string]any, providerMetadata map[string]any) error {
	toolID := toolCallID(data, &e.toolSeq)
	toolName := firstNonEmpty(e.startedTool[toolID], readString(data, "name"), "unknown_tool")
	if e.startedTool[toolID] == "" {
		if err := e.writeToolCall(map[string]any{
			"id":   toolID,
			"name": toolName,
		}, providerMetadata); err != nil {
			return err
		}
	}
	if err := e.writeToolInputAvailable(toolID, toolName, providerMetadata); err != nil {
		return err
	}
	output := readString(data, "content")
	status := strings.ToLower(strings.TrimSpace(readString(data, "status")))
	chunk := map[string]any{
		"toolCallId": toolID,
		"dynamic":    true,
	}
	switch status {
	case "", "success":
		chunk["type"] = "tool-output-available"
		chunk["output"] = output
	case "error", "blocked", "skipped":
		chunk["type"] = "tool-output-error"
		chunk["errorText"] = firstNonEmpty(
			output,
			readString(data, "synthetic_reason"),
			"Tool execution did not complete / 工具执行未完成",
		)
	default:
		chunk["type"] = "tool-output-error"
		chunk["errorText"] = firstNonEmpty(
			output,
			"Unknown tool result status: "+status+" / 未知工具结果状态："+status,
		)
	}
	if len(providerMetadata) > 0 {
		chunk["providerMetadata"] = providerMetadata
	}
	if illustration, ok := data["illustration"]; ok && illustration != nil {
		chunk["toolMetadata"] = map[string]any{"illustration": illustration}
	}
	delete(e.toolInputs, toolID)
	delete(e.startedTool, toolID)
	delete(e.availableTool, toolID)
	return e.writeChunk(chunk)
}

func (e *StreamEncoder) writeToolInputAvailable(toolID, toolName string, providerMetadata map[string]any) error {
	if e.availableTool[toolID] {
		return nil
	}
	inputRaw := e.toolInputs[toolID]
	chunk := map[string]any{
		"type":       "tool-input-available",
		"toolCallId": toolID,
		"toolName":   toolName,
		"input":      parseJSONValue(inputRaw),
		"dynamic":    true,
	}
	if len(providerMetadata) > 0 {
		chunk["providerMetadata"] = providerMetadata
	}
	if err := e.writeChunk(chunk); err != nil {
		return err
	}
	e.availableTool[toolID] = true
	return nil
}

func (e *StreamEncoder) settlePendingTools() error {
	for toolID, toolName := range e.startedTool {
		if err := e.writeToolInputAvailable(toolID, firstNonEmpty(toolName, "unknown_tool"), nil); err != nil {
			return err
		}
		if err := e.writeChunk(map[string]any{
			"type":       "tool-output-error",
			"toolCallId": toolID,
			"errorText":  "Tool stream ended before a result was received / 工具流结束时尚未收到结果",
			"dynamic":    true,
		}); err != nil {
			return err
		}
		delete(e.toolInputs, toolID)
		delete(e.startedTool, toolID)
		delete(e.availableTool, toolID)
	}
	return nil
}

func (e *StreamEncoder) writeData(dataType, id string, data map[string]any) error {
	chunk := map[string]any{
		"type": dataType,
		"id":   id,
		"data": data,
	}
	if providerMetadata := providerMetadataFromData(data); len(providerMetadata) > 0 {
		chunk["providerMetadata"] = providerMetadata
	}
	return e.writeChunk(chunk)
}

func (e *StreamEncoder) writeChunk(chunk map[string]any) error {
	raw, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.w, "data: %s\n\n", raw)
	return err
}

func eventDataMap(data any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	if value, ok := data.(map[string]any); ok {
		return cloneMap(value)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func providerMetadataFromData(data map[string]any) map[string]any {
	meta := messageMetadataFromData(data)
	if segmentID := readString(data, "display_segment_id"); segmentID != "" {
		if meta == nil {
			meta = map[string]any{}
		}
		meta["display_segment_id"] = segmentID
	}
	if len(meta) == 0 {
		return nil
	}
	return map[string]any{"agent": meta}
}

func messageMetadataFromData(data map[string]any) map[string]any {
	keys := []string{
		"created_at",
		"display_role",
		"display_phase",
		"history_type",
		"run_id",
		"agent_kind",
		"agent_name",
		"root_agent_name",
		"run_path",
		"subagent",
		"subagent_session_id",
		"subagent_type",
		"provider_call_id",
		"sse_hidden_fields",
		"sse_hidden_reason",
		"sse_display_notice",
		"sse_generated_chars",
		"turn_id",
		"navigation_turn_id",
		"turn_versions",
		"turn_version_index",
		"tool_presentation",
	}
	meta := map[string]any{}
	for _, key := range keys {
		if value, ok := data[key]; ok && !emptyValue(value) {
			meta[key] = value
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func toolCallID(data map[string]any, seq *int) string {
	if id := readString(data, "id"); id != "" {
		return id
	}
	if index := readString(data, "index"); index != "" {
		return "index:" + index
	}
	if value, ok := data["index"].(float64); ok {
		return fmt.Sprintf("index:%d", int(value))
	}
	*seq++
	return fmt.Sprintf("tool-%d", *seq)
}

func eventID(data map[string]any, fallback string) string {
	if id := readString(data, "id"); id != "" {
		return id
	}
	if runID := readString(data, "run_id"); runID != "" {
		return fallback + "-" + runID
	}
	return fallback
}

func readString(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprint(v)
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func emptyValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case bool:
		return !v
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

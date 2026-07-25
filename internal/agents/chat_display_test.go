package agents

import (
	"context"
	"errors"
	"strings"
	"testing"

	"denova/internal/agents/session"
)

func TestAppendAssistantIfAnyReturnsPersistenceFailure(t *testing.T) {
	wantErr := errors.New("disk full")
	conversation := &failingAssistantConversation{err: wantErr}
	var content strings.Builder
	content.WriteString("不会被误报为成功的正文")
	generated, err := appendAssistantIfAny(conversation, &content, nil, session.MessageMetadata{})
	if !errors.Is(err, wantErr) || generated != "不会被误报为成功的正文" {
		t.Fatalf("persistence failure must reach the run loop: generated=%q err=%v", generated, err)
	}
	if content.Len() == 0 {
		t.Fatal("failed persistence must not clear the only in-memory copy")
	}
}

type failingAssistantConversation struct{ err error }

func (c *failingAssistantConversation) AssembleModelContext(ctx context.Context, _ string, input ModelContextInput) (ModelContextResult, error) {
	return AssembleSingleUserModelContext(ctx, input)
}
func (c *failingAssistantConversation) AppendAssistant(string) error { return c.err }
func (c *failingAssistantConversation) MarkInterrupted(string, string, string) error {
	return nil
}
func (c *failingAssistantConversation) PendingInterruption() *session.Interruption { return nil }
func (c *failingAssistantConversation) ResolveInterruption(string) error           { return nil }

func TestDisplayRecorderKeepsWriteFileContentArgs(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	wantArgs := `{"file_path":"chapters/ch01.md","content":"第一行\n第二行"}`
	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"args":       wantArgs,
	}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	args := appender.events[0].Args
	if args != wantArgs {
		t.Fatalf("display history should keep full write args, got %q", args)
	}
}

func TestDisplayRecorderPersistsReclassifiedInteractiveContentAsThinking(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	recorder.Record(Event{Type: "interactive_content_reclassified", Data: map[string]interface{}{
		"agent_kind": AgentKindInteractiveStory,
		"content":    "我先检查资料，再开始写正文。",
	}})
	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindInteractiveStory,
		"id":         "call-lore",
		"name":       "list_lore_items",
	}})

	if len(appender.events) != 2 {
		t.Fatalf("events = %#v", appender.events)
	}
	if appender.events[0].Role != "thinking" || appender.events[0].Content != "我先检查资料，再开始写正文。" {
		t.Fatalf("reclassified content was not persisted as thinking: %#v", appender.events[0])
	}
}

func TestDisplayRecorderPreservesAlternatingThinkingAndAssistantSegments(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	for _, event := range []Event{
		{Type: "thinking", Data: map[string]interface{}{"run_id": "run-order", "content": "先分析。"}},
		{Type: "chunk", Data: map[string]interface{}{"run_id": "run-order", "content": "第一段"}},
		{Type: "chunk", Data: map[string]interface{}{"run_id": "run-order", "content": "正文。"}},
		{Type: "tool_call", Data: map[string]interface{}{"run_id": "run-order", "id": "call-1", "name": "read_file", "args": `{"path":"outline.md"}`}},
		{Type: "tool_result", Data: map[string]interface{}{"run_id": "run-order", "id": "call-1", "name": "read_file", "content": "读取完成"}},
		{Type: "thinking", Data: map[string]interface{}{"run_id": "run-order", "content": "再检查。"}},
		{Type: "chunk", Data: map[string]interface{}{"run_id": "run-order", "content": "第二段正文。"}},
		{Type: "done", Data: map[string]string{}},
	} {
		recorder.Record(event)
	}

	want := []session.DisplayEvent{
		{Role: "thinking", Content: "先分析。"},
		{Role: "assistant", Content: "第一段正文。"},
		{Role: "tool_call", Content: "read_file"},
		{Role: "thinking", Content: "再检查。"},
		{Role: "assistant", Content: "第二段正文。"},
	}
	if len(appender.events) != len(want) {
		t.Fatalf("display events = %#v, want %#v", appender.events, want)
	}
	for index := range want {
		if appender.events[index].Role != want[index].Role || appender.events[index].Content != want[index].Content {
			t.Fatalf("display event %d = %#v, want %#v", index, appender.events[index], want[index])
		}
	}
	if appender.events[2].Status != "success" {
		t.Fatalf("tool result should update the in-order tool card: %#v", appender.events[2])
	}
}

func TestDisplayRecorderPreservesSubAgentSegmentsAroundThinking(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}
	meta := map[string]interface{}{
		"run_id":              "run-subagent-order",
		"agent_name":          "researcher",
		"root_agent_name":     "DenovaAgent",
		"subagent":            true,
		"subagent_session_id": "run-subagent-order-subagent-01-researcher",
	}
	event := func(eventType, content string) Event {
		data := make(map[string]interface{}, len(meta)+1)
		for key, value := range meta {
			data[key] = value
		}
		data["content"] = content
		return Event{Type: eventType, Data: data}
	}

	for _, item := range []Event{
		event("chunk", "调研结论一。"),
		event("thinking", "继续核对来源。"),
		event("chunk", "调研结论二。"),
		{Type: "done", Data: map[string]string{}},
	} {
		recorder.Record(item)
	}

	wantRoles := []string{"assistant", "thinking", "assistant"}
	wantContent := []string{"调研结论一。", "继续核对来源。", "调研结论二。"}
	if len(appender.events) != len(wantRoles) {
		t.Fatalf("display events = %#v, want roles %v", appender.events, wantRoles)
	}
	for index := range wantRoles {
		got := appender.events[index]
		if got.Role != wantRoles[index] || got.Content != wantContent[index] || !got.SubAgent || got.SubAgentSessionID != meta["subagent_session_id"] {
			t.Fatalf("display event %d = %#v", index, got)
		}
	}
}

func TestDisplayRecorderCanSuppressOnlyRootAssistantSegments(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:                      appender,
		pendingToolIDs:                map[string]string{},
		suppressRootAssistantSegments: true,
	}
	rootChunk := map[string]interface{}{
		"run_id":  "run-plan",
		"content": "这段是计划协议前的说明，不应作为最终正文恢复。",
	}
	subAgentChunk := map[string]interface{}{
		"run_id":              "run-plan",
		"content":             "子 Agent 的可见结果仍需保留。",
		"subagent":            true,
		"subagent_session_id": "run-plan-subagent-01",
	}

	recorder.Record(Event{Type: "chunk", Data: rootChunk})
	recorder.Record(Event{Type: "chunk", Data: subAgentChunk})
	recorder.Record(Event{Type: "done", Data: map[string]string{}})

	if eventDataString(rootChunk, displaySegmentIDEventKey) == "" {
		t.Fatal("suppressed root chunks still need a stable live-stream segment ID")
	}
	if len(appender.events) != 1 || !appender.events[0].SubAgent || appender.events[0].Content != subAgentChunk["content"] {
		t.Fatalf("display events = %#v, want only the sub-agent assistant segment", appender.events)
	}
}

func TestDisplayRecorderAssignsUniqueStableIDsToTextSegments(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{appender: appender, pendingToolIDs: map[string]string{}}

	for _, event := range []Event{
		{Type: "thinking", Data: map[string]interface{}{"run_id": "run-segments", "content": "同样的开头"}},
		{Type: "chunk", Data: map[string]interface{}{"run_id": "run-segments", "content": "正文一"}},
		{Type: "thinking", Data: map[string]interface{}{"run_id": "run-segments", "content": "同样的开头"}},
		{Type: "chunk", Data: map[string]interface{}{"run_id": "run-segments", "content": "正文二"}},
		{Type: "done", Data: map[string]string{}},
	} {
		recorder.Record(event)
	}

	seen := make(map[string]struct{}, len(appender.events))
	for _, event := range appender.events {
		if event.ID == "" {
			t.Fatalf("display segment is missing a stable ID: %#v", event)
		}
		if _, duplicate := seen[event.ID]; duplicate {
			t.Fatalf("display segment ID %q was reused: %#v", event.ID, appender.events)
		}
		seen[event.ID] = struct{}{}
	}
}

func TestDisplayRecorderAnnotatesStreamingDeltasWithTheirSegmentID(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{appender: appender, pendingToolIDs: map[string]string{}}
	first := map[string]interface{}{"run_id": "run-stream-segments", "content": "第一"}
	second := map[string]interface{}{"run_id": "run-stream-segments", "content": "段"}
	reasoning := map[string]interface{}{"run_id": "run-stream-segments", "content": "检查"}

	recorder.Record(Event{Type: "chunk", Data: first})
	recorder.Record(Event{Type: "chunk", Data: second})
	recorder.Record(Event{Type: "thinking", Data: reasoning})

	firstID := eventDataString(first, displaySegmentIDEventKey)
	if firstID == "" || eventDataString(second, displaySegmentIDEventKey) != firstID {
		t.Fatalf("adjacent text deltas must share one segment ID: first=%#v second=%#v", first, second)
	}
	if reasoningID := eventDataString(reasoning, displaySegmentIDEventKey); reasoningID == "" || reasoningID == firstID {
		t.Fatalf("reasoning must start a different segment: text=%q reasoning=%#v", firstID, reasoning)
	}
}

func TestDisplayRecorderAppendsStreamingWriteFileContent(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"args":       "",
	}})
	recorder.Record(Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"delta":      `{"file_path":"chapters/ch02.md","content":"第一行`,
	}})
	recorder.Record(Event{Type: "tool_args_delta", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-1",
		"name":       "write_file",
		"delta":      `\n第二行\n第三行"}`,
	}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	args := appender.events[0].Args
	for _, want := range []string{"chapters/ch02.md", "content", "第一行", "第二行", "第三行"} {
		if !strings.Contains(args, want) {
			t.Fatalf("display history should keep streamed write content %q in args=%q", want, args)
		}
	}
}

func TestDisplayRecorderKeepsNonIDEWriteFileArgs(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	args := `{"file_path":"chapters/ch01.md","content":"第一行\n第二行"}`
	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindConfigManager,
		"id":         "call-1",
		"name":       "write_file",
		"args":       args,
	}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	if appender.events[0].Args != args {
		t.Fatalf("non-IDE args should stay unchanged: %q", appender.events[0].Args)
	}
}

func TestDisplayRecorderKeepsIDEEditFileChapterArgs(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	args := `{"file_path":"chapters/ch01.md","edits":[{"id":"paragraph-1","old_string":"旧段落","new_string":"新段落"}]}`
	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-1",
		"name":       "edit_file",
		"args":       args,
	}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	if appender.events[0].Args != args {
		t.Fatalf("edit_file args should stay unchanged: %q", appender.events[0].Args)
	}
}

func TestDisplayRecorderConvertsPlanProtocolToolCall(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-plan",
		"name":       "plan_questions",
		"args":       `{"questions":[{"id":"scope","question":"确认范围？"}]}`,
		"run_id":     "run-plan-tool",
	}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	if appender.events[0].Role != "plan_question" {
		t.Fatalf("role = %q, want plan_question", appender.events[0].Role)
	}
	if appender.events[0].Name != "" {
		t.Fatalf("plan protocol tool should not persist tool name, got %q", appender.events[0].Name)
	}
	if appender.events[0].Content == "" || !strings.Contains(appender.events[0].Content, `"questions"`) {
		t.Fatalf("plan event should keep question content: %#v", appender.events[0])
	}
}

func TestDisplayRecorderMarksPendingToolsSuccessOnDone(t *testing.T) {
	appender := &displayRecorderTestAppender{}
	recorder := &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: map[string]string{},
	}

	recorder.Record(Event{Type: "tool_call", Data: map[string]interface{}{
		"agent_kind": AgentKindIDE,
		"id":         "call-execute",
		"name":       "execute",
		"args":       `{"command":"pwd"}`,
	}})
	recorder.Record(Event{Type: "done", Data: map[string]string{}})

	if len(appender.events) != 1 {
		t.Fatalf("events = %d, want 1", len(appender.events))
	}
	if appender.events[0].Status != "success" {
		t.Fatalf("pending tool should be marked success on done: %#v", appender.events[0])
	}
	if len(recorder.pendingToolIDs) != 0 {
		t.Fatalf("pending tool ids should be cleared on done: %#v", recorder.pendingToolIDs)
	}
}

type displayRecorderTestAppender struct {
	events []session.DisplayEvent
}

func (a *displayRecorderTestAppender) AppendDisplayEvent(event session.DisplayEvent) error {
	a.events = append(a.events, event)
	return nil
}

func (a *displayRecorderTestAppender) UpdateDisplayToolStatus(id, name, status string) error {
	for i := len(a.events) - 1; i >= 0; i-- {
		if a.events[i].ID == id || (id == "" && a.events[i].Name == name) {
			a.events[i].Status = status
			return nil
		}
	}
	return nil
}

func (a *displayRecorderTestAppender) AppendDisplayToolArgs(id, name, delta string) error {
	for i := len(a.events) - 1; i >= 0; i-- {
		if a.events[i].ID == id {
			a.events[i].Args += delta
			return nil
		}
	}
	return nil
}

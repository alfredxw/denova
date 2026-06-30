package agent

import (
	"strings"
	"testing"

	"nova/internal/session"
)

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

	args := `{"file_path":"chapters/ch01.md","old_string":"旧段落","new_string":"新段落"}`
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

type displayRecorderTestAppender struct {
	events []session.DisplayEvent
}

func (a *displayRecorderTestAppender) AppendDisplayEvent(event session.DisplayEvent) error {
	a.events = append(a.events, event)
	return nil
}

func (a *displayRecorderTestAppender) UpdateDisplayToolStatus(id, name, status string) error {
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

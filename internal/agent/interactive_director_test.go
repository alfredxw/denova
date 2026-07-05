package agent

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/session"
)

func TestInteractiveDirectorDisplayHidesDirectorPlanWriteInput(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := &singleInstructionConversation{
		display:               display,
		hideDirectorToolInput: true,
		directorTools:         map[string]*directorToolDisplayState{},
	}

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID:     "call-1",
		Role:   "tool_call",
		Name:   "write_file",
		Args:   `{"file_path":"/tmp/work/.denova/interactive/stories/story-1/director/main/director.md","content":"一二三"}`,
		Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	got := display.latest()
	if got.AgentKind != config.AgentKindInteractiveDirector {
		t.Fatalf("agent kind = %q, want interactive director", got.AgentKind)
	}
	if got.Args != `{"file_path":"director.md"}` {
		t.Fatalf("director write args should hide content, got %q", got.Args)
	}
	if got.SSEDisplayNotice != directorPlanHiddenNotice || got.SSEHiddenReason != directorPlanHiddenReason {
		t.Fatalf("hidden metadata mismatch: %#v", got)
	}
	if got.SSEGeneratedChars != 3 {
		t.Fatalf("generated chars = %d, want 3", got.SSEGeneratedChars)
	}
}

func TestInteractiveDirectorDisplayStreamsHiddenDirectorPlanCharCount(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := &singleInstructionConversation{
		display:               display,
		hideDirectorToolInput: true,
		directorTools:         map[string]*directorToolDisplayState{},
	}

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write_file", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "write_file", `{"file_path":"director.md","content":"`+strings.Repeat("字", 101)); err != nil {
		t.Fatal(err)
	}
	running := display.latest()
	if running.Args != `{"file_path":"director.md"}` || strings.Contains(running.Args, "字") {
		t.Fatalf("streaming director args should only expose path, got %q", running.Args)
	}
	if running.SSEGeneratedChars != 101 {
		t.Fatalf("running generated chars = %d, want 101", running.SSEGeneratedChars)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "write_file", `尾"}`); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-1", "write_file", "success", "ok"); err != nil {
		t.Fatal(err)
	}

	done := display.latest()
	if done.Status != "success" || done.Result != "ok" {
		t.Fatalf("final director tool status mismatch: %#v", done)
	}
	if done.SSEGeneratedChars != 102 {
		t.Fatalf("final generated chars = %d, want 102", done.SSEGeneratedChars)
	}
	if strings.Contains(done.Args, "字") || strings.Contains(done.Args, "尾") {
		t.Fatalf("final director args should not expose content, got %q", done.Args)
	}
}

type directorDisplayConversation struct {
	events []session.DisplayEvent
}

func (c *directorDisplayConversation) PrepareMessages(_, _ string) ([]*schema.Message, error) {
	return nil, nil
}
func (c *directorDisplayConversation) AppendAssistant(string) error               { return nil }
func (c *directorDisplayConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c *directorDisplayConversation) PendingInterruption() *session.Interruption { return nil }
func (c *directorDisplayConversation) ResolveInterruption(string) error           { return nil }

func (c *directorDisplayConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	for i := range c.events {
		if c.events[i].Role == event.Role && c.events[i].ID == event.ID && event.ID != "" {
			c.events[i] = event
			return nil
		}
	}
	c.events = append(c.events, event)
	return nil
}

func (c *directorDisplayConversation) AppendDisplayToolArgs(id, _ string, delta string) error {
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].ID == id {
			c.events[i].Args += delta
			return nil
		}
	}
	return nil
}

func (c *directorDisplayConversation) UpdateDisplayToolStatus(id, _ string, status string) error {
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].ID == id {
			c.events[i].Status = status
			return nil
		}
	}
	return nil
}

func (c *directorDisplayConversation) UpdateDisplayToolResult(id, _ string, status, result string) error {
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].ID == id {
			c.events[i].Status = status
			c.events[i].Result = result
			return nil
		}
	}
	return nil
}

func (c *directorDisplayConversation) latest() session.DisplayEvent {
	if len(c.events) == 0 {
		return session.DisplayEvent{}
	}
	return c.events[len(c.events)-1]
}

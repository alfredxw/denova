package interactive

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

func TestInteractiveDirectorDisplayPreservesRawToolInputEvent(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := NewDirectorConversation(DirectorConversationOptions{Display: display})
	raw := `{"path":"/tmp/work/.denova/interactive/stories/story-1/director/main/director.md","content":"一二三"}`

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID:     "call-1",
		Role:   "tool_call",
		Name:   "write",
		Args:   raw,
		Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	got := display.latest()
	if got.AgentKind != config.AgentKindInteractiveDirector {
		t.Fatalf("agent kind = %q, want interactive director", got.AgentKind)
	}
	if got.Args != raw {
		t.Fatalf("director tool args = %q, want exact raw input %q", got.Args, raw)
	}
	if got.Content != "write" {
		t.Fatalf("content = %q, want tool name", got.Content)
	}
}

func TestInteractiveDirectorDisplayStreamsRawToolInputDeltas(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := NewDirectorConversation(DirectorConversationOptions{Display: display})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-1", Role: "tool_call", Name: "edit", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	chunks := []string{
		`not-json:{"path":"director.md","edits":[{"new_`,
		`string":"逐字到达"}]}`,
	}
	for _, chunk := range chunks {
		if err := conversation.AppendDisplayToolArgs("call-1", "edit", chunk); err != nil {
			t.Fatal(err)
		}
	}
	if display.toolArgAppends != len(chunks) {
		t.Fatalf("raw tool input appends = %d, want %d", display.toolArgAppends, len(chunks))
	}
	got := display.latest()
	if got.Args != chunks[0]+chunks[1] {
		t.Fatalf("streamed director args = %q, want exact concatenated deltas %q", got.Args, chunks[0]+chunks[1])
	}
	if err := conversation.UpdateDisplayToolResult("call-1", "edit", "success", "ok", nil); err != nil {
		t.Fatal(err)
	}
	got = display.latest()
	if got.Args != chunks[0]+chunks[1] || got.Status != "success" || got.Result != "ok" {
		t.Fatalf("final director tool event changed raw input or result: %#v", got)
	}
}

type directorDisplayConversation struct {
	events         []session.DisplayEvent
	toolArgAppends int
}

func (c *directorDisplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
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
	c.toolArgAppends++
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

func (c *directorDisplayConversation) UpdateDisplayToolResult(id, _ string, status, result string, _ *agent.ToolPresentation) error {
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

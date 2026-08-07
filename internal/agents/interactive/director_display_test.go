package interactive

import (
	"context"
	agentcontext "denova/internal/agents/context"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/session"
)

func TestInteractiveDirectorDisplayHidesDirectorPlanWriteInput(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := NewDirectorConversation(DirectorConversationOptions{Display: display})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID:     "call-1",
		Role:   "tool_call",
		Name:   "write",
		Args:   `{"path":"/tmp/work/.denova/interactive/stories/story-1/director/main/director.md","content":"一二三"}`,
		Status: "running",
	}); err != nil {
		t.Fatal(err)
	}

	got := display.latest()
	if got.AgentKind != config.AgentKindInteractiveDirector {
		t.Fatalf("agent kind = %q, want interactive director", got.AgentKind)
	}
	if got.Args != `{"path":"director.md"}` {
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
	conversation := NewDirectorConversation(DirectorConversationOptions{Display: display})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{ID: "call-1", Role: "tool_call", Name: "write", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "write", `{"path":"director.md","content":"`+strings.Repeat("字", 101)); err != nil {
		t.Fatal(err)
	}
	running := display.latest()
	if running.Args != `{"path":"director.md"}` || strings.Contains(running.Args, "字") {
		t.Fatalf("streaming director args should only expose path, got %q", running.Args)
	}
	if running.SSEGeneratedChars != 101 {
		t.Fatalf("running generated chars = %d, want 101", running.SSEGeneratedChars)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "write", `尾"}`); err != nil {
		t.Fatal(err)
	}
	if err := conversation.UpdateDisplayToolResult("call-1", "write", "success", "ok"); err != nil {
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

func TestInteractiveDirectorDisplayCompactsStructuredPlanInputWithoutChapterBodyHiding(t *testing.T) {
	display := &directorDisplayConversation{}
	conversation := NewDirectorConversation(DirectorConversationOptions{Display: display})

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "call-1", Role: "tool_call", Name: "submit_director_plan_update", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "submit_director_plan_update", `{"decision":{"mode":"patch"},"updates":[{"document":"agent-brief.md","base_hash":"hash","edits":[{"op":"replace_section","section":"当前阶段","content":"`+strings.Repeat("字", 101)); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("call-1", "submit_director_plan_update", `"}]}],"finalize":true}`); err != nil {
		t.Fatal(err)
	}

	if display.toolArgAppends != 0 {
		t.Fatalf("structured Director plan deltas reached the raw display appender %d times, want 0", display.toolArgAppends)
	}
	got := display.latest()
	if got.Args != `{"documents":1,"finalize":true,"mode":"patch"}` {
		t.Fatalf("structured Director plan args = %q, want compact summary", got.Args)
	}
	if strings.Contains(got.Args, "字") {
		t.Fatalf("structured Director plan args leaked generated content: %q", got.Args)
	}
	if got.SSEGeneratedChars != 101 {
		t.Fatalf("generated chars = %d, want 101", got.SSEGeneratedChars)
	}
}

func TestDirectorToolTextCounterCountsEveryBatchEditValue(t *testing.T) {
	counter := directorToolTextCounter{}
	keys := directorToolGeneratedTextKeys("edit")
	chunks := []string{
		`{"path":"director.md","edits":[{"old_string":"\"new_string\":\"trap\"","new_`,
		`string":"第一"},{"old_string":"x","new_string":"二\n`,
		`三"}]}`,
	}
	total := 0
	for _, chunk := range chunks {
		total += counter.countDelta(chunk, keys)
	}
	if total != 5 {
		t.Fatalf("streamed batch edit character count = %d, want 5", total)
	}
}

func TestDirectorToolDisplayStateSynchronizesBatchEditCharacterCount(t *testing.T) {
	state := directorToolDisplayState{
		name:    "edit",
		rawArgs: `{"path":"director.md","edits":[{"old_string":"one","new_string":"第一"},{"old_string":"two","new_string":"二\n三"}]}`,
	}
	state.syncDecodedGeneratedChars()
	if state.generatedChars != 5 {
		t.Fatalf("decoded batch edit character count = %d, want 5", state.generatedChars)
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

package chat

import (
	"context"
	"testing"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestDisplayRecorderPersistsSubAgentAssistantChunks(t *testing.T) {
	appender := &fakeDisplayAppender{}
	recorder := newDisplayEventRecorder(fakeDisplayConversation{appender: appender}, displayEventRecorderOptions{})
	meta := agentEventMetadata{
		RunID: "run-1", AgentName: "researcher", RootAgentName: "DenovaAgent",
		RunPath: []string{"DenovaAgent", "researcher"}, SubAgent: true,
		SubAgentSessionID: "run-1-subagent-01-researcher", SubAgentType: "researcher",
	}

	recorder.Record(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": "第一段"})})
	recorder.Record(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": "第二段"})})

	if len(appender.events) != 1 {
		t.Fatalf("expected one merged display event, got %#v", appender.events)
	}
	event := appender.events[0]
	if event.Role != "assistant" || event.Content != "第一段第二段" {
		t.Fatalf("unexpected persisted subagent event: %#v", event)
	}
	if !event.SubAgent || event.SubAgentSessionID != "run-1-subagent-01-researcher" || event.SubAgentType != "researcher" {
		t.Fatalf("subagent metadata missing: %#v", event)
	}
}

type fakeDisplayConversation struct {
	appender *fakeDisplayAppender
}

func (c fakeDisplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}
func (c fakeDisplayConversation) AppendAssistant(string) error               { return nil }
func (c fakeDisplayConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c fakeDisplayConversation) PendingInterruption() *session.Interruption { return nil }
func (c fakeDisplayConversation) ResolveInterruption(string) error           { return nil }
func (c fakeDisplayConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	return c.appender.AppendDisplayEvent(event)
}
func (c fakeDisplayConversation) UpdateDisplayToolStatus(id, name, status string) error {
	return c.appender.UpdateDisplayToolStatus(id, name, status)
}
func (c fakeDisplayConversation) AppendDisplayEventContent(id, role, delta string) error {
	return c.appender.AppendDisplayEventContent(id, role, delta)
}

type fakeDisplayAppender struct {
	events []session.DisplayEvent
}

func (a *fakeDisplayAppender) AppendDisplayEvent(event session.DisplayEvent) error {
	a.events = append(a.events, event)
	return nil
}

func (a *fakeDisplayAppender) UpdateDisplayToolStatus(_, _, _ string) error { return nil }

func (a *fakeDisplayAppender) AppendDisplayEventContent(id, role, delta string) error {
	for index := range a.events {
		if a.events[index].ID == id && a.events[index].Role == role {
			a.events[index].Content += delta
			return nil
		}
	}
	return nil
}

package chat

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"

	agent "github.com/alfredxw/denova/agent"
)

func TestPublicEventProjectorKeepsDisplayIdentityAcrossCyclesAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("follow-up-display")
	if err != nil {
		t.Fatal(err)
	}
	type displayContent struct {
		Content string
		Cycle   int
	}
	live := map[string]displayContent{}
	for cycle := 1; cycle <= 2; cycle++ {
		projector := NewPublicEventProjector(agentconversation.NewSessionConversation(sess), ChatRequest{}, agentrun.Options{}, func(event agentrun.Event) {
			if event.Type != "thinking" && event.Type != "chunk" {
				return
			}
			id := event.DataString(displaySegmentIDEventKey)
			if id == "" || live[id].Content != "" {
				t.Fatalf("cycle %d reused display identity %q: %#v", cycle, id, live)
			}
			if eventDataInt(event.Data, "agent_cycle") != cycle {
				t.Fatalf("cycle %d lost its live display boundary: %#v", cycle, event)
			}
			live[id] = displayContent{Content: event.DataString("content"), Cycle: cycle}
		})
		projector.ProjectRunStarted("same-run", cycle, "same-command", "follow_up", time.Now())
		for _, source := range []agent.EventSource{{Name: "root"}, {Name: "child", InvocationID: "child-session", Path: []string{"root", "child"}}} {
			projector.Project(agent.Event{RunID: "same-run", Payload: agent.ThinkingDelta{Source: source, Delta: fmt.Sprintf("%s thinking %d", source.Name, cycle)}})
			projector.Project(agent.Event{RunID: "same-run", Payload: agent.AssistantDelta{Source: source, Delta: fmt.Sprintf("%s answer %d", source.Name, cycle)}})
		}
		projector.Finalize(agent.ResultCompleted, "")
	}
	reloadedStore, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, history := range [][]session.HistoryEntry{sess.History(), reloaded.History()} {
		persisted := map[string]displayContent{}
		for _, entry := range history {
			if entry.Role != "thinking" && entry.Role != "assistant" {
				continue
			}
			if persisted[entry.ID].Content != "" {
				t.Fatalf("duplicate persisted display identity: %#v", entry)
			}
			persisted[entry.ID] = displayContent{Content: entry.Content, Cycle: entry.AgentCycle}
		}
		if len(live) != 8 || !reflect.DeepEqual(persisted, live) {
			t.Fatalf("persisted display = %#v, live = %#v", persisted, live)
		}
	}
}

func TestPublicEventProjectorScopesExplicitSkillsToCycle(t *testing.T) {
	var calls []string
	for cycle := 1; cycle <= 2; cycle++ {
		projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{}, func(event agentrun.Event) {
			if event.Type == "tool_call" {
				calls = append(calls, event.DataString("id"))
			}
		})
		prepared := AgentContextPreparation{ExplicitSkills: []novaskills.Invocation{{Name: "review", Instructions: "Review the draft."}}}
		run := agent.RunView{ID: "same-run", Cycle: cycle}
		projector.ProjectPreparedContext(run, prepared)
		projector.ProjectRunStarted(run.ID, run.Cycle, "same-command", "follow_up", time.Now())
		projector.ProjectPreparedContext(run, prepared)
	}
	if len(calls) != 2 || calls[0] == "" || calls[1] == "" || calls[0] == calls[1] {
		t.Fatalf("explicit Skill identities across cycles = %#v", calls)
	}
}

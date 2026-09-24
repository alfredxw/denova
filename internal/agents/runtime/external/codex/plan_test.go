package codex

import (
	"reflect"
	"testing"
	"time"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

func TestNativePlanNotificationsIncludeReplacementAndClear(t *testing.T) {
	fixture := newProtocolFixture(t)
	host := &testHost{events: make(chan agentrun.Event, 4)}
	done := startFixtureTurn(t, t.Context(), fixture, host)
	for _, plan := range [][]map[string]string{{{"step": "Verify", "status": "inProgress"}}, {}} {
		fixture.send(t, "", "turn/plan/updated", map[string]any{"threadId": "thread", "turnId": "turn", "plan": plan})
		select {
		case event := <-host.events:
			items, err := external.PlanItems(event)
			if err != nil || event.Type != "todo_updated" {
				t.Fatalf("event=%+v err=%v", event, err)
			}
			if len(plan) != 0 && !reflect.DeepEqual(items, []agent.TodoItem{{ID: "1", Text: "Verify", Status: agent.TodoInProgress}}) {
				t.Fatalf("items=%+v", items)
			}
			if len(plan) == 0 && len(items) != 0 {
				t.Fatalf("plan not cleared: %+v", items)
			}
		case <-time.After(time.Second):
			t.Fatal("plan was not streamed")
		}
	}
	fixture.send(t, "", "turn/completed", map[string]any{"threadId": "thread", "turn": map[string]string{"id": "turn", "status": "completed"}})
	if result := <-done; result.err != nil || host.calls.Load() != 0 {
		t.Fatalf("result=%+v", result)
	}
}

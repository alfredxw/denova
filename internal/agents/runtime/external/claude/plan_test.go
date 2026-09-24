package claude

import (
	"encoding/json"
	"reflect"
	"testing"

	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

func TestNativePlanObservesConfirmedTasksAndIgnoresFailedUpdates(t *testing.T) {
	var output streamOutput
	host := &testHost{}
	frames := []string{
		`{"type":"assistant","message":{"id":"create","content":[{"type":"tool_use","id":"create","name":"TaskCreate","input":{"subject":"Verify"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"create"}]},"tool_use_result":{"task":{"id":"42","subject":"Verify"}}}`,
		`{"type":"assistant","message":{"id":"failed","content":[{"type":"tool_use","id":"failed","name":"TaskUpdate","input":{"taskId":"42","status":"completed"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"failed","is_error":true}]},"tool_use_result":{"success":false}}`,
	}
	for _, frame := range frames {
		if err := output.feed([]byte(frame), host); err != nil {
			t.Fatal(err)
		}
	}
	want := []agent.TodoItem{{ID: "42", Text: "Verify", Status: agent.TodoPending}}
	if !reflect.DeepEqual(output.plan, want) || len(host.calls) != 0 {
		t.Fatalf("plan=%+v calls=%v", output.plan, host.calls)
	}
	// A resumed stream starts with the canonical observation, then accepts only
	// fields the runtime reports as changed, including deletion and empty plans.
	resumed := streamOutput{plan: output.result().Plan.Items}
	call := contentBlock{Name: "TaskUpdate", Input: json.RawMessage(`{"taskId":"42","status":"completed","subject":"Unaccepted rename"}`)}
	if err := resumed.observePlan(host, call, json.RawMessage(`{"success":true,"updatedFields":["status"]}`)); err != nil {
		t.Fatal(err)
	}
	want[0].Status = agent.TodoCompleted
	if !reflect.DeepEqual(resumed.plan, want) {
		t.Fatalf("unconfirmed mutation: %+v", resumed.plan)
	}
	call.Input = json.RawMessage(`{"taskId":"42","status":"deleted"}`)
	if err := resumed.observePlan(host, call, json.RawMessage(`{"success":true,"updatedFields":["deleted"]}`)); err != nil {
		t.Fatal(err)
	}
	if result := resumed.result(); result.Plan == nil || len(result.Plan.Items) != 0 {
		t.Fatalf("missing clear: %+v", result)
	}
	if _, err := external.PlanItems(external.PlanEvent(resumed.plan)); err != nil {
		t.Fatal(err)
	}
}

package platform

import (
	"encoding/json"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentStateEventsContainObservableStatus(t *testing.T) {
	execution := &agentExecution{}
	for _, payload := range []agent.EventPayload{agent.RunStarted{}, agent.InteractionResolved{}} {
		execution.receipt.Result.Status = "waiting"
		execution.consume(agent.Event{Payload: payload})
		event := execution.events[len(execution.events)-1]
		data, err := json.Marshal(event.Data)
		if err != nil || event.Kind != "state" || string(data) != `{"status":"running"}` {
			t.Fatalf("SSE state cannot be observed by consumers: %#v %s %v", event, data, err)
		}
	}
}

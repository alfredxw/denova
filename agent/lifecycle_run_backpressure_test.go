package agent

import (
	"context"
	"testing"
	"time"
)

func TestRunWaitDoesNotDependOnConsumingEvents(t *testing.T) {
	// Cross the public observer bound far enough that both the direct stream and
	// a subscriber reconnect must preserve authoritative settlement.
	chunks := make([]*Message, 2*publicRunEventBuffer)
	for index := range chunks {
		chunks[index] = AssistantMessage("x", nil)
	}
	model := &scriptedModel{responses: []scriptedModelResponse{{chunks: chunks}}}
	owner, err := New(
		context.Background(),
		Definition{Model: model},
		WithLimits(Limits{ObservationBuffer: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("stream without a display consumer"))
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := run.Wait(waitCtx)
	if err != nil || result.Status != ResultCompleted {
		t.Fatalf("Wait result=%#v error=%v", result, err)
	}

	var sawGap, sawSettlement bool
	for event := range run.Events() {
		switch event.Payload.(type) {
		case EventStreamGap:
			sawGap = true
		case RunSettled:
			sawSettlement = true
		}
	}
	if !sawGap || !sawSettlement {
		t.Fatalf("bounded event stream gap=%t settlement=%t", sawGap, sawSettlement)
	}
}

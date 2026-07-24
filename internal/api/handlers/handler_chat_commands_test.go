package handlers

import (
	"testing"

	appsvc "denova/internal/app"
)

func TestWritingAgentCommandKindIncludesQueueControls(t *testing.T) {
	t.Parallel()

	tests := map[string]appsvc.AgentCommandKind{
		"steer":         appsvc.AgentCommandSteer,
		"follow_up":     appsvc.AgentCommandFollowUp,
		"next_turn":     appsvc.AgentCommandNextTurn,
		"abort":         appsvc.AgentCommandAbort,
		"steer_queued":  appsvc.AgentCommandSteerQueued,
		"cancel_queued": appsvc.AgentCommandCancelQueued,
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := writingAgentCommandKind(input)
			if err != nil || got != want {
				t.Fatalf("writingAgentCommandKind(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
	if got, err := writingAgentCommandKind("queue"); err == nil {
		t.Fatalf("writingAgentCommandKind(queue) = %q, want error", got)
	}
}

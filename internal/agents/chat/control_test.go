package chat

import (
	"context"
	"testing"
	"time"

	"denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

func TestRunControlWatcherRecoversCancellationPanicAndExits(t *testing.T) {
	controls := make(chan agentrun.Control, 1)
	controls <- agentrun.Control{Kind: agentrun.ControlAbort, Reason: "test panic"}
	close(controls)
	done := startRunControlWatcher(
		context.Background(),
		controls,
		func(...agent.AgentCancelOption) (*agent.CancelHandle, bool) {
			panic("cancel panic")
		},
		&runControlState{},
	)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for recovered control watcher exit")
	}
}

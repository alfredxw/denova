package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

func TestInteractiveStartRegistryEvictsSettledTaskBuffersByBytesButKeepsIdentity(t *testing.T) {
	first := settledTaskWithReplay(t, "same-size")
	second := settledTaskWithReplay(t, "same-size")
	perTask := first.DisplayReplayBytes()
	registry := interactiveStartRegistry{replayByteLimit: perTask}
	firstIdentity := interactiveReplayIdentity("game-1", "fingerprint-1")
	secondIdentity := interactiveReplayIdentity("game-2", "fingerprint-2")
	if err := registry.remember(firstIdentity, first); err != nil {
		t.Fatal(err)
	}
	if err := registry.remember(secondIdentity, second); err != nil {
		t.Fatal(err)
	}

	if registry.records["game-1"].task != nil || registry.records["game-2"].task != second {
		t.Fatalf("Game replay byte pruning = %#v", registry.records)
	}
	if first.DisplayReplayBytes() != 0 {
		t.Fatalf("evicted Game Task still owns %d replay bytes", first.DisplayReplayBytes())
	}
	if replay, ok, err := registry.replay(firstIdentity); err != nil || ok || replay != nil {
		t.Fatalf("evicted Game Task should fall through to durable replay: task:%p ok:%t err:%v", replay, ok, err)
	}
	conflict := firstIdentity
	conflict.fingerprint = "different"
	if _, _, err := registry.replay(conflict); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("evicted Game identity no longer rejects conflicting payload: %v", err)
	}
	rebuilt := settledTaskWithReplay(t, "same-size")
	if err := registry.remember(firstIdentity, rebuilt); err != nil {
		t.Fatal(err)
	}
	if registry.records["game-1"].task != rebuilt {
		t.Fatal("durably rebuilt Game Task was not reattached to its retained identity")
	}
}

func settledTaskWithReplay(t *testing.T, content string) *apptask.Task {
	t.Helper()
	task := apptask.New(func(_ context.Context, _ *apptask.Task, emit func(agentrun.Event)) {
		emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": strings.Repeat(content, 32)}})
		emit(agentrun.Event{Type: "done", Data: map[string]any{}})
	})
	<-task.Done()
	return task
}

func interactiveReplayIdentity(commandID, fingerprint string) interactiveStartIdentity {
	return interactiveStartIdentity{
		request: InteractiveAgentStartRequest{CommandID: commandID}, fingerprint: fingerprint,
	}
}

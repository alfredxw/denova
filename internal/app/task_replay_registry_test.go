package app

import (
	"context"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"strings"
	"testing"
)

func TestWritingStartRegistryEvictsSettledTaskBuffersByLRUBytesButKeepsIdentity(t *testing.T) {
	first := settledTaskWithReplay(t, "same-size")
	second := settledTaskWithReplay(t, "same-size")
	third := settledTaskWithReplay(t, "same-size")
	perTask := first.DisplayReplayBytes()
	if perTask == 0 || second.DisplayReplayBytes() != perTask || third.DisplayReplayBytes() != perTask {
		t.Fatalf("test replay sizes are not stable: %d %d %d", perTask, second.DisplayReplayBytes(), third.DisplayReplayBytes())
	}

	registry := writingStartRegistry{replayByteLimit: 2 * perTask}
	rememberWritingTask(t, &registry, "writing-1", "fingerprint-1", first)
	rememberWritingTask(t, &registry, "writing-2", "fingerprint-2", second)
	if replay, ok, err := registry.replay("writing-1", "/workspace", "session-1", "fingerprint-1"); err != nil || !ok || replay != first {
		t.Fatalf("touch first replay = task:%p ok:%t err:%v", replay, ok, err)
	}
	rememberWritingTask(t, &registry, "writing-3", "fingerprint-3", third)

	if registry.records["writing-2"].task != nil {
		t.Fatal("least-recently-used settled Writing Task retained its display buffers")
	}
	if second.DisplayReplayBytes() != 0 {
		t.Fatalf("evicted Writing Task still owns %d replay bytes", second.DisplayReplayBytes())
	}
	evictedReplay, evictedSubscription, err := second.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if evictedReplay.Checkpoint == nil || evictedReplay.Checkpoint.Complete || len(evictedReplay.Checkpoint.Events) != 0 {
		t.Fatalf("evicted Task masqueraded as a complete empty replay: %#v", evictedReplay)
	}
	if _, open := <-evictedSubscription.Events(); open {
		t.Fatal("evicted settled Task returned a live subscription")
	}
	if registry.records["writing-1"].task != first || registry.records["writing-3"].task != third {
		t.Fatalf("recent Writing Tasks were evicted: %#v", registry.records)
	}
	if replay, ok, err := registry.replay("writing-2", "/workspace", "session-1", "fingerprint-2"); err != nil || ok || replay != nil {
		t.Fatalf("evicted Writing Task should fall through to durable replay: task:%p ok:%t err:%v", replay, ok, err)
	}
	if _, _, err := registry.replay("writing-2", "/workspace", "session-1", "different"); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("evicted Writing identity no longer rejects conflicting payload: %v", err)
	}
	rebuilt := settledTaskWithReplay(t, "same-size")
	rememberWritingTask(t, &registry, "writing-2", "fingerprint-2", rebuilt)
	if registry.records["writing-2"].task != rebuilt {
		t.Fatal("durably rebuilt Writing Task was not reattached to its retained identity")
	}
}

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

func TestWritingStartRegistryReservesActiveTaskReplayCapacityBeforeItGrows(t *testing.T) {
	settled := settledTaskWithReplay(t, "same-size")
	settledBytes := settled.DisplayReplayBytes()
	active, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	activeLimit := (settledBytes + 1) / 2
	active.ConfigureRetention(0, activeLimit)
	if activeLimit <= 0 {
		t.Fatal("invalid active replay capacity fixture")
	}
	registry := writingStartRegistry{replayByteLimit: active.DisplayReplayCharge()}
	rememberWritingTask(t, &registry, "settled", "settled-fingerprint", settled)
	if registry.records["settled"].task != settled {
		t.Fatal("settled replay did not fit before the active capacity reservation")
	}
	rememberWritingTask(t, &registry, "active", "active-fingerprint", active)

	if registry.records["settled"].task != nil || settled.DisplayReplayBytes() != 0 {
		t.Fatal("settled replay was not released to reserve the active Task's future capacity")
	}
	if registry.records["active"].task != active {
		t.Fatal("active Task was evicted while reserving its bounded replay capacity")
	}
	active.RejectStart(errors.New("test complete"))
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

func rememberWritingTask(t *testing.T, registry *writingStartRegistry, commandID, fingerprint string, task *apptask.Task) {
	t.Helper()
	if err := registry.remember(writingStartRecord{
		commandID: commandID, workspace: "/workspace", sessionID: "session-1", fingerprint: fingerprint, task: task,
	}); err != nil {
		t.Fatal(err)
	}
}

func interactiveReplayIdentity(commandID, fingerprint string) interactiveStartIdentity {
	return interactiveStartIdentity{
		request:     InteractiveAgentStartRequest{CommandID: commandID},
		fingerprint: fingerprint,
	}
}

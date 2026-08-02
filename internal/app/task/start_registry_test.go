package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
)

func TestStartRegistryEvictsSettledDisplayByLRUAndKeepsIdentity(t *testing.T) {
	first := settledStartTask(t, "same-size")
	second := settledStartTask(t, "same-size")
	third := settledStartTask(t, "same-size")
	perTask := first.DisplayReplayBytes()
	if perTask == 0 || second.DisplayReplayBytes() != perTask || third.DisplayReplayBytes() != perTask {
		t.Fatalf("test replay sizes are unstable: %d %d %d", perTask, second.DisplayReplayBytes(), third.DisplayReplayBytes())
	}

	registry := NewStartRegistry(StartRegistryOptions{Label: "Writing", ReplayByteLimit: 2 * perTask})
	rememberStartTask(t, &registry, "writing-1", "fingerprint-1", first)
	rememberStartTask(t, &registry, "writing-2", "fingerprint-2", second)
	if replay, ok, err := registry.Replay(startIdentity("writing-1", "fingerprint-1")); err != nil || !ok || replay != first {
		t.Fatalf("touch first replay = task:%p ok:%t err:%v", replay, ok, err)
	}
	rememberStartTask(t, &registry, "writing-3", "fingerprint-3", third)

	if registry.records["writing-2"].Task != nil || second.DisplayReplayBytes() != 0 {
		t.Fatal("least-recently-used settled Task retained display replay")
	}
	if registry.records["writing-1"].Task != first || registry.records["writing-3"].Task != third {
		t.Fatalf("recent Tasks were evicted: %#v", registry.records)
	}
	if replay, ok, err := registry.Replay(startIdentity("writing-2", "fingerprint-2")); err != nil || ok || replay != nil {
		t.Fatalf("evicted Task should fall through to durable replay: task:%p ok:%t err:%v", replay, ok, err)
	}
	if _, _, err := registry.Replay(startIdentity("writing-2", "different")); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("retained identity accepted a conflicting payload: %v", err)
	}
	rebuilt := settledStartTask(t, "same-size")
	rememberStartTask(t, &registry, "writing-2", "fingerprint-2", rebuilt)
	if registry.records["writing-2"].Task != rebuilt {
		t.Fatal("durably rebuilt Task was not rebound to retained identity")
	}
}

func TestStartRegistryReservesActiveReplayCapacityBeforeGrowth(t *testing.T) {
	settled := settledStartTask(t, "same-size")
	settledBytes := settled.DisplayReplayBytes()
	active, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	activeLimit := (settledBytes + 1) / 2
	active.ConfigureRetention(0, activeLimit)
	registry := NewStartRegistry(StartRegistryOptions{ReplayByteLimit: active.DisplayReplayCharge()})
	rememberStartTask(t, &registry, "settled", "settled-fingerprint", settled)
	rememberStartTask(t, &registry, "active", "active-fingerprint", active)

	if registry.records["settled"].Task != nil || settled.DisplayReplayBytes() != 0 {
		t.Fatal("settled replay was not released for active capacity")
	}
	if registry.records["active"].Task != active {
		t.Fatal("active Task was evicted while reserving bounded capacity")
	}
	active.RejectStart(errors.New("test complete"))
}

func TestStartRegistryLatestAndReleaseAreScopeBound(t *testing.T) {
	registry := NewStartRegistry(StartRegistryOptions{})
	first := settledStartTask(t, "first")
	second := settledStartTask(t, "second")
	if err := registry.Remember(StartRecord{Identity: StartIdentity{CommandID: "one", Scope: "a", SessionID: "s", Fingerprint: "1"}, Task: first}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remember(StartRecord{Identity: StartIdentity{CommandID: "two", Scope: "b", SessionID: "s", Fingerprint: "2"}, Task: second}); err != nil {
		t.Fatal(err)
	}
	if latest := registry.Latest("a", "s"); latest.Task != first || latest.Identity.CommandID != "one" {
		t.Fatalf("latest scoped record = %#v", latest)
	}
	registry.ReleaseScope("a", "s")
	if latest := registry.Latest("a", "s"); latest.Task != nil {
		t.Fatalf("released scope retained Task: %#v", latest)
	}
	if latest := registry.Latest("b", "s"); latest.Task != second {
		t.Fatalf("unrelated scope was released: %#v", latest)
	}
}

func settledStartTask(t *testing.T, content string) *Task {
	t.Helper()
	task := New(func(_ context.Context, _ *Task, emit func(agentrun.Event)) {
		emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": strings.Repeat(content, 32)}})
	})
	<-task.Done()
	return task
}

func rememberStartTask(t *testing.T, registry *StartRegistry, commandID, fingerprint string, task *Task) {
	t.Helper()
	if err := registry.Remember(StartRecord{Identity: startIdentity(commandID, fingerprint), Task: task}); err != nil {
		t.Fatal(err)
	}
}

func startIdentity(commandID, fingerprint string) StartIdentity {
	return StartIdentity{CommandID: commandID, Scope: "/workspace", SessionID: "session-1", Fingerprint: fingerprint}
}

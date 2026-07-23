package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"denova/internal/agent"
	"denova/internal/automation"
)

func TestAutomationFollowUpRegistryReplaysExactTaskAndRejectsConflict(t *testing.T) {
	identity, err := newAutomationFollowUpIdentity("run-1", "command-1", "continue")
	if err != nil {
		t.Fatal(err)
	}
	task := NewTask(nil)
	run := automation.RunRecord{ID: "run-1"}
	var registry automationFollowUpRegistry
	if err := registry.remember(identity, task, run); err != nil {
		t.Fatal(err)
	}
	replayed, replayedRun, ok, err := registry.replay(identity)
	if err != nil || !ok || replayed != task || replayedRun.ID != run.ID {
		t.Fatalf("replay task=%p run=%q ok=%v err=%v", replayed, replayedRun.ID, ok, err)
	}
	conflict, err := newAutomationFollowUpIdentity("run-1", "command-1", "different")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := registry.replay(conflict); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestAutomationFollowUpRegistryEvictsSettledDisplayButKeepsIdentity(t *testing.T) {
	first := settledTaskWithReplay(t, "same-size")
	second := settledTaskWithReplay(t, "same-size")
	perTask := first.displayReplayBytes()
	if perTask == 0 {
		t.Fatal("settled Automation replay has no display bytes")
	}
	firstIdentity, err := newAutomationFollowUpIdentity("run", "command-first", "first")
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := newAutomationFollowUpIdentity("run", "command-second", "second")
	if err != nil {
		t.Fatal(err)
	}
	registry := automationFollowUpRegistry{replayByteLimit: perTask}
	if err := registry.remember(firstIdentity, first, automation.RunRecord{ID: "run"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.remember(secondIdentity, second, automation.RunRecord{ID: "run"}); err != nil {
		t.Fatal(err)
	}
	if record := registry.records[firstIdentity.commandID]; record.task != nil {
		t.Fatalf("least-recent Automation display task was retained: %#v", record)
	}
	if first.displayReplayBytes() != 0 {
		t.Fatalf("evicted Automation task retains %d bytes", first.displayReplayBytes())
	}
	replay, subscription, err := first.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || replay.Checkpoint.Complete || len(replay.Checkpoint.Events) != 0 {
		t.Fatalf("evicted Automation replay is not explicitly incomplete: %#v", replay)
	}
	if _, open := <-subscription.Events(); open {
		t.Fatal("settled evicted Automation task returned a live subscription")
	}
	if task, _, ok, err := registry.replay(firstIdentity); err != nil || ok || task != nil {
		t.Fatalf("exact evicted replay task=%p ok=%t err=%v", task, ok, err)
	}
	conflict, err := newAutomationFollowUpIdentity("run", firstIdentity.commandID, "different")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := registry.replay(conflict); !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("evicted identity accepted conflicting payload: %v", err)
	}
	rebuilt := settledTaskWithReplay(t, "same-size")
	if err := registry.remember(firstIdentity, rebuilt, automation.RunRecord{ID: "run"}); err != nil {
		t.Fatal(err)
	}
	if registry.records[firstIdentity.commandID].task != rebuilt {
		t.Fatal("durably rebuilt Automation replay was not reattached")
	}
}

func TestAutomationFollowUpRegistryRejectsBeforeRuntimeWhenActiveCapacityIsFull(t *testing.T) {
	first, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.failBeforeStart(errors.New("test complete"))
	defer second.failBeforeStart(errors.New("test complete"))
	firstIdentity, _ := newAutomationFollowUpIdentity("run", "command-first", "first")
	secondIdentity, _ := newAutomationFollowUpIdentity("run", "command-second", "second")
	registry := automationFollowUpRegistry{replayByteLimit: first.displayReplayRegistryCharge()}
	firstReservation, err := registry.reserve(firstIdentity, first)
	if err != nil {
		t.Fatal(err)
	}
	firstReservation.bind(automation.RunRecord{ID: "run"})

	runtimeStarts := 0
	reservation, err := registry.reserve(secondIdentity, second)
	if err == nil {
		runtimeStarts++
		reservation.bind(automation.RunRecord{ID: "run"})
	}
	if !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("second admission error = %v", err)
	}
	if runtimeStarts != 0 {
		t.Fatalf("underlying Runtime start calls = %d, want zero", runtimeStarts)
	}
}

func TestAutomationFollowUpRegistryBoundsUnfinishedIdentities(t *testing.T) {
	release := make(chan struct{})
	tasks := make([]*Task, 0, maxRememberedAutomationFollowUps+1)
	defer func() {
		close(release)
		for _, task := range tasks {
			<-task.Done()
		}
	}()
	var registry automationFollowUpRegistry
	for index := 0; index < maxRememberedAutomationFollowUps; index++ {
		identity, err := newAutomationFollowUpIdentity("run", fmt.Sprintf("command-%03d", index), fmt.Sprintf("message-%03d", index))
		if err != nil {
			t.Fatal(err)
		}
		task := blockingAutomationRegistryTask(release)
		if index == 0 {
			registry.replayByteLimit = maxRememberedAutomationFollowUps * task.displayReplayRegistryCharge()
		}
		tasks = append(tasks, task)
		if err := registry.remember(identity, task, automation.RunRecord{ID: "run"}); err != nil {
			t.Fatal(err)
		}
	}
	lastIdentity, err := newAutomationFollowUpIdentity("run", "command-over-capacity", "last message")
	if err != nil {
		t.Fatal(err)
	}
	lastTask := blockingAutomationRegistryTask(release)
	tasks = append(tasks, lastTask)
	err = registry.remember(lastIdentity, lastTask, automation.RunRecord{ID: "run"})
	if !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("129th unfinished follow-up error = %v", err)
	}
	if len(registry.records) != maxRememberedAutomationFollowUps {
		t.Fatalf("unfinished registry records = %d, want %d", len(registry.records), maxRememberedAutomationFollowUps)
	}
}

func TestAutomationEvictedDisplayCheckpointRequiresCanonicalRehydrate(t *testing.T) {
	task := NewTask(func(_ context.Context, _ *Task, emit func(agent.Event)) {
		emit(agent.Event{Type: "agent_cycle_started", Data: map[string]any{"command_id": "follow-up"}})
		emit(agent.Event{Type: "chunk", Data: map[string]any{"content": strings.Repeat("bounded", 64)}})
	})
	<-task.Done()
	if task.releaseDisplayReplay() == 0 {
		t.Fatal("fixture did not release display replay")
	}
	replay, subscription, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || replay.Checkpoint.Complete || replay.Checkpoint.Settled != true {
		t.Fatalf("checkpoint = %#v", replay.Checkpoint)
	}
	if _, open := <-subscription.Events(); open {
		t.Fatal("settled rehydrate checkpoint returned a live stream")
	}
}

package configmanager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

type projectionProbe struct {
	stored, recovered          agentrun.RuntimeStatus
	storedCalls, recoveryCalls int
}

func (probe *projectionProbe) RuntimeStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error) {
	probe.storedCalls++
	return probe.stored, nil
}

func (probe *projectionProbe) RuntimeRecoveryStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error) {
	probe.recoveryCalls++
	return probe.recovered, nil
}

func TestIdleProjectionNeverOpensRecoveryActor(t *testing.T) {
	probe := &projectionProbe{stored: agentrun.RuntimeStatus{Phase: agentrun.RunPhaseIdle}}
	for index := 0; index < 256; index++ {
		status, ok := projectRuntime(context.Background(), probe, agentrun.Options{
			AgentKind: agentrun.AgentKindConfigManager, Workspace: "/book",
			SessionID: fmt.Sprintf("config-scope-%d", index),
		})
		if !ok || status.Phase != agentrun.RunPhaseIdle {
			t.Fatalf("idle projection %d = %#v projected=%t", index, status, ok)
		}
	}
	if probe.storedCalls != 256 || probe.recoveryCalls != 0 {
		t.Fatalf("projection calls stored=%d recovery=%d", probe.storedCalls, probe.recoveryCalls)
	}
}

func TestColdUnfinishedProjectionOpensCanonicalRecovery(t *testing.T) {
	probe := &projectionProbe{
		stored: agentrun.RuntimeStatus{
			Phase: agentrun.RunPhaseIdle, RecoveryPending: true,
			LastOperation: &agentrun.OperationSummary{Status: agentrun.OperationInterrupted},
		},
		recovered: agentrun.RuntimeStatus{
			Phase: agentrun.RunPhaseRunning, RecoveryPaused: true,
			ActiveCommandID: "accepted-start", ActiveOperation: "operation-1",
		},
	}
	status, ok := projectRuntime(context.Background(), probe, agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager, Workspace: "/book", SessionID: "config-scope",
	})
	if !ok || !status.RecoveryPaused || status.ActiveCommandID != "accepted-start" {
		t.Fatalf("canonical recovery projection = %#v projected=%t", status, ok)
	}
	if probe.storedCalls != 1 || probe.recoveryCalls != 1 {
		t.Fatalf("projection calls stored=%d recovery=%d", probe.storedCalls, probe.recoveryCalls)
	}
}

func TestNewNormalTaskSupersedesSettledRecoveryDisplay(t *testing.T) {
	recoveryTask, _ := apptask.NewDeferred(nil)
	recoveryTask.RejectStart(errors.New("settled recovery"))
	startTask, _ := apptask.NewDeferred(nil)
	startTask.RejectStart(errors.New("settled normal run"))
	selected, recoverySelected := selectDisplayRecord(
		taskRecord{CommandID: "new-normal", Task: startTask}, &recoveryRun{task: recoveryTask},
	)
	if recoverySelected || selected.Task != startTask || selected.CommandID != "new-normal" {
		t.Fatalf("selected display = %#v recovery_selected=%t", selected, recoverySelected)
	}
}

func TestSettledOrMismatchedDisplayIsNotStreamAttached(t *testing.T) {
	settled, _ := apptask.NewDeferred(nil)
	settled.RejectStart(errors.New("settled display"))
	runtime := agentrun.RuntimeStatus{
		Phase: agentrun.RunPhaseRunning, RecoveryPaused: true,
		ActiveCommandID: "accepted-start", ActiveOperation: "operation-1",
	}
	if displayOwnsRuntime(taskRecord{CommandID: "accepted-start", Task: settled}, runtime) {
		t.Fatal("settled Config Manager display was reported as stream attached")
	}
	active, _ := apptask.NewDeferred(nil)
	t.Cleanup(func() { active.RejectStart(errors.New("test cleanup")) })
	if displayOwnsRuntime(taskRecord{CommandID: "older-command", Task: active}, runtime) {
		t.Fatal("display task from another Runtime command was reported as attached")
	}
	if !displayOwnsRuntime(taskRecord{CommandID: "accepted-start", Task: active}, runtime) {
		t.Fatal("active display task for the exact Runtime command was not attached")
	}
}

func TestRecoveryRegistryRejectsUnboundedActiveRuns(t *testing.T) {
	registry := recoveryRegistry{replayByteLimit: (maxRememberedRecoveries + 1) * (64 << 20)}
	tasks := make([]*apptask.Task, 0, maxRememberedRecoveries+1)
	t.Cleanup(func() {
		for _, task := range tasks {
			task.RejectStart(errors.New("test cleanup"))
		}
	})
	for index := 0; index < maxRememberedRecoveries; index++ {
		task, _ := apptask.NewDeferred(nil)
		tasks = append(tasks, task)
		if err := registry.install(&recoveryRun{
			projectID: "book", sessionID: fmt.Sprintf("scope-%d", index), task: task,
		}); err != nil {
			t.Fatalf("install recovery %d: %v", index, err)
		}
	}
	overflow, _ := apptask.NewDeferred(nil)
	tasks = append(tasks, overflow)
	err := registry.install(&recoveryRun{projectID: "book", sessionID: "overflow", task: overflow})
	if !errors.Is(err, apptask.ErrReplayCapacity) || len(registry.runs) != maxRememberedRecoveries {
		t.Fatalf("overflow err=%v records=%d", err, len(registry.runs))
	}
}

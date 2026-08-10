package execution

import (
	"denova/internal/agents/run"
	"reflect"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRuntimeRecoveryActionsExposeOnlySafeOrderedIdentity(t *testing.T) {
	snapshot := agentrun.RuntimeStatus{
		Phase:           agentrun.RunPhaseRunning,
		RecoveryPaused:  true,
		ActiveCommandID: "start",
		ActiveOperation: "operation-parent",
		Queue: []agentrun.QueuedCommand{
			{
				CommandID: "follow", OperationID: "operation-parent", Delivery: agentrun.DeliveryFollowUp,
				Message: "secret",
			},
			{CommandID: "next", OperationID: "operation-next", Delivery: agentrun.DeliveryNextTurn},
		},
	}
	actions := RuntimeRecoveryActions(snapshot)
	if len(actions) != 3 {
		t.Fatalf("actions = %#v", actions)
	}
	if actions[0] != (RuntimeRecoveryAction{Kind: RuntimeRecoveryAttach, CommandID: "start", OperationID: "operation-parent"}) ||
		actions[1].Kind != RuntimeRecoveryAbort || actions[1].OperationID != "operation-parent" || actions[1].CommandID == "" ||
		actions[2] != (RuntimeRecoveryAction{Kind: RuntimeRecoveryFollowUp, CommandID: "follow", OperationID: "operation-parent"}) {
		t.Fatalf("ordered safe actions = %#v", actions)
	}
	if actions[1].CommandID != runtimeRecoveryAbortCommandID(snapshot) {
		t.Fatalf("abort identity is not deterministic: %#v", actions[1])
	}
}

func TestRuntimeRecoveryActionsStrictMatrix(t *testing.T) {
	tests := []struct {
		name     string
		snapshot agentrun.RuntimeStatus
		want     []RuntimeRecoveryAction
	}{
		{
			name: "paused running chooses steer over earlier queue entries",
			snapshot: agentrun.RuntimeStatus{
				Phase:           agentrun.RunPhaseRunning,
				RecoveryPaused:  true,
				ActiveCommandID: "start",
				ActiveOperation: "parent",
				Queue: []agentrun.QueuedCommand{
					{CommandID: "next", OperationID: "next-operation", Delivery: agentrun.DeliveryNextTurn},
					{CommandID: "follow", OperationID: "parent", Delivery: agentrun.DeliveryFollowUp},
					{CommandID: "steer", OperationID: "parent", Delivery: agentrun.DeliverySteer},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryAttach, CommandID: "start", OperationID: "parent"},
				{Kind: RuntimeRecoveryAbort, OperationID: "parent"},
				{Kind: RuntimeRecoverySteer, CommandID: "steer", OperationID: "parent"},
			},
		},
		{
			name: "input materialization recovery hides later queue",
			snapshot: agentrun.RuntimeStatus{
				Phase:           agentrun.RunPhaseRunning,
				RecoveryPaused:  true,
				ActiveCommandID: "start",
				ActiveOperation: "parent",
				InputRecovery: &agentrun.InputRecovery{
					CommandID: "recover-follow", OperationID: "parent", Delivery: agentrun.DeliveryFollowUp,
				},
				Queue: []agentrun.QueuedCommand{
					{CommandID: "later-steer", OperationID: "parent", Delivery: agentrun.DeliverySteer},
					{CommandID: "later-next", OperationID: "next-operation", Delivery: agentrun.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryAttach, CommandID: "start", OperationID: "parent"},
				{Kind: RuntimeRecoveryAbort, OperationID: "parent"},
				{Kind: RuntimeRecoveryFollowUp, CommandID: "recover-follow", OperationID: "parent"},
			},
		},
		{
			name: "paused compacting exposes structural and abort only",
			snapshot: agentrun.RuntimeStatus{
				Phase:           agentrun.RunPhaseCompacting,
				RecoveryPaused:  true,
				ActiveOperation: "compact-operation",
				ActiveStructural: &agentrun.StructuralOperation{
					CommandID: "compact", OperationID: "compact-operation", Kind: agentrun.StructuralCompactContext,
				},
				Queue: []agentrun.QueuedCommand{
					{CommandID: "queued-next", OperationID: "next-operation", Delivery: agentrun.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryCompactContext, CommandID: "compact", OperationID: "compact-operation"},
				{Kind: RuntimeRecoveryAbort, OperationID: "compact-operation"},
			},
		},
		{
			name: "idle exposes only first queued next turn",
			snapshot: agentrun.RuntimeStatus{
				Phase: agentrun.RunPhaseIdle,
				Queue: []agentrun.QueuedCommand{
					{CommandID: "orphan-follow", OperationID: "old-operation", Delivery: agentrun.DeliveryFollowUp},
					{CommandID: "next-first", OperationID: "next-operation-1", Delivery: agentrun.DeliveryNextTurn},
					{CommandID: "next-second", OperationID: "next-operation-2", Delivery: agentrun.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryNextTurn, CommandID: "next-first", OperationID: "next-operation-1"},
			},
		},
		{
			name: "idle terminal has no recovery action",
			snapshot: agentrun.RuntimeStatus{
				Phase: agentrun.RunPhaseIdle,
				LastOperation: &agentrun.OperationSummary{
					CommandID: "done", OperationID: "done-operation", Status: agentrun.OperationSucceeded,
				},
			},
			want: []RuntimeRecoveryAction{},
		},
		{
			name: "ordinary live running has no recovery action",
			snapshot: agentrun.RuntimeStatus{
				Phase:           agentrun.RunPhaseRunning,
				ActiveCommandID: "start",
				ActiveOperation: "live-operation",
				Queue: []agentrun.QueuedCommand{
					{CommandID: "queued-next", OperationID: "next-operation", Delivery: agentrun.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for index := range test.want {
				if test.want[index].Kind == RuntimeRecoveryAbort {
					test.want[index].CommandID = runtimeRecoveryAbortCommandID(test.snapshot)
				}
			}
			if got := RuntimeRecoveryActions(test.snapshot); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("actions = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestNewStartRecognizesPausedOrPendingRecoveryWithoutWaiting(t *testing.T) {
	for _, snapshot := range []runstate.StateSnapshot{
		{Phase: runstate.PhaseRunning, RecoveryPaused: true},
		{Phase: runstate.PhaseIdle, Queue: []runstate.QueuedInput{{Delivery: runstate.DeliveryNextTurn}}},
	} {
		if !snapshotRequiresExplicitRecovery(snapshot) {
			t.Fatalf("snapshot did not require recovery: %#v", snapshot)
		}
	}
	if snapshotRequiresExplicitRecovery(runstate.StateSnapshot{Phase: runstate.PhaseRunning}) {
		t.Fatal("ordinary live operation was treated as cold recovery")
	}
}

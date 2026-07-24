package agents

import (
	"encoding/json"
	"reflect"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRuntimeRecoveryActionsExposeOnlySafeOrderedIdentity(t *testing.T) {
	snapshot := runstate.StatusSnapshot{
		Phase:           runstate.PhaseRunning,
		RecoveryPaused:  true,
		ActiveCommandID: "start",
		ActiveOperation: "operation-parent",
		Queue: []runstate.QueuedInput{
			{
				CommandID: "follow", OperationID: "operation-parent", Delivery: runstate.DeliveryFollowUp,
				Input: runstate.UserInput{Text: "secret", RestoreDescriptor: json.RawMessage(`{"secret":true}`)},
			},
			{CommandID: "next", OperationID: "operation-next", Delivery: runstate.DeliveryNextTurn},
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
		snapshot runstate.StatusSnapshot
		want     []RuntimeRecoveryAction
	}{
		{
			name: "paused running chooses steer over earlier queue entries",
			snapshot: runstate.StatusSnapshot{
				Phase:           runstate.PhaseRunning,
				RecoveryPaused:  true,
				ActiveCommandID: "start",
				ActiveOperation: "parent",
				Queue: []runstate.QueuedInput{
					{CommandID: "next", OperationID: "next-operation", Delivery: runstate.DeliveryNextTurn},
					{CommandID: "follow", OperationID: "parent", Delivery: runstate.DeliveryFollowUp},
					{CommandID: "steer", OperationID: "parent", Delivery: runstate.DeliverySteer},
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
			snapshot: runstate.StatusSnapshot{
				Phase:           runstate.PhaseRunning,
				RecoveryPaused:  true,
				ActiveCommandID: "start",
				ActiveOperation: "parent",
				InputRecovery: &runstate.InputMaterializationRecovery{
					CommandID: "recover-follow", OperationID: "parent", Delivery: runstate.DeliveryFollowUp,
				},
				Queue: []runstate.QueuedInput{
					{CommandID: "later-steer", OperationID: "parent", Delivery: runstate.DeliverySteer},
					{CommandID: "later-next", OperationID: "next-operation", Delivery: runstate.DeliveryNextTurn},
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
			snapshot: runstate.StatusSnapshot{
				Phase:           runstate.PhaseCompacting,
				RecoveryPaused:  true,
				ActiveOperation: "compact-operation",
				ActiveStructural: &runstate.StructuralOperationSnapshot{
					CommandID: "compact", OperationID: "compact-operation", Kind: runstate.StructuralCompactContext,
				},
				Queue: []runstate.QueuedInput{
					{CommandID: "queued-next", OperationID: "next-operation", Delivery: runstate.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryCompactContext, CommandID: "compact", OperationID: "compact-operation"},
				{Kind: RuntimeRecoveryAbort, OperationID: "compact-operation"},
			},
		},
		{
			name: "idle exposes only first queued next turn",
			snapshot: runstate.StatusSnapshot{
				Phase: runstate.PhaseIdle,
				Queue: []runstate.QueuedInput{
					{CommandID: "orphan-follow", OperationID: "old-operation", Delivery: runstate.DeliveryFollowUp},
					{CommandID: "next-first", OperationID: "next-operation-1", Delivery: runstate.DeliveryNextTurn},
					{CommandID: "next-second", OperationID: "next-operation-2", Delivery: runstate.DeliveryNextTurn},
				},
			},
			want: []RuntimeRecoveryAction{
				{Kind: RuntimeRecoveryNextTurn, CommandID: "next-first", OperationID: "next-operation-1"},
			},
		},
		{
			name: "idle terminal has no recovery action",
			snapshot: runstate.StatusSnapshot{
				Phase: runstate.PhaseIdle,
				LastOperation: &runstate.OperationSummary{
					CommandID: "done", OperationID: "done-operation", Status: runstate.OperationSucceeded,
				},
			},
			want: []RuntimeRecoveryAction{},
		},
		{
			name: "ordinary live running has no recovery action",
			snapshot: runstate.StatusSnapshot{
				Phase:           runstate.PhaseRunning,
				ActiveCommandID: "start",
				ActiveOperation: "live-operation",
				Queue: []runstate.QueuedInput{
					{CommandID: "queued-next", OperationID: "next-operation", Delivery: runstate.DeliveryNextTurn},
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

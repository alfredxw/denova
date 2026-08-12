package agent

import (
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestRecoveryCandidatesExposeOpaqueCurrentChoices(t *testing.T) {
	state := runstate.StateSnapshot{
		Phase: runstate.PhaseRunning, RecoveryPaused: true, ActiveOperation: "active-run",
		Queue: []runstate.QueuedInput{
			{CommandID: "follow-command", OperationID: "queued-run", Delivery: runstate.DeliveryFollowUp},
			{CommandID: "next-command", OperationID: "next-run", Delivery: runstate.DeliveryNextTurn},
		},
	}
	actions := publicRecoveryActions(recoveryCandidatesFromState(state))
	if len(actions) != 2 {
		t.Fatalf("actions=%#v", actions)
	}
	if actions[0].Kind != RecoveryAbortRun || actions[0].RunID != "active-run" ||
		actions[1].Kind != RecoveryResumeInput || actions[1].RunID != "queued-run" {
		t.Fatalf("actions=%#v", actions)
	}
	for _, action := range actions {
		if len(action.ID) != 32 || action.ID == "active-run" || action.ID == "follow-command" {
			t.Fatalf("recovery action is not opaque: %#v", action)
		}
	}
}

func TestRecoveryCandidatesPreferSelectedInputMaterialization(t *testing.T) {
	status := runstate.StatusSnapshot{
		Phase: runstate.PhaseRunning, RecoveryPaused: true, ActiveOperation: "active-run",
		InputRecovery: &runstate.InputMaterializationRecovery{
			CommandID: "selected", OperationID: "active-run", Delivery: runstate.DeliverySteer,
		},
		Queue: []runstate.QueuedInput{{CommandID: "later", OperationID: "later-run", Delivery: runstate.DeliveryNextTurn}},
	}
	actions := publicRecoveryActions(recoveryCandidatesFromStatus(status))
	if len(actions) != 2 || actions[0].Kind != RecoveryAbortRun || actions[1].Kind != RecoveryResumeInput || actions[1].RunID != "active-run" {
		t.Fatalf("actions=%#v", actions)
	}
}

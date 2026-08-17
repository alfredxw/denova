package handlers

import (
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/run"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAgentRuntimeProjectionDTOIsExplicitAndBounded(t *testing.T) {
	large := strings.Repeat("界", agentRuntimeProjectionTextMaxBytes/3+2)
	snapshot := agentrun.RuntimeStatus{
		Cursor:          42,
		Phase:           agentrun.RunPhaseRunning,
		ActiveOperation: "operation-1",
		ActiveCycle:     3,
		ActiveOutput: agentrun.ActiveOutput{
			OperationID: "operation-1",
			Cycle:       3,
			Content:     large,
			Thinking:    "推理",
		},
		Queue: []agentrun.QueuedCommand{{
			CommandID: "command-2", OperationID: "operation-1", Delivery: agentrun.DeliveryFollowUp,
			Message: "继续", SteerRequested: true,
		}},
		OpenToolCalls: []agentrun.OpenToolCall{{
			CallID: "call-1", Name: "read",
			OperationID: "operation-1", Cycle: 3,
		}},
		LastOperation: &agentrun.OperationSummary{
			OperationID: "operation-0", CommandID: "command-0",
			Status: agentrun.OperationInterrupted, Reason: large,
		},
	}

	dto := newAgentRuntimeProjectionDTO(snapshot)
	if dto.Cursor != 42 || dto.Phase != "running" || !dto.RecoveryPaused || dto.ActiveOperationID != "operation-1" || dto.ActiveCycle != 3 {
		t.Fatalf("projection identity = %#v", dto)
	}
	if !dto.ActiveOutput.ContentTruncated || len(dto.ActiveOutput.Content) > agentRuntimeProjectionTextMaxBytes || !utf8.ValidString(dto.ActiveOutput.Content) {
		t.Fatalf("bounded content bytes=%d truncated=%t valid_utf8=%t", len(dto.ActiveOutput.Content), dto.ActiveOutput.ContentTruncated, utf8.ValidString(dto.ActiveOutput.Content))
	}
	if len(dto.Queue) != 1 || dto.Queue[0].Message != "继续" || !dto.Queue[0].SteerRequested || len(dto.OpenTools) != 1 || dto.OpenTools[0].Name != "read" {
		t.Fatalf("queue/tools projection = %#v / %#v", dto.Queue, dto.OpenTools)
	}
	if dto.LastOperation == nil || dto.LastOperation.Status != "interrupted" || !dto.LastOperation.ReasonTruncated || !utf8.ValidString(dto.LastOperation.Reason) {
		t.Fatalf("last operation projection = %#v", dto.LastOperation)
	}
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "arguments") {
		t.Fatalf("internal tool arguments leaked through public DTO: %s", encoded)
	}
}

func TestAddAgentRuntimeProjectionKeepsLegacyResponseWhenUnavailable(t *testing.T) {
	response := map[string]interface{}{"active": false}
	addAgentRuntimeProjection(response, agentrun.RuntimeStatus{}, agentRuntimeProjectionOptions{})
	if len(response) != 1 || response["active"] != false {
		t.Fatalf("legacy response changed without durable projection: %#v", response)
	}
}

func TestAddAgentRuntimeProjectionDoesNotOfferAttachmentForAttachedStream(t *testing.T) {
	response := map[string]interface{}{"active": true, "task_id": "task-refresh"}
	action := agentexecution.RuntimeRecoveryAction{
		Kind: agentexecution.RuntimeRecoveryAttach, CommandID: "attach-refresh",
		OperationID: "operation-refresh",
	}
	addAgentRuntimeProjection(response, agentrun.RuntimeStatus{
		Phase: agentrun.RunPhaseIdle,
		LastOperation: &agentrun.OperationSummary{
			CommandID: action.CommandID, OperationID: action.OperationID,
			Status: agentrun.OperationSucceeded,
		},
	}, agentRuntimeProjectionOptions{
		Available: true, StreamAttached: true, RecoveryActions: []agentexecution.RuntimeRecoveryAction{action},
	})
	if response["phase"] != string(agentrun.RunPhaseIdle) || response["recovery_paused"] != false ||
		response["runtime_recoverable"] != false || response["stream_attached"] != true {
		t.Fatalf("attached projection flags = %#v", response)
	}
	actions, ok := response["recovery_actions"].([]agentRuntimeRecoveryActionDTO)
	if !ok || len(actions) != 0 {
		t.Fatalf("attached projection actions = %#v", response["recovery_actions"])
	}
}

func TestAddAgentRuntimeProjectionNormalizesServiceRefreshWithoutActorProjection(t *testing.T) {
	response := map[string]interface{}{}
	action := agentexecution.RuntimeRecoveryAction{
		Kind: agentexecution.RuntimeRecoveryAttach, CommandID: "attach-refresh",
		OperationID: "operation-refresh",
	}
	addAgentRuntimeProjection(response, agentrun.RuntimeStatus{}, agentRuntimeProjectionOptions{
		Available: true, RecoveryActions: []agentexecution.RuntimeRecoveryAction{action},
	})
	if response["phase"] != string(agentrun.RunPhaseIdle) || response["recovery_paused"] != true ||
		response["runtime_recoverable"] != true {
		t.Fatalf("service refresh projection = %#v", response)
	}
}

func TestRecoveryProjectionExposesOnlyLiveAttachWithoutInputPayload(t *testing.T) {
	snapshot := agentrun.RuntimeStatus{
		Phase:           agentrun.RunPhaseRunning,
		ActiveCommandID: "start-command",
		ActiveOperation: "operation-1",
		ActiveCycle:     1,
		Queue: []agentrun.QueuedCommand{
			{
				CommandID: "steer-command", OperationID: "operation-1", Delivery: agentrun.DeliverySteer,
				Message: "SECRET USER PAYLOAD",
			},
			{
				CommandID: "next-command", OperationID: "operation-2", Delivery: agentrun.DeliveryNextTurn,
				Message: "another secret",
			},
		},
	}
	dto := newAgentRuntimeProjectionDTO(snapshot)
	if !dto.RuntimeRecoverable || dto.StreamAttached || len(dto.RecoveryActions) != 1 ||
		dto.RecoveryActions[0].Kind != "start_turn" || dto.RecoveryActions[0].OperationID != "operation-1" {
		t.Fatalf("recovery projection = %#v", dto)
	}
	encoded, err := json.Marshal(dto.RecoveryActions)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET USER PAYLOAD", "SECRET TURN REF", "SECRET DESCRIPTOR", "secret", "input", "descriptor"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("recovery action leaked %q: %s", secret, encoded)
		}
	}
}

func TestRecoveryProjectionDoesNotResumeIdleWork(t *testing.T) {
	dto := newAgentRuntimeProjectionDTO(agentrun.RuntimeStatus{
		Phase: agentrun.RunPhaseIdle,
		LastOperation: &agentrun.OperationSummary{
			CommandID: "start", OperationID: "parent", Status: agentrun.OperationInterrupted,
		},
		Queue: []agentrun.QueuedCommand{{
			CommandID: "next", OperationID: "next-operation", Delivery: agentrun.DeliveryNextTurn,
		}},
	})
	if dto.RuntimeRecoverable || len(dto.RecoveryActions) != 0 {
		t.Fatalf("idle work unexpectedly recoverable: %#v", dto)
	}
}

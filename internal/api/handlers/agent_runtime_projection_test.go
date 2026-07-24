package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	agents "denova/internal/agents"
)

func TestAgentRuntimeProjectionDTOIsExplicitAndBounded(t *testing.T) {
	large := strings.Repeat("界", agentRuntimeProjectionTextMaxBytes/3+2)
	snapshot := agents.RuntimeStatus{
		Cursor:          42,
		Phase:           agents.RunPhaseRunning,
		RecoveryPaused:  true,
		ActiveOperation: "operation-1",
		ActiveCycle:     3,
		ActiveOutput: agents.ActiveOutput{
			OperationID: "operation-1",
			Cycle:       3,
			Content:     large,
			Thinking:    "推理",
		},
		Queue: []agents.QueuedCommand{{
			CommandID: "command-2", OperationID: "operation-1", Delivery: agents.DeliveryFollowUp,
			Message: "继续", SteerRequested: true,
		}},
		OpenToolCalls: []agents.OpenToolCall{{
			CallID: "call-1", Name: "read_file",
			OperationID: "operation-1", Cycle: 3,
		}},
		LastOperation: &agents.OperationSummary{
			OperationID: "operation-0", CommandID: "command-0",
			Status: agents.OperationInterrupted, Reason: large,
		},
	}

	dto := newAgentRuntimeProjectionDTO(snapshot)
	if dto.Cursor != 42 || dto.Phase != "running" || !dto.RecoveryPaused || dto.ActiveOperationID != "operation-1" || dto.ActiveCycle != 3 {
		t.Fatalf("projection identity = %#v", dto)
	}
	if !dto.ActiveOutput.ContentTruncated || len(dto.ActiveOutput.Content) > agentRuntimeProjectionTextMaxBytes || !utf8.ValidString(dto.ActiveOutput.Content) {
		t.Fatalf("bounded content bytes=%d truncated=%t valid_utf8=%t", len(dto.ActiveOutput.Content), dto.ActiveOutput.ContentTruncated, utf8.ValidString(dto.ActiveOutput.Content))
	}
	if len(dto.Queue) != 1 || dto.Queue[0].Message != "继续" || !dto.Queue[0].SteerRequested || len(dto.OpenTools) != 1 || dto.OpenTools[0].Name != "read_file" {
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
	addAgentRuntimeProjection(response, agents.RuntimeStatus{}, agentRuntimeProjectionOptions{})
	if len(response) != 1 || response["active"] != false {
		t.Fatalf("legacy response changed without durable projection: %#v", response)
	}
}

func TestAddAgentRuntimeProjectionExposesServiceRefreshActionAfterActorIdle(t *testing.T) {
	response := map[string]interface{}{"active": true, "task_id": "task-refresh"}
	action := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "compact-refresh",
		OperationID: "operation-refresh",
	}
	addAgentRuntimeProjection(response, agents.RuntimeStatus{
		Phase: agents.RunPhaseIdle,
		LastOperation: &agents.OperationSummary{
			CommandID: action.CommandID, OperationID: action.OperationID,
			Status: agents.OperationSucceeded,
		},
	}, agentRuntimeProjectionOptions{
		Available: true, StreamAttached: true, RecoveryActions: []agents.RuntimeRecoveryAction{action},
	})
	if response["phase"] != string(agents.RunPhaseIdle) || response["recovery_paused"] != true ||
		response["runtime_recoverable"] != true || response["stream_attached"] != true {
		t.Fatalf("service refresh projection flags = %#v", response)
	}
	actions, ok := response["recovery_actions"].([]agentRuntimeRecoveryActionDTO)
	if !ok || len(actions) != 1 || actions[0] != (agentRuntimeRecoveryActionDTO{
		Kind: string(action.Kind), CommandID: string(action.CommandID), OperationID: string(action.OperationID),
	}) {
		t.Fatalf("service refresh projection actions = %#v", response["recovery_actions"])
	}
}

func TestAddAgentRuntimeProjectionNormalizesServiceRefreshWithoutActorProjection(t *testing.T) {
	response := map[string]interface{}{}
	action := agents.RuntimeRecoveryAction{
		Kind: agents.RuntimeRecoveryCompactContext, CommandID: "compact-refresh",
		OperationID: "operation-refresh",
	}
	addAgentRuntimeProjection(response, agents.RuntimeStatus{}, agentRuntimeProjectionOptions{
		Available: true, RecoveryActions: []agents.RuntimeRecoveryAction{action},
	})
	if response["phase"] != string(agents.RunPhaseIdle) || response["recovery_paused"] != true ||
		response["runtime_recoverable"] != true {
		t.Fatalf("service refresh projection = %#v", response)
	}
}

func TestRecoveryProjectionIsOrderedBoundedAndDoesNotLeakDurablePayload(t *testing.T) {
	snapshot := agents.RuntimeStatus{
		Phase:           agents.RunPhaseRunning,
		RecoveryPaused:  true,
		ActiveCommandID: "start-command",
		ActiveOperation: "operation-1",
		ActiveCycle:     1,
		Queue: []agents.QueuedCommand{
			{
				CommandID: "steer-command", OperationID: "operation-1", Delivery: agents.DeliverySteer,
				Message: "SECRET USER PAYLOAD",
			},
			{
				CommandID: "next-command", OperationID: "operation-2", Delivery: agents.DeliveryNextTurn,
				Message: "another secret",
			},
		},
	}
	dto := newAgentRuntimeProjectionDTO(snapshot)
	if !dto.RuntimeRecoverable || dto.StreamAttached || len(dto.RecoveryActions) != 3 {
		t.Fatalf("recovery projection = %#v", dto)
	}
	wantKinds := []string{"start_turn", "abort", "steer"}
	for index, want := range wantKinds {
		if dto.RecoveryActions[index].Kind != want {
			t.Fatalf("action order = %#v", dto.RecoveryActions)
		}
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

func TestRecoveryProjectionExposesOnlyFirstIdleNextTurn(t *testing.T) {
	snapshot := agents.RuntimeStatus{
		Phase: agents.RunPhaseIdle,
		LastOperation: &agents.OperationSummary{
			CommandID: "start", OperationID: "parent", Status: agents.OperationInterrupted,
			Reason: "runtime recovered an unfinished operation",
		},
	}
	for index := 0; index < 64; index++ {
		snapshot.Queue = append(snapshot.Queue, agents.QueuedCommand{
			CommandID:   agents.CommandID(fmt.Sprintf("next-%d", index)),
			OperationID: agents.OperationID(fmt.Sprintf("operation-%d", index)),
			Delivery:    agents.DeliveryNextTurn,
		})
	}
	dto := newAgentRuntimeProjectionDTO(snapshot)
	if len(dto.RecoveryActions) != 1 || dto.RecoveryActions[0] != (agentRuntimeRecoveryActionDTO{
		Kind: "next_turn", CommandID: "next-0", OperationID: "operation-0",
	}) {
		t.Fatalf("recovery actions = %#v", dto.RecoveryActions)
	}
}

func TestRecoveryProjectionStrictStateBoundaries(t *testing.T) {
	tests := []struct {
		name         string
		snapshot     agents.RuntimeStatus
		wantKinds    []string
		wantCommands []string
	}{
		{
			name: "input recovery hides later queue",
			snapshot: agents.RuntimeStatus{
				Phase:           agents.RunPhaseRunning,
				RecoveryPaused:  true,
				ActiveCommandID: "start",
				ActiveOperation: "parent",
				InputRecovery: &agents.InputRecovery{
					CommandID: "recover-next", OperationID: "next-operation", Delivery: agents.DeliveryNextTurn,
				},
				Queue: []agents.QueuedCommand{
					{CommandID: "later-steer", OperationID: "parent", Delivery: agents.DeliverySteer},
				},
			},
			wantKinds:    []string{"start_turn", "abort", "next_turn"},
			wantCommands: []string{"start", "", "recover-next"},
		},
		{
			name: "paused compacting exposes no queue action",
			snapshot: agents.RuntimeStatus{
				Phase:           agents.RunPhaseCompacting,
				RecoveryPaused:  true,
				ActiveOperation: "compact-operation",
				ActiveStructural: &agents.StructuralOperation{
					CommandID: "compact", OperationID: "compact-operation", Kind: agents.StructuralCompactContext,
				},
				Queue: []agents.QueuedCommand{
					{CommandID: "later-next", OperationID: "next-operation", Delivery: agents.DeliveryNextTurn},
				},
			},
			wantKinds:    []string{"compact_context", "abort"},
			wantCommands: []string{"compact", ""},
		},
		{
			name: "idle terminal is not recoverable",
			snapshot: agents.RuntimeStatus{
				Phase: agents.RunPhaseIdle,
				LastOperation: &agents.OperationSummary{
					CommandID: "done", OperationID: "done-operation", Status: agents.OperationSucceeded,
				},
			},
		},
		{
			name: "ordinary live running is not recoverable",
			snapshot: agents.RuntimeStatus{
				Phase:           agents.RunPhaseRunning,
				ActiveCommandID: "start",
				ActiveOperation: "live-operation",
				Queue: []agents.QueuedCommand{
					{CommandID: "queued-next", OperationID: "next-operation", Delivery: agents.DeliveryNextTurn},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dto := newAgentRuntimeProjectionDTO(test.snapshot)
			if dto.RuntimeRecoverable != (len(test.wantKinds) > 0) || len(dto.RecoveryActions) != len(test.wantKinds) {
				t.Fatalf("recovery projection = %#v", dto)
			}
			for index, wantKind := range test.wantKinds {
				action := dto.RecoveryActions[index]
				if action.Kind != wantKind {
					t.Fatalf("action[%d] = %#v, want kind %q", index, action, wantKind)
				}
				wantCommand := test.wantCommands[index]
				if wantCommand == "" {
					if action.CommandID == "" {
						t.Fatalf("action[%d] has empty server-derived command identity: %#v", index, action)
					}
					continue
				}
				if action.CommandID != wantCommand {
					t.Fatalf("action[%d] = %#v, want command %q", index, action, wantCommand)
				}
			}
		})
	}
}

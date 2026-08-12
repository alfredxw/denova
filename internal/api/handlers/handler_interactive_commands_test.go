package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	appsvc "denova/internal/app"
)

func TestInteractiveAgentCommandKindIsClosed(t *testing.T) {
	want := map[string]appsvc.CommandKind{
		"abort":         appsvc.CommandAbort,
		"follow_up":     appsvc.CommandFollowUp,
		"steer_queued":  appsvc.CommandSteerQueued,
		"cancel_queued": appsvc.CommandCancelQueued,
	}
	for value, expected := range want {
		kind, err := interactiveAgentCommandKind(value)
		if err != nil || kind != expected {
			t.Fatalf("kind(%q) = %q, %v, want %q", value, kind, err, expected)
		}
	}
	for _, value := range []string{"steer", "next_turn", "unknown"} {
		if kind, err := interactiveAgentCommandKind(value); err == nil {
			t.Fatalf("kind(%q) = %q, want error", value, kind)
		}
	}
}

func TestAgentCommandErrorMapsDomainCommitWinnerToConflict(t *testing.T) {
	requestContext := app.NewContext(0)
	(&Handlers{}).writeAgentCommandError(requestContext, appsvc.ErrAgentDomainCommitRejected, "operation-commit")

	if got := requestContext.Response.StatusCode(); got != http.StatusConflict {
		t.Fatalf("status = %d, want %d", got, http.StatusConflict)
	}
	var response agentRuntimeErrorResponse
	if err := json.Unmarshal(requestContext.Response.Body(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "agent_runtime.commit_won" || response.Error == "" {
		t.Fatalf("runtime error = %#v", response)
	}
	if response.Details["target_operation_id"] != "operation-commit" {
		t.Fatalf("runtime error details = %#v", response.Details)
	}
}

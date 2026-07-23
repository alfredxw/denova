package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	runstate "denova/internal/agent/runtime"
)

func TestInteractiveAgentCommandKindIsClosed(t *testing.T) {
	for _, value := range []string{"steer", "follow_up", "next_turn", "abort"} {
		kind, err := interactiveAgentCommandKind(value)
		if err != nil || string(kind) != value {
			t.Fatalf("kind(%q) = %q, %v", value, kind, err)
		}
	}
}

func TestAgentCommandErrorMapsDomainCommitWinnerToConflict(t *testing.T) {
	requestContext := app.NewContext(0)
	(&Handlers{}).writeAgentCommandError(requestContext, runstate.ErrDomainCommitRejected, "operation-commit")

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

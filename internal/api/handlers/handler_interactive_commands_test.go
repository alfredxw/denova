package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
	"github.com/cloudwego/hertz/pkg/app"

	appsvc "denova/internal/app"
)

func TestInteractiveAgentCommandKindIsClosed(t *testing.T) {
	kind, err := interactiveAgentCommandKind("abort")
	if err != nil || kind != appsvc.AgentCommandAbort {
		t.Fatalf("abort kind = %q, %v", kind, err)
	}
	for _, value := range []string{"steer", "follow_up", "next_turn", "steer_queued", "cancel_queued"} {
		if kind, err := interactiveAgentCommandKind(value); err == nil {
			t.Fatalf("kind(%q) = %q, want error", value, kind)
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

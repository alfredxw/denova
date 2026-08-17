package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	appsvc "denova/internal/app"
)

func TestAskResolutionErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name: "unknown id", err: appsvc.ErrAgentAskNotFound, wantStatus: http.StatusNotFound,
			wantCode: "agent_runtime.ask_not_found", wantMessage: "Ask interaction not found",
		},
		{
			name: "no workspace", err: appsvc.ErrNoWorkspace, wantStatus: http.StatusConflict,
			wantCode: "agent_runtime.no_workspace", wantMessage: "No workspace is open",
		},
		{
			name: "invalid answer", err: errors.New("invalid answer"), wantStatus: http.StatusBadRequest,
			wantCode: "agent_runtime.invalid_ask_answer", wantMessage: "Invalid ask answer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestContext := app.NewContext(0)
			writeAskResolutionError(requestContext, test.err)
			if got := requestContext.Response.StatusCode(); got != test.wantStatus {
				t.Fatalf("status = %d, want %d", got, test.wantStatus)
			}
			var response agentRuntimeErrorResponse
			if err := json.Unmarshal(requestContext.Response.Body(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != test.wantCode || response.Error != test.wantMessage {
				t.Fatalf("response = %#v, want code %q and error %q", response, test.wantCode, test.wantMessage)
			}
		})
	}
}

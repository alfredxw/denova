package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestAskResolutionDiagnosticsDistinguishInternalFailure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"definition", fmt.Errorf("approval validation: %w", agent.ErrDefinitionMismatch), 409, "agent_runtime.definition_mismatch"},
		{"invalid answer", fmt.Errorf("unknown approval choice: %w", agent.ErrInvalidInteractionResponse), 400, "agent_runtime.invalid_ask_answer"},
		{"unexpected", errors.New("persist approval: disk full"), 500, "agent_runtime.ask_failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", "zh-CN")
			writeAskResolutionError(context.Background(), c, tt.err)
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if c.Response.StatusCode() != tt.status || body.Code != tt.code || body.Details["detail"] == "" {
				t.Fatalf("status=%d body=%+v", c.Response.StatusCode(), body)
			}
		})
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"denova/internal/agents/conversationconfig"
	appsvc "denova/internal/app"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestAgentCommandUnsupportedCapabilityIsLocalizedClientError(t *testing.T) {
	for locale, want := range map[string]string{
		"zh-CN": "当前引擎不支持此操作或参数。",
		"en-US": "This runtime does not support the requested operation or parameters.",
	} {
		t.Run(locale, func(t *testing.T) {
			c := app.NewContext(0)
			c.Request.Header.Set("X-Denova-Locale", locale)
			h := &Handlers{}
			h.writeAgentCommandError(context.Background(), c, fmt.Errorf("suspend: %w", conversationconfig.ErrRuntimeCapabilityUnsupported), "external-test")
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if c.Response.StatusCode() != 400 || body.Code != "agent_runtime.capability_unsupported" || body.Error != want || body.Details["target_operation_id"] != "external-test" {
				t.Fatalf("unexpected response: %d %s", c.Response.StatusCode(), c.Response.Body())
			}
		})
	}
}

func TestWritingAgentCommandKindIncludesQueueControls(t *testing.T) {
	t.Parallel()

	tests := map[string]appsvc.CommandKind{
		"steer":         appsvc.CommandSteer,
		"follow_up":     appsvc.CommandFollowUp,
		"next_turn":     appsvc.CommandNextTurn,
		"abort":         appsvc.CommandAbort,
		"steer_queued":  appsvc.CommandSteerQueued,
		"cancel_queued": appsvc.CommandCancelQueued,
	}
	for input, want := range tests {
		input, want := input, want
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := writingAgentCommandKind(input)
			if err != nil || got != want {
				t.Fatalf("writingAgentCommandKind(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
	if got, err := writingAgentCommandKind("queue"); err == nil {
		t.Fatalf("writingAgentCommandKind(queue) = %q, want error", got)
	}
}

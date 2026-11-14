package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	novaApp "denova/internal/app"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func TestChatPreparationPausedTurnIsLocalizedClientError(t *testing.T) {
	for locale, want := range map[string]string{
		"zh-CN": "上一轮已暂停，请继续生成或放弃该轮后再发送",
		"en-US": "The previous turn is paused; resume or discard it before sending",
	} {
		t.Run(locale, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.Set("X-Denova-Locale", locale)
			handler := &Handlers{}
			handler.writeChatPreparationError(request, fmt.Errorf("suspend: %w", novaApp.ErrAgentOperationPaused))
			var body agentRuntimeErrorResponse
			if err := json.Unmarshal(request.Response.Body(), &body); err != nil {
				t.Fatal(err)
			}
			if request.Response.StatusCode() != consts.StatusConflict || body.Code != "agent_runtime.operation_paused" || body.Error != want {
				t.Fatalf("paused rejection status=%d body=%+v", request.Response.StatusCode(), request.Response.Body())
			}
		})
	}
}

func TestChatPreparationPausedTurnDefaultsToChineseWithoutLocale(t *testing.T) {
	request := app.NewContext(0)
	handler := &Handlers{}
	handler.writeChatPreparationError(request, fmt.Errorf("suspend: %w", novaApp.ErrAgentOperationPaused))
	var body agentRuntimeErrorResponse
	if err := json.Unmarshal(request.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	want := "上一轮已暂停，请继续生成或放弃该轮后再发送"
	if request.Response.StatusCode() != consts.StatusConflict || body.Code != "agent_runtime.operation_paused" || body.Error != want {
		t.Fatalf("paused rejection status=%d body=%+v", request.Response.StatusCode(), request.Response.Body())
	}
}
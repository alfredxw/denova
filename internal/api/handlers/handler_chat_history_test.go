package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestChatPreparationRejectsInvalidHistoryLocally(t *testing.T) {
	owner, err := agent.New(context.Background(), agent.Definition{Name: "history-test", Model: historyErrorModel{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("broken"))
	if err != nil {
		t.Fatal(err)
	}
	err = session.LoadCanonicalMessages(context.Background(), []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "missing", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		agent.UserMessage("continue"),
	})
	if err == nil {
		t.Fatal("incomplete historical tool batch was accepted")
	}
	for locale, want := range map[string]string{
		"zh-CN": "当前会话或故事分支的历史记录异常，暂时无法继续。原始记录已保留，你仍可使用其他会话或分支。",
		"en-US": "This conversation or story branch cannot continue because its history is invalid. The original records are preserved. Other conversations and branches remain available.",
	} {
		t.Run(locale, func(t *testing.T) {
			request := app.NewContext(0)
			request.Request.Header.Set("X-Denova-Locale", locale)
			(&Handlers{}).writeChatPreparationError(request, fmt.Errorf("prepare conversation: %w", err))
			var body agentRuntimeErrorResponse
			if decodeErr := json.Unmarshal(request.Response.Body(), &body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if request.Response.StatusCode() != 409 || body.Code != "agent_runtime.invalid_history" || body.Error != want {
				t.Fatalf("history rejection status=%d body=%+v", request.Response.StatusCode(), body)
			}
		})
	}
}

type historyErrorModel struct{}

func (historyErrorModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return nil, fmt.Errorf("history validation must not call the model")
}

func (historyErrorModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return nil, fmt.Errorf("history validation must not call the model")
}

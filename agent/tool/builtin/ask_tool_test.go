package builtin

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alfredxw/denova/agent"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentinteraction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestAskDerivesFreeTextAndRetriesInvalidChoicesThroughAgent(t *testing.T) {
	for _, test := range []struct {
		name         string
		recommendedA bool
		recommendedB bool
	}{
		{name: "no recommendation"},
		{name: "one recommendation", recommendedA: true},
		{name: "two recommendations", recommendedA: true, recommendedB: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			call := func(id, arguments string) *agentschema.Message {
				return agentschema.AssistantMessage("", []agentschema.ToolCall{{ID: id, Type: "function", Function: agentschema.FunctionCall{Name: "ask", Arguments: arguments}}})
			}
			owner, err := agent.New(ctx, agent.Definition{
				Name: "ask-schema", ModelIdentity: agentschema.CapabilityIdentity{Kind: "test.ask.schema", Version: 1}, Tools: Ask(),
				Model: &taskModel{responses: []*agentschema.Message{
					call("invalid", `{"questions":[{"id":"choice","prompt":"Pick one","options":[{"value":"a","label":"A"}]}]}`),
					call("valid", fmt.Sprintf(`{"questions":[{"id":"text","prompt":"Describe the scope"},{"id":"choice","prompt":"Pick one","options":[{"value":"a","label":"A","recommended":%t},{"value":"b","label":"B","recommended":%t}]}]}`, test.recommendedA, test.recommendedB)),
					agentschema.AssistantMessage("done", nil),
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close(context.Background())
			run, err := owner.Run(ctx, agent.Text("ask"))
			if err != nil {
				t.Fatal(err)
			}
			interactions, failures := 0, 0
			for event := range run.Events() {
				switch payload := event.Payload.(type) {
				case agentevent.ToolFinished:
					if payload.IsError {
						failures++
						if !strings.Contains(payload.Result, "two to four options") {
							t.Errorf("unhelpful validation feedback: %s", payload.Result)
						}
					}
				case agentevent.InteractionRequested:
					interactions++
					questions := payload.Request.Questions
					if len(questions) != 2 || !questions[0].AllowFreeText || questions[1].AllowFreeText || !payload.Request.AllowOther {
						t.Fatalf("host question types = %#v", payload.Request)
					}
					if len(questions[1].Options) != 2 || questions[1].Options[0].Recommended != test.recommendedA || questions[1].Options[1].Recommended != test.recommendedB {
						t.Fatalf("recommendations changed: %#v", questions[1].Options)
					}
					if err := run.Respond(ctx, payload.Request.ID, agentinteraction.InteractionResponse{Answers: []agentinteraction.InteractionAnswer{
						{QuestionID: "text", Text: "whole project"}, {QuestionID: "choice", Values: []string{"a"}},
					}}); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := run.Wait(ctx)
			if err != nil || result.Status != agentschema.ResultCompleted || interactions != 1 || failures != 1 {
				t.Fatalf("result=%#v interactions=%d failures=%d error=%v", result, interactions, failures, err)
			}
		})
	}
}

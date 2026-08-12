package agent

import (
	"context"
	"strings"
	"testing"
)

func validChoiceInteraction() InteractionRequest {
	return InteractionRequest{
		ID: "ask-choice", Kind: InteractionAsk, AllowOther: true,
		Questions: []InteractionQuestion{{
			ID: "scope", Prompt: LocalizedText{Chinese: "选择范围", English: "Choose scope"},
			Options: []InteractionOption{
				{Value: "small", Label: LocalizedText{Chinese: "小", English: "Small"}, Recommended: true},
				{Value: "large", Label: LocalizedText{Chinese: "大", English: "Large"}},
			},
		}},
	}
}

func TestStandardAskInteractionPreservesBoundedChoiceAndOtherContract(t *testing.T) {
	policy := StandardInteraction()
	request := validChoiceInteraction()
	if err := policy.ValidateRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	resolved, err := policy.Resolve(context.Background(), request, InteractionResponse{Answers: []InteractionAnswer{{
		QuestionID: "scope", Text: "Use a staged rollout",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Answers) != 1 || resolved.Answers[0].Text != "Use a staged rollout" {
		t.Fatalf("Other resolution=%#v", resolved)
	}

	tests := []struct {
		name   string
		mutate func(*InteractionRequest)
		want   string
	}{
		{name: "one option", mutate: func(value *InteractionRequest) { value.Questions[0].Options = value.Questions[0].Options[:1] }, want: "two to three"},
		{name: "reserved other", mutate: func(value *InteractionRequest) { value.Questions[0].Options[0].Value = "other" }, want: "reserved"},
		{name: "no recommendation", mutate: func(value *InteractionRequest) { value.Questions[0].Options[0].Recommended = false }, want: "exactly one"},
		{name: "two recommendations", mutate: func(value *InteractionRequest) { value.Questions[0].Options[1].Recommended = true }, want: "exactly one"},
		{name: "oversized id", mutate: func(value *InteractionRequest) {
			value.Questions[0].ID = strings.Repeat("x", maxInteractionStableIDBytes+1)
		}, want: "1..256"},
		{name: "oversized prompt", mutate: func(value *InteractionRequest) {
			value.Questions[0].Prompt.English = strings.Repeat("x", maxInteractionQuestionTextBytes)
		}, want: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validChoiceInteraction()
			test.mutate(&value)
			err := policy.ValidateRequest(context.Background(), value)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestStandardAskInteractionBoundsCustomResponseAndCancellation(t *testing.T) {
	policy := StandardInteraction()
	request := validChoiceInteraction()
	_, err := policy.Resolve(context.Background(), request, InteractionResponse{Answers: []InteractionAnswer{{
		QuestionID: "scope", Text: strings.Repeat("x", maxInteractionAnswerTextBytes+1),
	}}})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error=%v", err)
	}
	resolved, err := policy.Resolve(context.Background(), request, InteractionResponse{Cancelled: true})
	if err != nil || !resolved.Cancelled || len(resolved.Answers) != 0 {
		t.Fatalf("cancelled resolution=%#v error=%v", resolved, err)
	}
}

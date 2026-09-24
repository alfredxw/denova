package externaljournal

import (
	"context"
	"encoding/json"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// QuestionRequest interprets the persisted host Ask arguments with the public
// validator. Both execution and history derive the same request without a live
// engine or a second durable interaction record.
func QuestionRequest(executionID string, arguments json.RawMessage) (agent.InteractionRequest, error) {
	if len(arguments) > 128<<10 {
		return agent.InteractionRequest{}, errors.New("question input exceeds 128 KiB")
	}
	var input struct {
		Questions []agent.InteractionQuestion `json:"questions"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return agent.InteractionRequest{}, err
	}
	for index := range input.Questions {
		input.Questions[index].AllowFreeText = len(input.Questions[index].Options) == 0
	}
	request := agent.InteractionRequest{ID: "ask-" + executionID, Kind: agent.InteractionAsk, Questions: input.Questions, AllowOther: true}
	return request, agent.StandardInteraction().ValidateRequest(context.Background(), request)
}

func validateQuestionResult(finished FinishedTool) error {
	if len(finished.Result) > 256<<10 {
		return errors.New("question result exceeds 256 KiB")
	}
	var result struct {
		Schema string `json:"schema"`
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(finished.Result), &result); err != nil {
		return err
	}
	if result.Schema != "ask.result.v1" || result.ID != "ask-"+finished.ExecutionID || (result.Status != "answered" && result.Status != "cancelled") {
		return errors.New("invalid canonical external question result")
	}
	return nil
}

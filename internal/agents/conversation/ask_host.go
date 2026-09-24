package conversation

import (
	"strings"

	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// HostAskAnswer is the transport-neutral answer accepted from an interactive
// host. Public Agent Interaction validation and durable resolution remain the
// only authority behind this transport DTO.
type HostAskAnswer struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	CustomInput       string   `json:"custom_input,omitempty"`
}

type HostAskSelectedOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type HostAskAnswerResult struct {
	QuestionID      string                  `json:"question_id"`
	Question        string                  `json:"question"`
	SelectedOptions []HostAskSelectedOption `json:"selected_options,omitempty"`
	CustomInput     string                  `json:"custom_input,omitempty"`
}

// HostAskResolution is the stable answer/cancellation result exposed to any
// application host without leaking Session persistence types.
type HostAskResolution struct {
	Schema       string                `json:"schema"`
	ID           string                `json:"id"`
	Status       string                `json:"status"`
	Answers      []HostAskAnswerResult `json:"answers,omitempty"`
	CancelReason string                `json:"cancel_reason,omitempty"`
}

// InteractionAnswers converts host choices to the public validator's input.
// The UI's Other marker is represented by free text, never an option value.
func InteractionAnswers(answers []HostAskAnswer) []agent.InteractionAnswer {
	result := make([]agent.InteractionAnswer, len(answers))
	for index, answer := range answers {
		values := make([]string, 0, len(answer.SelectedOptionIDs))
		for _, value := range answer.SelectedOptionIDs {
			if value = strings.TrimSpace(value); value != "" && value != "other" {
				values = append(values, value)
			}
		}
		result[index] = agent.InteractionAnswer{QuestionID: answer.QuestionID, Values: values, Text: answer.CustomInput}
	}
	return result
}

// ProjectAskResolution formats an already validated resolution for the host.
// The caller owns durable acceptance and any permission/verification effects.
func ProjectAskResolution(request agent.InteractionRequest, resolution agent.InteractionResolution, cancelReason string) HostAskResolution {
	result := HostAskResolution{Schema: "ask.result.v1", ID: request.ID, Status: session.AskAnswered}
	if resolution.Cancelled {
		result.Status, result.CancelReason = session.AskCancelled, strings.TrimSpace(cancelReason)
		return result
	}
	for _, answer := range resolution.Answers {
		for _, question := range request.Questions {
			if question.ID != answer.QuestionID {
				continue
			}
			value := HostAskAnswerResult{QuestionID: answer.QuestionID, Question: strings.TrimSpace(question.Prompt), CustomInput: answer.Text}
			for _, selected := range answer.Values {
				for _, option := range question.Options {
					if option.Value == selected {
						value.SelectedOptions = append(value.SelectedOptions, HostAskSelectedOption{ID: option.Value, Label: strings.TrimSpace(option.Label)})
					}
				}
			}
			result.Answers = append(result.Answers, value)
		}
	}
	return result
}

package conversation

import (
	"context"
	"errors"
	"fmt"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

var ErrAskNotFound = errors.New("agent ask was not found")

// HostAskAnswer is the transport-neutral answer accepted from an interactive
// host. Session validation and durable resolution remain inside the Agent
// conversation boundary.
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

type askSession interface {
	ResolveAskFromHost(context.Context, string, string, []session.AskAnswer, string) (session.AskResolution, error)
}

func ResolveAsk(ctx context.Context, target askSession, askID, status string, answers []HostAskAnswer, cancelReason string) (HostAskResolution, error) {
	sessionAnswers := make([]session.AskAnswer, len(answers))
	for index, answer := range answers {
		sessionAnswers[index] = session.AskAnswer{
			QuestionID: answer.QuestionID, SelectedOptionIDs: append([]string(nil), answer.SelectedOptionIDs...), CustomInput: answer.CustomInput,
		}
	}
	resolution, err := target.ResolveAskFromHost(ctx, askID, status, sessionAnswers, cancelReason)
	if err != nil {
		if errors.Is(err, session.ErrAskNotFound) {
			return HostAskResolution{}, fmt.Errorf("%w: %v", ErrAskNotFound, err)
		}
		return HostAskResolution{}, err
	}
	return hostAskResolutionFromSession(resolution), nil
}

// ReconcileColdPendingAsk closes only an orphaned Ask owned by the exact
// durable cycle exposed by runtime recovery. Session performs the waiter check
// and journal transition under its canonical mutation lease.
func ReconcileColdPendingAsk(ctx context.Context, target *session.Session, runtime agentrun.RuntimeStatus) (bool, error) {
	if target == nil || (!runtime.RecoveryPaused && !runtime.RecoveryPending) || runtime.ActiveOperation == "" || runtime.ActiveCycle <= 0 {
		return false, nil
	}
	return target.ReconcileStalePendingAsk(ctx, session.AskCycleIdentity{
		CommandID:   string(runtime.ActiveCommandID),
		OperationID: string(runtime.ActiveOperation),
		Cycle:       runtime.ActiveCycle,
	})
}

func hostAskResolutionFromSession(resolution session.AskResolution) HostAskResolution {
	result := HostAskResolution{
		Schema: resolution.Schema, ID: resolution.ID, Status: resolution.Status, CancelReason: resolution.CancelReason,
		Answers: make([]HostAskAnswerResult, len(resolution.Answers)),
	}
	for answerIndex, answer := range resolution.Answers {
		converted := HostAskAnswerResult{
			QuestionID: answer.QuestionID, Question: answer.Question, CustomInput: answer.CustomInput,
			SelectedOptions: make([]HostAskSelectedOption, len(answer.SelectedOptions)),
		}
		for optionIndex, option := range answer.SelectedOptions {
			converted.SelectedOptions[optionIndex] = HostAskSelectedOption{ID: option.ID, Label: option.Label}
		}
		result.Answers[answerIndex] = converted
	}
	return result
}

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/internal/agents/session"
)

var ErrAgentAskNotPending = errors.New("agent ask is not pending")

// AgentAskAnswer is the App boundary accepted from interactive HTTP hosts.
// Session validation remains internal to the Agent subsystem.
type AgentAskAnswer struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	CustomInput       string   `json:"custom_input,omitempty"`
}

type AgentAskSelectedOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AgentAskAnswerResult struct {
	QuestionID      string                   `json:"question_id"`
	Question        string                   `json:"question"`
	SelectedOptions []AgentAskSelectedOption `json:"selected_options,omitempty"`
	CustomInput     string                   `json:"custom_input,omitempty"`
}

// AgentAskResolution is the stable App/API result for an answered or
// cancelled interaction. It deliberately does not expose Session types.
type AgentAskResolution struct {
	Schema       string                 `json:"schema"`
	ID           string                 `json:"id"`
	Status       string                 `json:"status"`
	Answers      []AgentAskAnswerResult `json:"answers,omitempty"`
	CancelReason string                 `json:"cancel_reason,omitempty"`
}

// AnswerSessionAsk answers the exact pending ask in a user IDE session. The
// blocked tool call remains inside the same durable Agent task.
func (a *App) AnswerSessionAsk(ctx context.Context, sessionID, askID string, answers []AgentAskAnswer) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskAnswered, answers, "")
}

func (a *App) CancelSessionAsk(ctx context.Context, sessionID, askID, reason string) (AgentAskResolution, error) {
	return a.resolveSessionAsk(ctx, sessionID, askID, session.AskCancelled, nil, reason)
}

func (a *App) resolveSessionAsk(ctx context.Context, sessionID, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	if a == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	a.mu.RLock()
	store := a.sessionStore
	selected := a.session
	a.mu.RUnlock()
	if store == nil || selected == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = selected.ID
	}
	if isAgentSessionID(sessionID) {
		return AgentAskResolution{}, fmt.Errorf("cannot resolve a fixed Agent ask through the IDE session endpoint: %s", sessionID)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return AgentAskResolution{}, err
	}
	return resolveAgentAsk(ctx, sess, askID, status, answers, cancelReason)
}

// AnswerConfigManagerAsk and CancelConfigManagerAsk use the same scoped session identity as Config
// Manager history and runtime recovery, so two open settings surfaces cannot
// answer each other's pending question.
func (a *App) AnswerConfigManagerAsk(ctx context.Context, scope ConfigManagerRequest, askID string, answers []AgentAskAnswer) (AgentAskResolution, error) {
	return a.resolveConfigManagerAsk(ctx, scope, askID, session.AskAnswered, answers, "")
}

func (a *App) CancelConfigManagerAsk(ctx context.Context, scope ConfigManagerRequest, askID, reason string) (AgentAskResolution, error) {
	return a.resolveConfigManagerAsk(ctx, scope, askID, session.AskCancelled, nil, reason)
}

func (a *App) resolveConfigManagerAsk(ctx context.Context, scope ConfigManagerRequest, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	store := a.configManager().sessionStore()
	if store == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	sessionID, err := configManagerSessionID(scope)
	if err != nil {
		return AgentAskResolution{}, err
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return AgentAskResolution{}, err
	}
	return resolveAgentAsk(ctx, sess, askID, status, answers, cancelReason)
}

type askSession interface {
	ResolveAskFromHost(context.Context, string, string, []session.AskAnswer, string) (session.AskResolution, error)
}

func resolveAgentAsk(ctx context.Context, sess askSession, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	sessionAnswers := make([]session.AskAnswer, len(answers))
	for index, answer := range answers {
		sessionAnswers[index] = session.AskAnswer{
			QuestionID: answer.QuestionID, SelectedOptionIDs: append([]string(nil), answer.SelectedOptionIDs...), CustomInput: answer.CustomInput,
		}
	}
	resolution, err := sess.ResolveAskFromHost(ctx, askID, status, sessionAnswers, cancelReason)
	if err != nil {
		if errors.Is(err, session.ErrAskNotPending) || errors.Is(err, session.ErrAskAlreadyResolved) || errors.Is(err, session.ErrAskContinuationUnavailable) {
			return AgentAskResolution{}, fmt.Errorf("%w: %v", ErrAgentAskNotPending, err)
		}
		return AgentAskResolution{}, err
	}
	return agentAskResolutionFromSession(resolution), nil
}

func agentAskResolutionFromSession(resolution session.AskResolution) AgentAskResolution {
	result := AgentAskResolution{
		Schema: resolution.Schema, ID: resolution.ID, Status: resolution.Status, CancelReason: resolution.CancelReason,
		Answers: make([]AgentAskAnswerResult, len(resolution.Answers)),
	}
	for answerIndex, answer := range resolution.Answers {
		converted := AgentAskAnswerResult{
			QuestionID: answer.QuestionID, Question: answer.Question, CustomInput: answer.CustomInput,
			SelectedOptions: make([]AgentAskSelectedOption, len(answer.SelectedOptions)),
		}
		for optionIndex, option := range answer.SelectedOptions {
			converted.SelectedOptions[optionIndex] = AgentAskSelectedOption{ID: option.ID, Label: option.Label}
		}
		result.Answers[answerIndex] = converted
	}
	return result
}

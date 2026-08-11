package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type InteractionKind string

const (
	InteractionAsk        InteractionKind = "ask"
	InteractionPermission InteractionKind = "permission"
)

// LocalizedText keeps host-visible interaction copy bilingual without tying
// the Agent package to a presentation framework.
type LocalizedText struct {
	Chinese string `json:"zh"`
	English string `json:"en"`
}

type InteractionOption struct {
	Value       string        `json:"value"`
	Label       LocalizedText `json:"label"`
	Description LocalizedText `json:"description,omitempty"`
	Recommended bool          `json:"recommended,omitempty"`
}

type InteractionQuestion struct {
	ID            string              `json:"id"`
	Prompt        LocalizedText       `json:"prompt"`
	Options       []InteractionOption `json:"options,omitempty"`
	Multiple      bool                `json:"multiple,omitempty"`
	AllowFreeText bool                `json:"allow_free_text,omitempty"`
}

type PermissionChoice string

const (
	PermissionAllowOnce PermissionChoice = "allow_once"
	PermissionRemember  PermissionChoice = "remember"
	PermissionDeny      PermissionChoice = "deny"
)

type PermissionPresentation struct {
	Tool      string          `json:"tool"`
	CallID    string          `json:"call_id"`
	Arguments json.RawMessage `json:"arguments"`
	Reason    LocalizedText   `json:"reason"`
}

// InteractionRequest is the only durable host-input vocabulary. Exactly one
// of Questions or Permission is selected by Kind.
type InteractionRequest struct {
	ID         string                  `json:"id"`
	Kind       InteractionKind         `json:"kind"`
	Questions  []InteractionQuestion   `json:"questions,omitempty"`
	Permission *PermissionPresentation `json:"permission,omitempty"`
	AllowOther bool                    `json:"allow_other,omitempty"`
}

type InteractionAnswer struct {
	QuestionID string   `json:"question_id"`
	Values     []string `json:"values,omitempty"`
	Text       string   `json:"text,omitempty"`
}

type InteractionResponse struct {
	Answers    []InteractionAnswer `json:"answers,omitempty"`
	Permission PermissionChoice    `json:"permission,omitempty"`
	Cancelled  bool                `json:"cancelled,omitempty"`
}

// InteractionResolution is validated, normalized, and durably persisted.
// Tools receive this value rather than untrusted transport input.
type InteractionResolution struct {
	Answers    []InteractionAnswer `json:"answers,omitempty"`
	Permission PermissionChoice    `json:"permission,omitempty"`
	Cancelled  bool                `json:"cancelled,omitempty"`
}

type InteractionPolicy interface {
	Identity() CapabilityIdentity
	ValidateRequest(context.Context, InteractionRequest) error
	Resolve(context.Context, InteractionRequest, InteractionResponse) (InteractionResolution, error)
}

type standardInteractionPolicy struct{}

func (standardInteractionPolicy) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "interaction.standard", Version: 1}
}

func (standardInteractionPolicy) ValidateRequest(_ context.Context, request InteractionRequest) error {
	if strings.TrimSpace(request.ID) == "" || request.ID != strings.TrimSpace(request.ID) {
		return errors.New("Interaction Request ID is required")
	}
	switch request.Kind {
	case InteractionAsk:
		if request.Permission != nil || len(request.Questions) < 1 || len(request.Questions) > 3 {
			return errors.New("ask Interaction requires one to three questions")
		}
		seen := make(map[string]struct{}, len(request.Questions))
		for index, question := range request.Questions {
			if err := validateInteractionQuestion(question); err != nil {
				return fmt.Errorf("Interaction question %d: %w", index, err)
			}
			if _, duplicate := seen[question.ID]; duplicate {
				return fmt.Errorf("Interaction question %q is duplicated", question.ID)
			}
			seen[question.ID] = struct{}{}
		}
	case InteractionPermission:
		if request.Permission == nil || len(request.Questions) != 0 || strings.TrimSpace(request.Permission.Tool) == "" ||
			strings.TrimSpace(request.Permission.CallID) == "" || !json.Valid(request.Permission.Arguments) {
			return errors.New("permission Interaction requires tool, call ID, and valid arguments")
		}
		if err := validateLocalizedText(request.Permission.Reason); err != nil {
			return fmt.Errorf("permission Interaction reason: %w", err)
		}
	default:
		return fmt.Errorf("unsupported Interaction kind %q", request.Kind)
	}
	return nil
}

func validateInteractionQuestion(question InteractionQuestion) error {
	if strings.TrimSpace(question.ID) == "" || question.ID != strings.TrimSpace(question.ID) {
		return errors.New("ID is required")
	}
	if err := validateLocalizedText(question.Prompt); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	if len(question.Options) == 0 && !question.AllowFreeText {
		return errors.New("question requires options or free text")
	}
	seen := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		if strings.TrimSpace(option.Value) == "" || option.Value != strings.TrimSpace(option.Value) {
			return errors.New("option value is required")
		}
		if _, duplicate := seen[option.Value]; duplicate {
			return fmt.Errorf("option %q is duplicated", option.Value)
		}
		seen[option.Value] = struct{}{}
		if err := validateLocalizedText(option.Label); err != nil {
			return fmt.Errorf("option %q label: %w", option.Value, err)
		}
		if option.Description != (LocalizedText{}) {
			if err := validateLocalizedText(option.Description); err != nil {
				return fmt.Errorf("option %q description: %w", option.Value, err)
			}
		}
	}
	return nil
}

func validateLocalizedText(value LocalizedText) error {
	if strings.TrimSpace(value.Chinese) == "" || strings.TrimSpace(value.English) == "" {
		return errors.New("both Chinese and English text are required")
	}
	return nil
}

func (policy standardInteractionPolicy) Resolve(ctx context.Context, request InteractionRequest, response InteractionResponse) (InteractionResolution, error) {
	if err := policy.ValidateRequest(ctx, request); err != nil {
		return InteractionResolution{}, err
	}
	if response.Cancelled {
		return InteractionResolution{Cancelled: true}, nil
	}
	switch request.Kind {
	case InteractionPermission:
		switch response.Permission {
		case PermissionAllowOnce, PermissionRemember, PermissionDeny:
			return InteractionResolution{Permission: response.Permission}, nil
		default:
			return InteractionResolution{}, errors.New("permission response must allow once, remember, or deny")
		}
	case InteractionAsk:
		return resolveAskInteraction(request, response)
	default:
		return InteractionResolution{}, fmt.Errorf("unsupported Interaction kind %q", request.Kind)
	}
}

func resolveAskInteraction(request InteractionRequest, response InteractionResponse) (InteractionResolution, error) {
	questions := make(map[string]InteractionQuestion, len(request.Questions))
	for _, question := range request.Questions {
		questions[question.ID] = question
	}
	answers := make(map[string]InteractionAnswer, len(response.Answers))
	for _, answer := range response.Answers {
		question, exists := questions[answer.QuestionID]
		if !exists {
			return InteractionResolution{}, fmt.Errorf("answer targets unknown question %q", answer.QuestionID)
		}
		if _, duplicate := answers[answer.QuestionID]; duplicate {
			return InteractionResolution{}, fmt.Errorf("question %q has multiple answers", answer.QuestionID)
		}
		allowed := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			allowed[option.Value] = struct{}{}
		}
		if !question.Multiple && len(answer.Values) > 1 {
			return InteractionResolution{}, fmt.Errorf("question %q accepts one option", answer.QuestionID)
		}
		seen := make(map[string]struct{}, len(answer.Values))
		for _, value := range answer.Values {
			if _, ok := allowed[value]; !ok {
				return InteractionResolution{}, fmt.Errorf("question %q received unknown option %q", answer.QuestionID, value)
			}
			if _, duplicate := seen[value]; duplicate {
				return InteractionResolution{}, fmt.Errorf("question %q repeats option %q", answer.QuestionID, value)
			}
			seen[value] = struct{}{}
		}
		if strings.TrimSpace(answer.Text) != "" && !question.AllowFreeText && !request.AllowOther {
			return InteractionResolution{}, fmt.Errorf("question %q does not accept free text", answer.QuestionID)
		}
		if len(answer.Values) == 0 && strings.TrimSpace(answer.Text) == "" {
			return InteractionResolution{}, fmt.Errorf("question %q has an empty answer", answer.QuestionID)
		}
		answer.Values = append([]string(nil), answer.Values...)
		answers[answer.QuestionID] = answer
	}
	if len(answers) != len(request.Questions) {
		return InteractionResolution{}, errors.New("every Interaction question requires an answer")
	}
	ordered := make([]InteractionAnswer, 0, len(request.Questions))
	for _, question := range request.Questions {
		ordered = append(ordered, answers[question.ID])
	}
	return InteractionResolution{Answers: ordered}, nil
}

type interactionClient interface {
	Request(context.Context, InteractionRequest) (InteractionResolution, error)
}

type interactionContextKey struct{}

func contextWithInteractionClient(ctx context.Context, client interactionClient) context.Context {
	return context.WithValue(ctx, interactionContextKey{}, client)
}

// RequestInteraction asks through the current durable Run. It is available to
// custom tools and returns an error when called outside Agent execution.
func RequestInteraction(ctx context.Context, request InteractionRequest) (InteractionResolution, error) {
	if ctx == nil {
		return InteractionResolution{}, ErrCapabilityUnsupported
	}
	client, ok := ctx.Value(interactionContextKey{}).(interactionClient)
	if !ok || client == nil {
		return InteractionResolution{}, ErrCapabilityUnsupported
	}
	return client.Request(ctx, request)
}

type engineInteractionClient struct {
	policy InteractionPolicy
	emit   runstate.EngineEventSink

	mu      sync.Mutex
	known   map[string]runstate.InteractionSnapshot
	waiters map[string]chan json.RawMessage
}

func newEngineInteractionClient(policy InteractionPolicy, snapshots []runstate.InteractionSnapshot, emit runstate.EngineEventSink) *engineInteractionClient {
	known := make(map[string]runstate.InteractionSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		known[snapshot.ID] = snapshot
	}
	return &engineInteractionClient{policy: policy, emit: emit, known: known, waiters: make(map[string]chan json.RawMessage)}
}

func (client *engineInteractionClient) Request(ctx context.Context, request InteractionRequest) (InteractionResolution, error) {
	if client == nil || client.policy == nil || client.emit == nil {
		return InteractionResolution{}, ErrCapabilityUnsupported
	}
	if err := client.policy.ValidateRequest(ctx, request); err != nil {
		return InteractionResolution{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return InteractionResolution{}, fmt.Errorf("encode Interaction request: %w", err)
	}
	client.mu.Lock()
	if known, exists := client.known[request.ID]; exists {
		if string(known.Request) != string(encoded) {
			client.mu.Unlock()
			return InteractionResolution{}, ErrInteractionStale
		}
		if known.Resolved {
			response := append(json.RawMessage(nil), known.Response...)
			client.mu.Unlock()
			return decodeInteractionResolution(response)
		}
	}
	if _, duplicate := client.waiters[request.ID]; duplicate {
		client.mu.Unlock()
		return InteractionResolution{}, fmt.Errorf("Interaction %q already has a waiter", request.ID)
	}
	waiter := make(chan json.RawMessage, 1)
	client.waiters[request.ID] = waiter
	client.mu.Unlock()

	if err := client.emit(runstate.EngineInteractionRequested{
		ID: request.ID, ToolCallID: CurrentToolExecutionID(ctx), Request: encoded,
	}); err != nil {
		client.mu.Lock()
		delete(client.waiters, request.ID)
		client.mu.Unlock()
		return InteractionResolution{}, err
	}
	select {
	case response := <-waiter:
		return decodeInteractionResolution(response)
	case <-ctx.Done():
		client.mu.Lock()
		delete(client.waiters, request.ID)
		client.mu.Unlock()
		return InteractionResolution{}, ctx.Err()
	}
}

func decodeInteractionResolution(encoded json.RawMessage) (InteractionResolution, error) {
	var resolution InteractionResolution
	if err := json.Unmarshal(encoded, &resolution); err != nil {
		return InteractionResolution{}, fmt.Errorf("decode Interaction resolution: %w", err)
	}
	return resolution, nil
}

func (client *engineInteractionClient) deliver(id string, response json.RawMessage) {
	client.mu.Lock()
	if waiter := client.waiters[id]; waiter != nil {
		delete(client.waiters, id)
		waiter <- append(json.RawMessage(nil), response...)
	}
	client.mu.Unlock()
}

func effectiveInteractionPolicy(policy InteractionPolicy) InteractionPolicy {
	if policy == nil {
		return standardInteractionPolicy{}
	}
	return policy
}

// StandardInteraction returns the built-in bilingual validation and
// normalization policy.
func StandardInteraction() InteractionPolicy { return standardInteractionPolicy{} }

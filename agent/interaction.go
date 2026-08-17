package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

type InteractionKind string

const (
	InteractionAsk        InteractionKind = "ask"
	InteractionPermission InteractionKind = "permission"
)

// LocalizedText keeps host-visible interaction copy bilingual without tying
// the Agent package to a presentation framework.
type LocalizedText struct {
	Chinese string `json:"zh" jsonschema:"minLength=1,maxLength=8192" jsonschema_description:"Simplified Chinese user-visible text."`
	English string `json:"en" jsonschema:"minLength=1,maxLength=8192" jsonschema_description:"English translation of the same user-visible text."`
}

type InteractionOption struct {
	Value       string        `json:"value" jsonschema:"minLength=1,maxLength=256,pattern=^[A-Za-z0-9][A-Za-z0-9._:-]*$" jsonschema_description:"Stable option ID; other is reserved for the host-provided free-text choice."`
	Label       LocalizedText `json:"label" jsonschema_description:"Concise bilingual option label."`
	Description LocalizedText `json:"description,omitempty" jsonschema_description:"Optional bilingual consequence or tradeoff."`
	Recommended bool          `json:"recommended,omitempty" jsonschema_description:"True for the single recommended option."`
}

type InteractionQuestion struct {
	ID            string              `json:"id" jsonschema:"minLength=1,maxLength=256,pattern=^[A-Za-z0-9][A-Za-z0-9._:-]*$" jsonschema_description:"Stable question ID used to correlate the answer."`
	Prompt        LocalizedText       `json:"prompt" jsonschema_description:"The same user-facing question in Chinese and English."`
	Options       []InteractionOption `json:"options,omitempty" jsonschema_description:"Two or three choices; the host adds Other automatically."`
	Multiple      bool                `json:"multiple,omitempty" jsonschema_description:"Allow more than one listed option."`
	AllowFreeText bool                `json:"allow_free_text,omitempty" jsonschema_description:"True only for a free-text question without options."`
}

type PermissionChoice string

const (
	PermissionAllowOnce PermissionChoice = "allow_once"
	PermissionRemember  PermissionChoice = "remember"
	PermissionDeny      PermissionChoice = "deny"
)

type PermissionPresentation struct {
	Tool               string              `json:"tool"`
	CallID             string              `json:"call_id"`
	Arguments          json.RawMessage     `json:"arguments"`
	Reason             LocalizedText       `json:"reason"`
	Mode               string              `json:"mode"`
	Command            string              `json:"command,omitempty"`
	Details            string              `json:"details,omitempty"`
	Cwd                string              `json:"cwd,omitempty"`
	Risk               string              `json:"risk"`
	RuleID             string              `json:"rule_id"`
	ArgsHash           string              `json:"args_hash"`
	CanRemember        bool                `json:"can_remember,omitempty"`
	RuleMatcherVersion int                 `json:"rule_matcher_version,omitempty"`
	RuleCommandKey     string              `json:"rule_command_key,omitempty"`
	RuleCommandPattern string              `json:"rule_command_pattern,omitempty"`
	Options            []InteractionOption `json:"options"`
}

// InteractionRequest is the only host-input vocabulary. Exactly one
// of Questions or Permission is selected by Kind.
type InteractionRequest struct {
	ID         string                  `json:"id"`
	Kind       InteractionKind         `json:"kind"`
	Questions  []InteractionQuestion   `json:"questions,omitempty"`
	Permission *PermissionPresentation `json:"permission,omitempty"`
	AllowOther bool                    `json:"allow_other,omitempty"`
}

type InteractionAnswer struct {
	QuestionID string   `json:"question_id" jsonschema:"minLength=1,maxLength=256" jsonschema_description:"Question ID being answered."`
	Values     []string `json:"values,omitempty" jsonschema_description:"Selected option values for a choice question."`
	Text       string   `json:"text,omitempty" jsonschema:"maxLength=65536" jsonschema_description:"User-entered answer for a free-text or Other choice."`
}

type InteractionResponse struct {
	Answers    []InteractionAnswer `json:"answers,omitempty"`
	Permission PermissionChoice    `json:"permission,omitempty"`
	Cancelled  bool                `json:"cancelled,omitempty"`
}

// InteractionResolution is validated and normalized before it reaches a Tool.
// Interaction waiters and responses intentionally remain process-local.
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

const (
	maxInteractionRequestBytes      = 128 << 10
	maxInteractionStableIDBytes     = 256
	maxInteractionQuestionTextBytes = 8 << 10
	maxInteractionOptionTextBytes   = 4 << 10
	maxInteractionAnswerTextBytes   = 64 << 10
	reservedInteractionOtherValue   = "other"
)

var interactionStableIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

func (standardInteractionPolicy) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "interaction.standard", Version: 1}
}

func (standardInteractionPolicy) ValidateRequest(_ context.Context, request InteractionRequest) error {
	if err := validateInteractionStableID("Interaction Request ID", request.ID); err != nil {
		return err
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
		encoded, err := json.Marshal(request.Questions)
		if err != nil {
			return fmt.Errorf("encode ask Interaction questions: %w", err)
		}
		if len(encoded) > maxInteractionRequestBytes {
			return fmt.Errorf("ask Interaction question payload exceeds %d bytes", maxInteractionRequestBytes)
		}
	case InteractionPermission:
		if request.Permission == nil || len(request.Questions) != 0 || strings.TrimSpace(request.Permission.Tool) == "" ||
			strings.TrimSpace(request.Permission.CallID) == "" || !json.Valid(request.Permission.Arguments) ||
			strings.TrimSpace(request.Permission.Mode) == "" || strings.TrimSpace(request.Permission.Risk) == "" ||
			strings.TrimSpace(request.Permission.RuleID) == "" || strings.TrimSpace(request.Permission.ArgsHash) == "" {
			return errors.New("permission Interaction requires tool, call ID, and valid arguments")
		}
		if err := validateLocalizedText(request.Permission.Reason); err != nil {
			return fmt.Errorf("permission Interaction reason: %w", err)
		}
		if err := validatePermissionOptions(request.Permission.Options, request.Permission.CanRemember); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported Interaction kind %q", request.Kind)
	}
	return nil
}

func validatePermissionOptions(options []InteractionOption, canRemember bool) error {
	want := map[string]bool{
		string(PermissionAllowOnce): true,
		string(PermissionDeny):      true,
		string(PermissionRemember):  canRemember,
	}
	seen := make(map[string]bool, len(options))
	for _, option := range options {
		if !want[option.Value] || seen[option.Value] {
			return fmt.Errorf("permission Interaction has invalid option %q", option.Value)
		}
		seen[option.Value] = true
		if err := validateLocalizedText(option.Label); err != nil {
			return fmt.Errorf("permission option %q label: %w", option.Value, err)
		}
		if err := validateLocalizedText(option.Description); err != nil {
			return fmt.Errorf("permission option %q description: %w", option.Value, err)
		}
	}
	if !seen[string(PermissionAllowOnce)] || !seen[string(PermissionDeny)] ||
		canRemember != seen[string(PermissionRemember)] {
		return errors.New("permission Interaction options do not match policy choices")
	}
	return nil
}

func validateInteractionQuestion(question InteractionQuestion) error {
	if err := validateInteractionStableID("ID", question.ID); err != nil {
		return err
	}
	if err := validateLocalizedText(question.Prompt); err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	if localizedTextBytes(question.Prompt) > maxInteractionQuestionTextBytes {
		return fmt.Errorf("prompt exceeds %d bytes", maxInteractionQuestionTextBytes)
	}
	if len(question.Options) == 0 {
		if !question.AllowFreeText || question.Multiple {
			return errors.New("free-text question must allow text and cannot be multi-select")
		}
		return nil
	}
	if len(question.Options) < 2 || len(question.Options) > 3 {
		return errors.New("choice question requires two to three options")
	}
	seen := make(map[string]struct{}, len(question.Options))
	recommended := 0
	for _, option := range question.Options {
		if err := validateInteractionStableID("option value", option.Value); err != nil {
			return err
		}
		if strings.EqualFold(option.Value, reservedInteractionOtherValue) {
			return fmt.Errorf("option value %q is reserved for the host-provided Other choice", option.Value)
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
		if localizedTextBytes(option.Label)+localizedTextBytes(option.Description) > maxInteractionOptionTextBytes {
			return fmt.Errorf("option %q text exceeds %d bytes", option.Value, maxInteractionOptionTextBytes)
		}
		if option.Recommended {
			recommended++
		}
	}
	if recommended != 1 {
		return errors.New("choice question requires exactly one recommended option")
	}
	return nil
}

func validateLocalizedText(value LocalizedText) error {
	if strings.TrimSpace(value.Chinese) == "" || strings.TrimSpace(value.English) == "" {
		return errors.New("both Chinese and English text are required")
	}
	return nil
}

func validateInteractionStableID(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || len(value) > maxInteractionStableIDBytes ||
		!interactionStableIDPattern.MatchString(value) {
		return fmt.Errorf("%s must contain 1..%d bytes using letters, numbers, '.', '_', ':', or '-'", field, maxInteractionStableIDBytes)
	}
	return nil
}

func localizedTextBytes(value LocalizedText) int {
	return len(value.Chinese) + len(value.English)
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
		if len(answer.Text) > maxInteractionAnswerTextBytes {
			return InteractionResolution{}, fmt.Errorf("custom answer for %q exceeds %d bytes", answer.QuestionID, maxInteractionAnswerTextBytes)
		}
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
		answer.Text = strings.TrimSpace(answer.Text)
		if len(question.Options) == 0 {
			if len(answer.Values) != 0 || answer.Text == "" {
				return InteractionResolution{}, fmt.Errorf("free-text question %q requires text and no option", answer.QuestionID)
			}
		} else if answer.Text != "" {
			if !request.AllowOther {
				return InteractionResolution{}, fmt.Errorf("question %q does not accept free text", answer.QuestionID)
			}
			if len(answer.Values) != 0 && !question.Multiple {
				return InteractionResolution{}, fmt.Errorf("question %q cannot combine Other with a selected option", answer.QuestionID)
			}
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

// RequestInteraction asks through the current Run. It is available to
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
	waiters map[string]chan json.RawMessage
}

func newEngineInteractionClient(policy InteractionPolicy, emit runstate.EngineEventSink) *engineInteractionClient {
	return &engineInteractionClient{policy: policy, emit: emit, waiters: make(map[string]chan json.RawMessage)}
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
	touchIdleActivity(ctx)
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

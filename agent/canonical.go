package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type CommitStage string

const (
	CommitInput   CommitStage = "input"
	CommitContext CommitStage = "context"
	CommitOutput  CommitStage = "output"
)

// CommitIdentity is the exact idempotency boundary shared by Agent and a
// product store. Adapters must never substitute a display or request ID.
type CommitIdentity struct {
	Session   SessionKey
	CommandID string
	RunID     string
	Cycle     int
	Stage     CommitStage
}

type InputCommitRequest struct {
	Identity CommitIdentity
	Hash     string
	Input    Input
}

type OutputCommitRequest struct {
	Identity CommitIdentity
	Hash     string
	// Message is the exact provider-neutral final output. Hosts may persist
	// continuation metadata and usage beside their product projection without
	// depending on a provider SDK type.
	Message Message
}

// ContextCommitRequest appends one model-visible, UI-hidden message batch to
// the same canonical lane as accepted input and final output. Sequence makes
// retries deterministic within one Agent cycle; the messages define the batch
// shape so callers cannot label content inconsistently.
type ContextCommitRequest struct {
	Identity CommitIdentity
	Sequence int
	Messages []Message
}

type CommitReceipt struct{ Revision string }

// OutputProjection optionally replaces the provider output retained in the
// Agent transcript. It supports host protocols such as structured plan cards:
// the raw output remains the canonical commit identity, while only the
// product-approved projection becomes future model context.
type OutputProjection struct {
	Content  string
	Thinking string
}

type OutputCommitReceipt struct {
	Revision   string
	Transcript *OutputProjection
}

type Effect struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type EffectRequest struct {
	ID       string
	Identity CommitIdentity
	CallID   string
	Index    int
	Effect   Effect
}

type EffectResult struct {
	ID       string
	Revision string
	Error    string
}

// CanonicalAdapter coordinates direct idempotent conversation commits to a
// host's product journal. Tool effects use Definition.Effects independently.
type CanonicalAdapter interface {
	Identity() CapabilityIdentity
	MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error)
	CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
}

// CanonicalContextAdapter is the optional extension used by hosts that own
// the conversation journal. Standalone Agents without a product journal keep
// using Agent's built-in transcript store.
type CanonicalContextAdapter interface {
	CommitContext(context.Context, ContextCommitRequest) (CommitReceipt, error)
}

// CanonicalAdapterFuncs is the compact adapter form for hosts whose product
// store already exposes idempotent commit functions.
type CanonicalAdapterFuncs struct {
	CapabilityIdentity CapabilityIdentity
	MaterializeInputFn func(context.Context, InputCommitRequest) (CommitReceipt, error)
	CommitOutputFn     func(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
}

func (adapter CanonicalAdapterFuncs) Identity() CapabilityIdentity {
	return adapter.CapabilityIdentity
}

func (adapter CanonicalAdapterFuncs) MaterializeInput(ctx context.Context, request InputCommitRequest) (CommitReceipt, error) {
	if adapter.MaterializeInputFn == nil {
		return CommitReceipt{}, ErrCapabilityUnsupported
	}
	return adapter.MaterializeInputFn(ctx, request)
}

func (adapter CanonicalAdapterFuncs) CommitOutput(ctx context.Context, request OutputCommitRequest) (OutputCommitReceipt, error) {
	if adapter.CommitOutputFn == nil {
		return OutputCommitReceipt{}, ErrCapabilityUnsupported
	}
	return adapter.CommitOutputFn(ctx, request)
}

// ValidateContextCommitMessages derives and validates the one supported batch
// shape from its messages. A context-state batch, a complete tool batch, and a
// delegated task-completion batch are deliberately mutually exclusive.
func ValidateContextCommitMessages(messages []*Message) error {
	if len(messages) == 0 || messages[0] == nil {
		return errors.New("canonical context commit requires messages")
	}
	first := messages[0]
	switch {
	case first.Role == Assistant && len(first.ToolCalls) > 0:
		return validateCanonicalToolBatch(messages)
	case first.Role == User && IsContextStateMessage(first):
		for index, message := range messages {
			if message == nil || message.Role != User || !IsContextStateMessage(message) || message.TaskCompletion != nil ||
				len(message.ToolCalls) != 0 || strings.TrimSpace(message.ToolCallID) != "" {
				return fmt.Errorf("canonical context state message %d is invalid", index)
			}
		}
		if _, err := rebuildContextStateSnapshot(messages); err != nil {
			return fmt.Errorf("validate canonical context state: %w", err)
		}
		return nil
	case first.Role == User && first.TaskCompletion != nil:
		for index, message := range messages {
			if message == nil || message.Role != User || message.TaskCompletion == nil ||
				strings.TrimSpace(message.TaskCompletion.CompletionID) == "" ||
				strings.TrimSpace(message.TaskCompletion.Author) == "" ||
				strings.TrimSpace(message.TaskCompletion.Recipient) == "" ||
				len(message.ToolCalls) != 0 || strings.TrimSpace(message.ToolCallID) != "" ||
				IsContextStateMessage(message) {
				return fmt.Errorf("canonical task completion message %d is invalid", index)
			}
		}
		return nil
	default:
		return errors.New("canonical context messages do not form a supported batch")
	}
}

func validateCanonicalToolBatch(messages []*Message) error {
	if len(messages) < 2 {
		return errors.New("canonical tool batch requires one assistant call message and its results")
	}
	pending, err := validateCanonicalToolCallMessage(messages[0])
	if err != nil {
		return err
	}
	if len(messages) != len(messages[0].ToolCalls)+1 {
		return errors.New("canonical tool batch must contain exactly one result per tool call")
	}
	for index, message := range messages[1:] {
		if message == nil || message.Role != ToolRole || len(message.ToolCalls) != 0 || message.TaskCompletion != nil {
			return fmt.Errorf("canonical tool result %d is invalid", index)
		}
		id := strings.TrimSpace(message.ToolCallID)
		if _, ok := pending[id]; !ok || id == "" {
			return fmt.Errorf("canonical tool result %d has no matching call", index)
		}
		delete(pending, id)
	}
	if len(pending) != 0 {
		return errors.New("canonical tool batch is missing tool results")
	}
	return nil
}

func validateCanonicalToolCallMessage(message *Message) (map[string]struct{}, error) {
	if message == nil || message.Role != Assistant || len(message.ToolCalls) == 0 ||
		strings.TrimSpace(message.ToolCallID) != "" || message.TaskCompletion != nil {
		return nil, errors.New("canonical tool batch requires an assistant tool-call message")
	}
	pending := make(map[string]struct{}, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" || strings.TrimSpace(call.Function.Name) == "" {
			return nil, errors.New("canonical tool batch contains an invalid tool call")
		}
		if err := ValidateToolArgumentsJSON(call.Function.Arguments); err != nil {
			return nil, fmt.Errorf("canonical tool call %q arguments: %w", id, err)
		}
		if _, duplicate := pending[id]; duplicate {
			return nil, fmt.Errorf("canonical tool batch repeats tool call %q", id)
		}
		pending[id] = struct{}{}
	}
	return pending, nil
}

// ValidateContextCommit verifies an Agent-owned context sequence before a host
// applies its own idempotent write.
func ValidateContextCommit(request ContextCommitRequest) error {
	if request.Identity.Stage != CommitContext {
		return errors.New("canonical context commit has a non-context identity")
	}
	if request.Sequence < 0 {
		return errors.New("canonical context commit requires a non-negative sequence")
	}
	messages := make([]*Message, len(request.Messages))
	for index := range request.Messages {
		messages[index] = request.Messages[index].Clone()
	}
	return ValidateContextCommitMessages(messages)
}

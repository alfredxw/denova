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

type ContextCommitKind string

const (
	ContextCommitState          ContextCommitKind = "context_state"
	ContextCommitToolBatch      ContextCommitKind = "tool_batch"
	ContextCommitTaskCompletion ContextCommitKind = "task_completion"
)

// ContextCommitRequest appends one model-visible, UI-hidden message batch to
// the same canonical lane as accepted input and final output. Kind and Ordinal
// make retries deterministic within one Agent cycle.
type ContextCommitRequest struct {
	Identity CommitIdentity
	Kind     ContextCommitKind
	Ordinal  int
	Hash     string
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

// CanonicalAdapter coordinates direct idempotent writes to a host's product
// store. ApplyEffects returns one result per item.
type CanonicalAdapter interface {
	Identity() CapabilityIdentity
	MaterializeInput(context.Context, InputCommitRequest) (CommitReceipt, error)
	CommitOutput(context.Context, OutputCommitRequest) (OutputCommitReceipt, error)
	ApplyEffects(context.Context, []EffectRequest) ([]EffectResult, error)
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
	ApplyEffectsFn     func(context.Context, []EffectRequest) ([]EffectResult, error)
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

func (adapter CanonicalAdapterFuncs) ApplyEffects(ctx context.Context, requests []EffectRequest) ([]EffectResult, error) {
	if adapter.ApplyEffectsFn == nil {
		return nil, ErrCapabilityUnsupported
	}
	return adapter.ApplyEffectsFn(ctx, requests)
}

func canonicalContextHash(kind ContextCommitKind, ordinal int, messages []*Message, adapter CapabilityIdentity) (string, error) {
	if err := adapter.validate("Canonical"); err != nil {
		return "", err
	}
	if ordinal < 0 || len(messages) == 0 {
		return "", errors.New("canonical context commit requires a non-negative ordinal and messages")
	}
	if err := validateCanonicalContextMessages(kind, messages); err != nil {
		return "", err
	}
	return hashCanonical(struct {
		Version  uint16
		Adapter  CapabilityIdentity
		Kind     ContextCommitKind
		Ordinal  int
		Messages []*Message
	}{Version: 1, Adapter: adapter, Kind: kind, Ordinal: ordinal, Messages: cloneMessages(messages)})
}

func validateCanonicalContextMessages(kind ContextCommitKind, messages []*Message) error {
	switch kind {
	case ContextCommitState:
		for index, message := range messages {
			if message == nil || message.Role != User || !IsContextStateMessage(message) {
				return fmt.Errorf("canonical context state message %d is invalid", index)
			}
		}
		if _, err := rebuildContextStateSnapshot(messages); err != nil {
			return fmt.Errorf("validate canonical context state: %w", err)
		}
		return nil
	case ContextCommitToolBatch:
		return validateCanonicalToolBatch(messages)
	case ContextCommitTaskCompletion:
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
		return fmt.Errorf("unsupported canonical context commit kind %q", kind)
	}
}

func validateCanonicalToolBatch(messages []*Message) error {
	if len(messages) < 2 || messages[0] == nil || messages[0].Role != Assistant || len(messages[0].ToolCalls) == 0 {
		return errors.New("canonical tool batch requires one assistant call message and its results")
	}
	if len(messages) != len(messages[0].ToolCalls)+1 {
		return errors.New("canonical tool batch must contain exactly one result per tool call")
	}
	pending := make(map[string]struct{}, len(messages[0].ToolCalls))
	for _, call := range messages[0].ToolCalls {
		id := strings.TrimSpace(call.ID)
		if id == "" || strings.TrimSpace(call.Function.Name) == "" {
			return errors.New("canonical tool batch contains an invalid tool call")
		}
		if _, duplicate := pending[id]; duplicate {
			return fmt.Errorf("canonical tool batch repeats tool call %q", id)
		}
		pending[id] = struct{}{}
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

// ValidateContextCommit verifies that a host received the exact context batch
// produced by Agent. Hosts call it before applying their own idempotent write.
func ValidateContextCommit(request ContextCommitRequest, adapter CapabilityIdentity) error {
	messages := make([]*Message, len(request.Messages))
	for index := range request.Messages {
		messages[index] = request.Messages[index].Clone()
	}
	want, err := canonicalContextHash(request.Kind, request.Ordinal, messages, adapter)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.Hash) != want {
		return errors.New("canonical context commit hash does not match messages")
	}
	return nil
}

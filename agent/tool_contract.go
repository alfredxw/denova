package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Tool is the single provider-neutral execution interface. Long-running tools
// report ephemeral progress through EmitToolProgress and return one final,
// structured result.
type Tool interface {
	Info(context.Context) (*ToolInfo, error)
	Run(context.Context, string, ...ToolOption) (ToolResult, error)
}

// ToolSource classifies where a tool reads or changes state.
type ToolSource string

const (
	ToolSourceOther   ToolSource = "other"
	ToolSourceRead    ToolSource = "read"
	ToolSourceWrite   ToolSource = "write"
	ToolSourceShell   ToolSource = "shell"
	ToolSourceLore    ToolSource = "lore"
	ToolSourceHistory ToolSource = "history"
	ToolSourceWeb     ToolSource = "web"
	ToolSourceImage   ToolSource = "image"
)

// ToolExecutionClass controls ordering inside one model-produced batch.
type ToolExecutionClass string

const (
	ToolExecutionParallelRead       ToolExecutionClass = "parallel_read"
	ToolExecutionWorkspaceExclusive ToolExecutionClass = "workspace_exclusive"
	ToolExecutionSessionExclusive   ToolExecutionClass = "session_exclusive"
	ToolExecutionConfigExclusive    ToolExecutionClass = "config_exclusive"
	ToolExecutionInteractiveWait    ToolExecutionClass = "interactive_wait"
	ToolExecutionChild              ToolExecutionClass = "child"
)

// ToolMutationScope identifies the state domain a successful call may
// change. It is deliberately independent from Source: a shell call reads and
// writes through one execution surface, while config and session mutations do
// not belong to the workspace mutation pipeline.
type ToolMutationScope string

const (
	ToolMutationNone      ToolMutationScope = "none"
	ToolMutationWorkspace ToolMutationScope = "workspace"
	ToolMutationSession   ToolMutationScope = "session"
	ToolMutationConfig    ToolMutationScope = "config"
	ToolMutationExternal  ToolMutationScope = "external"
)

// ToolPostCheckPolicy selects the domain-specific verification performed
// after a successful mutation. The policy must match MutationScope.
type ToolPostCheckPolicy string

const (
	ToolPostCheckNone            ToolPostCheckPolicy = "none"
	ToolPostCheckWorkspaceChange ToolPostCheckPolicy = "workspace_change"
	ToolPostCheckSessionState    ToolPostCheckPolicy = "session_state"
	ToolPostCheckConfigRevision  ToolPostCheckPolicy = "config_revision"
	ToolPostCheckExternalReceipt ToolPostCheckPolicy = "external_receipt"
)

// ToolRecoveryClass describes what is safe after a durable start without a
// matching completion.
type ToolRecoveryClass string

const (
	ToolRecoveryReadOnly      ToolRecoveryClass = "read_only"
	ToolRecoveryIdempotent    ToolRecoveryClass = "idempotent"
	ToolRecoveryReconcilable  ToolRecoveryClass = "reconcilable"
	ToolRecoveryNonIdempotent ToolRecoveryClass = "non_idempotent"
)

// ToolResultProjection declares how a result may enter model context.
type ToolResultProjection string

const ToolResultBoundedModelContext ToolResultProjection = "bounded_model_context"

// SteeringPolicy controls what a pending safe preemption may do to a call that
// has already started.
type SteeringPolicy string

const (
	SteeringFinishCurrent     SteeringPolicy = "finish_current"
	SteeringInterruptibleWait SteeringPolicy = "interruptible_wait"
)

// ToolDescriptor is the complete execution, recovery, and context contract for
// one model-visible tool.
type ToolDescriptor struct {
	Source           ToolSource           `json:"source"`
	Capability       string               `json:"capability,omitempty"`
	Execution        ToolExecutionClass   `json:"execution"`
	MutationScope    ToolMutationScope    `json:"mutation_scope"`
	PostCheck        ToolPostCheckPolicy  `json:"post_check"`
	Recovery         ToolRecoveryClass    `json:"recovery"`
	ResultProjection ToolResultProjection `json:"result_projection"`
	Steering         SteeringPolicy       `json:"steering"`
	MaxResultBytes   int                  `json:"max_result_bytes"`
}

// Validate rejects incomplete descriptors and inconsistent safety claims.
func (descriptor ToolDescriptor) Validate() error {
	switch descriptor.Source {
	case ToolSourceOther, ToolSourceRead, ToolSourceWrite, ToolSourceShell,
		ToolSourceLore, ToolSourceHistory, ToolSourceWeb, ToolSourceImage:
	default:
		return fmt.Errorf("invalid tool source %q", descriptor.Source)
	}
	switch descriptor.Execution {
	case ToolExecutionParallelRead, ToolExecutionWorkspaceExclusive, ToolExecutionSessionExclusive,
		ToolExecutionConfigExclusive, ToolExecutionInteractiveWait, ToolExecutionChild:
	default:
		return fmt.Errorf("invalid tool execution class %q", descriptor.Execution)
	}
	switch descriptor.MutationScope {
	case ToolMutationNone, ToolMutationWorkspace, ToolMutationSession, ToolMutationConfig, ToolMutationExternal:
	default:
		return fmt.Errorf("invalid tool mutation scope %q", descriptor.MutationScope)
	}
	switch descriptor.PostCheck {
	case ToolPostCheckNone, ToolPostCheckWorkspaceChange, ToolPostCheckSessionState,
		ToolPostCheckConfigRevision, ToolPostCheckExternalReceipt:
	default:
		return fmt.Errorf("invalid tool post-check policy %q", descriptor.PostCheck)
	}
	switch descriptor.Recovery {
	case ToolRecoveryReadOnly, ToolRecoveryIdempotent, ToolRecoveryReconcilable, ToolRecoveryNonIdempotent:
	default:
		return fmt.Errorf("invalid tool recovery class %q", descriptor.Recovery)
	}
	if descriptor.ResultProjection != ToolResultBoundedModelContext {
		return fmt.Errorf("invalid tool result projection %q", descriptor.ResultProjection)
	}
	switch descriptor.Steering {
	case SteeringFinishCurrent, SteeringInterruptibleWait:
	default:
		return fmt.Errorf("invalid tool steering policy %q", descriptor.Steering)
	}
	if descriptor.MaxResultBytes <= 0 {
		return errors.New("tool result limit must be positive")
	}
	if descriptor.Execution == ToolExecutionParallelRead && descriptor.MutationScope != ToolMutationNone {
		return errors.New("parallel read tool cannot mutate state")
	}
	if descriptor.Source == ToolSourceWrite && descriptor.MutationScope == ToolMutationNone {
		return errors.New("write-source tool must declare a mutation scope")
	}
	if err := validateToolExecutionMutation(descriptor.Execution, descriptor.MutationScope); err != nil {
		return err
	}
	if err := validateToolPostCheck(descriptor.MutationScope, descriptor.PostCheck); err != nil {
		return err
	}
	if descriptor.Steering == SteeringInterruptibleWait &&
		(descriptor.MutationScope != ToolMutationNone || descriptor.Recovery != ToolRecoveryReadOnly) {
		return errors.New("interruptible wait must be read-only and non-mutating")
	}
	return nil
}

func validateToolExecutionMutation(execution ToolExecutionClass, scope ToolMutationScope) error {
	switch execution {
	case ToolExecutionParallelRead, ToolExecutionChild:
		if scope != ToolMutationNone {
			return fmt.Errorf("execution class %q requires mutation scope %q", execution, ToolMutationNone)
		}
	case ToolExecutionWorkspaceExclusive:
		if scope != ToolMutationWorkspace && scope != ToolMutationExternal {
			return fmt.Errorf("execution class %q requires workspace or external mutation", execution)
		}
	case ToolExecutionSessionExclusive:
		if scope != ToolMutationSession && scope != ToolMutationExternal {
			return fmt.Errorf("execution class %q requires session or external mutation", execution)
		}
	case ToolExecutionConfigExclusive:
		if scope != ToolMutationConfig {
			return fmt.Errorf("execution class %q requires mutation scope %q", execution, ToolMutationConfig)
		}
	case ToolExecutionInteractiveWait:
		if scope != ToolMutationNone && scope != ToolMutationSession {
			return fmt.Errorf("execution class %q requires no mutation or a session mutation", execution)
		}
	}
	return nil
}

func validateToolPostCheck(scope ToolMutationScope, policy ToolPostCheckPolicy) error {
	want := ToolPostCheckNone
	switch scope {
	case ToolMutationWorkspace:
		want = ToolPostCheckWorkspaceChange
	case ToolMutationSession:
		want = ToolPostCheckSessionState
	case ToolMutationConfig:
		want = ToolPostCheckConfigRevision
	case ToolMutationExternal:
		want = ToolPostCheckExternalReceipt
	}
	if policy != ToolPostCheckNone && policy != want {
		return fmt.Errorf("post-check policy %q does not match mutation scope %q", policy, scope)
	}
	return nil
}

// ToolDefinition is the only registration unit accepted by Agent.
type ToolDefinition struct {
	Tool       Tool
	Descriptor ToolDescriptor
}

// Validate checks the concrete schema and its execution contract.
func (definition ToolDefinition) Validate(ctx context.Context) error {
	_, err := definition.snapshot(ctx)
	return err
}

func (definition ToolDefinition) snapshot(ctx context.Context) (ToolDefinitionSnapshot, error) {
	if definition.Tool == nil {
		return ToolDefinitionSnapshot{}, errors.New("tool definition has nil Tool")
	}
	if err := definition.Descriptor.Validate(); err != nil {
		return ToolDefinitionSnapshot{}, fmt.Errorf("tool descriptor: %w", err)
	}
	info, err := definition.Tool.Info(ctx)
	if err != nil {
		return ToolDefinitionSnapshot{}, fmt.Errorf("read tool info: %w", err)
	}
	if info == nil || strings.TrimSpace(info.Name) == "" {
		return ToolDefinitionSnapshot{}, errors.New("tool definition has no stable name")
	}
	if info.Name != strings.TrimSpace(info.Name) {
		return ToolDefinitionSnapshot{}, fmt.Errorf("tool %q has leading or trailing whitespace", info.Name)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		return ToolDefinitionSnapshot{}, fmt.Errorf("tool %q schema: %w", info.Name, err)
	}
	if err := validateToolSchema(schema); err != nil {
		return ToolDefinitionSnapshot{}, fmt.Errorf("tool %q schema: %w", info.Name, err)
	}
	return ToolDefinitionSnapshot{Info: cloneToolInfo(info), Descriptor: definition.Descriptor}, nil
}

// ToolDefinitionSnapshot contains immutable call metadata without exposing the
// concrete implementation.
type ToolDefinitionSnapshot struct {
	Info       *ToolInfo      `json:"info"`
	Descriptor ToolDescriptor `json:"descriptor"`
}

// ToolResultStatus is the exhaustive outcome of a tool call.
type ToolResultStatus string

const (
	ToolResultSuccess ToolResultStatus = "success"
	ToolResultError   ToolResultStatus = "error"
	ToolResultBlocked ToolResultStatus = "blocked"
	ToolResultSkipped ToolResultStatus = "skipped"
)

// ToolSyntheticReason identifies why no ordinary tool completion produced a
// result. Empty means the tool really executed.
type ToolSyntheticReason string

const (
	ToolSyntheticUnknownTool         ToolSyntheticReason = "unknown_tool"
	ToolSyntheticInvalidCall         ToolSyntheticReason = "invalid_call"
	ToolSyntheticInvalidArguments    ToolSyntheticReason = "invalid_arguments"
	ToolSyntheticModelIncomplete     ToolSyntheticReason = "model_output_incomplete"
	ToolSyntheticPolicyBlocked       ToolSyntheticReason = "policy_blocked"
	ToolSyntheticSteeringBeforeStart ToolSyntheticReason = "steering_before_start"
	ToolSyntheticSteeringInterrupted ToolSyntheticReason = "steering_interrupted"
	ToolSyntheticEffectUnknown       ToolSyntheticReason = "effect_unknown"
)

// ToolResultMetadata is display/durability metadata and never enters model
// content implicitly.
type ToolResultMetadata struct {
	OriginalModelBytes   int    `json:"original_model_bytes"`
	ReturnedModelBytes   int    `json:"returned_model_bytes"`
	OriginalDisplayBytes int    `json:"original_display_bytes"`
	ReturnedDisplayBytes int    `json:"returned_display_bytes"`
	ModelTruncated       bool   `json:"model_truncated"`
	DisplayTruncated     bool   `json:"display_truncated"`
	Target               string `json:"target,omitempty"`
	IdempotencyKey       string `json:"idempotency_key,omitempty"`
}

// ToolResult separates bounded model context from display content and
// structured durability details.
type ToolResult struct {
	ModelContent    string              `json:"model_content"`
	DisplayContent  string              `json:"display_content"`
	Details         json.RawMessage     `json:"details,omitempty"`
	Status          ToolResultStatus    `json:"status"`
	SyntheticReason ToolSyntheticReason `json:"synthetic_reason,omitempty"`
	Metadata        ToolResultMetadata  `json:"metadata"`
}

// TextToolResult constructs the common successful text result.
func TextToolResult(content string) ToolResult {
	return ToolResult{ModelContent: content, DisplayContent: content, Status: ToolResultSuccess}
}

// ToolErrorResult constructs a model-visible execution error.
func ToolErrorResult(modelContent, displayContent string) ToolResult {
	if displayContent == "" {
		displayContent = modelContent
	}
	return ToolResult{ModelContent: modelContent, DisplayContent: displayContent, Status: ToolResultError}
}

// SyntheticToolResult constructs a paired result for a call that did not
// complete normally.
func SyntheticToolResult(status ToolResultStatus, reason ToolSyntheticReason, content string) ToolResult {
	return ToolResult{
		ModelContent: content, DisplayContent: content,
		Status: status, SyntheticReason: reason,
	}
}

// IsError reports the provider/runtime error bit for this outcome.
func (result ToolResult) IsError() bool { return result.Status != ToolResultSuccess }

// NormalizeToolResult validates and bounds a result using its descriptor. It is
// safe to call more than once; metadata is recalculated from visible content.
func NormalizeToolResult(result ToolResult, descriptor ToolDescriptor) (ToolResult, error) {
	if err := descriptor.Validate(); err != nil {
		return ToolResult{}, err
	}
	switch result.Status {
	case ToolResultSuccess, ToolResultError, ToolResultBlocked, ToolResultSkipped:
	default:
		return ToolResult{}, fmt.Errorf("invalid tool result status %q", result.Status)
	}
	switch result.SyntheticReason {
	case "", ToolSyntheticUnknownTool, ToolSyntheticInvalidCall, ToolSyntheticInvalidArguments,
		ToolSyntheticModelIncomplete, ToolSyntheticPolicyBlocked, ToolSyntheticSteeringBeforeStart,
		ToolSyntheticSteeringInterrupted, ToolSyntheticEffectUnknown:
	default:
		return ToolResult{}, fmt.Errorf("invalid tool synthetic reason %q", result.SyntheticReason)
	}
	if result.Status == ToolResultSuccess && result.SyntheticReason != "" {
		return ToolResult{}, errors.New("successful tool result cannot be synthetic")
	}
	if len(result.Details) != 0 {
		if !json.Valid(result.Details) {
			return ToolResult{}, errors.New("tool result details must be valid JSON")
		}
		if len(result.Details) > descriptor.MaxResultBytes {
			return ToolResult{}, fmt.Errorf("tool result details exceed %d bytes", descriptor.MaxResultBytes)
		}
		result.Details = append(json.RawMessage(nil), result.Details...)
	}
	result.ModelContent = strings.ToValidUTF8(result.ModelContent, "\uFFFD")
	result.DisplayContent = strings.ToValidUTF8(result.DisplayContent, "\uFFFD")
	if result.DisplayContent == "" {
		result.DisplayContent = result.ModelContent
	}
	originalModelBytes := max(len(result.ModelContent), result.Metadata.OriginalModelBytes)
	originalDisplayBytes := max(len(result.DisplayContent), result.Metadata.OriginalDisplayBytes)
	result.Metadata.OriginalModelBytes = originalModelBytes
	result.Metadata.OriginalDisplayBytes = originalDisplayBytes
	modelContent, modelTruncated := truncateToolResult(result.ModelContent, descriptor.MaxResultBytes)
	displayContent, displayTruncated := truncateToolResult(result.DisplayContent, descriptor.MaxResultBytes)
	result.ModelContent = modelContent
	result.DisplayContent = displayContent
	result.Metadata.ModelTruncated = result.Metadata.ModelTruncated || modelTruncated
	result.Metadata.DisplayTruncated = result.Metadata.DisplayTruncated || displayTruncated
	result.Metadata.ReturnedModelBytes = len(result.ModelContent)
	result.Metadata.ReturnedDisplayBytes = len(result.DisplayContent)
	return result, nil
}

func truncateToolResult(content string, limit int) (string, bool) {
	if limit <= 0 || len(content) <= limit {
		return content, false
	}
	const suffix = "\n[tool result truncated]"
	end := limit - len(suffix)
	if end <= 0 {
		end = limit
		for end > 0 && !utf8.RuneStart(content[end]) {
			end--
		}
		return content[:end], true
	}
	for end > 0 && !utf8.RuneStart(content[end]) {
		end--
	}
	return strings.TrimRight(content[:end], "\n") + suffix, true
}

type toolProgressSink func(string)
type toolProgressContextKey struct{}

func contextWithToolProgress(ctx context.Context, sink toolProgressSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolProgressContextKey{}, sink)
}

// EmitToolProgress emits one ephemeral display update. Progress never enters
// model context directly.
func EmitToolProgress(ctx context.Context, delta string) bool {
	if ctx == nil || delta == "" {
		return false
	}
	sink, _ := ctx.Value(toolProgressContextKey{}).(toolProgressSink)
	if sink == nil {
		return false
	}
	sink(delta)
	return true
}

type toolSteeringSignal struct {
	done    <-chan struct{}
	pending func() bool
}
type toolSteeringContextKey struct{}

func contextWithToolSteering(ctx context.Context, signal toolSteeringSignal) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolSteeringContextKey{}, signal)
}

// ToolSteeringPending reports a safe preemption without consuming it.
func ToolSteeringPending(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	signal, _ := ctx.Value(toolSteeringContextKey{}).(toolSteeringSignal)
	return signal.pending != nil && signal.pending()
}

// ToolSteeringDone closes when a cancellation or safe preemption is requested.
// A nil channel means the call has no steering controller.
func ToolSteeringDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	signal, _ := ctx.Value(toolSteeringContextKey{}).(toolSteeringSignal)
	return signal.done
}

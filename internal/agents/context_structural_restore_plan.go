package agents

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/contextmaintenance"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

const ContextStructuralRestorePlanVersion = 1

// ContextStructuralDomain names the canonical store that owns a structural
// mutation. It is intentionally narrower than the runtime binding vocabulary:
// context compaction is supported only by Session and Story state.
type ContextStructuralDomain string

const (
	ContextStructuralDomainSession ContextStructuralDomain = "session"
	ContextStructuralDomainStory   ContextStructuralDomain = "story"
)

// ContextStructuralRestorePlan is the bounded deterministic description of an
// already prepared structural operation. Mutation is the exact canonical value
// the host will commit; it must never contain source transcripts or model input.
type ContextStructuralRestorePlan struct {
	Version    int                     `json:"version"`
	Domain     ContextStructuralDomain `json:"domain"`
	Action     ContextStructuralAction `json:"action"`
	Commit     bool                    `json:"commit"`
	IntentHash string                  `json:"intent_hash"`
	RecordID   string                  `json:"record_id"`
	Result     ContextStructuralResult `json:"result"`
	Mutation   json.RawMessage         `json:"mutation"`
}

type contextStructuralRestorePlanWire struct {
	Version    int                               `json:"version"`
	Domain     ContextStructuralDomain           `json:"domain"`
	Action     ContextStructuralAction           `json:"action"`
	Commit     bool                              `json:"commit"`
	IntentHash string                            `json:"intent_hash"`
	RecordID   string                            `json:"record_id"`
	Result     contextStructuralResultDescriptor `json:"result"`
	Mutation   json.RawMessage                   `json:"mutation"`
}

type contextStructuralResultDescriptor struct {
	Compaction contextCompactionResultDescriptor `json:"compaction"`
	Removed    bool                              `json:"removed"`
}

type contextCompactionResultDescriptor struct {
	contextmaintenance.CompactionCheckpoint
	Triggered                bool   `json:"triggered"`
	SkippedReason            string `json:"skipped_reason,omitempty"`
	ProjectedTokensBefore    int    `json:"projected_tokens_before,omitempty"`
	ProjectedTokensAfter     int    `json:"projected_tokens_after,omitempty"`
	ReservedCompletionTokens int    `json:"reserved_completion_tokens,omitempty"`
	ReservedToolResultTokens int    `json:"reserved_tool_result_tokens,omitempty"`
	SourceMessageCount       int    `json:"source_message_count,omitempty"`
	MessageCountBefore       int    `json:"message_count_before,omitempty"`
	MessageCountAfter        int    `json:"message_count_after,omitempty"`
}

func describeContextStructuralResult(result ContextStructuralResult) contextStructuralResultDescriptor {
	compaction := result.Compaction
	return contextStructuralResultDescriptor{
		Compaction: contextCompactionResultDescriptor{
			CompactionCheckpoint: NewContextCompactionCheckpoint("", compaction),
			Triggered:            compaction.Triggered, SkippedReason: compaction.SkippedReason,
			ProjectedTokensBefore: compaction.ProjectedTokensBefore, ProjectedTokensAfter: compaction.ProjectedTokensAfter,
			ReservedCompletionTokens: compaction.ReservedCompletionTokens, ReservedToolResultTokens: compaction.ReservedToolResultTokens,
			SourceMessageCount: compaction.SourceMessageCount, MessageCountBefore: compaction.MessageCountBefore,
			MessageCountAfter: compaction.MessageCountAfter,
		},
		Removed: result.Removed,
	}
}

func (descriptor contextStructuralResultDescriptor) result() ContextStructuralResult {
	compaction := descriptor.Compaction
	result := ContextCompactionResultFromCheckpoint(compaction.CompactionCheckpoint)
	result.Triggered = compaction.Triggered
	result.SkippedReason = compaction.SkippedReason
	result.ProjectedTokensBefore = compaction.ProjectedTokensBefore
	result.ProjectedTokensAfter = compaction.ProjectedTokensAfter
	result.ReservedCompletionTokens = compaction.ReservedCompletionTokens
	result.ReservedToolResultTokens = compaction.ReservedToolResultTokens
	result.SourceMessageCount = compaction.SourceMessageCount
	result.MessageCountBefore = compaction.MessageCountBefore
	result.MessageCountAfter = compaction.MessageCountAfter
	return ContextStructuralResult{
		Compaction: result,
		Removed:    descriptor.Removed,
	}
}

// ContextStructuralIntentHash binds authorization to one action, exact runtime
// binding, expected canonical revision, deterministic record ID, and canonical
// JSON mutation. Invalid JSON is rejected; there is no formatting fallback.
func ContextStructuralIntentHash(
	action ContextStructuralAction,
	binding RuntimeBinding,
	expectedRevision string,
	recordID string,
	mutation json.RawMessage,
) (string, error) {
	ref, err := binding.Ref()
	if err != nil {
		return "", err
	}
	return contextStructuralIntentHash(action, ref, expectedRevision, recordID, mutation)
}

func contextStructuralIntentHash(
	action ContextStructuralAction,
	binding runstate.BindingRef,
	expectedRevision string,
	recordID string,
	mutation json.RawMessage,
) (string, error) {
	if structuralKindForAction(action) == "" {
		return "", fmt.Errorf("unsupported structural context action %q", action)
	}
	if err := validateContextStructuralBindingRef(binding); err != nil {
		return "", err
	}
	expectedRevision = strings.TrimSpace(expectedRevision)
	if expectedRevision == "" {
		return "", fmt.Errorf("structural context expected revision is required")
	}
	limits := runstate.DefaultInputLimits()
	if len(expectedRevision) > limits.MaxContextRefFieldBytes {
		return "", fmt.Errorf("structural context expected revision exceeds %d bytes", limits.MaxContextRefFieldBytes)
	}
	recordID = strings.TrimSpace(recordID)
	if recordID == "" {
		return "", fmt.Errorf("structural context record id is required")
	}
	if len(recordID) > limits.MaxContextRefFieldBytes {
		return "", fmt.Errorf("structural context record id exceeds %d bytes", limits.MaxContextRefFieldBytes)
	}
	if len(mutation) > limits.MaxRestoreDescriptorBytes {
		return "", fmt.Errorf("structural context mutation exceeds %d bytes", limits.MaxRestoreDescriptorBytes)
	}
	canonicalMutation, err := canonicalContextStructuralMutation(mutation)
	if err != nil {
		return "", err
	}
	envelope := struct {
		Action           ContextStructuralAction `json:"action"`
		Binding          runstate.BindingRef     `json:"binding"`
		ExpectedRevision string                  `json:"expected_revision"`
		RecordID         string                  `json:"record_id"`
		Mutation         json.RawMessage         `json:"mutation"`
	}{
		Action: action, Binding: binding, ExpectedRevision: expectedRevision,
		RecordID: recordID, Mutation: canonicalMutation,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode structural context intent: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// EncodeContextStructuralRestorePlan validates and encodes a plan under the
// production restore-descriptor bound. The runtime repeats the bound check at
// command admission so custom smaller Runtime limits remain authoritative.
func EncodeContextStructuralRestorePlan(
	plan ContextStructuralRestorePlan,
	binding RuntimeBinding,
	expectedRevision string,
) (json.RawMessage, error) {
	ref, err := binding.Ref()
	if err != nil {
		return nil, err
	}
	return encodeContextStructuralRestorePlan(plan, ref, expectedRevision, runstate.DefaultInputLimits().MaxRestoreDescriptorBytes)
}

func encodeContextStructuralRestorePlan(
	plan ContextStructuralRestorePlan,
	binding runstate.BindingRef,
	expectedRevision string,
	maxBytes int,
) (json.RawMessage, error) {
	if maxBytes <= 0 {
		maxBytes = runstate.DefaultInputLimits().MaxRestoreDescriptorBytes
	}
	if contextStructuralRestorePlanDeclaredBytes(plan) > int64(maxBytes) {
		return nil, fmt.Errorf("structural context restore plan exceeds %d bytes", maxBytes)
	}
	normalized, err := validateContextStructuralRestorePlan(plan, binding, expectedRevision)
	if err != nil {
		return nil, err
	}
	wire := contextStructuralRestorePlanWire{
		Version: normalized.Version, Domain: normalized.Domain, Action: normalized.Action,
		Commit: normalized.Commit, IntentHash: normalized.IntentHash, RecordID: normalized.RecordID,
		Result: describeContextStructuralResult(normalized.Result), Mutation: normalized.Mutation,
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("encode structural context restore plan: %w", err)
	}
	if len(encoded) > maxBytes {
		return nil, fmt.Errorf("structural context restore plan exceeds %d bytes", maxBytes)
	}
	return encoded, nil
}

func contextStructuralRestorePlanDeclaredBytes(plan ContextStructuralRestorePlan) int64 {
	compaction := plan.Result.Compaction
	return int64(
		len(plan.Domain) + len(plan.Action) + len(plan.IntentHash) + len(plan.RecordID) + len(plan.Mutation) +
			len(compaction.SkippedReason) + len(compaction.Phase) + len(compaction.Strategy) + len(compaction.Summary),
	)
}

// DecodeContextStructuralRestorePlan rejects unknown fields and validates the
// descriptor against the immutable binding/revision carried by its snapshot.
func DecodeContextStructuralRestorePlan(
	descriptor json.RawMessage,
	binding RuntimeBinding,
	expectedRevision string,
) (ContextStructuralRestorePlan, error) {
	ref, err := binding.Ref()
	if err != nil {
		return ContextStructuralRestorePlan{}, err
	}
	return decodeContextStructuralRestorePlan(descriptor, ref, expectedRevision)
}

func decodeContextStructuralRestorePlan(
	descriptor json.RawMessage,
	binding runstate.BindingRef,
	expectedRevision string,
) (ContextStructuralRestorePlan, error) {
	if len(descriptor) == 0 {
		return ContextStructuralRestorePlan{}, fmt.Errorf("structural context restore descriptor is absent")
	}
	if len(descriptor) > runstate.DefaultInputLimits().MaxRestoreDescriptorBytes {
		return ContextStructuralRestorePlan{}, fmt.Errorf(
			"structural context restore descriptor exceeds %d bytes",
			runstate.DefaultInputLimits().MaxRestoreDescriptorBytes,
		)
	}
	var wire contextStructuralRestorePlanWire
	if err := decodeStrictContextStructuralJSON(descriptor, &wire); err != nil {
		return ContextStructuralRestorePlan{}, fmt.Errorf("decode structural context restore plan: %w", err)
	}
	if err := validateContextStructuralRestorePlanRequiredFields(descriptor); err != nil {
		return ContextStructuralRestorePlan{}, fmt.Errorf("decode structural context restore plan: %w", err)
	}
	plan := ContextStructuralRestorePlan{
		Version: wire.Version, Domain: wire.Domain, Action: wire.Action, Commit: wire.Commit,
		IntentHash: wire.IntentHash, RecordID: wire.RecordID, Result: wire.Result.result(), Mutation: wire.Mutation,
	}
	return validateContextStructuralRestorePlan(plan, binding, expectedRevision)
}

func validateContextStructuralRestorePlanRequiredFields(descriptor json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(descriptor, &fields); err != nil {
		return err
	}
	for _, name := range []string{"version", "domain", "action", "commit", "intent_hash", "record_id", "result", "mutation"} {
		if err := requireContextStructuralJSONField(fields, name); err != nil {
			return err
		}
	}
	var resultFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["result"], &resultFields); err != nil {
		return fmt.Errorf("result must be a JSON object: %w", err)
	}
	for _, name := range []string{"compaction", "removed"} {
		if err := requireContextStructuralJSONField(resultFields, name); err != nil {
			return fmt.Errorf("result: %w", err)
		}
	}
	var compactionFields map[string]json.RawMessage
	if err := json.Unmarshal(resultFields["compaction"], &compactionFields); err != nil {
		return fmt.Errorf("result.compaction must be a JSON object: %w", err)
	}
	if err := requireContextStructuralJSONField(compactionFields, "triggered"); err != nil {
		return fmt.Errorf("result.compaction: %w", err)
	}
	return nil
}

func requireContextStructuralJSONField(fields map[string]json.RawMessage, name string) error {
	value, ok := fields[name]
	if !ok {
		return fmt.Errorf("required field %q is absent", name)
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return fmt.Errorf("required field %q must not be null", name)
	}
	return nil
}

func validateContextStructuralRestorePlan(
	plan ContextStructuralRestorePlan,
	binding runstate.BindingRef,
	expectedRevision string,
) (ContextStructuralRestorePlan, error) {
	if plan.Version != ContextStructuralRestorePlanVersion {
		return ContextStructuralRestorePlan{}, fmt.Errorf("unsupported structural context restore plan version %d", plan.Version)
	}
	if err := validateContextStructuralBindingRef(binding); err != nil {
		return ContextStructuralRestorePlan{}, err
	}
	productBinding, err := ParseRuntimeBinding(binding)
	if err != nil {
		return ContextStructuralRestorePlan{}, err
	}
	switch plan.Domain {
	case ContextStructuralDomainSession:
		if productBinding.AgentKind != AgentKindGeneral && productBinding.AgentKind != AgentKindIDE && productBinding.AgentKind != AgentKindConfigManager && productBinding.AgentKind != AgentKindImage {
			return ContextStructuralRestorePlan{}, fmt.Errorf("session structural plan does not match %q binding", binding.Kind)
		}
	case ContextStructuralDomainStory:
		if productBinding.AgentKind != AgentKindInteractiveStory && productBinding.AgentKind != config.AgentKindInteractiveDirector {
			return ContextStructuralRestorePlan{}, fmt.Errorf("story structural plan does not match %q binding", binding.Kind)
		}
	default:
		return ContextStructuralRestorePlan{}, fmt.Errorf("unsupported structural context domain %q", plan.Domain)
	}
	if structuralKindForAction(plan.Action) == "" {
		return ContextStructuralRestorePlan{}, fmt.Errorf("unsupported structural context action %q", plan.Action)
	}
	switch plan.Action {
	case ContextStructuralCompact:
		if plan.Result.Removed || plan.Result.Compaction.Triggered != plan.Commit {
			return ContextStructuralRestorePlan{}, fmt.Errorf("compact structural plan commit/result mismatch")
		}
	case ContextStructuralRemove:
		if plan.Result.Removed != plan.Commit || plan.Result.Compaction != (ContextCompactionResult{}) {
			return ContextStructuralRestorePlan{}, fmt.Errorf("remove structural plan commit/result mismatch")
		}
	}
	plan.RecordID = strings.TrimSpace(plan.RecordID)
	plan.IntentHash = strings.TrimSpace(plan.IntentHash)
	canonicalMutation, err := canonicalContextStructuralMutation(plan.Mutation)
	if err != nil {
		return ContextStructuralRestorePlan{}, err
	}
	plan.Mutation = canonicalMutation
	wantHash, err := contextStructuralIntentHash(plan.Action, binding, expectedRevision, plan.RecordID, plan.Mutation)
	if err != nil {
		return ContextStructuralRestorePlan{}, err
	}
	if plan.IntentHash == "" || plan.IntentHash != wantHash {
		return ContextStructuralRestorePlan{}, fmt.Errorf("structural context restore plan intent hash mismatch")
	}
	return plan, nil
}

func validateContextStructuralBindingRef(binding runstate.BindingRef) error {
	productBinding, err := ParseRuntimeBinding(binding)
	if err != nil {
		return fmt.Errorf("validate structural context binding: %w", err)
	}
	if productBinding.AgentKind == AgentKindAutomation {
		return fmt.Errorf("automation bindings do not support structural context operations")
	}
	return nil
}

package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const maxHostEffectKindBytes = 4 << 10

// NewToolHostEffect constructs the only accepted stable identity for a tool
// host-effect outbox item. Adapters must use the exact EngineRequest binding,
// operation, cycle, and tool call identity; Harness validates the result again
// at the durable ToolCallFinished transaction.
func NewToolHostEffect(
	binding BindingRef,
	operationID OperationID,
	cycle int,
	callID string,
	index int,
	kind HostEffectKind,
	payload json.RawMessage,
) (HostEffect, error) {
	binding = binding.Clone()
	if err := ValidateBindingRef(binding); err != nil {
		return HostEffect{}, err
	}
	if strings.TrimSpace(string(operationID)) == "" || cycle <= 0 || strings.TrimSpace(callID) == "" || index < 0 {
		return HostEffect{}, fmt.Errorf("%w: host effect requires operation, cycle, call id, and non-negative index", ErrInvalidCommand)
	}
	if strings.TrimSpace(string(kind)) == "" || len(kind) > maxHostEffectKindBytes {
		return HostEffect{}, fmt.Errorf("%w: host effect kind is invalid", ErrInvalidCommand)
	}
	if len(payload) == 0 || !json.Valid(payload) {
		return HostEffect{}, fmt.Errorf("%w: host effect payload must be non-empty valid JSON", ErrInvalidCommand)
	}
	id := toolHostEffectID(binding, operationID, cycle, callID, index, kind)
	cloned := append(json.RawMessage(nil), payload...)
	return HostEffect{
		ID: id, Kind: kind, OperationID: operationID, Cycle: cycle, CallID: callID, Index: index,
		Payload: cloned, PayloadDescriptor: describePayload(cloned),
	}, nil
}

func toolHostEffectID(binding BindingRef, operationID OperationID, cycle int, callID string, index int, kind HostEffectKind) HostEffectID {
	identity := struct {
		Version     int            `json:"version"`
		Binding     BindingRef     `json:"binding"`
		OperationID OperationID    `json:"operation_id"`
		Cycle       int            `json:"cycle"`
		CallID      string         `json:"call_id"`
		Index       int            `json:"index"`
		Kind        HostEffectKind `json:"kind"`
	}{
		Version: 1, Binding: binding, OperationID: operationID, Cycle: cycle,
		CallID: callID, Index: index, Kind: kind,
	}
	encoded, _ := json.Marshal(identity)
	digest := sha256.Sum256(encoded)
	return HostEffectID("host-effect-" + hex.EncodeToString(digest[:]))
}

func cloneHostEffect(effect HostEffect) HostEffect {
	effect.Payload = append(json.RawMessage(nil), effect.Payload...)
	return effect
}

func hostEffectSnapshot(effect HostEffect) HostEffectSnapshot {
	return HostEffectSnapshot{
		ID: effect.ID, Kind: effect.Kind, OperationID: effect.OperationID, Cycle: effect.Cycle,
		CallID: effect.CallID, Index: effect.Index, PayloadDescriptor: effect.PayloadDescriptor,
	}
}

func validateHostEffect(binding BindingRef, effect HostEffect, limits BindingMemoryLimits) error {
	limits = limits.normalized()
	if strings.TrimSpace(string(effect.Kind)) == "" || len(effect.Kind) > maxHostEffectKindBytes {
		return fmt.Errorf("%w: host effect kind is invalid", ErrInvalidCommand)
	}
	if strings.TrimSpace(string(effect.OperationID)) == "" || effect.Cycle <= 0 || strings.TrimSpace(effect.CallID) == "" || effect.Index < 0 {
		return fmt.Errorf("%w: host effect identity is incomplete", ErrInvalidCommand)
	}
	if len(effect.Payload) == 0 || !json.Valid(effect.Payload) {
		return fmt.Errorf("%w: host effect payload must be non-empty valid JSON", ErrInvalidCommand)
	}
	if int64(len(effect.Payload)) > limits.MaxHostEffectBytes {
		return &ByteBudgetError{Scope: ByteBudgetHostEffect, Incoming: int64(len(effect.Payload)), Limit: limits.MaxHostEffectBytes}
	}
	wantID := toolHostEffectID(binding, effect.OperationID, effect.Cycle, effect.CallID, effect.Index, effect.Kind)
	if effect.ID != wantID {
		return fmt.Errorf("%w: host effect id does not match its stable runtime identity", ErrInvalidCommand)
	}
	wantDescriptor := describePayload(effect.Payload)
	if effect.PayloadDescriptor != wantDescriptor {
		return fmt.Errorf("%w: host effect payload descriptor mismatch", ErrInvalidCommand)
	}
	return nil
}

func reconcileHostEffect(ctx context.Context, reconciler EngineHostEffectReconciler, effect HostEffect) (resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("reconcile host effect %q panic: %v", effect.ID, recovered)
		}
	}()
	return reconciler.ReconcileHostEffect(ctx, cloneHostEffect(effect))
}

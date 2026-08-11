package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const maxInteractionIDBytes = 4 << 10

func validateInteractionID(id string) error {
	if strings.TrimSpace(id) != id || id == "" || len(id) > maxInteractionIDBytes {
		return fmt.Errorf("%w: invalid interaction id", ErrInvalidCommand)
	}
	return nil
}

func (s *harnessState) validateInteractionRequest(event InteractionRequestedEvent) error {
	if err := validateInteractionID(event.ID); err != nil {
		return err
	}
	if event.OperationID != s.activeOperation || event.Cycle != s.activeCycle || s.phase != PhaseRunning {
		return fmt.Errorf("%w: interaction does not match the active cycle", ErrInteractionStale)
	}
	if strings.TrimSpace(event.ToolCallID) == "" || len(event.ToolCallID) > maxInteractionIDBytes {
		return fmt.Errorf("%w: interaction tool call id is invalid", ErrInvalidCommand)
	}
	if len(event.Request) == 0 || !json.Valid(event.Request) || event.Descriptor != describePayload(event.Request) {
		return fmt.Errorf("%w: interaction request is invalid", ErrInvalidCommand)
	}
	limits := s.memoryLimits.normalized()
	if int64(len(event.Request)) > limits.MaxInteractionBytes {
		return &ByteBudgetError{Scope: ByteBudgetInteraction, Incoming: int64(len(event.Request)), Limit: limits.MaxInteractionBytes}
	}
	if current, exists := s.interactions[event.ID]; exists {
		if current.OperationID == event.OperationID && current.Cycle == event.Cycle && current.ToolCallID == event.ToolCallID &&
			string(current.Request) == string(event.Request) {
			return nil
		}
		return fmt.Errorf("%w: interaction id already belongs to a different request", ErrInvalidCommand)
	}
	if len(s.interactions) >= limits.MaxPendingInteractions {
		return &ByteBudgetError{Scope: ByteBudgetInteraction, Current: int64(len(s.interactions)), Incoming: 1, Limit: int64(limits.MaxPendingInteractions)}
	}
	return nil
}

func (s *harnessState) validateInteractionResolution(event InteractionResolvedEvent) error {
	if err := validateInteractionID(event.ID); err != nil {
		return err
	}
	current, exists := s.interactions[event.ID]
	if !exists || current.OperationID != event.OperationID || current.Cycle != event.Cycle || event.OperationID != s.activeOperation {
		return ErrInteractionStale
	}
	if current.Resolved {
		if string(current.Response) == string(event.Response) {
			return nil
		}
		return ErrInteractionStale
	}
	if len(event.Response) == 0 || !json.Valid(event.Response) || event.ResponseDescriptor != describePayload(event.Response) {
		return fmt.Errorf("%w: interaction response is invalid", ErrInvalidCommand)
	}
	limit := s.memoryLimits.normalized().MaxInteractionBytes
	if int64(len(event.Response)) > limit {
		return &ByteBudgetError{Scope: ByteBudgetInteraction, Incoming: int64(len(event.Response)), Limit: limit}
	}
	return nil
}

func (s *harnessState) hasInteractionForTool(callID string) bool {
	for _, interaction := range s.interactions {
		if interaction.ToolCallID == callID && interaction.OperationID == s.activeOperation && interaction.Cycle == s.activeCycle {
			return true
		}
	}
	return false
}

func (s *harnessState) removeInteractionsForTool(callID string) {
	for id, interaction := range s.interactions {
		if interaction.ToolCallID == callID {
			delete(s.interactions, id)
		}
	}
}

func interactionPayloadBytes(value InteractionSnapshot) int64 {
	return retainedObjectOverhead + int64(len(value.ID)+len(value.OperationID)+len(value.ToolCallID)+len(value.Request)+len(value.Response))
}

func (s *harnessState) interactionBytes() int64 {
	var total int64
	for _, value := range s.interactions {
		total += interactionPayloadBytes(value)
	}
	return total
}

func (h *Harness) resolveInteraction(
	ctx context.Context,
	state *harnessState,
	command ResolveInteraction,
	fingerprint string,
) (Receipt, error) {
	if state.phase != PhaseRunning || command.OperationID == "" || command.OperationID != state.activeOperation {
		return Receipt{}, ErrInteractionStale
	}
	interaction, exists := state.interactions[command.InteractionID]
	if !exists || interaction.OperationID != state.activeOperation || interaction.Cycle != state.activeCycle || interaction.Resolved {
		return Receipt{}, ErrInteractionStale
	}
	limit := state.memoryLimits.normalized().MaxInteractionBytes
	if int64(len(command.Response)) > limit {
		return Receipt{}, &ByteBudgetError{Scope: ByteBudgetInteraction, Incoming: int64(len(command.Response)), Limit: limit}
	}
	resolver, ok := h.engine.(EngineInteractionResolver)
	if !ok {
		return Receipt{}, fmt.Errorf("%w: Engine does not support interaction resolution", ErrInvalidCommand)
	}
	normalized, err := resolver.ResolveInteraction(ctx, InteractionResolveRequest{
		Snapshot: state.turnSnapshot(state.activeSnapshotID), Interaction: cloneInteractionSnapshot(interaction),
		Response: cloneRawMessage(command.Response),
	})
	if err != nil {
		return Receipt{}, err
	}
	if len(normalized) == 0 || !json.Valid(normalized) {
		return Receipt{}, fmt.Errorf("%w: Engine returned an invalid interaction resolution", ErrInvalidCommand)
	}
	if int64(len(normalized)) > limit {
		return Receipt{}, &ByteBudgetError{Scope: ByteBudgetInteraction, Incoming: int64(len(normalized)), Limit: limit}
	}
	payload := InteractionResolvedEvent{
		ID: interaction.ID, OperationID: interaction.OperationID, Cycle: interaction.Cycle,
		Response: cloneRawMessage(normalized), ResponseDescriptor: describePayload(normalized),
	}
	if err := state.validateInteractionResolution(payload); err != nil {
		return Receipt{}, err
	}
	committed, err := h.commit(ctx, state, []EventPayload{
		CommandAcceptedEvent{CommandID: command.ID, CommandKind: "resolve_interaction", OperationID: state.activeOperation, Fingerprint: fingerprint},
		payload,
	})
	if err != nil {
		return Receipt{}, err
	}
	receipt := receiptFromEvents(committed)
	if state.engineControls != nil {
		state.sendControl(EngineControl{
			Kind: EngineControlInteractionResolved, InteractionID: interaction.ID, Response: cloneRawMessage(normalized),
		})
		return receipt, nil
	}
	if state.recoveryPaused {
		if err := h.resumeResolvedInteraction(state, interaction.ID); err != nil {
			return receipt, err
		}
		return receipt, nil
	}
	// A committed resolution without a live control lane or recovery pause can
	// occur only while the Engine is crossing its completion boundary. The
	// durable receipt is still authoritative; never report a post-commit error
	// that would encourage the host to manufacture a second response.
	return receipt, nil
}

func (h *Harness) resumeResolvedInteraction(state *harnessState, id string) error {
	interaction, exists := state.interactions[id]
	if !state.recoveryPaused || state.engineControls != nil || !exists || !interaction.Resolved ||
		interaction.OperationID != state.activeOperation || interaction.Cycle != state.activeCycle {
		return nil
	}
	if _, err := h.commit(context.Background(), state, []EventPayload{InteractionRecoveryResumedEvent{
		ID: id, OperationID: interaction.OperationID, Cycle: interaction.Cycle,
	}}); err != nil {
		return err
	}
	h.startEngine(state, state.turnSnapshot(state.activeSnapshotID))
	return nil
}

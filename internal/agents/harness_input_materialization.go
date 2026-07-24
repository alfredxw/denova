package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/agents/session"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// HarnessInputMaterializationRequest is the provider-free semantic projection
// of one accepted runtime cycle. It is rebuilt solely from the durable v2 turn
// descriptor; process-local Runner, Conversation, and resolved prompt context
// are intentionally absent.
type HarnessInputMaterializationRequest struct {
	Binding        runstate.BindingRef
	Identity       HarnessCycleIdentity
	CommandKind    AgentCommandKind
	AgentKind      string
	RootAgentName  string
	Message        string
	Request        ChatRequest
	UserReferences []session.UserMessageReference
}

// HarnessInputMaterializer owns the canonical Session/Story input append. Plan
// must be pure. Materialize must be idempotent for the exact identity and plan
// hash because the runtime invokes it again only after a lost receipt cannot be
// proven by reconciliation.
type HarnessInputMaterializer interface {
	PlanHarnessInputMaterialization(context.Context, HarnessInputMaterializationRequest) (runstate.InputMaterializationPlan, error)
	MaterializeHarnessInput(context.Context, HarnessInputMaterializationRequest, runstate.InputMaterializationPlan) (runstate.InputMaterializationReceipt, error)
}

func (e *bindingHarnessEngine) PlanInputMaterialization(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	materialization, err := e.acceptedInputMaterializationRequest(request)
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	if e.owner.inputMaterializer == nil {
		return runstate.InputMaterializationPlan{}, nil
	}
	return e.owner.inputMaterializer.PlanHarnessInputMaterialization(ctx, materialization)
}

func (e *bindingHarnessEngine) MaterializeInput(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	materialization, err := e.acceptedInputMaterializationRequest(request)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	if e.owner.inputMaterializer == nil {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("agent harness input materializer is unavailable")
	}
	return e.owner.inputMaterializer.MaterializeHarnessInput(ctx, materialization, plan)
}

func (e *bindingHarnessEngine) acceptedInputMaterializationRequest(
	request runstate.InputMaterializationRequest,
) (HarnessInputMaterializationRequest, error) {
	if e == nil || e.owner == nil {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("materialize agent harness input: engine is nil")
	}
	if !request.Binding.Equal(e.binding) || !request.Snapshot.Binding.Equal(e.binding) {
		return HarnessInputMaterializationRequest{}, fmt.Errorf(
			"%w: factory=%+v request=%+v snapshot=%+v",
			ErrHarnessBindingMismatch,
			e.binding,
			request.Binding,
			request.Snapshot.Binding,
		)
	}
	return decodeHarnessInputMaterializationRequest(e.binding, request.Snapshot)
}

func decodeHarnessInputMaterializationRequest(
	binding runstate.BindingRef,
	snapshot runstate.TurnSnapshot,
) (HarnessInputMaterializationRequest, error) {
	if len(snapshot.Input.RestoreDescriptor) == 0 {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor is absent")
	}
	var descriptor harnessTurnRestoreDescriptor
	if err := json.Unmarshal(snapshot.Input.RestoreDescriptor, &descriptor); err != nil {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("decode accepted input durable descriptor: %w", err)
	}
	if descriptor.Version != harnessTurnRestoreDescriptorVersion {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("unsupported accepted input durable descriptor version %d", descriptor.Version)
	}
	switch descriptor.Kind {
	case AgentCommandStartTurn, AgentCommandSteer, AgentCommandFollowUp, AgentCommandNextTurn:
	default:
		return HarnessInputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor has unsupported command kind %q", descriptor.Kind)
	}
	request := CaptureChatRequestCallerInput(descriptor.Request.chatRequest())
	caller := chatRequestCallerView(request)
	if caller.Message != snapshot.Input.Text {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor message does not match runtime input")
	}
	options := descriptor.Options.runOptions()
	resolvedBinding, err := harnessBindingForOptions(options)
	if err != nil {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor binding: %w", err)
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("%w: durable input descriptor does not match runtime binding", ErrHarnessBindingMismatch)
	}
	if strings.TrimSpace(string(snapshot.CommandID)) == "" || strings.TrimSpace(string(snapshot.OperationID)) == "" || snapshot.Cycle <= 0 {
		return HarnessInputMaterializationRequest{}, fmt.Errorf("accepted input snapshot has incomplete durable identity")
	}
	return HarnessInputMaterializationRequest{
		Binding: binding,
		Identity: HarnessCycleIdentity{
			CommandID: snapshot.CommandID, OperationID: snapshot.OperationID, Cycle: snapshot.Cycle,
		},
		CommandKind:    descriptor.Kind,
		AgentKind:      descriptor.Options.AgentKind,
		RootAgentName:  descriptor.Options.RootAgentName,
		Message:        caller.Message,
		Request:        request,
		UserReferences: userMessageReferencesForRequest(request),
	}, nil
}

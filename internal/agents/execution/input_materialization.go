package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"encoding/json"
	"fmt"
	"strings"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/run"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// InputMaterializationRequest is the provider-free semantic projection
// of one accepted runtime cycle. It is rebuilt solely from the durable v2 turn
// descriptor; process-local Runner, Conversation, and resolved prompt context
// are intentionally absent.
type InputMaterializationRequest struct {
	Binding        agentrun.RuntimeBinding
	Identity       agentrun.CycleIdentity
	CommandKind    CommandKind
	AgentKind      string
	RootAgentName  string
	Message        string
	Request        agentchat.ChatRequest
	UserReferences []agentcontext.UserReference
}

// InputMaterializer owns the canonical Session/Story input append. Plan
// must be pure. Materialize must be idempotent for the exact identity and plan
// hash because the runtime invokes it again only after a lost receipt cannot be
// proven by reconciliation.
type InputMaterializer interface {
	PlanInput(context.Context, InputMaterializationRequest) (agentrun.InputMaterializationPlan, error)
	MaterializeInput(context.Context, InputMaterializationRequest, agentrun.InputMaterializationPlan) (agentrun.InputMaterializationReceipt, error)
}

func (e *bindingEngine) PlanInputMaterialization(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	materialization, err := e.acceptedInputMaterializationRequest(request)
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	if e.owner.profiles.empty() {
		return runstate.InputMaterializationPlan{}, nil
	}
	profile, err := e.owner.profiles.profile(e.binding.Profile)
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	input, ok := profile.(InputProfile)
	if !ok {
		return runstate.InputMaterializationPlan{}, nil
	}
	plan, err := input.PlanInput(ctx, materialization)
	return agentrun.InputMaterializationPlanToRuntime(plan), err
}

func (e *bindingEngine) MaterializeInput(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	materialization, err := e.acceptedInputMaterializationRequest(request)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	profile, err := e.owner.profiles.profile(e.binding.Profile)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	input, ok := profile.(InputProfile)
	if !ok {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("%w: profile %q", ErrInputMaterializationUnavailable, profile.ID())
	}
	receipt, err := input.MaterializeInput(ctx, materialization, agentrun.InputMaterializationPlanFromRuntime(plan))
	return agentrun.InputMaterializationReceiptToRuntime(receipt), err
}

func (e *bindingEngine) acceptedInputMaterializationRequest(
	request runstate.InputMaterializationRequest,
) (InputMaterializationRequest, error) {
	if e == nil || e.owner == nil {
		return InputMaterializationRequest{}, fmt.Errorf("materialize agent execution input: engine is nil")
	}
	if !request.Binding.Equal(e.binding) || !request.Snapshot.Binding.Equal(e.binding) {
		return InputMaterializationRequest{}, fmt.Errorf(
			"%w: factory=%+v request=%+v snapshot=%+v",
			ErrBindingMismatch,
			e.binding,
			request.Binding,
			request.Snapshot.Binding,
		)
	}
	return decodeInputMaterializationRequest(e.binding, request.Snapshot)
}

func decodeInputMaterializationRequest(
	binding runstate.BindingRef,
	snapshot runstate.TurnSnapshot,
) (InputMaterializationRequest, error) {
	if len(snapshot.Input.RestoreDescriptor) == 0 {
		return InputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor is absent")
	}
	var descriptor cycleRestoreDescriptor
	if err := json.Unmarshal(snapshot.Input.RestoreDescriptor, &descriptor); err != nil {
		return InputMaterializationRequest{}, fmt.Errorf("decode accepted input durable descriptor: %w", err)
	}
	if descriptor.Version != cycleRestoreDescriptorVersion {
		return InputMaterializationRequest{}, fmt.Errorf("unsupported accepted input durable descriptor version %d", descriptor.Version)
	}
	switch descriptor.Kind {
	case CommandStartTurn, CommandSteer, CommandFollowUp, CommandNextTurn:
	default:
		return InputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor has unsupported command kind %q", descriptor.Kind)
	}
	request := agentchat.CaptureChatRequestCallerInput(descriptor.Request.chatRequest())
	caller := agentchat.CallerView(request)
	if caller.Message != snapshot.Input.Text {
		return InputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor message does not match runtime input")
	}
	options := descriptor.Options.runOptions()
	resolvedBinding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return InputMaterializationRequest{}, fmt.Errorf("accepted input durable descriptor binding: %w", err)
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return InputMaterializationRequest{}, fmt.Errorf("%w: durable input descriptor does not match runtime binding", ErrBindingMismatch)
	}
	if strings.TrimSpace(string(snapshot.CommandID)) == "" || strings.TrimSpace(string(snapshot.OperationID)) == "" || snapshot.Cycle <= 0 {
		return InputMaterializationRequest{}, fmt.Errorf("accepted input snapshot has incomplete durable identity")
	}
	productBinding, err := agentrun.ParseRuntimeBinding(binding)
	if err != nil {
		return InputMaterializationRequest{}, err
	}
	return InputMaterializationRequest{
		Binding: productBinding,
		Identity: agentrun.CycleIdentity{
			CommandID: agentrun.CommandID(snapshot.CommandID), OperationID: agentrun.OperationID(snapshot.OperationID), Cycle: snapshot.Cycle,
		},
		CommandKind:    descriptor.Kind,
		AgentKind:      descriptor.Options.AgentKind,
		RootAgentName:  descriptor.Options.RootAgentName,
		Message:        caller.Message,
		Request:        request,
		UserReferences: agentchat.UserMessageReferencesForRequest(request),
	}, nil
}

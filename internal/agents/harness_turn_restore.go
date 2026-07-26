package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

const harnessTurnRestoreDescriptorVersion = 2

// ErrHarnessTurnRestoreUnavailable means an accepted queued command is still
// durable, but this process cannot yet rebuild its Runner/Conversation state.
// Replaying the exact command or reopening with WithHarnessTurnRestorer retries
// the same queued operation; the runtime never converts this into cancellation.
var ErrHarnessTurnRestoreUnavailable = errors.New("agent harness turn restore dependency is unavailable")

// HarnessTurnRestoreRequest is the stable semantic input reconstructed from a
// queued Steer, FollowUp, or NextTurn bounded durable descriptor. The host resolves fresh
// Runner, Conversation, BookService, callbacks, and server-owned context.
type HarnessTurnRestoreRequest struct {
	Binding          RuntimeBinding
	Kind             AgentCommandKind
	CommandID        CommandID
	OperationID      OperationID
	AfterOperationID OperationID
	Request          ChatRequest
	Options          RunOptions
	Deferred         bool
	// Emit is process-local display routing supplied only by the explicit
	// recovery observation. It is never encoded in the durable descriptor.
	Emit func(Event)
}

// HarnessTurnRestorer rebuilds process-local execution dependencies for one
// already accepted durable queued command. Implementations must be idempotent for the
// supplied CommandID and must not perform model or tool effects.
type HarnessTurnRestorer func(context.Context, HarnessTurnRestoreRequest) (HarnessTurnSpec, error)

type harnessTurnRestoreDescriptor struct {
	Version          int                          `json:"version"`
	Kind             AgentCommandKind             `json:"kind"`
	AfterOperationID runstate.OperationID         `json:"after_operation_id"`
	Request          harnessTurnRequestDescriptor `json:"request"`
	Options          harnessTurnOptionsDescriptor `json:"options"`
	Deferred         bool                         `json:"deferred"`
}

type harnessTurnRequestDescriptor struct {
	Message        string             `json:"message"`
	References     []string           `json:"references,omitempty"`
	LoreReferences []string           `json:"lore_references,omitempty"`
	StyleScenes    []string           `json:"style_scenes,omitempty"`
	Selections     []TextSelectionRef `json:"selections,omitempty"`
	IDEContext     IDEContextRef      `json:"ide_context,omitempty"`
	ReviewFeedback ReviewFeedbackRefs `json:"review_feedback,omitempty"`
	PlanMode       bool               `json:"plan_mode,omitempty"`
	WritingSkill   string             `json:"writing_skill,omitempty"`
	ImagePresetID  string             `json:"image_preset_id,omitempty"`
	TellerID       string             `json:"teller_id,omitempty"`
	Locale         string             `json:"locale,omitempty"`
}

func encodeHarnessTurnRestoreDescriptor(spec AgentCommandSpec) (json.RawMessage, error) {
	descriptor := harnessTurnRestoreDescriptor{
		Version:          harnessTurnRestoreDescriptorVersion,
		Kind:             spec.Kind,
		AfterOperationID: runstate.OperationID(spec.AfterOperationID),
		Request:          describeHarnessTurnRequest(spec.Request),
		Options:          describeHarnessDurableTurnOptions(spec.Options),
		Deferred:         spec.Prepare != nil,
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return nil, fmt.Errorf("encode agent harness turn restore descriptor: %w", err)
	}
	return encoded, nil
}

// withHarnessInputMaterializationDescriptor freezes the provider-free,
// caller-owned semantics needed to materialize canonical input after durable
// command acceptance. The runtime bounds this JSON through
// InputLimits.MaxRestoreDescriptorBytes (8 MiB by default), comfortably above
// the 128 KiB product context floor without making the journal unbounded.
func withHarnessInputMaterializationDescriptor(
	input runstate.UserInput,
	spec AgentCommandSpec,
) (runstate.UserInput, error) {
	encoded, err := encodeHarnessTurnRestoreDescriptor(spec)
	if err != nil {
		return runstate.UserInput{}, err
	}
	input.RestoreDescriptor = encoded
	return input, nil
}

func decodeHarnessTurnRestoreRequest(
	binding runstate.BindingRef,
	input runstate.QueuedInput,
) (HarnessTurnRestoreRequest, error) {
	if len(input.Input.RestoreDescriptor) == 0 {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor is absent", ErrHarnessTurnRestoreUnavailable)
	}
	var descriptor harnessTurnRestoreDescriptor
	if err := json.Unmarshal(input.Input.RestoreDescriptor, &descriptor); err != nil {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: decode durable descriptor: %v", ErrHarnessTurnRestoreUnavailable, err)
	}
	if descriptor.Version != harnessTurnRestoreDescriptorVersion {
		return HarnessTurnRestoreRequest{}, fmt.Errorf(
			"%w: unsupported durable descriptor version %d",
			ErrHarnessTurnRestoreUnavailable,
			descriptor.Version,
		)
	}
	expectedKind, err := agentCommandKindForQueuedDelivery(input.Delivery)
	if err != nil {
		return HarnessTurnRestoreRequest{}, err
	}
	if descriptor.Kind != expectedKind || (expectedKind == AgentCommandNextTurn && descriptor.AfterOperationID == "") {
		return HarnessTurnRestoreRequest{}, fmt.Errorf(
			"%w: durable descriptor kind %q does not match queued delivery %q",
			ErrHarnessTurnRestoreUnavailable,
			descriptor.Kind,
			input.Delivery,
		)
	}
	request := descriptor.Request.chatRequest()
	if request.Message != input.Input.Text {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor message does not match queued input", ErrHarnessTurnRestoreUnavailable)
	}
	options := descriptor.Options.runOptions().normalized(descriptor.Options.Workspace)
	resolvedBinding, err := harnessBindingForOptions(options)
	if err != nil {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: restore binding: %v", ErrHarnessTurnRestoreUnavailable, err)
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor binding does not match runtime", ErrHarnessTurnRestoreUnavailable)
	}
	semanticSpec := AgentCommandSpec{
		Kind: descriptor.Kind, CommandID: string(input.CommandID),
		OperationID:      OperationID(input.OperationID),
		AfterOperationID: OperationID(descriptor.AfterOperationID),
		Request:          request,
		Options:          options,
	}
	if descriptor.Deferred {
		semanticSpec.Prepare = func(context.Context) (HarnessTurnExecution, error) {
			return HarnessTurnExecution{}, ErrHarnessTurnRestoreUnavailable
		}
	}
	wantRef := harnessCommandTurnRef(resolvedBinding, semanticSpec.CommandID, harnessTurnSpecSemanticFingerprint(semanticSpec))
	if strings.TrimSpace(input.Input.TurnSpecRef) != wantRef {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor does not match turn reference", ErrHarnessTurnRestoreUnavailable)
	}
	productBinding, err := ParseRuntimeBinding(binding)
	if err != nil {
		return HarnessTurnRestoreRequest{}, fmt.Errorf("%w: decode product binding: %v", ErrHarnessTurnRestoreUnavailable, err)
	}
	return HarnessTurnRestoreRequest{
		Binding: productBinding, Kind: descriptor.Kind, CommandID: CommandID(input.CommandID), OperationID: OperationID(input.OperationID),
		AfterOperationID: OperationID(descriptor.AfterOperationID),
		Request:          request,
		Options:          options,
		Deferred:         descriptor.Deferred,
	}, nil
}

func agentCommandKindForQueuedDelivery(delivery runstate.DeliveryKind) (AgentCommandKind, error) {
	switch delivery {
	case runstate.DeliverySteer:
		return AgentCommandSteer, nil
	case runstate.DeliveryFollowUp:
		return AgentCommandFollowUp, nil
	case runstate.DeliveryNextTurn:
		return AgentCommandNextTurn, nil
	default:
		return "", fmt.Errorf("%w: delivery %q is not restorable", ErrHarnessTurnRestoreUnavailable, delivery)
	}
}

func describeHarnessTurnRequest(request ChatRequest) harnessTurnRequestDescriptor {
	caller := chatRequestCallerView(request)
	return harnessTurnRequestDescriptor{
		Message:        caller.Message,
		References:     append([]string(nil), caller.References...),
		LoreReferences: append([]string(nil), caller.LoreReferences...),
		StyleScenes:    append([]string(nil), caller.StyleScenes...),
		Selections:     append([]TextSelectionRef(nil), caller.Selections...),
		IDEContext: IDEContextRef{
			CurrentFile: caller.IDEContext.CurrentFile,
			OpenFiles:   append([]string(nil), caller.IDEContext.OpenFiles...),
		},
		ReviewFeedback: cloneReviewFeedbackRefs(caller.ReviewFeedback),
		PlanMode:       caller.PlanMode,
		WritingSkill:   caller.WritingSkill,
		ImagePresetID:  caller.ImagePresetID,
		TellerID:       caller.TellerID,
		Locale:         caller.Locale,
	}
}

func (descriptor harnessTurnRequestDescriptor) chatRequest() ChatRequest {
	return ChatRequest{
		Message:        descriptor.Message,
		References:     append([]string(nil), descriptor.References...),
		LoreReferences: append([]string(nil), descriptor.LoreReferences...),
		StyleScenes:    append([]string(nil), descriptor.StyleScenes...),
		Selections:     append([]TextSelectionRef(nil), descriptor.Selections...),
		IDEContext: IDEContextRef{
			CurrentFile: descriptor.IDEContext.CurrentFile,
			OpenFiles:   append([]string(nil), descriptor.IDEContext.OpenFiles...),
		},
		ReviewFeedback: append(ReviewFeedbackRefs(nil), descriptor.ReviewFeedback...),
		PlanMode:       descriptor.PlanMode,
		WritingSkill:   descriptor.WritingSkill,
		ImagePresetID:  descriptor.ImagePresetID,
		TellerID:       descriptor.TellerID,
		Locale:         descriptor.Locale,
	}
}

func (descriptor harnessTurnOptionsDescriptor) runOptions() RunOptions {
	return RunOptions{
		AgentKind: descriptor.AgentKind, RootAgentName: descriptor.RootAgentName,
		TaskID: descriptor.TaskID, AutomationTaskID: descriptor.AutomationTaskID,
		SessionID: descriptor.SessionID, ReviewThreadID: descriptor.ReviewThreadID,
		StoryID: descriptor.StoryID, BranchID: descriptor.BranchID, TurnID: descriptor.TurnID,
		MaintenanceTask: descriptor.MaintenanceTask, Workspace: descriptor.Workspace,
		Mode: descriptor.Mode, WriteMode: descriptor.WriteMode, WriteScope: descriptor.WriteScope,
		IdleTimeout:        time.Duration(descriptor.IdleTimeout),
		ToolResultMaxBytes: descriptor.ToolResultMaxBytes,
	}
}

func mergeRestoredHarnessTurn(request HarnessTurnRestoreRequest, restored HarnessTurnSpec) HarnessTurnSpec {
	stableRequest := request.Request
	stableRequest.StyleRules = append([]StyleRule(nil), restored.Request.StyleRules...)
	stableRequest.ImagePreset = restored.Request.ImagePreset
	stableRequest.ResolvedReviewFeedback = append(ReviewFeedbackContexts(nil), restored.Request.ResolvedReviewFeedback...)
	restored.Request = stableRequest

	stableOptions := request.Options
	stableOptions.Controls = restored.Options.Controls
	stableOptions.SystemPromptLog = restored.Options.SystemPromptLog
	stableOptions.OnMutationsVerified = restored.Options.OnMutationsVerified
	stableOptions.OnUserMessageCommitted = restored.Options.OnUserMessageCommitted
	restored.Options = stableOptions
	restored.CommandID = request.CommandID
	restored.CommandKind = request.Kind
	restored.Emit = request.Emit
	if restored.CycleCommit == nil {
		restored.CycleCommit = harnessCycleCommitForConversation(restored.Conversation)
	}
	return restored
}

package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"

	"denova/internal/agents/prompts"
	agentreview "denova/internal/agents/review"
)

const harnessTurnRestoreDescriptorVersion = 2

// ErrTurnRestoreUnavailable means an accepted queued command is still
// durable, but this process cannot yet rebuild its Runner/Conversation state.
// Replaying the exact command or reopening with WithTurnRestorer retries
// the same queued operation; the runtime never converts this into cancellation.
var ErrTurnRestoreUnavailable = errors.New("agent harness turn restore dependency is unavailable")

// TurnRestoreRequest is the stable semantic input reconstructed from a
// queued Steer, FollowUp, or NextTurn bounded durable descriptor. The host resolves fresh
// Runner, Conversation, BookService, callbacks, and server-owned context.
type TurnRestoreRequest struct {
	Binding          agentrun.RuntimeBinding
	Kind             CommandKind
	CommandID        agentrun.CommandID
	OperationID      agentrun.OperationID
	AfterOperationID agentrun.OperationID
	Request          agentchat.ChatRequest
	Options          agentrun.Options
	Deferred         bool
	// Emit is process-local display routing supplied only by the explicit
	// recovery observation. It is never encoded in the durable descriptor.
	Emit func(agentrun.Event)
}

// TurnRestorer rebuilds process-local execution dependencies for one
// already accepted durable queued command. Implementations must be idempotent for the
// supplied agentrun.CommandID and must not perform model or tool effects.
type TurnRestorer func(context.Context, TurnRestoreRequest) (TurnSpec, error)

type harnessTurnRestoreDescriptor struct {
	Version          int                          `json:"version"`
	Kind             CommandKind                  `json:"kind"`
	AfterOperationID runstate.OperationID         `json:"after_operation_id"`
	Request          harnessTurnRequestDescriptor `json:"request"`
	Options          harnessTurnOptionsDescriptor `json:"options"`
	Deferred         bool                         `json:"deferred"`
}

type harnessTurnRequestDescriptor struct {
	Message         string                       `json:"message"`
	References      []string                     `json:"references,omitempty"`
	LoreReferences  []string                     `json:"lore_references,omitempty"`
	StyleScenes     []string                     `json:"style_scenes,omitempty"`
	Selections      []agentchat.TextSelectionRef `json:"selections,omitempty"`
	IDEContext      prompts.IDEContextRef        `json:"ide_context,omitempty"`
	ReviewFeedback  agentreview.Refs             `json:"review_feedback,omitempty"`
	PlanMode        bool                         `json:"plan_mode,omitempty"`
	WritingSkill    string                       `json:"writing_skill,omitempty"`
	ImagePresetID   string                       `json:"image_preset_id,omitempty"`
	TellerID        string                       `json:"teller_id,omitempty"`
	Locale          string                       `json:"locale,omitempty"`
	InputVisibility agentrun.InputVisibility     `json:"input_visibility,omitempty"`
}

func encodeHarnessTurnRestoreDescriptor(spec CommandSpec) (json.RawMessage, error) {
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
	spec CommandSpec,
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
) (TurnRestoreRequest, error) {
	if len(input.Input.RestoreDescriptor) == 0 {
		return TurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor is absent", ErrTurnRestoreUnavailable)
	}
	var descriptor harnessTurnRestoreDescriptor
	if err := json.Unmarshal(input.Input.RestoreDescriptor, &descriptor); err != nil {
		return TurnRestoreRequest{}, fmt.Errorf("%w: decode durable descriptor: %v", ErrTurnRestoreUnavailable, err)
	}
	if descriptor.Version != harnessTurnRestoreDescriptorVersion {
		return TurnRestoreRequest{}, fmt.Errorf(
			"%w: unsupported durable descriptor version %d",
			ErrTurnRestoreUnavailable,
			descriptor.Version,
		)
	}
	expectedKind, err := agentCommandKindForQueuedDelivery(input.Delivery)
	if err != nil {
		return TurnRestoreRequest{}, err
	}
	if descriptor.Kind != expectedKind || (expectedKind == CommandNextTurn && descriptor.AfterOperationID == "") {
		return TurnRestoreRequest{}, fmt.Errorf(
			"%w: durable descriptor kind %q does not match queued delivery %q",
			ErrTurnRestoreUnavailable,
			descriptor.Kind,
			input.Delivery,
		)
	}
	request := descriptor.Request.chatRequest()
	if request.Message != input.Input.Text {
		return TurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor message does not match queued input", ErrTurnRestoreUnavailable)
	}
	options := descriptor.Options.runOptions().Normalize(descriptor.Options.Workspace)
	resolvedBinding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return TurnRestoreRequest{}, fmt.Errorf("%w: restore binding: %v", ErrTurnRestoreUnavailable, err)
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return TurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor binding does not match runtime", ErrTurnRestoreUnavailable)
	}
	semanticSpec := CommandSpec{
		Kind: descriptor.Kind, CommandID: string(input.CommandID),
		OperationID:      agentrun.OperationID(input.OperationID),
		AfterOperationID: agentrun.OperationID(descriptor.AfterOperationID),
		Request:          request,
		Options:          options,
	}
	if descriptor.Deferred {
		semanticSpec.Prepare = func(context.Context) (TurnExecution, error) {
			return TurnExecution{}, ErrTurnRestoreUnavailable
		}
	}
	wantRef := harnessCommandTurnRef(resolvedBinding, semanticSpec.CommandID, harnessTurnSpecSemanticFingerprint(semanticSpec))
	if strings.TrimSpace(input.Input.TurnSpecRef) != wantRef {
		return TurnRestoreRequest{}, fmt.Errorf("%w: durable descriptor does not match turn reference", ErrTurnRestoreUnavailable)
	}
	productBinding, err := agentrun.ParseRuntimeBinding(binding)
	if err != nil {
		return TurnRestoreRequest{}, fmt.Errorf("%w: decode product binding: %v", ErrTurnRestoreUnavailable, err)
	}
	return TurnRestoreRequest{
		Binding: productBinding, Kind: descriptor.Kind, CommandID: agentrun.CommandID(input.CommandID), OperationID: agentrun.OperationID(input.OperationID),
		AfterOperationID: agentrun.OperationID(descriptor.AfterOperationID),
		Request:          request,
		Options:          options,
		Deferred:         descriptor.Deferred,
	}, nil
}

func agentCommandKindForQueuedDelivery(delivery runstate.DeliveryKind) (CommandKind, error) {
	switch delivery {
	case runstate.DeliverySteer:
		return CommandSteer, nil
	case runstate.DeliveryFollowUp:
		return CommandFollowUp, nil
	case runstate.DeliveryNextTurn:
		return CommandNextTurn, nil
	default:
		return "", fmt.Errorf("%w: delivery %q is not restorable", ErrTurnRestoreUnavailable, delivery)
	}
}

func describeHarnessTurnRequest(request agentchat.ChatRequest) harnessTurnRequestDescriptor {
	caller := agentchat.CallerView(request)
	return harnessTurnRequestDescriptor{
		Message:        caller.Message,
		References:     append([]string(nil), caller.References...),
		LoreReferences: append([]string(nil), caller.LoreReferences...),
		StyleScenes:    append([]string(nil), caller.StyleScenes...),
		Selections:     append([]agentchat.TextSelectionRef(nil), caller.Selections...),
		IDEContext: prompts.IDEContextRef{
			CurrentFile: caller.IDEContext.CurrentFile,
			OpenFiles:   append([]string(nil), caller.IDEContext.OpenFiles...),
		},
		ReviewFeedback:  caller.ReviewFeedback.Clone(),
		PlanMode:        caller.PlanMode,
		WritingSkill:    caller.WritingSkill,
		ImagePresetID:   caller.ImagePresetID,
		TellerID:        caller.TellerID,
		Locale:          caller.Locale,
		InputVisibility: request.InputVisibility,
	}
}

func (descriptor harnessTurnRequestDescriptor) chatRequest() agentchat.ChatRequest {
	return agentchat.ChatRequest{
		Message:        descriptor.Message,
		References:     append([]string(nil), descriptor.References...),
		LoreReferences: append([]string(nil), descriptor.LoreReferences...),
		StyleScenes:    append([]string(nil), descriptor.StyleScenes...),
		Selections:     append([]agentchat.TextSelectionRef(nil), descriptor.Selections...),
		IDEContext: prompts.IDEContextRef{
			CurrentFile: descriptor.IDEContext.CurrentFile,
			OpenFiles:   append([]string(nil), descriptor.IDEContext.OpenFiles...),
		},
		ReviewFeedback:  append(agentreview.Refs(nil), descriptor.ReviewFeedback...),
		PlanMode:        descriptor.PlanMode,
		WritingSkill:    descriptor.WritingSkill,
		ImagePresetID:   descriptor.ImagePresetID,
		TellerID:        descriptor.TellerID,
		Locale:          descriptor.Locale,
		InputVisibility: descriptor.InputVisibility,
	}
}

func (descriptor harnessTurnOptionsDescriptor) runOptions() agentrun.Options {
	return agentrun.Options{
		AgentKind: descriptor.AgentKind, ProjectID: descriptor.ProjectID,
		RootAgentName: descriptor.RootAgentName,
		TaskID:        descriptor.TaskID, AutomationTaskID: descriptor.AutomationTaskID,
		SessionID: descriptor.SessionID, ReviewThreadID: descriptor.ReviewThreadID,
		StoryID: descriptor.StoryID, BranchID: descriptor.BranchID, TurnID: descriptor.TurnID,
		MaintenanceTask: descriptor.MaintenanceTask, Workspace: descriptor.Workspace,
		Mode: descriptor.Mode, WriteMode: descriptor.WriteMode, WriteScope: descriptor.WriteScope,
		IdleTimeout:        time.Duration(descriptor.IdleTimeout),
		ToolResultMaxBytes: descriptor.ToolResultMaxBytes,
	}
}

func mergeRestoredHarnessTurn(request TurnRestoreRequest, restored TurnSpec) TurnSpec {
	stableRequest := request.Request
	stableRequest.StyleRules = append([]prompts.StyleRule(nil), restored.Request.StyleRules...)
	stableRequest.ImagePreset = restored.Request.ImagePreset
	stableRequest.ResolvedReviewFeedback = append(agentreview.Contexts(nil), restored.Request.ResolvedReviewFeedback...)
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

package execution

import (
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

const cycleRestoreDescriptorVersion = 2

// ErrCyclePreparationUnavailable means an accepted queued command is still durable,
// but this process cannot yet rebuild its cycle dependencies. Replaying the
// exact command or reopening the runtime retries
// the same queued operation; the runtime never converts this into cancellation.
var ErrCyclePreparationUnavailable = errors.New("agent execution cycle preparation dependency is unavailable")

// CycleRestoreRequest is the stable semantic input reconstructed from a
// queued Steer, FollowUp, or NextTurn bounded durable descriptor. The host resolves fresh
// Runner, Conversation, BookService, callbacks, and server-owned context.
type CycleRestoreRequest struct {
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

type cycleRestoreDescriptor struct {
	Version          int                    `json:"version"`
	Kind             CommandKind            `json:"kind"`
	AfterOperationID runstate.OperationID   `json:"after_operation_id"`
	Request          cycleRequestDescriptor `json:"request"`
	Options          cycleOptionsDescriptor `json:"options"`
	Deferred         bool                   `json:"deferred"`
}

type cycleRequestDescriptor struct {
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

func encodeCycleRestoreDescriptor(spec CommandRequest) (json.RawMessage, error) {
	descriptor := cycleRestoreDescriptor{
		Version:          cycleRestoreDescriptorVersion,
		Kind:             spec.Kind,
		AfterOperationID: runstate.OperationID(spec.AfterOperationID),
		Request:          describeCycleRequest(spec.Request),
		Options:          describeDurableCycleOptions(spec.Options),
		Deferred:         spec.Kind != CommandStartTurn,
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return nil, fmt.Errorf("encode agent execution turn restore descriptor: %w", err)
	}
	return encoded, nil
}

// withInputMaterializationDescriptor freezes the provider-free,
// caller-owned semantics needed to materialize canonical input after durable
// command acceptance. The runtime bounds this JSON through
// InputLimits.MaxRestoreDescriptorBytes (8 MiB by default), comfortably above
// the 128 KiB product context floor without making the journal unbounded.
func withInputMaterializationDescriptor(
	input runstate.UserInput,
	spec CommandRequest,
) (runstate.UserInput, error) {
	encoded, err := encodeCycleRestoreDescriptor(spec)
	if err != nil {
		return runstate.UserInput{}, err
	}
	input.RestoreDescriptor = encoded
	return input, nil
}

func decodeCycleRestoreRequest(
	binding runstate.BindingRef,
	input runstate.QueuedInput,
) (CycleRestoreRequest, error) {
	if len(input.Input.RestoreDescriptor) == 0 {
		return CycleRestoreRequest{}, fmt.Errorf("%w: durable descriptor is absent", ErrCyclePreparationUnavailable)
	}
	var descriptor cycleRestoreDescriptor
	if err := json.Unmarshal(input.Input.RestoreDescriptor, &descriptor); err != nil {
		return CycleRestoreRequest{}, fmt.Errorf("%w: decode durable descriptor: %v", ErrCyclePreparationUnavailable, err)
	}
	if descriptor.Version != cycleRestoreDescriptorVersion {
		return CycleRestoreRequest{}, fmt.Errorf(
			"%w: unsupported durable descriptor version %d",
			ErrCyclePreparationUnavailable,
			descriptor.Version,
		)
	}
	expectedKind, err := agentCommandKindForQueuedDelivery(input.Delivery)
	if err != nil {
		return CycleRestoreRequest{}, err
	}
	if descriptor.Kind != expectedKind || (expectedKind == CommandNextTurn && descriptor.AfterOperationID == "") {
		return CycleRestoreRequest{}, fmt.Errorf(
			"%w: durable descriptor kind %q does not match queued delivery %q",
			ErrCyclePreparationUnavailable,
			descriptor.Kind,
			input.Delivery,
		)
	}
	request := descriptor.Request.chatRequest()
	if request.Message != input.Input.Text {
		return CycleRestoreRequest{}, fmt.Errorf("%w: durable descriptor message does not match queued input", ErrCyclePreparationUnavailable)
	}
	options := descriptor.Options.runOptions().Normalize(descriptor.Options.Workspace)
	resolvedBinding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return CycleRestoreRequest{}, fmt.Errorf("%w: restore binding: %v", ErrCyclePreparationUnavailable, err)
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return CycleRestoreRequest{}, fmt.Errorf("%w: durable descriptor binding does not match runtime", ErrCyclePreparationUnavailable)
	}
	semanticSpec := CommandRequest{
		Kind: descriptor.Kind, CommandID: string(input.CommandID),
		OperationID:      agentrun.OperationID(input.OperationID),
		AfterOperationID: agentrun.OperationID(descriptor.AfterOperationID),
		Request:          request,
		Options:          options,
	}
	wantRef := commandCycleRef(resolvedBinding, semanticSpec.CommandID, cycleSemanticFingerprint(semanticSpec))
	if strings.TrimSpace(input.Input.TurnSpecRef) != wantRef {
		return CycleRestoreRequest{}, fmt.Errorf("%w: durable descriptor does not match cycle reference", ErrCyclePreparationUnavailable)
	}
	productBinding, err := agentrun.ParseRuntimeBinding(binding)
	if err != nil {
		return CycleRestoreRequest{}, fmt.Errorf("%w: decode product binding: %v", ErrCyclePreparationUnavailable, err)
	}
	return CycleRestoreRequest{
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
		return "", fmt.Errorf("%w: delivery %q is not restorable", ErrCyclePreparationUnavailable, delivery)
	}
}

func describeCycleRequest(request agentchat.ChatRequest) cycleRequestDescriptor {
	caller := agentchat.CallerView(request)
	return cycleRequestDescriptor{
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

func (descriptor cycleRequestDescriptor) chatRequest() agentchat.ChatRequest {
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

func (descriptor cycleOptionsDescriptor) runOptions() agentrun.Options {
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

func restoredCycleSpec(request CycleRestoreRequest, cycle Cycle) cycleSpec {
	stableRequest := request.Request
	stableRequest.StyleRules = append([]prompts.StyleRule(nil), cycle.Request.StyleRules...)
	stableRequest.ImagePreset = cycle.Request.ImagePreset
	stableRequest.ResolvedReviewFeedback = append(agentreview.Contexts(nil), cycle.Request.ResolvedReviewFeedback...)

	stableOptions := request.Options
	stableOptions.Controls = cycle.Options.Controls
	stableOptions.SystemPromptLog = cycle.Options.SystemPromptLog
	stableOptions.OnMutationsVerified = cycle.Options.OnMutationsVerified
	stableOptions.OnUserMessageCommitted = cycle.Options.OnUserMessageCommitted
	return cycleSpec{
		CommandID: request.CommandID, CommandKind: request.Kind,
		Runner: cycle.Runner, Conversation: cycle.Conversation, BookService: cycle.BookService,
		Request: stableRequest, Options: stableOptions, Emit: request.Emit,
		Successor: cycle.Successor, CycleCommit: cycleCommitForConversation(cycle.Conversation),
	}
}

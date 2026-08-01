package harness

import (
	"context"
	"crypto/sha256"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/prompts"
	agentreview "denova/internal/agents/review"
	"denova/internal/book"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// CommandKind is the closed public command vocabulary for an active
// durable binding. Mode-specific app adapters choose the binding and construct
// the bounded TurnSpec; transports never submit workspace identity.
type CommandKind string

const (
	CommandStartTurn    CommandKind = "start_turn"
	CommandSteer        CommandKind = "steer"
	CommandFollowUp     CommandKind = "follow_up"
	CommandNextTurn     CommandKind = "next_turn"
	CommandSteerQueued  CommandKind = "steer_queued"
	CommandCancelQueued CommandKind = "cancel_queued"
	CommandAbort        CommandKind = "abort"
)

type CommandSpec struct {
	Kind             CommandKind
	CommandID        string
	OperationID      agentrun.OperationID
	AfterOperationID agentrun.OperationID
	TargetCommandID  agentrun.CommandID
	Reason           string
	Runner           *agent.Runner
	Conversation     agentchat.Conversation
	BookService      *book.Service
	Request          agentchat.ChatRequest
	Options          agentrun.Options
	Emit             func(agentrun.Event)
	Prepare          TurnPreparer
}

// SubmitCommand durably accepts one command and returns without waiting for
// the selected cycle to execute. commandID is the idempotency key and must be
// reused by a transport retry.
func (s *Service) SubmitCommand(ctx context.Context, spec CommandSpec) (agentrun.CommandReceipt, error) {
	if s == nil || s.coordinator == nil || s.coordinator.runtime == nil || s.coordinator.engine == nil {
		return agentrun.CommandReceipt{}, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	commandID := strings.TrimSpace(spec.CommandID)
	if commandID == "" {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	// Reject oversized or otherwise invalid caller identities before computing
	// semantic fingerprints, opening a binding, or registering process state.
	if err := s.coordinator.runtime.ValidateCommandID(commandID); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	spec.Request = agentchat.CaptureChatRequestCallerInput(spec.Request)
	workspace := ""
	if spec.BookService != nil {
		workspace = spec.BookService.Workspace()
	}
	spec.Options = spec.Options.Normalize(workspace)
	binding, err := agentrun.BindingForOptions(spec.Options)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	harness, err := s.coordinator.runtime.Open(ctx, binding)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}

	if spec.Kind == CommandAbort || spec.Kind == CommandSteerQueued || spec.Kind == CommandCancelQueued {
		var command runstate.Command
		switch spec.Kind {
		case CommandAbort:
			command = runstate.Abort{
				ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(spec.OperationID),
				Reason: strings.TrimSpace(spec.Reason),
			}
		case CommandSteerQueued:
			command = runstate.SteerQueued{
				ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(spec.OperationID),
				TargetCommandID: runstate.CommandID(spec.TargetCommandID),
			}
		case CommandCancelQueued:
			command = runstate.CancelQueued{
				ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(spec.OperationID),
				TargetCommandID: runstate.CommandID(spec.TargetCommandID), Reason: strings.TrimSpace(spec.Reason),
			}
		}
		receipt, submitErr := harness.Submit(ctx, command)
		return agentrun.CommandReceiptFromRuntime(receipt), submitErr
	}

	turnSemantics := harnessTurnSpecSemanticFingerprint(spec)
	turnRef := harnessCommandTurnRef(binding, commandID, turnSemantics)
	caller := agentchat.CallerView(spec.Request)
	input := runstate.UserInput{
		Text: caller.Message, ContextRefs: harnessContextRefs(spec.Request), TurnSpecRef: turnRef,
	}
	input, err = withHarnessInputMaterializationDescriptor(input, spec)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	var command runstate.Command
	switch spec.Kind {
	case CommandStartTurn:
		command = runstate.StartTurn{ID: runstate.CommandID(commandID), Input: input}
	case CommandSteer:
		command = runstate.Steer{ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(spec.OperationID), Input: input}
	case CommandFollowUp:
		command = runstate.FollowUp{ID: runstate.CommandID(commandID), OperationID: runstate.OperationID(spec.OperationID), Input: input}
	case CommandNextTurn:
		command = runstate.NextTurn{ID: runstate.CommandID(commandID), AfterOperationID: runstate.OperationID(spec.AfterOperationID), Input: input}
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported command kind %q", runstate.ErrInvalidCommand, spec.Kind)
	}

	registration, err := s.coordinator.engine.register(turnRef, command, TurnSpec{
		CommandID: agentrun.CommandID(commandID), CommandKind: spec.Kind,
		Runner: spec.Runner, Conversation: spec.Conversation,
		BookService: spec.BookService, Request: spec.Request,
		Options: spec.Options, Emit: spec.Emit,
		Prepare:     spec.Prepare,
		CycleCommit: harnessCycleCommitForConversation(spec.Conversation),
	})
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	defer registration.release()
	receipt, err := harness.Submit(ctx, command)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	// A fresh acceptance still owns this registered adapter state until Engine
	// consumes it. A replay after consumption registered a redundant state and
	// must release it; a replay while queued shares the durably pinned entry.
	if !receipt.Replayed {
		registration.accept()
	}
	return agentrun.CommandReceiptFromRuntime(receipt), nil
}

func harnessCommandSemanticFingerprint(command runstate.Command) string {
	return semanticJSONFingerprint("agent-adapter-command.v1", command)
}

// harnessTurnSpecSemanticFingerprint covers stable caller-supplied values that
// cannot be copied into the bounded durable UserInput. Server-resolved context
// is deliberately excluded: the first accepted spec owns that snapshot, while
// a later transport replay must remain idempotent if workspace state changed.
// Embedding this hash in TurnSpecRef makes the runtime reject a command ID reused
// with a different hidden payload even after its first spec was consumed.
func harnessTurnSpecSemanticFingerprint(spec CommandSpec) string {
	caller := agentchat.CallerView(spec.Request)
	options := describeHarnessTurnOptions(spec.Options)
	// TaskID and runtime policy values are display/execution concerns. They may
	// legitimately change while a caller retries an already accepted command and
	// therefore cannot participate in its durable transport identity.
	options.TaskID = ""
	options.ReviewThreadID = ""
	options.IdleTimeout = 0
	options.ToolResultMaxBytes = 0
	descriptor := struct {
		Message        string                       `json:"message"`
		References     []string                     `json:"references,omitempty"`
		LoreReferences []string                     `json:"lore_references,omitempty"`
		StyleScenes    []string                     `json:"style_scenes,omitempty"`
		Selections     []agentchat.TextSelectionRef `json:"selections,omitempty"`
		IDEContext     prompts.IDEContextRef        `json:"ide_context,omitempty"`
		ReviewFeedback agentreview.Refs             `json:"review_feedback,omitempty"`
		PlanMode       bool                         `json:"plan_mode"`
		WritingSkill   string                       `json:"writing_skill,omitempty"`
		ImagePresetID  string                       `json:"image_preset_id,omitempty"`
		TellerID       string                       `json:"teller_id,omitempty"`
		Locale         string                       `json:"locale,omitempty"`
		Options        harnessTurnOptionsDescriptor `json:"options"`
		Deferred       bool                         `json:"deferred"`
	}{
		Message:        caller.Message,
		References:     caller.References,
		LoreReferences: caller.LoreReferences,
		StyleScenes:    caller.StyleScenes,
		Selections:     caller.Selections,
		IDEContext:     caller.IDEContext,
		ReviewFeedback: caller.ReviewFeedback,
		PlanMode:       caller.PlanMode,
		WritingSkill:   caller.WritingSkill,
		ImagePresetID:  caller.ImagePresetID,
		TellerID:       caller.TellerID,
		Locale:         caller.Locale,
		Options:        options,
		Deferred:       spec.Prepare != nil,
	}
	return semanticJSONFingerprint("agent-turn-spec.v1", descriptor)
}

// RequestSemanticFingerprint identifies one caller-visible logical root
// request independently from server-resolved context and display task state.
// App admission uses it before allocating a Task; the runtime independently
// verifies the complete StartTurn command fingerprint at durable submission.
func RequestSemanticFingerprint(req agentchat.ChatRequest) string {
	caller := agentchat.CallerView(req)
	caller.CommandID = ""
	return semanticJSONFingerprint("agent-chat-request.v1", caller)
}

type harnessTurnOptionsDescriptor struct {
	AgentKind          string `json:"agent_kind"`
	ProjectID          string `json:"project_id,omitempty"`
	RootAgentName      string `json:"root_agent_name,omitempty"`
	TaskID             string `json:"task_id,omitempty"`
	AutomationTaskID   string `json:"automation_task_id,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
	ReviewThreadID     string `json:"review_thread_id,omitempty"`
	StoryID            string `json:"story_id,omitempty"`
	BranchID           string `json:"branch_id,omitempty"`
	TurnID             string `json:"turn_id,omitempty"`
	MaintenanceTask    string `json:"maintenance_task,omitempty"`
	Workspace          string `json:"workspace"`
	Mode               string `json:"mode,omitempty"`
	WriteMode          string `json:"write_mode,omitempty"`
	WriteScope         string `json:"write_scope,omitempty"`
	IdleTimeout        int64  `json:"idle_timeout,omitempty"`
	ToolResultMaxBytes int    `json:"tool_result_max_bytes,omitempty"`
}

func describeHarnessTurnOptions(options agentrun.Options) harnessTurnOptionsDescriptor {
	return harnessTurnOptionsDescriptor{
		AgentKind: options.AgentKind, ProjectID: options.ProjectID,
		RootAgentName: options.RootAgentName,
		TaskID:        options.TaskID, AutomationTaskID: options.AutomationTaskID,
		SessionID: options.SessionID, ReviewThreadID: options.ReviewThreadID,
		StoryID: options.StoryID, BranchID: options.BranchID, TurnID: options.TurnID,
		MaintenanceTask: options.MaintenanceTask, Workspace: options.Workspace,
		Mode: options.Mode, WriteMode: options.WriteMode, WriteScope: options.WriteScope,
		IdleTimeout:        int64(options.IdleTimeout),
		ToolResultMaxBytes: options.ToolResultMaxBytes,
	}
}

func describeHarnessDurableTurnOptions(options agentrun.Options) harnessTurnOptionsDescriptor {
	descriptor := describeHarnessTurnOptions(options)
	// These values are execution/display policy owned by the accepting process,
	// not caller semantics. Excluding them keeps an exact transport retry stable
	// when task allocation or runtime defaults changed after acceptance.
	descriptor.TaskID = ""
	descriptor.ReviewThreadID = ""
	descriptor.IdleTimeout = 0
	descriptor.ToolResultMaxBytes = 0
	return descriptor
}

// harnessTurnRuntimeSemanticFingerprint protects process-local adapter state
// that cannot be serialized into UserInput. Equal transport retries may rebuild
// objects, so this descriptor uses stable types and identity fields rather than
// pointers or mutable resolved context.
func harnessTurnRuntimeSemanticFingerprint(spec TurnSpec) string {
	workspace := ""
	if spec.BookService != nil {
		workspace = spec.BookService.Workspace()
	}
	options := describeHarnessTurnOptions(spec.Options)
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	options.TaskID = ""
	options.ReviewThreadID = ""
	options.IdleTimeout = 0
	options.ToolResultMaxBytes = 0
	descriptor := struct {
		Options          harnessTurnOptionsDescriptor `json:"options"`
		Deferred         bool                         `json:"deferred"`
		RunnerType       string                       `json:"runner_type"`
		ConversationType string                       `json:"conversation_type"`
		BookWorkspace    string                       `json:"book_workspace,omitempty"`
	}{
		Options:          options,
		Deferred:         spec.Prepare != nil,
		RunnerType:       fmt.Sprintf("%T", spec.Runner),
		ConversationType: fmt.Sprintf("%T", spec.Conversation),
		BookWorkspace:    workspace,
	}
	return semanticJSONFingerprint("agent-turn-runtime.v1", descriptor)
}

func harnessCommandTurnRef(binding runstate.BindingRef, commandID, turnSemantics string) string {
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		bindingRef = runstate.BindingRef{}
	}
	identity := struct {
		Binding       runstate.BindingRef `json:"binding"`
		CommandID     string              `json:"command_id"`
		TurnSemantics string              `json:"turn_semantics"`
	}{
		Binding: bindingRef, CommandID: strings.TrimSpace(commandID),
		TurnSemantics: strings.TrimSpace(turnSemantics),
	}
	return "command-turn-" + semanticJSONFingerprint("agent-turn-reference.v1", identity)
}

// semanticJSONFingerprint is the versioned canonical identity seam shared by
// transport retries and durable adapter references. encoding/json orders map
// keys and honors explicit descriptor fields, unlike Go debug formatting whose
// output can change with type names or implementation details.
func semanticJSONFingerprint(scope string, value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		// All callers use closed JSON-safe descriptors. Keep the function total
		// without making an impossible adapter encoding failure process-fatal.
		encoded = []byte("json_error:" + err.Error())
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(scope)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil))
}

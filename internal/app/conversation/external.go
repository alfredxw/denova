package conversationapp

import (
	"context"
	"denova/config"
	"fmt"
	"strings"
	"time"

	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	appagentruntime "denova/internal/app/agentruntime"
	agent "github.com/alfredxw/denova/agent"
	publiccontext "github.com/alfredxw/denova/agent/context"
)

func prepareExternal(ctx context.Context, runtime Runtime, request agentchat.ChatRequest, conversation *agentconversation.SessionConversation, assembly agents.ExternalAssembly, options agentrun.Options, emit func(agentrun.Event)) (external.StartRequest, error) {
	state, err := agentruntime.SessionState(options, runtime.Session)
	if err != nil {
		return external.StartRequest{}, err
	}
	goalContext, err := state.GoalContext(ctx)
	if err != nil {
		return external.StartRequest{}, err
	}
	plan, err := state.Plan(ctx)
	if err != nil {
		return external.StartRequest{}, err
	}
	history, err := external.PrepareHistory(ctx, runtime.Session)
	if err != nil {
		return external.StartRequest{}, err
	}
	input, err := prepareExternalInput(ctx, runtime, request, conversation, assembly, options, emit)
	if err != nil {
		return external.StartRequest{}, err
	}
	input.Revision, input.PreparedCursor, input.ContinuesOperationID = history.Revision, history.Cursor, history.ContinuesOperationID
	input.SourceBoundary = fmt.Sprint(history.ContextRevision)
	input.Input.Selection, input.LoadHistory, input.Input.Plan = history.Selection, history.Messages, plan
	input.Input.Text = goalContext + input.Input.Text
	input.ObservePlan, input.Checkpoint, input.PriorMutations = state.ObservePlan, history.Checkpoint, history.PriorMutations
	return input, nil
}

// prepareExternalInput is shared by initial input and live native steering.
// It assembles product context without reading an idle-only runtime checkpoint
// or committing a second operation while the current one is still running.
func prepareExternalInput(ctx context.Context, runtime Runtime, request agentchat.ChatRequest, conversation *agentconversation.SessionConversation, assembly agents.ExternalAssembly, options agentrun.Options, emit func(agentrun.Event)) (external.StartRequest, error) {
	prepared, err := agentchat.PrepareAgentContext(ctx, conversation, request, runtime.BookService, runtime.Workspace, time.Now().UTC())
	if err != nil {
		return external.StartRequest{}, err
	}
	fragments, err := publiccontext.ExportLifecycleFragments(prepared.ModelContext.Context)
	if err != nil {
		return external.StartRequest{}, err
	}
	if assembly.Context != nil {
		shared, err := assembly.Context.Materialize(ctx, agent.ContextRequest{})
		if err != nil {
			return external.StartRequest{}, err
		}
		fragments = append(fragments, shared...)
	}
	var instruction strings.Builder
	for _, fragment := range fragments {
		switch fragment.Placement {
		case agent.ContextLeadingMessage, agent.ContextStateMessage:
			if len(fragment.Content) > fragment.HardLimit {
				return external.StartRequest{}, fmt.Errorf("external context exceeds source limit: %s", fragment.Source)
			}
			instruction.WriteString("\n\n")
			instruction.WriteString(fragment.Content)
		case agent.ContextAuditOnly:
		case agent.ContextFinalUserPrefix, agent.ContextFinalUserMessage, agent.ContextCompactionCheckpoint:
			return external.StartRequest{}, fmt.Errorf("unsupported external shared context placement %q", fragment.Placement)
		default:
			return external.StartRequest{}, fmt.Errorf("unknown external context placement %q", fragment.Placement)
		}
	}
	text := ""
	for i := len(prepared.ModelContext.Messages) - 1; i >= 0; i-- {
		message := prepared.ModelContext.Messages[i]
		if message != nil && message.Role == agent.User {
			text = message.Content
			break
		}
	}
	if text == "" {
		return external.StartRequest{}, fmt.Errorf("external turn has no assembled user input")
	}
	return external.StartRequest{
		ProjectID: runtime.ProjectID, AttachmentRoot: runtime.ProjectStore, Session: runtime.Session, CommandID: request.CommandID,
		Fingerprint: agentexecution.RequestSemanticFingerprint(request),
		Input:       external.Input{Instructions: assembly.Composition.Instruction(), Text: instruction.String() + "\n\n" + text, Attachments: request.AttachedFiles},
		Message:     agent.Message{Role: agent.User, Content: request.Message, Attachments: request.AttachedFiles},
		Metadata: session.MessageMetadata{MessageID: request.CommandID + "-input", AgentKind: runtime.AgentKind,
			ContextOnly:    request.InputVisibility == agentrun.InputModelOnly,
			DisplayContent: request.DisplayMessage, UserReferences: agentchat.UserMessageReferences(request)},
		Definitions: assembly.Tools, ReviewThreadID: options.ReviewThreadID, Emit: emit,
		ToolPolicy: toolruntime.OrchestratorConfig{AgentKind: runtime.AgentKind, PolicyKind: runtime.AgentKind,
			Workspace: runtime.Workspace, ToolSettings: assembly.ToolSettings, EnforceToolSettings: true,
			ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtime.Config)},
		InputCommitEffect: options.InputCommitEffect, BookService: runtime.BookService, OnMutationsVerified: options.OnMutationsVerified,
		ProviderInputMaxBytes: config.ResolveAgentContext(&runtime.Config, runtime.AgentKind).MaxProviderInputBytes,
		Locale:                runtime.Config.Language,
	}, nil
}

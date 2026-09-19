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
	"denova/internal/agents/external"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	appagentruntime "denova/internal/app/agentruntime"
	agent "github.com/alfredxw/denova/agent"
	publiccontext "github.com/alfredxw/denova/agent/context"
)

func prepareExternal(ctx context.Context, runtime Runtime, request agentchat.ChatRequest, conversation *agentconversation.SessionConversation, assembly agents.ExternalAssembly, options agentrun.Options, emit func(agentrun.Event)) (external.StartRequest, error) {
	history, err := external.ReadHistory(ctx, runtime.Session)
	if err != nil {
		return external.StartRequest{}, err
	}
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
	instruction.WriteString(assembly.Composition.Instruction())
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
		Fingerprint: agentexecution.RequestSemanticFingerprint(request), Revision: history.Revision,
		PreparedCursor: history.Cursor, ContinuesOperationID: history.ContinuesOperationID,
		Input:   external.Input{Selection: history.Selection, Instructions: instruction.String(), History: history.Messages, Text: text, Attachments: request.AttachedFiles},
		Message: agent.Message{Role: agent.User, Content: request.Message, Attachments: request.AttachedFiles},
		Metadata: session.MessageMetadata{MessageID: request.CommandID + "-input", AgentKind: runtime.AgentKind,
			DisplayContent: request.DisplayMessage, UserReferences: agentchat.UserMessageReferences(request)},
		Definitions: assembly.Tools, ReviewThreadID: options.ReviewThreadID, Emit: emit,
		ToolPolicy: toolruntime.OrchestratorConfig{AgentKind: runtime.AgentKind, PolicyKind: runtime.AgentKind,
			Workspace: runtime.Workspace, ToolSettings: assembly.ToolSettings, EnforceToolSettings: true,
			ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(runtime.Config)},
		InputCommitEffect: options.InputCommitEffect, BookService: runtime.BookService, OnMutationsVerified: options.OnMutationsVerified,
		Checkpoint: history.Checkpoint, ProviderInputMaxBytes: config.ResolveAgentContext(&runtime.Config, runtime.AgentKind).MaxProviderInputBytes,
		Locale:         runtime.Config.Language,
		PriorMutations: history.PriorMutations,
	}, nil
}

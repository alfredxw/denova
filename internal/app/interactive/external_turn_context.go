package interactiveapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agents/attachment"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
	publiccontext "github.com/alfredxw/denova/agent/context"
)

func gameRuntimeBoundary(snapshot interactive.Snapshot) string {
	head := ""
	if snapshot.CurrentTurn != nil {
		head = snapshot.CurrentTurn.ID
	}
	return fmt.Sprintf("%s/%d", head, snapshot.ContextRevision)
}

func (turn *ExternalTurn) prepareInput(ctx context.Context, mode external.OperationMode) (external.Input, error) {
	c := turn.config.Conversation
	for _, guidance := range turn.config.Guidance {
		if err := turn.commitGuidance(guidance); err != nil {
			return external.Input{}, err
		}
	}
	// Schema initialization changes only the in-memory opening draft. Restore
	// it from accepted tool batches before assembling context after a restart.
	if c.openingStateSchemaDraft != nil {
		for _, definition := range turn.config.Assembly.Tools {
			info, err := definition.Tool.Info(ctx)
			if err != nil {
				return external.Input{}, err
			}
			if info.Name != "initialize_story_state_schema" {
				continue
			}
			for _, message := range turn.restored {
				for _, call := range message.ToolCalls {
					if call.Function.Name == info.Name {
						if _, err := definition.Tool.Run(ctx, call.Function.Arguments); err != nil {
							return external.Input{}, err
						}
					}
				}
			}
		}
	}
	prepared, err := agentchat.PrepareAgentContext(ctx, c, turn.config.Request, turn.config.BookService, c.workspace, time.Now().UTC())
	if err != nil {
		return external.Input{}, err
	}
	if mode == external.OperationTurn {
		if err := c.CommitModelInput(ctx, prepared.OriginalMessage, prepared.ModelContext); err != nil {
			return external.Input{}, err
		}
	}
	fragments, err := publiccontext.ExportLifecycleFragments(prepared.ModelContext.Context)
	if err != nil {
		return external.Input{}, err
	}
	if source := turn.config.Assembly.Context; source != nil {
		shared, err := source.Materialize(ctx, agent.ContextRequest{})
		if err != nil {
			return external.Input{}, err
		}
		fragments = append(fragments, shared...)
	}
	input := external.Input{Selection: *turn.config.Config.ActiveAgentRuntime, Instructions: turn.config.Assembly.Composition.Instruction(), Plan: turn.config.Plan}
	var dynamic strings.Builder
	for _, fragment := range fragments {
		switch fragment.Placement {
		case agent.ContextLeadingMessage, agent.ContextStateMessage:
			if len(fragment.Content) > fragment.HardLimit {
				return external.Input{}, fmt.Errorf("external Game context exceeds source limit: %s", fragment.Source)
			}
			dynamic.WriteString("\n\n" + fragment.Content)
		case agent.ContextAuditOnly:
		default:
			return external.Input{}, fmt.Errorf("unsupported external Game context placement %q", fragment.Placement)
		}
	}
	for i := len(prepared.ModelContext.Messages) - 1; i >= 0; i-- {
		if message := prepared.ModelContext.Messages[i]; message != nil && message.Role == agent.User {
			input.Text, input.Attachments = message.Content, message.Attachments
			break
		}
	}
	input.Text = dynamic.String() + "\n\n" + input.Text
	for _, guidance := range turn.config.Guidance {
		// An aligned provider resume omits History, including journaled guidance.
		// Deliver the accepted instruction in the new turn as well.
		input.Text += "\n\nAdditional user instructions:\n" + guidance.Message
		input.Attachments = append(input.Attachments, guidance.AttachedFiles...)
	}
	// Read the complete public branch, independently of Native compaction and
	// visibility preferences. Provider continuations and reasoning are omitted.
	story, err := c.storyContextForCycle()
	if err != nil {
		return external.Input{}, err
	}
	history, _, err := c.modelHistoryForCycle(story)
	if err != nil {
		return external.Input{}, err
	}
	projection, err := BuildModelContextProjection(history, nil, story.Snapshot, canonicalToolContextPolicy(c.ToolResultContextPolicy()), turn.identity)
	if err != nil {
		return external.Input{}, err
	}
	historyMessages := append(append([]*agent.Message(nil), projection.Messages...), turn.restored...)
	input.History = externalGameMessages(historyMessages)
	input.HistoryBoundary = fmt.Sprintf("%s/%s/%d", gameRuntimeBoundary(story.Snapshot), turn.identity.OperationID, c.modelContextBatchSequence)
	if err := c.loadTurnDraft(); err != nil {
		return external.Input{}, err
	}
	if narrative, err := c.LoadNarrativeCandidate(ctx); err != nil {
		return external.Input{}, err
	} else if narrative != "" {
		input.Text += "\n\nThe following narrative has already been accepted. Retain it verbatim and complete only missing turn submission modules:\n" + narrative
	}
	turn.tools = make(map[string]agent.ToolDefinition)
	for _, definition := range turn.config.Assembly.Tools {
		if err := definition.Validate(ctx); err != nil {
			return external.Input{}, err
		}
		info, err := definition.Tool.Info(ctx)
		if err != nil {
			return external.Input{}, err
		}
		schema, err := info.ToJSONSchema()
		if err != nil {
			return external.Input{}, err
		}
		body, err := json.Marshal(schema)
		if err != nil {
			return external.Input{}, err
		}
		turn.tools[info.Name] = definition
		input.Tools = append(input.Tools, external.Tool{Name: info.Name, Description: info.Desc, Schema: body})
	}
	if turn.config.Runtime != nil {
		return input, nil
	}
	return turn.prepareRuntimeInput(ctx, input, turn.config.Adapter)
}

func (turn *ExternalTurn) prepareRuntimeInput(ctx context.Context, source external.Input, adapter external.Adapter) (external.Input, error) {
	var usageErr error
	preparation := external.HistoryPreparation{Input: source, Adapter: adapter, ProviderInputMaxBytes: turn.inputLimit(), ResolveMedia: turn.media().Resolve, AddUsage: func(usage *agent.TokenUsage) { usageErr = turn.recordUsage(usage) }}
	prepare := turn.config.PrepareHistory
	if prepare == nil {
		prepare = func(ctx context.Context, preparation external.HistoryPreparation) (external.Input, error) {
			return preparation.Prepare(ctx)
		}
	}
	input, err := prepare(ctx, preparation)
	if err != nil {
		return external.Input{}, err
	}
	if usageErr != nil {
		return external.Input{}, usageErr
	}
	return input, nil
}

func externalGameMessages(messages []*agent.Message) []external.Message {
	result := make([]external.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		projected := external.Message{Role: string(message.Role), Text: message.Content, Attachments: message.Attachments, Cursor: uint64(len(result) + 1)}
		if message.Role == agent.ToolRole {
			projected.Role, projected.Text = "user", "Confirmed tool observation ("+message.ToolName+"):\n"+message.Content
			projected.ToolImages, projected.Attachments = message.Attachments, nil
		}
		if strings.TrimSpace(projected.Text) != "" || len(projected.Attachments)+len(projected.ToolImages) > 0 {
			result = append(result, projected)
		}
	}
	return result
}

func (turn *ExternalTurn) inputLimit() int {
	return config.ResolveAgentContext(&turn.config.Config, config.AgentKindInteractiveStory).MaxProviderInputBytes
}

func (turn *ExternalTurn) media() external.MediaProjection {
	resolver, _ := turn.config.Conversation.ToolArtifactStore().(agent.ToolArtifactPathResolver)
	return external.MediaProjection{Root: turn.config.Config.ProjectStoreDir,
		Scope: attachment.StoryScope(turn.config.Conversation.storyID), Artifacts: resolver}
}

func (turn *ExternalTurn) projectMedia(ctx context.Context, input external.Input) (external.Input, error) {
	return turn.media().Prepare(ctx, input, turn.inputLimit())
}

func (turn *ExternalTurn) projectToolImages(ctx context.Context, files []agent.Attachment) ([]agent.Attachment, error) {
	return turn.media().ResolveToolImages(ctx, files)
}

func (turn *ExternalTurn) recordUsage(usage *agent.TokenUsage) error {
	if usage == nil {
		return nil
	}
	return turn.config.Conversation.AppendDisplayEvent(session.DisplayEvent{Role: "token_usage", RunID: string(turn.identity.OperationID), AgentKind: config.AgentKindInteractiveStory,
		PromptTokens: usage.PromptTokens, CachedPromptTokens: usage.PromptTokenDetails.CachedTokens, CompletionTokens: usage.CompletionTokens,
		ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens, TotalTokens: usage.TotalTokens, CreatedAt: time.Now().UTC()})
}

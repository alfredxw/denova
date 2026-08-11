package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	agentplan "denova/internal/agents/plan"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

// PublicEventProjector converts the reusable Agent event vocabulary into the
// established Denova task/SSE and display transcript projection.
type PublicEventProjector struct {
	mu             sync.Mutex
	request        ChatRequest
	options        agentrun.Options
	emit           func(agentrun.Event)
	recorder       *displayEventRecorder
	plan           *agentplan.Parser
	content        strings.Builder
	thinking       strings.Builder
	usage          *runTokenUsageCollector
	generatedBytes int
	flushed        bool
	terminal       bool
	compaction     publicCompactionBinder
}

type publicCompactionBinder interface {
	BindAgentCompaction(*agent.CompactionState) error
}

func NewPublicEventProjector(
	conversation Conversation,
	request ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) *PublicEventProjector {
	projector := &PublicEventProjector{request: request, options: options, emit: emit}
	projector.compaction, _ = conversation.(publicCompactionBinder)
	projector.recorder = newDisplayEventRecorder(conversation, displayEventRecorderOptions{
		SuppressRootAssistantSegments: request.PlanMode,
	})
	if request.PlanMode {
		projector.plan = agentplan.NewParser(projector.rootMetadata().planMetadata(), planEventEmitter(projector.emitEvent))
	}
	return projector
}

// SetEmit rebinds the process-local display sink after a browser/app task
// reconnects. Durable Agent events remain the source of truth; the sink is
// intentionally not part of the persisted Definition identity.
func (projector *PublicEventProjector) SetEmit(emit func(agentrun.Event)) {
	if projector == nil {
		return
	}
	projector.mu.Lock()
	projector.emit = emit
	projector.mu.Unlock()
}

func (projector *PublicEventProjector) EmitProduct(event agentrun.Event) {
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projector.emitEvent(event)
}

func (projector *PublicEventProjector) Project(event agent.Event) {
	projector.mu.Lock()
	defer projector.mu.Unlock()
	meta := projector.metadata(event.RunID, agent.EventSource{})
	switch payload := event.Payload.(type) {
	case agent.AssistantDelta:
		meta = projector.metadata(event.RunID, payload.Source)
		content := payload.Delta
		if !meta.SubAgent && !payload.DisplayOnly {
			projector.generatedBytes += len(payload.Delta)
		}
		if projector.plan != nil && !meta.SubAgent && !payload.DisplayOnly {
			content = projector.plan.Push(content)
		}
		if content != "" {
			if !meta.SubAgent && !payload.DisplayOnly {
				projector.content.WriteString(content)
			}
			projector.emitEvent(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": content})})
		}
	case agent.ThinkingDelta:
		meta = projector.metadata(event.RunID, payload.Source)
		if payload.Delta != "" {
			if !meta.SubAgent && !payload.DisplayOnly {
				projector.thinking.WriteString(payload.Delta)
			}
			projector.emitEvent(agentrun.Event{Type: "thinking", Data: meta.appendTo(map[string]any{"content": payload.Delta})})
		}
	case agent.AssistantFinal:
		// Durable final output repairs a missed ephemeral stream and reconstructs
		// cold replay without duplicating content already delivered live.
		content := missingPublicOutputSuffix(projector.content.String(), payload.Content)
		if content != "" {
			projector.content.WriteString(content)
			projector.generatedBytes += len(content)
			projector.emitEvent(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": content})})
		}
		thinking := missingPublicOutputSuffix(projector.thinking.String(), payload.Thinking)
		if thinking != "" {
			projector.thinking.WriteString(thinking)
			projector.emitEvent(agentrun.Event{Type: "thinking", Data: meta.appendTo(map[string]any{"content": thinking})})
		}
	case agent.ModelCompleted:
		if projector.usage == nil {
			projector.usage = newRunTokenUsageCollector(event.RunID, projector.options.AgentKind)
		}
		calls := make([]agent.ToolCall, 0, len(payload.RequestedTools))
		for _, name := range payload.RequestedTools {
			calls = append(calls, agent.ToolCall{Function: agent.FunctionCall{Name: name}})
		}
		usage := payload.Usage
		projector.usage.AddMessage(&agent.Message{
			Role: agent.Assistant, ToolCalls: calls,
			ResponseMeta: &agent.ResponseMeta{FinishReason: payload.FinishReason, Usage: &usage},
		})
	case agent.ToolStarted:
		meta = projector.metadata(event.RunID, payload.Source)
		arguments := string(payload.Arguments)
		if handled, successful := agentplan.EmitToolCall(payload.Name, arguments, meta.planMetadata(), planEventEmitter(projector.emitEvent)); handled {
			if successful && projector.plan != nil {
				projector.plan.NoteSuccessfulBlock()
			}
			return
		}
		projector.emitEvent(agentrun.Event{Type: "tool_call", Data: meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": payload.CallID, "name": payload.Name, "args": arguments,
		})})
	case agent.ToolProgress:
		meta = projector.metadata(event.RunID, payload.Source)
		projector.emitEvent(agentrun.Event{Type: "tool_progress", Data: meta.appendTo(map[string]any{
			"id": payload.CallID, "name": "", "delta": payload.Delta,
		})})
	case agent.ToolFinished:
		meta = projector.metadata(event.RunID, payload.Source)
		data := meta.appendTo(map[string]any{
			"id": payload.CallID, "name": payload.Name, "content": payload.Result,
		})
		projection := payload.Projection
		if projection == nil {
			status := "success"
			if payload.IsError {
				status = "error"
			}
			data["status"] = status
		} else {
			content := projection.DisplayContent
			if content == "" {
				content = projection.ModelContent
			}
			if content == "" {
				content = "(无返回内容)"
			}
			data["content"] = content
			data["status"] = string(projection.Status)
			data["synthetic_reason"] = string(projection.SyntheticReason)
			data["model_truncated"] = projection.Metadata.ModelTruncated
			data["display_truncated"] = projection.Metadata.DisplayTruncated
			data["target"] = projection.Metadata.Target
			data["tool_result_original_tokens"] = toolresult.EstimatedTokens(int64(projection.Metadata.OriginalModelBytes))
			data["tool_result_inline_tokens"] = toolresult.EstimatedTokens(int64(projection.Metadata.ReturnedModelBytes))
			data["tool_result_retention_mode"] = string(projection.ResultRetention)
			if projection.ContextHints != nil {
				data["tool_result_context_value"] = string(projection.ContextHints.ContextValue)
				data["tool_result_recovery_kind"] = string(projection.ContextHints.Recovery.Kind)
			}
			data["artifact_count"] = len(projection.Artifacts)
			if persistence := projection.Metadata.ArtifactPersistence; persistence != nil {
				data["artifact_persist_attempted"] = persistence.Attempted
				data["artifact_persist_complete"] = persistence.Complete
				data["artifact_persist_failure_reason"] = persistence.FailureReason
				if persistence.Attempted && !persistence.Complete {
					data["artifact_persist_failure_count"] = 1
				}
			}
			if projection.Status == agent.ToolResultSuccess && isWorkspaceArtifactRead(payload.Name, projection.Metadata.Target) {
				data["artifact_reread_count"] = 1
			}
			domainPayload := projection.ModelContent
			if len(projection.Details) != 0 && json.Valid(projection.Details) {
				domainPayload = string(projection.Details)
			}
			_ = populateToolResultDomainProjection(projector.options, payload.Name, domainPayload, meta, data, projector.emitEvent)
		}
		if projector.usage != nil {
			projector.usage.NoteToolResult(payload.Name)
		}
		projector.emitEvent(agentrun.Event{Type: "tool_result", Data: data})
	case agent.InteractionRequested:
		projector.emitEvent(projectInteractionRequested(payload.Request, meta))
	case agent.InteractionResolved:
		status := "answered"
		if payload.Resolution.Cancelled || payload.Resolution.Permission == agent.PermissionDeny {
			status = "cancelled"
		}
		projector.emitEvent(agentrun.Event{Type: "ask_resolved", Data: meta.appendTo(map[string]any{
			"id": payload.ID, "status": status,
		})})
	case agent.CompactionCommitted:
		if projector.compaction != nil {
			state := payload.State
			if err := projector.compaction.BindAgentCompaction(&state); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] bind committed Compaction projection failed id=%s revision=%d err=%v", state.ID, state.Revision, err))
			}
		}
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(map[string]any{
			"id": payload.State.ID, "phase": "agent", "status": "completed",
			"summary": payload.State.Summary, "tokens_after": payload.State.TokenEstimate,
			"epoch":                payload.State.Revision,
			"source_message_count": payload.State.ReplacementTo - payload.State.ReplacementFrom,
		})})
	case agent.CompactionRemoved:
		if projector.compaction != nil {
			if err := projector.compaction.BindAgentCompaction(nil); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] clear removed Compaction projection failed id=%s revision=%d err=%v", payload.ID, payload.Revision, err))
			}
		}
	case agent.RunSettled:
		// The execution host finalizes all cycle projectors together after
		// committed Tool effects and post-run verification are complete.
	}
}

func missingPublicOutputSuffix(current, final string) string {
	if final == "" || current == final {
		return ""
	}
	if strings.HasPrefix(final, current) {
		return strings.TrimPrefix(final, current)
	}
	if current == "" {
		return final
	}
	// Live deltas are ordered before the final within one observation. A
	// divergent non-empty prefix cannot be repaired with append-only chunks.
	return ""
}

// Flush completes cycle-local plan and usage projections without publishing a
// terminal task event. A public Run may contain multiple queued cycles.
func (projector *PublicEventProjector) Flush() {
	if projector == nil {
		return
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projector.flushLocked()
}

// Finalize publishes exactly one terminal event after all product callbacks
// that belong before task completion have run.
func (projector *PublicEventProjector) Finalize(status agent.ResultStatus, reason string) {
	if projector == nil {
		return
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projector.flushLocked()
	if projector.terminal {
		return
	}
	projector.terminal = true
	switch status {
	case agent.ResultCompleted:
		projector.emitEvent(agentrun.Event{Type: "done", Data: map[string]string{}})
	case agent.ResultAborted:
		projector.emitEvent(agentrun.Event{Type: "aborted", Data: map[string]string{"reason": reason}})
	default:
		projector.emitEvent(agentrun.Event{Type: "error", Data: map[string]string{"message": reason}})
	}
}

func (projector *PublicEventProjector) flushLocked() {
	if projector.flushed {
		return
	}
	projector.flushed = true
	if projector.plan != nil {
		if remainder := projector.plan.Flush(); remainder != "" {
			projector.emitEvent(agentrun.Event{Type: "chunk", Data: projector.rootMetadata().appendTo(map[string]any{"content": remainder})})
		}
	}
	if projector.usage != nil {
		projector.usage.EmitIfAny(projector.emitEvent, projector.generatedBytes)
	}
}

func (projector *PublicEventProjector) Output() (string, string) {
	if projector == nil {
		return "", ""
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	return projector.content.String(), projector.thinking.String()
}

// ProjectCanonicalOutput preserves the existing Plan-mode product contract:
// the structured proposal is display-only and raw protocol tags do not enter
// future model context.
func (projector *PublicEventProjector) ProjectCanonicalOutput(message *agent.Message) (*agent.Message, *agent.OutputProjection) {
	if message == nil {
		return nil, nil
	}
	projected := message.Clone()
	if !projector.request.PlanMode {
		return projected, nil
	}
	parser := agentplan.NewParser(projector.rootMetadata().planMetadata(), nil)
	visible := parser.Push(projected.Content) + parser.Flush()
	if !parser.HasSuccessfulBlock() {
		projected.Content = visible
		return projected, nil
	}
	projected.Content = ""
	projected.ReasoningContent = ""
	projected.ToolCalls = nil
	return projected, &agent.OutputProjection{}
}

func (projector *PublicEventProjector) emitEvent(event agentrun.Event) {
	if projector.recorder != nil {
		projector.recorder.Record(event)
	}
	if projector.emit != nil {
		projector.emit(event)
	}
}

func (projector *PublicEventProjector) rootMetadata() agentEventMetadata {
	return projector.metadata(projector.options.TaskID, agent.EventSource{Name: projector.options.RootAgentName})
}

func (projector *PublicEventProjector) metadata(runID string, source agent.EventSource) agentEventMetadata {
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = projector.options.RootAgentName
	}
	path := append([]string(nil), source.Path...)
	if len(path) == 0 && name != "" {
		path = []string{name}
	}
	root := strings.TrimSpace(projector.options.RootAgentName)
	subAgent := len(path) > 1 || root != "" && name != "" && name != root
	return agentEventMetadata{
		RunID:     firstNonEmpty(strings.TrimSpace(runID), projector.options.TaskID),
		AgentKind: projector.options.AgentKind, AgentName: name, RootAgentName: root,
		RunPath: path, SubAgent: subAgent,
	}
}

func projectInteractionRequested(request agent.InteractionRequest, meta agentEventMetadata) agentrun.Event {
	questions := make([]map[string]any, len(request.Questions))
	for index, question := range request.Questions {
		options := make([]map[string]any, len(question.Options))
		for optionIndex, option := range question.Options {
			options[optionIndex] = map[string]any{
				"id": option.Value, "label": localizedValue(option.Label), "description": localizedValue(option.Description),
			}
		}
		questions[index] = map[string]any{
			"id": question.ID, "question": localizedValue(question.Prompt), "options": options,
			"multiple": question.Multiple, "allow_free_text": question.AllowFreeText,
		}
	}
	return agentrun.Event{Type: "ask_pending", Data: meta.appendTo(map[string]any{
		"id": request.ID, "kind": string(request.Kind), "questions": questions, "allow_other": request.AllowOther,
	})}
}

func localizedValue(value agent.LocalizedText) map[string]string {
	return map[string]string{"zh": value.Chinese, "en": value.English}
}

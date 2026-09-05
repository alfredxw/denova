package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agentplan "denova/internal/agents/plan"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

// PublicEventProjector converts the reusable Agent event vocabulary into the
// established Denova task/SSE and display transcript projection.
type PublicEventProjector struct {
	mu                      sync.Mutex
	conversation            Conversation
	request                 ChatRequest
	options                 agentrun.Options
	emit                    func(agentrun.Event)
	recorder                *displayEventRecorder
	plan                    *agentplan.Parser
	content                 strings.Builder
	thinking                strings.Builder
	usage                   *runTokenUsageCollector
	generatedBytes          int
	flushed                 bool
	terminal                bool
	compaction              publicCompactionBinder
	toolInputs              map[string]publicToolInput
	interactions            map[string]publicInteraction
	nestedContent           map[string]*strings.Builder
	nestedThinking          map[string]*strings.Builder
	runID                   string
	runStartedAt            time.Time
	runSummarized           bool
	interactive             publicInteractiveOutput
	explicitSkillsProjected bool
}

type publicToolInput struct {
	providerCallID string
	parentCallID   string
	name           string
	index          int
	descriptor     *agent.ToolDescriptor
	arguments      string
	targetEmitted  bool
}

type publicInteraction struct {
	request agent.InteractionRequest
	data    map[string]any
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
	projector := &PublicEventProjector{
		conversation: conversation, request: request, options: options, emit: emit,
		toolInputs: make(map[string]publicToolInput), interactions: make(map[string]publicInteraction),
		nestedContent: make(map[string]*strings.Builder), nestedThinking: make(map[string]*strings.Builder),
	}
	projector.compaction, _ = conversation.(publicCompactionBinder)
	projector.recorder = newDisplayEventRecorder(conversation, displayEventRecorderOptions{
		SuppressRootAssistantSegments: request.PlanMode,
	})
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
	projector.projectLocked(event, agent.EventSource{}, "")
}

func (projector *PublicEventProjector) projectLocked(event agent.Event, inherited agent.EventSource, parentCallID string) {
	projector.bindRunIDLocked(event.RunID)
	meta := projector.metadata(event.RunID, inherited, parentCallID)
	switch payload := event.Payload.(type) {
	case agent.NestedEvent:
		child := payload.Child
		// Denova display remains attached to the parent operation while the
		// typed NestedEvent retains the exact child Run identity for observers.
		child.RunID = event.RunID
		projector.projectLocked(child, inheritedEventSource(payload.Source, inherited), firstNonEmpty(payload.ParentCallID, parentCallID))
	case agent.RunAccepted:
		// Acceptance is already represented by the task transport and the
		// Denova-specific agent_cycle_started edge.
	case agent.RunStarted:
		// The execution host owns command delivery and calls ProjectRunStarted
		// with the durable cycle plus Denova-only command metadata.
	case agent.AssistantDelta:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		content := payload.Delta
		if !meta.SubAgent && !payload.DisplayOnly {
			projector.generatedBytes += len(payload.Delta)
		}
		if projector.plan != nil && !meta.SubAgent && !payload.DisplayOnly {
			content = projector.plan.Push(content)
		}
		if content != "" {
			if projector.projectInteractiveAssistantDeltaLocked(meta, payload.DisplayOnly, content) {
				return
			}
			if !meta.SubAgent && !payload.DisplayOnly {
				projector.content.WriteString(content)
			}
			projector.emitEvent(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": content})})
			if meta.SubAgent && !payload.DisplayOnly {
				projector.nestedOutput(projector.nestedContent, meta).WriteString(content)
			}
		}
	case agent.ThinkingDelta:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		if payload.Delta != "" {
			if !meta.SubAgent && !payload.DisplayOnly {
				projector.thinking.WriteString(payload.Delta)
			}
			projector.emitEvent(agentrun.Event{Type: "thinking", Data: meta.appendTo(map[string]any{"content": payload.Delta})})
			if meta.SubAgent && !payload.DisplayOnly {
				projector.nestedOutput(projector.nestedThinking, meta).WriteString(payload.Delta)
			}
		}
	case agent.AssistantFinal:
		if meta.SubAgent {
			content := missingPublicOutputSuffix(projector.nestedOutput(projector.nestedContent, meta).String(), payload.Content)
			if content != "" {
				projector.nestedOutput(projector.nestedContent, meta).WriteString(content)
				projector.emitEvent(agentrun.Event{Type: "chunk", Data: meta.appendTo(map[string]any{"content": content})})
			}
			thinking := missingPublicOutputSuffix(projector.nestedOutput(projector.nestedThinking, meta).String(), payload.Thinking)
			if thinking != "" {
				projector.nestedOutput(projector.nestedThinking, meta).WriteString(thinking)
				projector.emitEvent(agentrun.Event{Type: "thinking", Data: meta.appendTo(map[string]any{"content": thinking})})
			}
			// Repaired deltas are the only display content; RunSettled publishes the status.
			return
		}
		projector.finishInteractiveResponseLocked(nil)
		// Final output repairs any missed live delta without duplicating content
		// already delivered to this in-process display stream.
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
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		projector.finishInteractiveResponseLocked(payload.RequestedTools)
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
	case agent.ContextNormalized:
		projector.emitEvent(agentrun.Event{Type: "context_normalizer", Data: meta.appendTo(map[string]any{
			"status": "repaired", "context_normalizer_repair_count": payload.RepairCount,
			"messages_before": payload.MessagesBefore, "messages_after": payload.MessagesAfter,
		})})
	case agent.ToolInputStarted:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		projector.observeInteractiveToolLocked(meta, payload.Name)
		if agentplan.EmitToolRunning(payload.Name, meta.planMetadata(), planEventEmitter(projector.emitEvent)) {
			return
		}
		if _, exists := projector.toolInputs[payload.CallID]; exists {
			return
		}
		providerCallID := payload.ProviderCallID
		if providerCallID == "" {
			providerCallID = payload.CallID
		}
		projector.toolInputs[payload.CallID] = publicToolInput{
			providerCallID: providerCallID, parentCallID: payload.ParentCallID,
			name: payload.Name, index: payload.Index, descriptor: payload.Descriptor,
		}
		data := meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": providerCallID, "name": payload.Name,
			"index": payload.Index, "args": "",
		})
		if payload.ParentCallID != "" {
			data["parent_call_id"] = payload.ParentCallID
		}
		appendToolDescriptorProjection(data, payload.Descriptor)
		projector.emitEvent(agentrun.Event{Type: "tool_call", Data: data})
	case agent.ToolInputDelta:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		if agentplan.IsToolName(payload.Name) || payload.Delta == "" {
			return
		}
		input, exists := projector.toolInputs[payload.CallID]
		if !exists {
			providerCallID := payload.ProviderCallID
			if providerCallID == "" {
				providerCallID = payload.CallID
			}
			input = publicToolInput{providerCallID: providerCallID, name: payload.Name}
			projector.emitEvent(agentrun.Event{Type: "tool_call", Data: meta.appendTo(map[string]any{
				"id": payload.CallID, "provider_call_id": providerCallID, "name": payload.Name, "args": "",
			})})
		}
		if payload.ProviderCallID != "" {
			input.providerCallID = payload.ProviderCallID
		}
		if payload.Name != "" {
			input.name = payload.Name
		}
		input.arguments += payload.Delta
		projector.toolInputs[payload.CallID] = input
		if !input.targetEmitted {
			if target := toolresult.TargetFromArguments(input.arguments); target != "" {
				input.targetEmitted = true
				projector.toolInputs[payload.CallID] = input
				projector.emitEvent(agentrun.Event{Type: "tool_target", Data: meta.appendTo(map[string]any{
					"id": payload.CallID, "provider_call_id": input.providerCallID,
					"name": input.name, "index": input.index, "target": target,
				})})
			}
		}
		projector.emitEvent(agentrun.Event{Type: "tool_args_delta", Data: meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": input.providerCallID,
			"name": input.name, "index": input.index, "delta": payload.Delta,
		})})
	case agent.ToolStarted:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		projector.observeInteractiveToolLocked(meta, payload.Name)
		arguments := string(payload.Arguments)
		if handled, successful := agentplan.EmitToolCall(payload.Name, arguments, meta.planMetadata(), planEventEmitter(projector.emitEvent)); handled {
			if successful && projector.plan != nil {
				projector.plan.NoteSuccessfulBlock()
			}
			return
		}
		input, streamed := projector.toolInputs[payload.CallID]
		if !streamed {
			providerCallID := firstNonEmpty(payload.ProviderCallID, payload.CallID)
			input = publicToolInput{
				providerCallID: providerCallID, name: payload.Name, index: payload.Index, descriptor: payload.Descriptor,
			}
			projector.toolInputs[payload.CallID] = input
			data := meta.appendTo(map[string]any{
				"id": payload.CallID, "provider_call_id": input.providerCallID, "name": payload.Name,
				"index": payload.Index, "args": arguments,
			})
			if input.parentCallID != "" {
				data["parent_call_id"] = input.parentCallID
			}
			if target := toolresult.TargetFromArguments(arguments); target != "" {
				data["target"] = target
				input.targetEmitted = true
			}
			appendToolDescriptorProjection(data, payload.Descriptor)
			projector.emitEvent(agentrun.Event{Type: "tool_call", Data: data})
		}
		if payload.ProviderCallID != "" {
			input.providerCallID = payload.ProviderCallID
		}
		if payload.Descriptor != nil {
			input.descriptor = payload.Descriptor
		}
		input.index = payload.Index
		if input.arguments == "" {
			input.arguments = arguments
		}
		projector.toolInputs[payload.CallID] = input
		if streamed && !input.targetEmitted {
			if target := toolresult.TargetFromArguments(arguments); target != "" {
				input.targetEmitted = true
				projector.toolInputs[payload.CallID] = input
				projector.emitEvent(agentrun.Event{Type: "tool_target", Data: meta.appendTo(map[string]any{
					"id": payload.CallID, "provider_call_id": input.providerCallID,
					"name": payload.Name, "index": payload.Index, "target": target,
				})})
			}
		}
		data := meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": input.providerCallID, "name": payload.Name, "index": payload.Index,
		})
		if input.parentCallID != "" {
			data["parent_call_id"] = input.parentCallID
		}
		appendToolDescriptorProjection(data, input.descriptor)
		projector.emitEvent(agentrun.Event{Type: "tool_started", Data: data})
	case agent.ToolProgress:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		input := projector.toolInputs[payload.CallID]
		data := meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": firstNonEmpty(payload.ProviderCallID, payload.CallID),
			"name": payload.Name, "index": payload.Index, "delta": payload.Delta,
		})
		if input.parentCallID != "" {
			data["parent_call_id"] = input.parentCallID
		}
		appendToolDescriptorProjection(data, payload.Descriptor)
		projector.emitEvent(agentrun.Event{Type: "tool_progress", Data: data})
	case agent.ToolFinished:
		meta = projector.metadata(event.RunID, inheritedEventSource(payload.Source, inherited), parentCallID)
		input := projector.toolInputs[payload.CallID]
		delete(projector.toolInputs, payload.CallID)
		data := meta.appendTo(map[string]any{
			"id": payload.CallID, "provider_call_id": firstNonEmpty(payload.ProviderCallID, input.providerCallID, payload.CallID),
			"name": payload.Name, "index": payload.Index, "content": payload.Result,
		})
		if input.parentCallID != "" {
			data["parent_call_id"] = input.parentCallID
		}
		descriptor := payload.Descriptor
		if descriptor == nil {
			descriptor = input.descriptor
		}
		appendToolDescriptorProjection(data, descriptor)
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
			for _, warning := range projectPublicToolResult(projector.options, payload.Name, domainPayload, meta, data, projector.emitEvent) {
				slog.WarnContext(context.Background(), "[agent-public-runtime] project ToolResult display data failed",
					"tool", payload.Name, "error", warning)
			}
		}
		if projector.usage != nil {
			projector.usage.NoteToolResult(payload.Name)
		}
		projector.emitEvent(agentrun.Event{Type: "tool_result", Data: data})
	case agent.ArtifactProduced:
		if meta.SubAgent {
			projector.emitEvent(agentrun.Event{Type: "subagent_artifact", Data: meta.appendTo(map[string]any{
				"call_id": payload.CallID, "artifact": payload.Artifact,
			})})
		}
	case agent.EventStreamGap:
		projector.emitEvent(agentrun.Event{Type: "agent_event_stream_gap", Data: meta.appendTo(map[string]any{
			"dropped": payload.Dropped, "resume_after": payload.ResumeAfter,
		})})
	case agent.GoalUpdated:
		data := meta.appendTo(map[string]any{
			"schema": "agent.goal.v1", "present": payload.Present,
			"id": payload.State.ID, "objective": payload.State.Objective,
			"status": payload.State.Status, "revision": payload.State.Revision,
			"report": payload.State.Report, "created_at": payload.State.CreatedAt,
			"updated_at":             payload.State.UpdatedAt,
			"active_since":           payload.State.ActiveSince,
			"active_duration_millis": payload.State.ActiveDurationMillis,
		})
		projector.emitEvent(agentrun.Event{Type: "goal_updated", Data: data})
	case agent.GoalEvaluationFailed:
		projector.emitEvent(agentrun.Event{Type: "goal_evaluation_failed", Data: meta.appendTo(map[string]any{
			"code": payload.Code, "goal_id": payload.GoalID, "goal_revision": payload.GoalRevision,
			"message": "目标完成度评估失败，目标仍保持进行中；请重试或继续执行。 / Goal completion evaluation failed; the goal remains active. Retry or continue execution.",
			"detail":  payload.Detail,
		})})
	case agent.TodoUpdated:
		items := make([]map[string]any, len(payload.State.Items))
		for index, item := range payload.State.Items {
			items[index] = map[string]any{"id": item.ID, "text": item.Text, "status": item.Status}
		}
		projector.emitEvent(agentrun.Event{Type: "todo_updated", Data: meta.appendTo(map[string]any{
			"schema": "agent.todo.v1", "revision": payload.State.Revision, "items": items,
		})})
	case agent.InteractionRequested:
		projected := projectInteractionRequested(payload.Request, meta)
		data, _ := projected.Data.(map[string]any)
		projector.interactions[payload.Request.ID] = publicInteraction{request: payload.Request, data: clonePublicEventData(data)}
		projector.emitEvent(projected)
	case agent.InteractionResolved:
		status := "answered"
		if payload.Resolution.Cancelled || payload.Resolution.Permission == agent.PermissionDeny {
			status = "cancelled"
		}
		interaction := projector.interactions[payload.ID]
		delete(projector.interactions, payload.ID)
		data := clonePublicEventData(interaction.data)
		if data == nil {
			data = meta.appendTo(map[string]any{
				"schema": "ask.pending.v1", "id": payload.ID, "tool_call_id": payload.ID,
				"agent_kind": meta.AgentKind, "questions": []map[string]any{},
			})
		}
		data["status"] = status
		if answers := projectInteractionAnswers(interaction.request, payload.Resolution); len(answers) > 0 {
			data["answers"] = answers
		}
		projector.emitEvent(agentrun.Event{Type: "ask_resolved", Data: data})
	case agent.CleanupStarted:
		projector.emitEvent(agentrun.Event{Type: "context_cleanup", Data: meta.appendTo(cleanupMetricsData(map[string]any{
			"id": payload.ID, "phase": maintenancePhase(payload.Automatic), "status": "started",
			"action": "cleanup", "trigger_reason": payload.Reason, "transient": payload.Transient,
		}, payload.Metrics))})
	case agent.CleanupCompleted:
		projector.emitEvent(agentrun.Event{Type: "context_cleanup", Data: meta.appendTo(cleanupMetricsData(map[string]any{
			"id": payload.ID, "phase": maintenancePhase(payload.Automatic), "status": "completed",
			"action": "cleanup", "trigger_reason": payload.Reason, "transient": payload.Transient,
		}, payload.Metrics))})
	case agent.CleanupFailed:
		projector.emitEvent(agentrun.Event{Type: "context_cleanup", Data: meta.appendTo(cleanupMetricsData(map[string]any{
			"id": payload.ID, "phase": maintenancePhase(payload.Automatic), "status": "failed",
			"action": "cleanup", "trigger_reason": payload.Reason, "error": payload.Reason,
		}, payload.Metrics))})
	case agent.CleanupSkipped:
		projector.emitEvent(agentrun.Event{Type: "context_cleanup", Data: meta.appendTo(cleanupMetricsData(map[string]any{
			"id": payload.ID, "phase": maintenancePhase(payload.Automatic), "status": "skipped",
			"action": "cleanup", "trigger_reason": payload.Reason, "skipped_reason": payload.Reason,
		}, payload.Metrics))})
	case agent.CleanupCommitted:
		projector.emitEvent(agentrun.Event{Type: "context_cleanup", Data: meta.appendTo(cleanupMetricsData(map[string]any{
			"id": payload.State.ID, "phase": maintenancePhase(payload.Automatic), "status": "committed",
			"action": "cleanup", "epoch": int(payload.State.Revision), "renderer": payload.State.Renderer,
			"source_start": payload.State.SourceStart, "source_end": payload.State.SourceEnd,
			"replacement_count": len(payload.State.Replacements),
		}, payload.State.Metrics))})
	case agent.CompactionStarted:
		action := "compact"
		if payload.Remove {
			action = "remove"
		}
		phase := "agent"
		if payload.Automatic {
			phase = "model_step"
		}
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(compactionMetricsData(map[string]any{
			"id": payload.ID, "phase": phase, "status": "started", "action": action,
		}, payload.Metrics))})
	case agent.CompactionCommitted:
		if projector.compaction != nil {
			state := payload.State
			if err := projector.compaction.BindAgentCompaction(&state); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] bind committed Compaction projection failed id=%s revision=%d err=%v", state.ID, state.Revision, err))
			}
		}
		phase := "agent"
		if payload.Automatic {
			phase = "model_step"
		}
		metrics := payload.State.Metrics
		if metrics.SourceMessageCount == 0 {
			metrics.SourceMessageCount = payload.State.ReplacementTo - payload.State.ReplacementFrom
		}
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(compactionMetricsData(map[string]any{
			"id": payload.State.ID, "phase": phase, "status": "completed",
			"summary": payload.State.Summary, "tokens_after": payload.State.TokenEstimate,
			"epoch": int(payload.State.Revision),
		}, metrics))})
	case agent.CompactionRemoved:
		if projector.compaction != nil {
			if err := projector.compaction.BindAgentCompaction(nil); err != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] clear removed Compaction projection failed id=%s revision=%d err=%v", payload.ID, payload.Revision, err))
			}
		}
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(map[string]any{
			"id": payload.ID, "phase": "agent", "status": "removed", "epoch": payload.Revision,
		})})
	case agent.CompactionFailed:
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(compactionMetricsData(map[string]any{
			"id": payload.ID, "phase": "model_step", "status": "failed", "reason": payload.Reason,
			"consecutive_failures": payload.ConsecutiveFailures, "failure_fuse_open": payload.FailureFuseOpen,
		}, payload.Metrics))})
	case agent.CompactionSkipped:
		projector.emitEvent(agentrun.Event{Type: "context_compaction", Data: meta.appendTo(compactionMetricsData(map[string]any{
			"id": payload.ID, "phase": "model_step", "status": "skipped", "skipped_reason": payload.Reason,
			"consecutive_failures": payload.ConsecutiveFailures, "failure_fuse_open": payload.FailureFuseOpen,
		}, payload.Metrics))})
	case agent.SessionCleared:
		// Clear commands are initiated by Denova endpoints, which own UI refresh.
	case agent.ContextLimitReached:
		// The following RunSettled result carries the terminal, user-visible
		// error without publishing two competing terminal events.
	case agent.RunSettled:
		if meta.SubAgent {
			projector.emitEvent(agentrun.Event{Type: "subagent_settled", Data: meta.appendTo(map[string]any{
				"status": payload.Status, "reason": payload.Reason,
			})})
			delete(projector.nestedContent, nestedEventKey(meta))
			delete(projector.nestedThinking, nestedEventKey(meta))
			return
		}
		projector.finishInteractiveResponseLocked(nil)
		// The execution host finalizes all cycle projectors together after
		// committed Tool effects and post-run verification are complete.
	}
}

func inheritedEventSource(source, inherited agent.EventSource) agent.EventSource {
	if source.Name == "" && len(source.Path) == 0 && source.InvocationID == "" && source.InvocationType == "" {
		return inherited
	}
	if source.InvocationID == "" && inherited.InvocationID != "" {
		source.InvocationID = inherited.InvocationID
		source.InvocationType = inherited.InvocationType
	}
	if len(inherited.Path) != 0 {
		overlap := 0
		maxOverlap := min(len(inherited.Path), len(source.Path))
		for size := maxOverlap; size > 0; size-- {
			matched := true
			for index := 0; index < size; index++ {
				if inherited.Path[len(inherited.Path)-size+index] != source.Path[index] {
					matched = false
					break
				}
			}
			if matched {
				overlap = size
				break
			}
		}
		path := append([]string(nil), inherited.Path...)
		path = append(path, source.Path[overlap:]...)
		source.Path = path
		if len(path) != 0 {
			source.Name = path[len(path)-1]
		}
	}
	return source
}

func nestedEventKey(meta agentEventMetadata) string {
	return firstNonEmpty(meta.SubAgentSessionID, strings.Join(meta.RunPath, "/"), meta.AgentName)
}

func (projector *PublicEventProjector) nestedOutput(
	outputs map[string]*strings.Builder,
	meta agentEventMetadata,
) *strings.Builder {
	key := nestedEventKey(meta)
	output := outputs[key]
	if output == nil {
		output = &strings.Builder{}
		outputs[key] = output
	}
	return output
}

func maintenancePhase(automatic bool) string {
	if automatic {
		return "model_step"
	}
	return "agent"
}

func cleanupMetricsData(data map[string]any, metrics agent.CleanupMetrics) map[string]any {
	data["estimated_tokens_before"] = metrics.EstimatedTokensBefore
	data["local_projected_tokens"] = metrics.LocalProjectedTokens
	data["observed_prompt_tokens"] = metrics.ObservedPromptTokens
	data["effective_tokens"] = metrics.EffectiveTokens
	data["estimated_tokens_after"] = metrics.EstimatedTokensAfter
	data["projected_tokens_after"] = metrics.EstimatedTokensAfter
	data["estimated_reclaimed_tokens"] = metrics.ReclaimedTokens
	data["actual_reclaimed_tokens"] = metrics.ReclaimedTokens
	data["context_window_tokens"] = metrics.ContextWindowTokens
	data["pressure"] = metrics.BodyPressureBefore
	data["full_pressure"] = metrics.PressureBefore
	data["pressure_after"] = metrics.BodyPressureAfter
	data["full_pressure_after"] = metrics.PressureAfter
	data["body_pressure_before"] = metrics.BodyPressureBefore
	data["body_pressure_after"] = metrics.BodyPressureAfter
	data["stable_prefix_tokens"] = metrics.StablePrefixTokens
	data["candidate_tokens"] = metrics.CandidateTokens
	data["cache_viable_candidate_tokens"] = metrics.CacheViableCandidateTokens
	data["cleanup_skipped_below_minimum_count"] = metrics.SkippedBelowMinimumCount
	data["cleanup_skipped_warm_suffix_count"] = metrics.SkippedWarmSuffixCount
	data["eager_receipt_candidate_count"] = metrics.EagerCandidateCount
	data["eager_receipt_applied_count"] = metrics.EagerSelectedCount
	data["eager_receipt_fallback_count"] = max(0, metrics.EagerCandidateCount-metrics.EagerSelectedCount)
	data["superseded_candidate_count"] = metrics.SupersededCandidateCount
	data["discardable_candidate_count"] = metrics.DiscardableCandidateCount
	data["minimum_cleanup_tokens"] = metrics.MinimumCleanupTokens
	data["protected_result_count"] = metrics.ProtectedResults
	data["earliest_changed_index"] = metrics.EarliestChanged
	data["warm_suffix_tokens"] = metrics.WarmSuffixTokens
	data["placeholder_tokens"] = metrics.PlaceholderTokens
	if _, frozen := data["replacement_count"]; !frozen || metrics.ReplacementCount > 0 {
		data["replacement_count"] = metrics.ReplacementCount
	}
	data["eager_only"] = metrics.EagerOnly
	data["pressure_scope"] = metrics.PressureScope
	data["provider_cache_state"] = metrics.ProviderCacheState
	data["cleanup_execution_mode"] = metrics.ExecutionMode
	data["placeholder_renderer_version"] = metrics.RendererVersion
	return data
}

func compactionMetricsData(data map[string]any, metrics agent.CompactionMetrics) map[string]any {
	data["estimated_tokens_before"] = metrics.EstimatedTokensBefore
	data["observed_prompt_tokens"] = metrics.ObservedPromptTokens
	data["observed_estimate_tokens"] = metrics.ObservedEstimateTokens
	data["estimated_tokens_after"] = metrics.EstimatedTokensAfter
	data["projected_tokens_before"] = metrics.ProjectedTokensBefore
	data["projected_tokens_after"] = metrics.ProjectedTokensAfter
	data["reserved_tokens"] = metrics.ReservedTokens
	data["context_window_tokens"] = metrics.ContextWindowTokens
	data["threshold"] = metrics.Threshold
	data["recovery_band"] = metrics.RecoveryBand
	data["recovery_target_tokens"] = metrics.RecoveryTargetTokens
	data["recovery_band_met"] = metrics.RecoveryBandMet
	data["degraded"] = metrics.Degraded
	data["stable_prefix_tokens"] = metrics.StablePrefixTokens
	data["source_message_count"] = metrics.SourceMessageCount
	data["message_count_before"] = metrics.MessageCountBefore
	data["message_count_after"] = metrics.MessageCountAfter
	data["cache_expected_prefix_tokens"] = metrics.CacheExpectedPrefixTokens
	data["cache_read_tokens"] = metrics.CacheReadTokens
	data["candidate_fingerprint"] = metrics.CandidateFingerprint
	data["candidate_generation"] = metrics.CandidateGeneration
	if metrics.CacheExpectedPrefixTokens > 0 {
		data["cache_hit_ratio"] = float64(metrics.CacheReadTokens) / float64(metrics.CacheExpectedPrefixTokens)
	} else {
		data["cache_hit_ratio"] = float64(0)
	}
	return data
}

func appendToolDescriptorProjection(data map[string]any, descriptor *agent.ToolDescriptor) {
	if data == nil || descriptor == nil || descriptor.Execution == "" {
		return
	}
	data["source"] = string(descriptor.Source)
	data["mutation_scope"] = string(descriptor.MutationScope)
	data["post_check"] = string(descriptor.PostCheck)
	data["max_result_bytes"] = descriptor.MaxResultBytes
	presentation, err := descriptor.Presentation.Normalize()
	if err == nil {
		data["tool_presentation"] = presentation
	}
}

// ProjectRunStarted restores Denova's product cycle edge from the public
// lifecycle. Command delivery and accepted input remain host-owned metadata;
// the public Agent event supplies the durable Run and cycle identity.
func (projector *PublicEventProjector) ProjectRunStarted(runID string, cycle int, commandID, delivery string, startedAt time.Time) {
	if projector == nil {
		return
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projector.bindRunIDLocked(runID)
	if !startedAt.IsZero() && (projector.runStartedAt.IsZero() || startedAt.Before(projector.runStartedAt)) {
		projector.runStartedAt = startedAt.UTC()
	}
	delivery = strings.TrimSpace(delivery)
	// Agent names the reusable runtime delivery "start" while Denova's public
	// command contract calls the same product action "start_turn".
	if delivery == "start" {
		delivery = "start_turn"
	}
	data := map[string]any{
		"command_id": strings.TrimSpace(commandID), "delivery": delivery,
		"message": projector.request.Message, "operation_id": strings.TrimSpace(runID),
		"run_id": strings.TrimSpace(runID), "cycle": cycle,
	}
	if !projector.runStartedAt.IsZero() {
		data["run_started_at"] = projector.runStartedAt.Format(time.RFC3339Nano)
	}
	projector.emitEvent(agentrun.Event{Type: "agent_cycle_started", Data: data})
}

// ProjectPreparedContext restores the established visible Skill load cards at
// the same seam that injects explicitly requested Skills into the first model
// request. These are product projections, not synthetic Agent tool executions.
func (projector *PublicEventProjector) ProjectPreparedContext(prepared AgentContextPreparation) {
	if projector == nil || len(prepared.ExplicitSkills) == 0 {
		return
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	if projector.explicitSkillsProjected {
		return
	}
	projector.explicitSkillsProjected = true
	meta := projector.rootMetadata()
	for index, invocation := range prepared.ExplicitSkills {
		name := strings.TrimSpace(invocation.Name)
		if name == "" {
			continue
		}
		arguments, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			slog.ErrorContext(context.Background(), "[agent-public-runtime] encode explicit Skill projection failed", "skill", name, "error", err)
			continue
		}
		callID := fmt.Sprintf("%s-explicit-skill-%02d", firstNonEmpty(projector.runID, "run"), index+1)
		projector.emitEvent(agentrun.Event{Type: "tool_call", Data: meta.appendTo(map[string]any{
			"id": callID, "provider_call_id": callID, "name": "skill", "args": string(arguments),
		})})
		projector.emitEvent(agentrun.Event{Type: "tool_result", Data: meta.appendTo(map[string]any{
			"id": callID, "provider_call_id": callID, "name": "skill", "status": "success",
			"content": invocation.Instructions,
		})})
	}
}

func (projector *PublicEventProjector) bindRunIDLocked(runID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	if projector.runID == "" {
		projector.runID = runID
	}
	if projector.request.PlanMode && projector.plan == nil {
		projector.plan = agentplan.NewParser(projector.rootMetadata().planMetadata(), planEventEmitter(projector.emitEvent))
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

// SummarizeRun closes one settled public Run without closing the surrounding
// product operation, which may continue with an already queued successor Run.
func (projector *PublicEventProjector) SummarizeRun(status agent.ResultStatus) {
	if projector == nil {
		return
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projector.flushLocked()
	projector.emitExecutionSummaryLocked(status)
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
	if status == agent.ResultAborted && strings.TrimSpace(reason) == agentrun.AbortReasonUserRequested {
		markInterruptionIfNeeded(projector.conversation, projector.request.Message, projector.content.String(), reason)
	}
	projector.emitExecutionSummaryLocked(status)
	switch status {
	case agent.ResultCompleted:
		projector.emitEvent(agentrun.Event{Type: "done", Data: map[string]string{}})
	case agent.ResultAborted:
		projector.emitEvent(agentrun.NewAbortedEvent(reason))
	default:
		data := map[string]string{"message": reason}
		if agent.IsModelIncompleteTerminalReason(reason) {
			data["code"] = reason
		}
		projector.emitEvent(agentrun.Event{Type: "error", Data: data})
	}
}

func (projector *PublicEventProjector) emitExecutionSummaryLocked(status agent.ResultStatus) {
	if projector.runSummarized || projector.runStartedAt.IsZero() || strings.TrimSpace(projector.runID) == "" {
		return
	}
	projector.runSummarized = true
	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(projector.runStartedAt)
	if duration < 0 {
		duration = 0
	}
	projector.emitEvent(agentrun.Event{Type: "execution_summary", Data: map[string]any{
		"run_id":          projector.runID,
		"run_started_at":  projector.runStartedAt.Format(time.RFC3339Nano),
		"run_finished_at": finishedAt.Format(time.RFC3339Nano),
		"duration_ms":     duration.Milliseconds(),
		"status":          string(status),
	}})
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

// TerminalProjected reports whether Finalize already delivered the task's
// terminal display event. Recovery observers use it to avoid adding a second
// fallback terminal when a resumed cycle owns a newly bound projector.
func (projector *PublicEventProjector) TerminalProjected() bool {
	if projector == nil {
		return false
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	return projector.terminal
}

// ProjectCanonicalOutput turns the raw final Agent message into the output
// already approved by Denova's public projection. Plan protocol tags stay
// display-only, while Game commits the narrative accumulated across model and
// tool steps instead of trusting only the final provider message.
func (projector *PublicEventProjector) ProjectCanonicalOutput(message *agent.Message) (*agent.Message, *agent.OutputProjection) {
	if projector == nil || message == nil {
		return nil, nil
	}
	projector.mu.Lock()
	defer projector.mu.Unlock()
	projected := message.Clone()
	if projector.request.PlanMode {
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
	if projector.options.AgentKind != agentrun.AgentKindInteractiveStory {
		return projected, nil
	}
	projector.finishInteractiveResponseLocked(nil)
	content := projector.content.String()
	if content == "" {
		return projected, nil
	}
	thinking := projector.thinking.String()
	projected.Content = content
	projected.ReasoningContent = thinking
	projected.ToolCalls = nil
	return projected, &agent.OutputProjection{Content: content, Thinking: thinking}
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
	return projector.metadata(firstNonEmpty(projector.runID, projector.options.TaskID), agent.EventSource{Name: projector.options.RootAgentName}, "")
}

func (projector *PublicEventProjector) metadata(runID string, source agent.EventSource, parentCallID string) agentEventMetadata {
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
	meta := agentEventMetadata{
		RunID:     firstNonEmpty(strings.TrimSpace(runID), projector.options.TaskID),
		AgentKind: projector.options.AgentKind, AgentName: name, RootAgentName: root,
		RunPath: path, SubAgent: subAgent, ParentCallID: strings.TrimSpace(parentCallID),
	}
	if subAgent {
		meta.SubAgentSessionID = strings.TrimSpace(source.InvocationID)
		meta.SubAgentType = firstNonEmpty(strings.TrimSpace(source.InvocationType), name)
	}
	return meta
}

func projectInteractionRequested(request agent.InteractionRequest, meta agentEventMetadata) agentrun.Event {
	toolCallID := request.ID
	kind := "question"
	questions := make([]map[string]any, len(request.Questions))
	for index, question := range request.Questions {
		options := make([]map[string]any, len(question.Options))
		recommended := ""
		for optionIndex, option := range question.Options {
			options[optionIndex] = map[string]any{
				"id": option.Value, "label": strings.TrimSpace(option.Label), "description": strings.TrimSpace(option.Description),
			}
			if option.Recommended && recommended == "" {
				recommended = option.Value
			}
		}
		questions[index] = map[string]any{
			"id": question.ID, "question": strings.TrimSpace(question.Prompt), "options": options,
			"multi_select": question.Multiple, "recommended_option_id": recommended,
		}
	}
	data := meta.appendTo(map[string]any{
		"schema": "ask.pending.v1", "id": request.ID, "kind": kind,
		"tool_call_id": toolCallID, "agent_kind": meta.AgentKind, "status": "pending",
		"questions": questions, "allow_other": request.AllowOther,
	})
	if request.Kind == agent.InteractionPermission && request.Permission != nil {
		permission := request.Permission
		kind = "tool_approval"
		toolCallID = firstNonEmpty(permission.CallID, request.ID)
		data["kind"] = kind
		data["tool_call_id"] = toolCallID
		data["questions"] = []map[string]any{}
		data["approval"] = map[string]any{
			"mode": permission.Mode, "tool_name": permission.Tool,
			"command": permission.Command, "details": permission.Details, "cwd": permission.Cwd,
			"risk": permission.Risk, "rule_id": permission.RuleID, "args_hash": permission.ArgsHash,
			"can_remember": permission.CanRemember, "rule_matcher_version": permission.RuleMatcherVersion,
			"rule_match_key": permission.RuleMatchKey, "rule_display_pattern": permission.RuleDisplayPattern,
		}
	}
	return agentrun.Event{Type: "ask_pending", Data: data}
}

func clonePublicEventData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	clone := make(map[string]any, len(data))
	for key, value := range data {
		clone[key] = value
	}
	return clone
}

func projectInteractionAnswers(request agent.InteractionRequest, resolution agent.InteractionResolution) []map[string]any {
	if request.Kind == agent.InteractionPermission && resolution.Permission != "" {
		option := "allow-once"
		switch resolution.Permission {
		case agent.PermissionRemember:
			option = "allow-workspace"
		case agent.PermissionDeny:
			option = "deny"
		}
		return []map[string]any{{
			"question_id": "tool-approval", "question": "工具授权 / Tool approval",
			"selected_options": []map[string]any{{"id": option, "label": option}},
		}}
	}
	questions := make(map[string]agent.InteractionQuestion, len(request.Questions))
	for _, question := range request.Questions {
		questions[question.ID] = question
	}
	answers := make([]map[string]any, 0, len(resolution.Answers))
	for _, answer := range resolution.Answers {
		question := questions[answer.QuestionID]
		labels := make(map[string]string, len(question.Options))
		for _, option := range question.Options {
			labels[option.Value] = strings.TrimSpace(option.Label)
		}
		selected := make([]map[string]any, 0, len(answer.Values))
		for _, value := range answer.Values {
			selected = append(selected, map[string]any{"id": value, "label": firstNonEmpty(labels[value], value)})
		}
		answers = append(answers, map[string]any{
			"question_id": answer.QuestionID, "question": strings.TrimSpace(question.Prompt),
			"selected_options": selected, "custom_input": strings.TrimSpace(answer.Text),
		})
	}
	return answers
}

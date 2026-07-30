package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	producttools "denova/internal/agents/tools"
)

func (l *chatAgentLoop) handleOutput(event *agent.AgentEvent) chatLoopResult {
	run := l.run
	eventMeta := run.subAgentSessions.decorate(metadataForAgentEvent(event, run.options.RootAgentName))
	eventMeta.AgentKind = run.options.AgentKind
	messageOutput := event.Output.MessageOutput
	if messageOutput.Role == agent.ToolRole {
		return l.handleToolOutput(messageOutput, eventMeta)
	}
	if messageOutput.Role != agent.Assistant && messageOutput.Role != "" {
		return chatLoopResult{action: chatLoopContinue}
	}
	return l.handleAssistantOutput(messageOutput, eventMeta)
}

func (l *chatAgentLoop) handleToolOutput(messageOutput *agent.MessageVariant, eventMeta agentEventMetadata) chatLoopResult {
	run := l.run
	if messageOutput.Message == nil {
		return chatLoopResult{action: chatLoopContinue}
	}
	content, drainErr := drainContent(l.ctx, messageOutput, run.options.IdleTimeout)
	if drainErr != nil {
		if errors.Is(drainErr, agent.ErrStreamCanceled) && run.control.hasTriggeredControl() {
			return chatLoopResult{action: chatLoopContinue}
		}
		return l.toolDrainFailed(drainErr)
	}

	fullToolContent := content
	if content == "" {
		content = "(无返回内容)"
	}
	message := messageOutput.Message
	executionID := strings.TrimSpace(messageOutput.ExecutionID)
	if executionID == "" {
		return l.toolDrainFailed(errors.New("tool result is missing an execution ID"))
	}
	providerCallID := firstNonEmpty(messageOutput.ProviderCallID, message.ToolCallID)
	completionKey := completedToolKey(eventMeta, message.ToolName, executionID)
	if _, completed := l.finishedTools[completionKey]; completed {
		// Typed completion already updated display in real completion order. The
		// source-ordered tool message is still the only cross-turn transcript input.
		if err := run.toolContext.RecordToolResult(message, eventMeta); err != nil {
			return l.toolDrainFailed(fmt.Errorf("persist bounded rich tool batch: %w", err))
		}
		delete(l.finishedTools, completionKey)
		return chatLoopResult{action: chatLoopContinue}
	}
	logToolResult(message.ToolName, executionID, content)
	run.usage.NoteToolResult(message.ToolName)
	data := eventMeta.appendTo(map[string]interface{}{
		"id":               executionID,
		"provider_call_id": providerCallID,
		"name":             message.ToolName,
		"content":          content,
	})
	if message.ToolResult != nil {
		data["status"] = string(message.ToolResult.Status)
		data["synthetic_reason"] = string(message.ToolResult.SyntheticReason)
	} else {
		// Legacy/custom tool messages without the structured summary predate
		// typed execution events. Preserve their historical success semantics;
		// native tools always carry an explicit status.
		data["status"] = string(agent.ToolResultSuccess)
	}
	if itemIDs, deletedIDs := parseWriteLoreItemsToolResult(message.ToolName, fullToolContent); len(itemIDs) > 0 || len(deletedIDs) > 0 {
		data["item_ids"] = itemIDs
		data["deleted_ids"] = deletedIDs
	}
	if illustrationResult, parseErr := producttools.ParseChapterIllustrationResult(message.ToolName, fullToolContent); parseErr != nil {
		run.logger.Warn("parse_chapter_illustration_result_failed", slog.String("tool", message.ToolName), slog.String("error_class", safeErrorClass(parseErr.Error())))
	} else if illustrationResult != nil {
		data["illustration"] = illustrationResult
		data["target"] = illustrationResult.MetaPath
	} else if interactiveImageResult, parseErr := producttools.ParseInteractiveImageResult(message.ToolName, fullToolContent); parseErr != nil {
		run.logger.Warn("parse_interactive_image_result_failed", slog.String("tool", message.ToolName), slog.String("error_class", safeErrorClass(parseErr.Error())))
	} else if interactiveImageResult != nil {
		data["interactive_image"] = interactiveImageResult
		data["target"] = interactiveImageResult.MetaPath
	} else if target := producttools.ParseGeneratedImageTarget(message.ToolName, fullToolContent); target != "" {
		data["target"] = target
	}
	if receipt, ok := producttools.ParseWorkspaceChangeReceipt(message.ToolName, fullToolContent); ok {
		data["workspace_change"] = receipt
		workspaceChangeData := eventMeta.appendTo(map[string]interface{}{
			"id":               receipt.ChangeSetID,
			"workspace":        receipt.Workspace,
			"change_group_id":  receipt.ChangeGroupID,
			"review_thread_id": receipt.ReviewThreadID,
			"change_set_id":    receipt.ChangeSetID,
			"path":             receipt.Path,
			"affected_paths":   []string{receipt.Path},
			"base_revision":    receipt.BaseRevision,
			"revision":         receipt.Revision,
			"review_status":    receipt.ReviewStatus,
			"apply_state":      receipt.ApplyState,
			"workspace_change": receipt,
		})
		run.emit(Event{Type: "workspace_change", Data: workspaceChangeData})
	}
	if err := run.toolContext.RecordToolResult(message, eventMeta); err != nil {
		return l.toolDrainFailed(fmt.Errorf("persist bounded rich tool batch: %w", err))
	}
	run.emit(Event{Type: "tool_result", Data: data})
	return chatLoopResult{action: chatLoopContinue}
}

func (l *chatAgentLoop) handleToolExecution(event *agent.AgentEvent) chatLoopResult {
	run := l.run
	execution := event.Output.ToolExecution
	eventMeta := run.subAgentSessions.decorate(metadataForAgentEvent(event, run.options.RootAgentName))
	eventMeta.AgentKind = run.options.AgentKind
	data := eventMeta.appendTo(map[string]interface{}{
		"id": execution.ExecutionID, "provider_call_id": execution.ProviderCallID,
		"name": execution.ToolName, "index": execution.Index,
	})
	descriptor := execution.Definition.Descriptor
	if descriptor.Execution != "" {
		data["source"] = string(descriptor.Source)
		data["mutation_scope"] = string(descriptor.MutationScope)
		data["post_check"] = string(descriptor.PostCheck)
		data["max_result_bytes"] = descriptor.MaxResultBytes
	}
	switch execution.Phase {
	case agent.ToolExecutionStarted:
		run.emit(Event{Type: "tool_started", Data: data})
	case agent.ToolExecutionProgress:
		data["delta"] = execution.Delta
		run.emit(Event{Type: "tool_progress", Data: data})
	case agent.ToolExecutionFinished:
		if execution.Result == nil || isPlanProtocolToolName(execution.ToolName) {
			return chatLoopResult{action: chatLoopContinue}
		}
		result := *execution.Result
		content := result.DisplayContent
		if content == "" {
			content = result.ModelContent
		}
		if content == "" {
			content = "(无返回内容)"
		}
		data["content"] = content
		data["status"] = string(result.Status)
		data["synthetic_reason"] = string(result.SyntheticReason)
		data["model_truncated"] = result.Metadata.ModelTruncated
		data["display_truncated"] = result.Metadata.DisplayTruncated
		data["target"] = result.Metadata.Target
		data["tool_result_original_tokens"] = estimatedToolResultTokens(int64(result.Metadata.OriginalModelBytes))
		data["tool_result_inline_tokens"] = estimatedToolResultTokens(int64(result.Metadata.ReturnedModelBytes))
		data["tool_result_retention_mode"] = string(result.ResultRetention)
		if result.ContextHints != nil {
			data["tool_result_context_value"] = string(result.ContextHints.ContextValue)
			data["tool_result_recovery_kind"] = string(result.ContextHints.Recovery.Kind)
		}
		data["artifact_count"] = len(result.Artifacts)
		if persistence := result.Metadata.ArtifactPersistence; persistence != nil {
			data["artifact_persist_attempted"] = persistence.Attempted
			data["artifact_persist_complete"] = persistence.Complete
			data["artifact_persist_failure_reason"] = persistence.FailureReason
			if persistence.Attempted && !persistence.Complete {
				data["artifact_persist_failure_count"] = 1
			}
		}
		if result.Status == agent.ToolResultSuccess && isWorkspaceArtifactRead(execution.ToolName, result.Metadata.Target) {
			data["artifact_reread_count"] = 1
		}
		payload := result.ModelContent
		if len(result.Details) != 0 && json.Valid(result.Details) {
			payload = string(result.Details)
		}
		populateToolResultDomainData(run, execution.ToolName, payload, eventMeta, data)
		run.usage.NoteToolResult(execution.ToolName)
		logToolResult(execution.ToolName, execution.ExecutionID, content)
		if execution.ExecutionID != "" {
			l.finishedTools[completedToolKey(eventMeta, execution.ToolName, execution.ExecutionID)] = struct{}{}
		}
		run.emit(Event{Type: "tool_result", Data: data})
	}
	return chatLoopResult{action: chatLoopContinue}
}

func isWorkspaceArtifactRead(toolName, target string) bool {
	if strings.TrimSpace(toolName) != "read" {
		return false
	}
	path := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(target), "\\", "/"))
	return strings.HasPrefix(path, ".denova/artifacts/") || strings.HasPrefix(path, ".nova/artifacts/") ||
		strings.Contains(path, "/.denova/artifacts/") || strings.Contains(path, "/.nova/artifacts/") ||
		strings.Contains(path, ".jsonl.artifacts/") || strings.Contains(path, "/artifacts/game/")
}

func completedToolKey(meta agentEventMetadata, toolName, callID string) string {
	parts := []string{
		meta.RunID,
		meta.RootAgentName,
		meta.AgentName,
		meta.SubAgentSessionID,
		strings.Join(meta.RunPath, "\x1f"),
		strings.TrimSpace(toolName),
		strings.TrimSpace(callID),
	}
	return strings.Join(parts, "\x1e")
}

func populateToolResultDomainData(run *chatRun, toolName, payload string, eventMeta agentEventMetadata, data map[string]interface{}) {
	if itemIDs, deletedIDs := parseWriteLoreItemsToolResult(toolName, payload); len(itemIDs) > 0 || len(deletedIDs) > 0 {
		data["item_ids"] = itemIDs
		data["deleted_ids"] = deletedIDs
	}
	if illustrationResult, parseErr := producttools.ParseChapterIllustrationResult(toolName, payload); parseErr != nil {
		run.logger.Warn("parse_chapter_illustration_result_failed", slog.String("tool", toolName), slog.String("error_class", safeErrorClass(parseErr.Error())))
	} else if illustrationResult != nil {
		data["illustration"] = illustrationResult
		data["target"] = illustrationResult.MetaPath
	} else if interactiveImageResult, parseErr := producttools.ParseInteractiveImageResult(toolName, payload); parseErr != nil {
		run.logger.Warn("parse_interactive_image_result_failed", slog.String("tool", toolName), slog.String("error_class", safeErrorClass(parseErr.Error())))
	} else if interactiveImageResult != nil {
		data["interactive_image"] = interactiveImageResult
		data["target"] = interactiveImageResult.MetaPath
	} else if target := producttools.ParseGeneratedImageTarget(toolName, payload); target != "" {
		data["target"] = target
	}
	if receipt, ok := producttools.ParseWorkspaceChangeReceipt(toolName, payload); ok {
		data["workspace_change"] = receipt
		workspaceChangeData := eventMeta.appendTo(map[string]interface{}{
			"id": receipt.ChangeSetID, "workspace": receipt.Workspace,
			"change_group_id": receipt.ChangeGroupID, "review_thread_id": receipt.ReviewThreadID,
			"change_set_id": receipt.ChangeSetID, "path": receipt.Path,
			"affected_paths": []string{receipt.Path}, "base_revision": receipt.BaseRevision,
			"revision": receipt.Revision, "review_status": receipt.ReviewStatus,
			"apply_state": receipt.ApplyState, "workspace_change": receipt,
		})
		run.emit(Event{Type: "workspace_change", Data: workspaceChangeData})
	}
}

func (l *chatAgentLoop) toolDrainFailed(drainErr error) chatLoopResult {
	run := l.run
	before := run.fullAssistantOutputSnapshot()
	discardPlanAssistantContentIfNeeded(run.req.PlanMode, l.planParser, &run.fullContent, &run.fullThinking)
	run.captureEffectiveAssistantDelta(before)
	terminalContent, terminalThinking := run.snapshotOutput()
	if run.ctx.Err() != nil {
		err := run.ctx.Err()
		run.logger.Warn("run_interrupted", slog.String("reason", "context"), slog.String("error_class", safeErrorClass(err.Error())), slog.Int("generated_bytes", len(terminalContent)))
		run.finish("aborted", err.Error(), len(terminalContent))
		run.emit(Event{Type: "aborted", Data: map[string]string{}})
		return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeAborted, err, err.Error(), terminalContent, terminalThinking)}
	}
	l.cancel()
	run.logger.Error("run_interrupted", slog.String("reason", "tool_result_idle_timeout"), slog.String("error_class", safeErrorClass(drainErr.Error())), slog.Int("generated_bytes", len(terminalContent)))
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, drainErr.Error())
	run.finish("error", drainErr.Error(), len(terminalContent))
	run.emit(Event{Type: "error", Data: map[string]string{"message": drainErr.Error()}})
	return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeFailed, drainErr, drainErr.Error(), terminalContent, terminalThinking)}
}

func (l *chatAgentLoop) handleAssistantOutput(messageOutput *agent.MessageVariant, eventMeta agentEventMetadata) chatLoopResult {
	run := l.run
	if messageOutput.IsStreaming && messageOutput.MessageStream != nil {
		before := run.fullAssistantOutputSnapshot()
		msg, streamErr := processStreamingEvent(l.ctx, messageOutput, &run.fullContent, &run.fullThinking, run.options.IdleTimeout, run.options.ToolResultMaxBytes, eventMeta, l.planParser, run.emit)
		run.captureEffectiveAssistantDelta(before)
		if streamErr != nil {
			// Completion-guard retries arrive after response frames. Preserve the
			// rejected call's provider usage even though its prose is discarded.
			run.usage.AddMessage(msg)
			if reason, retrying := interactiveCompletionRetryFromError(streamErr); retrying {
				run.logger.Info("interactive_completion_retry", slog.String("code", reason.Code), slog.Int("generated_bytes", run.fullContent.Len()))
				return chatLoopResult{action: chatLoopContinue}
			}
			if errors.Is(streamErr, agent.ErrStreamCanceled) && run.control.hasTriggeredControl() {
				return chatLoopResult{action: chatLoopContinue}
			}
			return l.assistantStreamFailed(streamErr)
		}
		run.toolContext.RecordAssistantToolCalls(msg, eventMeta)
		run.usage.AddMessage(msg)
		if run.req.PlanMode && l.planParser != nil && l.planParser.HasSuccessfulBlock() {
			l.cancel()
			return chatLoopResult{action: chatLoopStop}
		}
		return chatLoopResult{action: chatLoopContinue}
	}
	if messageOutput.Message == nil {
		return chatLoopResult{action: chatLoopContinue}
	}
	before := run.fullAssistantOutputSnapshot()
	processErr := processNonStreamingEvent(messageOutput, &run.fullContent, &run.fullThinking, run.options.ToolResultMaxBytes, eventMeta, l.planParser, run.emit)
	run.captureEffectiveAssistantDelta(before)
	if processErr != nil {
		return l.assistantStreamFailed(processErr)
	}
	run.toolContext.RecordAssistantToolCalls(messageOutput.Message, eventMeta)
	run.usage.AddMessage(messageOutput.Message)
	if run.req.PlanMode && l.planParser != nil && l.planParser.HasSuccessfulBlock() {
		l.cancel()
		return chatLoopResult{action: chatLoopStop}
	}
	return chatLoopResult{action: chatLoopContinue}
}

func (l *chatAgentLoop) assistantStreamFailed(streamErr error) chatLoopResult {
	run := l.run
	l.flushPlanOutput()
	terminalContent, terminalThinking := run.snapshotOutput()
	if run.ctx.Err() != nil {
		err := run.ctx.Err()
		run.logger.Warn("run_interrupted", slog.String("reason", "context"), slog.String("error_class", safeErrorClass(err.Error())), slog.Int("generated_bytes", len(terminalContent)))
		run.finish("aborted", err.Error(), len(terminalContent))
		run.emit(Event{Type: "aborted", Data: map[string]string{}})
		return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeAborted, err, err.Error(), terminalContent, terminalThinking)}
	}
	l.cancel()
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, streamErr.Error())
	run.finish("error", streamErr.Error(), len(terminalContent))
	return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeFailed, streamErr, streamErr.Error(), terminalContent, terminalThinking)}
}

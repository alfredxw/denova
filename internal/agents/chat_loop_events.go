package agents

import (
	"errors"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"
)

func (l *chatAgentLoop) handleOutput(event *agent.AgentEvent) chatLoopResult {
	run := l.run
	eventMeta := run.subAgentSessions.decorate(metadataForAgentEvent(event, run.options.RootAgentName))
	eventMeta.AgentKind = run.options.AgentKind
	messageOutput := event.Output.MessageOutput
	if messageOutput.Role == agent.Tool {
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
	logToolResult(message.ToolName, message.ToolCallID, content)
	run.usage.NoteToolResult(message.ToolName)
	data := eventMeta.appendTo(map[string]interface{}{
		"id":      message.ToolCallID,
		"name":    message.ToolName,
		"content": content,
	})
	if itemIDs, deletedIDs := parseWriteLoreItemsToolResult(message.ToolName, fullToolContent); len(itemIDs) > 0 || len(deletedIDs) > 0 {
		data["item_ids"] = itemIDs
		data["deleted_ids"] = deletedIDs
	}
	if illustrationResult, parseErr := parseChapterIllustrationToolResult(message.ToolName, fullToolContent); parseErr != nil {
		run.logger.Warn("parse_chapter_illustration_result_failed", slog.String("tool", message.ToolName), slog.Any("error", parseErr))
	} else if illustrationResult != nil {
		data["illustration"] = illustrationResult
		data["target"] = illustrationResult.MetaPath
	} else if interactiveImageResult, parseErr := parseInteractiveImageToolResult(message.ToolName, fullToolContent); parseErr != nil {
		run.logger.Warn("parse_interactive_image_result_failed", slog.String("tool", message.ToolName), slog.Any("error", parseErr))
	} else if interactiveImageResult != nil {
		data["interactive_image"] = interactiveImageResult
		data["target"] = interactiveImageResult.MetaPath
	} else if target := parseGeneratedImageToolTarget(message.ToolName, fullToolContent); target != "" {
		data["target"] = target
	}
	if receipt, ok := parseWorkspaceChangeToolReceipt(message.ToolName, fullToolContent); ok {
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
	run.toolContext.RecordToolResult(message.ToolName, message.ToolCallID, content, eventMeta)
	run.emit(Event{Type: "tool_result", Data: data})
	return chatLoopResult{action: chatLoopContinue}
}

func (l *chatAgentLoop) toolDrainFailed(drainErr error) chatLoopResult {
	run := l.run
	discardPlanAssistantContentIfNeeded(run.req.PlanMode, l.planParser, &run.fullContent, &run.fullThinking)
	terminalContent, terminalThinking := run.snapshotOutput()
	if run.ctx.Err() != nil {
		err := run.ctx.Err()
		run.logger.Warn("run_interrupted", slog.String("reason", "context"), slog.Any("error", err), slog.Int("generated_bytes", len(terminalContent)))
		run.finish("aborted", err.Error(), len(terminalContent))
		run.emit(Event{Type: "aborted", Data: map[string]string{}})
		return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeAborted, err, err.Error(), terminalContent, terminalThinking)}
	}
	l.cancel()
	run.logger.Error("run_interrupted", slog.String("reason", "tool_result_idle_timeout"), slog.Any("error", drainErr), slog.Int("generated_bytes", len(terminalContent)))
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, drainErr.Error())
	run.finish("error", drainErr.Error(), len(terminalContent))
	run.emit(Event{Type: "error", Data: map[string]string{"message": drainErr.Error()}})
	return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeFailed, drainErr, drainErr.Error(), terminalContent, terminalThinking)}
}

func (l *chatAgentLoop) handleAssistantOutput(messageOutput *agent.MessageVariant, eventMeta agentEventMetadata) chatLoopResult {
	run := l.run
	if messageOutput.IsStreaming && messageOutput.MessageStream != nil {
		msg, streamErr := processStreamingEvent(l.ctx, messageOutput, &run.fullContent, &run.fullThinking, run.options.IdleTimeout, run.options.ToolResultMaxBytes, eventMeta, l.planParser, run.emit)
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
	processNonStreamingEvent(messageOutput, &run.fullContent, &run.fullThinking, run.options.ToolResultMaxBytes, eventMeta, l.planParser, run.emit)
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
		run.logger.Warn("run_interrupted", slog.String("reason", "context"), slog.Any("error", err), slog.Int("generated_bytes", len(terminalContent)))
		run.finish("aborted", err.Error(), len(terminalContent))
		run.emit(Event{Type: "aborted", Data: map[string]string{}})
		return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeAborted, err, err.Error(), terminalContent, terminalThinking)}
	}
	l.cancel()
	markInterruptionIfNeeded(run.conversation, run.resumeInterruption, run.originalMessage, terminalContent, streamErr.Error())
	run.finish("error", streamErr.Error(), len(terminalContent))
	return chatLoopResult{action: chatLoopTerminal, outcome: outcomeFromOutput(RunOutcomeFailed, streamErr, streamErr.Error(), terminalContent, terminalThinking)}
}

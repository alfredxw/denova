package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
)

var ErrInterruptionNotPending = errors.New("requested interruption is no longer pending")

func markInterruptionIfNeeded(conversation Conversation, userMessage, assistantContent, reason string) {
	if conversation == nil {
		return
	}
	if err := conversation.MarkInterrupted(userMessage, assistantContent, reason); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-run] mark interruption failed err=%v", err))
	}
}

// ResolveRequestedInterruption returns the exact checkpoint requested by the
// caller. A natural-language Continue remains supported, while UI-driven
// resume uses an ID so concurrent tabs cannot resume the wrong interruption.
func ResolveRequestedInterruption(request ChatRequest, pending *session.Interruption) (*session.Interruption, error) {
	id := strings.TrimSpace(request.ResumeInterruptionID)
	if id != "" {
		if pending == nil || strings.TrimSpace(pending.ID) != id || pending.Status != session.InterruptionPending {
			return nil, fmt.Errorf("%w: interruption_id=%q", ErrInterruptionNotPending, id)
		}
		return pending, nil
	}
	if shouldResumeInterruptedRequest(request.Message) {
		return pending, nil
	}
	return nil, nil
}

func requestResumesInterruption(request ChatRequest) bool {
	return strings.TrimSpace(request.ResumeInterruptionID) != "" || shouldResumeInterruptedRequest(request.Message)
}

func shouldResumeInterruptedRequest(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	switch trimmed {
	case "继续", "继续。", "继续！", "继续写", "继续生成", "继续完成", "接着来", "接着写", "续上", "继续刚才":
		return true
	}
	lower := strings.ToLower(strings.TrimRight(trimmed, ".!"))
	return lower == "continue" || lower == "resume" || lower == "keep going" || lower == "continue writing" || lower == "continue generation" ||
		strings.HasPrefix(trimmed, "继续刚才") || strings.HasPrefix(trimmed, "继续之前") || strings.HasPrefix(trimmed, "从中断的地方继续")
}

func buildInterruptedResumeMessage(current string, interrupted *session.Interruption) string {
	if interrupted == nil {
		return current
	}
	return prompts.ResumeFromInterruption(current, prompts.InterruptedResume{
		UserMessage:      interrupted.UserMessage,
		AssistantContent: interrupted.AssistantContent,
		Reason:           interrupted.Reason,
	})
}

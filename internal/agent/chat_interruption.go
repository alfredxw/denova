package agent

import (
	"log"
	"strings"

	"nova/internal/prompts"
	"nova/internal/session"
)

func markInterruptionIfNeeded(conversation Conversation, resumed *session.Interruption, userMessage, assistantContent, reason string) {
	if resumed != nil {
		return
	}
	if err := conversation.MarkInterrupted(userMessage, assistantContent, reason); err != nil {
		log.Printf("[agent-run] mark interruption failed err=%v", err)
	}
}

func shouldResumeInterruptedRequest(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	switch trimmed {
	case "继续", "继续。", "继续！", "接着来", "接着写", "续上", "继续刚才":
		return true
	}
	return strings.HasPrefix(trimmed, "继续刚才") || strings.HasPrefix(trimmed, "继续之前") || strings.HasPrefix(trimmed, "从中断的地方继续")
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

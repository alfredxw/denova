package agents

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// Message is the product-facing alias of Agent's stable model message. App and
// transport layers depend on Agent composition, while provider/session code can
// use the same wire without exposing an implementation-specific framework.
type Message = agent.Message

// ToolArtifactStore is the stable facade used by application conversations
// without coupling them directly to the underlying Agent module package.
type ToolArtifactStore = agent.ToolArtifactStore
type ToolArtifactBackend = agent.ToolArtifactBackend

type Role = agent.RoleType
type ToolCall = agent.ToolCall
type FunctionCall = agent.FunctionCall
type ToolInfo = agent.ToolInfo
type StreamReader[T any] = agent.StreamReader[T]

const (
	RoleSystem    = agent.System
	RoleUser      = agent.User
	RoleAssistant = agent.Assistant
	RoleTool      = agent.ToolRole
)

var (
	SystemMessage    = agent.SystemMessage
	UserMessage      = agent.UserMessage
	AssistantMessage = agent.AssistantMessage
	ToolMessage      = agent.ToolMessage
	TextToolResult   = agent.TextToolResult
	WithToolName     = agent.WithToolName
)

func StreamReaderFromArray[T any](values []T) *StreamReader[T] {
	return agent.StreamReaderFromArray(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

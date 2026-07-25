package agents

import agent "github.com/alfredxw/denova/agent"

// Message is the product-facing alias of Agent's stable model message. App and
// transport layers depend on Agent composition, while provider/session code can
// use the same wire without exposing an implementation-specific framework.
type Message = agent.Message

type Role = agent.RoleType
type ToolCall = agent.ToolCall
type FunctionCall = agent.FunctionCall
type ToolInfo = agent.ToolInfo
type StreamReader[T any] = agent.StreamReader[T]

// Runner and Runnable keep the application layer coupled to Denova's Agent
// facade instead of the concrete Agent module.
type Runner = agent.Runner
type Runnable = agent.Runnable

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

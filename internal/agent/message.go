package agent

import "github.com/alfredxw/denova/adk"

// Message is the product-facing alias of ADK's stable model message. App and
// transport layers depend on Agent composition, while provider/session code can
// use the same wire without exposing an implementation-specific framework.
type Message = adk.Message

type Role = adk.RoleType
type ToolCall = adk.ToolCall
type FunctionCall = adk.FunctionCall
type ToolInfo = adk.ToolInfo
type StreamReader[T any] = adk.StreamReader[T]

// Runner and Runnable keep the application layer coupled to Denova's Agent
// facade instead of the concrete ADK module.
type Runner = adk.Runner
type Runnable = adk.Runnable

const (
	RoleSystem    = adk.System
	RoleUser      = adk.User
	RoleAssistant = adk.Assistant
	RoleTool      = adk.Tool
)

var (
	SystemMessage    = adk.SystemMessage
	UserMessage      = adk.UserMessage
	AssistantMessage = adk.AssistantMessage
	ToolMessage      = adk.ToolMessage
	WithToolName     = adk.WithToolName
)

func StreamReaderFromArray[T any](values []T) *StreamReader[T] {
	return adk.StreamReaderFromArray(values)
}

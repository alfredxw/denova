// Package adk provides Denova's provider-neutral agent development core.
//
// The package is intentionally layered around stable seams:
//
//   - Message and StreamReader define persisted and streaming wire data.
//   - BaseChatModel and BaseTool isolate provider and tool implementations.
//   - Agent owns the native model/tool loop; Runner is only its entry point.
//   - Middleware and Host expose runtime integration without importing a
//     provider SDK, a workflow engine, or Denova internal packages.
//
// The package does not provide durable checkpoint storage. Hosts that need
// persistence should record AgentEvent values and their own domain state.
package adk
